package webapp

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMyBooksOffersPreviouslyReadOnlyOnUnstartedToReadBooks(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	ctx := t.Context()
	current, err := store.GetCurrentReading(ctx, fixtures.OwnerID, "de")
	require.NoError(t, err)
	currentRefusal := readingTestRequest(t, h, "/library/books/"+current.BookID+"/previously-read", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, currentRefusal.Code)
	assert.Contains(t, currentRefusal.Header().Get("Location"), "/de/library?error=")
	assert.Contains(t, currentRefusal.Header().Get("Location"), url.QueryEscape(catalogueReadCurrentReadingRefusal))
	require.NoError(t, store.EndCurrentReading(ctx, fixtures.OwnerID, "de", current.BookID, current.SnapshotID))
	reading, err := store.StartCurrentReading(ctx, fixtures.OwnerID, "de", "fixture-route-match")
	require.NoError(t, err)
	require.NoError(t, store.EndCurrentReading(ctx, fixtures.OwnerID, "de", reading.BookID, reading.SnapshotID))

	page := catalogPage(t, h, cookies, "/de/library")
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	started := myBookRowHTML(t, body, "fixture-route-match")
	assert.NotContains(t, started, "Mark as previously read", "a Book that held the current reading is not unstarted")
	unstarted := myBookRowHTML(t, body, "fixture-route-differs")
	assert.Contains(t, unstarted, "Mark as previously read")

	for _, bookID := range []string{"fixture-route-match", "fixture-read-history"} {
		response := readingTestRequest(t, h, "/library/books/"+bookID+"/previously-read", url.Values{"csrf_token": {csrf}}, cookies)
		assert.Equal(t, http.StatusSeeOther, response.Code, bookID)
		assert.Contains(t, response.Header().Get("Location"), "error=", "an ineligible Book gets page feedback: %s", bookID)
	}
	startedDetail, err := store.GetBookDetail(ctx, fixtures.OwnerID, "fixture-route-match")
	require.NoError(t, err)
	assert.Zero(t, startedDetail.CompletionCount, "a refused assertion records no history")
}

type failingPreviouslyReadStore struct{ *fixtures.Store }

func (f failingPreviouslyReadStore) MarkBookPreviouslyRead(context.Context, string, string) error {
	return errors.New("previously read write failed")
}

func TestMyBooksPreviouslyReadWriteFailureIsAServerError(t *testing.T) {
	h, cookies, csrf, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Store.MyBooks = failingPreviouslyReadStore{Store: fixtures.NewStore()}
	})
	response := readingTestRequest(t, h, "/library/books/"+fixtures.BookID+"/previously-read", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusInternalServerError, response.Code)
}

func myBookRowHTML(t *testing.T, body, bookID string) string {
	t.Helper()
	start := strings.Index(body, `id="book-row-`+bookID+`"`)
	require.NotEqual(t, -1, start, "My Books row for %s is missing", bookID)
	row := body[start:]
	end := strings.Index(row, "</li>")
	require.NotEqual(t, -1, end, "My Books row for %s is unterminated", bookID)
	return row[:end]
}

