package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderJourney(t *testing.T, view journeyPageView, message, pageError string) string {
	t.Helper()
	var output bytes.Buffer
	require.NoError(t, JourneyPage(domain.User{ID: "owner-1", Username: "learner"}, "csrf-token", view, message, pageError).Render(context.Background(), &output))
	return output.String()
}

func testJourneyBook(id, title, status string) journeyBookView {
	return journeyBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: id, Title: title, Language: "de"}, AnalysisStatus: status}}
}

func TestJourneyPageRendersGoalAndProvisionalOrder(t *testing.T) {
	goal := testJourneyBook("goal", "Goal book", "ready")
	provisional := []journeyBookView{
		testJourneyBook("first", "First provisional book", "ready"),
		testJourneyBook("second", "Second provisional book", "ready"),
	}
	html := renderJourney(t, journeyPageView{Goal: &goal, Provisional: provisional}, "", "")
	goalIndex := strings.Index(html, "Goal book")
	provisionalIndex := strings.Index(html, `id="provisional-journey-heading"`)
	firstIndex := strings.Index(html, "First provisional book")
	secondIndex := strings.Index(html, "Second provisional book")
	assert.True(t, goalIndex >= 0 && provisionalIndex >= 0 && firstIndex >= 0 && secondIndex >= 0 && goalIndex <= provisionalIndex && firstIndex <= secondIndex, "journey order was not goal-first and learner-ordered: goal=%d provisional=%d first=%d second=%d", goalIndex, provisionalIndex, firstIndex, secondIndex)
	assert.Contains(t, html, `<ol class="journey-list" aria-label="Your order">`)
	assert.NotContains(t, html, "vocabulary-efficient")
	assert.NotContains(t, html, "advisory order")
}

func TestJourneyGoalShowsSnapshotBoundDeckRecoveryWithoutConsent(t *testing.T) {
	goal := testJourneyBook("goal", "Goal book", "analyzed")
	goal.GoalSnapshotSize = 2
	goal.GoalVocabularyEligible = 1
	goal.GoalPreparation = &domain.DeckPreparation{ID: "goal-preparation", GoalSnapshotID: "snapshot", State: domain.DeckPreparationFailed, FailureClass: "provider"}
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	assert.Contains(t, html, "Reserved vocabulary</strong>: <span class=\"numeric\">2</span> frozen identities.")
	assert.Contains(t, html, "1 currently eligible frozen Reserved identities")
	assert.Contains(t, html, "Goal deck preparation")
	assert.Contains(t, html, "Retry Goal deck")
	assert.Contains(t, html, `action="/deck-preparations/goal-preparation/retry"`)
	goalCardStart := strings.Index(html, `id="journey-book-goal"`)
	goalCardEnd := strings.Index(html[goalCardStart:], "</article>")
	require.GreaterOrEqual(t, goalCardStart, 0)
	require.Greater(t, goalCardEnd, 0)
	assert.NotContains(t, html[goalCardStart:goalCardStart+goalCardEnd], "external_translation_consent")
}

func TestJourneyGoalRendersEmptyReservedVocabularyCount(t *testing.T) {
	goal := testJourneyBook("empty-goal", "Empty Goal book", "analyzed")
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	assert.Contains(t, html, "Reserved vocabulary</strong>: <span class=\"numeric\">0</span> frozen identities.")
	assert.NotContains(t, html, "Deck preparation unavailable")
	assert.NotContains(t, html, "Retry deck preparation")
}

func TestJourneyGoalShowsReservedVocabularyWhenDeckIsUnavailable(t *testing.T) {
	goal := testJourneyBook("unavailable-goal", "Unavailable Goal book", "analyzed")
	goal.GoalSnapshotSize = 2
	goal.GoalDeckUnavailable = true
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	assert.Contains(t, html, "Reserved vocabulary</strong>: <span class=\"numeric\">2</span> frozen identities.")
	assert.Contains(t, html, "Deck preparation unavailable")
	assert.Contains(t, html, "Retry deck preparation")
}

func TestJourneyPageRendersCanonicalBookTitle(t *testing.T) {
	book := testJourneyBook("canonical-book", "Acquisition title", "ready")
	book.Book.BookTitle = "Catalogue title"
	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{book}}, "", "")
	assert.True(t, strings.Contains(html, "Catalogue title") && !strings.Contains(html, "Acquisition title"), "Journey did not use canonical Book title: %s", html)
}

