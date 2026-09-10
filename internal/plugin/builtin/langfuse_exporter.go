package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/plugin"
)

type LangfuseExporterPlugin struct {
	httpClient *http.Client
}

func NewLangfuseExporterPlugin() *LangfuseExporterPlugin {
	return &LangfuseExporterPlugin{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (p *LangfuseExporterPlugin) Name() string {
	return "langfuse-exporter"
}

func (p *LangfuseExporterPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPostUpstream}
}

func (p *LangfuseExporterPlugin) IsAsyncPostUpstream() bool {
	return true
}

func (p *LangfuseExporterPlugin) ExecPostUpstream(pctx *plugin.PluginContext, resp *canonical.CanonicalResponse, cfg map[string]any) error {
	publicKey := getStringConfig(cfg, "public_key", "")
	secretKey := getStringConfig(cfg, "secret_key", "")
	if publicKey == "" || secretKey == "" {
		return nil
	}

	host := strings.TrimRight(getStringConfig(cfg, "host", "https://cloud.langfuse.com"), "/")

	traceID := ""
	if pctx.CanReq != nil {
		traceID = pctx.CanReq.Model + "-" + time.Now().Format("20060102150405.000")
	}

	// Prepare payload for Langfuse ingestion API
	now := time.Now().UTC().Format(time.RFC3339Nano)
	batchItem := map[string]any{
		"id":        traceID,
		"type":      "trace-create",
		"timestamp": now,
		"body": map[string]any{
			"id":        traceID,
			"name":      "llm-gateway-request",
			"timestamp": now,
			"metadata": map[string]any{
				"model":    pctx.Model,
				"provider": pctx.Provider,
			},
		},
	}

	if resp != nil {
		batchItem["body"].(map[string]any)["usage"] = map[string]any{
			"input":  resp.PromptTokens,
			"output": resp.CompletionTokens,
			"total":  resp.TotalTokens,
		}
	}

	payload := map[string]any{
		"batch": []any{batchItem},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	// Non-blocking asynchronous dispatch
	go func(url, pubKey, secKey string, data []byte) {
		reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url+"/api/public/ingestion", bytes.NewReader(data))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth(pubKey, secKey)

		resp, err := p.httpClient.Do(req)
		if err == nil && resp != nil {
			_ = resp.Body.Close()
		}
	}(host, publicKey, secretKey, payloadBytes)

	return nil
}
