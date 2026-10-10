package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

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

func TestJourneyPageIdentifiesCurrentReadingSinceAndKeepsLifecycleActionsWithBook(t *testing.T) {
	goal := testJourneyBook("current", "Der lange Weg nach Hause", "ready")
	goal.Book.BookAuthor = "A. Reader"
	goal.Cover = domain.BookCover{State: domain.BookCoverUnavailable}
	goal.ReadingSince = time.Date(2026, time.January, 3, 12, 0, 0, 0, time.UTC)
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")

	assert.Contains(t, html, "Reading since")
	assert.Contains(t, html, `<time datetime="2026-01-03">January 3, 2026</time>`)
	assert.Contains(t, html, "Der lange Weg nach Hause")
	sinceIndex := strings.Index(html, `<p class="journey-book__since">`)
	titleIndex := strings.Index(html, `<h1 class="journey-book__title">`)
	assert.True(t, sinceIndex >= 0 && sinceIndex < titleIndex, "Reading since must remain outside the Book title heading: %s", html)
	assert.Contains(t, html, "By A. Reader")
	assert.Contains(t, html, `class="journey-book__title-page journey-book__title-page--current"`)
	assert.Contains(t, html, `class="journey-book__lifecycle"`)
	assert.Contains(t, html, "Mark reading finished")
	assert.Contains(t, html, "Switch current reading")
	assert.Contains(t, html, "End current reading")
	assert.NotContains(t, html, "Set aside this Book")
}

func TestIsCurrentReadingShowsSnapshotBoundDeckRecoveryWithoutConsent(t *testing.T) {
	goal := testJourneyBook("goal", "Goal book", "analyzed")
	goal.GoalSnapshotSize = 2
	goal.GoalSnapshotID = "snapshot"
	goal.GoalVocabularyEligible = 1
	goal.GoalPreparation = &domain.DeckPreparation{ID: "goal-preparation", SnapshotID: "snapshot", State: domain.DeckPreparationFailed, FailureClass: "provider"}
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	assert.Contains(t, html, "2 lemmas are set aside from vocabulary selection while you read this Book.")
	assert.Contains(t, html, "1 currently eligible frozen Reserved identities")
	assert.Contains(t, html, "Book deck")
	assert.Contains(t, html, "Retry deck preparation")
	assert.Contains(t, html, `action="/reading/books/goal/deck/retry"`)
	assert.Contains(t, html, `name="expected_current_snapshot_id" value="snapshot"`)
	assert.NotContains(t, html, `action="/deck-preparations/goal-preparation/retry"`)
	goalCardStart := strings.Index(html, `id="journey-book-goal"`)
	goalCardEnd := strings.Index(html[goalCardStart:], "</article>")
	require.GreaterOrEqual(t, goalCardStart, 0)
	require.Greater(t, goalCardEnd, 0)
	assert.NotContains(t, html[goalCardStart:goalCardStart+goalCardEnd], "external_translation_consent")
}

func TestIsCurrentReadingRendersEmptyReservedVocabularyCount(t *testing.T) {
	goal := testJourneyBook("empty-goal", "Empty Goal book", "analyzed")
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	assert.Contains(t, html, "0 lemmas are set aside from vocabulary selection while you read this Book.")
	assert.NotContains(t, html, "Deck preparation unavailable")
	assert.NotContains(t, html, "Retry deck preparation")
}

func TestIsCurrentReadingShowsReservedVocabularyWhenDeckIsUnavailable(t *testing.T) {
	goal := testJourneyBook("unavailable-goal", "Unavailable Goal book", "analyzed")
	goal.GoalSnapshotSize = 2
	goal.GoalDeckUnavailable = true
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	assert.Contains(t, html, "2 lemmas are set aside from vocabulary selection while you read this Book.")
	assert.Contains(t, html, "Deck unavailable.")
	assert.Contains(t, html, "Retry deck preparation")
}

func TestIsCurrentReadingShowsMissingDeckWithoutChangingGoalFacts(t *testing.T) {
	goal := testJourneyBook("missing-goal", "Missing Goal deck", "analyzed")
	goal.GoalSnapshotSize = 2
	goal.GoalSnapshotID = "missing-snapshot"
	goal.GoalDeckMissing = true
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	assert.Contains(t, html, "Deck missing.")
	assert.Contains(t, html, "Prepare a deck for the vocabulary reserved for this reading.")
	assert.Contains(t, html, "Prepare deck")
	assert.Contains(t, html, `name="expected_current_snapshot_id" value="missing-snapshot"`)
}

func TestReservedVocabularySummaryPluralizesLearnerCopy(t *testing.T) {
	assert.Equal(t, "1 lemma is set aside from vocabulary selection while you read this Book.", reservedVocabularySummary(1))
	assert.Equal(t, "2 lemmas are set aside from vocabulary selection while you read this Book.", reservedVocabularySummary(2))
}

func TestIsCurrentReadingShowsReadingStateAndPageAction(t *testing.T) {
	goal := testJourneyBook("goal-state", "Goal state book", "analyzed")
	html := renderJourney(t, journeyPageView{Goal: &goal}, "", "")
	assert.Contains(t, html, "Reserved vocabulary</h2>")
	assert.Contains(t, html, "Analysis</h2>")
	assert.Contains(t, html, "Book deck</h2>")
	assert.Contains(t, html, `href="/library">Add books from My Books</a>`)
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
	assert.Equal(t, "/reading/books/stale/reanalyze", action.URL)
	assert.True(t, action.Submit)
	unassessed := testJourneyBook("unassessed", "Unassessed book", "not analyzed")
	unassessed.Book.Source.MediaType = "application/epub+zip"
	unassessed.Book.Source.ContentRevisionID = "revision"
	unassessed.Book.Source.ContentSnapshotID = "snapshot"
	action = journeyAnalysisAction(unassessed)
	assert.Equal(t, "Analysis incomplete", action.Status)
	assert.Equal(t, "Retry analysis", action.Label)
	assert.Equal(t, "/reading/books/unassessed/reanalyze", action.URL)
	assert.True(t, action.Submit)

	missingSnapshot := testJourneyBook("missing-snapshot", "Missing snapshot", "analyzed")
	missingSnapshot.Book.Source.MediaType = "application/epub+zip"
	missingSnapshot.Book.Source.ContentRevisionID = "current-revision"
	action = journeyAnalysisAction(missingSnapshot)
	assert.Equal(t, "Assessment unavailable", action.Status)
	assert.Equal(t, "Retry acquisition", action.Label)

	for _, test := range []struct {
		status, wantStatus string
	}{
		{status: "analysis queued", wantStatus: "Analysis queued"},
		{status: "analysis running", wantStatus: "Analysis running"},
	} {
		item := testJourneyBook(test.status, test.status, test.status)
		item.Book.Source.MediaType = "application/epub+zip"
		item.Book.Source.ContentRevisionID = "revision"
		item.Book.Source.ContentSnapshotID = "snapshot"
		item.Book.AnalysisJobID = 42
		queuedAction := journeyAnalysisAction(item)
		assert.Equal(t, test.wantStatus, queuedAction.Status)
		assert.Equal(t, "View analysis status", queuedAction.Label)
		assert.False(t, queuedAction.Submit)
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
	assert.Equal(t, "/reading/books/incomplete/reanalyze", action.URL)
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
