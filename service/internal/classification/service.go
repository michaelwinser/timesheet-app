package classification

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/michaelw/timesheet-app/service/internal/store"
)

// Confidence thresholds
const (
	ConfidenceFloor   = 0.5 // Below: don't classify
	ConfidenceCeiling = 0.8 // Above: auto-classify without review flag
)

// Source represents how an event was classified
type Source string

const (
	SourceManual Source = "manual"
	SourceRule   Source = "rule"
	SourceLLM    Source = "llm"
)

// Vote represents a single vote in the scoring system
type Vote struct {
	RuleID    *uuid.UUID
	ProjectID *uuid.UUID
	Attended  *bool
	Weight    float64
	Source    Source
}

// ClassificationResult represents the result of classifying an event
type ClassificationResult struct {
	ProjectID   *uuid.UUID
	Attended    *bool
	Confidence  float64
	NeedsReview bool
	Source      Source
	Votes       []Vote
}

// Service orchestrates classification of calendar events
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
	rules, err := s.ruleStore.List(ctx, userID, false)
	if err != nil {
		return nil, err
	}

	// Convert event to properties
	props := eventToProperties(event)

	// Evaluate project rules
	projectResult := s.evaluateProjectRules(rules, props)

	// For now, return project result
	// Attendance rules would be evaluated separately
	return projectResult, nil
}

// evaluateProjectRules evaluates all project-targeting rules and returns scores
func (s *Service) evaluateProjectRules(rules []*store.ClassificationRule, props *EventProperties) *ClassificationResult {
	// Map of project_id -> total score
	scores := make(map[uuid.UUID]float64)
	votes := make([]Vote, 0)

	var totalWeight float64

	for _, rule := range rules {
		// Skip attendance rules
		if rule.ProjectID == nil {
			continue
		}

		// Parse and evaluate the query
		ast, err := Parse(rule.Query)
		if err != nil {
			// Skip invalid rules
			continue
		}

		if Evaluate(ast, props) {
			scores[*rule.ProjectID] += rule.Weight
			totalWeight += rule.Weight
			votes = append(votes, Vote{
				RuleID:    &rule.ID,
				ProjectID: rule.ProjectID,
				Weight:    rule.Weight,
				Source:    SourceRule,
			})
		}
	}

	if len(scores) == 0 {
		return &ClassificationResult{
			ProjectID:   nil,
			Confidence:  0,
			NeedsReview: false,
			Source:      SourceRule,
			Votes:       votes,
		}
	}

	// Find the winner
	var winnerID uuid.UUID
	var winnerScore float64

	for projectID, score := range scores {
		if score > winnerScore {
			winnerID = projectID
			winnerScore = score
		}
	}

	// Calculate confidence
	confidence := winnerScore / totalWeight
	if confidence > 1.0 {
		confidence = 1.0
	}

	// Determine if review is needed
	needsReview := confidence >= ConfidenceFloor && confidence < ConfidenceCeiling

	// Don't classify if below floor
	if confidence < ConfidenceFloor {
		return &ClassificationResult{
			ProjectID:   nil,
			Confidence:  confidence,
			NeedsReview: false,
			Source:      SourceRule,
			Votes:       votes,
		}
	}

	return &ClassificationResult{
		ProjectID:   &winnerID,
		Confidence:  confidence,
		NeedsReview: needsReview,
		Source:      SourceRule,
		Votes:       votes,
	}
}

