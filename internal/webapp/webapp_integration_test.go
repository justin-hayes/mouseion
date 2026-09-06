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
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/enrichmentjob"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river/rivertype"
)

type recordingAnalysis struct{ owner, source, scope string }
type recordingAnalysisInsights struct {
	owner, corpus string
	coverage      domain.AnalysisCoverage
}

func (r *recordingAnalysisInsights) Coverage(_ context.Context, owner, corpus string) (domain.AnalysisCoverage, error) {
	r.owner, r.corpus = owner, corpus
	return r.coverage, nil
}

func (r *recordingAnalysisInsights) JourneyProjection(context.Context, string, string) (domain.JourneyProjectionResult, error) {
	return domain.JourneyProjectionResult{}, nil
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
	if err != nil {
		t.Fatal(err)
	}
	u, err := store.CreateUserWithPassword(ctx, username, hash, legacyAdmin)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

type recordingKnownVocab struct {
	service *knownvocab.Service
	status  knownvocab.Status
	owner   string
}
type recordingEnrichment struct {
	owner      string
	candidates []enrichment.Candidate
	cancelled  bool
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
	p := domain.DeckPreparation{ID: "prep-1", OwnerID: owner, SourceMaterialID: "book-1", AnalysisRunID: analysisID, State: domain.DeckPreparationQueued, Filename: "Stored Book.apkg", DeckName: "Mouseion::de::Stored Book"}
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

func (r *recordingEnrichment) SubmitEnrichment(_ context.Context, owner string, candidates []enrichment.Candidate) (enrichmentjob.Handle, error) {
	r.owner, r.candidates = owner, append([]enrichment.Candidate(nil), candidates...)
	return enrichmentjob.Handle{ID: 88}, nil
}
func (r *recordingEnrichment) Get(_ context.Context, owner string, id int64) (enrichmentjob.Status, error) {
	if owner != r.owner || id != 88 {
		return enrichmentjob.Status{}, enrichmentjob.ErrNotFound
	}
	state := rivertype.JobStateRunning
	if r.cancelled {
		state = rivertype.JobStateCancelled
	}
	return enrichmentjob.Status{ID: 88, Completed: 1, Total: 2, State: state, Attempt: 2, Error: "temporary provider failure"}, nil
}
func (r *recordingEnrichment) Cancel(ctx context.Context, owner string, id int64) (enrichmentjob.Status, error) {
	if _, err := r.Get(ctx, owner, id); err != nil {
		return enrichmentjob.Status{}, err
	}
	r.cancelled = true
	return r.Get(ctx, owner, id)
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
	return analysis.Handle{ID: 42, DisplayNumber: 1}, nil
}

func (r *recordingAnalysis) SubmitScopedAnalysis(_ context.Context, owner, source, scope string) (analysis.Handle, error) {
	r.owner, r.source, r.scope = owner, source, scope
	return analysis.Handle{ID: 1, DisplayNumber: 1}, nil
}
func (r *recordingAnalysis) Get(_ context.Context, owner string, id int64) (analysis.Status, error) {
	if owner != r.owner || id != 42 {
		return analysis.Status{}, analysis.ErrNotFound
	}
	return analysis.Status{ID: 42, DisplayNumber: 1, State: rivertype.JobStateCompleted, Progress: 100, CorpusID: "corpus-result", Attempt: 1}, nil
}

func TestEPUBScopeReviewGermanItalianOverridesValidationOwnershipAndCSRF(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "scope-web-integration-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "scope-web-alice", "alice-password", false)
	createAccount(t, ctx, store, "scope-web-bob", "bob-password", false)

	units := domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{
		{ID: domain.EPUBUnitID(0, "chapter"), Order: 0, SpineIndex: 0, Text: "Erstes Kapitel.", EndOffset: 15, ManifestID: "chapter", MediaType: "application/xhtml+xml", Linear: true},
		{ID: domain.EPUBUnitID(1, "bibliography"), Order: 1, SpineIndex: 1, Text: "Bibliografia finale.", StartOffset: 17, EndOffset: 37, ManifestID: "bibliography", MediaType: "application/xhtml+xml", Linear: true},
	}}
	content := []byte("Erstes Kapitel.\n\nBibliografia finale.")
	german, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: alice.ID, Language: "de", SourceIdentifier: "analyze-de", Title: "Analyze book", MediaType: "application/epub+zip", Content: content, FullText: string(content),
	}, units)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{
		OwnerID: alice.ID, Language: "de", SourceIdentifier: "analyze-plain", Title: "Plain text", MediaType: "text/plain", Content: []byte("plain text"), FullText: "plain text",
	})
	if err != nil {
		t.Fatal(err)
	}
	missingUnits, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{
		OwnerID: alice.ID, Language: "de", SourceIdentifier: "analyze-missing", Title: "Missing units", MediaType: "application/epub+zip", Content: []byte("epub"), FullText: "epub",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []domain.SourceMaterial{german, plain, missingUnits} {
		bookID, resolveErr := store.ResolveOrCreateBookForAcquisition(ctx, alice.ID, source.SourceIdentifier, source.Language, source.Title)
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		if linkErr := store.LinkSourceToBook(ctx, alice.ID, bookID, source.ID); linkErr != nil {
			t.Fatal(linkErr)
		}
	}
	germanDetail, err := store.GetBookDetail(ctx, alice.ID, german.ID)
	if err != nil {
		t.Fatal(err)
	}

	recorder := &recordingAnalysis{}
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: store, Analysis: recorder, Capabilities: readyGerman(), SessionLifetime: time.Hour})
	cookies, csrf := loginCookies(t, h, "scope-web-alice", "alice-password")
	bobCookies, bobCSRF := loginCookies(t, h, "scope-web-bob", "bob-password")

	first := perform(t, h, "POST", "/books/"+germanDetail.Book.ID+"/analyze", url.Values{"csrf_token": {csrf}}, cookies)
	if first.Code != http.StatusSeeOther || recorder.scope == "" {
		t.Fatalf("first EPUB analysis=%d scope=%q body=%s", first.Code, recorder.scope, first.Body.String())
	}
	firstScopeID := recorder.scope
	snapshotID, extracted, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, german.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := store.GetEPUBReviewedScope(ctx, alice.ID, german.ID, firstScopeID)
	if err != nil {
		t.Fatal(err)
	}
	if scope.SourceContent.RevisionID != german.ContentRevisionID || scope.SourceContent.Digest != german.ContentDigest || scope.SourceContent.DigestVersion != german.ContentDigestVersion {
		t.Fatalf("scope content identity=%+v source=%+v", scope.SourceContent, german)
	}
	if scope.SourceUnitSnapshot.SnapshotID != snapshotID || scope.SourceUnitSnapshot.ExtractedUnitsSchemaVersion != extracted.SchemaVersion {
		t.Fatalf("scope snapshot identity=%+v snapshot=%s units=%+v", scope.SourceUnitSnapshot, snapshotID, extracted)
	}
	var expected []domain.EPUBSelectedUnitReference
	for _, unit := range extracted.Units {
		if strings.TrimSpace(unit.Text) != "" {
			expected = append(expected, domain.EPUBSelectedUnitReference{UnitID: unit.ID, Order: unit.Order})
		}
	}
	if !reflect.DeepEqual(scope.SelectedUnits, expected) {
		t.Fatalf("full-book scope selected=%+v want=%+v", scope.SelectedUnits, expected)
	}

	recorder.scope = ""
	second := perform(t, h, "POST", "/books/"+german.ID+"/analyze", url.Values{"csrf_token": {csrf}}, cookies)
	if second.Code != http.StatusSeeOther || recorder.scope != firstScopeID {
		t.Fatalf("idempotent EPUB analysis=%d scope=%q want=%q body=%s", second.Code, recorder.scope, firstScopeID, second.Body.String())
	}

	recorder.scope = "sentinel"
	plainResult := perform(t, h, "POST", "/books/"+plain.ID+"/analyze", url.Values{"csrf_token": {csrf}}, cookies)
	if plainResult.Code != http.StatusSeeOther || recorder.scope != "" || recorder.source != plain.ID {
		t.Fatalf("plain analysis=%d owner=%q source=%q scope=%q body=%s", plainResult.Code, recorder.owner, recorder.source, recorder.scope, plainResult.Body.String())
	}
	if csrfResult := perform(t, h, "POST", "/books/"+plain.ID+"/analyze", nil, cookies); csrfResult.Code != http.StatusForbidden {
		t.Fatalf("analyze without csrf=%d body=%s", csrfResult.Code, csrfResult.Body.String())
	}
	if got := perform(t, h, "GET", "/books/"+german.ID+"/analyze", nil, bobCookies); got.Code != http.StatusMethodNotAllowed {
		t.Fatalf("cross-owner analyze GET=%d body=%s", got.Code, got.Body.String())
	}
	if got := perform(t, h, "POST", "/books/"+germanDetail.Book.ID+"/analyze", url.Values{"csrf_token": {bobCSRF}}, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner analyze POST=%d body=%s", got.Code, got.Body.String())
	}
	missing := perform(t, h, "POST", "/books/"+missingUnits.ID+"/analyze", url.Values{"csrf_token": {csrf}}, cookies)
	if missing.Code != http.StatusConflict || !strings.Contains(missing.Body.String(), "no extracted EPUB units") {
		t.Fatalf("missing EPUB units=%d body=%s", missing.Code, missing.Body.String())
	}
}

