package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"llm-gateway/internal/adapter/canonical"
	"llm-gateway/internal/model"

	"github.com/google/uuid"
)

type mockPreAuthPlugin struct {
	called bool
}

func (m *mockPreAuthPlugin) Name() string { return "mock-pre-auth" }
func (m *mockPreAuthPlugin) ExecPreAuth(pctx *PluginContext, cfg map[string]any) error {
	m.called = true
	return nil
}

type mockStopPlugin struct{}

func (m *mockStopPlugin) Name() string { return "mock-stop" }
func (m *mockStopPlugin) ExecPreRouting(pctx *PluginContext, cfg map[string]any) error {
	pctx.Stop = true
	pctx.StopStatusCode = 403
	pctx.StopErrorBody = []byte("forbidden by mock")
	return nil
}

func TestResolveEffectiveBindings_ScopeSpecificity(t *testing.T) {
	keyID := uuid.New()
	userID := uuid.New()
	tenantID := uuid.New()

	apiKey := &model.APIKey{ID: keyID, UserID: userID, TenantID: &tenantID}
	user := &model.User{ID: userID, Role: "admin"}

	allBindings := []model.PluginBinding{
		{
			PluginName: "rate-limiting",
			ScopeLevel: "global",
			IsActive:   true,
			Priority:   100,
			Config:     json.RawMessage(`{"rpm": 10}`),
		},
		{
			PluginName:    "rate-limiting",
			ScopeLevel:    "tenant",
			ScopeTenantID: &tenantID,
			IsActive:      true,
			Priority:      100,
			Config:        json.RawMessage(`{"rpm": 50}`),
		},
		{
			PluginName: "rate-limiting",
			ScopeLevel: "api_key",
			ScopeKeyID: &keyID,
			IsActive:   true,
			Priority:   100,
			Config:     json.RawMessage(`{"rpm": 200}`),
		},
		{
			PluginName: "pii-masking",
			ScopeLevel: "global",
			IsActive:   true,
			Priority:   10,
			Config:     json.RawMessage(`{"replacement": "[REDACTED]"}`),
		},
	}

	effective := ResolveEffectiveBindings(allBindings, apiKey, user, &tenantID, "openai", "gpt-4o")

	if len(effective) != 2 {
		t.Fatalf("expected 2 effective bindings, got %d", len(effective))
	}

	// Should be sorted by Priority ASC: pii-masking (10) then rate-limiting (100)
	if effective[0].PluginName != "pii-masking" {
		t.Errorf("expected first binding to be pii-masking, got %s", effective[0].PluginName)
	}
	if effective[1].PluginName != "rate-limiting" {
		t.Errorf("expected second binding to be rate-limiting, got %s", effective[1].PluginName)
	}

	// For rate-limiting, api_key scope must win over tenant and global
	if effective[1].ScopeLevel != "api_key" {
		t.Errorf("expected rate-limiting scope to be api_key, got %s", effective[1].ScopeLevel)
	}
}

func TestPluginEngine_RunPipelineAndShortCircuit(t *testing.T) {
	reg := NewRegistry()
	preAuth := &mockPreAuthPlugin{}
	stopPlugin := &mockStopPlugin{}

	reg.Register("mock-pre-auth", preAuth)
	reg.Register("mock-stop", stopPlugin)

	engine := NewPluginEngine(nil, reg)

	pctx := &PluginContext{
		Ctx:     context.Background(),
		CanReq:  &canonical.CanonicalRequest{Model: "gpt-4o"},
		ExtraHeaders: make(map[string]string),
	}

	bindings := []model.PluginBinding{
		{PluginName: "mock-pre-auth", ScopeLevel: "global", IsActive: true},
		{PluginName: "mock-stop", ScopeLevel: "global", IsActive: true},
	}

	if err := engine.RunPreAuth(pctx, bindings); err != nil {
		t.Fatalf("unexpected error in RunPreAuth: %v", err)
	}
	if !preAuth.called {
		t.Errorf("expected mockPreAuthPlugin to be called")
	}

	if err := engine.RunPreRouting(pctx, bindings); err != nil {
		t.Fatalf("unexpected error in RunPreRouting: %v", err)
	}
	if !pctx.Stop {
		t.Errorf("expected pctx.Stop to be true")
	}
	if pctx.StopStatusCode != 403 {
		t.Errorf("expected status code 403, got %d", pctx.StopStatusCode)
	}
}
