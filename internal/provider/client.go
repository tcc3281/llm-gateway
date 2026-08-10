package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ProviderType string

const (
	ProviderOpenAI    ProviderType = "openai"
	ProviderAnthropic ProviderType = "anthropic"
	ProviderGemini    ProviderType = "gemini"
)

type ProviderClient struct {
	httpClient *http.Client
}

func NewProviderClient() *ProviderClient {
	return &ProviderClient{
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // Support long LLM generation
		},
	}
}

// ErrNoModelsEndpoint is returned for providers that do not expose a public
// models-list API. Anthropic is currently the only one.
var ErrNoModelsEndpoint = fmt.Errorf("provider has no public models-list API")

// ListModels returns the model IDs advertised by the upstream provider using the
// given apiKey. OpenAI and Gemini expose GET /v1/models; Anthropic does not, so
// its models are surfaced through the DB model_pricings table instead.
func (c *ProviderClient) ListModels(ctx context.Context, p ProviderType, apiKey string) ([]string, error) {
	switch p {
	case ProviderOpenAI:
		return c.listOpenAIModels(ctx, apiKey)
	case ProviderGemini:
		return c.listGeminiModels(ctx, apiKey)
	default:
		return nil, ErrNoModelsEndpoint
	}
}

func (c *ProviderClient) listOpenAIModels(ctx context.Context, apiKey string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai models: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("openai models: decode: %w", err)
	}

	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

func (c *ProviderClient) listGeminiModels(ctx context.Context, apiKey string) ([]string, error) {
	u := "https://generativelanguage.googleapis.com/v1beta/models?key=" + url.QueryEscape(apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini models: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("gemini models: decode: %w", err)
	}

	ids := make([]string, 0, len(out.Models))
	for _, m := range out.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func ResolveProvider(modelName string) ProviderType {
	m := strings.ToLower(modelName)
	if strings.HasPrefix(m, "claude") {
		return ProviderAnthropic
	}
	if strings.HasPrefix(m, "gemini") {
		return ProviderGemini
	}
	return ProviderOpenAI
}

type UpstreamRequest struct {
	Provider ProviderType
	Model    string
	Stream   bool
	Payload  []byte
}

type UpstreamResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

// SendRequest forwards the payload to the upstream provider using the given
// apiKey. The key is resolved at runtime from the stateful provider_keys table
// (via KeyStore) or from a per-request x-upstream-api-key header override.
func (c *ProviderClient) SendRequest(ctx context.Context, req UpstreamRequest, apiKey string) (*UpstreamResponse, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("no API key provided for provider %s", req.Provider)
	}

	url, headers, err := c.buildRequestDetails(req, apiKey)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(req.Payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed sending request to upstream %s: %w", req.Provider, err)
	}

	return &UpstreamResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       resp.Body,
	}, nil
}

func (c *ProviderClient) buildRequestDetails(req UpstreamRequest, apiKey string) (string, map[string]string, error) {
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	switch req.Provider {
	case ProviderOpenAI:
		url := "https://api.openai.com/v1/chat/completions"
		headers["Authorization"] = "Bearer " + apiKey
		return url, headers, nil

	case ProviderAnthropic:
		url := "https://api.anthropic.com/v1/messages"
		headers["x-api-key"] = apiKey
		headers["anthropic-version"] = "2023-06-01"
		return url, headers, nil

	case ProviderGemini:
		endpoint := "generateContent"
		if req.Stream {
			endpoint = "streamGenerateContent?alt=sse"
		}
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:%s?key=%s", req.Model, endpoint, apiKey)
		return url, headers, nil

	default:
		return "", nil, fmt.Errorf("unsupported provider: %s", req.Provider)
	}
}
