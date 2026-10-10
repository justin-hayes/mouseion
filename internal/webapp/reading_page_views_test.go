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

func renderReading(t *testing.T, view readingPageView, message, pageError string) string {
	t.Helper()
	var output bytes.Buffer
	require.NoError(t, ReadingPage(domain.User{ID: "owner-1", Username: "learner"}, "csrf-token", view, message, pageError).Render(context.Background(), &output))
	return output.String()
}

func testReadingBook(id, title string, signals domain.AnalysisSignals) readingBookView {
	return readingBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: id, Title: title, Language: "de"}, Signals: signals}}
}

func TestReadingPageIdentifiesCurrentReadingSinceAndKeepsLifecycleActionsWithBook(t *testing.T) {
	currentReading := testReadingBook("current", "Der lange Weg nach Hause", testNoContent)
	currentReading.Book.BookAuthor = "A. Reader"
	currentReading.Cover = domain.BookCover{State: domain.BookCoverUnavailable}
	currentReading.ReadingSince = time.Date(2026, time.January, 3, 12, 0, 0, 0, time.UTC)
	html := renderReading(t, readingPageView{CurrentReading: &currentReading}, "", "")

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
	currentReading := testReadingBook("goal", "Goal book", testAnalyzed)
	currentReading.CurrentReadingSnapshotSize = 2
	currentReading.CurrentReadingSnapshotID = "snapshot"
	currentReading.CurrentReadingVocabularyEligible = 1
	currentReading.CurrentReadingPreparation = &domain.DeckPreparation{ID: "goal-preparation", SnapshotID: "snapshot", State: domain.DeckPreparationFailed, FailureClass: "provider"}
	html := renderReading(t, readingPageView{CurrentReading: &currentReading}, "", "")
	assert.Contains(t, html, "2 lemmas are set aside from vocabulary selection while you read this Book.")
	assert.Contains(t, html, "1 currently eligible frozen Reserved identities")
	assert.Contains(t, html, "Book deck")
	assert.Contains(t, html, "Retry deck preparation")
	assert.Contains(t, html, `action="/reading/books/goal/deck/retry"`)
	assert.Contains(t, html, `name="expected_current_snapshot_id" value="snapshot"`)
	assert.NotContains(t, html, `action="/deck-preparations/goal-preparation/retry"`)
	currentReadingCardStart := strings.Index(html, `id="journey-book-goal"`)
	currentReadingCardEnd := strings.Index(html[currentReadingCardStart:], "</article>")
	require.GreaterOrEqual(t, currentReadingCardStart, 0)
	require.Greater(t, currentReadingCardEnd, 0)
	assert.NotContains(t, html[currentReadingCardStart:currentReadingCardStart+currentReadingCardEnd], "external_translation_consent")
}

func TestIsCurrentReadingRendersEmptyReservedVocabularyCount(t *testing.T) {
	currentReading := testReadingBook("empty-goal", "Empty Goal book", testAnalyzed)
	html := renderReading(t, readingPageView{CurrentReading: &currentReading}, "", "")
	assert.Contains(t, html, "0 lemmas are set aside from vocabulary selection while you read this Book.")
	assert.NotContains(t, html, "Deck preparation unavailable")
	assert.NotContains(t, html, "Retry deck preparation")
}

func TestIsCurrentReadingShowsReservedVocabularyWhenDeckIsUnavailable(t *testing.T) {
	currentReading := testReadingBook("unavailable-goal", "Unavailable Goal book", testAnalyzed)
	currentReading.CurrentReadingSnapshotSize = 2
	currentReading.CurrentReadingDeckUnavailable = true
	html := renderReading(t, readingPageView{CurrentReading: &currentReading}, "", "")
	assert.Contains(t, html, "2 lemmas are set aside from vocabulary selection while you read this Book.")
	assert.Contains(t, html, "Deck unavailable.")
	assert.Contains(t, html, "Retry deck preparation")
}

func TestIsCurrentReadingShowsMissingDeckWithoutChangingCurrentReadingFacts(t *testing.T) {
	currentReading := testReadingBook("missing-goal", "Missing Goal deck", testAnalyzed)
	currentReading.CurrentReadingSnapshotSize = 2
	currentReading.CurrentReadingSnapshotID = "missing-snapshot"
	currentReading.CurrentReadingDeckMissing = true
	html := renderReading(t, readingPageView{CurrentReading: &currentReading}, "", "")
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
	currentReading := testReadingBook("goal-state", "Goal state book", testAnalyzed)
	html := renderReading(t, readingPageView{CurrentReading: &currentReading}, "", "")
	assert.Contains(t, html, "Reserved vocabulary</h2>")
	assert.Contains(t, html, "Analysis</h2>")
	assert.Contains(t, html, "Book deck</h2>")
	assert.Contains(t, html, `href="/library">Add books from My Books</a>`)
}

