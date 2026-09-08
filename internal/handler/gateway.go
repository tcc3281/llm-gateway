package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"llm-gateway/internal/auth"
	"llm-gateway/internal/db"
	"llm-gateway/internal/model"
	"llm-gateway/internal/service"
	"llm-gateway/internal/telemetry"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type GatewayHandler struct {
	authService *auth.AuthService
	gwService   *service.GatewayService
	redis       *db.RedisClient
	pg          *db.PostgresDB
}

func NewGatewayHandler(authService *auth.AuthService, gwService *service.GatewayService, redis *db.RedisClient, pg *db.PostgresDB) *GatewayHandler {
	return &GatewayHandler{
		authService: authService,
		gwService:   gwService,
		redis:       redis,
		pg:          pg,
	}
}

type GateErr struct {
	StatusCode int
	Message    string
	Type       string
	Code       string
}

func (e *GateErr) Error() string {
	return e.Message
}

// OpenAI Compatible Endpoint: POST /v1/chat/completions
func (h *GatewayHandler) HandleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	tracer := otel.Tracer(telemetry.TracerName)
	ctx, span := tracer.Start(ctx, "llmgw.chat_completions")
	defer span.End()

	apiKey, gateErr := h.authenticate(r)
	if gateErr != nil {
		span.RecordError(gateErr)
		WriteErrorResponse(w, "openai", gateErr.StatusCode, gateErr.Message, gateErr.Type, gateErr.Code)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		span.RecordError(err)
		WriteErrorResponse(w, "openai", http.StatusBadRequest, "Failed to read request body", "invalid_request_error", "bad_request")
		return
	}

	params := service.ProxyParams{
		InboundFormat:  "openai",
		RequestBody:    body,
		ApiKey:         apiKey,
		UpstreamAPIKey: r.Header.Get("x-upstream-api-key"),
		ResponseWriter: w,
	}

	if err := h.gwService.HandleRequest(ctx, params); err != nil {
		span.RecordError(err)
		h.writeServiceError(w, "openai", err)
	}
}

// OpenAI Compatible Endpoint: POST /v1/embeddings
func (h *GatewayHandler) HandleEmbeddings(w http.ResponseWriter, r *http.Request) {
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	tracer := otel.Tracer(telemetry.TracerName)
	ctx, span := tracer.Start(ctx, "llmgw.embeddings")
	defer span.End()

	apiKey, gateErr := h.authenticate(r)
	if gateErr != nil {
		span.RecordError(gateErr)
		WriteErrorResponse(w, "openai", gateErr.StatusCode, gateErr.Message, gateErr.Type, gateErr.Code)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		span.RecordError(err)
		WriteErrorResponse(w, "openai", http.StatusBadRequest, "Failed to read request body", "invalid_request_error", "bad_request")
		return
	}

	params := service.ProxyParams{
		InboundFormat:  "openai",
		RequestBody:    body,
		ApiKey:         apiKey,
		UpstreamAPIKey: r.Header.Get("x-upstream-api-key"),
		ResponseWriter: w,
	}

	if err := h.gwService.HandleEmbeddings(ctx, params); err != nil {
		span.RecordError(err)
		h.writeServiceError(w, "openai", err)
	}
}

// Anthropic Compatible Endpoint: POST /v1/messages
func (h *GatewayHandler) HandleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	tracer := otel.Tracer(telemetry.TracerName)
	ctx, span := tracer.Start(ctx, "llmgw.messages")
	defer span.End()

	apiKey, gateErr := h.authenticate(r)
	if gateErr != nil {
		span.RecordError(gateErr)
		WriteErrorResponse(w, "anthropic", gateErr.StatusCode, gateErr.Message, gateErr.Type, gateErr.Code)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		span.RecordError(err)
		WriteErrorResponse(w, "anthropic", http.StatusBadRequest, "Failed to read request body", "invalid_request_error", "bad_request")
		return
	}

	params := service.ProxyParams{
		InboundFormat:  "anthropic",
		RequestBody:    body,
		ApiKey:         apiKey,
		UpstreamAPIKey: r.Header.Get("x-upstream-api-key"),
		ResponseWriter: w,
	}

	if err := h.gwService.HandleRequest(ctx, params); err != nil {
		span.RecordError(err)
		h.writeServiceError(w, "anthropic", err)
	}
}

