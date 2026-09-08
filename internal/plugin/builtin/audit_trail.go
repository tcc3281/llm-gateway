package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/plugin"
)

type AuditTrailPlugin struct {
	httpClient *http.Client
}

func NewAuditTrailPlugin() *AuditTrailPlugin {
	return &AuditTrailPlugin{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (p *AuditTrailPlugin) Name() string {
	return "audit-trail"
}

func (p *AuditTrailPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPostUpstream}
}

func (p *AuditTrailPlugin) ExecPostUpstream(pctx *plugin.PluginContext, resp *canonical.CanonicalResponse, cfg map[string]any) error {
	dest, _ := cfg["destination"].(string)
	if dest == "" {
		dest = "log"
	}

	record := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"model":     pctx.Model,
		"provider":  pctx.Provider,
	}

	if pctx.APIKey != nil {
		record["key_prefix"] = pctx.APIKey.KeyPrefix
		record["key_id"] = pctx.APIKey.ID.String()
	}
	if pctx.Tenant != nil {
		record["tenant_id"] = pctx.Tenant.ID.String()
		record["tenant_name"] = pctx.Tenant.Name
	}
	if resp != nil {
		record["prompt_tokens"] = resp.PromptTokens
		record["completion_tokens"] = resp.CompletionTokens
		record["total_tokens"] = resp.TotalTokens
	}

	if dest == "webhook" {
		webhookURL, _ := cfg["webhook_url"].(string)
		if webhookURL != "" {
			data, err := json.Marshal(record)
			if err == nil {
				go func(url string, body []byte) {
					reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
					if err == nil {
						req.Header.Set("Content-Type", "application/json")
						res, err := p.httpClient.Do(req)
						if err == nil && res != nil {
							_ = res.Body.Close()
						}
					}
				}(webhookURL, data)
			}
		}
	} else {
		// Log destination
		slog.Info("audit-trail entry", "audit", record)
	}

	return nil
}
