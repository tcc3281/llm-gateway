package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"llm-gateway/internal/model"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ---------------------------------------------------------------------------
// Plugins & Extensibility (v0.3.0)
// ---------------------------------------------------------------------------

func (p *PostgresDB) ListPlugins() ([]model.Plugin, error) {
	query := `
		SELECT id, name, display_name, description, category, hooks, config_schema, is_builtin, version, created_at, updated_at
		FROM plugins
		ORDER BY category ASC, name ASC`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Plugin
	for rows.Next() {
		var pl model.Plugin
		var hooks []string
		if err := rows.Scan(
			&pl.ID, &pl.Name, &pl.DisplayName, &pl.Description, &pl.Category,
			pq.Array(&hooks), &pl.ConfigSchema, &pl.IsBuiltin, &pl.Version,
			&pl.CreatedAt, &pl.UpdatedAt,
		); err != nil {
			return nil, err
		}
		pl.Hooks = hooks
		list = append(list, pl)
	}
	return list, rows.Err()
}

func (p *PostgresDB) GetPluginByName(name string) (*model.Plugin, error) {
	query := `
		SELECT id, name, display_name, description, category, hooks, config_schema, is_builtin, version, created_at, updated_at
		FROM plugins
		WHERE name = $1`
	var pl model.Plugin
	var hooks []string
	err := p.QueryRow(query, name).Scan(
		&pl.ID, &pl.Name, &pl.DisplayName, &pl.Description, &pl.Category,
		pq.Array(&hooks), &pl.ConfigSchema, &pl.IsBuiltin, &pl.Version,
		&pl.CreatedAt, &pl.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	pl.Hooks = hooks
	return &pl, nil
}

func (p *PostgresDB) ListPluginBindings(tenantID *uuid.UUID, pluginName string) ([]model.PluginBinding, error) {
	query := `
		SELECT pb.id, pb.plugin_name, pb.scope_level, pb.scope_provider, pb.scope_model,
		       pb.scope_tenant_id, pb.scope_role, pb.scope_user_id, pb.scope_key_id,
		       pb.config, pb.is_active, pb.priority, pb.created_by, pb.created_at, pb.updated_at,
		       p.display_name, p.category
		FROM plugin_bindings pb
		JOIN plugins p ON p.name = pb.plugin_name
		WHERE ($1::uuid IS NULL OR pb.scope_tenant_id = $1)
		  AND ($2::text = '' OR pb.plugin_name = $2)
		ORDER BY pb.priority ASC, pb.created_at ASC`
	rows, err := p.Query(query, tenantID, pluginName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.PluginBinding
	for rows.Next() {
		var b model.PluginBinding
		if err := rows.Scan(
			&b.ID, &b.PluginName, &b.ScopeLevel, &b.ScopeProvider, &b.ScopeModel,
			&b.ScopeTenantID, &b.ScopeRole, &b.ScopeUserID, &b.ScopeKeyID,
			&b.Config, &b.IsActive, &b.Priority, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
			&b.PluginDisplayName, &b.Category,
		); err != nil {
			return nil, err
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (p *PostgresDB) ListActivePluginBindings() ([]model.PluginBinding, error) {
	query := `
		SELECT pb.id, pb.plugin_name, pb.scope_level, pb.scope_provider, pb.scope_model,
		       pb.scope_tenant_id, pb.scope_role, pb.scope_user_id, pb.scope_key_id,
		       pb.config, pb.is_active, pb.priority, pb.created_by, pb.created_at, pb.updated_at,
		       p.display_name, p.category
		FROM plugin_bindings pb
		JOIN plugins p ON p.name = pb.plugin_name
		WHERE pb.is_active = TRUE
		ORDER BY pb.priority ASC`
	rows, err := p.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.PluginBinding
	for rows.Next() {
		var b model.PluginBinding
		if err := rows.Scan(
			&b.ID, &b.PluginName, &b.ScopeLevel, &b.ScopeProvider, &b.ScopeModel,
			&b.ScopeTenantID, &b.ScopeRole, &b.ScopeUserID, &b.ScopeKeyID,
			&b.Config, &b.IsActive, &b.Priority, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
			&b.PluginDisplayName, &b.Category,
		); err != nil {
			return nil, err
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

func (p *PostgresDB) GetPluginBindingByID(id uuid.UUID) (*model.PluginBinding, error) {
	query := `
		SELECT pb.id, pb.plugin_name, pb.scope_level, pb.scope_provider, pb.scope_model,
		       pb.scope_tenant_id, pb.scope_role, pb.scope_user_id, pb.scope_key_id,
		       pb.config, pb.is_active, pb.priority, pb.created_by, pb.created_at, pb.updated_at,
		       p.display_name, p.category
		FROM plugin_bindings pb
		JOIN plugins p ON p.name = pb.plugin_name
		WHERE pb.id = $1`
	var b model.PluginBinding
	err := p.QueryRow(query, id).Scan(
		&b.ID, &b.PluginName, &b.ScopeLevel, &b.ScopeProvider, &b.ScopeModel,
		&b.ScopeTenantID, &b.ScopeRole, &b.ScopeUserID, &b.ScopeKeyID,
		&b.Config, &b.IsActive, &b.Priority, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
		&b.PluginDisplayName, &b.Category,
	)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (p *PostgresDB) CreatePluginBinding(b *model.PluginBinding) error {
	query := `
		INSERT INTO plugin_bindings (
			plugin_name, scope_level, scope_provider, scope_model,
			scope_tenant_id, scope_role, scope_user_id, scope_key_id,
			config, is_active, priority, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at`
	return p.QueryRow(
		query,
		b.PluginName, b.ScopeLevel, b.ScopeProvider, b.ScopeModel,
		b.ScopeTenantID, b.ScopeRole, b.ScopeUserID, b.ScopeKeyID,
		b.Config, b.IsActive, b.Priority, b.CreatedBy,
	).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
}

func (p *PostgresDB) UpdatePluginBinding(id uuid.UUID, config *json.RawMessage, isActive *bool, priority *int) error {
	sets := []string{"updated_at = NOW()"}
	args := []any{id}
	idx := 2

	if config != nil {
		sets = append(sets, fmt.Sprintf("config = $%d", idx))
		args = append(args, *config)
		idx++
	}
	if isActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", idx))
		args = append(args, *isActive)
		idx++
	}
	if priority != nil {
		sets = append(sets, fmt.Sprintf("priority = $%d", idx))
		args = append(args, *priority)
		idx++
	}

	query := fmt.Sprintf("UPDATE plugin_bindings SET %s WHERE id = $1", strings.Join(sets, ", "))
	res, err := p.Exec(query, args...)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (p *PostgresDB) DeletePluginBinding(id uuid.UUID) error {
	res, err := p.Exec(`DELETE FROM plugin_bindings WHERE id = $1`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
