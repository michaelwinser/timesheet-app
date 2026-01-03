package classification

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/michaelw/timesheet-app/service/internal/store"
)

// Source represents how an event was classified
type Source string

const (
	SourceManual Source = "manual"
	SourceRule   Source = "rule"
	SourceLLM    Source = "llm"
)

// ServiceVote represents a vote with UUIDs for database storage
type ServiceVote struct {
	RuleID    *uuid.UUID
	ProjectID *uuid.UUID
	Attended  *bool
	Weight    float64
	Source    Source
}

// ClassificationResult represents the result of classifying an event (service layer)
type ClassificationResult struct {
	ProjectID   *uuid.UUID
	Attended    *bool
	Confidence  float64
	NeedsReview bool
	Source      Source
	Votes       []ServiceVote
}

// Service orchestrates classification of calendar events.
// It handles I/O and database operations, delegating pure classification
// logic to the Classify function.
type Service struct {
	pool       *pgxpool.Pool
	ruleStore  *store.ClassificationRuleStore
	eventStore *store.CalendarEventStore
}

// NewService creates a new classification service
func NewService(pool *pgxpool.Pool, ruleStore *store.ClassificationRuleStore, eventStore *store.CalendarEventStore) *Service {
	return &Service{
		pool:       pool,
		ruleStore:  ruleStore,
		eventStore: eventStore,
	}
}

// ClassifyEvent evaluates all rules against an event and returns the classification result
func (s *Service) ClassifyEvent(ctx context.Context, userID uuid.UUID, event *store.CalendarEvent) (*ClassificationResult, error) {
	// Get all enabled rules for the user
	storeRules, err := s.ruleStore.List(ctx, userID, false)
	if err != nil {
		return nil, err
	}

	// Convert to library types
	rules := storeRulesToLibraryRules(storeRules)
	item := eventToItem(event)

	// Use pure classifier
	results := Classify(rules, []Item{item}, DefaultConfig())
	if len(results) == 0 {
		return &ClassificationResult{
			ProjectID:   nil,
			Confidence:  0,
			NeedsReview: false,
			Source:      SourceRule,
			Votes:       nil,
		}, nil
	}

	// Convert result back to service types
	return libraryResultToServiceResult(results[0], storeRules), nil
}

