package classification

import (
	"net/mail"
	"strings"
	"time"
	"unicode"
)

// EventProperties provides access to event properties for rule evaluation
type EventProperties struct {
	Title          string
	Description    string
	Attendees      []string // Email addresses
	StartTime      time.Time
	EndTime        time.Time
	ResponseStatus string // accepted, declined, needsAction, tentative
	Transparency   string // opaque, transparent
	IsRecurring    bool
	CalendarName   string // Name of the source calendar
}

// Evaluate evaluates a query against event properties
// Returns true if the query matches the event
func Evaluate(node QueryNode, props *EventProperties) bool {
	switch n := node.(type) {
	case *ConditionNode:
		result := evaluateCondition(n, props)
		if n.Negated {
			return !result
		}
		return result

	case *AndNode:
		for _, child := range n.Children {
			if !Evaluate(child, props) {
				return false
			}
		}
		return true

	case *OrNode:
		for _, child := range n.Children {
			if Evaluate(child, props) {
				return true
			}
		}
		return false

	default:
		return false
	}
}

func evaluateCondition(cond *ConditionNode, props *EventProperties) bool {
	switch cond.Property {
	case "title":
		return containsWordIgnoreCase(props.Title, cond.Value)

	case "description":
		return containsWordIgnoreCase(props.Description, cond.Value)

	case "attendees":
		// Match if any attendee contains the value
		for _, attendee := range props.Attendees {
			if containsIgnoreCase(attendee, cond.Value) {
				return true
			}
		}
		return false

	case "domain":
		// Match if any attendee has this domain
		targetDomain := strings.ToLower(cond.Value)
		for _, attendee := range props.Attendees {
			domain := extractDomain(attendee)
			if domain == targetDomain {
				return true
			}
		}
		return false

	case "email":
		// Exact email match in attendees
		targetEmail := strings.ToLower(cond.Value)
		for _, attendee := range props.Attendees {
			if strings.ToLower(attendee) == targetEmail {
				return true
			}
		}
		return false

	case "response":
		// Response status: accepted, declined, needsAction, tentative
		return strings.EqualFold(props.ResponseStatus, cond.Value)

	case "recurring":
		// recurring:yes or recurring:no
		wantRecurring := strings.EqualFold(cond.Value, "yes") || strings.EqualFold(cond.Value, "true")
		return props.IsRecurring == wantRecurring

	case "transparency":
		// transparency:opaque or transparency:transparent
		return strings.EqualFold(props.Transparency, cond.Value)

	case "day-of-week":
		// day-of-week:mon, tue, wed, thu, fri, sat, sun
		dayName := strings.ToLower(props.StartTime.Weekday().String())
		shortDay := dayName[:3]
		return strings.EqualFold(shortDay, cond.Value) || strings.EqualFold(dayName, cond.Value)

	case "time-of-day":
		// time-of-day:>17:00 or time-of-day:<09:00
		return evaluateTimeOfDay(props.StartTime, cond.Value)

	case "has-attendees":
		// has-attendees:yes or has-attendees:no
		wantAttendees := strings.EqualFold(cond.Value, "yes") || strings.EqualFold(cond.Value, "true")
		hasAttendees := len(props.Attendees) > 0
		return hasAttendees == wantAttendees

	case "is-all-day":
		// is-all-day:yes - check if event spans full day(s)
		wantAllDay := strings.EqualFold(cond.Value, "yes") || strings.EqualFold(cond.Value, "true")
		isAllDay := isAllDayEvent(props.StartTime, props.EndTime)
		return isAllDay == wantAllDay

	case "calendar":
		// Match against calendar name (word boundary)
		return containsWordIgnoreCase(props.CalendarName, cond.Value)

	case "text":
		// Text search across title, description, and attendees (word boundary)
		if containsWordIgnoreCase(props.Title, cond.Value) {
			return true
		}
		if containsWordIgnoreCase(props.Description, cond.Value) {
			return true
		}
		for _, attendee := range props.Attendees {
			if containsWordIgnoreCase(attendee, cond.Value) {
				return true
			}
		}
		return false

	default:
		// Unknown property, no match
		return false
	}
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// containsWordIgnoreCase checks if s contains word as a complete word (case-insensitive).
// A word boundary is any non-alphanumeric character or start/end of string.
// This prevents "AC" from matching inside "Jack" while still matching "AC 123" or "fly AC".
//
// For multi-word phrases (containing spaces), falls back to substring matching since
// phrases like "out of office" are specific enough to not cause false positives.
func containsWordIgnoreCase(s, word string) bool {
	sLower := strings.ToLower(s)
	wordLower := strings.ToLower(word)

	// Multi-word phrases: use substring matching (they're specific enough)
	if strings.Contains(wordLower, " ") {
		return strings.Contains(sLower, wordLower)
	}

	// Single words: require word boundary matching
	// Tokenize the string into words
	words := tokenize(sLower)

	// Check if any word matches exactly
	for _, w := range words {
		if w == wordLower {
			return true
		}
	}
	return false
}

// tokenize splits a string into words, treating any non-alphanumeric character as a delimiter.
// Returns lowercase words.
func tokenize(s string) []string {
	var words []string
	var current strings.Builder

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
	}

	// Don't forget the last word
	if current.Len() > 0 {
		words = append(words, current.String())
	}

	return words
}

