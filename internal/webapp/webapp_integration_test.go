//go:build integration

package webapp

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/testwrite"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingAnalysis struct {
	owner, source, scope string
	calls                int
}
type staticCapabilities struct {
	value analyzer.Capabilities
	err   error
}

func (s staticCapabilities) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return s.value, s.err
}

func readyGerman() staticCapabilities {
	return staticCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}}}}
}

type metadataBookAcquisitionStub struct {
	target cataloguesync.AcquisitionTarget
}

type catalogueLanguageCorrectionRefresher struct {
	store        *persistence.PostgresStore
	connectionID string
}

type emptyCatalogueSyncReader struct{}

func (emptyCatalogueSyncReader) Languages(context.Context, string, string) (opds.Feed, error) {
	return opds.Feed{Entries: []opds.Entry{
		{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://language.example/opds/language/7"}}},
		{Title: "Italian", Links: []opds.Link{{Rel: "subsection", Href: "https://language.example/opds/language/9"}}},
	}}, nil
}

func (emptyCatalogueSyncReader) BrowseLanguage(context.Context, string, string, string) (opds.Feed, error) {
	return opds.Feed{}, nil
}

func (reader emptyCatalogueSyncReader) BrowseLanguageUnfiltered(ctx context.Context, owner, connection, language string) (opds.Feed, error) {
	return reader.BrowseLanguage(ctx, owner, connection, language)
}

func (catalogueLanguageCorrectionRefresher) RegisterConnection(context.Context, string, string) error {
	return nil
}

func (catalogueLanguageCorrectionRefresher) UnregisterConnection(string, string) error { return nil }

func (r catalogueLanguageCorrectionRefresher) RefreshEntry(ctx context.Context, owner, bookID string) (cataloguesync.RefreshResult, error) {
	book, err := r.store.GetBook(ctx, owner, bookID)
	if err != nil {
		return cataloguesync.RefreshResult{}, err
	}
	alias, err := r.store.GetBookCatalogEntryAlias(ctx, owner, bookID)
	if err != nil {
		return cataloguesync.RefreshResult{}, err
	}
	reconciled, err := r.store.ReconcileCatalogueEntry(ctx, owner, r.connectionID, alias.Value, book.Title, book.Author, "it")
	return cataloguesync.RefreshResult{Book: reconciled.Book, Updated: reconciled.LanguageChanged}, err
}

func (metadataBookAcquisitionStub) RegisterConnection(context.Context, string, string) error {
	return nil
}

func (metadataBookAcquisitionStub) UnregisterConnection(string, string) error { return nil }

func (s metadataBookAcquisitionStub) FindAcquisitionTarget(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
	return s.target, nil
}

func createAccount(t *testing.T, ctx context.Context, store *persistence.PostgresStore, username, password string, legacyAdmin bool) domain.User {
	t.Helper()
	hash, err := auth.HashPassword(password)
	require.NoError(t, err)
	u, err := store.CreateUserWithPassword(ctx, username, hash, legacyAdmin)
	require.NoError(t, err)
	return u
}

type recordingKnownVocab struct {
	service *knownvocab.Service
	status  knownvocab.Status
	owner   string
}
type recordingPreparedDeck struct {
	preparations map[string]domain.DeckPreparation
	downloads    int
}

func (r *recordingPreparedDeck) Submit(_ context.Context, owner, analysisID string) (prepareddeck.Handle, error) {
	for _, p := range r.preparations {
		if p.OwnerID == owner && p.AnalysisRunID == analysisID {
			return prepareddeck.Handle{Preparation: p, JobID: 91}, nil
		}
	}
	p := domain.DeckPreparation{ID: "prep-1", OwnerID: owner, SourceMaterialID: "00000000-0000-0000-0000-000000000001", AnalysisRunID: analysisID, State: domain.DeckPreparationQueued, Filename: "Stored Book.apkg", DeckName: "Mouseion::de::Stored Book"}
	r.preparations[p.ID] = p
	return prepareddeck.Handle{Preparation: p, JobID: 91}, nil
}
func (r *recordingPreparedDeck) SubmitForGoal(_ context.Context, owner, analysisID, snapshotID string) (prepareddeck.Handle, error) {
	p := domain.DeckPreparation{ID: "goal-prep-1", OwnerID: owner, SourceMaterialID: "00000000-0000-0000-0000-000000000001", AnalysisRunID: analysisID, GoalSnapshotID: snapshotID, State: domain.DeckPreparationQueued, Filename: "Stored Book.apkg", DeckName: "Mouseion::de::Stored Book"}
	r.preparations[p.ID] = p
	return prepareddeck.Handle{Preparation: p, JobID: 94}, nil
}
func (r *recordingPreparedDeck) Get(_ context.Context, owner, id string) (domain.DeckPreparation, error) {
	p, ok := r.preparations[id]
	if !ok || p.OwnerID != owner {
		return domain.DeckPreparation{}, persistence.ErrNotFound
	}
	return p, nil
}
func (r *recordingPreparedDeck) Cancel(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	p, err := r.Get(ctx, owner, id)
	if err != nil {
		return p, err
	}
	if p.State != domain.DeckPreparationQueued && p.State != domain.DeckPreparationPreparing {
		return p, persistence.ErrInvalidTransition
	}
	p.State = domain.DeckPreparationCancelled
	r.preparations[id] = p
	return p, nil
}
func (r *recordingPreparedDeck) Retry(ctx context.Context, owner, id string) (prepareddeck.Handle, error) {
	p, err := r.Get(ctx, owner, id)
	if err != nil {
		return prepareddeck.Handle{}, err
	}
	if p.State != domain.DeckPreparationFailed && p.State != domain.DeckPreparationCancelled {
		return prepareddeck.Handle{}, persistence.ErrInvalidTransition
	}
	p.State, p.Error = domain.DeckPreparationQueued, ""
	r.preparations[id] = p
	return prepareddeck.Handle{Preparation: p, JobID: 92}, nil
}
func (r *recordingPreparedDeck) Reprepare(ctx context.Context, owner, id string) (prepareddeck.Handle, error) {
	p, err := r.Get(ctx, owner, id)
	if err != nil {
		return prepareddeck.Handle{}, err
	}
	if p.State != domain.DeckPreparationReady || p.RetiredAt != nil {
		return prepareddeck.Handle{}, persistence.ErrInvalidTransition
	}
	retiredAt := time.Now()
	p.RetiredAt = &retiredAt
	r.preparations[id] = p
	p.ID += "-next"
	p.RetiredAt, p.State, p.Error = nil, domain.DeckPreparationQueued, ""
	r.preparations[p.ID] = p
	return prepareddeck.Handle{Preparation: p, JobID: 95}, nil
}
func (r *recordingPreparedDeck) Rerender(ctx context.Context, owner, id string) (prepareddeck.Handle, error) {
	p, err := r.Get(ctx, owner, id)
	if err != nil {
		return prepareddeck.Handle{}, err
	}
	return prepareddeck.Handle{Preparation: p, JobID: 93}, nil
}
func (r *recordingPreparedDeck) Download(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	p, err := r.Get(ctx, owner, id)
	if err != nil {
		return p, err
	}
	if p.State != domain.DeckPreparationReady {
		return p, persistence.ErrInvalidTransition
	}
	r.downloads++
	return p, nil
}

func (r *recordingKnownVocab) Submit(ctx context.Context, owner, language, input string) (knownvocab.Handle, error) {
	result, err := r.service.Import(ctx, owner, language, strings.NewReader(input))
	if err != nil {
		return knownvocab.Handle{}, err
	}
	r.status = knownvocab.Status{ID: 77, Language: language, State: rivertype.JobStateCompleted, Processed: len(result.Entries) + len(result.Rejected), Total: len(result.Entries) + len(result.Rejected), Imported: result.Imported, AlreadyKnown: result.AlreadyKnown, Rejected: result.Rejected}
	r.owner = owner
	return knownvocab.Handle{ID: 77}, nil
}
func (r *recordingKnownVocab) Get(_ context.Context, owner string, id int64) (knownvocab.Status, error) {
	if owner != r.owner || id != r.status.ID {
		return knownvocab.Status{}, knownvocab.ErrJobNotFound
	}
	return r.status, nil
}

func (r *recordingAnalysis) SubmitAnalysis(_ context.Context, owner, source string) (analysis.Handle, error) {
	r.owner, r.source, r.scope = owner, source, ""
	r.calls++
	return analysis.Handle{ID: 42, DisplayNumber: 1}, nil
}

func (r *recordingAnalysis) SubmitToReadBookAnalysis(ctx context.Context, owner, bookID, source string) (analysis.Handle, error) {
	return r.SubmitAnalysis(ctx, owner, source)
}

func (r *recordingAnalysis) Get(_ context.Context, owner string, id int64) (analysis.Status, error) {
	if owner != r.owner || id != 42 {
		return analysis.Status{}, analysis.ErrNotFound
	}
	return analysis.Status{ID: 42, DisplayNumber: 1, State: rivertype.JobStateCompleted, Progress: 100, CorpusID: "corpus-result", Attempt: 1}, nil
}

func TestFirstAccountOnboardingAndExistingLogin(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "first-account-web-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), Capabilities: readyGerman(), SessionLifetime: time.Hour})

	page := perform(t, h, "GET", "/login", nil, nil)
	assert.Equal(t, http.StatusOK, page.Code)
	assert.True(t, strings.Contains(page.Body.String(), "Create your account"), "body=%s", page.Body.String())
	assert.False(t, strings.Contains(page.Body.String(), `action="/login"`), "body=%s", page.Body.String())
	csrf := hiddenToken(t, page.Body.String())
	csrfCookieValue := cookieNamed(t, page.Result().Cookies(), csrfCookie)
	got := perform(t, h, "POST", "/onboarding", url.Values{"username": {"alice"}, "password": {"alice-password"}}, nil)
	assert.Equal(t, http.StatusForbidden, got.Code)
	created := perform(t, h, "POST", "/onboarding", url.Values{"csrf_token": {csrf}, "username": {"alice"}, "password": {"alice-password"}}, []*http.Cookie{csrfCookieValue})
	assert.Equal(t, http.StatusSeeOther, created.Code)
	assert.Equal(t, "/", created.Header().Get("Location"))
	assert.NotEmpty(t, cookieNamed(t, created.Result().Cookies(), webauth.CookieName).Value)

	page = perform(t, h, "GET", "/login", nil, nil)
	assert.Equal(t, http.StatusOK, page.Code)
	assert.False(t, strings.Contains(page.Body.String(), "Create your account"), "body=%s", page.Body.String())
	assert.True(t, strings.Contains(page.Body.String(), `action="/login"`), "body=%s", page.Body.String())
	navigationRequest := httptest.NewRequestWithContext(t.Context(), "GET", "/library?sort=title", nil)
	navigationRequest.Header.Set("Accept", "text/html")
	navigationRequest.Header.Set("Sec-Fetch-Mode", "navigate")
	navigation := httptest.NewRecorder()
	h.ServeHTTP(navigation, navigationRequest)
	assert.Equal(t, http.StatusSeeOther, navigation.Code)
	assert.Equal(t, "/login?next=%2Flibrary%3Fsort%3Dtitle", navigation.Header().Get("Location"))
	returnPage := perform(t, h, "GET", navigation.Header().Get("Location"), nil, nil)
	assert.True(t, strings.Contains(returnPage.Body.String(), `name="next" value="/library?sort=title"`), "body=%s", returnPage.Body.String())
	returnCSRF := hiddenToken(t, returnPage.Body.String())
	returnCSRFCookie := cookieNamed(t, returnPage.Result().Cookies(), csrfCookie)
	returnedLogin := perform(t, h, "POST", "/login", url.Values{"csrf_token": {returnCSRF}, "next": {"/library?sort=title"}, "username": {"alice"}, "password": {"alice-password"}}, []*http.Cookie{returnCSRFCookie})
	assert.Equal(t, http.StatusSeeOther, returnedLogin.Code)
	assert.Equal(t, "/library?sort=title", returnedLogin.Header().Get("Location"))
	csrf = hiddenToken(t, page.Body.String())
	csrfCookieValue = cookieNamed(t, page.Result().Cookies(), csrfCookie)
	blocked := perform(t, h, "POST", "/onboarding", url.Values{"csrf_token": {csrf}, "username": {"bob"}, "password": {"bob-password"}}, []*http.Cookie{csrfCookieValue})
	assert.Equal(t, http.StatusNotFound, blocked.Code)
	login := perform(t, h, "POST", "/login", url.Values{"csrf_token": {csrf}, "username": {"alice"}, "password": {"alice-password"}}, []*http.Cookie{csrfCookieValue})
	assert.Equal(t, http.StatusSeeOther, login.Code)
	assert.Equal(t, "/", login.Header().Get("Location"))
}

func TestKnownVocabImportUsesDerivedLibraryLanguages(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "known-vocab-derived-language-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "alice", "alice-password", false)
	_, err = store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "German library book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	known := &recordingKnownVocab{service: knownvocab.NewService(store)}
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), KnownVocab: known, SessionLifetime: time.Hour})
	cookies, csrf := loginCookies(t, h, "alice", "alice-password")

	browse := perform(t, h, http.MethodGet, "/vocabulary", nil, cookies)
	assert.Equal(t, http.StatusOK, browse.Code)
	assert.Contains(t, browse.Body.String(), "No current analyzed Books contribute vocabulary yet.")
	assert.Contains(t, browse.Body.String(), `href="/vocabulary/import"`)
	importPage := perform(t, h, http.MethodGet, "/vocabulary/import", nil, cookies)
	assert.Equal(t, http.StatusOK, importPage.Code)
	assert.Contains(t, importPage.Body.String(), `action="/vocabulary/import"`)

	imported := multipartUpload(t, h, "/vocabulary/import?language=it", cookies, map[string]string{"csrf_token": csrf, "language": "it"}, "Haus\n")
	assert.Equal(t, http.StatusSeeOther, imported.Code)
	assert.Equal(t, "/vocabulary/imports/77/status", imported.Header().Get("Location"))
	assert.Equal(t, alice.ID, known.owner)
	assert.Equal(t, "de", known.status.Language)
}

