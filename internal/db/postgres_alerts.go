package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"llm-gateway/internal/model"

	"github.com/google/uuid"
)

// ==========================================================================
// Alert Rules & Webhook History Repositories
// ==========================================================================

func (p *PostgresDB) CreateAlertRule(r *model.AlertRule) error {
	r.ID = uuid.New()
	now := time.Now()
	r.CreatedAt = now
	r.UpdatedAt = now
	if r.WindowSeconds <= 0 {
		r.WindowSeconds = 60
	}
	if r.CooldownSeconds <= 0 {
		r.CooldownSeconds = 900
	}
	if r.Severity == "" {
		r.Severity = "warning"
	}
	if len(r.Channels) == 0 && r.WebhookURL != "" {
		r.Channels = []model.AlertChannelConfig{
			{
				ChannelType:   r.ChannelType,
				ChannelName:   r.ChannelType,
				WebhookURL:    r.WebhookURL,
				WebhookSecret: r.WebhookSecret,
				IsEnabled:     true,
			},
		}
	} else if len(r.Channels) > 0 && r.WebhookURL == "" {
		r.WebhookURL = r.Channels[0].WebhookURL
		r.WebhookSecret = r.Channels[0].WebhookSecret
		r.ChannelType = r.Channels[0].ChannelType
	}
	if r.ChannelType == "" {
		r.ChannelType = "generic"
	}

	channelsJSON, err := json.Marshal(r.Channels)
	if err != nil {
		channelsJSON = []byte("[]")
	}

	query := `
		INSERT INTO alert_rules (
			id, name, event_type, tenant_id, threshold_value,
			window_seconds, cooldown_seconds, webhook_url, webhook_secret,
			channel_type, is_active, created_at, updated_at,
			severity, filter_provider, filter_model, channels
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING created_at, updated_at`

	return p.QueryRow(
		query,
		r.ID, r.Name, r.EventType, r.TenantID, r.ThresholdValue,
		r.WindowSeconds, r.CooldownSeconds, r.WebhookURL, r.WebhookSecret,
		r.ChannelType, r.IsActive, r.CreatedAt, r.UpdatedAt,
		r.Severity, r.FilterProvider, r.FilterModel, channelsJSON,
	).Scan(&r.CreatedAt, &r.UpdatedAt)
}

