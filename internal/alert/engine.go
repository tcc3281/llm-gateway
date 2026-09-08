package alert

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"llm-gateway/internal/db"
	"llm-gateway/internal/model"
)

// Engine manages background rule evaluation, debouncing, anti-spam rate limiting,
// and webhook dispatches. It is completely decoupled from the gateway request hot path.
type Engine struct {
	pg           *db.PostgresDB
	eventQueue   chan model.AlertEvent
	droppedCount int64

	// Cooldown tracking: key -> last alert timestamp
	cooldownLock sync.RWMutex
	cooldowns    map[string]time.Time

	// Circuit breaker per webhook host: host -> consecutive failures & open until
	circuitLock sync.RWMutex
	circuits    map[string]*circuitState

	// Global rate limiter for outbound webhooks (tokens per minute)
	rateLimiter chan struct{}

	httpClient *http.Client
	ctx        context.Context
	cancel     context.CancelFunc
}

type circuitState struct {
	consecutiveFailures int
	openUntil           time.Time
}

func NewEngine(pg *db.PostgresDB, queueSize int) *Engine {
	if queueSize <= 0 {
		queueSize = 5000
	}
	ctx, cancel := context.WithCancel(context.Background())

	e := &Engine{
		pg:          pg,
		eventQueue:  make(chan model.AlertEvent, queueSize),
		cooldowns:   make(map[string]time.Time),
		circuits:    make(map[string]*circuitState),
		rateLimiter: make(chan struct{}, 30), // Allow up to 30 webhook dispatches per minute burst
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		ctx:    ctx,
		cancel: cancel,
	}

	// Refill rate limiter bucket at 30 tokens per minute (1 every 2s)
	go e.startRateLimiterRefill(2 * time.Second)

	// Background worker pool for processing real-time events
	go e.startEventWorkers(2)

	// Periodic check for threshold evaluations (e.g. low tenant credit)
	go e.startPeriodicEvaluator(60 * time.Second)

	// Periodic cleanup of stale cooldown & circuit breaker states
	go e.startCleanupLoop(10 * time.Minute)

	return e
}

// Emit sends an alert event to the engine asynchronously. It NEVER blocks the caller.
func (e *Engine) Emit(evt model.AlertEvent) {
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}

	select {
	case e.eventQueue <- evt:
	default:
		atomic.AddInt64(&e.droppedCount, 1)
	}
}

// Close gracefully stops the alert engine.
func (e *Engine) Close() {
	e.cancel()
	close(e.eventQueue)
}

// DroppedCount returns the total number of events dropped due to queue congestion.
func (e *Engine) DroppedCount() int64 {
	return atomic.LoadInt64(&e.droppedCount)
}

func (e *Engine) startEventWorkers(workers int) {
	for i := 0; i < workers; i++ {
		go func() {
			for {
				select {
				case <-e.ctx.Done():
					return
				case evt, ok := <-e.eventQueue:
					if !ok {
						return
					}
					e.processEvent(evt)
				}
			}
		}()
	}
}

