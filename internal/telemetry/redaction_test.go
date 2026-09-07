package telemetry

import (
	"strings"
	"testing"
)

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean string remains unchanged",
			input:    "Hello world, this is a harmless prompt.",
			expected: "Hello world, this is a harmless prompt.",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "masks email address",
			input:    "Contact us at support@example.com or dev.lead@sub.domain.org for info.",
			expected: "Contact us at [REDACTED_EMAIL] or [REDACTED_EMAIL] for info.",
		},
		{
			name:     "masks credit card numbers",
			input:    "Card: 4532 1234 5678 9012 or 4532-1234-5678-9012.",
			expected: "Card: [REDACTED_CARD] or [REDACTED_CARD].",
		},
		{
			name:     "masks openai and bearer api keys",
			input:    "My key is sk-abcdef1234567890abcdef and token is Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.",
			expected: "My key is [REDACTED_KEY] and token is [REDACTED_KEY].",
		},
		{
			name:     "masks phone numbers",
			input:    "Call me at +1 555-123-4567 or (555) 987-6543 today.",
			expected: "Call me at [REDACTED_PHONE] or [REDACTED_PHONE] today.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeText(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeText(%q) = %q, want %q", tt.input, got, tt.expected)
			}
			if strings.Contains(got, "support@example.com") {
				t.Errorf("Unredacted email leaked in output: %s", got)
			}
		})
	}
}