func TestFirstAccountOnboardingAndExistingLogin(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "first-account-web-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: store, Capabilities: readyGerman(), SessionLifetime: time.Hour})

	page := perform(t, h, "GET", "/login", nil, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Create your account") || strings.Contains(page.Body.String(), `action="/login"`) {
		t.Fatalf("fresh login page=%d %s", page.Code, page.Body.String())
	}
	csrf := hiddenToken(t, page.Body.String())
	csrfCookieValue := cookieNamed(t, page.Result().Cookies(), csrfCookie)
	if got := perform(t, h, "POST", "/onboarding", url.Values{"username": {"alice"}, "password": {"alice-password"}}, nil); got.Code != http.StatusForbidden {
		t.Fatalf("onboarding without csrf=%d", got.Code)
	}
	created := perform(t, h, "POST", "/onboarding", url.Values{"csrf_token": {csrf}, "username": {"alice"}, "password": {"alice-password"}}, []*http.Cookie{csrfCookieValue})
	if created.Code != http.StatusSeeOther || created.Header().Get("Location") != "/" || cookieNamed(t, created.Result().Cookies(), webauth.CookieName).Value == "" {
		t.Fatalf("onboarding=%d location=%q cookies=%v body=%s", created.Code, created.Header().Get("Location"), created.Result().Cookies(), created.Body.String())
	}

	page = perform(t, h, "GET", "/login", nil, nil)
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), "Create your account") || !strings.Contains(page.Body.String(), `action="/login"`) {
		t.Fatalf("existing login page=%d %s", page.Code, page.Body.String())
	}
	navigationRequest := httptest.NewRequest("GET", "/library?sort=title", nil)
	navigationRequest.Header.Set("Accept", "text/html")
	navigationRequest.Header.Set("Sec-Fetch-Mode", "navigate")
	navigation := httptest.NewRecorder()
	h.ServeHTTP(navigation, navigationRequest)
	if navigation.Code != http.StatusSeeOther || navigation.Header().Get("Location") != "/login?next=%2Flibrary%3Fsort%3Dtitle" {
		t.Fatalf("unauthenticated navigation=%d location=%q body=%s", navigation.Code, navigation.Header().Get("Location"), navigation.Body.String())
	}
	returnPage := perform(t, h, "GET", navigation.Header().Get("Location"), nil, nil)
	if !strings.Contains(returnPage.Body.String(), `name="next" value="/library?sort=title"`) {
		t.Fatalf("login return path missing: %s", returnPage.Body.String())
	}
	returnCSRF := hiddenToken(t, returnPage.Body.String())
	returnCSRFCookie := cookieNamed(t, returnPage.Result().Cookies(), csrfCookie)
	returnedLogin := perform(t, h, "POST", "/login", url.Values{"csrf_token": {returnCSRF}, "next": {"/library?sort=title"}, "username": {"alice"}, "password": {"alice-password"}}, []*http.Cookie{returnCSRFCookie})
	if returnedLogin.Code != http.StatusSeeOther || returnedLogin.Header().Get("Location") != "/library?sort=title" {
		t.Fatalf("returned login=%d location=%q body=%s", returnedLogin.Code, returnedLogin.Header().Get("Location"), returnedLogin.Body.String())
	}
	csrf = hiddenToken(t, page.Body.String())
	csrfCookieValue = cookieNamed(t, page.Result().Cookies(), csrfCookie)
	blocked := perform(t, h, "POST", "/onboarding", url.Values{"csrf_token": {csrf}, "username": {"bob"}, "password": {"bob-password"}}, []*http.Cookie{csrfCookieValue})
	if blocked.Code != http.StatusNotFound {
		t.Fatalf("second onboarding=%d %s", blocked.Code, blocked.Body.String())
	}
	login := perform(t, h, "POST", "/login", url.Values{"csrf_token": {csrf}, "username": {"alice"}, "password": {"alice-password"}}, []*http.Cookie{csrfCookieValue})
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/" {
		t.Fatalf("existing login=%d location=%q body=%s", login.Code, login.Header().Get("Location"), login.Body.String())
	}
}

