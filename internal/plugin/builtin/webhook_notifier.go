package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/plugin"
)

// WebhookNotifierPlugin triggers asynchronous webhooks on errors, slow requests, or token surges.
type WebhookNotifierPlugin struct {
	httpClient *http.Client
}

func NewWebhookNotifierPlugin() *WebhookNotifierPlugin {
	return &WebhookNotifierPlugin{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (p *WebhookNotifierPlugin) Name() string {
	return "webhook-notifier"
}

func (p *WebhookNotifierPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPostUpstream}
}

func (p *WebhookNotifierPlugin) IsAsyncPostUpstream() bool {
	return true
}

func (p *WebhookNotifierPlugin) ExecPostUpstream(pctx *plugin.PluginContext, resp *canonical.CanonicalResponse, cfg map[string]any) error {
	webhookURL := getStringConfig(cfg, "webhook_url", "")
	if webhookURL == "" {
		return nil
	}

	notifyOnErrors := getBoolConfig(cfg, "notify_on_errors", true)
	notifyOnSlow := getBoolConfig(cfg, "notify_on_slow", true)
	slowThresholdSec := getFloatConfig(cfg, "slow_threshold_seconds", 5.0)
	minTokensThreshold := getIntConfig(cfg, "min_tokens_threshold", 0)

	shouldNotify := false
	var triggerReasons []string

	// Check status code errors
	if notifyOnErrors && pctx.StatusCode >= 400 {
		shouldNotify = true
		triggerReasons = append(triggerReasons, "error_status_code")
	}

	// Check slow duration
	durationSec := pctx.Duration.Seconds()
	if notifyOnSlow && slowThresholdSec > 0 && durationSec >= slowThresholdSec {
		shouldNotify = true
		triggerReasons = append(triggerReasons, "slow_request")
	}

	// Check token threshold
	totalTokens := 0
	if resp != nil {
		totalTokens = resp.TotalTokens
	}
	if minTokensThreshold > 0 && totalTokens >= minTokensThreshold {
		shouldNotify = true
		triggerReasons = append(triggerReasons, "high_token_consumption")
	}

	if !shouldNotify {
		return nil
	}

	payload := map[string]any{
		"event":           "llmgw.alert",
		"timestamp":       time.Now().UTC().Format(time.RFC3339Nano),
		"reasons":         triggerReasons,
		"model":           pctx.Model,
		"provider":        pctx.Provider,
		"status_code":     pctx.StatusCode,
		"duration_sec":    durationSec,
		"total_tokens":    totalTokens,
	}

	if pctx.Tenant != nil {
		payload["tenant_id"] = pctx.Tenant.ID.String()
		payload["tenant_name"] = pctx.Tenant.Name
	}
	if pctx.APIKey != nil {
		payload["key_prefix"] = pctx.APIKey.KeyPrefix
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	// Asynchronous dispatch
	go func(url string, body []byte) {
		reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := p.httpClient.Do(req)
		if err == nil && res != nil {
			_ = res.Body.Close()
		}
	}(webhookURL, data)

	return nil
}
