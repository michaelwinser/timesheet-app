package classification

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/michaelw/timesheet-app/service/internal/store"
	"github.com/michaelw/timesheet-app/service/internal/timeentry"
)

// ServiceVote represents a vote with UUIDs for database storage
type ServiceVote struct {
	RuleID    *uuid.UUID
	TargetID  *uuid.UUID
	Attended  *bool
	Weight    float64
	Source    MatchSource
}

// ClassificationResult represents the result of classifying an event (service layer)
type ClassificationResult struct {
	TargetID    *uuid.UUID
	Attended    *bool
	Confidence  float64
	NeedsReview bool
	Source      MatchSource
	Votes       []ServiceVote
}

// Service orchestrates classification of calendar events.
// It handles I/O and database operations, delegating pure classification
// logic to the Classify function.
type Service struct {
	pool             *pgxpool.Pool
	ruleStore        *store.ClassificationRuleStore
	eventStore       *store.CalendarEventStore
	timeEntryStore   *store.TimeEntryStore
	timeEntryService *timeentry.Service
}

// NewService creates a new classification service
func NewService(pool *pgxpool.Pool, ruleStore *store.ClassificationRuleStore, eventStore *store.CalendarEventStore, timeEntryStore *store.TimeEntryStore) *Service {
	return &Service{
		pool:             pool,
		ruleStore:        ruleStore,
		eventStore:       eventStore,
		timeEntryStore:   timeEntryStore,
		timeEntryService: timeentry.NewService(eventStore, timeEntryStore),
	}
}

// ClassifyEvent evaluates rules and targets against an event and returns the classification result.
// Targets represent classification destinations (e.g., projects) with their fingerprint attributes.
func (s *Service) ClassifyEvent(ctx context.Context, userID uuid.UUID, event *store.CalendarEvent, targets []Target) (*ClassificationResult, error) {
	// Get all enabled rules for the user
	storeRules, err := s.ruleStore.List(ctx, userID, false)
	if err != nil {
		return nil, err
	}

	// Convert to library types
	rules := storeRulesToLibraryRules(storeRules)
	item := eventToItem(event)

	// Use pure classifier with targets
	results := Classify(rules, targets, []Item{item}, DefaultConfig())
	if len(results) == 0 {
		return &ClassificationResult{
			TargetID:    nil,
			Confidence:  0,
			NeedsReview: false,
			Source:      MatchSourceRule,
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
			Source:      MatchSourceRule,
			Votes:       nil,
		}, nil
	}

	// Convert result back to service types
	return attendanceResultToServiceResult(results[0], storeRules), nil
}

