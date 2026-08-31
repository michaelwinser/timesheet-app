package classification

import (
	"context"

	"github.com/google/uuid"

	"github.com/michaelw/timesheet-app/service/internal/store"
)

// IngestionFilter is a user-defined rule for input noise: events that should
// never enter the classification stream at all.
//
// Filters are deliberately not classification rules. They are evaluated at
// ingestion rather than during classification, and they are not votes - there
// is no scoring, no confidence and no winner. They share the query syntax
// because that is familiar, not because they share the evaluation model.
// See: https://github.com/michaelwinser/timesheet-app/issues/110
type IngestionFilter struct {
	ID    string
	Name  string
	Query string
}

// IngestionFilterSet is a compiled set of filters, ready to evaluate against
// events. Queries are parsed once rather than per event.
type IngestionFilterSet struct {
	filters []compiledFilter
}

type compiledFilter struct {
	filter IngestionFilter
	ast    QueryNode
}

// NewIngestionFilterSet compiles filters for evaluation. A filter whose query
// does not parse is skipped and returned in the second result, so the caller
// can report it rather than silently ingesting everything it was meant to
// catch.
func NewIngestionFilterSet(filters []IngestionFilter) (*IngestionFilterSet, []IngestionFilter) {
	set := &IngestionFilterSet{}
	var invalid []IngestionFilter

	for _, f := range filters {
		ast, err := Parse(f.Query)
		if err != nil {
			invalid = append(invalid, f)
			continue
		}
		set.filters = append(set.filters, compiledFilter{filter: f, ast: ast})
	}

	return set, invalid
}

// Len reports how many filters will be evaluated.
func (s *IngestionFilterSet) Len() int {
	if s == nil {
		return 0
	}
	return len(s.filters)
}

// Match reports whether any filter matches the event, and which one did. The
// first match wins; filters are independent, so evaluation order carries no
// meaning beyond which name gets reported.
func (s *IngestionFilterSet) Match(event *store.CalendarEvent) (bool, IngestionFilter) {
	if s == nil || len(s.filters) == 0 {
		return false, IngestionFilter{}
	}

	props := EventToProperties(event)
	for _, cf := range s.filters {
		if Evaluate(cf.ast, props) {
			return true, cf.filter
		}
	}
	return false, IngestionFilter{}
}

// EventToProperties exposes the event-to-properties conversion so that callers
// outside this package - the sync paths, which evaluate ingestion filters
// before an event is ever stored - can match against an event.
func EventToProperties(event *store.CalendarEvent) *EventProperties {
	return itemToProperties(eventToItem(event))
}

// SuppressionResult reports what a re-evaluation changed.
type SuppressionResult struct {
	Evaluated    int
	NowHidden    int // newly suppressed
	NowVisible   int // no longer matched by any filter, so restored
	InvalidRules []IngestionFilter
}

// ReevaluateSuppression re-applies a user's enabled ingestion filters to every
// stored event, hiding what now matches and restoring what no longer does.
//
// This is what makes a filter reversible: disabling or deleting one brings its
// events back rather than leaving them hidden forever. It runs over all of a
// user's events, so it is a deliberate action - on filter changes - rather than
// something to call per request.
func (s *Service) ReevaluateSuppression(ctx context.Context, userID uuid.UUID, filters []IngestionFilter) (*SuppressionResult, error) {
	set, invalid := NewIngestionFilterSet(filters)

	events, err := s.eventStore.ListForFilterEvaluation(ctx, userID)
	if err != nil {
		return nil, err
	}

	var toHide, toRestore []uuid.UUID
	for _, event := range events {
		matched, _ := set.Match(event)
		switch {
		case matched && !event.IsSuppressed:
			toHide = append(toHide, event.ID)
		case !matched && event.IsSuppressed:
			toRestore = append(toRestore, event.ID)
		}
	}

	if _, err := s.eventStore.SetSuppressedBulk(ctx, userID, toHide, true); err != nil {
		return nil, err
	}
	if _, err := s.eventStore.SetSuppressedBulk(ctx, userID, toRestore, false); err != nil {
		return nil, err
	}

	return &SuppressionResult{
		Evaluated:    len(events),
		NowHidden:    len(toHide),
		NowVisible:   len(toRestore),
		InvalidRules: invalid,
	}, nil
}