func (e *Engine) processEvent(evt model.AlertEvent) {
	if e.pg == nil {
		return
	}

	// Find active rules matching this event type
	rules, err := e.pg.ListAlertRules(evt.TenantID, evt.EventType)
	if err != nil {
		log.Printf("[AlertEngine] Failed to list alert rules: %v", err)
		return
	}

	for _, rule := range rules {
		if !rule.IsActive {
			continue
		}

		// Check if rule applies to this tenant (if tenant-specific)
		if rule.TenantID != nil && evt.TenantID != nil && *rule.TenantID != *evt.TenantID {
			continue
		}

		// Filter by Provider if configured
		if rule.FilterProvider != "" && rule.FilterProvider != "*" && evt.Metadata != nil {
			if prov, ok := evt.Metadata["provider"].(string); ok && prov != "" && !strings.EqualFold(prov, rule.FilterProvider) {
				continue
			}
		}

		// Filter by Model if configured
		if rule.FilterModel != "" && rule.FilterModel != "*" && evt.Model != "" && !strings.EqualFold(evt.Model, rule.FilterModel) {
			continue
		}

		// Evaluate rule threshold
		triggered := false
		switch rule.EventType {
		case "rate_limit_surge", "error_rate_surge", "iam_deny_surge":
			// For count-based surges, if value >= threshold
			if evt.Value >= rule.ThresholdValue {
				triggered = true
			}
		case "latency_anomaly":
			if evt.Value >= rule.ThresholdValue {
				triggered = true
			}
		case "budget_threshold":
			if evt.Value <= rule.ThresholdValue {
				triggered = true
			}
		default:
			triggered = true
		}

		if !triggered {
			continue
		}

		// Check cooldown to prevent notification spam
		entityKey := "global"
		if evt.TenantID != nil {
			entityKey = evt.TenantID.String()
		} else if evt.KeyID != nil {
			entityKey = evt.KeyID.String()
		}
		cooldownKey := fmt.Sprintf("%s:%s", rule.ID.String(), entityKey)

		if e.isInCooldown(cooldownKey, time.Duration(rule.CooldownSeconds)*time.Second) {
			// Log suppressed event for audit trail
			_ = e.pg.InsertAlertHistory(&model.AlertHistory{
				RuleID:      &rule.ID,
				RuleName:    rule.Name,
				EventType:   rule.EventType,
				TenantID:    evt.TenantID,
				Title:       fmt.Sprintf("[COOLDOWN SUPPRESSED] %s", rule.Name),
				Message:     fmt.Sprintf("Alert condition met but suppressed within %ds cooldown. Event: %s", rule.CooldownSeconds, evt.Message),
				Status:      "suppressed_cooldown",
				TriggeredAt: time.Now(),
			})
			continue
		}

		// Mark cooldown
		e.setCooldown(cooldownKey)

		// Dispatch webhook
		go e.dispatchWebhook(rule, evt)
	}
}

func (e *Engine) dispatchWebhook(rule model.AlertRule, evt model.AlertEvent) {
	channels := rule.Channels
	if len(channels) == 0 && rule.WebhookURL != "" {
		channels = []model.AlertChannelConfig{
			{
				ChannelType:   rule.ChannelType,
				ChannelName:   rule.ChannelType,
				WebhookURL:    rule.WebhookURL,
				WebhookSecret: rule.WebhookSecret,
				IsEnabled:     true,
			},
		}
	}

	for _, ch := range channels {
		if !ch.IsEnabled || ch.WebhookURL == "" {
			continue
		}
		e.dispatchToChannel(rule, ch, evt)
	}
}

