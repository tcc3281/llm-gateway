package cache

import (
	"testing"
	"time"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/model"

	"github.com/google/uuid"
)

func TestCacheKeyDeterminism(t *testing.T) {
	temp := 0.7
	req1 := &canonical.CanonicalRequest{
		Model: "gpt-4o",
		Messages: []canonical.ChatMessage{
			{Role: "user", Content: "Explain quantum computing"},
		},
		Temperature: &temp,
		MaxTokens:   500,
	}

	req2 := &canonical.CanonicalRequest{
		Model: "gpt-4o",
		Messages: []canonical.ChatMessage{
			{Role: "user", Content: "Explain quantum computing"},
		},
		Temperature: &temp,
		MaxTokens:   500,
	}

	key1 := CacheKey(req1)
	key2 := CacheKey(req2)

	if key1 == "" {
		t.Fatal("CacheKey should not be empty")
	}

	if key1 != key2 {
		t.Fatalf("CacheKey must be deterministic: key1=%s, key2=%s", key1, key2)
	}

	// Changing message should change cache key
	req3 := &canonical.CanonicalRequest{
		Model: "gpt-4o",
		Messages: []canonical.ChatMessage{
			{Role: "user", Content: "Explain quantum computing in 5 words"},
		},
		Temperature: &temp,
		MaxTokens:   500,
	}
	key3 := CacheKey(req3)
	if key1 == key3 {
		t.Fatalf("Different requests must not produce the same cache key: %s == %s", key1, key3)
	}
}

func TestEffectiveTTL(t *testing.T) {
	globalTTL := 3600 * time.Second
	rc := NewResponseCache(nil, globalTTL)

	// Case 1: nil key -> disabled
	if ttl, ok := rc.EffectiveTTL(nil); ok || ttl != 0 {
		t.Errorf("Expected nil key to disable cache, got ok=%v, ttl=%v", ok, ttl)
	}

	// Case 2: -1 -> disabled
	keyDisabled := &model.APIKey{ID: uuid.New(), CacheTTLSeconds: -1}
	if ttl, ok := rc.EffectiveTTL(keyDisabled); ok || ttl != 0 {
		t.Errorf("Expected CacheTTLSeconds=-1 to disable cache, got ok=%v, ttl=%v", ok, ttl)
	}

	// Case 3: 0 -> inherits global TTL
	keyGlobal := &model.APIKey{ID: uuid.New(), CacheTTLSeconds: 0}
	if ttl, ok := rc.EffectiveTTL(keyGlobal); !ok || ttl != globalTTL {
		t.Errorf("Expected CacheTTLSeconds=0 to use global TTL %v, got ok=%v, ttl=%v", globalTTL, ok, ttl)
	}

	// Case 4: >0 -> custom TTL
	keyCustom := &model.APIKey{ID: uuid.New(), CacheTTLSeconds: 120}
	if ttl, ok := rc.EffectiveTTL(keyCustom); !ok || ttl != 120*time.Second {
		t.Errorf("Expected CacheTTLSeconds=120 to use custom TTL 120s, got ok=%v, ttl=%v", ok, ttl)
	}
}