func TestMetadataOnlyBookDetailAcquiresIntoExistingBook(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "metadata-acquisition-integration-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	owner := createAccount(t, ctx, store, "metadata-owner", "owner-password", false)
	downloads := 0
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/book.epub" {
			http.NotFound(w, r)
			return
		}
		downloads++
		w.Header().Set("Content-Type", opds.EPUBMediaType)
		testwrite.Bytes(t, w, testEPUBVariant(t, "metadata-entry", "Metadata-only synced book", "Hallo Welt."))
	}))
	defer catalog.Close()
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Metadata catalog", URL: catalog.URL + "/opds"})
	require.NoError(t, err)
	bookResult, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, "metadata-entry", "Metadata-only synced book", "", "de")
	require.NoError(t, err)
	target := cataloguesync.AcquisitionTarget{
		ConnectionID: connection.ID,
		Language:     "de",
		Entry:        opds.Entry{ID: "metadata-entry", Title: "Metadata-only synced book", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "/book.epub"}}},
		Href:         "/book.epub",
	}
	recorder := &recordingAnalysis{}
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		OPDS:     opds.NewService(store, epub.NewService(store), catalog.Client()),
		Analysis: recorder, CatalogueSync: metadataBookAcquisitionStub{target: target}, Capabilities: readyGerman(), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "owner-password")
	bookState, err := store.GetBookDetail(ctx, owner.ID, bookResult.Book.ID)
	require.NoError(t, err)
	bookPage := perform(t, h, "GET", "/books/"+bookResult.Book.ID, nil, cookies)
	assert.Equal(t, http.StatusNotFound, bookPage.Code)
	added := perform(t, h, "POST", "/library/books/"+bookResult.Book.ID+"/to-read", url.Values{"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(bookState.DispositionRevision, 10)}}, cookies)
	assert.Equal(t, http.StatusSeeOther, added.Code)
	assert.Equal(t, 1, downloads)
	assert.Equal(t, 1, recorder.calls)
	readded := perform(t, h, "POST", "/library/books/"+bookResult.Book.ID+"/to-read", url.Values{"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(bookState.DispositionRevision, 10)}}, cookies)
	assert.Equal(t, http.StatusSeeOther, readded.Code)
	assert.Equal(t, 1, downloads)
	assert.Equal(t, 1, recorder.calls)
	bookState, err = store.GetBookDetail(ctx, owner.ID, bookResult.Book.ID)
	require.NoError(t, err)
	removed := perform(t, h, "POST", "/library/books/"+bookResult.Book.ID+"/set-aside", url.Values{"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(bookState.DispositionRevision, 10)}}, cookies)
	assert.Equal(t, http.StatusSeeOther, removed.Code)
	assert.Contains(t, removed.Header().Get("Location"), "disposition=set_aside", "location=%q body=%s", removed.Header().Get("Location"), removed.Body.String())
	assert.Equal(t, 1, recorder.calls)
	bookState, err = store.GetBookDetail(ctx, owner.ID, bookResult.Book.ID)
	require.NoError(t, err)
	alreadyAcquired := perform(t, h, "POST", "/library/books/"+bookResult.Book.ID+"/to-read", url.Values{"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(bookState.DispositionRevision, 10)}}, cookies)
	assert.Equal(t, http.StatusSeeOther, alreadyAcquired.Code)
	assert.Equal(t, 1, downloads)
	assert.Equal(t, 2, recorder.calls)
	sources, err := store.ListSourceMaterials(ctx, owner.ID)
	require.NoError(t, err)
	assert.Len(t, sources, 1)
	assert.Equal(t, bookResult.Book.ID, sources[0].BookID)
	legacyPage := perform(t, h, "GET", "/books/"+sources[0].Source.ID, nil, cookies)
	assert.Equal(t, http.StatusNotFound, legacyPage.Code)
	books, err := store.ListMyBooksWithEvidence(ctx, owner.ID)
	require.NoError(t, err)
	assert.Len(t, books, 1)
	assert.Equal(t, bookResult.Book.ID, books[0].Book.ID)
	assert.NotNil(t, books[0].Acquired)
}

func TestAuthenticatedMetadataRefreshCorrectsCurrentReadingLanguage(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "catalogue-language-http-integration-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner := createAccount(t, ctx, store, "language-refresh-owner", "owner-password", false)
	book, _, _, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "http-language-correction", "Language correction", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Language catalog", URL: "https://language.example/opds"})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_aliases(owner_id,book_id,connection_id,alias_type,namespace,value) VALUES($1,$2,$3,$4,$5,$6)`, owner.ID, book.ID, connection.ID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, "language-entry")
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	_, err = store.ImportPreviouslyRead(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	syncWorker := &cataloguesync.Worker{
		Connections: store, Catalogue: store, Statuses: store, Reader: emptyCatalogueSyncReader{},
		Capabilities: staticCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
			{Language: "de", DisplayName: "German", Ready: true},
			{Language: "it", DisplayName: "Italian", Ready: true},
		}}},
	}
	require.NoError(t, syncWorker.Work(ctx, &river.Job[cataloguesync.SyncArgs]{Args: cataloguesync.SyncArgs{OwnerID: owner.ID, ConnectionID: connection.ID}}))
	var retainedHistory, vanishedAnalysis int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&retainedHistory)
	require.NoError(t, err)
	assert.Equal(t, 1, retainedHistory, "upstream disappearance erased reading history")
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&vanishedAnalysis)
	require.NoError(t, err)
	assert.Equal(t, 1, vanishedAnalysis, "upstream disappearance erased analysis provenance")
	sourcesBeforeRefresh, err := store.ListSourceMaterials(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, sourcesBeforeRefresh, 1)
	preparationsBeforeRefresh, err := store.ListDeckPreparationsForSourceMaterial(ctx, owner.ID, sourcesBeforeRefresh[0].Source.ID)
	require.NoError(t, err)
	assert.Len(t, preparationsBeforeRefresh, 1, "upstream disappearance erased local deck state")

	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		CatalogueSync: catalogueLanguageCorrectionRefresher{store: store, connectionID: connection.ID}, SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "owner-password")
	refreshed := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/refresh", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, refreshed.Code)

	corrected, err := store.GetBook(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, "it", corrected.LanguageTag)
	disposition, err := store.GetBookDisposition(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, disposition)
	for _, language := range []string{"de", "it"} {
		current, getErr := store.GetCurrentReading(ctx, owner.ID, language)
		require.NoError(t, getErr)
		assert.Empty(t, current.BookID, "metadata refresh left the book in a %s current-reading role", language)
	}
	var releasedAt *time.Time
	err = store.Pool().QueryRow(ctx, `SELECT released_at FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2`, owner.ID, reading.SnapshotID).Scan(&releasedAt)
	require.NoError(t, err)
	assert.NotNil(t, releasedAt)
	italian, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "it", "", false, 0, 20)
	require.NoError(t, err)
	assert.Len(t, italian.Items, 1, "corrected Book was not placed in its new language collection")
	german, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 20)
	require.NoError(t, err)
	assert.Empty(t, german.Items, "corrected Book remained in its old language collection")
	deleted := perform(t, h, http.MethodPost, "/connections/"+connection.ID+"/delete", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, deleted.Code)
	_, err = store.GetOpdsConnection(ctx, owner.ID, connection.ID)
	require.ErrorIs(t, err, persistence.ErrNotFound)
	disposition, err = store.GetBookDisposition(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, disposition, "connection deletion erased learner disposition")
	var retainedCompletions, retainedAnalysis int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&retainedCompletions)
	require.NoError(t, err)
	assert.Equal(t, 1, retainedCompletions, "connection deletion erased reading history")
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&retainedAnalysis)
	require.NoError(t, err)
	assert.Equal(t, 1, retainedAnalysis, "connection deletion erased analysis provenance")
	sources, err := store.ListSourceMaterials(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, sources, 1, "connection deletion erased acquired provenance")
	preparations, err := store.ListDeckPreparationsForSourceMaterial(ctx, owner.ID, sources[0].Source.ID)
	require.NoError(t, err)
	assert.Len(t, preparations, 1, "connection deletion erased local deck state")
}