// extractDomain extracts the domain from an email address
func extractDomain(email string) string {
	// Try to parse as email address
	addr, err := mail.ParseAddress(email)
	if err == nil {
		email = addr.Address
	}

	// Find @ and return everything after
	parts := strings.Split(strings.ToLower(email), "@")
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

// evaluateTimeOfDay evaluates time comparisons like >17:00 or <09:00
func evaluateTimeOfDay(t time.Time, value string) bool {
	if len(value) < 2 {
		return false
	}

	var op string
	var timeStr string

	if value[0] == '>' || value[0] == '<' {
		op = string(value[0])
		timeStr = value[1:]
		if len(timeStr) > 0 && timeStr[0] == '=' {
			op += "="
			timeStr = timeStr[1:]
		}
	} else {
		// Exact match
		op = "="
		timeStr = value
	}

	// Parse HH:MM
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return false
	}

	var targetHour, targetMin int
	_, err := parseTimeComponents(parts[0], parts[1], &targetHour, &targetMin)
	if err != nil {
		return false
	}

	eventMinutes := t.Hour()*60 + t.Minute()
	targetMinutes := targetHour*60 + targetMin

	switch op {
	case ">":
		return eventMinutes > targetMinutes
	case ">=":
		return eventMinutes >= targetMinutes
	case "<":
		return eventMinutes < targetMinutes
	case "<=":
		return eventMinutes <= targetMinutes
	case "=":
		return eventMinutes == targetMinutes
	default:
		return false
	}
}

func parseTimeComponents(hourStr, minStr string, hour, min *int) (bool, error) {
	var err error
	*hour = 0
	*min = 0

	for _, c := range hourStr {
		if c < '0' || c > '9' {
			return false, nil
		}
		*hour = *hour*10 + int(c-'0')
	}

	for _, c := range minStr {
		if c < '0' || c > '9' {
			return false, nil
		}
		*min = *min*10 + int(c-'0')
	}

	return true, err
}

// isAllDayEvent checks if an event spans full days
func isAllDayEvent(start, end time.Time) bool {
	// All-day events typically start at midnight and end at midnight
	// Check if both start and end are at midnight
	startMidnight := start.Hour() == 0 && start.Minute() == 0 && start.Second() == 0
	endMidnight := end.Hour() == 0 && end.Minute() == 0 && end.Second() == 0

	// Duration should be at least 24 hours
	duration := end.Sub(start)
	return startMidnight && endMidnight && duration >= 24*time.Hour
}

// ExtractDomains returns unique domains from attendee list
func ExtractDomains(attendees []string) []string {
	seen := make(map[string]bool)
	var domains []string

	for _, attendee := range attendees {
		domain := extractDomain(attendee)
		if domain != "" && !seen[domain] {
			seen[domain] = true
			domains = append(domains, domain)
		}
	}

	return domains
}

// ExtendedEventProperties includes event properties plus classification metadata
// for filtering by project, client, confidence level, and skip status
type ExtendedEventProperties struct {
	EventProperties
	ProjectID    *string
	ProjectName  *string
	ClientName   *string
	Confidence   *float64
	IsClassified bool
	IsSkipped    bool
}

// EvaluateExtended evaluates a query against extended event properties
// This supports all standard conditions plus: project:, client:, confidence:
func EvaluateExtended(node QueryNode, props *ExtendedEventProperties) bool {
	switch n := node.(type) {
	case *ConditionNode:
		result := evaluateExtendedCondition(n, props)
		if n.Negated {
			return !result
		}
		return result

	case *AndNode:
		for _, child := range n.Children {
			if !EvaluateExtended(child, props) {
				return false
			}
		}
		return true

	case *OrNode:
		for _, child := range n.Children {
			if EvaluateExtended(child, props) {
				return true
			}
		}
		return false

	default:
		return false
	}
}

func evaluateExtendedCondition(cond *ConditionNode, props *ExtendedEventProperties) bool {
	switch cond.Property {
	case "project":
		// Special case: project:unclassified
		if strings.EqualFold(cond.Value, "unclassified") {
			return props.ProjectID == nil || !props.IsClassified
		}
		// Match against project name (word boundary)
		if props.ProjectName == nil {
			return false
		}
		return containsWordIgnoreCase(*props.ProjectName, cond.Value)

	case "client":
		// Match against client name (word boundary)
		if props.ClientName == nil {
			return false
		}
		return containsWordIgnoreCase(*props.ClientName, cond.Value)

	case "confidence":
		// confidence:high, confidence:medium, confidence:low
		if props.Confidence == nil {
			// No confidence = low
			return strings.EqualFold(cond.Value, "low")
		}
		conf := *props.Confidence
		switch strings.ToLower(cond.Value) {
		case "high":
			return conf >= ConfidenceCeiling // >= 0.8
		case "medium":
			return conf >= ConfidenceFloor && conf < ConfidenceCeiling // 0.5 <= conf < 0.8
		case "low":
			return conf < ConfidenceFloor // < 0.5
		default:
			return false
		}

	case "status":
		// status:pending, status:classified, status:skipped
		switch strings.ToLower(cond.Value) {
		case "pending":
			return !props.IsClassified && !props.IsSkipped
		case "classified":
			return props.IsClassified && !props.IsSkipped
		case "skipped":
			return props.IsSkipped
		default:
			return false
		}

	default:
		// Delegate to standard condition evaluation
		return evaluateCondition(cond, &props.EventProperties)
	}
}