func TestMyBooksMetadataOnlyGridItemExposesOnlySupportedActions(t *testing.T) {
	book := domain.MyBook{Book: domain.Book{ID: "metadata-book", OwnerID: "owner", Title: "A book without an EPUB", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, Disposition: domain.BookDispositionToRead}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.NotContains(t, html, "Reading Journey")
	assert.NotContains(t, html, "Primary Goal")
	if main := strings.Index(html, "<main"); main >= 0 {
		html = html[main:]
	}
	for _, want := range []string{"A book without an EPUB", "No cover available", `aria-hidden="true"`, "More actions", "Remove from My Books", `action="/library/books/metadata-book/remove"`} {
		assert.Contains(t, html, want, "metadata-only My Books item missing %q: %s", want, html)
	}
	assert.NotContains(t, html, "Refresh metadata", "metadata-only My Books item still exposed the retired refresh action: %s", html)
	assert.NotContains(t, html, `href="/books/metadata-book"`, "metadata-only My Books row linked to the retired Book detail page: %s", html)
	row := html
	if start := strings.Index(row, `aria-labelledby="book-title-metadata-book"`); start >= 0 {
		row = row[start:]
		if end := strings.Index(row, "</li>"); end >= 0 {
			row = row[:end+len("</li>")]
		}
	}
	for _, forbidden := range []string{"Not acquired / metadata only", "Review scope", "Prepare deck", "View analysis result", "Start analysis", `action="/books/metadata-book/analyze"`, "coverage", "current reading", "Analysis evidence"} {
		assert.NotContains(t, row, forbidden, "metadata-only My Books item exposed unsupported state or action %q: %s", forbidden, row)
	}
}

func TestMyBookMarginEvidenceUsesOnlyPresentCurrentEvidence(t *testing.T) {
	tests := []struct {
		name string
		book domain.MyBook
		want string
		omit string
	}{
		{name: "metadata only is neutral", book: domain.MyBook{}, omit: "0%"},
		{name: "analysis running", book: domain.MyBook{Acquired: &domain.SourceMaterialSummary{Signals: testRunning}}, want: "Analysis running."},
		{name: "failed", book: domain.MyBook{Acquired: &domain.SourceMaterialSummary{Signals: testFailed}}, want: "Analysis failed."},
		{name: "dependency parsing unavailable", book: domain.MyBook{Acquired: &domain.SourceMaterialSummary{Signals: domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, LatestRun: domain.RunFailed, FailureCode: domain.BookPreparationCodeDependencyParsingUnavailable}}}, want: "Analysis failed because dependency parsing is unavailable."},
		{name: "analysis interrupted", book: domain.MyBook{Acquired: &domain.SourceMaterialSummary{Signals: domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, LatestRun: domain.RunFailed, FailureCode: domain.BookPreparationCodeAnalysisInterrupted}}}, want: "The analysis was interrupted."},
		{name: "cancelled", book: domain.MyBook{Acquired: &domain.SourceMaterialSummary{Signals: testCancelled}}, want: "Analysis cancelled."},
		{name: "stale", book: domain.MyBook{Acquired: &domain.SourceMaterialSummary{Signals: testStale}}, want: "Analysis out of date."},
		{name: "unavailable", book: domain.MyBook{Acquired: &domain.SourceMaterialSummary{Signals: testNoContent}}, want: "Content unavailable."},
		{name: "not analysed", book: domain.MyBook{Acquired: &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ContentRevisionID: "revision", ContentSnapshotID: "snapshot"}, Signals: testNotAnalyzed}}, want: "Not analysed yet."},
		{name: "coverage with next band gap", book: domain.MyBook{Acquired: &domain.SourceMaterialSummary{Signals: testAnalyzed}, CoverageMatureTokens: 974, CoverageTotalTokens: 1000}, want: "97.4% of running words Mature. 16 more words to reach 99%.", omit: "ready"},
		{name: "deck state is distinct from coverage", book: domain.MyBook{IsCurrentReading: true, Acquired: &domain.SourceMaterialSummary{Signals: testAnalyzed}, CoverageTotalTokens: 0, DeckState: "ready", DeckCardCount: 412}, want: "Deck ready: 412 cards.", omit: "0%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := myBookMarginEvidence(tt.book)
			if tt.want != "" {
				assert.Contains(t, got, tt.want)
			}
			if tt.omit != "" {
				assert.NotContains(t, strings.ToLower(got), strings.ToLower(tt.omit))
			}
		})
	}
}