func (e *Engine) dispatchToChannel(rule model.AlertRule, ch model.AlertChannelConfig, evt model.AlertEvent) {
	// 1. Rate limiter check (anti-spike protection)
	select {
	case <-e.rateLimiter:
		// token acquired
	default:
		// Global outbound rate limit exceeded, log failure
		errMsg := "Outbound webhook dispatch throttled (rate limit exceeded)"
		_ = e.pg.InsertAlertHistory(&model.AlertHistory{
			RuleID:       &rule.ID,
			RuleName:     rule.Name,
			EventType:    rule.EventType,
			TenantID:     evt.TenantID,
			Title:        rule.Name,
			Message:      evt.Message,
			Status:       "failed",
			ErrorMessage: &errMsg,
			TriggeredAt:  time.Now(),
		})
		return
	}

	// 2. Circuit breaker check for webhook target host
	u, err := url.Parse(ch.WebhookURL)
	if err != nil {
		errMsg := fmt.Sprintf("Invalid webhook URL: %v", err)
		_ = e.pg.InsertAlertHistory(&model.AlertHistory{
			RuleID:       &rule.ID,
			RuleName:     rule.Name,
			EventType:    rule.EventType,
			TenantID:     evt.TenantID,
			Title:        rule.Name,
			Message:      evt.Message,
			Status:       "failed",
			ErrorMessage: &errMsg,
			TriggeredAt:  time.Now(),
		})
		return
	}

	host := u.Host
	if e.isCircuitOpen(host) {
		errMsg := fmt.Sprintf("Circuit breaker open for host %s (consecutive failures)", host)
		_ = e.pg.InsertAlertHistory(&model.AlertHistory{
			RuleID:       &rule.ID,
			RuleName:     rule.Name,
			EventType:    rule.EventType,
			TenantID:     evt.TenantID,
			Title:        rule.Name,
			Message:      evt.Message,
			Status:       "failed",
			ErrorMessage: &errMsg,
			TriggeredAt:  time.Now(),
		})
		return
	}

	// 3. Format payload based on channel type (generic, slack, discord, pagerduty)
	payloadBytes, contentType := formatWebhookPayload(ch.ChannelType, rule.Severity, rule.Name, evt)

	// 4. Build HTTP request
	req, err := http.NewRequestWithContext(e.ctx, http.MethodPost, ch.WebhookURL, bytes.NewReader(payloadBytes))
	if err != nil {
		errMsg := err.Error()
		_ = e.pg.InsertAlertHistory(&model.AlertHistory{
			RuleID:       &rule.ID,
			RuleName:     rule.Name,
			EventType:    rule.EventType,
			TenantID:     evt.TenantID,
			Title:        rule.Name,
			Message:      evt.Message,
			Status:       "failed",
			ErrorMessage: &errMsg,
			TriggeredAt:  time.Now(),
		})
		return
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "LLM-Gateway-AlertEngine/1.0")

	// 5. Add HMAC-SHA256 signature if secret is configured
	if ch.WebhookSecret != "" {
		sig := computeHMAC(payloadBytes, ch.WebhookSecret)
		req.Header.Set("X-Gateway-Signature", "sha256="+sig)
	}

	// 6. Execute request
	resp, err := e.httpClient.Do(req)
	if err != nil {
		e.recordCircuitFailure(host)
		errMsg := err.Error()
		_ = e.pg.InsertAlertHistory(&model.AlertHistory{
			RuleID:       &rule.ID,
			RuleName:     rule.Name,
			EventType:    rule.EventType,
			TenantID:     evt.TenantID,
			Title:        rule.Name,
			Message:      evt.Message,
			Payload:      payloadBytes,
			Status:       "failed",
			ErrorMessage: &errMsg,
			TriggeredAt:  time.Now(),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		e.recordCircuitSuccess(host)
		_ = e.pg.InsertAlertHistory(&model.AlertHistory{
			RuleID:      &rule.ID,
			RuleName:    rule.Name,
			EventType:   rule.EventType,
			TenantID:    evt.TenantID,
			Title:       rule.Name,
			Message:     evt.Message,
			Payload:     payloadBytes,
			Status:      "delivered",
			StatusCode:  &resp.StatusCode,
			TriggeredAt: time.Now(),
		})
	} else {
		e.recordCircuitFailure(host)
		errMsg := fmt.Sprintf("HTTP %d from webhook target", resp.StatusCode)
		_ = e.pg.InsertAlertHistory(&model.AlertHistory{
			RuleID:       &rule.ID,
			RuleName:     rule.Name,
			EventType:    rule.EventType,
			TenantID:     evt.TenantID,
			Title:        rule.Name,
			Message:      evt.Message,
			Payload:      payloadBytes,
			Status:       "failed",
			StatusCode:   &resp.StatusCode,
			ErrorMessage: &errMsg,
			TriggeredAt:  time.Now(),
		})
	}
}

// TestWebhook sends a test notification to verify target connectivity.
func (e *Engine) TestWebhook(webhookURL, channelType, secret string) (int, error) {
	mockEvt := model.AlertEvent{
		EventType: "test_ping",
		Message:   "This is a test notification from LLM Gateway Alert Engine.",
		Value:     1.0,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"test":    true,
			"gateway": "LLM Gateway Production",
		},
	}

	payloadBytes, contentType := formatWebhookPayload(channelType, "info", "Test Alert Ping", mockEvt)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, webhookURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return 0, err
	}

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "LLM-Gateway-AlertEngine/1.0")

	if secret != "" {
		sig := computeHMAC(payloadBytes, secret)
		req.Header.Set("X-Gateway-Signature", "sha256="+sig)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("webhook responded with HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func (e *Engine) startPeriodicEvaluator(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.evaluatePeriodicRules()
		}
	}
}

