package classification

import (
	"fmt"
	"testing"
	"time"
)

func TestClassify_SingleRule(t *testing.T) {
	rules := []Rule{
		{
			ID:       "rule-1",
			Query:    "title:standup",
			TargetID: "project-a",
			Weight:   1.0,
		},
	}

	items := []Item{
		{
			ID: "event-1",
			Attributes: map[string]any{
				"title": "Daily Standup",
			},
		},
		{
			ID: "event-2",
			Attributes: map[string]any{
				"title": "Lunch meeting",
			},
		},
	}

	results := Classify(rules, nil, items, DefaultConfig())

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First event should match
	if results[0].ItemID != "event-1" {
		t.Errorf("expected ItemID event-1, got %s", results[0].ItemID)
	}
	if results[0].TargetID != "project-a" {
		t.Errorf("expected TargetID project-a, got %s", results[0].TargetID)
	}
	if results[0].Confidence != 1.0 {
		t.Errorf("expected Confidence 1.0, got %f", results[0].Confidence)
	}

	// Second event should not match
	if results[1].ItemID != "event-2" {
		t.Errorf("expected ItemID event-2, got %s", results[1].ItemID)
	}
	if results[1].TargetID != "" {
		t.Errorf("expected empty TargetID, got %s", results[1].TargetID)
	}
}

func TestClassify_MultipleRulesScoring(t *testing.T) {
	rules := []Rule{
		{
			ID:       "rule-1",
			Query:    "title:meeting",
			TargetID: "project-a",
			Weight:   1.0,
		},
		{
			ID:       "rule-2",
			Query:    "domain:acme.com",
			TargetID: "project-a",
			Weight:   2.0,
		},
		{
			ID:       "rule-3",
			Query:    "title:internal",
			TargetID: "project-b",
			Weight:   1.0,
		},
	}

	items := []Item{
		{
			ID: "event-1",
			Attributes: map[string]any{
				"title":     "Meeting with Acme",
				"attendees": []string{"bob@acme.com"},
			},
		},
	}

	results := Classify(rules, nil, items, DefaultConfig())

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]

	// Project A should win with score 3.0 (rule-1 + rule-2)
	if result.TargetID != "project-a" {
		t.Errorf("expected TargetID project-a, got %s", result.TargetID)
	}

	// Confidence should be 3/3 = 1.0 (all matching votes for winner)
	if result.Confidence != 1.0 {
		t.Errorf("expected Confidence 1.0, got %f", result.Confidence)
	}

	// Should have 2 votes
	if len(result.Votes) != 2 {
		t.Errorf("expected 2 votes, got %d", len(result.Votes))
	}
}

func TestClassify_ConflictingRules(t *testing.T) {
	rules := []Rule{
		{
			ID:       "rule-1",
			Query:    "title:sync",
			TargetID: "project-a",
			Weight:   1.0,
		},
		{
			ID:       "rule-2",
			Query:    "title:sync",
			TargetID: "project-b",
			Weight:   2.0,
		},
	}

	items := []Item{
		{
			ID: "event-1",
			Attributes: map[string]any{
				"title": "Weekly Sync",
			},
		},
	}

	results := Classify(rules, nil, items, DefaultConfig())

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]

	// Project B should win with higher weight
	if result.TargetID != "project-b" {
		t.Errorf("expected TargetID project-b, got %s", result.TargetID)
	}

	// Confidence: 2/(1+2) = 0.666...
	expectedConfidence := 2.0 / 3.0
	if result.Confidence < expectedConfidence-0.01 || result.Confidence > expectedConfidence+0.01 {
		t.Errorf("expected Confidence ~%f, got %f", expectedConfidence, result.Confidence)
	}

	// Should NOT need review (confidence 66% >= ceiling 65%)
	if result.NeedsReview {
		t.Error("expected NeedsReview to be false")
	}
}

