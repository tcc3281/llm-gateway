package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/db"
	"llm-gateway/internal/model"
)

const DefaultResponseCacheTTL = time.Hour // used when key.CacheTTLSeconds == 0

type ResponseCache struct {
	redis     *db.RedisClient
	globalTTL time.Duration
}

func NewResponseCache(redis *db.RedisClient, globalTTL time.Duration) *ResponseCache {
	if globalTTL <= 0 {
		globalTTL = DefaultResponseCacheTTL
	}
	return &ResponseCache{redis: redis, globalTTL: globalTTL}
}

// CacheKey computes a deterministic SHA256 cache key from a canonical request.
// Structure fields: Model, Messages, MaxTokens, Temperature, Tools.
func CacheKey(req *canonical.CanonicalRequest) string {
	type fingerprint struct {
		Model       string                      `json:"model"`
		Messages    []canonical.ChatMessage     `json:"messages"`
		MaxTokens   int                         `json:"max_tokens,omitempty"`
		Temperature *float64                    `json:"temperature,omitempty"`
		Tools       []canonical.ToolDefinition `json:"tools,omitempty"`
	}
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(fingerprint{
		Model:       req.Model,
		Messages:    req.Messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Tools:       req.Tools,
	})
	return "llmcache:" + hex.EncodeToString(h.Sum(nil))
}

// EffectiveTTL returns the TTL to use for this key's cache entries.
// Returns 0, false if cache is disabled for this key.
func (c *ResponseCache) EffectiveTTL(apiKey *model.APIKey) (time.Duration, bool) {
	if apiKey == nil || apiKey.CacheTTLSeconds == -1 {
		return 0, false // cache explicitly disabled
	}
	if apiKey.CacheTTLSeconds == 0 {
		return c.globalTTL, true
	}
	return time.Duration(apiKey.CacheTTLSeconds) * time.Second, true
}

func (c *ResponseCache) Get(ctx context.Context, key string) ([]byte, bool) {
	if c == nil || c.redis == nil {
		return nil, false
	}
	val, err := c.redis.GetRaw(ctx, key)
	if err != nil || len(val) == 0 {
		return nil, false
	}
	return val, true
}

func (c *ResponseCache) Set(ctx context.Context, key string, body []byte, ttl time.Duration) {
	if c == nil || c.redis == nil || ttl <= 0 || len(body) == 0 {
		return
	}
	_ = c.redis.SetRaw(ctx, key, body, ttl)
}