func TestMyBookMarginEvidenceShowsDeckStateOnlyForCurrentReading(t *testing.T) {
	book := domain.MyBook{Acquired: &domain.SourceMaterialSummary{Signals: testAnalyzed}, DeckState: "ready", DeckCardCount: 12}
	assert.NotContains(t, myBookMarginEvidence(book), "Deck")
	book.IsCurrentReading = true
	assert.Contains(t, myBookMarginEvidence(book), "Deck ready: 12 cards.")
	for state, want := range map[string]string{
		"queued":    "Deck preparation running.",
		"preparing": "Deck preparation running.",
		"failed":    "Deck preparation failed.",
		"cancelled": "Deck preparation cancelled.",
	} {
		book.DeckState = state
		assert.Contains(t, myBookMarginEvidence(book), want, state)
	}
}

func TestMyBookMarginEvidenceDatedDeckReadyNamesThePreparationDate(t *testing.T) {
	preparedAt := time.Date(2026, time.September, 9, 0, 0, 0, 0, time.UTC)
	book := domain.MyBook{
		IsCurrentReading: true,
		Acquired:         &domain.SourceMaterialSummary{Signals: testAnalyzed},
		DeckState:        "ready",
		DeckCardCount:    12,
		DeckPreparedAt:   &preparedAt,
	}
	assert.Contains(t, myBookMarginEvidence(book), "Deck ready: 12 cards, prepared 9 September 2026.")
}

func TestMyBookMarginEvidenceShowsLatestCompletionForReadBucket(t *testing.T) {
	completedAt := time.Date(2026, time.August, 2, 0, 0, 0, 0, time.UTC)
	book := domain.MyBook{
		Disposition:        domain.BookDispositionInbox,
		CompletionCount:    1,
		LatestCompletionAt: &completedAt,
	}
	assert.Equal(t, "Finished 2 Aug 2026.", myBookMarginEvidence(book))
}

func TestReanalysisMarginNoteNamesTheFailureReason(t *testing.T) {
	shadowed := func(code domain.BookPreparationCode) domain.BookEvidenceClassification {
		return domain.BookEvidenceClassification{Phase: domain.PhaseAnalyzed, Run: domain.RunFailed, FailureCode: code}
	}
	assert.Equal(t, "Re-analysis failed. Dependency parsing is unavailable.", reanalysisMarginNote(shadowed(domain.BookPreparationCodeDependencyParsingUnavailable)))
	assert.Equal(t, "Re-analysis failed. The analysis was interrupted.", reanalysisMarginNote(shadowed(domain.BookPreparationCodeAnalysisInterrupted)))
	assert.Equal(t, "Re-analysis failed.", reanalysisMarginNote(shadowed(domain.BookPreparationCodeAnalysisFailed)))
}

func TestMyBookEvidenceRecoveryHonorsToReadAndCurrentReadingPolicy(t *testing.T) {
	book := domain.MyBook{
		Disposition: domain.BookDispositionToRead,
		Acquired: &domain.SourceMaterialSummary{
			Signals: testFailed,
			Source:  domain.SourceMaterial{ContentRevisionID: "revision", ContentSnapshotID: "snapshot"},
		},
	}
	assert.Equal(t, "Retry analysis", myBookEvidenceRecovery(book))
	book.Disposition = domain.BookDispositionInbox
	book.CompletionCount = 1
	assert.Equal(t, "Retry analysis by reading again", myBookEvidenceRecovery(book), "failed evidence on a Read Book retains a valid recovery action")
	assert.Equal(t, "/library/books/book/read-again", myBookEvidenceRecoveryURL(domain.MyBook{Book: domain.Book{ID: "book"}, Disposition: domain.BookDispositionInbox, CompletionCount: 1}))
	book.Disposition = domain.BookDispositionToRead
	book.IsCurrentReading = true
	assert.Empty(t, myBookEvidenceRecovery(book), "current reading rejects its own reanalysis POST")
	book.IsCurrentReading = false
	book.Acquired.Signals = testCancelled
	assert.Equal(t, "Retry analysis", myBookEvidenceRecovery(book))
	book.Acquired.Signals = testNotAnalyzed
	book.Acquired.Signals = testNoContent
	assert.Equal(t, "Retry acquisition", myBookEvidenceRecovery(book), "either missing content identity makes content unavailable")
}