// EvaluateAttendance evaluates attendance rules for an event
func (s *Service) EvaluateAttendance(ctx context.Context, userID uuid.UUID, event *store.CalendarEvent) (*ClassificationResult, error) {
	// Get attendance rules
	rules, err := s.ruleStore.ListAttendanceRules(ctx, userID)
	if err != nil {
		return nil, err
	}

	props := eventToProperties(event)

	// Map of attended (true/false) -> total score
	scores := make(map[bool]float64)
	votes := make([]Vote, 0)

	var totalWeight float64

	for _, rule := range rules {
		if rule.Attended == nil {
			continue
		}

		ast, err := Parse(rule.Query)
		if err != nil {
			continue
		}

		if Evaluate(ast, props) {
			scores[*rule.Attended] += rule.Weight
			totalWeight += rule.Weight
			votes = append(votes, Vote{
				RuleID:   &rule.ID,
				Attended: rule.Attended,
				Weight:   rule.Weight,
				Source:   SourceRule,
			})
		}
	}

	if len(scores) == 0 {
		// Default: assume attended
		attended := true
		return &ClassificationResult{
			Attended:    &attended,
			Confidence:  1.0,
			NeedsReview: false,
			Source:      SourceRule,
			Votes:       votes,
		}, nil
	}

	// Find winner
	var winnerAttended bool
	var winnerScore float64

	for attended, score := range scores {
		if score > winnerScore {
			winnerAttended = attended
			winnerScore = score
		}
	}

	confidence := winnerScore / totalWeight
	if confidence > 1.0 {
		confidence = 1.0
	}

	needsReview := confidence >= ConfidenceFloor && confidence < ConfidenceCeiling

	if confidence < ConfidenceFloor {
		attended := true // Default to attended if uncertain
		return &ClassificationResult{
			Attended:    &attended,
			Confidence:  confidence,
			NeedsReview: true,
			Source:      SourceRule,
			Votes:       votes,
		}, nil
	}

	return &ClassificationResult{
		Attended:    &winnerAttended,
		Confidence:  confidence,
		NeedsReview: needsReview,
		Source:      SourceRule,
		Votes:       votes,
	}, nil
}

// PreviewRule evaluates a query against events and returns matching events with conflict info
func (s *Service) PreviewRule(ctx context.Context, userID uuid.UUID, query string, targetProjectID *uuid.UUID, startDate, endDate *time.Time) (*RulePreview, error) {
	// Parse the query first
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

	for _, event := range events {
		props := eventToProperties(event)

		if Evaluate(ast, props) {
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

// eventToProperties converts a CalendarEvent to EventProperties
func eventToProperties(event *store.CalendarEvent) *EventProperties {
	props := &EventProperties{
		Title:       event.Title,
		StartTime:   event.StartTime,
		EndTime:     event.EndTime,
		IsRecurring: event.IsRecurring,
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

	// Parse attendees from JSON
	if event.Attendees != nil {
		props.Attendees = event.Attendees
	}

	return props
}

// ApplyRules runs classification on pending events
func (s *Service) ApplyRules(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time, dryRun bool) (*ApplyResult, error) {
	// Get pending events
	pendingStatus := store.StatusPending
	events, err := s.eventStore.List(ctx, userID, startDate, endDate, &pendingStatus, nil)
	if err != nil {
		return nil, err
	}

	result := &ApplyResult{
		Classified: make([]*ClassifiedEvent, 0),
		Skipped:    0,
	}

	for _, event := range events {
		classResult, err := s.ClassifyEvent(ctx, userID, event)
		if err != nil {
			continue
		}

		if classResult.ProjectID == nil {
			result.Skipped++
			continue
		}

		classified := &ClassifiedEvent{
			EventID:     event.ID,
			ProjectID:   *classResult.ProjectID,
			Confidence:  classResult.Confidence,
			NeedsReview: classResult.NeedsReview,
		}
		result.Classified = append(result.Classified, classified)

		if !dryRun {
			// Apply the classification
			source := store.SourceRule
			_, err := s.eventStore.Classify(ctx, userID, event.ID, classResult.ProjectID, false)
			if err != nil {
				// Log error but continue
				continue
			}

			// Update confidence and needs_review
			_, err = s.pool.Exec(ctx, `
				UPDATE calendar_events
				SET classification_confidence = $3,
				    needs_review = $4,
				    classification_source = $5
				WHERE id = $1 AND user_id = $2
			`, event.ID, userID, classResult.Confidence, classResult.NeedsReview, source)
			if err != nil {
				continue
			}
		}
	}

	return result, nil
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
func MarshalVotes(votes []Vote) ([]byte, error) {
	return json.Marshal(votes)
}
