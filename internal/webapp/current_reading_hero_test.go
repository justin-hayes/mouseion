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

func renderHero(t *testing.T, item readingBookView) string {
	t.Helper()
	var output bytes.Buffer
	require.NoError(t, CurrentReadingHero(item, "csrf-token").Render(context.Background(), &output))
	return output.String()
}

func heroTestBook() readingBookView {
	item := testReadingBook("goal", "Der lange Weg nach Hause", testAnalyzed)
	item.Book.BookAuthor = "A. Reader"
	item.Book.AnalysisRunID = "run"
	item.Book.CorpusID = "corpus"
	item.Cover = domain.BookCover{State: domain.BookCoverAvailable, Width: 200, Height: 300}
	item.ReadingSince = time.Date(2026, time.January, 3, 12, 0, 0, 0, time.UTC)
	item.Coverage = &domain.AnalysisCoverage{AnalyzableTokenCount: 1000, MatureTokenCount: 925, InStudyTokenCount: 950, ToTarget: domain.CoverageThreshold{TargetPercent: 98, LemmaCount: 12, Reachable: true}}
	return item
}

func requireInOrder(t *testing.T, html string, parts ...string) {
	t.Helper()
	last := -1
	for _, part := range parts {
		index := strings.Index(html, part)
		require.GreaterOrEqual(t, index, 0, "missing %q in %s", part, html)
		require.Greater(t, index, last, "%q is out of order in %s", part, html)
		last = index
	}
}

func TestCurrentReadingHeroShowsEveryElementInOrder(t *testing.T) {
	html := renderCurrentReadingSection(t, func() *readingBookView { item := heroTestBook(); return &item }(), "", "", "")

	requireInOrder(t, html,
		`aria-labelledby="current-reading-title"`,
		`class="book-cover-media__image"`,
		`Currently reading`,
		`<h2 id="current-reading-title" class="current-hero__title"><a href="/books/goal">Der lange Weg nach Hause</a></h2>`,
		`<p class="current-hero__author">A. Reader</p>`,
		`Reading since <time datetime="2026-01-03">January 3, 2026</time>`,
		`data-current-reading-mature`,
		`>Open Book</a>`,
		`Mark reading finished`,
		`End current reading`,
	)
	assert.NotContains(t, html, "data-needs-action", "a ready Book has no status line")
	assert.NotContains(t, html, "Analysis details")
	assert.Contains(t, html, `id="journey-book-goal"`, "the hero keeps the Book anchor for focus")
	assert.NotContains(t, html, `aria-label="Current reading"`)
}

func TestCurrentReadingHeroIsAbsentWithoutACurrentReading(t *testing.T) {
	html := renderCurrentReadingSection(t, nil, "", "", "")
	assert.NotContains(t, html, "current-hero")
	assert.NotContains(t, html, "Open Book")
	assert.Contains(t, html, "No current Book yet")
}

func TestCurrentReadingHeroKeepsItsShapeWithoutACover(t *testing.T) {
	item := heroTestBook()
	item.Cover = domain.BookCover{State: domain.BookCoverUnavailable}
	html := renderHero(t, item)
	assert.Contains(t, html, `class="book-cover-media__placeholder"`)
	assert.Contains(t, html, `class="title-plate__title">Der lange Weg nach Hause</span>`)
	assert.NotContains(t, html, "book-cover-media__image")
	requireInOrder(t, html, "current-hero__cover", "Currently reading", "current-hero__title", "Open Book")
}

func TestCurrentReadingHeroStatusLineAppearsOnlyWhenTheBookNeedsAction(t *testing.T) {
	ready := heroTestBook()
	assert.NotContains(t, renderHero(t, ready), "data-needs-action")

	updating := heroTestBook()
	updating.Coverage = nil
	updating.CountsUpdating = true
	assert.NotContains(t, renderHero(t, updating), "data-needs-action", "an updating Book needs nothing from the learner")

	queued := testReadingBook("queued", "Queued", testQueued)
	assert.NotContains(t, renderHero(t, queued), "data-needs-action")

	for _, test := range []struct {
		name  string
		item  func() readingBookView
		label string
	}{
		{"failed analysis", func() readingBookView {
			item := testReadingBook("failed", "Failed", testFailed)
			item.Book.Source.MediaType = "application/epub+zip"
			return item
		}, "Analysis failed."},
		{"stale analysis", func() readingBookView { return testReadingBook("stale", "Stale", testStale) }, "Analysis out of date."},
		{"missing Book language", func() readingBookView {
			item := heroTestBook()
			item.LanguageMissing = true
			return item
		}, "Choose a Book language."},
	} {
		t.Run(test.name, func(t *testing.T) {
			html := renderHero(t, test.item())
			assert.Contains(t, html, `data-needs-action`)
			assert.Contains(t, html, "<strong>"+test.label+"</strong>")
			requireInOrder(t, html, "current-hero__title", "data-needs-action", "current-hero__actions")
		})
	}
}