func TestMyBooksReadFailureOffersReadAgainRecovery(t *testing.T) {
	book := domain.MyBook{
		Book:            domain.Book{ID: "read-failed", OwnerID: "owner", Title: "Read failure"},
		Disposition:     domain.BookDispositionInbox,
		CompletionCount: 1,
		Acquired: &domain.SourceMaterialSummary{
			Source:  domain.SourceMaterial{ContentRevisionID: "revision", ContentSnapshotID: "snapshot"},
			Signals: testFailed,
		},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, "Analysis failed.")
	assert.Contains(t, html, "Retry analysis by reading again")
	assert.Contains(t, html, `action="/library/books/read-failed/read-again"`)
	assert.Contains(t, html, `name="expected_revision" value="0"`)
}

func TestMyBooksRemoveActionIsRefusedOnTheCurrentReading(t *testing.T) {
	current := domain.MyBook{Book: domain.Book{ID: "current-book", OwnerID: "owner", Title: "Current book"}, IsCurrentReading: true}
	other := domain.MyBook{Book: domain.Book{ID: "other-book", OwnerID: "owner", Title: "Other book"}, Disposition: domain.BookDispositionToRead}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{current, other}, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, "Remove from My Books")
	assert.Contains(t, html, "Your current reading cannot be removed. Finish or end the reading first.")
	assert.Contains(t, html, `action="/library/books/other-book/remove"`)
	assert.NotContains(t, html, `action="/library/books/current-book/remove"`, "the current Book exposes no Remove form")
}