func TestClassify_BelowConfidenceFloor(t *testing.T) {
	config := Config{
		ConfidenceFloor:   0.5,
		ConfidenceCeiling: 0.8,
	}

	rules := []Rule{
		{
			ID:       "rule-1",
			Query:    "title:sync",
			TargetID: "project-a",
			Weight:   1.0,
		},
		{
			ID:       "rule-2",
			Query:    "title:sync",
			TargetID: "project-b",
			Weight:   1.0,
		},
		{
			ID:       "rule-3",
			Query:    "title:sync",
			TargetID: "project-c",
			Weight:   1.0,
		},
	}

	items := []Item{
		{
			ID: "event-1",
			Attributes: map[string]any{
				"title": "Weekly Sync",
			},
		},
	}

	results := Classify(rules, nil, items, config)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]

	// 3-way tie means confidence = 1/3 ≈ 0.33, below floor
	if result.TargetID != "" {
		t.Errorf("expected empty TargetID when below floor, got %s", result.TargetID)
	}

	if result.NeedsReview {
		t.Error("expected NeedsReview to be false when below floor")
	}
}

func TestClassify_AboveConfidenceCeiling(t *testing.T) {
	config := Config{
		ConfidenceFloor:   0.5,
		ConfidenceCeiling: 0.8,
	}

	rules := []Rule{
		{
			ID:       "rule-1",
			Query:    "title:standup",
			TargetID: "project-a",
			Weight:   1.0,
		},
	}

	items := []Item{
		{
			ID: "event-1",
			Attributes: map[string]any{
				"title": "Daily Standup",
			},
		},
	}

	results := Classify(rules, nil, items, config)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]

	// Single matching rule = 100% confidence
	if result.Confidence != 1.0 {
		t.Errorf("expected Confidence 1.0, got %f", result.Confidence)
	}

	// Should NOT need review (confidence > ceiling)
	if result.NeedsReview {
		t.Error("expected NeedsReview to be false when above ceiling")
	}
}

func TestClassifyAttendance_DNA(t *testing.T) {
	rules := []Rule{
		{
			ID:       "rule-1",
			Query:    "response:declined",
			TargetID: TargetDNA,
			Weight:   1.0,
		},
	}

	items := []Item{
		{
			ID: "event-1",
			Attributes: map[string]any{
				"title":           "Meeting I declined",
				"response_status": "declined",
			},
		},
		{
			ID: "event-2",
			Attributes: map[string]any{
				"title":           "Meeting I accepted",
				"response_status": "accepted",
			},
		},
	}

	results := ClassifyAttendance(rules, items, DefaultConfig())

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First event: declined, should be marked as not attended
	if results[0].Attended {
		t.Error("expected first event to be not attended")
	}

	// Second event: no matching DNA rule, defaults to attended
	if !results[1].Attended {
		t.Error("expected second event to be attended by default")
	}
}

func TestPreviewRules(t *testing.T) {
	items := []Item{
		{
			ID: "event-1",
			Attributes: map[string]any{
				"title": "Daily Standup",
			},
		},
		{
			ID: "event-2",
			Attributes: map[string]any{
				"title": "Lunch meeting",
			},
		},
		{
			ID: "event-3",
			Attributes: map[string]any{
				"title": "Team Standup",
			},
		},
	}

	matchingIDs, err := PreviewRules("title:standup", items)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(matchingIDs) != 2 {
		t.Errorf("expected 2 matches, got %d", len(matchingIDs))
	}

	// Should match event-1 and event-3
	hasEvent1 := false
	hasEvent3 := false
	for _, id := range matchingIDs {
		if id == "event-1" {
			hasEvent1 = true
		}
		if id == "event-3" {
			hasEvent3 = true
		}
	}

	if !hasEvent1 || !hasEvent3 {
		t.Errorf("expected event-1 and event-3 to match, got %v", matchingIDs)
	}
}

func TestPreviewRules_InvalidQuery(t *testing.T) {
	items := []Item{
		{
			ID: "event-1",
			Attributes: map[string]any{
				"title": "Test",
			},
		},
	}

	// Unclosed quote is invalid
	_, err := PreviewRules(`title:"unclosed`, items)
	if err == nil {
		t.Error("expected error for invalid query")
	}
}

func TestItemToAttributes_AllFields(t *testing.T) {
	now := time.Now()
	item := Item{
		ID: "test-event",
		Attributes: map[string]any{
			"title":           "Test Meeting",
			"description":     "A test description",
			"attendees":       []string{"alice@example.com", "bob@example.com"},
			"start_time":      now,
			"end_time":        now.Add(time.Hour),
			"response_status": "accepted",
			"transparency":    "opaque",
			"is_recurring":    true,
		},
	}

	props := itemToProperties(item)

	if props.Title != "Test Meeting" {
		t.Errorf("expected Title 'Test Meeting', got %s", props.Title)
	}
	if props.Description != "A test description" {
		t.Errorf("expected Description 'A test description', got %s", props.Description)
	}
	if len(props.Attendees) != 2 {
		t.Errorf("expected 2 attendees, got %d", len(props.Attendees))
	}
	if !props.StartTime.Equal(now) {
		t.Errorf("expected StartTime %v, got %v", now, props.StartTime)
	}
	if props.ResponseStatus != "accepted" {
		t.Errorf("expected ResponseStatus 'accepted', got %s", props.ResponseStatus)
	}
	if props.Transparency != "opaque" {
		t.Errorf("expected Transparency 'opaque', got %s", props.Transparency)
	}
	if !props.IsRecurring {
		t.Error("expected IsRecurring to be true")
	}
}