func (e *Engine) evaluatePeriodicRules() {
	if e.pg == nil {
		return
	}

	// 1. Evaluate Low Prepaid Budget Rules
	tenants, err := e.pg.ListTenants()
	if err != nil {
		return
	}

	for _, t := range tenants {
		if !t.IsActive || t.BillingMode != "prepaid" {
			continue
		}

		// Find rules for budget threshold
		rules, err := e.pg.ListAlertRules(&t.ID, "budget_threshold")
		if err != nil {
			continue
		}

		for _, r := range rules {
			if !r.IsActive {
				continue
			}
			if t.CreditBalance <= r.ThresholdValue {
				e.Emit(model.AlertEvent{
					EventType: "budget_threshold",
					TenantID:  &t.ID,
					Value:     t.CreditBalance,
					Message: fmt.Sprintf(
						"Tenant %q prepaid balance is $%.2f, below configured threshold of $%.2f.",
						t.Name, t.CreditBalance, r.ThresholdValue,
					),
					Metadata: map[string]interface{}{
						"tenant_name":    t.Name,
						"credit_balance": t.CreditBalance,
						"threshold":      r.ThresholdValue,
					},
					Timestamp: time.Now(),
				})
			}
		}
	}
}

func (e *Engine) startRateLimiterRefill(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			select {
			case e.rateLimiter <- struct{}{}:
			default:
				// Bucket is full
			}
		}
	}
}

func (e *Engine) startCleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.cleanupStaleState()
		}
	}
}

func (e *Engine) cleanupStaleState() {
	e.cooldownLock.Lock()
	now := time.Now()
	for k, t := range e.cooldowns {
		if now.Sub(t) > 2*time.Hour {
			delete(e.cooldowns, k)
		}
	}
	e.cooldownLock.Unlock()

	e.circuitLock.Lock()
	for k, c := range e.circuits {
		if now.After(c.openUntil) && c.consecutiveFailures == 0 {
			delete(e.circuits, k)
		}
	}
	e.circuitLock.Unlock()
}

func (e *Engine) isInCooldown(key string, cooldown time.Duration) bool {
	e.cooldownLock.RLock()
	defer e.cooldownLock.RUnlock()

	lastTime, exists := e.cooldowns[key]
	if !exists {
		return false
	}
	return time.Since(lastTime) < cooldown
}

func (e *Engine) setCooldown(key string) {
	e.cooldownLock.Lock()
	defer e.cooldownLock.Unlock()
	e.cooldowns[key] = time.Now()
}

func (e *Engine) isCircuitOpen(host string) bool {
	e.circuitLock.RLock()
	defer e.circuitLock.RUnlock()

	st, exists := e.circuits[host]
	if !exists {
		return false
	}
	return time.Now().Before(st.openUntil)
}

func (e *Engine) recordCircuitSuccess(host string) {
	e.circuitLock.Lock()
	defer e.circuitLock.Unlock()
	delete(e.circuits, host)
}

func (e *Engine) recordCircuitFailure(host string) {
	e.circuitLock.Lock()
	defer e.circuitLock.Unlock()

	st, exists := e.circuits[host]
	if !exists {
		st = &circuitState{}
		e.circuits[host] = st
	}
	st.consecutiveFailures++
	if st.consecutiveFailures >= 3 {
		st.openUntil = time.Now().Add(5 * time.Minute) // Back off for 5 minutes
	}
}

