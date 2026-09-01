package webapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type testJourneyProjectionProvider struct {
	result domain.JourneyProjectionResult
	err    error
}

func (p testJourneyProjectionProvider) JourneyProjection(context.Context, string, string) (domain.JourneyProjectionResult, error) {
	return p.result, p.err
}

func TestRouteEvidenceLabelsDistinguishEvidenceStates(t *testing.T) {
	coverage := &domain.AnalysisCoverage{AnalyzableTokenCount: 10, KnownTokenCount: 5}
	tests := []struct {
		name  string
		book  domain.JourneyRouteBook
		label string
	}{
		{name: "current", book: domain.JourneyRouteBook{Comparable: true, Coverage: coverage}, label: "Current"},
		{name: "conditional", book: domain.JourneyRouteBook{Comparable: true, Coverage: coverage, ConditionalCoverage: coverage}, label: "Conditional"},
		{name: "stale", book: domain.JourneyRouteBook{IncomparableReason: "stale: old corpus"}, label: "Stale"},
		{name: "unavailable", book: domain.JourneyRouteBook{IncomparableReason: "unavailable: no corpus"}, label: "Coverage unavailable"},
		{name: "different language", book: domain.JourneyRouteBook{IncomparableReason: "different study language"}, label: "Coverage unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := routeEvidenceLabel(test.book); got != test.label {
				t.Fatalf("routeEvidenceLabel() = %q, want %q", got, test.label)
			}
		})
	}
}

func TestBuildRouteComparisonViewHasConditionalOnlyWhenOrderExists(t *testing.T) {
	book := domain.JourneyRouteBook{BookID: "book-1", Comparable: true}
	without := buildRouteComparisonView(domain.JourneyProjectionResult{LearnerOrder: []domain.JourneyRouteBook{book}, AdvisoryOrder: []domain.JourneyRouteBook{book}}, map[string]string{"book-1": "A book"})
	if without.HasConditional || without.ComparisonUnavailable {
		t.Fatalf("empty conditional comparison view = %#v", without)
	}
	with := buildRouteComparisonView(domain.JourneyProjectionResult{
		LearnerOrder:             []domain.JourneyRouteBook{book},
		AdvisoryOrder:            []domain.JourneyRouteBook{book},
		ConditionalAdvisoryOrder: []domain.JourneyRouteBook{book},
	}, map[string]string{"book-1": "A book"})
	if !with.HasConditional || with.LearnerOrder[0].Title != "A book" {
		t.Fatalf("conditional comparison view = %#v", with)
	}
}

func TestBuildRouteComparisonViewMarksEmptyProjectionUnavailable(t *testing.T) {
	view := buildRouteComparisonView(domain.JourneyProjectionResult{}, nil)
	if !view.ComparisonUnavailable {
		t.Fatal("empty projection was not marked unavailable")
	}
}

func TestJourneyRouteComparisonSoftensProviderFailureAndEmptyResult(t *testing.T) {
	failed, err := journeyRouteComparison(context.Background(), testJourneyProjectionProvider{err: errors.New("projection failed")}, "owner-1", nil)
	if err == nil || failed != nil {
		t.Fatalf("provider failure view=%#v err=%v", failed, err)
	}
	empty, err := journeyRouteComparison(context.Background(), testJourneyProjectionProvider{}, "owner-1", nil)
	if err != nil || empty == nil || !empty.ComparisonUnavailable {
		t.Fatalf("empty provider result view=%#v err=%v", empty, err)
	}
	available, err := journeyRouteComparison(context.Background(), testJourneyProjectionProvider{result: domain.JourneyProjectionResult{
		LearnerOrder:  []domain.JourneyRouteBook{{BookID: "book-1"}},
		AdvisoryOrder: []domain.JourneyRouteBook{{BookID: "book-1"}},
	}}, "owner-1", nil)
	if err != nil || available == nil || available.ComparisonUnavailable {
		t.Fatalf("available provider result view=%#v err=%v", available, err)
	}
}

func TestRouteComparisonRenderingKeepsOrderAndExplainsEvidence(t *testing.T) {
	coverage := func(known int64) *domain.AnalysisCoverage {
		return &domain.AnalysisCoverage{AnalyzableTokenCount: 100, KnownTokenCount: known}
	}
	rank := func(value int) *int { return &value }
	books := []domain.JourneyRouteBook{
		{BookID: "match", Position: 1, Comparable: true, Coverage: coverage(90), Rank: rank(1)},
		{BookID: "differs", Position: 2, Comparable: true, Coverage: coverage(20), Rank: rank(4)},
		{BookID: "tie-a", Position: 3, Comparable: true, Coverage: coverage(50), Rank: rank(2)},
		{BookID: "tie-b", Position: 4, Comparable: true, Coverage: coverage(50), Rank: rank(3)},
		{BookID: "unavailable", Position: 5, IncomparableReason: "unassessed: no current analyzed corpus"},
	}
	view := buildRouteComparisonView(domain.JourneyProjectionResult{
		Language: "de", LearnerOrder: books,
		AdvisoryOrder:   []domain.JourneyRouteBook{books[0], books[2], books[3], books[1], books[4]},
		ComparableCount: 4, IncomparableCount: 1,
	}, map[string]string{"match": "Match", "differs": "Differs", "tie-a": "Tie A", "tie-b": "Tie B", "unavailable": "Unavailable"})
	html := renderJourney(t, journeyPageView{RouteComparison: view}, "", "", "")
	for _, want := range []string{"canonical", "current known-token coverage", "not ranked", "Match", "Differs", "Tie A", "Tie B", "unassessed"} {
		if !strings.Contains(strings.ToLower(html), strings.ToLower(want)) {
			t.Errorf("comparison rendering missing %q: %s", want, html)
		}
	}
	if strings.Contains(strings.ToLower(html), "optimal") || strings.Contains(strings.ToLower(html), "best next book") {
		t.Fatalf("comparison used prohibited recommendation language: %s", html)
	}
	learnerStart := strings.Index(html, "Match")
	differsStart := strings.Index(html, "Differs")
	if learnerStart < 0 || differsStart < 0 || learnerStart > differsStart {
		t.Fatalf("learner order was not preserved in rendered comparison: %s", html)
	}
}
