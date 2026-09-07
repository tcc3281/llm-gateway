package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"llm-gateway/internal/adapter/anthropic"
	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/adapter/gemini"
	"llm-gateway/internal/adapter/openai"
	"llm-gateway/internal/provider"
)

// isFallbackableStatus reports whether an upstream HTTP status code warrants
// attempting the next provider in the fallback chain.
// 429 (rate-limit) and 5xx (server errors/gateway timeouts) trigger fallback.
// Client errors (4xx other than 429) indicate a bad request and must not fallback.
func isFallbackableStatus(code int) bool {
	return code == http.StatusTooManyRequests ||
		code == http.StatusInternalServerError ||
		code == http.StatusBadGateway ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusGatewayTimeout
}

// FallbackAttempt records the outcome of attempting one upstream provider.
type FallbackAttempt struct {
	Provider   string
	StatusCode int
	Err        error
	Latency    time.Duration
}

// resolveFallbackChain queries PostgreSQL for active providers with a non-null
// fallback_priority, ordered by priority ASC, and returns [primaryTarget, ...fallbacks].
func (g *GatewayService) resolveFallbackChain(ctx context.Context, primaryTarget ResolvedTarget) []ResolvedTarget {
	chain := []ResolvedTarget{primaryTarget}
	if g.pg == nil {
		return chain
	}

	fbProviders, err := g.pg.GetFallbackProviders(primaryTarget.Name)
	if err != nil || len(fbProviders) == 0 {
		return chain
	}

	for _, pk := range fbProviders {
		// Do not duplicate primary target
		if pk.Provider == primaryTarget.Name {
			continue
		}
		var pt provider.ProviderType
		switch pk.Provider {
		case "openai":
			pt = provider.ProviderOpenAI
		case "anthropic":
			pt = provider.ProviderAnthropic
		case "gemini":
			pt = provider.ProviderGemini
		default:
			pt = provider.ProviderOpenAI
			if pk.Protocol == "anthropic" {
				pt = provider.ProviderAnthropic
			}
		}
		chain = append(chain, ResolvedTarget{
			Provider: pt,
			Name:     pk.Provider,
			BaseURL:  pk.BaseURL,
		})
	}
	return chain
}

// fallbackModelForTarget determines the appropriate model name when dispatching
// to target. If target speaks OpenAI and has custom BaseURL, or if target matches
// the original model, it preserves the original model. Otherwise it maps between
// standard model tiers (pro vs flash/mini).
func (g *GatewayService) fallbackModelForTarget(canReq *canonical.CanonicalRequest, target ResolvedTarget) string {
	origModel := canReq.Model

	// Custom OpenAI-compatible provider with base_url
	if target.Provider == provider.ProviderOpenAI && target.BaseURL != "" {
		return origModel
	}

	lowerModel := strings.ToLower(origModel)
	isLightTier := strings.Contains(lowerModel, "mini") ||
		strings.Contains(lowerModel, "flash") ||
		strings.Contains(lowerModel, "haiku") ||
		strings.Contains(lowerModel, "nano")

	switch target.Name {
	case "anthropic":
		if strings.HasPrefix(lowerModel, "claude") {
			return origModel
		}
		if isLightTier {
			return "claude-3-5-haiku-20241022"
		}
		return "claude-3-5-sonnet-20241022"

	case "gemini":
		if strings.HasPrefix(lowerModel, "gemini") {
			return origModel
		}
		if isLightTier {
			return "gemini-1.5-flash"
		}
		return "gemini-1.5-pro"

	case "openai":
		if !strings.HasPrefix(lowerModel, "claude") && !strings.HasPrefix(lowerModel, "gemini") {
			return origModel
		}
		if isLightTier {
			return "gpt-4o-mini"
		}
		return "gpt-4o"

	default:
		return origModel
	}
}

// encodeForTarget encodes the canonical request into wire payload for target.
func encodeForTarget(canReq *canonical.CanonicalRequest, target ResolvedTarget, modelForTarget string) ([]byte, error) {
	reqCopy := *canReq
	reqCopy.Model = modelForTarget

	switch target.Provider {
	case provider.ProviderOpenAI:
		return openai.EncodeCanonicalRequest(&reqCopy)
	case provider.ProviderAnthropic:
		return anthropic.EncodeCanonicalRequest(&reqCopy)
	case provider.ProviderGemini:
		return gemini.EncodeCanonicalRequest(&reqCopy)
	default:
		return nil, fmt.Errorf("unsupported provider type %v", target.Provider)
	}
}

// sendWithFallback tries providers in chain order until one succeeds or the chain
// is exhausted.
func (g *GatewayService) sendWithFallback(
	ctx context.Context,
	targets []ResolvedTarget,
	canReq *canonical.CanonicalRequest,
	overrideKey string,
) (*provider.UpstreamResponse, ResolvedTarget, string, []FallbackAttempt, error) {
	var attempts []FallbackAttempt
	delays := []time.Duration{0, 100 * time.Millisecond, 200 * time.Millisecond}

	for i, target := range targets {
		if i > 0 && i-1 < len(delays) {
			select {
			case <-ctx.Done():
				return nil, ResolvedTarget{}, "", attempts, ctx.Err()
			case <-time.After(delays[i-1]):
			}
		}

		targetModel := g.fallbackModelForTarget(canReq, target)
		payload, err := encodeForTarget(canReq, target, targetModel)
		if err != nil {
			attempts = append(attempts, FallbackAttempt{
				Provider: target.Name,
				Err:      fmt.Errorf("encode failed: %w", err),
			})
			continue
		}

		// Credential lookup
		upstreamKey := overrideKey
		if upstreamKey == "" || i > 0 {
			// Per-request override only applies to primary; fallbacks use KeyStore
			upstreamKey, err = g.keyStore.GetKey(ctx, target.Name)
			if err != nil {
				attempts = append(attempts, FallbackAttempt{
					Provider: target.Name,
					Err:      fmt.Errorf("key lookup failed: %w", err),
				})
				continue
			}
		}

		upReq := provider.UpstreamRequest{
			Provider: target.Provider,
			Name:     target.Name,
			BaseURL:  target.BaseURL,
			Model:    targetModel,
			Stream:   canReq.Stream,
			Payload:  payload,
		}

		start := time.Now()
		resp, err := g.providerClient.SendRequest(ctx, upReq, upstreamKey)
		latency := time.Since(start)

		if err != nil {
			attempts = append(attempts, FallbackAttempt{
				Provider: target.Name,
				Err:      err,
				Latency:  latency,
			})
			log.Printf("Provider %s failed with network error (%v), checking fallback...", target.Name, err)
			continue
		}

		if isFallbackableStatus(resp.StatusCode) {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			attempts = append(attempts, FallbackAttempt{
				Provider:   target.Name,
				StatusCode: resp.StatusCode,
				Err:        fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes))),
				Latency:    latency,
			})
			log.Printf("Provider %s returned HTTP %d, checking fallback...", target.Name, resp.StatusCode)
			continue
		}

		// Success or client-side error (non-fallbackable)
		attempts = append(attempts, FallbackAttempt{
			Provider:   target.Name,
			StatusCode: resp.StatusCode,
			Latency:    latency,
		})
		return resp, target, targetModel, attempts, nil
	}

	lastErr := fmt.Errorf("all %d provider attempt(s) failed", len(attempts))
	if len(attempts) > 0 && attempts[len(attempts)-1].Err != nil {
		lastErr = attempts[len(attempts)-1].Err
	}
	return nil, ResolvedTarget{}, "", attempts, lastErr
}
