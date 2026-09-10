package db

import (
	"database/sql"
	"errors"
	"fmt"

	"llm-gateway/internal/model"

	"github.com/google/uuid"
)

// ============================================================================
// Virtual Models & Dynamic Routing Repositories (v0.3.0)
// ============================================================================

func (p *PostgresDB) CreateVirtualModel(vm *model.VirtualModel) error {
	if vm.ID == uuid.Nil {
		vm.ID = uuid.New()
	}
	if vm.RoutingStrategy == "" {
		vm.RoutingStrategy = "weighted"
	}
	tx, err := p.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO virtual_models (id, name, description, tenant_id, routing_strategy, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING created_at, updated_at`
	err = tx.QueryRow(query, vm.ID, vm.Name, vm.Description, vm.TenantID, vm.RoutingStrategy, vm.IsActive).
		Scan(&vm.CreatedAt, &vm.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert virtual_model: %w", err)
	}

	for i := range vm.Targets {
		t := &vm.Targets[i]
		t.VirtualModelID = vm.ID
		if t.Weight <= 0 {
			t.Weight = 100
		}
		if t.Priority <= 0 {
			t.Priority = 1
		}
		targetQuery := `
			INSERT INTO virtual_model_targets (virtual_model_id, provider, model_name, weight, priority, is_active, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
			RETURNING id, created_at, updated_at`
		err = tx.QueryRow(targetQuery, vm.ID, t.Provider, t.ModelName, t.Weight, t.Priority, t.IsActive).
			Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			return fmt.Errorf("insert virtual_model_target: %w", err)
		}
	}

	return tx.Commit()
}

func (p *PostgresDB) GetVirtualModelByID(id uuid.UUID) (*model.VirtualModel, error) {
	vm := &model.VirtualModel{}
	query := `
		SELECT id, name, description, tenant_id, routing_strategy, is_active, created_at, updated_at
		FROM virtual_models
		WHERE id = $1`
	err := p.QueryRow(query, id).Scan(
		&vm.ID, &vm.Name, &vm.Description, &vm.TenantID, &vm.RoutingStrategy, &vm.IsActive, &vm.CreatedAt, &vm.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	targets, err := p.getVirtualModelTargets(vm.ID)
	if err != nil {
		return nil, err
	}
	vm.Targets = targets
	return vm, nil
}

func (p *PostgresDB) GetVirtualModelByName(name string, tenantID *uuid.UUID) (*model.VirtualModel, error) {
	vm := &model.VirtualModel{}
	var query string
	var err error

	// 1. First check tenant-scoped virtual model if tenantID provided
	if tenantID != nil && *tenantID != uuid.Nil {
		query = `
			SELECT id, name, description, tenant_id, routing_strategy, is_active, created_at, updated_at
			FROM virtual_models
			WHERE name = $1 AND tenant_id = $2 AND is_active = TRUE`
		err = p.QueryRow(query, name, *tenantID).Scan(
			&vm.ID, &vm.Name, &vm.Description, &vm.TenantID, &vm.RoutingStrategy, &vm.IsActive, &vm.CreatedAt, &vm.UpdatedAt,
		)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	// 2. Fallback to global virtual model (tenant_id IS NULL)
	if vm.ID == uuid.Nil {
		query = `
			SELECT id, name, description, tenant_id, routing_strategy, is_active, created_at, updated_at
			FROM virtual_models
			WHERE name = $1 AND tenant_id IS NULL AND is_active = TRUE`
		err = p.QueryRow(query, name).Scan(
			&vm.ID, &vm.Name, &vm.Description, &vm.TenantID, &vm.RoutingStrategy, &vm.IsActive, &vm.CreatedAt, &vm.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
	}

	targets, err := p.getVirtualModelTargets(vm.ID)
	if err != nil {
		return nil, err
	}
	vm.Targets = targets
	return vm, nil
}

func (p *PostgresDB) getVirtualModelTargets(virtualModelID uuid.UUID) ([]model.VirtualModelTarget, error) {
	query := `
		SELECT id, virtual_model_id, provider, model_name, weight, priority, is_active, created_at, updated_at
		FROM virtual_model_targets
		WHERE virtual_model_id = $1
		ORDER BY priority ASC, id ASC`
	rows, err := p.Query(query, virtualModelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []model.VirtualModelTarget
	for rows.Next() {
		var t model.VirtualModelTarget
		if err := rows.Scan(
			&t.ID, &t.VirtualModelID, &t.Provider, &t.ModelName, &t.Weight, &t.Priority, &t.IsActive, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	if targets == nil {
		targets = []model.VirtualModelTarget{}
	}
	return targets, nil
}

func (p *PostgresDB) ListVirtualModels(tenantID *uuid.UUID) ([]model.VirtualModel, error) {
	var query string
	var rows *sql.Rows
	var err error

	if tenantID != nil && *tenantID != uuid.Nil {
		query = `
			SELECT id, name, description, tenant_id, routing_strategy, is_active, created_at, updated_at
			FROM virtual_models
			WHERE tenant_id = $1 OR tenant_id IS NULL
			ORDER BY name ASC`
		rows, err = p.Query(query, *tenantID)
	} else {
		query = `
			SELECT id, name, description, tenant_id, routing_strategy, is_active, created_at, updated_at
			FROM virtual_models
			ORDER BY name ASC`
		rows, err = p.Query(query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.VirtualModel
	for rows.Next() {
		var vm model.VirtualModel
		if err := rows.Scan(
			&vm.ID, &vm.Name, &vm.Description, &vm.TenantID, &vm.RoutingStrategy, &vm.IsActive, &vm.CreatedAt, &vm.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, vm)
	}

	for i := range list {
		targets, err := p.getVirtualModelTargets(list[i].ID)
		if err != nil {
			return nil, err
		}
		list[i].Targets = targets
	}

	if list == nil {
		list = []model.VirtualModel{}
	}
	return list, nil
}

func (p *PostgresDB) UpdateVirtualModel(vm *model.VirtualModel) error {
	tx, err := p.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		UPDATE virtual_models
		SET name = $2, description = $3, tenant_id = $4, routing_strategy = $5, is_active = $6, updated_at = NOW()
		WHERE id = $1`
	res, err := tx.Exec(query, vm.ID, vm.Name, vm.Description, vm.TenantID, vm.RoutingStrategy, vm.IsActive)
	if err != nil {
		return fmt.Errorf("update virtual_model: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}

	if vm.Targets != nil {
		if _, err := tx.Exec(`DELETE FROM virtual_model_targets WHERE virtual_model_id = $1`, vm.ID); err != nil {
			return fmt.Errorf("delete old targets: %w", err)
		}
		for i := range vm.Targets {
			t := &vm.Targets[i]
			t.VirtualModelID = vm.ID
			if t.Weight <= 0 {
				t.Weight = 100
			}
			if t.Priority <= 0 {
				t.Priority = 1
			}
			targetQuery := `
				INSERT INTO virtual_model_targets (virtual_model_id, provider, model_name, weight, priority, is_active, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
				RETURNING id, created_at, updated_at`
			err = tx.QueryRow(targetQuery, vm.ID, t.Provider, t.ModelName, t.Weight, t.Priority, t.IsActive).
				Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
			if err != nil {
				return fmt.Errorf("insert target: %w", err)
			}
		}
	}

	return tx.Commit()
}

func (p *PostgresDB) DeleteVirtualModel(id uuid.UUID) error {
	res, err := p.Exec(`DELETE FROM virtual_models WHERE id = $1`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CheckModelNameInCatalog reports whether modelName already exists as a real provider model
// in either model_pricings or provider_models. Returns (exists, providerName, error).
func (p *PostgresDB) CheckModelNameInCatalog(modelName string) (bool, string, error) {
	var prov string
	query := `
		SELECT provider FROM model_pricings WHERE LOWER(model_name) = LOWER($1)
		UNION
		SELECT provider FROM provider_models WHERE LOWER(model_name) = LOWER($1)
		LIMIT 1`
	err := p.QueryRow(query, modelName).Scan(&prov)
	if err == nil {
		return true, prov, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	return false, "", err
}
