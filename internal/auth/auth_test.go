package auth

import (
	"testing"
	"time"

	"llm-gateway/internal/model"

	"github.com/google/uuid"
)

func TestValidateAdminKey(t *testing.T) {
	service := NewAuthService(nil, nil, "super-secret-admin-key")

	if !service.ValidateAdminKey("super-secret-admin-key") {
		t.Errorf("expected ValidateAdminKey to return true for matching key")
	}

	if service.ValidateAdminKey("wrong-key") {
		t.Errorf("expected ValidateAdminKey to return false for wrong key")
	}

	if service.ValidateAdminKey("") {
		t.Errorf("expected ValidateAdminKey to return false for empty key")
	}

	emptyService := NewAuthService(nil, nil, "")
	if emptyService.ValidateAdminKey("any-key") {
		t.Errorf("expected ValidateAdminKey to return false when adminKey is unconfigured")
	}
}

func TestPasswordHashingAndVerification(t *testing.T) {
	service := NewAuthService(nil, nil, "master-key")

	password := "CorrectHorseBatteryStaple123!"
	hash1, err := service.HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error hashing password: %v", err)
	}

	hash2, err := service.HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error hashing password: %v", err)
	}

	// Salts should make different hashes for the same password
	if hash1 == hash2 {
		t.Errorf("expected different hashes due to unique random salt, got same")
	}

	// Verification of correct password
	if !service.VerifyPassword(password, hash1) {
		t.Errorf("expected VerifyPassword to return true for correct password")
	}
	if !service.VerifyPassword(password, hash2) {
		t.Errorf("expected VerifyPassword to return true for correct password against hash2")
	}

	// Verification of wrong password
	if service.VerifyPassword("WrongPassword!", hash1) {
		t.Errorf("expected VerifyPassword to return false for incorrect password")
	}

	// Edge cases
	if service.VerifyPassword("", hash1) {
		t.Errorf("expected VerifyPassword to return false for empty password")
	}
	if service.VerifyPassword(password, "") {
		t.Errorf("expected VerifyPassword to return false for empty hash")
	}
	if service.VerifyPassword(password, "malformed-hash-string") {
		t.Errorf("expected VerifyPassword to return false for malformed hash")
	}
}

func TestSessionTokenLifecycle(t *testing.T) {
	service := NewAuthService(nil, nil, "signing-key-12345")
	userID := uuid.New()

	user := &model.User{
		ID:     userID,
		Email:  "member@enterprise.internal",
		Name:   "Alice Johnson",
		Role:   "admin",
		IsRoot: true,
	}

	// 1. Generate token
	token, err := service.CreateSessionToken(user, 1*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error creating session token: %v", err)
	}

	// 2. Verify token
	claims, err := service.VerifySessionToken(token)
	if err != nil {
		t.Fatalf("unexpected error verifying session token: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("expected user ID %s, got %s", userID, claims.UserID)
	}
	if claims.Email != "member@enterprise.internal" {
		t.Errorf("expected email member@enterprise.internal, got %s", claims.Email)
	}
	if claims.Name != "Alice Johnson" {
		t.Errorf("expected name Alice Johnson, got %s", claims.Name)
	}
	if claims.Role != "admin" {
		t.Errorf("expected role admin, got %s", claims.Role)
	}
	if !claims.IsRoot {
		t.Errorf("expected is_root to be true")
	}

	// 3. Expired token check
	expiredToken, err := service.CreateSessionToken(user, -1*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error creating expired token: %v", err)
	}
	_, err = service.VerifySessionToken(expiredToken)
	if err == nil {
		t.Errorf("expected error for expired session token, got nil")
	}

	// 4. Tampered token check
	tamperedToken := token[:len(token)-4] + "ffff"
	_, err = service.VerifySessionToken(tamperedToken)
	if err == nil {
		t.Errorf("expected error for tampered token signature, got nil")
	}
}
