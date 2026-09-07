package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
	if key == "" || s.adminKey == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(key), []byte(s.adminKey)) == 1
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

// HashPassword hashes a plain text password with a 16-byte random salt using HMAC-SHA256
func (s *AuthService) HashPassword(password string) (string, error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", err
	}
	saltHex := hex.EncodeToString(saltBytes)

	mac := hmac.New(sha256.New, saltBytes)
	mac.Write([]byte(password))
	hashHex := hex.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("sha256$%s$%s", saltHex, hashHex), nil
}

// VerifyPassword verifies a plain text password against a stored salted hash
func (s *AuthService) VerifyPassword(password, storedHash string) bool {
	if password == "" || storedHash == "" {
		return false
	}
	parts := strings.Split(storedHash, "$")
	if len(parts) != 3 || parts[0] != "sha256" {
		return false
	}
	saltBytes, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	expectedHashBytes, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, saltBytes)
	mac.Write([]byte(password))
	computedHashBytes := mac.Sum(nil)

	return subtle.ConstantTimeCompare(expectedHashBytes, computedHashBytes) == 1
}

// SessionClaims carries identity information in signed session tokens
type SessionClaims struct {
	UserID    uuid.UUID `json:"user_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	IsRoot    bool      `json:"is_root"`
	ExpiresAt int64     `json:"exp"`
}

// CreateSessionToken generates a tamper-proof HMAC-SHA256 signed bearer token
func (s *AuthService) CreateSessionToken(user *model.User, duration time.Duration) (string, error) {
	expiresAt := time.Now().Add(duration).Unix()
	payloadRaw := fmt.Sprintf("%s|%s|%s|%s|%t|%d",
		user.ID.String(),
		user.Email,
		user.Name,
		user.Role,
		user.IsRoot,
		expiresAt,
	)
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payloadRaw))

	secret := []byte(s.adminKey)
	if len(secret) == 0 {
		secret = []byte("default-emergency-signing-key")
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payloadB64))
	sigHex := hex.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("%s.%s", payloadB64, sigHex), nil
}

// VerifySessionToken verifies and decodes a signed bearer session token
func (s *AuthService) VerifySessionToken(tokenString string) (*SessionClaims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid token format")
	}
	payloadB64 := parts[0]
	sigHex := parts[1]

	secret := []byte(s.adminKey)
	if len(secret) == 0 {
		secret = []byte("default-emergency-signing-key")
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payloadB64))
	computedSigHex := hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(sigHex), []byte(computedSigHex)) != 1 {
		return nil, errors.New("invalid token signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, errors.New("malformed token payload")
	}

	fields := strings.Split(string(payloadBytes), "|")
	if len(fields) < 6 {
		return nil, errors.New("invalid claims payload")
	}

	uid, err := uuid.Parse(fields[0])
	if err != nil {
		return nil, errors.New("invalid user id in claims")
	}

	isRoot, _ := strconv.ParseBool(fields[4])
	expiresAt, err := strconv.ParseInt(fields[5], 10, 64)
	if err != nil {
		return nil, errors.New("invalid expiry in claims")
	}

	if time.Now().Unix() > expiresAt {
		return nil, errors.New("session token expired")
	}

	return &SessionClaims{
		UserID:    uid,
		Email:     fields[1],
		Name:      fields[2],
		Role:      fields[3],
		IsRoot:    isRoot,
		ExpiresAt: expiresAt,
	}, nil
}

