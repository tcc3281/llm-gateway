package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"llm-gateway/internal/db"
	"llm-gateway/internal/model"

	"github.com/google/uuid"
)

type AuthService struct {
	pg          *db.PostgresDB
	redis       *db.RedisClient
	adminKey    string
}

func NewAuthService(pg *db.PostgresDB, redis *db.RedisClient, adminKey string) *AuthService {
	return &AuthService{
		pg:       pg,
		redis:    redis,
		adminKey: adminKey,
	}
}

// GenerateSecretKey creates a key like: sk-gw-abcdef123456... (returns plain key & struct)
func (s *AuthService) CreateAPIKey(ctx context.Context, userID uuid.UUID, name string, budgetLimit float64, rateLimitRPM int, cacheTTLSeconds int) (string, *model.APIKey, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, err
	}
	secretHex := hex.EncodeToString(bytes)
	plainKey := fmt.Sprintf("sk-gw-%s", secretHex)
	prefix := plainKey[:10]
	hash := s.HashKey(plainKey)

	apiKey := &model.APIKey{
		UserID:          userID,
		KeyPrefix:       prefix,
		KeyHash:         hash,
		Name:            name,
		BudgetLimit:     budgetLimit,
		RateLimitRPM:    rateLimitRPM,
		CacheTTLSeconds: cacheTTLSeconds,
		IsActive:        true,
	}

	if err := s.pg.CreateAPIKey(apiKey); err != nil {
		return "", nil, err
	}

	return plainKey, apiKey, nil
}

func (s *AuthService) ValidateAPIKey(ctx context.Context, plainKey string) (*model.APIKey, error) {
	if plainKey == "" {
		return nil, errors.New("empty API key")
	}

	hash := s.HashKey(plainKey)

	// 1. Try Redis cache first
	if s.redis != nil {
		cachedKey, err := s.redis.GetCachedKey(ctx, hash)
		if err == nil && cachedKey != nil {
			if !cachedKey.IsActive {
				return nil, errors.New("API key is inactive")
			}
			return cachedKey, nil
		}
	}

	// 2. Fallback to PostgreSQL
	apiKey, err := s.pg.GetAPIKeyByHash(hash)
	if err != nil {
		return nil, errors.New("invalid API key")
	}

	if !apiKey.IsActive {
		return nil, errors.New("API key is inactive")
	}

	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("API key has expired")
	}

	// Cache valid key in Redis for 10 minutes
	if s.redis != nil {
		_ = s.redis.SetCachedKey(ctx, apiKey, 10*time.Minute)
	}

	return apiKey, nil
}

func (s *AuthService) ValidateAdminKey(key string) bool {
	return key != "" && key == s.adminKey
}

// RevokeAPIKey deactivates a key in PostgreSQL and invalidates its Redis cache
// immediately so the key stops working even before the cache TTL expires.
func (s *AuthService) RevokeAPIKey(ctx context.Context, id uuid.UUID) error {
	if s.pg == nil {
		return errors.New("database unavailable")
	}
	key, err := s.pg.GetAPIKeyByID(id)
	if err != nil {
		return err
	}
	if err := s.pg.DeactivateAPIKey(id); err != nil {
		return err
	}
	if s.redis != nil {
		_ = s.redis.DeleteCachedKey(ctx, key.KeyHash)
		_ = s.redis.DeleteCachedBudgetSpent(ctx, key.ID.String())
	}
	return nil
}

// UpdateAPIKey edits a key's name, budget, rate limit, cache TTL and expiry, then
// invalidates the Redis cache so the new limits apply immediately (cached keys
// would otherwise live up to their 10-minute TTL).
func (s *AuthService) UpdateAPIKey(ctx context.Context, id uuid.UUID, name string, budgetLimit float64, rateLimitRPM int, cacheTTLSeconds int, expiresAt *time.Time) error {
	if s.pg == nil {
		return errors.New("database unavailable")
	}
	key, err := s.pg.GetAPIKeyByID(id)
	if err != nil {
		return err
	}
	if err := s.pg.UpdateAPIKey(id, name, budgetLimit, rateLimitRPM, cacheTTLSeconds, expiresAt); err != nil {
		return err
	}
	if s.redis != nil {
		_ = s.redis.DeleteCachedKey(ctx, key.KeyHash)
		_ = s.redis.DeleteCachedBudgetSpent(ctx, key.ID.String())
	}
	return nil
}

// ReactivateAPIKey re-enables a previously revoked key and invalidates its
// Redis cache so the new state is authoritative.
func (s *AuthService) ReactivateAPIKey(ctx context.Context, id uuid.UUID) error {
	if s.pg == nil {
		return errors.New("database unavailable")
	}
	key, err := s.pg.GetAPIKeyByID(id)
	if err != nil {
		return err
	}
	if err := s.pg.ReactivateAPIKey(id); err != nil {
		return err
	}
	if s.redis != nil {
		_ = s.redis.DeleteCachedKey(ctx, key.KeyHash)
	}
	return nil
}

func (s *AuthService) HashKey(plainKey string) string {
	h := sha256.Sum256([]byte(plainKey))
	return hex.EncodeToString(h[:])
}
