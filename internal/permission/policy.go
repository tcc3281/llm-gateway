package permission

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// PolicyDocument represents an AWS IAM-style policy document.
type PolicyDocument struct {
	Version   string      `json:"Version"`
	Statement []Statement `json:"Statement"`
}

// Statement is an individual permission rule.
type Statement struct {
	Sid      string   `json:"Sid,omitempty"`
	Effect   string   `json:"Effect"` // "Allow" or "Deny"
	Action   []string `json:"Action"`
	Resource []string `json:"Resource"`
}

// ParsePolicy parses and validates raw JSON into a PolicyDocument.
func ParsePolicy(data []byte) (*PolicyDocument, error) {
	var doc PolicyDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("invalid policy JSON: %w", err)
	}

	if doc.Version == "" {
		doc.Version = "1"
	}

	for i, stmt := range doc.Statement {
		effect := strings.ToLower(strings.TrimSpace(stmt.Effect))
		if effect != "allow" && effect != "deny" {
			return nil, fmt.Errorf("statement[%d] invalid effect %q (must be 'Allow' or 'Deny')", i, stmt.Effect)
		}
		if len(stmt.Action) == 0 {
			return nil, fmt.Errorf("statement[%d] must specify at least one Action", i)
		}
		if len(stmt.Resource) == 0 {
			return nil, fmt.Errorf("statement[%d] must specify at least one Resource", i)
		}
	}

	return &doc, nil
}

// MatchPattern checks whether target matches pattern with wildcard support.
// Supports:
//   - "*" matches anything
//   - "model:*" matches "model:gpt-4o"
//   - "model:claude-*" matches "model:claude-3-5-sonnet"
//   - exact match (case-insensitive)
func MatchPattern(pattern, target string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	target = strings.ToLower(strings.TrimSpace(target))

	if pattern == "*" || pattern == target {
		return true
	}

	// Action wildcards like "*:*"
	if pattern == "*:*" {
		return true
	}

	// path.Match style wildcard matching (handles * and ?)
	// Replace colons temporarily if needed or match directly
	if strings.Contains(pattern, "*") {
		// If pattern ends with "*", check prefix
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(target, prefix) {
				return true
			}
		}
		// Try standard filepath / pattern match
		if matched, err := path.Match(pattern, target); err == nil && matched {
			return true
		}
	}

	return false
}

// ActionMatches returns true if action matches any of the statement action patterns.
func ActionMatches(patterns []string, action string) bool {
	for _, p := range patterns {
		if MatchPattern(p, action) {
			return true
		}
	}
	return false
}

// ResourceMatches returns true if resource matches any of the statement resource patterns.
func ResourceMatches(patterns []string, resource string) bool {
	for _, p := range patterns {
		if MatchPattern(p, resource) {
			return true
		}
	}
	return false
}

// Evaluate evaluates a PolicyDocument against an action and resource.
// Returns (matched bool, isDeny bool).
func (p *PolicyDocument) Evaluate(action, resource string) (matched bool, isDeny bool) {
	for _, stmt := range p.Statement {
		if ActionMatches(stmt.Action, action) && ResourceMatches(stmt.Resource, resource) {
			isDeny := strings.EqualFold(stmt.Effect, "deny")
			return true, isDeny
		}
	}
	return false, false
}
