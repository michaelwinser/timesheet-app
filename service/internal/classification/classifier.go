package classification

import (
	"strings"
	"time"
)

// Confidence thresholds for classification decisions
const (
	ConfidenceFloor   = 0.5 // Below: don't classify
	ConfidenceCeiling = 0.8 // Above: auto-classify without review flag
)

// TargetDNA is a special target ID for "did not attend" rules
const TargetDNA = "DNA"

// Rule represents a classification rule (pure, no DB dependencies)
type Rule struct {
	ID       string  // Rule identifier
	Query    string  // Gmail-style query string
	TargetID string  // Project ID or TargetDNA for attendance rules
	Weight   float64 // Rule weight for scoring
}

// Item represents an item to be classified (pure, no DB dependencies)
type Item struct {
	ID         string         // Item identifier
	Properties map[string]any // Properties for matching (title, attendees, etc.)
}

// Vote represents a single vote from a rule that matched
type Vote struct {
	RuleID   string
	TargetID string
	Weight   float64
}

// Result represents the classification result for a single item
type Result struct {
	ItemID      string
	TargetID    string  // Winner target ID, empty if no classification
	Confidence  float64
	NeedsReview bool
	Votes       []Vote
}

// Config holds configuration for the classifier
type Config struct {
	ConfidenceFloor   float64
	ConfidenceCeiling float64
}

// DefaultConfig returns the default classifier configuration
func DefaultConfig() Config {
	return Config{
		ConfidenceFloor:   ConfidenceFloor,
		ConfidenceCeiling: ConfidenceCeiling,
	}
}

// Classify evaluates rules against items and returns classification results.
// This is a pure function with no side effects or I/O.
func Classify(rules []Rule, items []Item, config Config) []Result {
	results := make([]Result, 0, len(items))

	for _, item := range items {
		result := classifyItem(rules, item, config)
		results = append(results, result)
	}

	return results
}

// classifyItem evaluates all rules against a single item
func classifyItem(rules []Rule, item Item, config Config) Result {
	// Convert item properties to EventProperties for evaluation
	props := itemToProperties(item)

	// Collect votes from matching rules
	scores := make(map[string]float64)
	votes := make([]Vote, 0)
	var totalWeight float64

	for _, rule := range rules {
		ast, err := Parse(rule.Query)
		if err != nil {
			// Skip invalid rules
			continue
		}

		if Evaluate(ast, props) {
			scores[rule.TargetID] += rule.Weight
			totalWeight += rule.Weight
			votes = append(votes, Vote{
				RuleID:   rule.ID,
				TargetID: rule.TargetID,
				Weight:   rule.Weight,
			})
		}
	}

	// No matching rules
	if len(scores) == 0 {
		return Result{
			ItemID:      item.ID,
			TargetID:    "",
			Confidence:  0,
			NeedsReview: false,
			Votes:       votes,
		}
	}

	// Find the winner
	var winnerID string
	var winnerScore float64

	for targetID, score := range scores {
		if score > winnerScore {
			winnerID = targetID
			winnerScore = score
		}
	}

	// Calculate confidence
	confidence := winnerScore / totalWeight
	if confidence > 1.0 {
		confidence = 1.0
	}

	// Determine if review is needed based on thresholds
	needsReview := confidence >= config.ConfidenceFloor && confidence < config.ConfidenceCeiling

	// Don't classify if below floor
	if confidence < config.ConfidenceFloor {
		return Result{
			ItemID:      item.ID,
			TargetID:    "",
			Confidence:  confidence,
			NeedsReview: false,
			Votes:       votes,
		}
	}

	return Result{
		ItemID:      item.ID,
		TargetID:    winnerID,
		Confidence:  confidence,
		NeedsReview: needsReview,
		Votes:       votes,
	}
}

