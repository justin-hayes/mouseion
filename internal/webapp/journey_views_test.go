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
	provisionalIndex := strings.Index(html, "Provisional Journey")
	firstIndex := strings.Index(html, "First provisional book")
	secondIndex := strings.Index(html, "Second provisional book")
	assert.True(t, goalIndex >= 0 && provisionalIndex >= 0 && firstIndex >= 0 && secondIndex >= 0 && goalIndex <= provisionalIndex && firstIndex <= secondIndex, "journey order was not goal-first and learner-ordered: goal=%d provisional=%d first=%d second=%d", goalIndex, provisionalIndex, firstIndex, secondIndex)
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
	assert.Equal(t, "Analysis not started", action.Status)
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
	assert.Equal(t, "80.0% if reserved vocabulary graduates; 75.0% after the top 2 deck-eligible lemmas", journeyProjectedCoverage(item))
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