// EvaluateAttendance evaluates attendance rules for an event
func (s *Service) EvaluateAttendance(ctx context.Context, userID uuid.UUID, event *store.CalendarEvent) (*ClassificationResult, error) {
	// Get attendance rules
	storeRules, err := s.ruleStore.ListAttendanceRules(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Convert to library types
	rules := storeRulesToAttendanceRules(storeRules)
	item := eventToItem(event)

	// Use pure classifier for attendance
	results := ClassifyAttendance(rules, []Item{item}, DefaultConfig())
	if len(results) == 0 {
		attended := true
		return &ClassificationResult{
			Attended:    &attended,
			Confidence:  1.0,
			NeedsReview: false,
			Source:      SourceRule,
			Votes:       nil,
		}, nil
	}

	// Convert result back to service types
	return attendanceResultToServiceResult(results[0], storeRules), nil
}

// PreviewRule evaluates a query against events and returns matching events with conflict info
func (s *Service) PreviewRule(ctx context.Context, userID uuid.UUID, query string, targetProjectID *uuid.UUID, startDate, endDate *time.Time) (*RulePreview, error) {
	// Get events in the date range
	events, err := s.eventStore.List(ctx, userID, startDate, endDate, nil, nil)
	if err != nil {
		return nil, err
	}

	// Convert events to items
	items := make([]Item, 0, len(events))
	eventMap := make(map[string]*store.CalendarEvent)
	for _, event := range events {
		item := eventToItem(event)
		items = append(items, item)
		eventMap[item.ID] = event
	}

	// Use pure library to find matches
	matchingIDs, err := PreviewRules(query, items)
	if err != nil {
		return nil, err
	}

	preview := &RulePreview{
		Matches:   make([]*MatchedEvent, 0),
		Conflicts: make([]*Conflict, 0),
	}

	for _, id := range matchingIDs {
		event := eventMap[id]
		if event == nil {
			continue
		}

		matched := &MatchedEvent{
			EventID:   event.ID,
			Title:     event.Title,
			StartTime: event.StartTime,
		}
		preview.Matches = append(preview.Matches, matched)

		// Check for conflicts
		if event.ProjectID != nil && targetProjectID != nil && *event.ProjectID != *targetProjectID {
			var currentSource string
			if event.ClassificationSource != nil {
				currentSource = string(*event.ClassificationSource)
			}

			preview.Conflicts = append(preview.Conflicts, &Conflict{
				EventID:          event.ID,
				CurrentProjectID: event.ProjectID,
				CurrentSource:    currentSource,
				ProposedProject:  targetProjectID,
			})

			if currentSource == "manual" {
				preview.Stats.ManualConflicts++
			}
		}
	}

	preview.Stats.TotalMatches = len(preview.Matches)
	preview.Stats.WouldChange = len(preview.Conflicts)
	preview.Stats.AlreadyCorrect = preview.Stats.TotalMatches - preview.Stats.WouldChange

	return preview, nil
}

// RulePreview contains the preview results for a rule
type RulePreview struct {
	Matches   []*MatchedEvent `json:"matches"`
	Conflicts []*Conflict     `json:"conflicts"`
	Stats     PreviewStats    `json:"stats"`
}

// MatchedEvent represents an event that matches a rule
type MatchedEvent struct {
	EventID   uuid.UUID `json:"event_id"`
	Title     string    `json:"title"`
	StartTime time.Time `json:"start_time"`
}

// Conflict represents a classification conflict
type Conflict struct {
	EventID          uuid.UUID  `json:"event_id"`
	CurrentProjectID *uuid.UUID `json:"current_project_id"`
	CurrentSource    string     `json:"current_source"`
	ProposedProject  *uuid.UUID `json:"proposed_project_id"`
}

// PreviewStats contains summary statistics for a preview
type PreviewStats struct {
	TotalMatches    int `json:"total_matches"`
	AlreadyCorrect  int `json:"already_correct"`
	WouldChange     int `json:"would_change"`
	ManualConflicts int `json:"manual_conflicts"`
}

// ApplyRules runs classification on pending events
func (s *Service) ApplyRules(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time, dryRun bool) (*ApplyResult, error) {
	// Get pending events
	pendingStatus := store.StatusPending
	events, err := s.eventStore.List(ctx, userID, startDate, endDate, &pendingStatus, nil)
	if err != nil {
		return nil, err
	}

	// Get all enabled rules
	storeRules, err := s.ruleStore.List(ctx, userID, false)
	if err != nil {
		return nil, err
	}

	// Convert rules to library types
	rules := storeRulesToLibraryRules(storeRules)

	// Convert events to items
	items := make([]Item, 0, len(events))
	eventMap := make(map[string]*store.CalendarEvent)
	for _, event := range events {
		item := eventToItem(event)
		items = append(items, item)
		eventMap[item.ID] = event
	}

	// Use pure classifier
	results := Classify(rules, items, DefaultConfig())

	applyResult := &ApplyResult{
		Classified: make([]*ClassifiedEvent, 0),
		Skipped:    0,
	}

	for _, libResult := range results {
		event := eventMap[libResult.ItemID]
		if event == nil {
			continue
		}

		if libResult.TargetID == "" {
			applyResult.Skipped++
			continue
		}

		projectID, err := uuid.Parse(libResult.TargetID)
		if err != nil {
			applyResult.Skipped++
			continue
		}

		classified := &ClassifiedEvent{
			EventID:     event.ID,
			ProjectID:   projectID,
			Confidence:  libResult.Confidence,
			NeedsReview: libResult.NeedsReview,
		}
		applyResult.Classified = append(applyResult.Classified, classified)

		if !dryRun {
			// Apply the classification
			source := store.SourceRule
			_, err := s.eventStore.Classify(ctx, userID, event.ID, &projectID, false)
			if err != nil {
				continue
			}

			// Update confidence and needs_review
			_, err = s.pool.Exec(ctx, `
				UPDATE calendar_events
				SET classification_confidence = $3,
				    needs_review = $4,
				    classification_source = $5
				WHERE id = $1 AND user_id = $2
			`, event.ID, userID, libResult.Confidence, libResult.NeedsReview, source)
			if err != nil {
				continue
			}
		}
	}

	return applyResult, nil
}

// ApplyResult contains the results of applying rules
type ApplyResult struct {
	Classified []*ClassifiedEvent `json:"classified"`
	Skipped    int                `json:"skipped"`
}

// ClassifiedEvent represents an event that was classified
type ClassifiedEvent struct {
	EventID     uuid.UUID `json:"event_id"`
	ProjectID   uuid.UUID `json:"project_id"`
	Confidence  float64   `json:"confidence"`
	NeedsReview bool      `json:"needs_review"`
}

// MarshalVotes converts votes to JSON for storage/debugging
func MarshalVotes(votes []ServiceVote) ([]byte, error) {
	return json.Marshal(votes)
}

// --- Conversion functions between store types and library types ---

// storeRulesToLibraryRules converts store rules to pure library rules (project classification)
func storeRulesToLibraryRules(storeRules []*store.ClassificationRule) []Rule {
	rules := make([]Rule, 0, len(storeRules))
	for _, sr := range storeRules {
		// Skip attendance rules for project classification
		if sr.ProjectID == nil {
			continue
		}
		rules = append(rules, Rule{
			ID:       sr.ID.String(),
			Query:    sr.Query,
			TargetID: sr.ProjectID.String(),
			Weight:   sr.Weight,
		})
	}
	return rules
}

// storeRulesToAttendanceRules converts store rules to pure library rules (attendance)
func storeRulesToAttendanceRules(storeRules []*store.ClassificationRule) []Rule {
	rules := make([]Rule, 0, len(storeRules))
	for _, sr := range storeRules {
		// Only process attendance rules
		if sr.Attended == nil {
			continue
		}
		targetID := "attended:true"
		if !*sr.Attended {
			targetID = TargetDNA
		}
		rules = append(rules, Rule{
			ID:       sr.ID.String(),
			Query:    sr.Query,
			TargetID: targetID,
			Weight:   sr.Weight,
		})
	}
	return rules
}

// eventToItem converts a CalendarEvent to a library Item
func eventToItem(event *store.CalendarEvent) Item {
	props := make(map[string]any)

	props["title"] = event.Title
	props["start_time"] = event.StartTime
	props["end_time"] = event.EndTime
	props["is_recurring"] = event.IsRecurring

	if event.Description != nil {
		props["description"] = *event.Description
	}

	if event.ResponseStatus != nil {
		props["response_status"] = *event.ResponseStatus
	}

	if event.Transparency != nil {
		props["transparency"] = *event.Transparency
	}

	if event.Attendees != nil {
		props["attendees"] = event.Attendees
	}

	return Item{
		ID:         event.ID.String(),
		Properties: props,
	}
}

// libraryResultToServiceResult converts a library Result to a ServiceResult
func libraryResultToServiceResult(result Result, storeRules []*store.ClassificationRule) *ClassificationResult {
	// Build a map of rule IDs to store rules for vote conversion
	ruleMap := make(map[string]*store.ClassificationRule)
	for _, r := range storeRules {
		ruleMap[r.ID.String()] = r
	}

	// Convert votes
	votes := make([]ServiceVote, 0, len(result.Votes))
	for _, v := range result.Votes {
		storeRule := ruleMap[v.RuleID]
		if storeRule != nil {
			votes = append(votes, ServiceVote{
				RuleID:    &storeRule.ID,
				ProjectID: storeRule.ProjectID,
				Weight:    v.Weight,
				Source:    SourceRule,
			})
		}
	}

	// Parse project ID
	var projectID *uuid.UUID
	if result.TargetID != "" {
		if id, err := uuid.Parse(result.TargetID); err == nil {
			projectID = &id
		}
	}

	return &ClassificationResult{
		ProjectID:   projectID,
		Confidence:  result.Confidence,
		NeedsReview: result.NeedsReview,
		Source:      SourceRule,
		Votes:       votes,
	}
}

// attendanceResultToServiceResult converts an AttendanceResult to a ServiceResult
func attendanceResultToServiceResult(result AttendanceResult, storeRules []*store.ClassificationRule) *ClassificationResult {
	// Build a map of rule IDs to store rules
	ruleMap := make(map[string]*store.ClassificationRule)
	for _, r := range storeRules {
		ruleMap[r.ID.String()] = r
	}

	// Convert votes
	votes := make([]ServiceVote, 0, len(result.Votes))
	for _, v := range result.Votes {
		storeRule := ruleMap[v.RuleID]
		if storeRule != nil {
			votes = append(votes, ServiceVote{
				RuleID:   &storeRule.ID,
				Attended: storeRule.Attended,
				Weight:   v.Weight,
				Source:   SourceRule,
			})
		}
	}

	attended := result.Attended
	return &ClassificationResult{
		Attended:    &attended,
		Confidence:  result.Confidence,
		NeedsReview: result.NeedsReview,
		Source:      SourceRule,
		Votes:       votes,
	}
}