func TestAuthenticatedNeedsLanguageBookRemainsActionableAndOutsideStudyLanguages(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "needs-language-actionable-integration-secret")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner := createAccount(t, ctx, store, "needs-language-owner", "owner-password", false)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Awaiting a language", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), Capabilities: readyGerman(), SessionLifetime: time.Hour})
	cookies, _ := loginCookies(t, h, owner.Username, "owner-password")
	page := perform(t, h, http.MethodGet, "/library?needs-language", nil, cookies)
	assert.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "Awaiting a language")
	assert.Contains(t, page.Body.String(), "Fix the language in the catalog, then re-sync")
	assert.Contains(t, page.Body.String(), `href="/catalogs"`)

	unknown, err := store.ListMyBooksBrowse(ctx, owner.ID, "", domain.LanguageUnknown, "", false, 0, 20)
	require.NoError(t, err)
	assert.Len(t, unknown.Items, 1)
	assert.Equal(t, book.ID, unknown.Items[0].Book.ID)
	for _, language := range []string{"de", "it"} {
		books, browseErr := store.ListMyBooksBrowse(ctx, owner.ID, "", language, "", false, 0, 20)
		require.NoError(t, browseErr)
		assert.Empty(t, books.Items, "needs-language Book appeared in the chosen %s collection", language)
	}
	studyLanguages, err := store.ListStudyLanguages(ctx, owner.ID)
	require.NoError(t, err)
	assert.Empty(t, studyLanguages, "needs-language Book produced a misleading study-language scope")
}