func TestJourneyEvidenceActionsRemainAvailable(t *testing.T) {
	stale := testJourneyBook("stale", "Stale book", "stale")
	stale.Book.Source.MediaType = "application/epub+zip"
	stale.Book.Source.ContentRevisionID = "current-revision"
	stale.Book.Source.ContentSnapshotID = "current-snapshot"
	stale.Book.AnalysisState = "completed"
	stale.Book.AnalysisRunID = "old-run"
	stale.Book.CorpusID = "old-corpus"
	action := journeyAnalysisAction(stale)
	assert.Equal(t, "Stale analysis", action.Status)
	assert.Equal(t, "Re-analyze", action.Label)
	assert.Equal(t, "/journey/books/stale/reanalyze", action.URL)
	assert.True(t, action.Submit)
	unassessed := testJourneyBook("unassessed", "Unassessed book", "not analyzed")
	unassessed.Book.Source.MediaType = "application/epub+zip"
	unassessed.Book.Source.ContentRevisionID = "revision"
	unassessed.Book.Source.ContentSnapshotID = "snapshot"
	action = journeyAnalysisAction(unassessed)
	assert.Equal(t, "Analysis incomplete", action.Status)
	assert.Equal(t, "Retry analysis", action.Label)
	assert.Equal(t, "/journey/books/unassessed/reanalyze", action.URL)
	assert.True(t, action.Submit)
}

func TestJourneyPageUsesCanonicalJourneyEntryLink(t *testing.T) {
	book := testJourneyBook("source-book", "Completed book", "analyzed")
	book.BookID = "canonical-book"
	book.Book.Source.MediaType = "application/epub+zip"
	book.Book.Source.ContentRevisionID = "revision"
	book.Book.Source.ContentSnapshotID = "snapshot"
	book.Book.AnalysisState = "completed"
	book.Book.AnalysisRunID = "run"
	book.Book.CorpusID = "corpus"
	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{book}}, "", "")
	assert.True(t, strings.Contains(html, `href="/journey/canonical-book"`) && !strings.Contains(html, `href="/books/source-book"`), "Journey card used a non-canonical entry link: %s", html)
}

func TestJourneyCoverageLabelsConditionalVocabulary(t *testing.T) {
	item := testJourneyBook("conditional", "Conditional book", "analyzed")
	item.Coverage = &domain.AnalysisCoverage{
		AnalyzableTokenCount: 100,
		KnownTokenCount:      50,
		ReservedTokenCount:   30,
		Projections:          []domain.CoverageProjection{{TopLemmaCount: 2, ProjectedTokenCount: 75}},
	}
	assert.Equal(t, "50.0%", journeyCurrentCoverage(item))
}

func TestJourneyPageRendersSequentialForecastMeaningsAndLowerBound(t *testing.T) {
	item := testJourneyBook("forecast", "Forecast book", "analyzed")
	item.Forecast = &domain.JourneyForecastEntry{
		BookID:     "forecast",
		Current:    &domain.JourneyForecastCoverage{KnownTokenCount: 40, AnalyzableTokenCount: 100},
		AfterGoal:  &domain.JourneyForecastCoverage{KnownTokenCount: 60, AnalyzableTokenCount: 100},
		OnArrival:  &domain.JourneyForecastCoverage{KnownTokenCount: 60, AnalyzableTokenCount: 100},
		LowerBound: true,
	}
	item.ForecastHasGoal = true
	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{item}}, "", "")
	for _, want := range []string{
		`aria-label="Journey coverage forecast"`,
		"Current coverage",
		"After Primary Goal",
		"On arrival",
		"40.0%",
		"60.0%",
		"No change",
		"Lower bound:",
	} {
		assert.Contains(t, html, want)
	}
	assert.NotContains(t, html, "same as after Goal")
}

func TestJourneyForecastLedgerShowsSignedChangesAndNoActiveGoal(t *testing.T) {
	item := testJourneyBook("delta", "Delta book", "analyzed")
	item.Forecast = &domain.JourneyForecastEntry{
		Current:   &domain.JourneyForecastCoverage{KnownTokenCount: 40, AnalyzableTokenCount: 100},
		AfterGoal: &domain.JourneyForecastCoverage{KnownTokenCount: 60, AnalyzableTokenCount: 100},
		OnArrival: &domain.JourneyForecastCoverage{KnownTokenCount: 75, AnalyzableTokenCount: 100},
	}
	item.ForecastHasGoal = true
	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{item}}, "", "")
	assert.Contains(t, html, "+20.0 percentage points")
	assert.Contains(t, html, "+15.0 percentage points")
	assert.Contains(t, html, "Evidence and calculation")
	assert.Contains(t, html, "40 of 100 analyzable tokens")

	item.ForecastHasGoal = false
	html = renderJourney(t, journeyPageView{Provisional: []journeyBookView{item}}, "", "")
	assert.Contains(t, html, "No active Primary Goal")
	assert.NotContains(t, html, "+20.0 percentage points")
}

func TestJourneyForecastDeltaDoesNotHideSmallChanges(t *testing.T) {
	left := &domain.JourneyForecastCoverage{KnownTokenCount: 2, AnalyzableTokenCount: 100000}
	right := &domain.JourneyForecastCoverage{KnownTokenCount: 1, AnalyzableTokenCount: 100000}

	assert.Equal(t, "+0.00 percentage points", journeyForecastDeltaLabel(left, right))
}