func TestKnownVocabImportUsesDerivedLibraryLanguages(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "known-vocab-derived-language-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "alice", "alice-password", false)
	if _, err = store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "German library book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"}); err != nil {
		t.Fatal(err)
	}
	known := &recordingKnownVocab{service: knownvocab.NewService(store)}
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: store, KnownVocab: known, SessionLifetime: time.Hour})
	cookies, csrf := loginCookies(t, h, "alice", "alice-password")

	imported := multipartUpload(t, h, "/vocabulary/import", cookies, map[string]string{"csrf_token": csrf, "language": "de"}, "Haus\n")
	if imported.Code != http.StatusSeeOther || imported.Header().Get("Location") != "/vocabulary/imports/77/status" {
		t.Fatalf("derived-language import=%d location=%q body=%s", imported.Code, imported.Header().Get("Location"), imported.Body.String())
	}
	if known.owner != alice.ID || known.status.Language != "de" {
		t.Fatalf("import was not submitted for derived language: owner=%q status=%+v", known.owner, known.status)
	}
}

func TestMetadataOnlyBookDetailAcquiresIntoExistingBook(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "metadata-acquisition-integration-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
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
		_, _ = w.Write(testEPUBVariant(t, "metadata-entry", "Metadata-only synced book", "Hallo Welt."))
	}))
	defer catalog.Close()
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Metadata catalog", URL: catalog.URL + "/opds"})
	if err != nil {
		t.Fatal(err)
	}
	bookResult, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, "metadata-entry", "Metadata-only synced book", "de")
	if err != nil {
		t.Fatal(err)
	}
	target := cataloguesync.AcquisitionTarget{
		ConnectionID: connection.ID,
		Language:     "de",
		Entry:        opds.Entry{ID: "metadata-entry", Title: "Metadata-only synced book", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: "/book.epub"}}},
		Href:         "/book.epub",
	}
	recorder := &recordingAnalysis{}
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: store,
		OPDS:     opds.NewService(store, epub.NewService(store), catalog.Client()),
		Analysis: recorder, CatalogueSync: metadataBookAcquisitionStub{target: target}, Capabilities: readyGerman(), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "owner-password")
	bookPage := perform(t, h, "GET", "/books/"+bookResult.Book.ID, nil, cookies)
	if bookPage.Code != http.StatusOK || !strings.Contains(bookPage.Body.String(), "Start analysis") || strings.Contains(bookPage.Body.String(), "Acquire EPUB content") {
		t.Fatalf("metadata-only book page=%d %s", bookPage.Code, bookPage.Body.String())
	}
	started := perform(t, h, "POST", "/books/"+bookResult.Book.ID+"/analyze", url.Values{"csrf_token": {csrf}}, cookies)
	if started.Code != http.StatusSeeOther || recorder.scope == "" || downloads != 1 {
		t.Fatalf("one-click analysis=%d scope=%q downloads=%d body=%s", started.Code, recorder.scope, downloads, started.Body.String())
	}
	retried := perform(t, h, "POST", "/books/"+bookResult.Book.ID+"/analyze", url.Values{"csrf_token": {csrf}}, cookies)
	if retried.Code != http.StatusSeeOther || downloads != 1 {
		t.Fatalf("re-analysis=%d downloads=%d body=%s", retried.Code, downloads, retried.Body.String())
	}
	acquiredPage := perform(t, h, "GET", "/books/"+bookResult.Book.ID, nil, cookies)
	if acquiredPage.Code != http.StatusOK || !strings.Contains(acquiredPage.Body.String(), "Metadata-only synced book") || strings.Contains(acquiredPage.Body.String(), "Acquire EPUB content") {
		t.Fatalf("canonical acquired book page=%d body=%s", acquiredPage.Code, acquiredPage.Body.String())
	}
	sources, err := store.ListSourceMaterials(ctx, owner.ID)
	if err != nil || len(sources) != 1 || sources[0].BookID != bookResult.Book.ID {
		t.Fatalf("promoted sources=%+v err=%v", sources, err)
	}
	legacyPage := perform(t, h, "GET", "/books/"+sources[0].Source.ID, nil, cookies)
	if legacyPage.Code != http.StatusOK || !strings.Contains(legacyPage.Body.String(), "Metadata-only synced book") {
		t.Fatalf("legacy acquired book page=%d body=%s", legacyPage.Code, legacyPage.Body.String())
	}
	books, err := store.ListMyBooksWithEvidence(ctx, owner.ID)
	if err != nil || len(books) != 1 || books[0].Book.ID != bookResult.Book.ID || books[0].Acquired == nil {
		t.Fatalf("promoted My Books=%+v err=%v", books, err)
	}
}