func TestPreparedDeckWebLifecycleOwnershipAndPureDownload(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "prepared-deck-web-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "alice", "alice-password", false)
	createAccount(t, ctx, store, "bob", "bob-password", false)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Disposition from My Books", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	decks := &recordingPreparedDeck{preparations: make(map[string]domain.DeckPreparation)}
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), PreparedDeck: decks, Capabilities: readyGerman(), SessionLifetime: time.Hour})
	aliceCookies, aliceCSRF := loginCookies(t, h, "alice", "alice-password")
	bobCookies, bobCSRF := loginCookies(t, h, "bob", "bob-password")

	got := perform(t, h, "POST", "/jobs/42/deck/preparations", nil, aliceCookies)
	assert.Equal(t, http.StatusForbidden, got.Code)
	created := perform(t, h, "POST", "/jobs/42/deck/preparations", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
	assert.Equal(t, http.StatusSeeOther, created.Code)
	assert.Equal(t, "/deck-preparations/prep-1/status", created.Header().Get("Location"))
	preparation := decks.preparations["prep-1"]
	preparation.SourceMaterialID = book.ID
	decks.preparations[preparation.ID] = preparation
	statusPage := perform(t, h, "GET", created.Header().Get("Location"), nil, aliceCookies)
	assert.Equal(t, http.StatusOK, statusPage.Code)
	assert.True(t, strings.Contains(statusPage.Body.String(), "Deck preparation queued"), "body=%s", statusPage.Body.String())
	assert.True(t, strings.Contains(statusPage.Body.String(), "Cancel preparation"), "body=%s", statusPage.Body.String())
	status := perform(t, h, "GET", created.Header().Get("Location")+"?format=json", nil, aliceCookies)
	assert.Equal(t, http.StatusOK, status.Code)
	assert.Equal(t, "application/json; charset=utf-8", status.Header().Get("Content-Type"))
	assert.True(t, strings.Contains(status.Body.String(), `"state":"queued"`), "body=%s", status.Body.String())
	assert.True(t, strings.Contains(status.Body.String(), `"progress":0`), "body=%s", status.Body.String())
	got = perform(t, h, "GET", "/deck-preparations/prep-1/status", nil, bobCookies)
	assert.Equal(t, http.StatusNotFound, got.Code)
	got = perform(t, h, "GET", "/deck-preparations/missing/status", nil, aliceCookies)
	assert.Equal(t, http.StatusNotFound, got.Code)
	got = perform(t, h, "GET", "/deck-preparations/prep-1/download", nil, aliceCookies)
	assert.Equal(t, http.StatusConflict, got.Code)
	got = perform(t, h, "POST", "/deck-preparations/prep-1/cancel", nil, aliceCookies)
	assert.Equal(t, http.StatusForbidden, got.Code)
	cancelled := perform(t, h, "POST", "/deck-preparations/prep-1/cancel?format=json", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
	assert.Equal(t, http.StatusOK, cancelled.Code)
	assert.True(t, strings.Contains(cancelled.Body.String(), `"state":"cancelled"`), "body=%s", cancelled.Body.String())
	got = perform(t, h, "POST", "/deck-preparations/prep-1/retry", nil, aliceCookies)
	assert.Equal(t, http.StatusForbidden, got.Code)
	retried := perform(t, h, "POST", "/deck-preparations/prep-1/retry?format=json", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
	assert.Equal(t, http.StatusOK, retried.Code)
	assert.True(t, strings.Contains(retried.Body.String(), `"state":"queued"`), "body=%s", retried.Body.String())
	got = perform(t, h, "POST", "/deck-preparations/prep-1/cancel", url.Values{"csrf_token": {bobCSRF}}, bobCookies)
	assert.Equal(t, http.StatusNotFound, got.Code)

	ready := decks.preparations["prep-1"]
	ready.State, ready.Artifact = domain.DeckPreparationReady, []byte("newest-rendered-apkg")
	ready.DeckRevision = 2
	ready.TotalCards, ready.CardsWithEnglish, ready.CardsWithContextualSentenceTranslations, ready.QualityOmissions = 7, 6, 5, 2
	decks.preparations[ready.ID] = ready
	updatedPage := perform(t, h, "GET", "/deck-preparations/prep-1/status", nil, aliceCookies)
	assert.Equal(t, http.StatusOK, updatedPage.Code)
	assert.Contains(t, updatedPage.Body.String(), "Updated deck revision available")
	assert.Contains(t, updatedPage.Body.String(), "revision 2")
	assert.Contains(t, updatedPage.Body.String(), "Not in To Read")
	assert.Contains(t, updatedPage.Body.String(), `href="/library"`, "focused deck disposition action leads to My Books")
	assert.NotContains(t, updatedPage.Body.String(), "Move to To Read")
	assert.NotContains(t, updatedPage.Body.String(), `action="/reading/books/`)
	for i := range 2 {
		download := perform(t, h, "GET", "/deck-preparations/prep-1/download", nil, aliceCookies)
		assert.Equal(t, http.StatusOK, download.Code, "download %d", i)
		assert.Equal(t, "newest-rendered-apkg", download.Body.String(), "download %d", i)
		assert.Equal(t, "application/vnd.anki", download.Header().Get("Content-Type"), "download %d", i)
		assert.True(t, strings.Contains(download.Header().Get("Content-Disposition"), `filename="Stored Book.apkg"`), "download %d content-disposition=%q", i, download.Header().Get("Content-Disposition"))
		assert.Equal(t, "Mouseion::de::Stored Book", download.Header().Get("X-Mouseion-Deck-Name"), "download %d", i)
		assert.Equal(t, "7", download.Header().Get("X-Mouseion-Cards-Total"), "download %d", i)
		assert.Equal(t, "6", download.Header().Get("X-Mouseion-Cards-With-English"), "download %d", i)
		assert.Equal(t, "5", download.Header().Get("X-Mouseion-Cards-With-English-Sentence"), "download %d", i)
		assert.Equal(t, "2", download.Header().Get("X-Mouseion-Cards-Quality-Omitted"), "download %d", i)
		assert.Equal(t, "2", download.Header().Get("X-Mouseion-Deck-Revision"), "download %d", i)
	}
	assert.Equal(t, 2, decks.downloads)
	assert.Equal(t, domain.DeckPreparationReady, decks.preparations["prep-1"].State)
	got = perform(t, h, "GET", "/deck-preparations/prep-1/download", nil, bobCookies)
	assert.Equal(t, http.StatusNotFound, got.Code)
}

func loginCookies(t *testing.T, h http.Handler, username, password string) ([]*http.Cookie, string) {
	page := perform(t, h, "GET", "/login", nil, nil)
	token := hiddenToken(t, page.Body.String())
	csrfCookieValue := cookieNamed(t, page.Result().Cookies(), csrfCookie)
	response := perform(t, h, "POST", "/login", url.Values{"csrf_token": {token}, "username": {username}, "password": {password}}, []*http.Cookie{csrfCookieValue})
	assert.Equal(t, http.StatusSeeOther, response.Code, "login %s=%d %s", username, response.Code, response.Body.String())
	csrfCookieValue = cookieNamed(t, response.Result().Cookies(), csrfCookie)
	return []*http.Cookie{csrfCookieValue, cookieNamed(t, response.Result().Cookies(), webauth.CookieName)}, csrfCookieValue.Value
}
func multipartUpload(t *testing.T, h http.Handler, path string, cookies []*http.Cookie, fields map[string]string, content string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}
	field, filename := "dataset", "frequency.csv"
	if strings.HasPrefix(path, "/vocabulary/import") {
		field, filename = "vocabulary_file", "known.txt"
	}
	part, err := writer.CreateFormFile(field, filename)
	require.NoError(t, err)
	_, err = io.WriteString(part, content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	r := httptest.NewRequestWithContext(t.Context(), "POST", path, &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func perform(t *testing.T, h http.Handler, method, path string, form url.Values, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequestWithContext(t.Context(), method, path, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func hiddenToken(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(body)
	require.Len(t, match, 2, "csrf token absent: %s", body)
	return match[1]
}
func cookieNamed(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	require.Failf(t, "cookie absent", "cookie %s absent", name)
	return nil
}
func testEPUBVariant(t *testing.T, identifier, title, text string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	files := map[string]string{"mimetype": "application/epub+zip", "META-INF/container.xml": `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`, `OEBPS/content.opf`: fmt.Sprintf(`<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">%s</dc:identifier><dc:title>%s</dc:title></metadata><manifest><item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="chapter"/></spine></package>`, identifier, title), `OEBPS/chapter.xhtml`: fmt.Sprintf(`<html xmlns="http://www.w3.org/1999/xhtml"><body><p>%s</p></body></html>`, text)}
	for name, content := range files {
		w, err := z.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, z.Close())
	return b.Bytes()
}
