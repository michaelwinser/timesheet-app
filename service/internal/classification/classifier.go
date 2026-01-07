package classification

import (
	"fmt"
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

// MatchSource indicates where a classification match originated
type MatchSource string

const (
	MatchSourceRule        MatchSource = "rule"        // User-defined explicit rule
	MatchSourceFingerprint MatchSource = "fingerprint" // Target attribute match
)

// Rule represents a classification rule (pure, no DB dependencies)
type Rule struct {
	ID       string  // Rule identifier
	Query    string  // Gmail-style query string
	TargetID string  // Target ID or TargetDNA for attendance rules
	Weight   float64 // Rule weight for scoring
}

// Target represents a classification target (e.g., project, category)
// Attributes are the "fingerprints" used for matching items to this target.
// Convention for attribute names:
//   - "domains": list of domain strings to match against item attendee emails
//   - "emails": list of email addresses to match against item attendees
//   - "keywords": list of keywords to match against item title/description
//   - "_description", "_notes": context for future LLM use (ignored by rule-based matching)
type Target struct {
	ID         string         // Target identifier
	Attributes map[string]any // Attributes for matching (domains, emails, keywords, etc.)
}

// Item represents an item to be classified (pure, no DB dependencies)
type Item struct {
	ID         string         // Item identifier
	Attributes map[string]any // Attributes for matching (title, attendees, etc.)
}

// Vote represents a single vote from a rule that matched
type Vote struct {
	RuleID   string
	TargetID string
	Weight   float64
	Source   MatchSource // Where this vote came from
}

// Result represents the classification result for a single item
type Result struct {
	ItemID       string
	TargetID     string      // Winner target ID, empty if no classification
	Confidence   float64
	NeedsReview  bool
	MatchSource  MatchSource // Primary source of the winning classification
	Votes        []Vote
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

// Classify evaluates rules and target attributes against items and returns classification results.
// This is a pure function with no side effects or I/O.
//
// Classification uses two sources:
//  1. Explicit rules - user-defined query patterns
//  2. Target attributes - "fingerprints" that generate implicit matching rules
//
// Target attributes are converted to rules using conventions:
//   - "domains" → domain:X queries
//   - "emails" → email:X queries
//   - "keywords" → title:X queries
func Classify(rules []Rule, targets []Target, items []Item, config Config) []Result {
	// Generate rules from target attributes
	allRules, fingerprintRuleIDs := generateTargetRules(targets)
	allRules = append(allRules, rules...)

	results := make([]Result, 0, len(items))

	for _, item := range items {
		result := classifyItem(allRules, fingerprintRuleIDs, item, config)
		results = append(results, result)
	}

	return results
}

// generateTargetRules creates classification rules from target attributes.
// Returns the generated rules and a set of rule IDs that are fingerprint-based.
func generateTargetRules(targets []Target) ([]Rule, map[string]bool) {
	var rules []Rule
	fingerprintRuleIDs := make(map[string]bool)

	for _, target := range targets {
		// Generate rules for domain attributes
		if domains, ok := getStringSlice(target.Attributes, "domains"); ok {
			for _, domain := range domains {
				ruleID := fmt.Sprintf("fp:domain:%s:%s", target.ID, domain)
				rules = append(rules, Rule{
					ID:       ruleID,
					Query:    "domain:" + domain,
					TargetID: target.ID,
					Weight:   1.0,
				})
				fingerprintRuleIDs[ruleID] = true
			}
		}

		// Generate rules for email attributes
		if emails, ok := getStringSlice(target.Attributes, "emails"); ok {
			for _, email := range emails {
				ruleID := fmt.Sprintf("fp:email:%s:%s", target.ID, email)
				rules = append(rules, Rule{
					ID:       ruleID,
					Query:    "email:" + email,
					TargetID: target.ID,
					Weight:   1.0,
				})
				fingerprintRuleIDs[ruleID] = true
			}
		}

		// Generate rules for keyword attributes
		if keywords, ok := getStringSlice(target.Attributes, "keywords"); ok {
			for _, keyword := range keywords {
				ruleID := fmt.Sprintf("fp:keyword:%s:%s", target.ID, keyword)
				rules = append(rules, Rule{
					ID:       ruleID,
					Query:    "title:" + quoteIfNeeded(keyword),
					TargetID: target.ID,
					Weight:   1.0,
				})
				fingerprintRuleIDs[ruleID] = true
			}
		}
	}

	return rules, fingerprintRuleIDs
}

// getStringSlice extracts a string slice from a map value
func getStringSlice(m map[string]any, key string) ([]string, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	if slice, ok := v.([]string); ok {
		return slice, len(slice) > 0
	}
	return nil, false
}

// quoteIfNeeded wraps a value in quotes if it contains spaces
func quoteIfNeeded(value string) string {
	if strings.Contains(value, " ") {
		return `"` + value + `"`
	}
	return value
}

// classifyItem evaluates all rules against a single item
func classifyItem(rules []Rule, fingerprintRuleIDs map[string]bool, item Item, config Config) Result {
	// Convert item attributes to EventProperties for evaluation
	props := itemToProperties(item)

	// Collect votes from matching rules
	scores := make(map[string]float64)
	votes := make([]Vote, 0)
	var totalWeight float64

	// Track fingerprint vs rule weight per target for source determination
	fingerprintWeight := make(map[string]float64)
	ruleWeight := make(map[string]float64)

	for _, rule := range rules {
		ast, err := Parse(rule.Query)
		if err != nil {
			// Skip invalid rules
			continue
		}

		if Evaluate(ast, props) {
			scores[rule.TargetID] += rule.Weight
			totalWeight += rule.Weight

			source := MatchSourceRule
			if fingerprintRuleIDs[rule.ID] {
				source = MatchSourceFingerprint
				fingerprintWeight[rule.TargetID] += rule.Weight
			} else {
				ruleWeight[rule.TargetID] += rule.Weight
			}

			votes = append(votes, Vote{
				RuleID:   rule.ID,
				TargetID: rule.TargetID,
				Weight:   rule.Weight,
				Source:   source,
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

	// Determine primary match source for the winner
	matchSource := MatchSourceRule
	if fingerprintWeight[winnerID] > ruleWeight[winnerID] {
		matchSource = MatchSourceFingerprint
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
		MatchSource: matchSource,
		Votes:       votes,
	}
}

// itemToProperties converts a generic Item to EventProperties for evaluation
func itemToProperties(item Item) *EventProperties {
	props := &EventProperties{}

	if v, ok := item.Attributes["title"].(string); ok {
		props.Title = v
	}

	if v, ok := item.Attributes["description"].(string); ok {
		props.Description = v
	}

	if v, ok := item.Attributes["attendees"].([]string); ok {
		props.Attendees = v
	}

	if v, ok := item.Attributes["start_time"].(time.Time); ok {
		props.StartTime = v
	}

	if v, ok := item.Attributes["end_time"].(time.Time); ok {
		props.EndTime = v
	}

	if v, ok := item.Attributes["response_status"].(string); ok {
		props.ResponseStatus = v
	}

	if v, ok := item.Attributes["transparency"].(string); ok {
		props.Transparency = v
	}

	if v, ok := item.Attributes["is_recurring"].(bool); ok {
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

// RuleEvaluation represents the result of evaluating a single rule against an item
type RuleEvaluation struct {
	RuleID     string
	Query      string
	TargetID   string
	TargetName string      // Populated by caller
	Weight     float64
	Source     MatchSource // rule or fingerprint
	Matched    bool
}

// TargetScore represents the accumulated score for a classification target
type TargetScore struct {
	TargetID         string
	TargetName       string  // Populated by caller
	TotalWeight      float64
	RuleWeight       float64 // Weight from explicit rules
	FingerprintWeight float64 // Weight from fingerprint matches
	IsWinner         bool
}

// ExplainResult contains detailed information about how an item was classified
type ExplainResult struct {
	ItemID           string
	Evaluations      []RuleEvaluation // All rules evaluated (matched and unmatched)
	TargetScores     []TargetScore    // Scores by target
	TotalWeight      float64          // Sum of all matching rule weights
	WinnerTargetID   string
	WinnerConfidence float64
	NeedsReview      bool
	MatchSource      MatchSource // Primary source of winning classification
	Outcome          string      // Human-readable outcome description
}

// ExplainClassification evaluates all rules against a single item and returns
// detailed information about the classification decision. Unlike Classify, this
// shows ALL rules including those that didn't match.
func ExplainClassification(rules []Rule, targets []Target, item Item, config Config) *ExplainResult {
	// Generate rules from target attributes
	allRules, fingerprintRuleIDs := generateTargetRules(targets)
	allRules = append(allRules, rules...)

	// Build target name map
	targetNames := make(map[string]string)
	for _, t := range targets {
		if name, ok := t.Attributes["name"].(string); ok {
			targetNames[t.ID] = name
		}
	}

	// Convert item attributes to EventProperties for evaluation
	props := itemToProperties(item)

	// Evaluate all rules
	evaluations := make([]RuleEvaluation, 0, len(allRules))
	scores := make(map[string]float64)
	var totalWeight float64

	// Track fingerprint vs rule weight per target
	fingerprintWeight := make(map[string]float64)
	ruleWeight := make(map[string]float64)

	for _, rule := range allRules {
		ast, err := Parse(rule.Query)
		if err != nil {
			// Include invalid rules as non-matching
			evaluations = append(evaluations, RuleEvaluation{
				RuleID:     rule.ID,
				Query:      rule.Query,
				TargetID:   rule.TargetID,
				TargetName: targetNames[rule.TargetID],
				Weight:     rule.Weight,
				Source:     MatchSourceRule,
				Matched:    false,
			})
			continue
		}

		matched := Evaluate(ast, props)
		source := MatchSourceRule
		if fingerprintRuleIDs[rule.ID] {
			source = MatchSourceFingerprint
		}

		evaluations = append(evaluations, RuleEvaluation{
			RuleID:     rule.ID,
			Query:      rule.Query,
			TargetID:   rule.TargetID,
			TargetName: targetNames[rule.TargetID],
			Weight:     rule.Weight,
			Source:     source,
			Matched:    matched,
		})

		if matched {
			scores[rule.TargetID] += rule.Weight
			totalWeight += rule.Weight

			if source == MatchSourceFingerprint {
				fingerprintWeight[rule.TargetID] += rule.Weight
			} else {
				ruleWeight[rule.TargetID] += rule.Weight
			}
		}
	}

	// Build target scores
	targetScores := make([]TargetScore, 0)
	for targetID, score := range scores {
		targetScores = append(targetScores, TargetScore{
			TargetID:         targetID,
			TargetName:       targetNames[targetID],
			TotalWeight:      score,
			RuleWeight:       ruleWeight[targetID],
			FingerprintWeight: fingerprintWeight[targetID],
		})
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

	// Mark winner in scores
	for i := range targetScores {
		if targetScores[i].TargetID == winnerID {
			targetScores[i].IsWinner = true
		}
	}

	// Determine primary match source for the winner
	matchSource := MatchSourceRule
	if fingerprintWeight[winnerID] > ruleWeight[winnerID] {
		matchSource = MatchSourceFingerprint
	}

	// Calculate confidence
	var confidence float64
	if totalWeight > 0 {
		confidence = winnerScore / totalWeight
		if confidence > 1.0 {
			confidence = 1.0
		}
	}

	// Determine outcome
	var outcome string
	needsReview := false
	if len(scores) == 0 {
		outcome = "No rules matched - event would remain unclassified"
	} else if confidence < config.ConfidenceFloor {
		outcome = fmt.Sprintf("Confidence %.0f%% below threshold %.0f%% - would not classify", confidence*100, config.ConfidenceFloor*100)
	} else if confidence < config.ConfidenceCeiling {
		needsReview = true
		outcome = fmt.Sprintf("Classified to %s with %.0f%% confidence (needs review)", targetNames[winnerID], confidence*100)
	} else {
		outcome = fmt.Sprintf("Classified to %s with %.0f%% confidence (auto-approved)", targetNames[winnerID], confidence*100)
	}

	return &ExplainResult{
		ItemID:           item.ID,
		Evaluations:      evaluations,
		TargetScores:     targetScores,
		TotalWeight:      totalWeight,
		WinnerTargetID:   winnerID,
		WinnerConfidence: confidence,
		NeedsReview:      needsReview,
		MatchSource:      matchSource,
		Outcome:          outcome,
	}
}
