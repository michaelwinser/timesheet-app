package classification

import (
	"testing"
)

// TestParse_ValueWithColon covers issue #104: the tokenizer emitted ':' as a
// standalone token everywhere, so a value containing a colon was split and the
// leftover fragment failed to parse.
func TestParse_ValueWithColon(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected string // canonical form via nodeToString
		wantErr  bool
	}{
		{
			name:     "time value keeps its colon",
			query:    "time-of-day:>17:00",
			expected: "time-of-day:>17:00",
		},
		{
			name:     "time value with less-than",
			query:    "time-of-day:<09:00",
			expected: "time-of-day:<09:00",
		},
		{
			name:     "colon value combined with another condition",
			query:    "time-of-day:>17:00 title:sync",
			expected: "time-of-day:>17:00 title:sync",
		},
		{
			name:     "colon value inside a group",
			query:    "(time-of-day:>17:00 OR time-of-day:<09:00)",
			expected: "(time-of-day:>17:00 OR time-of-day:<09:00)",
		},
		{
			name:     "negated colon value",
			query:    "-time-of-day:>17:00",
			expected: "-time-of-day:>17:00",
		},
		{
			name:     "quoted value still supported",
			query:    `time-of-day:">17:00"`,
			expected: "time-of-day:>17:00",
		},

		// Unchanged behaviour
		{
			name:     "simple condition",
			query:    "title:sync",
			expected: "title:sync",
		},
		{
			name:     "quoted phrase",
			query:    `text:"out of office"`,
			expected: `text:"out of office"`,
		},
		{
			name:     "emoji value",
			query:    `title:"🔄"`,
			expected: "title:🔄",
		},
		{
			name:     "unqualified term",
			query:    "standup",
			expected: "text:standup",
		},
		{
			name:     "OR expression",
			query:    "title:a OR title:b",
			expected: "(title:a OR title:b)",
		},
		{
			name:    "property with no value",
			query:   "title:",
			wantErr: true,
		},
		{
			name:    "unclosed quote",
			query:   `title:"sync`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := Parse(tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) = %v, want error", tt.query, nodeToString(node))
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.query, err)
			}
			if got := nodeToString(node); got != tt.expected {
				t.Errorf("Parse(%q) = %q, want %q", tt.query, got, tt.expected)
			}
		})
	}
}