func TestJourneyGoalForecastOmitsOnArrivalStage(t *testing.T) {
	goal := testJourneyBook("goal-forecast", "Goal forecast book", "analyzed")
	goal.Forecast = &domain.JourneyForecastEntry{
		Current:   &domain.JourneyForecastCoverage{KnownTokenCount: 40, AnalyzableTokenCount: 100},
		AfterGoal: &domain.JourneyForecastCoverage{KnownTokenCount: 60, AnalyzableTokenCount: 100},
		OnArrival: &domain.JourneyForecastCoverage{KnownTokenCount: 75, AnalyzableTokenCount: 100},
	}
	goal.ForecastHasGoal = true
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	goalStart := strings.Index(html, `id="journey-book-goal-forecast"`)
	goalEnd := strings.Index(html[goalStart:], "</article>")
	require.GreaterOrEqual(t, goalStart, 0)
	require.Greater(t, goalEnd, 0)
	goalHTML := html[goalStart : goalStart+goalEnd]
	assert.Contains(t, goalHTML, "Current coverage")
	assert.Contains(t, goalHTML, "After Primary Goal")
	assert.NotContains(t, goalHTML, "On arrival")
}

func TestJourneyHealthyEvidenceStaysQuiet(t *testing.T) {
	item := testJourneyBook("healthy", "Healthy book", "analyzed")
	item.Book.Source.MediaType = "application/epub+zip"
	item.Book.Source.ContentRevisionID = "revision"
	item.Book.Source.ContentSnapshotID = "snapshot"
	item.Book.AnalysisState = "completed"
	item.Book.AnalysisRunID = "run"
	item.Book.CorpusID = "corpus"
	item.Coverage = &domain.AnalysisCoverage{AnalyzableTokenCount: 10}
	item.Forecast = &domain.JourneyForecastEntry{Current: &domain.JourneyForecastCoverage{KnownTokenCount: 5, AnalyzableTokenCount: 10}}
	html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{item}}, "", "")
	assert.NotContains(t, html, "Current evidence")
	assert.NotContains(t, html, "Assessment:")
	assert.NotContains(t, html, "Analysis result ready")
	assert.Contains(t, html, "Current coverage")
}

func TestJourneyExceptionalEvidenceNamesStateAndRecovery(t *testing.T) {
	tests := []struct {
		name, status, wantState, wantLabel, wantAction string
	}{
		{name: "stale", status: "stale", wantState: "stale", wantLabel: "Stale evidence", wantAction: "Re-analyze"},
		{name: "unavailable", status: "", wantState: "unavailable", wantLabel: "Evidence unavailable", wantAction: "Retry acquisition"},
		{name: "incomplete", status: "not analyzed", wantState: "incomplete", wantLabel: "Incomplete evidence", wantAction: "Retry analysis"},
		{name: "failed", status: "analysis failed", wantState: "failed", wantLabel: "Analysis failed", wantAction: "Retry analysis"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := testJourneyBook(tt.name, tt.name, tt.status)
			if tt.name != "unavailable" {
				item.Book.Source.MediaType = "application/epub+zip"
				item.Book.Source.ContentRevisionID = "revision"
				item.Book.Source.ContentSnapshotID = "snapshot"
			}
			assert.Equal(t, tt.wantState, journeyEvidenceState(item))
			html := renderJourney(t, journeyPageView{Provisional: []journeyBookView{item}}, "", "")
			assert.Contains(t, html, tt.wantLabel)
			assert.Contains(t, html, tt.wantAction)
		})
	}
}

func TestJourneyIncompleteAnalyzedEvidenceOffersReanalysis(t *testing.T) {
	item := testJourneyBook("incomplete", "Incomplete analyzed book", "analyzed")
	item.Book.Source.MediaType = "application/epub+zip"
	item.Book.Source.ContentRevisionID = "revision"
	item.Book.Source.ContentSnapshotID = "snapshot"
	item.Book.AnalysisState = "completed"
	item.Book.AnalysisRunID = "run"
	item.Book.CorpusID = "corpus"
	item.StatisticsUnavailable = true

	action := journeyAnalysisAction(item)
	assert.Equal(t, "Analysis incomplete", action.Status)
	assert.Equal(t, "Retry analysis", action.Label)
	assert.Equal(t, "/journey/books/incomplete/reanalyze", action.URL)
	assert.True(t, action.Submit)
}

func TestJourneyTreatsAnalyzedEvidenceAndEligibleGoalsAsCurrent(t *testing.T) {
	item := testJourneyBook("analyzed", "Analyzed book", "analyzed")
	item.Book.Source.MediaType = "application/epub+zip"
	item.Book.Source.ContentRevisionID = "revision"
	item.Book.Source.ContentSnapshotID = "snapshot"
	item.Book.AnalysisState = "completed"
	item.Book.AnalysisRunID = "run"
	item.Book.CorpusID = "corpus"
	item.Coverage = &domain.AnalysisCoverage{AnalyzableTokenCount: 10}

	assert.Equal(t, "current", journeyEvidenceState(item))
	eligible, message := journeyGoalEligibility(item.Book)
	assert.True(t, eligible)
	assert.Empty(t, message)
}