// PreviewRule evaluates a query against events and returns matching events with conflict info
func (s *Service) PreviewRule(ctx context.Context, userID uuid.UUID, query string, targetProjectID *uuid.UUID, startDate, endDate *time.Time) (*RulePreview, error) {
	// Parse query first to validate syntax
	ast, err := Parse(query)
	if err != nil {
		return nil, err
	}

	// Get events in the date range
	events, err := s.eventStore.List(ctx, userID, startDate, endDate, nil, nil)
	if err != nil {
		return nil, err
	}

	preview := &RulePreview{
		Matches:   make([]*MatchedEvent, 0),
		Conflicts: make([]*Conflict, 0),
	}

	// Evaluate each event using extended properties (supports project:, client:, confidence:)
	for _, event := range events {
		extProps := eventToExtendedProperties(event)

		if !EvaluateExtended(ast, extProps) {
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

// eventToExtendedProperties converts a CalendarEvent to ExtendedEventProperties
func eventToExtendedProperties(event *store.CalendarEvent) *ExtendedEventProperties {
	props := &ExtendedEventProperties{
		EventProperties: EventProperties{
			Title:       event.Title,
			Attendees:   event.Attendees,
			StartTime:   event.StartTime,
			EndTime:     event.EndTime,
			IsRecurring: event.IsRecurring,
		},
		Confidence:   event.ClassificationConfidence,
		IsClassified: event.ClassificationStatus == store.StatusClassified,
	}

	if event.Description != nil {
		props.Description = *event.Description
	}
	if event.ResponseStatus != nil {
		props.ResponseStatus = *event.ResponseStatus
	}
	if event.Transparency != nil {
		props.Transparency = *event.Transparency
	}
	if event.CalendarName != nil {
		props.CalendarName = *event.CalendarName
	}

	if event.ProjectID != nil {
		id := event.ProjectID.String()
		props.ProjectID = &id
	}
	if event.Project != nil {
		props.ProjectName = &event.Project.Name
		props.ClientName = event.Project.Client
	}

	return props
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

// ApplyRules runs classification on pending events and re-evaluates unlocked classified events.
// Per the PRD, unlocked items (classified by rule/fingerprint, not manual) should update freely.
// Targets represent classification destinations (e.g., projects) with their fingerprint attributes.
// The caller is responsible for providing targets with appropriate attributes.
func (s *Service) ApplyRules(ctx context.Context, userID uuid.UUID, targets []Target, startDate, endDate *time.Time, dryRun bool) (*ApplyResult, error) {
	// Get pending events
	pendingStatus := store.StatusPending
	pendingEvents, err := s.eventStore.List(ctx, userID, startDate, endDate, &pendingStatus, nil)
	if err != nil {
		return nil, err
	}

	// Get events eligible for reclassification (classified by rule/fingerprint, not locked)
	reclassifyEvents, err := s.eventStore.ListForReclassification(ctx, userID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Combine both sets of events
	events := append(pendingEvents, reclassifyEvents...)

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

	// Use pure classifier with targets
	results := Classify(rules, targets, items, DefaultConfig())

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

		targetID, err := uuid.Parse(libResult.TargetID)
		if err != nil {
			applyResult.Skipped++
			continue
		}

		classified := &ClassifiedEvent{
			EventID:     event.ID,
			TargetID:    targetID,
			Confidence:  libResult.Confidence,
			NeedsReview: libResult.NeedsReview,
		}
		applyResult.Classified = append(applyResult.Classified, classified)

		if !dryRun {
			// Map MatchSource to store.ClassificationSource
			source := store.SourceRule
			if libResult.MatchSource == MatchSourceFingerprint {
				source = store.SourceFingerprint
			}

			_, err := s.eventStore.Classify(ctx, userID, event.ID, &targetID, false)
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

	// Recalculate time entries for all affected dates
	if !dryRun && len(applyResult.Classified) > 0 {
		affectedDates := make(map[time.Time]bool)
		for _, c := range applyResult.Classified {
			event := eventMap[c.EventID.String()]
			if event != nil {
				eventDate := time.Date(event.StartTime.Year(), event.StartTime.Month(), event.StartTime.Day(), 0, 0, 0, 0, time.UTC)
				affectedDates[eventDate] = true
			}
		}

		for date := range affectedDates {
			// Recalculate time entries for this date using the analyzer
			if err := s.timeEntryService.RecalculateForDate(ctx, userID, date); err != nil {
				// Log but don't fail - classification succeeded
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
	TargetID    uuid.UUID `json:"target_id"`
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
	attrs := make(map[string]any)

	attrs["title"] = event.Title
	attrs["start_time"] = event.StartTime
	attrs["end_time"] = event.EndTime
	attrs["is_recurring"] = event.IsRecurring

	if event.Description != nil {
		attrs["description"] = *event.Description
	}

	if event.ResponseStatus != nil {
		attrs["response_status"] = *event.ResponseStatus
	}

	if event.Transparency != nil {
		attrs["transparency"] = *event.Transparency
	}

	if event.Attendees != nil {
		attrs["attendees"] = event.Attendees
	}

	if event.CalendarName != nil {
		attrs["calendar_name"] = *event.CalendarName
	}

	return Item{
		ID:         event.ID.String(),
		Attributes: attrs,
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
		vote := ServiceVote{
			Weight: v.Weight,
			Source: v.Source,
		}
		// For user-defined rules, include the rule ID
		if storeRule := ruleMap[v.RuleID]; storeRule != nil {
			vote.RuleID = &storeRule.ID
			vote.TargetID = storeRule.ProjectID
		} else {
			// For fingerprint-generated rules, parse target ID
			if id, err := uuid.Parse(v.TargetID); err == nil {
				vote.TargetID = &id
			}
		}
		votes = append(votes, vote)
	}

	// Parse target ID
	var targetID *uuid.UUID
	if result.TargetID != "" {
		if id, err := uuid.Parse(result.TargetID); err == nil {
			targetID = &id
		}
	}

	return &ClassificationResult{
		TargetID:    targetID,
		Confidence:  result.Confidence,
		NeedsReview: result.NeedsReview,
		Source:      result.MatchSource,
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
				Source:   v.Source,
			})
		}
	}

	attended := result.Attended
	return &ClassificationResult{
		Attended:    &attended,
		Confidence:  result.Confidence,
		NeedsReview: result.NeedsReview,
		Source:      MatchSourceRule,
		Votes:       votes,
	}
}

// RecalculateTimeEntries recalculates time entries for a specific date.
// This should be called after event classification changes.
func (s *Service) RecalculateTimeEntries(ctx context.Context, userID uuid.UUID, date time.Time) error {
	return s.timeEntryService.RecalculateForDate(ctx, userID, date)
}

// RecalculateTimeEntriesForEvent recalculates the time entry affected by a specific event.
func (s *Service) RecalculateTimeEntriesForEvent(ctx context.Context, userID uuid.UUID, event *store.CalendarEvent) error {
	return s.timeEntryService.RecalculateForEvent(ctx, userID, event)
}

// ExplainEventClassification evaluates all rules against an event and returns
// detailed information showing which rules matched and how scores were calculated.
// This is useful for debugging classification decisions.
func (s *Service) ExplainEventClassification(ctx context.Context, userID uuid.UUID, eventID uuid.UUID, targets []Target) (*ExplainResult, error) {
	// Get the event
	event, err := s.eventStore.GetByID(ctx, userID, eventID)
	if err != nil {
		return nil, err
	}

	// Get all enabled rules for the user
	storeRules, err := s.ruleStore.List(ctx, userID, false)
	if err != nil {
		return nil, err
	}

	// Convert to library types
	rules := storeRulesToLibraryRules(storeRules)
	item := eventToItem(event)

	// Build a map for rule names (queries) by ID
	ruleQueries := make(map[string]string)
	for _, r := range storeRules {
		ruleQueries[r.ID.String()] = r.Query
	}

	// Use pure classifier explain function
	result := ExplainClassification(rules, targets, item, DefaultConfig())

	return result, nil
}
