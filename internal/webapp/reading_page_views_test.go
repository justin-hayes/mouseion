package webapp

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readingPageForTest renders the Current reading where it lives now: pinned at
// the top of the merged Reading page.
func readingPageForTest(user domain.User, csrf string, view readingPageView) templ.Component {
	return MyBooksPage(user, csrf, nil, "", "", MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", CurrentReading: view.CurrentReading})
}

func renderReading(t *testing.T, view readingPageView) string {
	t.Helper()
	var output bytes.Buffer
	require.NoError(t, readingPageForTest(domain.User{ID: "owner-1", Username: "learner"}, "csrf-token", view).Render(context.Background(), &output))
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
	html := renderReading(t, readingPageView{CurrentReading: &currentReading})

	assert.Contains(t, html, "Reading since")
	assert.Contains(t, html, `<time datetime="2026-01-03">January 3, 2026</time>`)
	assert.Contains(t, html, "Der lange Weg nach Hause")
	assert.NotContains(t, html, "By A. Reader", "the hero shows the author without a byline")
	assert.Contains(t, html, `<p class="current-hero__author">A. Reader</p>`)
	assert.Contains(t, html, `class="journey-book__lifecycle"`)
	assert.Contains(t, html, "Mark reading finished")
	assert.NotContains(t, html, "Switch current reading", "Read next on Reading replaces Switch")
	assert.Contains(t, html, "End current reading")
	assert.NotContains(t, html, "Set aside this Book")
}

func TestIsCurrentReadingShowsNoReservedVocabularyOrDeck(t *testing.T) {
	currentReading := testReadingBook("goal", "Goal book", testAnalyzed)
	html := renderReading(t, readingPageView{CurrentReading: &currentReading})
	assert.NotContains(t, html, "set aside from vocabulary selection")
	assert.NotContains(t, html, "Reserved vocabulary")
	assert.NotContains(t, html, "Reserved identities")
	assert.NotContains(t, html, "Book deck")
	assert.NotContains(t, html, "/deck/")
	assert.NotContains(t, html, "Prepare deck")
}

func TestIsCurrentReadingShowsReadingStateAndPageAction(t *testing.T) {
	currentReading := testReadingBook("goal-state", "Goal state book", testAnalyzed)
	html := renderReading(t, readingPageView{CurrentReading: &currentReading})
	assert.Contains(t, html, `class="current-hero"`)
	assert.NotContains(t, html, "Book deck</h2>")
}

func TestReadingTreatsAnalyzedEvidenceAndEligibleCurrentReadingsAsCurrent(t *testing.T) {
	item := testReadingBook("analyzed", "Analyzed book", testAnalyzed)
	item.Book.Source.MediaType = "application/epub+zip"
	item.Book.Source.ContentRevisionID = "revision"
	item.Book.Source.ContentSnapshotID = "snapshot"
	item.Book.AnalysisRunID = "run"
	item.Book.CorpusID = "corpus"
	item.Coverage = &domain.AnalysisCoverage{AnalyzableTokenCount: 10}

	assert.Equal(t, domain.CurrentReadingEligible, item.Book.EvidenceClassification().Eligibility, "eligibility is owned by the domain classification")
}
