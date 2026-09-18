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
	consent      bool
}

func (r *recordingPreparedDeck) Submit(_ context.Context, owner, analysisID string, consent bool) (prepareddeck.Handle, error) {
	r.consent = consent
	for _, p := range r.preparations {
		if p.OwnerID == owner && p.AnalysisRunID == analysisID {
			return prepareddeck.Handle{Preparation: p, JobID: 91}, nil
		}
	}
	p := domain.DeckPreparation{ID: "prep-1", OwnerID: owner, SourceMaterialID: "00000000-0000-0000-0000-000000000001", AnalysisRunID: analysisID, State: domain.DeckPreparationQueued, Filename: "Stored Book.apkg", DeckName: "Mouseion::de::Stored Book"}
	r.preparations[p.ID] = p
	return prepareddeck.Handle{Preparation: p, JobID: 91}, nil
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
func (r *recordingPreparedDeck) Retry(ctx context.Context, owner, id string, consent bool) (prepareddeck.Handle, error) {
	p, err := r.Get(ctx, owner, id)
	if err != nil {
		return prepareddeck.Handle{}, err
	}
	if p.State != domain.DeckPreparationFailed && p.State != domain.DeckPreparationCancelled {
		return prepareddeck.Handle{}, persistence.ErrInvalidTransition
	}
	r.consent = consent
	p.State, p.Error = domain.DeckPreparationQueued, ""
	r.preparations[id] = p
	return prepareddeck.Handle{Preparation: p, JobID: 92}, nil
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
	bookResult, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, "metadata-entry", "Metadata-only synced book", "de")
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
	bookPage := perform(t, h, "GET", "/books/"+bookResult.Book.ID, nil, cookies)
	assert.Equal(t, http.StatusNotFound, bookPage.Code)
	journey, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	added := perform(t, h, "POST", "/journey/books/"+bookResult.Book.ID+"/add", url.Values{"csrf_token": {csrf}, "expected_revision": {fmt.Sprintf("%d", journey.Revision)}}, cookies)
	assert.Equal(t, http.StatusSeeOther, added.Code)
	assert.Equal(t, 1, downloads)
	assert.Equal(t, 1, recorder.calls)
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Len(t, journey.Entries, 1)
	assert.Equal(t, bookResult.Book.ID, journey.Entries[0].BookID)
	readded := perform(t, h, "POST", "/journey/books/"+bookResult.Book.ID+"/add", url.Values{"csrf_token": {csrf}, "expected_revision": {fmt.Sprintf("%d", journey.Revision)}}, cookies)
	assert.Equal(t, http.StatusSeeOther, readded.Code)
	assert.Equal(t, 1, downloads)
	assert.Equal(t, 1, recorder.calls)
	removed := perform(t, h, "POST", "/journey/books/"+bookResult.Book.ID+"/remove", url.Values{"csrf_token": {csrf}, "expected_revision": {fmt.Sprintf("%d", journey.Revision)}}, cookies)
	assert.Equal(t, http.StatusSeeOther, removed.Code)
	assert.True(t, strings.Contains(removed.Header().Get("Location"), "removed+from+Reading+Journey"), "location=%q body=%s", removed.Header().Get("Location"), removed.Body.String())
	assert.Equal(t, 1, recorder.calls)
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, journey.Entries)
	alreadyAcquired := perform(t, h, "POST", "/journey/books/"+bookResult.Book.ID+"/add", url.Values{"csrf_token": {csrf}, "expected_revision": {fmt.Sprintf("%d", journey.Revision)}}, cookies)
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