func formatWebhookPayload(channelType, severity, title string, evt model.AlertEvent) ([]byte, string) {
	if severity == "" {
		severity = "warning"
	}
	sevUpper := strings.ToUpper(severity)

	switch channelType {
	case "slack":
		color := "#F59E0B"
		if severity == "critical" || evt.EventType == "budget_threshold" || evt.EventType == "error_rate_surge" {
			color = "#EF4444"
		} else if severity == "info" {
			color = "#22D3EE"
		}

		fields := []map[string]interface{}{
			{"title": "Severity", "value": sevUpper, "short": true},
			{"title": "Event Type", "value": evt.EventType, "short": true},
			{"title": "Metric Value", "value": fmt.Sprintf("%.2f", evt.Value), "short": true},
		}
		if evt.Model != "" {
			fields = append(fields, map[string]interface{}{"title": "Model", "value": evt.Model, "short": true})
		}

		slackPayload := map[string]interface{}{
			"text": fmt.Sprintf("*[LLM Gateway Alert - %s]* %s", sevUpper, title),
			"attachments": []map[string]interface{}{
				{
					"color":  color,
					"title":  title,
					"text":   evt.Message,
					"footer": "LLM Gateway Obsidian Alert Engine",
					"ts":     evt.Timestamp.Unix(),
					"fields": fields,
				},
			},
		}
		b, _ := json.Marshal(slackPayload)
		return b, "application/json"

	case "discord":
		color := 16098851 // Amber
		if severity == "critical" || evt.EventType == "budget_threshold" || evt.EventType == "error_rate_surge" {
			color = 15673636 // Red
		} else if severity == "info" {
			color = 2282478 // Cyan
		}

		fields := []map[string]interface{}{
			{"name": "Severity", "value": sevUpper, "inline": true},
			{"name": "Event Type", "value": evt.EventType, "inline": true},
			{"name": "Metric Value", "value": fmt.Sprintf("%.2f", evt.Value), "inline": true},
		}
		if evt.Model != "" {
			fields = append(fields, map[string]interface{}{"name": "Model", "value": evt.Model, "inline": true})
		}

		discordPayload := map[string]interface{}{
			"content": fmt.Sprintf("**[LLM Gateway Alert - %s]** %s", sevUpper, title),
			"embeds": []map[string]interface{}{
				{
					"title":       title,
					"description": evt.Message,
					"color":       color,
					"timestamp":   evt.Timestamp.Format(time.RFC3339),
					"fields":      fields,
				},
			},
		}
		b, _ := json.Marshal(discordPayload)
		return b, "application/json"

	case "pagerduty":
		pdSeverity := "warning"
		if severity == "critical" {
			pdSeverity = "error"
		} else if severity == "info" {
			pdSeverity = "info"
		}
		pdPayload := map[string]interface{}{
			"routing_key":  "llm-gateway",
			"event_action": "trigger",
			"payload": map[string]interface{}{
				"summary":   fmt.Sprintf("[%s] %s: %s", sevUpper, title, evt.Message),
				"source":    "llm-gateway-alert-engine",
				"severity":  pdSeverity,
				"timestamp": evt.Timestamp.Format(time.RFC3339),
				"custom_details": map[string]interface{}{
					"event_type": evt.EventType,
					"value":      evt.Value,
					"tenant_id":  evt.TenantID,
					"model":      evt.Model,
					"metadata":   evt.Metadata,
				},
			},
		}
		b, _ := json.Marshal(pdPayload)
		return b, "application/json"

	default: // Generic JSON
		genericPayload := map[string]interface{}{
			"event_type": evt.EventType,
			"severity":   severity,
			"title":      title,
			"message":    evt.Message,
			"value":      evt.Value,
			"tenant_id":  evt.TenantID,
			"model":      evt.Model,
			"timestamp":  evt.Timestamp.Format(time.RFC3339),
			"metadata":   evt.Metadata,
		}
		b, _ := json.Marshal(genericPayload)
		return b, "application/json"
	}
}

func computeHMAC(data []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
