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
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 300
	transport.MaxIdleConnsPerHost = 100
	transport.IdleConnTimeout = 90 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	// A streaming response can legitimately outlast any fixed client timeout
	// (long thinking + generation), so the overall request must be unbounded.
	// ResponseHeaderTimeout only covers the gap up to the upstream's first byte;
	// once headers arrive the stream is read at its own pace. A provider that
	// hangs before responding (dead LB, stalled key validation) still fails fast
	// here instead of tying up the goroutine forever.
	transport.ResponseHeaderTimeout = 60 * time.Second
	transport.ForceAttemptHTTP2 = true

	return &ProviderClient{
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

// Do forwards an arbitrary HTTP request using the shared pooled client.
func (c *ProviderClient) Do(req *http.Request) (*http.Response, error) {
	return c.httpClient.Do(req)
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
	case ProviderAnthropic:
		return c.listAnthropicModels(ctx, apiKey)
	case ProviderGemini:
		return c.listGeminiModels(ctx, apiKey)
	default:
		return nil, ErrNoModelsEndpoint
	}
}

func (c *ProviderClient) listAnthropicModels(ctx context.Context, apiKey string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic models: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("anthropic models: decode: %w", err)
	}

	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

// ListModelsCustom fetches the model list of an OpenAI/Anthropic-compatible
// provider at a user-configured base URL. URL + auth mirror buildRequestDetails:
// openai → {base}/models with Bearer; anthropic → {root}/v1/models with x-api-key.
func (c *ProviderClient) ListModelsCustom(ctx context.Context, protocol ProviderType, baseURL, apiKey string) ([]string, error) {
	base := normalizeBaseURL(baseURL)
	var listURL, authName, authVal string
	switch protocol {
	case ProviderOpenAI:
		listURL = base + "/models"
		authName, authVal = "Authorization", "Bearer "+apiKey
	case ProviderAnthropic:
		// Avoid "https://x.com/v1/v1/models": strip a bare trailing /v1 first.
		listURL = strings.TrimSuffix(base, "/v1") + "/v1/models"
		authName, authVal = "x-api-key", apiKey
	default:
		return nil, fmt.Errorf("custom provider %s: protocol %s unsupported for model listing", baseURL, protocol)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(authName, authVal)
	if protocol == ProviderAnthropic {
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("custom models (%s): %w", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("custom models (%s): HTTP %d: %s", baseURL, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("custom models (%s): decode: %w", baseURL, err)
	}

	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
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
	// Provider is the wire protocol spoken to the upstream: it selects the
	// request encoding, response decoding and auth header style. For custom
	// OpenAI/Anthropic-compatible providers it is the provider's configured
	// protocol (openai | anthropic).
	Provider ProviderType
	// Name is the actual provider name for key lookup, cost and telemetry. For
	// built-ins it equals string(Provider); for custom providers it is the
	// model-prefix segment (e.g. "openrouter").
	Name string
	// BaseURL overrides the built-in upstream URL. Empty means built-in.
	BaseURL string
	Model   string
	Stream  bool
	Payload []byte

	// ExtraHeaders injected by plugins (e.g. header-transform)
	ExtraHeaders map[string]string
	// RemoveHeaders stripped by plugins
	RemoveHeaders []string
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
	for k, v := range req.ExtraHeaders {
		httpReq.Header.Set(k, v)
	}
	for _, r := range req.RemoveHeaders {
		httpReq.Header.Del(r)
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

	// Custom OpenAI/Anthropic-compatible provider: send the wire protocol to a
	// user-configured base URL. req.Provider already carries the configured
	// protocol, so this mirrors the built-in URL + header shapes below.
	if req.BaseURL != "" {
		base := normalizeBaseURL(req.BaseURL)
		switch req.Provider {
		case ProviderOpenAI:
			headers["Authorization"] = "Bearer " + apiKey
			return base + "/chat/completions", headers, nil
		case ProviderAnthropic:
			headers["x-api-key"] = apiKey
			headers["anthropic-version"] = "2023-06-01"
			// Avoid "https://x.com/v1/v1/messages": the messages endpoint already
			// includes the /v1 prefix, so strip a bare trailing /v1 first.
			base = strings.TrimSuffix(base, "/v1")
			return base + "/v1/messages", headers, nil
		default:
			return "", nil, fmt.Errorf("custom provider %s: protocol %s unsupported", req.Name, req.Provider)
		}
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
		// Streaming uses streamGenerateContent with alt=sse; both query params
		// must be combined with & (not appended after alt, which would produce
		// "?alt=sse?key=..." and make alt's value invalid).
		query := "key=" + url.QueryEscape(apiKey)
		endpoint := "generateContent"
		if req.Stream {
			endpoint = "streamGenerateContent"
			query = "alt=sse&" + query
		}
		url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:%s?%s", req.Model, endpoint, query)
		return url, headers, nil

	default:
		return "", nil, fmt.Errorf("unsupported provider: %s", req.Provider)
	}
}

// normalizeBaseURL trims a trailing slash and passes through a base URL that
// already includes the full path, so users may configure either the API root
// ("https://api.example.com/v1") or the full endpoint.
func normalizeBaseURL(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/chat/completions") || strings.HasSuffix(base, "/v1/messages") {
		return base
	}
	return base
}