// OpenAI Models Endpoint: GET /v1/models (auth required, live provider list)
func (h *GatewayHandler) HandleListModels(w http.ResponseWriter, r *http.Request) {
	if _, gateErr := h.authenticate(r); gateErr != nil {
		WriteErrorResponse(w, "openai", gateErr.StatusCode, gateErr.Message, gateErr.Type, gateErr.Code)
		return
	}

	models, err := h.gwService.ListAvailableModels(r.Context())
	if err != nil {
		WriteErrorResponse(w, "openai", http.StatusInternalServerError, "Failed to load models: "+err.Error(), "api_error", "internal_error")
		return
	}

	response := map[string]interface{}{
		"object": "list",
		"data":   models,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// OpenAI Retrieve Model Endpoint: GET /v1/models/{id} (auth required)
func (h *GatewayHandler) HandleGetModel(w http.ResponseWriter, r *http.Request) {
	if _, gateErr := h.authenticate(r); gateErr != nil {
		WriteErrorResponse(w, "openai", gateErr.StatusCode, gateErr.Message, gateErr.Type, gateErr.Code)
		return
	}

	modelID := r.PathValue("id")
	if modelID == "" {
		// Fallback for custom routing if PathValue is empty
		parts := strings.Split(r.URL.Path, "/v1/models/")
		if len(parts) > 1 {
			modelID = parts[1]
		}
	}

	if modelID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Model ID is required",
				"type":    "invalid_request_error",
				"param":   "model",
				"code":    "missing_model_id",
			},
		})
		return
	}

	models, err := h.gwService.ListAvailableModels(r.Context())
	if err != nil {
		WriteErrorResponse(w, "openai", http.StatusInternalServerError, "Failed to load models: "+err.Error(), "api_error", "internal_error")
		return
	}

	for _, m := range models {
		if m.ID == modelID {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(m)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": fmt.Sprintf("The model '%s' does not exist", modelID),
			"type":    "invalid_request_error",
			"param":   "model",
			"code":    "model_not_found",
		},
	})
}