func TestReadingEvidenceActionsRemainAvailable(t *testing.T) {
	stale := testReadingBook("stale", "Stale book", testStale)
	stale.Book.Source.MediaType = "application/epub+zip"
	stale.Book.Source.ContentRevisionID = "current-revision"
	stale.Book.Source.ContentSnapshotID = "current-snapshot"
	stale.Book.AnalysisRunID = "old-run"
	stale.Book.CorpusID = "old-corpus"
	action := readingAnalysisAction(stale)
	assert.Equal(t, "Stale analysis", action.Status)
	assert.Equal(t, "Re-analyze", action.Label)
	assert.Equal(t, "/reading/books/stale/reanalyze", action.URL)
	assert.True(t, action.Submit)
	unassessed := testReadingBook("unassessed", "Unassessed book", testNotAnalyzed)
	unassessed.Book.Source.MediaType = "application/epub+zip"
	unassessed.Book.Source.ContentRevisionID = "revision"
	unassessed.Book.Source.ContentSnapshotID = "snapshot"
	action = readingAnalysisAction(unassessed)
	assert.Equal(t, "Analysis incomplete", action.Status)
	assert.Equal(t, "Retry analysis", action.Label)
	assert.Equal(t, "/reading/books/unassessed/reanalyze", action.URL)
	assert.True(t, action.Submit)

	missingSnapshot := testReadingBook("missing-snapshot", "Missing snapshot", testNoContent)
	missingSnapshot.Book.Source.MediaType = "application/epub+zip"
	missingSnapshot.Book.Source.ContentRevisionID = "current-revision"
	action = readingAnalysisAction(missingSnapshot)
	assert.Equal(t, "Assessment unavailable", action.Status)
	assert.Equal(t, "Retry acquisition", action.Label)

	for _, test := range []struct {
		name, wantStatus string
		signals          domain.AnalysisSignals
	}{
		{name: "analysis queued", signals: testQueued, wantStatus: "Analysis queued"},
		{name: "analysis running", signals: testRunning, wantStatus: "Analysis running"},
	} {
		item := testReadingBook(test.name, test.name, test.signals)
		item.Book.Source.MediaType = "application/epub+zip"
		item.Book.Source.ContentRevisionID = "revision"
		item.Book.Source.ContentSnapshotID = "snapshot"
		item.Book.AnalysisJobID = 42
		queuedAction := readingAnalysisAction(item)
		assert.Equal(t, test.wantStatus, queuedAction.Status)
		assert.Equal(t, "View analysis status", queuedAction.Label)
		assert.False(t, queuedAction.Submit)
	}
}

func TestReadingIncompleteAnalyzedEvidenceOffersReanalysis(t *testing.T) {
	item := testReadingBook("incomplete", "Incomplete analyzed book", testAnalyzed)
	item.Book.Source.MediaType = "application/epub+zip"
	item.Book.Source.ContentRevisionID = "revision"
	item.Book.Source.ContentSnapshotID = "snapshot"
	item.Book.AnalysisRunID = "run"
	item.Book.CorpusID = "corpus"
	item.StatisticsUnavailable = true

	action := readingAnalysisAction(item)
	assert.Equal(t, "Analysis incomplete", action.Status)
	assert.Equal(t, "Retry analysis", action.Label)
	assert.Equal(t, "/reading/books/incomplete/reanalyze", action.URL)
	assert.True(t, action.Submit)
}

func TestReadingTreatsAnalyzedEvidenceAndEligibleCurrentReadingsAsCurrent(t *testing.T) {
	item := testReadingBook("analyzed", "Analyzed book", testAnalyzed)
	item.Book.Source.MediaType = "application/epub+zip"
	item.Book.Source.ContentRevisionID = "revision"
	item.Book.Source.ContentSnapshotID = "snapshot"
	item.Book.AnalysisRunID = "run"
	item.Book.CorpusID = "corpus"
	item.Coverage = &domain.AnalysisCoverage{AnalyzableTokenCount: 10}

	assert.Equal(t, readingEvidenceCurrent, readingEvidenceState(item))
	assert.True(t, item.Book.EvidenceClassification().Eligibility == domain.CurrentReadingEligible, "eligibility is owned by the domain classification")
}
