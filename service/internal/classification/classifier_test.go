package classification

import (
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
			Properties: map[string]any{
				"title": "Daily Standup",
			},
		},
		{
			ID: "event-2",
			Properties: map[string]any{
				"title": "Lunch meeting",
			},
		},
	}

	results := Classify(rules, items, DefaultConfig())

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
			Properties: map[string]any{
				"title":     "Meeting with Acme",
				"attendees": []string{"bob@acme.com"},
			},
		},
	}

	results := Classify(rules, items, DefaultConfig())

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
			Properties: map[string]any{
				"title": "Weekly Sync",
			},
		},
	}

	results := Classify(rules, items, DefaultConfig())

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

	// Should need review (confidence < ceiling)
	if !result.NeedsReview {
		t.Error("expected NeedsReview to be true")
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
			Properties: map[string]any{
				"title": "Weekly Sync",
			},
		},
	}

	results := Classify(rules, items, config)

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
			Properties: map[string]any{
				"title": "Daily Standup",
			},
		},
	}

	results := Classify(rules, items, config)

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
			Properties: map[string]any{
				"title":           "Meeting I declined",
				"response_status": "declined",
			},
		},
		{
			ID: "event-2",
			Properties: map[string]any{
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
			Properties: map[string]any{
				"title": "Daily Standup",
			},
		},
		{
			ID: "event-2",
			Properties: map[string]any{
				"title": "Lunch meeting",
			},
		},
		{
			ID: "event-3",
			Properties: map[string]any{
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
			Properties: map[string]any{
				"title": "Test",
			},
		},
	}

	_, err := PreviewRules("invalid query without colon", items)
	if err == nil {
		t.Error("expected error for invalid query")
	}
}

func TestItemToProperties_AllFields(t *testing.T) {
	now := time.Now()
	item := Item{
		ID: "test-event",
		Properties: map[string]any{
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
