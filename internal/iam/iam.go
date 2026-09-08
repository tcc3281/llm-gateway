package iam

import (
	"context"

	"llm-gateway/internal/auth"
	"llm-gateway/internal/db"
	"llm-gateway/internal/model"
	"llm-gateway/internal/permission"

	"github.com/google/uuid"
)

// Service encapsulates the Identity, Authentication, and Access Management subsystems of the Control Plane.
type Service struct {
	Auth       *auth.AuthService
	Permission *permission.PermissionEngine
	pg         *db.PostgresDB
	redis      *db.RedisClient
}

// NewService creates a unified IAM Service.
func NewService(pg *db.PostgresDB, redis *db.RedisClient, adminKey string) *Service {
	return &Service{
		Auth:       auth.NewAuthService(pg, redis, adminKey),
		Permission: permission.NewPermissionEngine(pg),
		pg:         pg,
		redis:      redis,
	}
}

// ValidateKey validates an incoming API key and returns the key model if active.
func (s *Service) ValidateKey(ctx context.Context, plainKey string) (*model.APIKey, error) {
	return s.Auth.ValidateAPIKey(ctx, plainKey)
}

// CheckModelAccess evaluates whether an API key has access to a specific model pattern.
func (s *Service) CheckModelAccess(ctx context.Context, keyID uuid.UUID, model string) (bool, error) {
	return s.Permission.CheckKeyModelAccess(ctx, keyID, model)
}

// CheckUserPermission checks if a user has permission to perform an action on a target.
func (s *Service) CheckUserPermission(ctx context.Context, userID uuid.UUID, action string, targetResource string) (bool, error) {
	return s.Permission.CheckUserPermission(ctx, userID, action, targetResource)
}