func TestPreparedDeckWebLifecycleOwnershipAndPureDownload(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "prepared-deck-web-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authService := auth.New(store, time.Hour)
	createAccount(t, ctx, store, "alice", "alice-password", false)
	createAccount(t, ctx, store, "bob", "bob-password", false)
	decks := &recordingPreparedDeck{preparations: make(map[string]domain.DeckPreparation)}
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: store, PreparedDeck: decks, Capabilities: readyGerman(), SessionLifetime: time.Hour})
	aliceCookies, aliceCSRF := loginCookies(t, h, "alice", "alice-password")
	bobCookies, bobCSRF := loginCookies(t, h, "bob", "bob-password")

	if got := perform(t, h, "POST", "/jobs/42/deck/preparations", nil, aliceCookies); got.Code != http.StatusForbidden {
		t.Fatalf("create without csrf=%d", got.Code)
	}
	created := perform(t, h, "POST", "/jobs/42/deck/preparations", url.Values{"csrf_token": {aliceCSRF}, "external_translation_consent": {"on"}}, aliceCookies)
	if created.Code != http.StatusSeeOther || created.Header().Get("Location") != "/deck-preparations/prep-1/status" || !decks.consent {
		t.Fatalf("create=%d location=%q consent=%v", created.Code, created.Header().Get("Location"), decks.consent)
	}
	statusPage := perform(t, h, "GET", created.Header().Get("Location"), nil, aliceCookies)
	if statusPage.Code != http.StatusOK || !strings.Contains(statusPage.Body.String(), "Deck preparation queued") || !strings.Contains(statusPage.Body.String(), "Cancel preparation") {
		t.Fatalf("server-rendered status=%d %s", statusPage.Code, statusPage.Body.String())
	}
	status := perform(t, h, "GET", created.Header().Get("Location")+"?format=json", nil, aliceCookies)
	if status.Code != http.StatusOK || status.Header().Get("Content-Type") != "application/json; charset=utf-8" || !strings.Contains(status.Body.String(), `"state":"queued"`) || !strings.Contains(status.Body.String(), `"progress":0`) {
		t.Fatalf("status=%d headers=%v body=%s", status.Code, status.Header(), status.Body.String())
	}
	if got := perform(t, h, "GET", "/deck-preparations/prep-1/status", nil, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner status=%d", got.Code)
	}
	if got := perform(t, h, "GET", "/deck-preparations/missing/status", nil, aliceCookies); got.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d", got.Code)
	}
	if got := perform(t, h, "GET", "/deck-preparations/prep-1/download", nil, aliceCookies); got.Code != http.StatusConflict {
		t.Fatalf("non-ready download=%d", got.Code)
	}
	if got := perform(t, h, "POST", "/deck-preparations/prep-1/cancel", nil, aliceCookies); got.Code != http.StatusForbidden {
		t.Fatalf("cancel without csrf=%d", got.Code)
	}
	cancelled := perform(t, h, "POST", "/deck-preparations/prep-1/cancel?format=json", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
	if cancelled.Code != http.StatusOK || !strings.Contains(cancelled.Body.String(), `"state":"cancelled"`) {
		t.Fatalf("cancel=%d %s", cancelled.Code, cancelled.Body.String())
	}
	if got := perform(t, h, "POST", "/deck-preparations/prep-1/retry", nil, aliceCookies); got.Code != http.StatusForbidden {
		t.Fatalf("retry without csrf=%d", got.Code)
	}
	retried := perform(t, h, "POST", "/deck-preparations/prep-1/retry?format=json", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
	if retried.Code != http.StatusOK || !strings.Contains(retried.Body.String(), `"state":"queued"`) {
		t.Fatalf("retry=%d %s", retried.Code, retried.Body.String())
	}
	if got := perform(t, h, "POST", "/deck-preparations/prep-1/cancel", url.Values{"csrf_token": {bobCSRF}}, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner cancel=%d", got.Code)
	}

	ready := decks.preparations["prep-1"]
	ready.State, ready.Artifact = domain.DeckPreparationReady, []byte("immutable-apkg")
	ready.TotalCards, ready.CardsWithEnglish, ready.CardsWithContextualSentenceTranslations, ready.QualityOmissions = 7, 6, 5, 2
	decks.preparations[ready.ID] = ready
	for i := 0; i < 2; i++ {
		download := perform(t, h, "GET", "/deck-preparations/prep-1/download", nil, aliceCookies)
		if download.Code != http.StatusOK || download.Body.String() != "immutable-apkg" || download.Header().Get("Content-Type") != "application/vnd.anki" || !strings.Contains(download.Header().Get("Content-Disposition"), `filename="Stored Book.apkg"`) || download.Header().Get("X-Mouseion-Deck-Name") != "Mouseion::de::Stored Book" || download.Header().Get("X-Mouseion-Cards-Total") != "7" || download.Header().Get("X-Mouseion-Cards-With-English") != "6" || download.Header().Get("X-Mouseion-Cards-With-English-Sentence") != "5" || download.Header().Get("X-Mouseion-Cards-Quality-Omitted") != "2" {
			t.Fatalf("download %d=%d headers=%v body=%q", i, download.Code, download.Header(), download.Body.String())
		}
	}
	if decks.downloads != 2 || decks.preparations["prep-1"].State != domain.DeckPreparationReady {
		t.Fatalf("downloads=%d state=%s", decks.downloads, decks.preparations["prep-1"].State)
	}
	if got := perform(t, h, "GET", "/deck-preparations/prep-1/download", nil, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner download=%d", got.Code)
	}
}

func TestLearningCampaignQueueViewsActivationAndOwnership(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "campaign-web-secret-0123456789ab")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "campaign-web-alice", "alice-password", false)
	createAccount(t, ctx, store, "campaign-web-bob", "bob-password", false)
	decks := &recordingPreparedDeck{preparations: make(map[string]domain.DeckPreparation)}
	for i, title := range []string{"Completed Book", "Abandoned Book", "Active Book", "Queued Book"} {
		source, sourceErr := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: fmt.Sprintf("campaign-web-%d", i), Title: title, MediaType: "text/plain", ContentHash: fmt.Sprintf("campaign-web-hash-%d", i), Content: []byte(title), FullText: title})
		if sourceErr != nil {
			t.Fatal(sourceErr)
		}
		preparation, prepErr := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: alice.ID, SourceMaterialID: source.ID, Filename: fmt.Sprintf("book-%d.apkg", i), DeckName: "Mouseion::de::" + title, ContentHash: source.ContentHash})
		if prepErr != nil {
			t.Fatal(prepErr)
		}
		if preparation, prepErr = store.ClaimDeckPreparation(ctx, alice.ID, preparation.ID); prepErr != nil {
			t.Fatal(prepErr)
		}
		preparation, prepErr = store.CompleteDeckPreparation(ctx, alice.ID, preparation.ID, domain.DeckPreparation{Artifact: []byte("apkg"), Filename: preparation.Filename, DeckName: preparation.DeckName, TotalCards: i + 1})
		if prepErr != nil {
			t.Fatal(prepErr)
		}
		decks.preparations[preparation.ID] = preparation
		if title == "Completed Book" {
			deck, deckErr := store.PutDeck(ctx, alice.ID, "de", "Campaign completion")
			if deckErr != nil {
				t.Fatal(deckErr)
			}
			if _, generatedErr := store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: "lernen", UPOS: "VERB", FirstDeckID: deck.ID, FirstSourceMaterialID: &source.ID}); generatedErr != nil {
				t.Fatal(generatedErr)
			}
		}
	}
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: store, PreparedDeck: decks, Capabilities: readyGerman(), SessionLifetime: time.Hour})
	aliceCookies, aliceCSRF := loginCookies(t, h, "campaign-web-alice", "alice-password")
	bobCookies, bobCSRF := loginCookies(t, h, "campaign-web-bob", "bob-password")

	page := perform(t, h, "GET", "/journey", nil, aliceCookies)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Prepared books") || !strings.Contains(page.Body.String(), "Completed Book") || !strings.Contains(page.Body.String(), "Deck ready") {
		t.Fatalf("prepared campaign page=%d %s", page.Code, page.Body.String())
	}
	ready, err := store.ListUnassignedReadyDeckPreparations(ctx, alice.ID)
	if err != nil || len(ready) != 4 {
		t.Fatalf("ready preparations=%+v, %v", ready, err)
	}
	if got := perform(t, h, "POST", "/campaigns", url.Values{"csrf_token": {aliceCSRF}, "deck_preparation_id": {ready[0].ID}}, aliceCookies); got.Code != http.StatusMethodNotAllowed {
		t.Fatalf("ready deck campaign creation route=%d, want 405", got.Code)
	}
	for _, preparation := range ready {
		if _, err = store.CreateLearningCampaign(ctx, alice.ID, preparation.SourceMaterialID, preparation.ID); err != nil {
			t.Fatalf("campaign fixture setup=%v", err)
		}
	}
	campaigns, err := store.ListLearningCampaigns(ctx, alice.ID)
	if err != nil || len(campaigns) != 4 {
		t.Fatalf("campaigns=%+v, %v", campaigns, err)
	}
	campaignNamed := func(title string) domain.LearningCampaign {
		for _, campaign := range campaigns {
			if decks.preparations[campaign.DeckPreparationID].DeckName == "Mouseion::de::"+title {
				return campaign
			}
		}
		t.Fatalf("campaign for %q not found", title)
		return domain.LearningCampaign{}
	}
	completedCampaign := campaignNamed("Completed Book")
	abandonedCampaign := campaignNamed("Abandoned Book")
	activeCampaign := campaignNamed("Active Book")
	queuedCampaign := campaignNamed("Queued Book")
	if got := perform(t, h, "POST", "/campaigns/"+campaigns[0].ID+"/activate", nil, aliceCookies); got.Code != http.StatusForbidden {
		t.Fatalf("activate without csrf=%d", got.Code)
	}
	activate := func(campaign domain.LearningCampaign) {
		response := perform(t, h, "POST", "/campaigns/"+campaign.ID+"/activate", campaignForm(aliceCSRF, campaign), aliceCookies)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("activate %s=%d %s", campaign.ID, response.Code, response.Body.String())
		}
	}
	activate(completedCampaign)
	stale := perform(t, h, "POST", "/campaigns/"+completedCampaign.ID+"/book-finished", campaignForm(aliceCSRF, completedCampaign), aliceCookies)
	if stale.Code != http.StatusSeeOther || !strings.Contains(stale.Header().Get("Location"), "Only+the+active+campaign+can+be+updated.") {
		t.Fatalf("stale campaign form=%d location=%q", stale.Code, stale.Header().Get("Location"))
	}
	conflict := perform(t, h, "POST", "/campaigns/"+abandonedCampaign.ID+"/activate", campaignForm(aliceCSRF, abandonedCampaign), aliceCookies)
	if conflict.Code != http.StatusSeeOther || !strings.Contains(conflict.Header().Get("Location"), "Finish+or+abandon") {
		t.Fatalf("second active=%d location=%q", conflict.Code, conflict.Header().Get("Location"))
	}
	if got := perform(t, h, "POST", "/campaigns/"+completedCampaign.ID+"/book-finished", nil, aliceCookies); got.Code != http.StatusForbidden {
		t.Fatalf("finish book without csrf=%d", got.Code)
	}
	finished := perform(t, h, "POST", "/campaigns/"+completedCampaign.ID+"/book-finished", campaignForm(aliceCSRF, domain.LearningCampaign{ID: completedCampaign.ID, Status: domain.CampaignActive, BookProgress: domain.BookReading, DeckProgress: domain.DeckStudying}), aliceCookies)
	if finished.Code != http.StatusSeeOther || !strings.Contains(finished.Header().Get("Location"), "Book+marked+finished") {
		t.Fatalf("finish book=%d location=%q", finished.Code, finished.Header().Get("Location"))
	}
	progressPage := perform(t, h, "GET", "/journey", nil, aliceCookies)
	if body := progressPage.Body.String(); !strings.Contains(body, "Book</dt><dd>Finished · ") || strings.Contains(body, "Mark book finished") || !strings.Contains(body, "Complete campaign and add") {
		t.Fatalf("book progress page=%s", body)
	}
	if got := perform(t, h, "POST", "/campaigns/"+completedCampaign.ID+"/deck-reviewed", campaignForm(bobCSRF, domain.LearningCampaign{ID: completedCampaign.ID, Status: domain.CampaignActive, BookProgress: domain.BookFinished, DeckProgress: domain.DeckStudying}), bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner review=%d", got.Code)
	}
	reviewed := perform(t, h, "POST", "/campaigns/"+completedCampaign.ID+"/deck-reviewed", campaignForm(aliceCSRF, domain.LearningCampaign{ID: completedCampaign.ID, Status: domain.CampaignActive, BookProgress: domain.BookFinished, DeckProgress: domain.DeckStudying}), aliceCookies)
	if reviewed.Code != http.StatusSeeOther || !strings.Contains(reviewed.Header().Get("Location"), "Campaign+complete") {
		t.Fatalf("review deck=%d location=%q", reviewed.Code, reviewed.Header().Get("Location"))
	}
	var known, generated int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND canonical_lemma='lernen'`, alice.ID).Scan(&known); err != nil || known != 1 {
		t.Fatalf("graduated known vocabulary=%d err=%v", known, err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='lernen'`, alice.ID).Scan(&generated); err != nil || generated != 1 {
		t.Fatalf("generated history=%d err=%v", generated, err)
	}
	repeated := perform(t, h, "POST", "/campaigns/"+completedCampaign.ID+"/deck-reviewed", campaignForm(aliceCSRF, domain.LearningCampaign{ID: completedCampaign.ID, Status: domain.CampaignComplete, BookProgress: domain.BookFinished, DeckProgress: domain.DeckReviewed}), aliceCookies)
	if repeated.Code != http.StatusSeeOther || !strings.Contains(repeated.Header().Get("Location"), "Only+the+active") {
		t.Fatalf("repeat review=%d location=%q", repeated.Code, repeated.Header().Get("Location"))
	}
	activate(abandonedCampaign)
	if got := perform(t, h, "POST", "/campaigns/"+abandonedCampaign.ID+"/abandon", nil, aliceCookies); got.Code != http.StatusForbidden {
		t.Fatalf("abandon without csrf=%d", got.Code)
	}
	if got := perform(t, h, "POST", "/campaigns/"+abandonedCampaign.ID+"/abandon", campaignForm(bobCSRF, domain.LearningCampaign{ID: abandonedCampaign.ID, Status: domain.CampaignActive, BookProgress: domain.BookReading, DeckProgress: domain.DeckStudying}), bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner abandon=%d", got.Code)
	}
	abandoned := perform(t, h, "POST", "/campaigns/"+abandonedCampaign.ID+"/abandon", campaignForm(aliceCSRF, domain.LearningCampaign{ID: abandonedCampaign.ID, Status: domain.CampaignActive, BookProgress: domain.BookReading, DeckProgress: domain.DeckStudying}), aliceCookies)
	if abandoned.Code != http.StatusSeeOther || !strings.Contains(abandoned.Header().Get("Location"), "Campaign+abandoned") {
		t.Fatalf("abandon=%d location=%q", abandoned.Code, abandoned.Header().Get("Location"))
	}
	repeatedAbandon := perform(t, h, "POST", "/campaigns/"+abandonedCampaign.ID+"/abandon", campaignForm(aliceCSRF, domain.LearningCampaign{ID: abandonedCampaign.ID, Status: domain.CampaignAbandoned, BookProgress: domain.BookAbandoned, DeckProgress: domain.DeckAbandoned}), aliceCookies)
	if repeatedAbandon.Code != http.StatusSeeOther || !strings.Contains(repeatedAbandon.Header().Get("Location"), "Only+an+active+or+prepared+campaign+can+be+abandoned.") {
		t.Fatalf("repeat abandon=%d location=%q", repeatedAbandon.Code, repeatedAbandon.Header().Get("Location"))
	}
	activate(activeCampaign)

	page = perform(t, h, "GET", "/journey", nil, aliceCookies)
	body := page.Body.String()
	for _, expected := range []string{"Campaign history &amp; operations", "Completed Book", "Abandoned Book", "Active Book", "Queued Book", ">Complete<", ">Abandoned<", ">Active<", ">Prepared<", "Book</dt><dd>Reading", "Deck</dt><dd>Studying", "Completed 20", "Mark book finished", "Mark deck reviewed", "Abandon campaign"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("campaign page missing %q: %s", expected, body)
		}
	}
	if got := perform(t, h, "POST", "/campaigns/"+queuedCampaign.ID+"/activate", campaignForm(bobCSRF, queuedCampaign), bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner activation=%d", got.Code)
	}
	bobPage := perform(t, h, "GET", "/journey", nil, bobCookies)
	if bobPage.Code != http.StatusOK || strings.Contains(bobPage.Body.String(), "Active Book") {
		t.Fatalf("bob campaign page=%d %s", bobPage.Code, bobPage.Body.String())
	}
	queuedAbandoned := perform(t, h, "POST", "/campaigns/"+queuedCampaign.ID+"/abandon", campaignForm(aliceCSRF, queuedCampaign), aliceCookies)
	if queuedAbandoned.Code != http.StatusSeeOther || !strings.Contains(queuedAbandoned.Header().Get("Location"), "Campaign+abandoned") {
		t.Fatalf("queued abandon=%d location=%q", queuedAbandoned.Code, queuedAbandoned.Header().Get("Location"))
	}
	if queuedCampaign, err = store.GetLearningCampaign(ctx, alice.ID, queuedCampaign.ID); err != nil || queuedCampaign.Status != domain.CampaignAbandoned {
		t.Fatalf("queued campaign after endpoint abandonment=%+v err=%v", queuedCampaign, err)
	}
}