// itemToProperties converts a generic Item to EventProperties for evaluation
func itemToProperties(item Item) *EventProperties {
	props := &EventProperties{}

	if v, ok := item.Properties["title"].(string); ok {
		props.Title = v
	}

	if v, ok := item.Properties["description"].(string); ok {
		props.Description = v
	}

	if v, ok := item.Properties["attendees"].([]string); ok {
		props.Attendees = v
	}

	if v, ok := item.Properties["start_time"].(time.Time); ok {
		props.StartTime = v
	}

	if v, ok := item.Properties["end_time"].(time.Time); ok {
		props.EndTime = v
	}

	if v, ok := item.Properties["response_status"].(string); ok {
		props.ResponseStatus = v
	}

	if v, ok := item.Properties["transparency"].(string); ok {
		props.Transparency = v
	}

	if v, ok := item.Properties["is_recurring"].(bool); ok {
		props.IsRecurring = v
	}

	return props
}

// ClassifyAttendance evaluates attendance rules separately from project rules.
// Returns whether the item was attended (true) or not (false).
func ClassifyAttendance(rules []Rule, items []Item, config Config) []AttendanceResult {
	results := make([]AttendanceResult, 0, len(items))

	for _, item := range items {
		result := classifyItemAttendance(rules, item, config)
		results = append(results, result)
	}

	return results
}

// AttendanceResult represents the attendance classification for an item
type AttendanceResult struct {
	ItemID      string
	Attended    bool
	Confidence  float64
	NeedsReview bool
	Votes       []Vote
}

// classifyItemAttendance evaluates attendance rules for a single item
func classifyItemAttendance(rules []Rule, item Item, config Config) AttendanceResult {
	props := itemToProperties(item)

	// Collect votes: true = attended, false = did not attend
	attendedScore := 0.0
	didNotAttendScore := 0.0
	votes := make([]Vote, 0)
	var totalWeight float64

	for _, rule := range rules {
		// Only process DNA rules for attendance
		if rule.TargetID != TargetDNA && !strings.HasPrefix(rule.TargetID, "attended:") {
			continue
		}

		ast, err := Parse(rule.Query)
		if err != nil {
			continue
		}

		if Evaluate(ast, props) {
			if rule.TargetID == TargetDNA {
				didNotAttendScore += rule.Weight
			} else {
				attendedScore += rule.Weight
			}
			totalWeight += rule.Weight
			votes = append(votes, Vote{
				RuleID:   rule.ID,
				TargetID: rule.TargetID,
				Weight:   rule.Weight,
			})
		}
	}

	// No matching rules - default to attended
	if len(votes) == 0 {
		return AttendanceResult{
			ItemID:      item.ID,
			Attended:    true,
			Confidence:  1.0,
			NeedsReview: false,
			Votes:       votes,
		}
	}

	// Determine winner
	attended := attendedScore >= didNotAttendScore
	winnerScore := attendedScore
	if !attended {
		winnerScore = didNotAttendScore
	}

	confidence := winnerScore / totalWeight
	if confidence > 1.0 {
		confidence = 1.0
	}

	needsReview := confidence >= config.ConfidenceFloor && confidence < config.ConfidenceCeiling

	// If below floor, default to attended with review flag
	if confidence < config.ConfidenceFloor {
		return AttendanceResult{
			ItemID:      item.ID,
			Attended:    true,
			Confidence:  confidence,
			NeedsReview: true,
			Votes:       votes,
		}
	}

	return AttendanceResult{
		ItemID:      item.ID,
		Attended:    attended,
		Confidence:  confidence,
		NeedsReview: needsReview,
		Votes:       votes,
	}
}

// PreviewRules evaluates a query against items without applying any changes.
// Useful for rule testing before creation.
func PreviewRules(query string, items []Item) ([]string, error) {
	ast, err := Parse(query)
	if err != nil {
		return nil, err
	}

	var matchingIDs []string
	for _, item := range items {
		props := itemToProperties(item)
		if Evaluate(ast, props) {
			matchingIDs = append(matchingIDs, item.ID)
		}
	}

	return matchingIDs, nil
}