func (h *GatewayHandler) authenticate(r *http.Request) (*model.APIKey, *GateErr) {
	authHeader := r.Header.Get("Authorization")
	plainKey := ""
	if strings.HasPrefix(authHeader, "Bearer ") {
		plainKey = strings.TrimPrefix(authHeader, "Bearer ")
	} else if xApiKey := r.Header.Get("x-api-key"); xApiKey != "" {
		plainKey = xApiKey
	}

	if plainKey == "" {
		return nil, &GateErr{
			StatusCode: http.StatusUnauthorized,
			Message:    "You must provide an API key in the Authorization header (e.g. Bearer sk-gw-...) or x-api-key header.",
			Type:       "invalid_request_error",
			Code:       "missing_api_key",
		}
	}

	apiKey, err := h.authService.ValidateAPIKey(r.Context(), plainKey)
	if err != nil {
		return nil, &GateErr{
			StatusCode: http.StatusUnauthorized,
			Message:    "Incorrect API key provided: " + err.Error(),
			Type:       "invalid_request_error",
			Code:       "invalid_api_key",
		}
	}

	// 1. Check Rate Limiting (RPM - Requests Per Minute)
	if h.redis != nil && apiKey.RateLimitRPM > 0 {
		allowed, err := h.redis.AllowRequest(r.Context(), apiKey.ID.String(), apiKey.RateLimitRPM)
		if err == nil && !allowed {
			if h.gwService != nil && h.gwService.AlertEngine() != nil {
				h.gwService.AlertEngine().Emit(model.AlertEvent{
					EventType: "rate_limit_surge",
					TenantID:  apiKey.TenantID,
					Value:     1.0,
					Message:   fmt.Sprintf("Rate limit exceeded (%d RPM) on API key %s (%s)", apiKey.RateLimitRPM, apiKey.Name, apiKey.KeyPrefix),
					Metadata: map[string]interface{}{
						"key_id":     apiKey.ID.String(),
						"key_name":   apiKey.Name,
						"key_prefix": apiKey.KeyPrefix,
						"rpm":        apiKey.RateLimitRPM,
					},
					Timestamp: time.Now(),
				})
			}
			return nil, &GateErr{
				StatusCode: http.StatusTooManyRequests,
				Message:    fmt.Sprintf("Rate limit exceeded (%d RPM). Please slow down your requests.", apiKey.RateLimitRPM),
				Type:       "requests_error",
				Code:       "rate_limit_exceeded",
			}
		}
	}

	// 2. Check Budget Limit Quota (cached in Redis to avoid synchronous PostgreSQL aggregation on hot path)
	if apiKey.BudgetLimit > 0 {
		var spent float64
		var err error
		gotFromCache := false

		if h.redis != nil {
			if cachedSpent, cErr := h.redis.GetCachedBudgetSpent(r.Context(), apiKey.ID.String()); cErr == nil {
				spent = cachedSpent
				gotFromCache = true
			}
		}

		if !gotFromCache && h.pg != nil {
			spent, err = h.pg.GetSpentCostByKeyID(apiKey.ID)
			if err == nil && h.redis != nil {
				_ = h.redis.SetCachedBudgetSpent(r.Context(), apiKey.ID.String(), spent, 60*time.Second)
			}
		}

		if err == nil && spent >= apiKey.BudgetLimit {
			return nil, &GateErr{
				StatusCode: http.StatusTooManyRequests,
				Message:    fmt.Sprintf("Budget limit of $%.2f has been reached for this API Key (Spent: $%.2f).", apiKey.BudgetLimit, spent),
				Type:       "insufficient_quota",
				Code:       "insufficient_quota",
			}
		}
	}

	return apiKey, nil
}

// writeServiceError maps gateway service errors to the client's protocol format.
// A missing provider key is surfaced as 503 Service Unavailable so operators
// can distinguish "gateway misconfigured" from "upstream failed". A model that
// is not in the registry is surfaced as 404 model_not_found — the gateway never
// guesses an upstream from the model-name prefix.
func (h *GatewayHandler) writeServiceError(w http.ResponseWriter, inboundFormat string, err error) {
	if service.IsPaymentRequiredError(err) {
		WriteErrorResponse(w, inboundFormat, http.StatusPaymentRequired, err.Error(), "insufficient_quota", "payment_required")
		return
	}
	if service.IsPermissionDeniedError(err) {
		if h.gwService != nil && h.gwService.AlertEngine() != nil {
			h.gwService.AlertEngine().Emit(model.AlertEvent{
				EventType: "iam_deny_surge",
				Value:     1.0,
				Message:   fmt.Sprintf("IAM policy permission denied: %s", err.Error()),
				Metadata: map[string]interface{}{
					"error": err.Error(),
				},
				Timestamp: time.Now(),
			})
		}
		WriteErrorResponse(w, inboundFormat, http.StatusForbidden, err.Error(), "permission_error", "forbidden")
		return
	}
	if service.IsUnknownModelError(err) {
		WriteErrorResponse(w, inboundFormat, http.StatusNotFound, err.Error(), "invalid_request_error", "model_not_found")
		return
	}
	if service.IsProviderKeyError(err) {
		WriteErrorResponse(w, inboundFormat, http.StatusServiceUnavailable, err.Error(), "api_error", "provider_key_missing")
		return
	}
	WriteErrorResponse(w, inboundFormat, http.StatusInternalServerError, err.Error(), "api_error", "internal_error")
}
