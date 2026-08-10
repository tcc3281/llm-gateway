package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"llm-gateway/internal/config"
	"llm-gateway/internal/model"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
}

func NewRedis(cfg *config.Config) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisHost,
		Password: cfg.RedisPass,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return &RedisClient{client: client}, nil
}

// API Key Caching
func (r *RedisClient) GetCachedKey(ctx context.Context, keyHash string) (*model.APIKey, error) {
	cacheKey := fmt.Sprintf("apikey:%s", keyHash)
	val, err := r.client.Get(ctx, cacheKey).Result()
	if err != nil {
		return nil, err
	}

	var apiKey model.APIKey
	if err := json.Unmarshal([]byte(val), &apiKey); err != nil {
		return nil, err
	}
	return &apiKey, nil
}

func (r *RedisClient) SetCachedKey(ctx context.Context, apiKey *model.APIKey, ttl time.Duration) error {
	cacheKey := fmt.Sprintf("apikey:%s", apiKey.KeyHash)
	data, err := json.Marshal(apiKey)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, cacheKey, data, ttl).Err()
}

// DeleteCachedKey invalidates a cached API key immediately (used on revoke).
func (r *RedisClient) DeleteCachedKey(ctx context.Context, keyHash string) error {
	return r.client.Del(ctx, fmt.Sprintf("apikey:%s", keyHash)).Err()
}

// Provider API Key Caching (stateful keys read from DB, cached for speed)
func (r *RedisClient) GetCachedProviderKey(ctx context.Context, provider string) (string, error) {
	return r.client.Get(ctx, fmt.Sprintf("provkey:%s", provider)).Result()
}

func (r *RedisClient) SetCachedProviderKey(ctx context.Context, provider, apiKey string, ttl time.Duration) error {
	return r.client.Set(ctx, fmt.Sprintf("provkey:%s", provider), apiKey, ttl).Err()
}

func (r *RedisClient) DeleteCachedProviderKey(ctx context.Context, provider string) error {
	return r.client.Del(ctx, fmt.Sprintf("provkey:%s", provider)).Err()
}

// Models List Caching (GET /v1/models result, refreshed live from providers)
const modelsCacheKey = "models:list"

func (r *RedisClient) GetCachedModels(ctx context.Context) ([]model.ProviderModel, error) {
	val, err := r.client.Get(ctx, modelsCacheKey).Result()
	if err != nil {
		return nil, err
	}
	var models []model.ProviderModel
	if err := json.Unmarshal([]byte(val), &models); err != nil {
		return nil, err
	}
	return models, nil
}

func (r *RedisClient) SetCachedModels(ctx context.Context, models []model.ProviderModel, ttl time.Duration) error {
	data, err := json.Marshal(models)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, modelsCacheKey, data, ttl).Err()
}

// InvalidateModelsCache drops the cached /v1/models list so the next request
// re-fetches live from providers (used when provider keys or pricing change).
func (r *RedisClient) InvalidateModelsCache(ctx context.Context) error {
	return r.client.Del(ctx, modelsCacheKey).Err()
}

// Rate Limiter: Fixed Window / Sliding Counter per minute
func (r *RedisClient) AllowRequest(ctx context.Context, keyID string, limitRPM int) (bool, error) {
	if limitRPM <= 0 {
		return true, nil // Unlimited
	}

	now := time.Now()
	minuteKey := fmt.Sprintf("ratelimit:%s:%s", keyID, now.Format("2006-01-02T15:04"))

	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, minuteKey)
	pipe.Expire(ctx, minuteKey, 2*time.Minute)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return true, nil // Fallback on redis error to not block traffic
	}

	return incr.Val() <= int64(limitRPM), nil
}