func TestCurrentReadingHeroNeverOffersAnActionTheServerRejects(t *testing.T) {
	// The server refuses to retry the Current reading's analysis (ADR 0107), so
	// the hero names the failure and where recovery works, with no form.
	failed := testReadingBook("failed", "Failed", testFailed)
	failed.Book.Source.MediaType = "application/epub+zip"
	failed.Book.Source.ContentRevisionID = "revision"
	failed.Book.Source.ContentSnapshotID = "snapshot"
	noContent := testReadingBook("goal", "No content", testNoContent)
	noContent.Book.Source.MediaType = "application/epub+zip"
	noContent.Book.Source.ContentRevisionID = "revision"
	missingStatistics := heroTestBook()
	missingStatistics.StatisticsUnavailable = true

	for _, test := range []struct {
		name   string
		item   readingBookView
		status string
	}{
		{"failed analysis", failed, "<strong>Analysis failed.</strong>"},
		{"stale analysis", testReadingBook("stale", "Stale", testStale), "<strong>Analysis out of date.</strong>"},
		{"unavailable content", noContent, ""},
		{"missing statistics", missingStatistics, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			html := renderHero(t, test.item)
			assert.NotContains(t, html, "Analysis details")
			assert.NotContains(t, html, "reanalyze")
			assert.NotContains(t, html, "Retry")
			if test.status == "" {
				assert.NotContains(t, html, "data-needs-action", "only a failed or stale analysis is a status line")
				return
			}
			assert.Contains(t, html, test.status)
			assert.Contains(t, html, "End this reading to retry it from To Read.")
		})
	}
}

func TestCurrentReadingHeroLinksTheTitleToBookDetailEvenWithoutReadyAnalysis(t *testing.T) {
	html := renderHero(t, testReadingBook("goal", "Not yet", testNoContent))
	assert.Contains(t, html, `<h2 id="current-reading-title" class="current-hero__title"><a href="/books/goal">Not yet</a></h2>`)
	assert.Contains(t, html, `>Open Book</a>`)
}

func TestCurrentReadingHeroShowsAClassicalWorkOnce(t *testing.T) {
	item := heroTestBook()
	item.Book.BookTitle = "Iliad (registered copy)"
	item.Book.BookAuthor = "Homer"
	item.Book.Source.Language = "grc"
	item.ReadingUnit = &domain.BookReadingUnit{WorkTitle: "Iliad", OriginalTitle: "Ἰλιάς"}
	html := renderHero(t, item)
	assert.Equal(t, 1, strings.Count(html, "<article"), "one hero")
	assert.Contains(t, html, `class="current-hero__title"><a href="/books/goal">Iliad</a>`, "the Work title, not the registered copy")
	assert.NotContains(t, html, "registered copy")
	assert.Contains(t, html, `<p class="current-hero__original" lang="grc">Ἰλιάς</p>`)
	requireInOrder(t, html, "current-hero__title", "current-hero__original", "current-hero__author")
}

func TestCurrentReadingHeroStatusLineForClassicalPreparation(t *testing.T) {
	prepared := func(status domain.ClassicalPreparationStatus) readingBookView {
		book := classicalStatusBook(status)
		item := testReadingBook("greek", "Anabasis", testNoContent)
		item.ReadingUnit = book.ReadingUnit
		item.ClassicalPreparation = book.ClassicalPreparation
		return item
	}

	running := renderHero(t, prepared(domain.ClassicalPreparationStatus{State: domain.ClassicalPreparationRunning, CurrentRevision: 1}))
	assert.NotContains(t, running, "data-needs-action", "preparation in progress needs nothing from the learner")

	failed := renderHero(t, prepared(domain.ClassicalPreparationStatus{Stage: domain.BookPreparationStageFetch, CurrentRevision: 1}))
	assert.Contains(t, failed, "data-needs-action")
	assert.Contains(t, failed, "<strong>"+classicalAssessmentUnavailable+".</strong>")
}

func TestCurrentReadingHeroShowsReadyCoverageAsFiguresAndABar(t *testing.T) {
	html := renderHero(t, heroTestBook())
	requireInOrder(t, html,
		`<span class="current-hero__figure numeric" data-current-reading-mature>92.5%</span>`,
		`Mature</span>`,
		`data-current-reading-in-study>95.0%</span>`,
		`With words in study</span>`,
		`role="img" aria-label="Mature 92.5%; with words in study 95.0%"`,
		`style="width:92.500%;"`,
		`style="width:2.500%;"`,
		`925 of 1,000 word occurrences are Mature.`,
		`About 12 more words to reach Target coverage (98%).`,
		`current-hero__actions`,
	)
	assert.Contains(t, html, `<span class="coverage-bar__mature" aria-hidden="true"`)
	assert.NotContains(t, html, "Coverage returns when the counts are ready.")
	assert.NotContains(t, html, "Mature coverage:")
}

func TestCurrentReadingHeroShowsUpdatingAndUnavailableCoverageAsWords(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*readingBookView)
		word string
	}{
		{"updating", func(item *readingBookView) { item.Coverage = nil; item.CountsUpdating = true }, "Updating"},
		{"unavailable", func(item *readingBookView) { item.Coverage = nil }, "Unavailable"},
		{"statistics unavailable", func(item *readingBookView) { item.StatisticsUnavailable = true }, "Unavailable"},
		{"no analyzable words", func(item *readingBookView) { item.Coverage = &domain.AnalysisCoverage{} }, "Unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := heroTestBook()
			test.edit(&item)
			html := renderHero(t, item)
			assert.Equal(t, 2, strings.Count(html, ">"+test.word+"</span>"), "both figures read %q", test.word)
			assert.Contains(t, html, "Coverage returns when the counts are ready.")
			assert.NotContains(t, html, "0.0%")
			assert.NotContains(t, html, "coverage-bar")
			assert.NotContains(t, html, "word occurrences are Mature")
			assert.NotContains(t, html, "data-needs-action", "words in place of figures are not a status line")
		})
	}
}