func campaignForm(csrf string, campaign domain.LearningCampaign) url.Values {
	return url.Values{
		"csrf_token":               {csrf},
		"expected_campaign_status": {string(campaign.Status)},
		"expected_book_progress":   {string(campaign.BookProgress)},
		"expected_deck_progress":   {string(campaign.DeckProgress)},
	}
}

func TestJourneyReorderingEndpointsAreOwnerScopedAndStaleSafe(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "journey-reorder-web-integration-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "journey-web-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "journey-web-bob", "bob-password", false)
	newBook := func(owner domain.User, title string) domain.Book {
		book, createErr := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return book
	}
	goal := newBook(alice, "Anchored Goal")
	first := newBook(alice, "First provisional")
	second := newBook(alice, "Second provisional")
	third := newBook(alice, "Third provisional")
	foreign := newBook(bob, "Foreign provisional")
	notMember := newBook(alice, "Removed provisional")
	if _, err = store.CreatePrimaryGoal(ctx, alice.ID, goal.ID); err != nil {
		t.Fatal(err)
	}
	journeyRevision := int64(0)
	for _, book := range []domain.Book{first, second, third} {
		journeyRevision, err = store.AddToReadingJourney(ctx, alice.ID, book.ID, journeyRevision)
		if err != nil {
			t.Fatal(err)
		}
	}
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: store, SessionLifetime: time.Hour})
	aliceCookies, csrf := loginCookies(t, h, alice.Username, "alice-password")
	bobCookies, _ := loginCookies(t, h, bob.Username, "bob-password")
	page := perform(t, h, "GET", "/journey", nil, aliceCookies)
	if page.Code != http.StatusOK {
		t.Fatalf("journey page=%d %s", page.Code, page.Body.String())
	}
	expected := hiddenInputValue(t, page.Body.String(), "expected_revision")
	moveForm := func(token, revision string) url.Values {
		return url.Values{"csrf_token": {token}, "expected_revision": {revision}}
	}
	if moved := perform(t, h, "POST", "/journey/entries/"+second.ID+"/move-earlier", moveForm(csrf, expected), aliceCookies); moved.Code != http.StatusSeeOther || !strings.HasPrefix(moved.Header().Get("Location"), "/journey?message=") {
		t.Fatalf("move earlier=%d location=%q body=%s", moved.Code, moved.Header().Get("Location"), moved.Body.String())
	}
	journey, err := store.GetReadingJourney(ctx, alice.ID)
	if err != nil || journey.Entries[0].BookID != second.ID || journey.Entries[1].BookID != first.ID {
		t.Fatalf("after move earlier journey=%+v err=%v", journey.Entries, err)
	}
	page = perform(t, h, "GET", "/journey", nil, aliceCookies)
	if moved := perform(t, h, "POST", "/journey/entries/"+second.ID+"/move-later", moveForm(csrf, hiddenInputValue(t, page.Body.String(), "expected_revision")), aliceCookies); moved.Code != http.StatusSeeOther || !strings.HasPrefix(moved.Header().Get("Location"), "/journey?message=") {
		t.Fatalf("move later=%d location=%q", moved.Code, moved.Header().Get("Location"))
	}
	journey, _ = store.GetReadingJourney(ctx, alice.ID)
	if journey.Entries[0].BookID != first.ID || journey.Entries[1].BookID != second.ID {
		t.Fatalf("after move later journey=%+v", journey.Entries)
	}
	staleRevision := hiddenInputValue(t, page.Body.String(), "expected_revision")
	stale := perform(t, h, "POST", "/journey/entries/"+third.ID+"/move-earlier", moveForm(csrf, staleRevision), aliceCookies)
	if stale.Code != http.StatusSeeOther || !strings.Contains(stale.Header().Get("Location"), "This+Journey+changed+since+this+page+was+loaded") {
		t.Fatalf("stale=%d location=%q", stale.Code, stale.Header().Get("Location"))
	}
	journeyAfterStale, _ := store.GetReadingJourney(ctx, alice.ID)
	if len(journeyAfterStale.Entries) != len(journey.Entries) || journeyAfterStale.Entries[0].BookID != journey.Entries[0].BookID || journeyAfterStale.Entries[1].BookID != journey.Entries[1].BookID || journeyAfterStale.Entries[2].BookID != journey.Entries[2].BookID {
		t.Fatalf("stale request changed journey=%+v before=%+v", journeyAfterStale.Entries, journey.Entries)
	}
	if foreignResponse := perform(t, h, "POST", "/journey/entries/"+foreign.ID+"/move-earlier", moveForm(csrf, fmt.Sprintf("%d", journey.Revision)), aliceCookies); foreignResponse.Code != http.StatusNotFound {
		t.Fatalf("cross-owner move=%d", foreignResponse.Code)
	}
	if absent := perform(t, h, "POST", "/journey/entries/"+notMember.ID+"/move-earlier", moveForm(csrf, fmt.Sprintf("%d", journey.Revision)), aliceCookies); absent.Code != http.StatusSeeOther || !strings.Contains(absent.Header().Get("Location"), "no+longer+in+your+Reading+Journey") {
		t.Fatalf("non-member move=%d location=%q", absent.Code, absent.Header().Get("Location"))
	}
	if missingCSRF := perform(t, h, "POST", "/journey/entries/"+first.ID+"/move-later", url.Values{"expected_revision": {fmt.Sprintf("%d", journey.Revision)}}, aliceCookies); missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing csrf=%d", missingCSRF.Code)
	}
	if invalidCSRF := perform(t, h, "POST", "/journey/entries/"+first.ID+"/move-later", moveForm("invalid", fmt.Sprintf("%d", journey.Revision)), aliceCookies); invalidCSRF.Code != http.StatusForbidden {
		t.Fatalf("invalid csrf=%d", invalidCSRF.Code)
	}
	page = perform(t, h, "GET", "/journey", nil, aliceCookies)
	form := moveForm(csrf, hiddenInputValue(t, page.Body.String(), "expected_revision"))
	request := httptest.NewRequest(http.MethodPost, "/journey/entries/"+third.ID+"/move-earlier", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("HX-Request", "true")
	for _, cookie := range aliceCookies {
		request.AddCookie(cookie)
	}
	htmxRecorder := httptest.NewRecorder()
	h.ServeHTTP(htmxRecorder, request)
	if htmxRecorder.Code != http.StatusOK || !strings.Contains(htmxRecorder.Body.String(), `id="provisional-journey-list"`) || !strings.Contains(htmxRecorder.Body.String(), `aria-live="polite"`) || strings.Contains(htmxRecorder.Body.String(), "<!doctype html>") {
		t.Fatalf("htmx reorder=%d body=%s", htmxRecorder.Code, htmxRecorder.Body.String())
	}

	journeyPage := perform(t, h, "GET", "/journey", nil, aliceCookies)
	journeyPageRevision := hiddenInputValue(t, journeyPage.Body.String(), "expected_revision")
	addForm := func(revision string) url.Values {
		return url.Values{"csrf_token": {csrf}, "expected_revision": {revision}, "deck_preparation_id": {"deck-for-" + notMember.ID}}
	}
	added := perform(t, h, "POST", "/journey/books/"+notMember.ID+"/add", addForm(journeyPageRevision), aliceCookies)
	if added.Code != http.StatusSeeOther || !strings.HasPrefix(added.Header().Get("Location"), "/journey?message=") {
		t.Fatalf("add to Journey=%d location=%q body=%s", added.Code, added.Header().Get("Location"), added.Body.String())
	}
	journey, err = store.GetReadingJourney(ctx, alice.ID)
	if err != nil || len(journey.Entries) != 4 {
		t.Fatalf("added Journey=%+v err=%v", journey.Entries, err)
	}
	repeated := perform(t, h, "POST", "/journey/books/"+notMember.ID+"/add", addForm(fmt.Sprintf("%d", journey.Revision)), aliceCookies)
	if repeated.Code != http.StatusSeeOther || !strings.Contains(repeated.Header().Get("Location"), "already+in+your+Reading+Journey") {
		t.Fatalf("idempotent add=%d location=%q", repeated.Code, repeated.Header().Get("Location"))
	}
	staleAdd := perform(t, h, "POST", "/journey/books/"+notMember.ID+"/add", addForm(journeyPageRevision), aliceCookies)
	if staleAdd.Code != http.StatusSeeOther || !strings.Contains(staleAdd.Header().Get("Location"), "This+Journey+changed+since+this+page+was+loaded") {
		t.Fatalf("stale add=%d location=%q", staleAdd.Code, staleAdd.Header().Get("Location"))
	}

	// The ready-deck surfaces post the source-material id, which differs from
	// the books.id that Journey membership stores (issue #506). The add must
	// resolve the source material to its linked book and persist that identity.
	deckBook := newBook(alice, "Deck-prepared provisional")
	var sourceID string
	if err = store.Pool().QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,'application/epub+zip',$5,$6,$7) RETURNING id::text`, alice.ID, "de", "issue-506-identifier", "Deck-prepared provisional", "issue-506", "issue-506", "issue-506").Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	if err = store.LinkSourceToBook(ctx, alice.ID, deckBook.ID, sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: alice.ID, SourceMaterialID: sourceID, Filename: "issue-506.apkg", DeckName: "Issue 506 deck", ContentHash: "issue-506"}); err != nil {
		t.Fatal(err)
	}
	sourceJourneyPage := perform(t, h, "GET", "/journey", nil, aliceCookies)
	addedFromSource := perform(t, h, "POST", "/journey/books/"+sourceID+"/add", addForm(hiddenInputValue(t, sourceJourneyPage.Body.String(), "expected_revision")), aliceCookies)
	if addedFromSource.Code != http.StatusSeeOther || !strings.HasPrefix(addedFromSource.Header().Get("Location"), "/journey?message=") {
		t.Fatalf("source-material add=%d location=%q body=%s", addedFromSource.Code, addedFromSource.Header().Get("Location"), addedFromSource.Body.String())
	}
	journey, err = store.GetReadingJourney(ctx, alice.ID)
	if err != nil || len(journey.Entries) != 5 {
		t.Fatalf("source-material add Journey=%+v err=%v", journey.Entries, err)
	}
	containsDeckBook := false
	for _, entry := range journey.Entries {
		if entry.BookID == deckBook.ID {
			containsDeckBook = true
			break
		}
	}
	if !containsDeckBook {
		t.Fatalf("source-material add did not persist book %s: %+v", deckBook.ID, journey.Entries)
	}

	// Keep Bob's authenticated session in this test to exercise the owner
	// boundary through the same route.
	if bobPage := perform(t, h, "GET", "/journey", nil, bobCookies); bobPage.Code != http.StatusOK || strings.Contains(bobPage.Body.String(), "Anchored Goal") {
		t.Fatalf("bob journey=%d %s", bobPage.Code, bobPage.Body.String())
	}
}

func loginCookies(t *testing.T, h http.Handler, username, password string) ([]*http.Cookie, string) {
	page := perform(t, h, "GET", "/login", nil, nil)
	token := hiddenToken(t, page.Body.String())
	csrfCookieValue := cookieNamed(t, page.Result().Cookies(), csrfCookie)
	response := perform(t, h, "POST", "/login", url.Values{"csrf_token": {token}, "username": {username}, "password": {password}}, []*http.Cookie{csrfCookieValue})
	if response.Code != http.StatusSeeOther {
		t.Fatalf("login %s=%d %s", username, response.Code, response.Body.String())
	}
	csrfCookieValue = cookieNamed(t, response.Result().Cookies(), csrfCookie)
	return []*http.Cookie{csrfCookieValue, cookieNamed(t, response.Result().Cookies(), webauth.CookieName)}, csrfCookieValue.Value
}
func multipartUpload(t *testing.T, h http.Handler, path string, cookies []*http.Cookie, fields map[string]string, content string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	field, filename := "dataset", "frequency.csv"
	if path == "/vocabulary/import" {
		field, filename = "vocabulary_file", "known.txt"
	}
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(part, content); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", path, &body)
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
	r := httptest.NewRequest(method, path, body)
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
	if len(match) != 2 {
		t.Fatalf("csrf token absent: %s", body)
	}
	return match[1]
}
func hiddenInputValue(t *testing.T, body, name string) string {
	values := hiddenInputValues(t, body, name)
	if len(values) == 0 {
		t.Fatalf("hidden input %s absent: %s", name, body)
	}
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
	t.Fatalf("cookie %s absent", name)
	return nil
}
func testEPUB(t *testing.T) []byte {
	return testEPUBVariant(t, "book-1", "Test Book", "Hallo Welt.")
}
func testEPUBVariant(t *testing.T, identifier, title, text string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	files := map[string]string{"mimetype": "application/epub+zip", "META-INF/container.xml": `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`, `OEBPS/content.opf`: fmt.Sprintf(`<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">%s</dc:identifier><dc:title>%s</dc:title></metadata><manifest><item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="chapter"/></spine></package>`, identifier, title), `OEBPS/chapter.xhtml`: fmt.Sprintf(`<html xmlns="http://www.w3.org/1999/xhtml"><body><p>%s</p></body></html>`, text)}
	for name, content := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
