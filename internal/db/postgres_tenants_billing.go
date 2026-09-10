package db

import (
	"fmt"
	"strings"

	"llm-gateway/internal/model"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ---------------------------------------------------------------------------
// Multi-Tenant B2B Database Operations
// ---------------------------------------------------------------------------

func (p *PostgresDB) CreateTenant(t *model.Tenant) error {
	t.ID = uuid.New()
	if t.AllowedModels == nil {
		t.AllowedModels = []string{"*"}
	}
	if t.BillingMode == "" {
		t.BillingMode = "prepaid"
	}
	if t.Type == "" {
		t.Type = "external"
	}

	query := `
		INSERT INTO tenants (id, name, type, billing_mode, credit_balance, budget_limit_monthly, allowed_models, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`
	return p.QueryRow(query, t.ID, t.Name, t.Type, t.BillingMode, t.CreditBalance, t.BudgetLimitMonthly, pq.Array(t.AllowedModels), t.IsActive).
		Scan(&t.CreatedAt, &t.UpdatedAt)
}

func (p *PostgresDB) GetTenantByID(id uuid.UUID) (*model.Tenant, error) {
	query := `
		SELECT id, name, type, billing_mode, credit_balance, budget_limit_monthly, allowed_models, is_active, created_at, updated_at
		FROM tenants WHERE id = $1`
	var t model.Tenant
	err := p.QueryRow(query, id).Scan(
		&t.ID, &t.Name, &t.Type, &t.BillingMode, &t.CreditBalance, &t.BudgetLimitMonthly, pq.Array(&t.AllowedModels), &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (p *PostgresDB) ListTenants() ([]model.Tenant, error) {
	query := `
		SELECT id, name, type, billing_mode, credit_balance, budget_limit_monthly, allowed_models, is_active, created_at, updated_at
		FROM tenants ORDER BY created_at DESC`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []model.Tenant
	for rows.Next() {
		var t model.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Type, &t.BillingMode, &t.CreditBalance, &t.BudgetLimitMonthly, pq.Array(&t.AllowedModels), &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	}
	return tenants, nil
}

func (p *PostgresDB) UpdateTenant(id uuid.UUID, patch *model.TenantPatch) error {
	sets := []string{"updated_at = NOW()"}
	args := []interface{}{id}
	idx := 2

	if patch.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", idx))
		args = append(args, *patch.Name)
		idx++
	}
	if patch.Type != nil {
		sets = append(sets, fmt.Sprintf("type = $%d", idx))
		args = append(args, *patch.Type)
		idx++
	}
	if patch.BillingMode != nil {
		sets = append(sets, fmt.Sprintf("billing_mode = $%d", idx))
		args = append(args, *patch.BillingMode)
		idx++
	}
	if patch.BudgetLimitMonthly != nil {
		sets = append(sets, fmt.Sprintf("budget_limit_monthly = $%d", idx))
		args = append(args, *patch.BudgetLimitMonthly)
		idx++
	}
	if patch.AllowedModels != nil {
		sets = append(sets, fmt.Sprintf("allowed_models = $%d", idx))
		args = append(args, pq.Array(*patch.AllowedModels))
		idx++
	}
	if patch.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", idx))
		args = append(args, *patch.IsActive)
		idx++
	}

	query := fmt.Sprintf("UPDATE tenants SET %s WHERE id = $1", strings.Join(sets, ", "))
	_, err := p.Exec(query, args...)
	return err
}

func (p *PostgresDB) DeleteTenant(id uuid.UUID) error {
	_, err := p.Exec(`DELETE FROM tenants WHERE id = $1`, id)
	return err
}

func (p *PostgresDB) TopUpTenantCredit(tenantID uuid.UUID, amount float64, txType, contractRef, notes string, createdBy *uuid.UUID) (*model.TenantCreditTransaction, error) {
	tx, err := p.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var newBalance float64
	err = tx.QueryRow(`
		UPDATE tenants
		SET credit_balance = credit_balance + $1, updated_at = NOW()
		WHERE id = $2
		RETURNING credit_balance`, amount, tenantID).Scan(&newBalance)
	if err != nil {
		return nil, fmt.Errorf("failed to update tenant balance: %w", err)
	}

	var transaction model.TenantCreditTransaction
	transaction.TenantID = tenantID
	transaction.Amount = amount
	transaction.BalanceAfter = newBalance
	transaction.Type = txType
	transaction.ContractRef = contractRef
	transaction.Notes = notes
	transaction.CreatedBy = createdBy

	err = tx.QueryRow(`
		INSERT INTO tenant_credit_transactions (tenant_id, amount, balance_after, type, contract_ref, notes, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`,
		tenantID, amount, newBalance, txType, contractRef, notes, createdBy,
	).Scan(&transaction.ID, &transaction.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to record credit transaction: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &transaction, nil
}

func (p *PostgresDB) DeductTenantCredit(tenantID uuid.UUID, cost float64) (float64, error) {
	var newBalance float64
	err := p.QueryRow(`
		UPDATE tenants
		SET credit_balance = credit_balance - $1, updated_at = NOW()
		WHERE id = $2
		RETURNING credit_balance`, cost, tenantID).Scan(&newBalance)
	if err != nil {
		return 0, err
	}
	return newBalance, nil
}

func (p *PostgresDB) ListTenantTransactions(tenantID uuid.UUID, limit, offset int) ([]model.TenantCreditTransaction, int, error) {
	if limit <= 0 {
		limit = 50
	}
	var total int
	err := p.QueryRow(`SELECT COUNT(*) FROM tenant_credit_transactions WHERE tenant_id = $1`, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, tenant_id, amount, balance_after, type, COALESCE(contract_ref, ''), COALESCE(notes, ''), created_by, created_at
		FROM tenant_credit_transactions
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`
	rows, err := p.Query(query, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var txs []model.TenantCreditTransaction
	for rows.Next() {
		var tx model.TenantCreditTransaction
		if err := rows.Scan(&tx.ID, &tx.TenantID, &tx.Amount, &tx.BalanceAfter, &tx.Type, &tx.ContractRef, &tx.Notes, &tx.CreatedBy, &tx.CreatedAt); err != nil {
			return nil, 0, err
		}
		txs = append(txs, tx)
	}
	return txs, total, nil
}

func (p *PostgresDB) GetTenantsWithLowCredits(threshold float64) ([]model.Tenant, error) {
	query := `
		SELECT id, name, type, billing_mode, credit_balance, budget_limit_monthly,
		       allowed_models, is_active, created_at, updated_at
		FROM tenants
		WHERE billing_mode = 'prepaid' AND is_active = TRUE AND credit_balance <= $1
		ORDER BY credit_balance ASC`

	rows, err := p.Query(query, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Tenant
	for rows.Next() {
		var t model.Tenant
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Type, &t.BillingMode, &t.CreditBalance, &t.BudgetLimitMonthly,
			pq.Array(&t.AllowedModels), &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, nil
}
