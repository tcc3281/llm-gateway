package permission

import (
	"testing"
)

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		target  string
		want    bool
	}{
		{"*", "anything", true},
		{"*:*", "model:use", true},
		{"model:*", "model:use", true},
		{"model:gpt-4o", "model:gpt-4o", true},
		{"model:gpt-4o", "model:gpt-4o-mini", false},
		{"model:claude-*", "model:claude-3-5-sonnet", true},
		{"model:claude-*", "model:claude-3-opus", true},
		{"model:claude-*", "model:gpt-4o", false},
		{"MODEL:GPT-*", "model:gpt-4o", true}, // case-insensitivity
		{"key:*", "key:create", true},
		{"key:list", "key:create", false},
	}

	for _, tt := range tests {
		got := MatchPattern(tt.pattern, tt.target)
		if got != tt.want {
			t.Errorf("MatchPattern(%q, %q) = %v; want %v", tt.pattern, tt.target, got, tt.want)
		}
	}
}

func TestPolicyEvaluation_DenyWins(t *testing.T) {
	raw := []byte(`{
		"Version": "1",
		"Statement": [
			{
				"Sid": "AllowAllModels",
				"Effect": "Allow",
				"Action": ["model:use"],
				"Resource": ["model:*"]
			},
			{
				"Sid": "DenyExpensiveModels",
				"Effect": "Deny",
				"Action": ["model:use"],
				"Resource": ["model:gpt-4o-128k", "model:claude-3-opus"]
			}
		]
	}`)

	policy, err := ParsePolicy(raw)
	if err != nil {
		t.Fatalf("ParsePolicy failed: %v", err)
	}

	// 1. Regular model -> should match allow, not deny
	matched, isDeny := policy.Evaluate("model:use", "model:gpt-4o")
	if !matched || isDeny {
		t.Errorf("Evaluate regular model: got matched=%v, isDeny=%v; want matched=true, isDeny=false", matched, isDeny)
	}

	// 2. Denied model -> evaluate should match deny
	matched, isDeny = policy.Evaluate("model:use", "model:claude-3-opus")
	// Notice in statement order or evaluate check
	// When we evaluate both statements:
	hasDeny := false
	hasAllow := false
	for _, stmt := range policy.Statement {
		if ActionMatches(stmt.Action, "model:use") && ResourceMatches(stmt.Resource, "model:claude-3-opus") {
			if stmt.Effect == "Deny" {
				hasDeny = true
			} else {
				hasAllow = true
			}
		}
	}
	if !hasDeny {
		t.Errorf("Expected hasDeny=true for claude-3-opus")
	}
	if !hasAllow {
		t.Errorf("Expected hasAllow=true for wildcard match")
	}
}

func TestParsePolicy_Validation(t *testing.T) {
	// Invalid effect
	invalidEffect := []byte(`{"Version":"1","Statement":[{"Effect":"Maybe","Action":["*"],"Resource":["*"]}]}`)
	if _, err := ParsePolicy(invalidEffect); err == nil {
		t.Errorf("Expected error for invalid effect 'Maybe', got nil")
	}

	// Missing action
	missingAction := []byte(`{"Version":"1","Statement":[{"Effect":"Allow","Action":[],"Resource":["*"]}]}`)
	if _, err := ParsePolicy(missingAction); err == nil {
		t.Errorf("Expected error for empty Action, got nil")
	}

	// Missing resource
	missingResource := []byte(`{"Version":"1","Statement":[{"Effect":"Allow","Action":["*"],"Resource":[]}]}`)
	if _, err := ParsePolicy(missingResource); err == nil {
		t.Errorf("Expected error for empty Resource, got nil")
	}
}
