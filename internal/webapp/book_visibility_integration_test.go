//go:build integration

package webapp

import (
	"context"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// visibilityDiscoveryReader offers one German EPUB through the catalogue, so
// the real sync worker discovers a brand-new Book.
type visibilityDiscoveryReader struct{}

func (visibilityDiscoveryReader) Languages(context.Context, string, string) (opds.Feed, error) {
	return opds.Feed{Entries: []opds.Entry{
		{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://discovery.example/opds/language/7"}}},
	}}, nil
}

func (visibilityDiscoveryReader) BrowseLanguage(_ context.Context, _, _, languageID string) (opds.Feed, error) {
	if languageID != "7" {
		return opds.Feed{}, nil
	}
	return opds.Feed{Entries: []opds.Entry{{
		ID: "discovered-entry", Title: "Discovered visible", Author: "Discovery author",
		Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "https://discovery.example/opds/epub/discovered-entry"}},
	}}}, nil
}

func (reader visibilityDiscoveryReader) BrowseLanguageUnfiltered(ctx context.Context, owner, connection, language string) (opds.Feed, error) {
	return reader.BrowseLanguage(ctx, owner, connection, language)
}

func openVisibilityStore(t *testing.T, ctx context.Context) *persistence.PostgresStore {
	t.Helper()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))
	return store
}

func visibilityHandler(store *persistence.PostgresStore) http.Handler {
	authService := auth.New(store, time.Hour)
	return New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), nil), AnalysisInsights: analysisinsights.NewService(store), PreparedDeck: prepareddeck.NewService(store, nil), SessionLifetime: time.Hour,
	})
}

// seedVisibilityToReadBook creates an analyzed German To Read Book that the
// Reading chooser can assess and start.
func seedVisibilityToReadBook(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, suffix, title, lemma string) domain.Book {
	t.Helper()
	counts := []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: lemma, UPOS: "NOUN", OccurrenceCount: 3}}
	book, _, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner, suffix, title, counts)
	publishProjectedCounts(t, ctx, store, owner, book.ID, counts)
	_, err := store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,'de',$3,'NOUN',3,'[]','[]','{}')`, owner, corpus.ID, lemma)
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner, book.ID, domain.BookDispositionToRead))
	return book
}

func visibilityRevision(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, bookID string) string {
	t.Helper()
	_, revision, err := store.GetBookVisibility(ctx, owner, bookID)
	require.NoError(t, err)
	return strconv.FormatInt(revision, 10)
}

func TestHiddenToReadBookLeavesAndReturnsToReadingChooser(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "hidden-chooser-visibility-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store := openVisibilityStore(t, ctx)
	owner := createAccount(t, ctx, store, "hidden-chooser", "learner-password", false)
	hidden := seedVisibilityToReadBook(t, ctx, store, owner.ID, "chooser-hidden", "Chooser hidden", "haus")
	visible := seedVisibilityToReadBook(t, ctx, store, owner.ID, "chooser-visible", "Chooser visible", "baum")
	h := visibilityHandler(store)
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")
	startPath := func(bookID string) string { return "/reading/books/" + bookID + "/start" }

	before := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, before.Code)
	require.Contains(t, before.Body.String(), startPath(hidden.ID), "the Book starts visible in the chooser")

	hide := perform(t, h, http.MethodPost, "/library/books/"+hidden.ID+"/hide", url.Values{
		"csrf_token": {csrf}, "expected_visibility_revision": {visibilityRevision(t, ctx, store, owner.ID, hidden.ID)}, "return_to": {"/reading"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, hide.Code)
	hiddenChooser := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	assert.NotContains(t, hiddenChooser.Body.String(), startPath(hidden.ID), "a Hidden To Read Book is omitted from the chooser")
	assert.NotContains(t, hiddenChooser.Body.String(), hidden.Title)
	assert.Contains(t, hiddenChooser.Body.String(), startPath(visible.ID), "other To Read Books remain in the chooser")

	unhide := perform(t, h, http.MethodPost, "/library/books/"+hidden.ID+"/unhide", url.Values{
		"csrf_token": {csrf}, "expected_visibility_revision": {visibilityRevision(t, ctx, store, owner.ID, hidden.ID)}, "return_to": {"/reading"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, unhide.Code)
	restored := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	assert.Contains(t, restored.Body.String(), startPath(hidden.ID), "Unhide returns the Book to the chooser")
	assert.Contains(t, restored.Body.String(), hidden.Title)

	start := perform(t, h, http.MethodPost, startPath(hidden.ID), url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, start.Code)
	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, hidden.ID, current.BookID, "the unhidden Book can be started")
}

func TestHiddenCurrentReadingStaysInReadingWithItsReservations(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "hidden-current-reading-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store := openVisibilityStore(t, ctx)
	owner := createAccount(t, ctx, store, "hidden-current", "learner-password", false)
	book := seedVisibilityToReadBook(t, ctx, store, owner.ID, "hidden-current", "Working desk hidden", "haus")
	h := visibilityHandler(store)
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")

	start := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, start.Code)
	before, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.Equal(t, book.ID, before.BookID)

	hide := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/hide", url.Values{
		"csrf_token": {csrf}, "expected_visibility_revision": {visibilityRevision(t, ctx, store, owner.ID, book.ID)}, "return_to": {"/reading"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, hide.Code)
	hidden, _, err := store.GetBookVisibility(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	require.True(t, hidden)

	page := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), `class="journey-book__title"`, "the Hidden current reading still renders as the Reading working desk")
	assert.Contains(t, page.Body.String(), book.Title)
	assert.NotContains(t, page.Body.String(), "reading-chooser-entry", "the working desk is not the chooser")

	after, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, before, after, "Hiding does not end the current reading")
	var released bool
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT released_at IS NOT NULL FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2`, owner.ID, before.SnapshotID).Scan(&released))
	assert.False(t, released, "Hiding does not release the current reservation")
	disposition, err := store.GetBookDisposition(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, disposition)
}

