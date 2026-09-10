package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"llm-gateway/internal/analytics"
	"llm-gateway/internal/provider"
	"llm-gateway/internal/telemetry"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

func (g *GatewayService) HandleEmbeddings(ctx context.Context, params ProxyParams) error {
	startTime := time.Now()

	// 1. Parse client payload in OpenAI format
	var embedReq struct {
		Model string      `json:"model"`
		Input interface{} `json:"input"`
	}
	if err := json.Unmarshal(params.RequestBody, &embedReq); err != nil {
		return fmt.Errorf("invalid embeddings payload: %w", err)
	}

	// 1b. IAM & Key Model Permission Check
	if g.permEngine != nil && params.ApiKey != nil {
		allowed, pErr := g.permEngine.CheckKeyModelAccess(ctx, params.ApiKey.ID, embedReq.Model)
		if pErr != nil || !allowed {
			return &ErrPermissionDenied{
				Message: fmt.Sprintf("access denied: model %q is not permitted by API key policy", embedReq.Model),
			}
		}

		allowed, pErr = g.permEngine.CheckModelAccess(ctx, params.ApiKey.UserID, embedReq.Model)
		if pErr != nil || !allowed {
			return &ErrPermissionDenied{
				Message: fmt.Sprintf("access denied: model %q is not permitted by IAM group policy", embedReq.Model),
			}
		}
	}

	// 1c. Multi-Tenant System Boundary Check (Layer 1 Contract)
	if err := g.checkTenantBoundary(ctx, params.ApiKey, embedReq.Model); err != nil {
		return err
	}

	// 2. Resolve target provider
	var target ResolvedTarget
	var err error
	var tenantID *uuid.UUID
	if params.ApiKey != nil {
		tenantID = params.ApiKey.TenantID
	}
	vm, vmTargets, vmErr := g.resolveVirtualModel(ctx, embedReq.Model, tenantID)
	if vmErr != nil {
		return vmErr
	}
	if vm != nil && len(vmTargets) > 0 {
		target = vmTargets[0]
		if target.TargetModel != "" {
			embedReq.Model = target.TargetModel
		}
	} else {
		target, err = g.resolveTarget(ctx, embedReq.Model)
		if err != nil {
			return err
		}
	}

	// 3. Resolve upstream API key
	upstreamKey := params.UpstreamAPIKey
	if upstreamKey == "" {
		upstreamKey, err = g.keyStore.GetKey(ctx, target.Name)
		if err != nil {
			g.recordTelemetry(params, target.Name, embedReq.Model, 0, 0, 0, 0, analytics.CostResult{}, 0, http.StatusServiceUnavailable, err.Error(), startTime)
			return &ErrProviderKey{Provider: target.Name}
		}
	}

	// 4. Extract inputs as string slice
	var inputs []string
	switch val := embedReq.Input.(type) {
	case string:
		inputs = []string{val}
	case []interface{}:
		for _, item := range val {
			if s, ok := item.(string); ok {
				inputs = append(inputs, s)
			}
		}
	}

	// Approximate input tokens
	promptTokens := 0
	for _, s := range inputs {
		tokens := len(s) / 4
		if tokens == 0 && len(s) > 0 {
			tokens = 1
		}
		promptTokens += tokens
	}

	// Multi-Tenant Hold & Settle: Reserve worst-case cost against prepaid credit balance
	heldAmount, hErr := g.reserveTenantHold(ctx, params.ApiKey, target.Name, embedReq.Model, promptTokens, 0)
	if hErr != nil {
		return hErr
	}
	if heldAmount > 0 {
		defer func() {
			if g.redis != nil && params.ApiKey != nil && params.ApiKey.TenantID != nil {
				_ = g.redis.ReleaseTenantCreditHold(context.Background(), params.ApiKey.TenantID.String(), heldAmount)
			}
		}()
	}

	// 5. Execute based on Provider protocol
	if target.Provider == provider.ProviderGemini {
		// Translation OpenAI -> Gemini -> OpenAI
		var url string
		var payloadBytes []byte

		if len(inputs) <= 1 {
			// Single embedding
			textVal := ""
			if len(inputs) == 1 {
				textVal = inputs[0]
			}
			url = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:embedContent?key=%s", embedReq.Model, upstreamKey)
			
			geminiReq := map[string]interface{}{
				"content": map[string]interface{}{
					"parts": []map[string]interface{}{
						{"text": textVal},
					},
				},
			}
			payloadBytes, _ = json.Marshal(geminiReq)
		} else {
			// Batch embeddings
			url = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:batchEmbedContents?key=%s", embedReq.Model, upstreamKey)
			
			modelPath := embedReq.Model
			if !strings.HasPrefix(modelPath, "models/") {
				modelPath = "models/" + modelPath
			}

			requestsList := make([]interface{}, 0, len(inputs))
			for _, text := range inputs {
				requestsList = append(requestsList, map[string]interface{}{
					"model": modelPath,
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": text},
						},
					},
				})
			}
			geminiReq := map[string]interface{}{
				"requests": requestsList,
			}
			payloadBytes, _ = json.Marshal(geminiReq)
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payloadBytes))
		if err != nil {
			return fmt.Errorf("failed to create http request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := g.providerClient.Do(httpReq)
		if err != nil {
			g.recordTelemetry(params, target.Name, embedReq.Model, 0, 0, 0, 0, analytics.CostResult{}, 0, http.StatusInternalServerError, err.Error(), startTime)
			return fmt.Errorf("failed sending request to Gemini: %w", err)
		}
		defer resp.Body.Close()

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusOK {
			params.ResponseWriter.Header().Set("Content-Type", "application/json")
			params.ResponseWriter.WriteHeader(resp.StatusCode)
			params.ResponseWriter.Write(respBytes)
			g.recordTelemetry(params, target.Name, embedReq.Model, 0, 0, 0, 0, analytics.CostResult{}, 0, resp.StatusCode, string(respBytes), startTime)
			return nil
		}

		// Decode Gemini response and map back to OpenAI
		type GeminiEmbedding struct {
			Values []float64 `json:"values"`
		}

		var openAIRespData map[string]interface{}
		if len(inputs) <= 1 {
			var geminiResp struct {
				Embedding GeminiEmbedding `json:"embedding"`
			}
			if err := json.Unmarshal(respBytes, &geminiResp); err != nil {
				return fmt.Errorf("failed to decode Gemini response: %w", err)
			}
			openAIRespData = map[string]interface{}{
				"object": "list",
				"data": []interface{}{
					map[string]interface{}{
						"object":    "embedding",
						"index":     0,
						"embedding": geminiResp.Embedding.Values,
					},
				},
				"model": embedReq.Model,
				"usage": map[string]interface{}{
					"prompt_tokens": promptTokens,
					"total_tokens":  promptTokens,
				},
			}
		} else {
			var geminiResp struct {
				Embeddings []GeminiEmbedding `json:"embeddings"`
			}
			if err := json.Unmarshal(respBytes, &geminiResp); err != nil {
				return fmt.Errorf("failed to decode Gemini response: %w", err)
			}
			dataList := make([]interface{}, 0, len(geminiResp.Embeddings))
			for i, emb := range geminiResp.Embeddings {
				dataList = append(dataList, map[string]interface{}{
					"object":    "embedding",
					"index":     i,
					"embedding": emb.Values,
				})
			}
			openAIRespData = map[string]interface{}{
				"object": "list",
				"data":   dataList,
				"model":  embedReq.Model,
				"usage": map[string]interface{}{
					"prompt_tokens": promptTokens,
					"total_tokens":  promptTokens,
				},
			}
		}

		openAIRespBytes, _ := json.Marshal(openAIRespData)
		params.ResponseWriter.Header().Set("Content-Type", "application/json")
		params.ResponseWriter.WriteHeader(http.StatusOK)
		params.ResponseWriter.Write(openAIRespBytes)

		costResult := g.costCalc.CalculateCost(target.Name, embedReq.Model, promptTokens, 0, 0, 0)
		g.recordTelemetry(params, target.Name, embedReq.Model, promptTokens, 0, 0, 0, costResult, 0, http.StatusOK, "", startTime)
		if span := trace.SpanFromContext(ctx); span != nil && span.IsRecording() {
			telemetry.SetGenAIAttributes(span, target.Name, embedReq.Model, "embeddings")
			telemetry.SetGenAIUsage(span, promptTokens, 0)
			telemetry.SetLLMGWAttributes(span, target.Name, false, false, 0)
		}

		var keyID, userID string
		if params.ApiKey != nil {
			keyID = params.ApiKey.ID.String()
			userID = params.ApiKey.UserID.String()
		}
		telemetry.RecordDurationMetric(ctx, target.Name, embedReq.Model, "embeddings", time.Since(startTime).Seconds(), http.StatusOK)
		telemetry.RecordTokenMetrics(ctx, target.Name, embedReq.Model, keyID, userID, promptTokens, 0, 0)
		telemetry.RecordCostMetric(ctx, target.Name, embedReq.Model, keyID, userID, costResult.Cost)

	} else {
		// For OpenAI & OpenAI-compatible: transparent proxying
		var url string
		headers := map[string]string{
			"Content-Type": "application/json",
		}

		if target.BaseURL != "" {
			base := strings.TrimRight(target.BaseURL, "/")
			if strings.HasSuffix(base, "/chat/completions") {
				base = strings.TrimSuffix(base, "/chat/completions")
			} else if strings.HasSuffix(base, "/v1/chat/completions") {
				base = strings.TrimSuffix(base, "/v1/chat/completions")
			} else if strings.HasSuffix(base, "/v1") {
				// keep it
			}
			url = base + "/embeddings"
			if target.Provider == provider.ProviderOpenAI {
				headers["Authorization"] = "Bearer " + upstreamKey
			} else if target.Provider == provider.ProviderAnthropic {
				headers["x-api-key"] = upstreamKey
				headers["anthropic-version"] = "2023-06-01"
			}
		} else {
			// Built-in OpenAI
			url = "https://api.openai.com/v1/embeddings"
			headers["Authorization"] = "Bearer " + upstreamKey
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(params.RequestBody))
		if err != nil {
			return fmt.Errorf("failed to create http request: %w", err)
		}
		for k, v := range headers {
			httpReq.Header.Set(k, v)
		}

		resp, err := g.providerClient.Do(httpReq)
		if err != nil {
			g.recordTelemetry(params, target.Name, embedReq.Model, 0, 0, 0, 0, analytics.CostResult{}, 0, http.StatusInternalServerError, err.Error(), startTime)
			return fmt.Errorf("failed sending request to upstream: %w", err)
		}
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		params.ResponseWriter.Header().Set("Content-Type", "application/json")
		params.ResponseWriter.WriteHeader(resp.StatusCode)
		params.ResponseWriter.Write(bodyBytes)

		var keyID, userID string
		if params.ApiKey != nil {
			keyID = params.ApiKey.ID.String()
			userID = params.ApiKey.UserID.String()
		}
		telemetry.RecordDurationMetric(ctx, target.Name, embedReq.Model, "embeddings", time.Since(startTime).Seconds(), resp.StatusCode)

		if resp.StatusCode == http.StatusOK {
			var respData struct {
				Usage struct {
					PromptTokens int `json:"prompt_tokens"`
					TotalTokens  int `json:"total_tokens"`
				} `json:"usage"`
			}
			_ = json.Unmarshal(bodyBytes, &respData)

			tokens := respData.Usage.PromptTokens
			if tokens <= 0 {
				tokens = promptTokens
			}
			costResult := g.costCalc.CalculateCost(target.Name, embedReq.Model, tokens, 0, 0, 0)
			g.recordTelemetry(params, target.Name, embedReq.Model, tokens, 0, 0, 0, costResult, 0, resp.StatusCode, "", startTime)
			if span := trace.SpanFromContext(ctx); span != nil && span.IsRecording() {
				telemetry.SetGenAIAttributes(span, target.Name, embedReq.Model, "embeddings")
				telemetry.SetGenAIUsage(span, tokens, 0)
				telemetry.SetLLMGWAttributes(span, target.Name, false, false, 0)
			}
			telemetry.RecordTokenMetrics(ctx, target.Name, embedReq.Model, keyID, userID, tokens, 0, 0)
			telemetry.RecordCostMetric(ctx, target.Name, embedReq.Model, keyID, userID, costResult.Cost)
		} else {
			g.recordTelemetry(params, target.Name, embedReq.Model, 0, 0, 0, 0, analytics.CostResult{}, 0, resp.StatusCode, string(bodyBytes), startTime)
		}
	}

	return nil
}
