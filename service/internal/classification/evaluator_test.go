package classification

import (
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple words",
			input:    "hello world",
			expected: []string{"hello", "world"},
		},
		{
			name:     "with punctuation",
			input:    "Jack / Michael - Xander",
			expected: []string{"Jack", "Michael", "Xander"},
		},
		{
			name:     "with numbers",
			input:    "AC 123 flight",
			expected: []string{"AC", "123", "flight"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "only punctuation",
			input:    "---///",
			expected: nil,
		},
		{
			name:     "mixed case preserved",
			input:    "Hello WORLD",
			expected: []string{"Hello", "WORLD"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenize(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("tokenize(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("tokenize(%q)[%d] = %q, want %q", tt.input, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestContainsWordIgnoreCase(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		word     string
		expected bool
	}{
		// Basic positive cases
		{
			name:     "exact match",
			s:        "AC",
			word:     "AC",
			expected: true,
		},
		{
			name:     "word at start",
			s:        "AC 123",
			word:     "AC",
			expected: true,
		},
		{
			name:     "word at end",
			s:        "flight AC",
			word:     "AC",
			expected: true,
		},
		{
			name:     "word in middle",
			s:        "book AC flight",
			word:     "AC",
			expected: true,
		},
		{
			name:     "case insensitive match",
			s:        "Book ac Flight",
			word:     "AC",
			expected: true,
		},

		// The key test case from issue #44
		{
			name:     "AC should NOT match Jack",
			s:        "Jack / Michael - Xander Immigration discussion",
			word:     "AC",
			expected: false,
		},

		// More negative cases
		{
			name:     "substring should not match",
			s:        "Jackson",
			word:     "Jack",
			expected: false,
		},
		{
			name:     "prefix should not match",
			s:        "Jacks house",
			word:     "Jack",
			expected: false,
		},
		{
			name:     "suffix should not match",
			s:        "hijack",
			word:     "Jack",
			expected: false,
		},
		{
			name:     "embedded should not match",
			s:        "blackjack game",
			word:     "Jack",
			expected: false,
		},

		// Edge cases
		{
			name:     "empty string",
			s:        "",
			word:     "AC",
			expected: false,
		},
		{
			name:     "empty word",
			s:        "hello world",
			word:     "",
			expected: false,
		},
		{
			name:     "word with numbers",
			s:        "Flight AC123 departing",
			word:     "AC123",
			expected: true,
		},
		{
			name:     "partial number match should fail",
			s:        "AC1234",
			word:     "AC123",
			expected: false,
		},

		// Multi-word phrases should use substring matching
		{
			name:     "multi-word phrase matches",
			s:        "Out of Office - John",
			word:     "out of office",
			expected: true,
		},
		{
			name:     "multi-word phrase case insensitive",
			s:        "Weekly Team Meeting",
			word:     "team meeting",
			expected: true,
		},
		{
			name:     "multi-word phrase no match",
			s:        "Weekly meeting with team",
			word:     "team meeting",
			expected: false, // "team" and "meeting" are there but not as "team meeting"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsWordIgnoreCase(tt.s, tt.word)
			if result != tt.expected {
				t.Errorf("containsWordIgnoreCase(%q, %q) = %v, want %v", tt.s, tt.word, result, tt.expected)
			}
		})
	}
}

func TestEvaluate_TitleWordBoundary(t *testing.T) {
	// Test that title matching uses word boundaries
	props := &EventProperties{
		Title: "Jack / Michael - Xander Immigration discussion",
	}

	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{
			name:     "AC should not match Jack",
			query:    "title:AC",
			expected: false,
		},
		{
			name:     "Jack should match as whole word",
			query:    "title:Jack",
			expected: true,
		},
		{
			name:     "Michael should match",
			query:    "title:Michael",
			expected: true,
		},
		{
			name:     "Immigration should match",
			query:    "title:Immigration",
			expected: true,
		},
		{
			name:     "Imm should not match Immigration",
			query:    "title:Imm",
			expected: false,
		},
		{
			name:     "discussion should match case insensitive",
			query:    "title:DISCUSSION",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ast, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.query, err)
			}
			result := Evaluate(ast, props)
			if result != tt.expected {
				t.Errorf("Evaluate(%q) = %v, want %v", tt.query, result, tt.expected)
			}
		})
	}
}