func TestEmptyMyBooksBucketWithOnlyHiddenBooksOffersShowHiddenThenUnhide(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "hidden-empty-bucket-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store := openVisibilityStore(t, ctx)
	owner := createAccount(t, ctx, store, "hidden-empty", "learner-password", false)
	book := seedVisibilityToReadBook(t, ctx, store, owner.ID, "hidden-empty", "Only hidden to read", "haus")
	h := visibilityHandler(store)
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")

	hide := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/hide", url.Values{
		"csrf_token": {csrf}, "expected_visibility_revision": {visibilityRevision(t, ctx, store, owner.ID, book.ID)}, "return_to": {"/library?disposition=to_read"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, hide.Code)

	empty := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	require.Equal(t, http.StatusOK, empty.Code)
	assert.NotContains(t, empty.Body.String(), "Only hidden to read</h2>", "the empty bucket lists no visible Book")
	assert.Contains(t, empty.Body.String(), "1 hidden book is not shown.")
	match := regexp.MustCompile(`href="([^"]*show-hidden[^"]*)">Show hidden books</a>`).FindStringSubmatch(empty.Body.String())
	require.Len(t, match, 2, "the empty view offers Show hidden books")

	shown := perform(t, h, http.MethodGet, html.UnescapeString(match[1]), nil, cookies)
	require.Equal(t, http.StatusOK, shown.Code)
	assert.Contains(t, shown.Body.String(), "Only hidden to read", "following Show hidden books lists the hidden Book")
	assert.Contains(t, shown.Body.String(), "/library/books/"+book.ID+"/unhide", "the listed Book offers Unhide")
	assert.Contains(t, shown.Body.String(), "Unhide")
}