func TestClassify_MultiWordKeywordFingerprint(t *testing.T) {
	// Test that multi-word keywords in target attributes are properly quoted
	// and matched (issue #19)
	targets := []Target{
		{
			ID: "project-a",
			Attributes: map[string]any{
				"keywords": []string{"out of office", "team meeting"},
			},
		},
	}

	items := []Item{
		{
			ID: "event-1",
			Attributes: map[string]any{
				"title": "Out of Office - John",
			},
		},
		{
			ID: "event-2",
			Attributes: map[string]any{
				"title": "Weekly Team Meeting",
			},
		},
		{
			ID: "event-3",
			Attributes: map[string]any{
				"title": "Lunch",
			},
		},
	}

	results := Classify(nil, targets, items, DefaultConfig())

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// First event should match "out of office"
	if results[0].TargetID != "project-a" {
		t.Errorf("event-1: expected TargetID project-a, got %s", results[0].TargetID)
	}

	// Second event should match "team meeting"
	if results[1].TargetID != "project-a" {
		t.Errorf("event-2: expected TargetID project-a, got %s", results[1].TargetID)
	}

	// Third event should not match
	if results[2].TargetID != "" {
		t.Errorf("event-3: expected empty TargetID, got %s", results[2].TargetID)
	}
}

func TestQuoteIfNeeded(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"standup", "standup"},
		{"out of office", `"out of office"`},
		{"team meeting", `"team meeting"`},
		{"single", "single"},
		{"with  multiple   spaces", `"with  multiple   spaces"`},
	}

	for _, tt := range tests {
		result := quoteIfNeeded(tt.input)
		if result != tt.expected {
			t.Errorf("quoteIfNeeded(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestEvaluateExtended_ProjectFilter(t *testing.T) {
	projectName := "Acme Project"
	clientName := "Acme Corp"
	confidence := 0.9

	props := &ExtendedEventProperties{
		EventProperties: EventProperties{
			Title: "Team Standup",
		},
		ProjectName:  &projectName,
		ClientName:   &clientName,
		Confidence:   &confidence,
		IsClassified: true,
	}

	tests := []struct {
		query    string
		expected bool
	}{
		{"project:acme", true},
		{"project:unknown", false},
		{"client:acme", true},
		{"client:unknown", false},
		{"confidence:high", true},
		{"confidence:medium", false},
		{"confidence:low", false},
		{"project:acme client:acme", true},
		{"project:acme confidence:high", true},
	}

	for _, tt := range tests {
		ast, err := Parse(tt.query)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.query, err)
			continue
		}
		result := EvaluateExtended(ast, props)
		if result != tt.expected {
			t.Errorf("EvaluateExtended(%q) = %v, expected %v", tt.query, result, tt.expected)
		}
	}
}

func TestEvaluateExtended_Unclassified(t *testing.T) {
	// Test unclassified event
	unclassified := &ExtendedEventProperties{
		EventProperties: EventProperties{
			Title: "Unclassified Meeting",
		},
		ProjectID:    nil,
		IsClassified: false,
	}

	ast, _ := Parse("project:unclassified")
	if !EvaluateExtended(ast, unclassified) {
		t.Error("Expected unclassified event to match project:unclassified")
	}

	// Test classified event
	projectID := "123"
	projectName := "Test Project"
	classified := &ExtendedEventProperties{
		EventProperties: EventProperties{
			Title: "Classified Meeting",
		},
		ProjectID:    &projectID,
		ProjectName:  &projectName,
		IsClassified: true,
	}

	if EvaluateExtended(ast, classified) {
		t.Error("Expected classified event NOT to match project:unclassified")
	}
}