func TestPreparedDeckWebLifecycleOwnershipAndPureDownload(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "prepared-deck-web-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	createAccount(t, ctx, store, "alice", "alice-password", false)
	createAccount(t, ctx, store, "bob", "bob-password", false)
	decks := &recordingPreparedDeck{preparations: make(map[string]domain.DeckPreparation)}
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), PreparedDeck: decks, Capabilities: readyGerman(), SessionLifetime: time.Hour})
	aliceCookies, aliceCSRF := loginCookies(t, h, "alice", "alice-password")
	bobCookies, bobCSRF := loginCookies(t, h, "bob", "bob-password")

	got := perform(t, h, "POST", "/jobs/42/deck/preparations", nil, aliceCookies)
	assert.Equal(t, http.StatusForbidden, got.Code)
	created := perform(t, h, "POST", "/jobs/42/deck/preparations", url.Values{"csrf_token": {aliceCSRF}, "external_translation_consent": {"on"}}, aliceCookies)
	assert.Equal(t, http.StatusSeeOther, created.Code)
	assert.Equal(t, "/deck-preparations/prep-1/status", created.Header().Get("Location"))
	assert.True(t, decks.consent)
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
	for i := 0; i < 2; i++ {
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

func TestJourneyReorderingEndpointsAreOwnerScopedAndStaleSafe(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "journey-reorder-web-integration-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "journey-web-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "journey-web-bob", "bob-password", false)
	newBook := func(owner domain.User, title string) domain.Book {
		book, createErr := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
		require.NoError(t, createErr)
		return book
	}
	goal, goalSource, _, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "reorder-goal", "Anchored Goal", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "goal", UPOS: "NOUN", OccurrenceCount: 1}})
	require.NoError(t, store.LinkSourceToBook(ctx, alice.ID, goal.ID, goalSource.ID))
	first := newBook(alice, "First provisional")
	second := newBook(alice, "Second provisional")
	third := newBook(alice, "Third provisional")
	foreign := newBook(bob, "Foreign provisional")
	notMember := newBook(alice, "Removed provisional")
	journeyRevision := int64(0)
	for _, book := range []domain.Book{goal, first, second, third} {
		journeyRevision, err = store.AddToReadingJourney(ctx, alice.ID, "de", book.ID, journeyRevision)
		require.NoError(t, err)
	}
	_, err = store.CreatePrimaryGoal(ctx, alice.ID, "de", goal.ID)
	require.NoError(t, err)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	aliceCookies, csrf := loginCookies(t, h, alice.Username, "alice-password")
	bobCookies, _ := loginCookies(t, h, bob.Username, "bob-password")
	page := perform(t, h, "GET", "/journey", nil, aliceCookies)
	assert.Equal(t, http.StatusOK, page.Code)
	expected := hiddenInputValue(t, page.Body.String(), "expected_revision")
	moveForm := func(token, revision string) url.Values {
		return url.Values{"csrf_token": {token}, "expected_revision": {revision}}
	}
	moved := perform(t, h, "POST", "/journey/entries/"+second.ID+"/move-earlier", moveForm(csrf, expected), aliceCookies)
	assert.Equal(t, http.StatusSeeOther, moved.Code)
	assert.True(t, strings.HasPrefix(moved.Header().Get("Location"), "/journey?message="), "location=%q body=%s", moved.Header().Get("Location"), moved.Body.String())
	journey, err := store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, goal.ID, journey.Entries[0].BookID)
	assert.Equal(t, second.ID, journey.Entries[1].BookID)
	assert.Equal(t, first.ID, journey.Entries[2].BookID)
	page = perform(t, h, "GET", "/journey", nil, aliceCookies)
	moved = perform(t, h, "POST", "/journey/entries/"+second.ID+"/move-later", moveForm(csrf, hiddenInputValue(t, page.Body.String(), "expected_revision")), aliceCookies)
	assert.Equal(t, http.StatusSeeOther, moved.Code)
	assert.True(t, strings.HasPrefix(moved.Header().Get("Location"), "/journey?message="), "location=%q", moved.Header().Get("Location"))
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, goal.ID, journey.Entries[0].BookID)
	assert.Equal(t, first.ID, journey.Entries[1].BookID)
	assert.Equal(t, second.ID, journey.Entries[2].BookID)
	staleRevision := hiddenInputValue(t, page.Body.String(), "expected_revision")
	stale := perform(t, h, "POST", "/journey/entries/"+third.ID+"/move-earlier", moveForm(csrf, staleRevision), aliceCookies)
	assert.Equal(t, http.StatusSeeOther, stale.Code)
	assert.True(t, strings.Contains(stale.Header().Get("Location"), "This+Journey+changed+since+this+page+was+loaded"), "location=%q", stale.Header().Get("Location"))
	journeyAfterStale, err := store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Len(t, journeyAfterStale.Entries, len(journey.Entries))
	assert.Equal(t, journey.Entries[0].BookID, journeyAfterStale.Entries[0].BookID)
	assert.Equal(t, journey.Entries[1].BookID, journeyAfterStale.Entries[1].BookID)
	assert.Equal(t, journey.Entries[2].BookID, journeyAfterStale.Entries[2].BookID)
	foreignResponse := perform(t, h, "POST", "/journey/entries/"+foreign.ID+"/move-earlier", moveForm(csrf, fmt.Sprintf("%d", journey.Revision)), aliceCookies)
	assert.Equal(t, http.StatusNotFound, foreignResponse.Code)
	absent := perform(t, h, "POST", "/journey/entries/"+notMember.ID+"/move-earlier", moveForm(csrf, fmt.Sprintf("%d", journey.Revision)), aliceCookies)
	assert.Equal(t, http.StatusSeeOther, absent.Code)
	assert.True(t, strings.Contains(absent.Header().Get("Location"), "no+longer+in+your+Reading+Journey"), "location=%q", absent.Header().Get("Location"))
	missingCSRF := perform(t, h, "POST", "/journey/entries/"+first.ID+"/move-later", url.Values{"expected_revision": {fmt.Sprintf("%d", journey.Revision)}}, aliceCookies)
	assert.Equal(t, http.StatusForbidden, missingCSRF.Code)
	invalidCSRF := perform(t, h, "POST", "/journey/entries/"+first.ID+"/move-later", moveForm("invalid", fmt.Sprintf("%d", journey.Revision)), aliceCookies)
	assert.Equal(t, http.StatusForbidden, invalidCSRF.Code)
	page = perform(t, h, "GET", "/journey", nil, aliceCookies)
	form := moveForm(csrf, hiddenInputValue(t, page.Body.String(), "expected_revision"))
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/journey/entries/"+third.ID+"/move-earlier", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Hx-Request", "true")
	for _, cookie := range aliceCookies {
		request.AddCookie(cookie)
	}
	htmxRecorder := httptest.NewRecorder()
	h.ServeHTTP(htmxRecorder, request)
	assert.Equal(t, http.StatusOK, htmxRecorder.Code)
	assert.True(t, strings.Contains(htmxRecorder.Body.String(), `id="provisional-journey-list"`), "body=%s", htmxRecorder.Body.String())
	assert.True(t, strings.Contains(htmxRecorder.Body.String(), `aria-live="polite"`), "body=%s", htmxRecorder.Body.String())
	assert.False(t, strings.Contains(htmxRecorder.Body.String(), "<!doctype html>"), "body=%s", htmxRecorder.Body.String())

	journeyPage := perform(t, h, "GET", "/journey", nil, aliceCookies)
	journeyPageRevision := hiddenInputValue(t, journeyPage.Body.String(), "expected_revision")
	addForm := func(revision string) url.Values {
		return url.Values{"csrf_token": {csrf}, "expected_revision": {revision}, "deck_preparation_id": {"deck-for-" + notMember.ID}}
	}
	added := perform(t, h, "POST", "/journey/books/"+notMember.ID+"/add", addForm(journeyPageRevision), aliceCookies)
	assert.Equal(t, http.StatusSeeOther, added.Code)
	assert.True(t, strings.HasPrefix(added.Header().Get("Location"), "/journey?message="), "location=%q body=%s", added.Header().Get("Location"), added.Body.String())
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Len(t, journey.Entries, 5)
	repeated := perform(t, h, "POST", "/journey/books/"+notMember.ID+"/add", addForm(fmt.Sprintf("%d", journey.Revision)), aliceCookies)
	assert.Equal(t, http.StatusSeeOther, repeated.Code)
	assert.True(t, strings.Contains(repeated.Header().Get("Location"), "already+in+your+Reading+Journey"), "location=%q", repeated.Header().Get("Location"))
	staleAdd := perform(t, h, "POST", "/journey/books/"+notMember.ID+"/add", addForm(journeyPageRevision), aliceCookies)
	assert.Equal(t, http.StatusSeeOther, staleAdd.Code)
	assert.True(t, strings.Contains(staleAdd.Header().Get("Location"), "This+Journey+changed+since+this+page+was+loaded"), "location=%q", staleAdd.Header().Get("Location"))

	// The ready-deck surfaces post the source-material id, which differs from
	// the books.id that Journey membership stores (issue #506). The add must
	// resolve the source material to its linked book and persist that identity.
	deckBook := newBook(alice, "Deck-prepared provisional")
	var sourceID string
	err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,'application/epub+zip',$5,$6,$7) RETURNING id::text`, alice.ID, "de", "issue-506-identifier", "Deck-prepared provisional", "issue-506", "issue-506", "issue-506").Scan(&sourceID)
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, alice.ID, deckBook.ID, sourceID))
	_, err = store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: alice.ID, SourceMaterialID: sourceID, Filename: "issue-506.apkg", DeckName: "Issue 506 deck", ContentHash: "issue-506"})
	require.NoError(t, err)
	sourceJourneyPage := perform(t, h, "GET", "/journey", nil, aliceCookies)
	addedFromSource := perform(t, h, "POST", "/journey/books/"+sourceID+"/add", addForm(hiddenInputValue(t, sourceJourneyPage.Body.String(), "expected_revision")), aliceCookies)
	assert.Equal(t, http.StatusSeeOther, addedFromSource.Code)
	assert.True(t, strings.HasPrefix(addedFromSource.Header().Get("Location"), "/journey?message="), "location=%q body=%s", addedFromSource.Header().Get("Location"), addedFromSource.Body.String())
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Len(t, journey.Entries, 6)
	containsDeckBook := false
	for _, entry := range journey.Entries {
		if entry.BookID == deckBook.ID {
			containsDeckBook = true
			break
		}
	}
	assert.True(t, containsDeckBook, "source-material add did not persist book %s: %+v", deckBook.ID, journey.Entries)

	// Keep Bob's authenticated session in this test to exercise the owner
	// boundary through the same route.
	bobPage := perform(t, h, "GET", "/journey", nil, bobCookies)
	assert.Equal(t, http.StatusOK, bobPage.Code)
	assert.False(t, strings.Contains(bobPage.Body.String(), "Anchored Goal"), "body=%s", bobPage.Body.String())
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
func hiddenInputValue(t *testing.T, body, name string) string {
	values := hiddenInputValues(t, body, name)
	require.NotEmpty(t, values, "hidden input %s absent: %s", name, body)
	return values[0]
}
func hiddenInputValues(t *testing.T, body, name string) []string {
	t.Helper()
	pattern := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]+)"`)
	matches := pattern.FindAllStringSubmatch(body, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) == 2 {
			values = append(values, match[1])
		}
	}
	return values
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