func TestHideAndUnhideUnderPreviousStudyLanguageRecoverWithoutChange(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "hidden-stale-language-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store := openVisibilityStore(t, ctx)
	owner := createAccount(t, ctx, store, "hidden-stale-language", "learner-password", false)
	book := seedVisibilityToReadBook(t, ctx, store, owner.ID, "hidden-stale-language", "Stale visibility", "haus")
	h := visibilityHandler(store)
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")
	// Forms rendered while Italian was the study language carry the Italian suffix.
	staleItalian := csrf + ".it"
	current := csrf + ".de"

	staleHide := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/hide", url.Values{
		"csrf_token": {staleItalian}, "expected_visibility_revision": {"0"}, "return_to": {"/library?disposition=to_read"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, staleHide.Code)
	assert.Equal(t, languageChangedPath(), staleHide.Header().Get("Location"))
	hidden, revision, err := store.GetBookVisibility(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	require.False(t, hidden, "a stale Hide from another language changes nothing")
	require.Zero(t, revision)

	// Control: the same request under the active language is accepted, so the
	// stale rejection above is about language and not an unrelated failure.
	hideCurrent := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/hide", url.Values{
		"csrf_token": {current}, "expected_visibility_revision": {"0"}, "return_to": {"/library?disposition=to_read"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, hideCurrent.Code)
	require.NotEqual(t, languageChangedPath(), hideCurrent.Header().Get("Location"))
	hidden, revision, err = store.GetBookVisibility(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	require.True(t, hidden)
	require.EqualValues(t, 1, revision)

	staleUnhide := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/unhide", url.Values{
		"csrf_token": {staleItalian}, "expected_visibility_revision": {"1"}, "return_to": {"/library?show-hidden"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, staleUnhide.Code)
	assert.Equal(t, languageChangedPath(), staleUnhide.Header().Get("Location"))
	hidden, revision, err = store.GetBookVisibility(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.True(t, hidden, "a stale Unhide from another language changes nothing")
	assert.EqualValues(t, 1, revision)

	notice := perform(t, h, http.MethodGet, languageChangedPath(), nil, cookies)
	require.Equal(t, http.StatusOK, notice.Code)
	assert.Contains(t, notice.Body.String(), "Your study language changed")
}

func TestCatalogueDiscoveredBookAppearsVisibleInbox(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "hidden-discovery-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	store := openVisibilityStore(t, ctx)
	owner := createAccount(t, ctx, store, "hidden-discovery", "learner-password", false)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Discovery catalog", URL: "https://discovery.example/opds"})
	require.NoError(t, err)
	worker := &cataloguesync.Worker{
		Connections: store, Catalogue: store, Statuses: store, Reader: visibilityDiscoveryReader{},
		Capabilities: staticCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}},
	}
	require.NoError(t, worker.Work(ctx, &river.Job[cataloguesync.SyncArgs]{Args: cataloguesync.SyncArgs{OwnerID: owner.ID, ConnectionID: connection.ID}}))

	books, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 20)
	require.NoError(t, err)
	require.Len(t, books.Items, 1, "sync discovered the entry")
	discovered := books.Items[0]
	assert.Equal(t, "Discovered visible", discovered.Book.Title)
	hidden, revision, err := store.GetBookVisibility(ctx, owner.ID, discovered.Book.ID)
	require.NoError(t, err)
	assert.False(t, hidden, "a discovered Book is visible")
	assert.Zero(t, revision, "a discovered Book has no visibility choice yet")
	disposition, err := store.GetBookDisposition(ctx, owner.ID, discovered.Book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, disposition)

	h := visibilityHandler(store)
	cookies, _ := loginCookies(t, h, owner.Username, "learner-password")
	inbox := perform(t, h, http.MethodGet, "/library?disposition=inbox", nil, cookies)
	require.Equal(t, http.StatusOK, inbox.Code)
	assert.Contains(t, inbox.Body.String(), `id="book-row-`+discovered.Book.ID+`"`, "the discovered Book is listed in Inbox")
	assert.NotContains(t, inbox.Body.String(), "</span>Hidden</span>", "the discovered Book is not marked Hidden")
	assert.NotContains(t, inbox.Body.String(), "1 hidden book is not shown.")
}