func TestMyBooksReadingActionHidesAddForExistingMember(t *testing.T) {
	book := domain.MyBook{
		Book:     domain.Book{ID: "journey-book", OwnerID: "owner", Title: "Journey book"},
		IsToRead: true,
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.False(t, strings.Contains(html, `action="/reading/books/journey-book/to-read"`) || strings.Contains(html, "Move to To Read"), "existing To Read book still exposed add action: %s", html)
	assert.Contains(t, html, "To Read")
	assert.Contains(t, html, `<span class="status-badge status-badge--neutral"><span class="status-badge__shape status-badge__shape--ring" aria-hidden="true"></span>To Read</span>`)
	assert.NotContains(t, html, `<span aria-hidden="true">●</span>`)
	assert.Contains(t, html, "View in Reading")
	assert.Contains(t, html, `href="/reading#journey-book-journey-book"`)
}

func TestMyBooksReadingActionHidesAddForCurrentReading(t *testing.T) {
	book := domain.MyBook{
		Book:             domain.Book{ID: "goal-book", OwnerID: "owner", Title: "Goal book"},
		IsCurrentReading: true,
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.False(t, strings.Contains(html, `action="/reading/books/goal-book/to-read"`) || strings.Contains(html, "Move to To Read"), "current reading still exposed add action: %s", html)
	assert.Contains(t, html, "Currently reading")
	assert.Contains(t, html, `<span class="status-badge status-badge--info"><span class="status-badge__shape status-badge__shape--dot" aria-hidden="true"></span>Currently reading</span>`)
	assert.Contains(t, html, `href="/reading#journey-book-goal-book"`)
	assert.NotContains(t, html, "To Read")
}

func TestAnalyzedMyBookShowsCurrentResultWithoutDuplicateStartAction(t *testing.T) {
	book := domain.MyBook{
		Book:     domain.Book{ID: "analyzed-book", OwnerID: "owner", Title: "Analyzed book"},
		IsToRead: true,
		Acquired: &domain.SourceMaterialSummary{
			Source:        domain.SourceMaterial{ID: "source-analyzed-book", MediaType: "application/epub+zip", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"},
			Signals:       testAnalyzed,
			AnalysisRunID: "run-analyzed-book",
			CorpusID:      "corpus-analyzed-book",
		},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.True(t, strings.Contains(html, `href="/reading#journey-book-analyzed-book"`) && !strings.Contains(html, `href="/books/analyzed-book"`) && !strings.Contains(html, "View analysis result") && !strings.Contains(html, `action="/books/analyzed-book/analyze"`), "analyzed Journey member exposed an invalid My Books action or link: %s", html)
	assert.NotContains(t, html, "Analysis evidence")
}

func TestAnalyzedNonReadingMyBookKeepsEvidenceWithoutLink(t *testing.T) {
	book := domain.MyBook{
		Book: domain.Book{ID: "analyzed-outside-journey", OwnerID: "owner", Title: "Analyzed outside Journey"},
		Acquired: &domain.SourceMaterialSummary{
			Source:        domain.SourceMaterial{ID: "source-analyzed-outside-journey", MediaType: "application/epub+zip", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"},
			Signals:       testAnalyzed,
			AnalysisRunID: "run-analyzed-outside-journey",
			CorpusID:      "corpus-analyzed-outside-journey",
		},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.False(t, strings.Contains(html, "Analysis evidence") || strings.Contains(html, `href="/reading/analyzed-outside-journey"`) || strings.Contains(html, `href="/books/`) || strings.Contains(html, "View analysis result"), "analyzed non-member My Books item exposed an invalid link or state: %s", html)
}

func TestMyBooksRowRendersCanonicalBookTitle(t *testing.T) {
	book := domain.MyBook{
		Book: domain.Book{ID: "canonical-book", OwnerID: "owner", Title: "Refreshed catalogue title", Author: "Catalogue author"},
		Acquired: &domain.SourceMaterialSummary{
			Source: domain.SourceMaterial{ID: "source-canonical-book", Title: "Acquisition-internal title"},
		},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", []domain.MyBook{book}, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	assert.Contains(t, html, "Refreshed catalogue title", "My Books omitted canonical Book title: %s", html)
	assert.Contains(t, html, "By Catalogue author")
	assert.NotContains(t, html, "Acquisition-internal title", "My Books rendered acquisition-internal title: %s", html)
}

func TestMyBooksOmitsEvidenceAndAcquisitionState(t *testing.T) {
	states := []domain.BookEvidenceState{
		domain.BookUnavailable,
		domain.BookNotAcquired,
		domain.BookAcquiredUnassessed,
		domain.BookAnalyzed,
		domain.BookStale,
	}
	books := make([]domain.MyBook, 0, len(states))
	for i, state := range states {
		book := domain.MyBook{Book: domain.Book{ID: "book-" + string(rune('a'+i)), OwnerID: "owner", Title: "Book " + string(rune('A'+i)), LanguageState: domain.LanguageChosen, LanguageTag: "de"}}
		if state != domain.BookNotAcquired {
			signals := testNotAnalyzed
			revision := "revision"
			if state == domain.BookUnavailable {
				revision = ""
				signals = testNoContent
			}
			if state == domain.BookAnalyzed {
				signals = testAnalyzed
			}
			if state == domain.BookStale {
				signals = testStale
			}
			book.Acquired = &domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "source-" + book.Book.ID, OwnerID: "owner", Title: book.Book.Title, Language: "de", MediaType: "application/epub+zip", ContentRevisionID: revision, ContentSnapshotID: "snapshot"}, BookID: book.Book.ID, Signals: signals}
		}
		books = append(books, book)
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	for _, label := range []string{"Unavailable", "Not acquired", "Ready to analyze", "Analysis not started", "Analyzed", "Stale analysis", "current reading", "Next action", "Analysis evidence"} {
		assert.NotContains(t, html, label)
	}
	assert.NotContains(t, html, `href="/books/`, "learner My Books linked to the retired Book detail page: %s", html)
	assert.False(t, strings.Contains(html, "Start analysis") || strings.Contains(html, `/analyze`), "My Books rows exposed an explicit analysis action: %s", html)
}

func TestMyBooksListUsesNativeListAndStablePresentationStates(t *testing.T) {
	books := []domain.MyBook{
		{Book: domain.Book{ID: "available", OwnerID: "owner", Title: "A very long title that remains fully visible", Author: "An author with a name long enough to wrap in a narrow grid item"}, Cover: domain.BookCover{State: domain.BookCoverAvailable, Width: 600, Height: 900}, IsToRead: true},
		{Book: domain.Book{ID: "pending", OwnerID: "owner", Title: "Pending cover"}, Cover: domain.BookCover{State: domain.BookCoverPending}},
		{Book: domain.Book{ID: "unavailable", OwnerID: "owner", Title: "Unavailable cover"}, Cover: domain.BookCover{State: domain.BookCoverUnavailable}},
		{Book: domain.Book{ID: "none", OwnerID: "owner", Title: "No advertised cover"}},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()

	assert.Contains(t, html, `<ul class="library-books" role="list">`)
	assert.NotContains(t, html, `class="resource-card library-book"`)
	assert.Equal(t, 4, strings.Count(html, `class="library-book"`))
	assert.NotContains(t, html, `role="grid"`)
	assert.NotContains(t, html, `aria-rowindex`)
	for _, label := range []string{"Cover pending", "Cover unavailable", "No cover available"} {
		assert.Contains(t, html, label)
	}
	assert.Contains(t, html, `src="/books/available/cover"`)
	assert.Contains(t, html, `width="600" height="900" loading="lazy"`)
	assert.Contains(t, html, "An author with a name long enough to wrap in a narrow grid item")
	for _, forbidden := range []string{"Analysis evidence", "Not acquired", "current reading", "Next action", "Ready to analyze"} {
		assert.NotContains(t, html, forbidden)
	}

	start := strings.Index(html, `id="book-row-available"`)
	require.GreaterOrEqual(t, start, 0)
	end := strings.Index(html[start:], "</li>")
	require.Positive(t, end)
	item := html[start : start+end]
	for _, content := range []string{`<a class="library-book__identity-link"`, "A very long title that remains fully visible", "By An author", "To Read", "View in Reading", "More actions"} {
		assert.Contains(t, item, content)
	}
	assert.Equal(t, 1, strings.Count(item, `class="library-book__identity-link"`), "cover and title must share one identity link")
	assert.Less(t, strings.Index(item, `class="book-cover-media"`), strings.Index(item, "A very long title"))
	assert.Less(t, strings.Index(item, "A very long title"), strings.Index(item, "By An author"))
	assert.Less(t, strings.Index(item, "By An author"), strings.Index(item, "To Read"))
	assert.Less(t, strings.Index(item, "To Read"), strings.Index(item, "View in Reading"))
	assert.Less(t, strings.Index(item, "View in Reading"), strings.Index(item, "More actions"))
}

func TestMyBooksBrowseResultsOmitUnfilteredCountAndRedundantHeading(t *testing.T) {
	state := MyBooksBrowseState{Enabled: true, Language: "de", LanguageLabel: "German", AllCount: 2, ScopeTotal: 2, Total: 2}
	books := []domain.MyBook{
		{Book: domain.Book{ID: "book-one", OwnerID: "owner", Title: "One", LanguageTag: "de", LanguageState: domain.LanguageChosen}},
		{Book: domain.Book{ID: "book-two", OwnerID: "owner", Title: "Two", LanguageTag: "de", LanguageState: domain.LanguageChosen}},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", state).Render(context.Background(), &output))
	html := output.String()
	resultsStart := strings.Index(html, `<section id="library-results"`)
	require.GreaterOrEqual(t, resultsStart, 0)
	results := html[resultsStart:]
	assert.NotContains(t, results, "books found", "unfiltered browsing does not need a redundant result count")
	assert.NotContains(t, results, `<h2 id="library-books-heading"`)
	assert.Contains(t, results, `aria-label="My Books results"`)

	state.Query = "missing"
	state.Total = 0
	state.TextNoMatch = true
	output.Reset()
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", state).Render(context.Background(), &output))
	assert.Contains(t, output.String(), "No To Read books in German match")
}

func TestMyBooksRowsShowCanonicalBucketStatusInSharedBadgeFormat(t *testing.T) {
	books := []domain.MyBook{
		{Book: domain.Book{ID: "current", OwnerID: "owner", Title: "Current"}, IsCurrentReading: true},
		{Book: domain.Book{ID: "to-read", OwnerID: "owner", Title: "To Read"}, Disposition: domain.BookDispositionToRead},
		{Book: domain.Book{ID: "read", OwnerID: "owner", Title: "Read"}, Disposition: domain.BookDispositionInbox, CompletionCount: 1},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	for _, label := range []string{"Currently reading", "To Read", "Read"} {
		assert.Contains(t, html, ">"+label+"</span>")
	}
	assert.NotContains(t, html, "Set Aside")
	assert.Contains(t, html, `status-badge__shape--dot`)
	assert.Contains(t, html, `status-badge__shape--ring`)
	assert.NotContains(t, html, "Disposition:")
	assert.NotContains(t, html, `<span aria-hidden="true">●</span>`)
}

func TestMyBooksEmptyOnboardingGuidesConnectionLanguageAndSync(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{"Find your first Book", "My Books holds the Books you choose", `href="/catalogs">Browse the Catalog</a>`} {
		assert.Contains(t, html, want, "empty My Books onboarding missing %q: %s", want, html)
	}
	assert.False(t, strings.Contains(html, "Add books") || strings.Contains(html, "Add a book"), "empty My Books onboarding exposed retired acquisition wording: %s", html)
}

func TestMyBooksEmptyAncientGreekStateLinksToTheCatalog(t *testing.T) {
	var output bytes.Buffer
	browse := MyBooksBrowseState{Enabled: true, Language: "grc", LanguageLabel: "Ancient Greek", AllCount: 0}
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", nil, "", "", browse).Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{"Find your first Book", `href="/catalogs">Browse the Catalog</a>`} {
		assert.Contains(t, html, want, "empty Ancient Greek My Books state missing %q: %s", want, html)
	}
}

func TestUpstreamBrowserRoutesAreRetired(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	for _, route := range []string{"/catalog", "/opds/browse", "/opds/language", "/opds/search", "/library/books/book-id"} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, route, nil)
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		h.ServeHTTP(response, r)
		assert.Equal(t, http.StatusNotFound, response.Code, route)
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/library/books/book-id", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)
}

func TestUnassessedBookDetailAndStandaloneAnalysisRoutesAreRetired(t *testing.T) {
	h, cookies, csrf, _ := readingFixtureSession(t)
	for _, path := range []string{"/books/fixture-empty", "/books/fixture-book", "/books/fixture-failed"} {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		assert.Equal(t, http.StatusNotFound, response.Code, path)
	}
	assert.Equal(t, http.StatusNotFound, readingTestRequest(t, h, "/books/fixture-empty/analyze", url.Values{"csrf_token": {csrf}}, cookies).Code)
	assert.Equal(t, http.StatusNotFound, readingTestRequest(t, h, "/books/fixture-metadata-only/refresh", url.Values{"csrf_token": {csrf}}, cookies).Code)
	assert.Equal(t, http.StatusNotFound, readingTestRequest(t, h, "/books/fixture-book/deck/preparations", url.Values{"csrf_token": {csrf}}, cookies).Code)
}

func TestCompletedAnalysisCompatibilityRouteRedirectsToReadingEntry(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/books/fixture-book/analyses/fixture-run", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/de/reading#journey-book-fixture-book", response.Header().Get("Location"))
	request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/books/fixture-book/analyses/old-run", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusNotFound, response.Code)
}

func TestRetiredMetadataRefreshRouteIsUnsupported(t *testing.T) {
	h, cookies, csrf, _ := readingFixtureSession(t)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request := httptest.NewRequestWithContext(t.Context(), method, "/library/books/fixture-metadata-only/refresh", strings.NewReader(url.Values{"csrf_token": {csrf}}.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		assert.Equal(t, http.StatusNotFound, response.Code, "retired refresh route answered %s with %d", method, response.Code)
	}
}

func TestMyBookMarginEvidenceShowsRegisteredReadingUnit(t *testing.T) {
	t.Parallel()
	registered := domain.MyBook{
		Book:        domain.Book{ID: "greek", Title: "Anabasis", Author: "Xenophon", LanguageState: domain.LanguageChosen, LanguageTag: "grc"},
		ReadingUnit: &domain.BookReadingUnit{EditionLabel: "Marchant 1904"},
	}
	assert.Equal(t, notPreparedYetNote, myBookMarginEvidence(registered), "unprepared registered Book is called out")
	assert.Equal(t, "Marchant 1904", myBookEditionLabel(registered))
	assert.Empty(t, myBookEditionLabel(domain.MyBook{Book: domain.Book{ID: "ordinary", Title: "Ordinary"}}), "ordinary Book shows no edition")
	assert.Empty(t, myBookMarginEvidence(domain.MyBook{Book: domain.Book{ID: "ordinary", Title: "Ordinary"}}), "ordinary metadata-only Book stays neutral")
}

func TestMyBookRowRendersRegisteredEditionAndPreparation(t *testing.T) {
	t.Parallel()
	book := domain.MyBook{
		Book:        domain.Book{ID: "greek", Title: "Anabasis", Author: "Xenophon", LanguageState: domain.LanguageChosen, LanguageTag: "grc"},
		ReadingUnit: &domain.BookReadingUnit{EditionLabel: "Marchant 1904"},
	}
	var output bytes.Buffer
	require.NoError(t, MyBookRow("csrf", book, "").Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{"Anabasis", "By Xenophon", "Marchant 1904", notPreparedYetNote} {
		assert.Contains(t, html, want, "registered My Books row missing %q: %s", want, html)
	}
}

func TestMyBooksTellsTwoEditionsOfOneWorkApartAsOrdinaryRows(t *testing.T) {
	edition := func(id, label string) domain.MyBook {
		return domain.MyBook{
			Book: domain.Book{ID: id, OwnerID: "owner", Title: "Anabasis", LanguageState: domain.LanguageChosen, LanguageTag: "grc"},
			ReadingUnit: &domain.BookReadingUnit{
				WorkKey: "xenophon-anabasis", WorkTitle: "Anabasis", WorkAuthor: "Xenophon", EditionLabel: label,
			},
			Disposition: domain.BookDispositionToRead,
		}
	}
	books := []domain.MyBook{edition("edition-a", "Perseus edition"), edition("edition-b", "Oxford edition")}
	state := MyBooksBrowseState{Enabled: true, Language: "grc", AllCount: 2, ScopeTotal: 2, Total: 2, Disposition: domain.BookDispositionToRead, Page: 1}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", state).Render(context.Background(), &output))
	body := output.String()

	first := myBookRowHTML(t, body, "edition-a")
	second := myBookRowHTML(t, body, "edition-b")
	t.Run("each row names its own Edition", func(t *testing.T) {
		require.Contains(t, first, "Perseus edition")
		require.NotContains(t, first, "Oxford edition")
		require.Contains(t, second, "Oxford edition")
	})
	t.Run("each row has its own Remove", func(t *testing.T) {
		require.Contains(t, first, `action="/library/books/edition-a/remove"`)
		require.Contains(t, second, `action="/library/books/edition-b/remove"`)
	})
	t.Run("no grouped Work surface", func(t *testing.T) {
		require.NotContains(t, body, "library-work")
		require.NotContains(t, body, "Books in")
	})
}
