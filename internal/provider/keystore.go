package provider

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"llm-gateway/internal/db"
	"llm-gateway/internal/model"
	"llm-gateway/internal/secure"
)

// ErrProviderKeyNotFound is returned when no active key exists for a provider.
var ErrProviderKeyNotFound = errors.New("provider key not found")

// KeyStore is the single source of truth for upstream provider credentials.
// Keys live in the provider_keys table (optionally encrypted at rest) and are
// cached in Redis for sub-millisecond lookups on the hot path.
type KeyStore struct {
	pg        *db.PostgresDB
	redis     *db.RedisClient
	masterKey []byte // nil => plaintext at rest (dev only)
}

func NewKeyStore(pg *db.PostgresDB, redis *db.RedisClient, masterKeyHex string) *KeyStore {
	var key []byte
	if masterKeyHex != "" {
		key = secure.DeriveKey(masterKeyHex)
	}
	return &KeyStore{pg: pg, redis: redis, masterKey: key}
}

// GetKey returns the decrypted API key for a provider, checking Redis first.
func (s *KeyStore) GetKey(ctx context.Context, provider string) (string, error) {
	if s.redis != nil {
		if cached, err := s.redis.GetCachedProviderKey(ctx, provider); err == nil && cached != "" {
			return secure.Decrypt(cached, s.masterKey)
		}
	}

	if s.pg == nil {
		return "", fmt.Errorf("database unavailable")
	}
	pk, err := s.pg.GetProviderKey(provider)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: %s", ErrProviderKeyNotFound, provider)
		}
		return "", err
	}
	plain, err := secure.Decrypt(pk.APIKey, s.masterKey)
	if err != nil {
		return "", err
	}

	if s.redis != nil {
		_ = s.redis.SetCachedProviderKey(ctx, provider, pk.APIKey, 10*time.Minute)
	}
	return plain, nil
}

// SetKey stores (and encrypts) a provider key, then invalidates the cache.
// baseURL is the custom endpoint for OpenAI/Anthropic-compatible providers ("" =
// built-in with a hardcoded upstream URL); protocol is the wire format
// ("openai" | "anthropic") spoken to that base URL.
func (s *KeyStore) SetKey(ctx context.Context, provider, apiKey, baseURL, protocol string) error {
	if s.pg == nil {
		return errors.New("database unavailable")
	}
	enc, err := secure.Encrypt(apiKey, s.masterKey)
	if err != nil {
		return err
	}
	pk := &model.ProviderKey{Provider: provider, APIKey: enc, BaseURL: baseURL, Protocol: protocol, IsActive: true}
	if err := s.pg.UpsertProviderKey(pk); err != nil {
		return err
	}
	if s.redis != nil {
		_ = s.redis.DeleteCachedProviderKey(ctx, provider)
	}
	return nil
}

// ProviderConfig carries the routing-relevant fields of a provider key: the
// custom base URL and the wire protocol, used by the gateway to resolve custom
// OpenAI/Anthropic-compatible providers from a model-name prefix.
type ProviderConfig struct {
	Provider string
	BaseURL  string
	Protocol string
}

// GetProviderInfo returns the config for an active provider key. Built-in
// providers have BaseURL == ""; custom (compatible) providers have a non-empty
// BaseURL. Not found or inactive => ErrProviderKeyNotFound.
func (s *KeyStore) GetProviderInfo(ctx context.Context, provider string) (*ProviderConfig, error) {
	if s.pg == nil {
		return nil, errors.New("database unavailable")
	}
	pk, err := s.pg.GetProviderKey(provider)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrProviderKeyNotFound, provider)
		}
		return nil, err
	}
	return &ProviderConfig{Provider: pk.Provider, BaseURL: pk.BaseURL, Protocol: pk.Protocol}, nil
}

// ListActiveProviders returns the routing config of every active provider key.
// The model-discovery loop iterates this to refresh each provider's catalog.
func (s *KeyStore) ListActiveProviders() ([]ProviderConfig, error) {
	if s.pg == nil {
		return nil, errors.New("database unavailable")
	}
	rows, err := s.pg.ListProviderKeys()
	if err != nil {
		return nil, err
	}
	out := make([]ProviderConfig, 0, len(rows))
	for _, k := range rows {
		if !k.IsActive {
			continue
		}
		out = append(out, ProviderConfig{Provider: k.Provider, BaseURL: k.BaseURL, Protocol: k.Protocol})
	}
	return out, nil
}

// DeleteKey removes a provider key and invalidates the cache.
func (s *KeyStore) DeleteKey(ctx context.Context, provider string) error {
	if s.redis != nil {
		_ = s.redis.DeleteCachedProviderKey(ctx, provider)
	}
	if s.pg == nil {
		return errors.New("database unavailable")
	}
	return s.pg.DeleteProviderKey(provider)
}

// ProviderKeyInfo is the admin-facing view: never exposes the secret.
type ProviderKeyInfo struct {
	Provider   string    `json:"provider"`
	KeyPreview string    `json:"key_preview"`
	BaseURL    string    `json:"base_url,omitempty"` // "" = built-in (hardcoded upstream)
	Protocol   string    `json:"protocol,omitempty"` // "openai" | "anthropic" for custom providers
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ListKeys returns all provider keys with the secret redacted.
func (s *KeyStore) ListKeys() ([]ProviderKeyInfo, error) {
	if s.pg == nil {
		return nil, errors.New("database unavailable")
	}
	rows, err := s.pg.ListProviderKeys()
	if err != nil {
		return nil, err
	}
	out := make([]ProviderKeyInfo, 0, len(rows))
	for _, k := range rows {
		preview := "****"
		if plain, err := secure.Decrypt(k.APIKey, s.masterKey); err == nil {
			preview = secure.Mask(plain)
		}
		out = append(out, ProviderKeyInfo{
			Provider:   k.Provider,
			KeyPreview: preview,
			BaseURL:    k.BaseURL,
			Protocol:   k.Protocol,
			IsActive:   k.IsActive,
			CreatedAt:  k.CreatedAt,
			UpdatedAt:  k.UpdatedAt,
		})
	}
	return out, nil
}