func TestEvaluateExtended_ConfidenceLevels(t *testing.T) {
	tests := []struct {
		confidence *float64
		query      string
		expected   bool
	}{
		{ptr(0.9), "confidence:high", true},
		{ptr(0.65), "confidence:high", true},  // at ceiling
		{ptr(0.64), "confidence:high", false}, // just below ceiling
		{ptr(0.64), "confidence:medium", true},
		{ptr(0.5), "confidence:medium", true}, // at floor
		{ptr(0.49), "confidence:medium", false},
		{ptr(0.49), "confidence:low", true},
		{ptr(0.0), "confidence:low", true},
		{nil, "confidence:low", true}, // nil confidence = low
		{nil, "confidence:high", false},
	}

	for _, tt := range tests {
		props := &ExtendedEventProperties{
			EventProperties: EventProperties{Title: "Test"},
			Confidence:      tt.confidence,
		}
		ast, _ := Parse(tt.query)
		result := EvaluateExtended(ast, props)
		confStr := "nil"
		if tt.confidence != nil {
			confStr = fmt.Sprintf("%.2f", *tt.confidence)
		}
		if result != tt.expected {
			t.Errorf("EvaluateExtended(%q) with confidence=%s = %v, expected %v", tt.query, confStr, result, tt.expected)
		}
	}
}

func ptr(f float64) *float64 {
	return &f
}

func TestEvaluate_CalendarName(t *testing.T) {
	props := &EventProperties{
		Title:        "Weekly Standup",
		CalendarName: "Work Calendar",
	}

	tests := []struct {
		query    string
		expected bool
	}{
		{"calendar:work", true},
		{`calendar:"Work Calendar"`, true},
		{"calendar:personal", false},
		{"-calendar:personal", true},
		{"-calendar:work", false},
	}

	for _, tt := range tests {
		ast, err := Parse(tt.query)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.query, err)
			continue
		}
		result := Evaluate(ast, props)
		if result != tt.expected {
			t.Errorf("Evaluate(%q) = %v, expected %v", tt.query, result, tt.expected)
		}
	}
}

func TestEvaluate_TextSearch(t *testing.T) {
	props := &EventProperties{
		Title:       "Weekly Team Standup",
		Description: "Discuss project progress and blockers",
		Attendees:   []string{"alice@example.com", "bob@acme.com"},
	}

	tests := []struct {
		query    string
		expected bool
	}{
		// Explicit text: prefix searches title, description, and calendar name
		// Note: attendees excluded from text: - use domain: or email: instead
		// See: https://github.com/michaelwinser/timesheet-app/issues/84
		{"text:standup", true},       // matches title
		{"text:progress", true},      // matches description
		{"text:alice", false},        // attendees not searched by text:
		{"text:acme", false},         // attendees not searched by text:
		{"text:unknown", false},      // no match
		{"-text:unknown", true},      // negated, no match = true
		{"-text:standup", false},     // negated, match = false
		// Unqualified terms implicitly use text search
		{"standup", true},
		{"progress", true},
		{"alice", false},             // attendees not searched
		{"unknown", false},
		{"-unknown", true},
		{"-standup", false},
		// Combined with other conditions
		{"standup title:weekly", true},
		{"standup calendar:work", false}, // calendar is empty
		{"standup OR progress", true},
		{"standup unknown", false}, // AND: both must match
	}

	for _, tt := range tests {
		ast, err := Parse(tt.query)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.query, err)
			continue
		}
		result := Evaluate(ast, props)
		if result != tt.expected {
			t.Errorf("Evaluate(%q) = %v, expected %v", tt.query, result, tt.expected)
		}
	}
}

func TestEvaluateExtended_Calendar(t *testing.T) {
	props := &ExtendedEventProperties{
		EventProperties: EventProperties{
			Title:        "Daily Standup",
			CalendarName: "Engineering",
		},
		IsClassified: false,
	}

	tests := []struct {
		query    string
		expected bool
	}{
		{"calendar:engineering", true},
		{"calendar:Engineering", true},
		{"calendar:personal", false},
		{"calendar:engineering title:standup", true},
		{"calendar:personal OR title:standup", true},
	}

	for _, tt := range tests {
		ast, err := Parse(tt.query)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.query, err)
			continue
		}
		result := EvaluateExtended(ast, props)
		if result != tt.expected {
			t.Errorf("EvaluateExtended(%q) = %v, expected %v", tt.query, result, tt.expected)
		}
	}
}