func (p *PostgresDB) GetAlertRule(id uuid.UUID) (*model.AlertRule, error) {
	query := `
		SELECT id, name, event_type, tenant_id, threshold_value,
		       window_seconds, cooldown_seconds, webhook_url, webhook_secret,
		       channel_type, is_active, created_at, updated_at,
		       COALESCE(severity, 'warning'), COALESCE(filter_provider, ''),
		       COALESCE(filter_model, ''), COALESCE(channels, '[]'::jsonb)
		FROM alert_rules WHERE id = $1`

	var r model.AlertRule
	var channelsRaw []byte
	err := p.QueryRow(query, id).Scan(
		&r.ID, &r.Name, &r.EventType, &r.TenantID, &r.ThresholdValue,
		&r.WindowSeconds, &r.CooldownSeconds, &r.WebhookURL, &r.WebhookSecret,
		&r.ChannelType, &r.IsActive, &r.CreatedAt, &r.UpdatedAt,
		&r.Severity, &r.FilterProvider, &r.FilterModel, &channelsRaw,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("alert rule not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	if len(channelsRaw) > 0 {
		_ = json.Unmarshal(channelsRaw, &r.Channels)
	}
	if len(r.Channels) == 0 && r.WebhookURL != "" {
		r.Channels = []model.AlertChannelConfig{
			{
				ChannelType:   r.ChannelType,
				ChannelName:   r.ChannelType,
				WebhookURL:    r.WebhookURL,
				WebhookSecret: r.WebhookSecret,
				IsEnabled:     true,
			},
		}
	}
	return &r, nil
}

func (p *PostgresDB) ListAlertRules(tenantID *uuid.UUID, eventType string) ([]model.AlertRule, error) {
	query := `
		SELECT id, name, event_type, tenant_id, threshold_value,
		       window_seconds, cooldown_seconds, webhook_url, webhook_secret,
		       channel_type, is_active, created_at, updated_at,
		       COALESCE(severity, 'warning'), COALESCE(filter_provider, ''),
		       COALESCE(filter_model, ''), COALESCE(channels, '[]'::jsonb)
		FROM alert_rules
		WHERE ($1::uuid IS NULL OR tenant_id = $1 OR tenant_id IS NULL)
		  AND ($2 = '' OR event_type = $2)
		ORDER BY created_at DESC`

	rows, err := p.Query(query, tenantID, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []model.AlertRule
	for rows.Next() {
		var r model.AlertRule
		var channelsRaw []byte
		if err := rows.Scan(
			&r.ID, &r.Name, &r.EventType, &r.TenantID, &r.ThresholdValue,
			&r.WindowSeconds, &r.CooldownSeconds, &r.WebhookURL, &r.WebhookSecret,
			&r.ChannelType, &r.IsActive, &r.CreatedAt, &r.UpdatedAt,
			&r.Severity, &r.FilterProvider, &r.FilterModel, &channelsRaw,
		); err != nil {
			return nil, err
		}
		if len(channelsRaw) > 0 {
			_ = json.Unmarshal(channelsRaw, &r.Channels)
		}
		if len(r.Channels) == 0 && r.WebhookURL != "" {
			r.Channels = []model.AlertChannelConfig{
				{
					ChannelType:   r.ChannelType,
					ChannelName:   r.ChannelType,
					WebhookURL:    r.WebhookURL,
					WebhookSecret: r.WebhookSecret,
					IsEnabled:     true,
				},
			}
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func (p *PostgresDB) UpdateAlertRule(id uuid.UUID, patch *model.AlertRulePatch) (*model.AlertRule, error) {
	current, err := p.GetAlertRule(id)
	if err != nil {
		return nil, err
	}

	if patch.Name != nil {
		current.Name = *patch.Name
	}
	if patch.EventType != nil {
		current.EventType = *patch.EventType
	}
	if patch.Severity != nil {
		current.Severity = *patch.Severity
	}
	if patch.TenantID != nil {
		current.TenantID = patch.TenantID
	}
	if patch.FilterProvider != nil {
		current.FilterProvider = *patch.FilterProvider
	}
	if patch.FilterModel != nil {
		current.FilterModel = *patch.FilterModel
	}
	if patch.ThresholdValue != nil {
		current.ThresholdValue = *patch.ThresholdValue
	}
	if patch.WindowSeconds != nil {
		current.WindowSeconds = *patch.WindowSeconds
	}
	if patch.CooldownSeconds != nil {
		current.CooldownSeconds = *patch.CooldownSeconds
	}
	if patch.WebhookURL != nil {
		current.WebhookURL = *patch.WebhookURL
	}
	if patch.WebhookSecret != nil {
		current.WebhookSecret = *patch.WebhookSecret
	}
	if patch.ChannelType != nil {
		current.ChannelType = *patch.ChannelType
	}
	if patch.Channels != nil {
		current.Channels = *patch.Channels
		if len(current.Channels) > 0 && current.Channels[0].WebhookURL != "" {
			current.WebhookURL = current.Channels[0].WebhookURL
			current.WebhookSecret = current.Channels[0].WebhookSecret
			current.ChannelType = current.Channels[0].ChannelType
		}
	}
	if patch.IsActive != nil {
		current.IsActive = *patch.IsActive
	}
	current.UpdatedAt = time.Now()

	channelsJSON, err := json.Marshal(current.Channels)
	if err != nil {
		channelsJSON = []byte("[]")
	}

	query := `
		UPDATE alert_rules SET
			name = $1, event_type = $2, tenant_id = $3, threshold_value = $4,
			window_seconds = $5, cooldown_seconds = $6, webhook_url = $7, webhook_secret = $8,
			channel_type = $9, is_active = $10, updated_at = $11,
			severity = $12, filter_provider = $13, filter_model = $14, channels = $15
		WHERE id = $16
		RETURNING updated_at`

	err = p.QueryRow(
		query,
		current.Name, current.EventType, current.TenantID, current.ThresholdValue,
		current.WindowSeconds, current.CooldownSeconds, current.WebhookURL, current.WebhookSecret,
		current.ChannelType, current.IsActive, current.UpdatedAt,
		current.Severity, current.FilterProvider, current.FilterModel, channelsJSON,
		id,
	).Scan(&current.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return current, nil
}

func (p *PostgresDB) DeleteAlertRule(id uuid.UUID) error {
	res, err := p.Exec(`DELETE FROM alert_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("alert rule not found")
	}
	return nil
}

func (p *PostgresDB) InsertAlertHistory(h *model.AlertHistory) error {
	if h.ID == uuid.Nil {
		h.ID = uuid.New()
	}
	if h.TriggeredAt.IsZero() {
		h.TriggeredAt = time.Now()
	}
	if len(h.Payload) == 0 {
		h.Payload = json.RawMessage(`{}`)
	}

	query := `
		INSERT INTO alert_history (
			id, rule_id, rule_name, event_type, tenant_id,
			title, message, payload, status, status_code, error_message, triggered_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err := p.Exec(
		query,
		h.ID, h.RuleID, h.RuleName, h.EventType, h.TenantID,
		h.Title, h.Message, h.Payload, h.Status, h.StatusCode, h.ErrorMessage, h.TriggeredAt,
	)
	return err
}

func (p *PostgresDB) ListAlertHistory(limit, offset int, status string) ([]model.AlertHistory, int, error) {
	if limit <= 0 {
		limit = 50
	}
	var total int
	countQuery := `SELECT COUNT(*) FROM alert_history WHERE ($1 = '' OR status = $1)`
	if err := p.QueryRow(countQuery, status).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, rule_id, rule_name, event_type, tenant_id,
		       title, message, payload, status, status_code, error_message, triggered_at
		FROM alert_history
		WHERE ($1 = '' OR status = $1)
		ORDER BY triggered_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := p.Query(query, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var history []model.AlertHistory
	for rows.Next() {
		var h model.AlertHistory
		if err := rows.Scan(
			&h.ID, &h.RuleID, &h.RuleName, &h.EventType, &h.TenantID,
			&h.Title, &h.Message, &h.Payload, &h.Status, &h.StatusCode, &h.ErrorMessage, &h.TriggeredAt,
		); err != nil {
			return nil, 0, err
		}
		history = append(history, h)
	}
	return history, total, nil
}
