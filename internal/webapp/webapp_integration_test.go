//go:build integration

package webapp

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
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

type recordingAnalysis struct{ owner, source string }
type recordingAnalysisInsights struct {
	owner, corpus string
	coverage      domain.AnalysisCoverage
}

func (r *recordingAnalysisInsights) Coverage(_ context.Context, owner, corpus string) (domain.AnalysisCoverage, error) {
	r.owner, r.corpus = owner, corpus
	return r.coverage, nil
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

func (r *recordingPreparedDeck) Submit(_ context.Context, owner, source string, consent bool) (prepareddeck.Handle, error) {
	r.consent = consent
	for _, p := range r.preparations {
		if p.OwnerID == owner && p.SourceMaterialID == source {
			return prepareddeck.Handle{Preparation: p, JobID: 91}, nil
		}
	}
	p := domain.DeckPreparation{ID: "prep-1", OwnerID: owner, SourceMaterialID: source, State: domain.DeckPreparationQueued, Filename: "Stored Book.apkg", DeckName: "Mouseion::de::Stored Book"}
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
	r.owner, r.source = owner, source
	return analysis.Handle{ID: 42, DisplayNumber: 1}, nil
}
func (r *recordingAnalysis) Get(_ context.Context, owner string, id int64) (analysis.Status, error) {
	if owner != r.owner || id != 42 {
		return analysis.Status{}, analysis.ErrNotFound
	}
	return analysis.Status{ID: 42, DisplayNumber: 1, State: rivertype.JobStateCompleted, Progress: 100, CorpusID: "corpus-result", Attempt: 1}, nil
}

func TestFirstAccountOnboardingAndExistingLogin(t *testing.T) {
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

func TestNLPCapabilityDiscoveryAndDegradedBehavior(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "alice", "alice-password", false)
	if _, err = store.PutSupportedLanguage(ctx, "de", "German"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutLanguageProfile(ctx, alice.ID, "de", "German"); err != nil {
		t.Fatal(err)
	}
	webAuth := webauth.New(authService, false, time.Hour)
	h := New(Services{
		Auth: authService, WebAuth: webAuth, Store: store,
		Capabilities: staticCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
			{Language: "fr", DisplayName: "French", Ready: true},
			{Language: "it", DisplayName: "Italian", Ready: false},
		}}},
		SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, "alice", "alice-password")

	settings := perform(t, h, "GET", "/settings", nil, cookies)
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), "French (fr)") || strings.Contains(settings.Body.String(), "Italian (it)") {
		t.Fatalf("capability settings=%d %s", settings.Code, settings.Body.String())
	}

	h = New(Services{
		Auth: authService, WebAuth: webAuth, Store: store,
		Capabilities:    staticCapabilities{err: errors.New("nlp unavailable")},
		SessionLifetime: time.Hour,
	})
	settings = perform(t, h, "GET", "/settings", nil, cookies)
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), "NLP language discovery is temporarily unavailable") || !strings.Contains(settings.Body.String(), "German") || strings.Contains(settings.Body.String(), `action="/settings/languages"`) {
		t.Fatalf("degraded settings=%d %s", settings.Code, settings.Body.String())
	}
	blocked := perform(t, h, "POST", "/settings/languages", url.Values{"csrf_token": {csrf}, "language": {"fr"}}, cookies)
	if blocked.Code != http.StatusServiceUnavailable {
		t.Fatalf("degraded language add=%d %s", blocked.Code, blocked.Body.String())
	}
	profiles, err := store.ListLanguageProfiles(ctx, alice.ID)
	if err != nil || len(profiles) != 1 || profiles[0].Language != "de" {
		t.Fatalf("saved profiles changed during degradation: %+v err=%v", profiles, err)
	}
}

func TestAddStudyLanguageSyncsFreshCapabilityReference(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "alice", "alice-password", false)
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: store,
		Capabilities: readyGerman(), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, "alice", "alice-password")

	var references int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM supported_languages`).Scan(&references); err != nil || references != 0 {
		t.Fatalf("fresh supported languages=%d err=%v", references, err)
	}
	unsupported := perform(t, h, "POST", "/settings/languages", url.Values{"csrf_token": {csrf}, "language": {"zz"}}, cookies)
	if unsupported.Code != http.StatusBadRequest || !strings.Contains(unsupported.Body.String(), "unsupported study language") {
		t.Fatalf("unsupported language=%d %s", unsupported.Code, unsupported.Body.String())
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM supported_languages`).Scan(&references); err != nil || references != 0 {
		t.Fatalf("unsupported language changed references=%d err=%v", references, err)
	}

	added := perform(t, h, "POST", "/settings/languages", url.Values{"csrf_token": {csrf}, "language": {"de"}}, cookies)
	if added.Code != http.StatusSeeOther {
		t.Fatalf("add study language=%d %s", added.Code, added.Body.String())
	}
	profiles, err := store.ListLanguageProfiles(ctx, alice.ID)
	if err != nil || len(profiles) != 1 || profiles[0].OwnerID != alice.ID || profiles[0].Language != "de" || profiles[0].DisplayName != "German" {
		t.Fatalf("profiles=%+v err=%v", profiles, err)
	}
}

func TestLoginBrowseAcquireAndImportedBookOwnerScoping(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "webapp-integration-secret")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	authService := auth.New(store, time.Hour)
	createAccount(t, ctx, store, "admin", "admin-password", true)
	alice := createAccount(t, ctx, store, "alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "bob", "bob-password", false)
	if _, err = store.PutSupportedLanguage(ctx, "de", "German"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutLanguageProfile(ctx, alice.ID, "de", "German"); err != nil {
		t.Fatal(err)
	}
	epubBytes := testEPUB(t)
	catalogRequests := 0
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		catalogRequests++
		switch r.URL.Path {
		case "/opds":
			w.Header().Set("Content-Type", "application/atom+xml")
			fmt.Fprintf(w, `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Library</title><entry><id>book-1</id><title>Test Book</title><link rel="%s" type="%s" href="/book.epub"/></entry></feed>`, opds.AcquisitionRel, opds.EPUBMediaType)
		case "/opds/language":
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>Languages</title><entry><id>/opds/language/1</id><title>German</title><link rel="subsection" type="application/atom+xml" href="/opds/language/1"/></entry></feed>`))
		case "/opds/language/1":
			w.Header().Set("Content-Type", "application/atom+xml")
			fmt.Fprintf(w, `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>German</title><entry><id>book-pdf</id><title>PDF Book</title><link rel="%s" type="application/pdf" href="/book.pdf"/></entry><entry><id>book-1</id><title>Test Book</title><link rel="%s" type="%s" href="/book.epub"/></entry></feed>`, opds.AcquisitionRel, opds.AcquisitionRel, opds.EPUBMediaType)
		case "/book.epub":
			w.Header().Set("Content-Type", opds.EPUBMediaType)
			_, _ = w.Write(epubBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer catalog.Close()
	connection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Library", URL: catalog.URL + "/opds"})
	if err != nil {
		t.Fatal(err)
	}
	bobConnection, err := store.CreateOpdsConnection(ctx, bob.ID, domain.OpdsConnection{Name: "Private", URL: catalog.URL + "/opds"})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &recordingAnalysis{}
	opdsService := opds.NewService(store, epub.NewService(store), catalog.Client())
	webAuth := webauth.New(authService, false, time.Hour)
	knownJobs := &recordingKnownVocab{service: knownvocab.NewService(store)}
	h := New(Services{Auth: authService, WebAuth: webAuth, Store: store, OPDS: opdsService, Analysis: recorder, KnownVocab: knownJobs, Capabilities: readyGerman(), SessionLifetime: time.Hour})
	loginPage := perform(t, h, "GET", "/login", nil, nil)
	csrf := hiddenToken(t, loginPage.Body.String())
	csrfCookieValue := cookieNamed(t, loginPage.Result().Cookies(), csrfCookie)
	form := url.Values{"csrf_token": {csrf}, "username": {"alice"}, "password": {"alice-password"}}
	login := perform(t, h, "POST", "/login", form, []*http.Cookie{csrfCookieValue})
	if login.Code != http.StatusSeeOther {
		t.Fatalf("login=%d %s", login.Code, login.Body.String())
	}
	session := cookieNamed(t, login.Result().Cookies(), webauth.CookieName)
	csrfCookieValue = cookieNamed(t, login.Result().Cookies(), csrfCookie)
	csrf = csrfCookieValue.Value
	cookies := []*http.Cookie{csrfCookieValue, session}
	home := perform(t, h, "GET", "/", nil, cookies)
	if home.Code != http.StatusSeeOther || home.Header().Get("Location") != "/library" {
		t.Fatalf("home=%d location=%q", home.Code, home.Header().Get("Location"))
	}
	requestsBeforeUnsupported := catalogRequests
	unsupportedBrowse := perform(t, h, "GET", "/opds/language?connection="+connection.ID+"&language=xx", nil, cookies)
	if unsupportedBrowse.Code != http.StatusBadRequest || catalogRequests != requestsBeforeUnsupported {
		t.Fatalf("unsupported browse=%d requests=%d want %d", unsupportedBrowse.Code, catalogRequests, requestsBeforeUnsupported)
	}
	unsupportedRootBrowse := perform(t, h, "GET", "/opds/browse?connection="+connection.ID+"&language=xx", nil, cookies)
	if unsupportedRootBrowse.Code != http.StatusBadRequest || catalogRequests != requestsBeforeUnsupported {
		t.Fatalf("unsupported root browse=%d requests=%d want %d", unsupportedRootBrowse.Code, catalogRequests, requestsBeforeUnsupported)
	}
	unsupportedAcquire := perform(t, h, "POST", "/opds/acquire", url.Values{"csrf_token": {csrf}, "connection": {connection.ID}, "language": {"xx"}, "entry_id": {"book-1"}, "title": {"Test Book"}, "href": {catalog.URL + "/book.epub"}}, cookies)
	if unsupportedAcquire.Code != http.StatusBadRequest || catalogRequests != requestsBeforeUnsupported {
		t.Fatalf("unsupported acquire=%d requests=%d want %d", unsupportedAcquire.Code, catalogRequests, requestsBeforeUnsupported)
	}
	browse := perform(t, h, "GET", "/opds/browse?connection="+connection.ID+"&language=de", nil, cookies)
	if browse.Code != 200 || strings.Contains(browse.Body.String(), "Test Book") || !strings.Contains(browse.Body.String(), "Search is not available") {
		t.Fatalf("browse=%d %s", browse.Code, browse.Body.String())
	}
	crossOwner := perform(t, h, "GET", "/opds/browse?connection="+bobConnection.ID+"&language=de", nil, cookies)
	if crossOwner.Code != http.StatusNotFound {
		t.Fatalf("cross-owner browse=%d %s", crossOwner.Code, crossOwner.Body.String())
	}
	if got := perform(t, h, "GET", "/catalog?connection="+bobConnection.ID, nil, cookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner catalog=%d %s", got.Code, got.Body.String())
	}
	if got := perform(t, h, "POST", "/connections/"+bobConnection.ID, url.Values{"csrf_token": {csrf}, "name": {"Stolen"}, "url": {catalog.URL}, "language": {"de"}}, cookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner update=%d %s", got.Code, got.Body.String())
	}
	if got := perform(t, h, "POST", "/connections/"+bobConnection.ID+"/delete", url.Values{"csrf_token": {csrf}}, cookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner delete=%d %s", got.Code, got.Body.String())
	}
	if got := perform(t, h, "POST", "/opds/acquire", url.Values{"csrf_token": {csrf}, "connection": {bobConnection.ID}, "language": {"de"}, "entry_id": {"book-1"}, "title": {"Test Book"}, "href": {catalog.URL + "/book.epub"}}, cookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner acquire=%d %s", got.Code, got.Body.String())
	}
	catalogPage := perform(t, h, "GET", "/catalog?connection="+connection.ID, nil, cookies)
	if catalogPage.Code != 200 || !strings.Contains(catalogPage.Body.String(), `value="de"`) {
		t.Fatalf("catalog=%d %s", catalogPage.Code, catalogPage.Body.String())
	}
	languageBooks := perform(t, h, "GET", "/opds/language?connection="+connection.ID+"&language=de", nil, cookies)
	if languageBooks.Code != 200 || !strings.Contains(languageBooks.Body.String(), "Test Book") || strings.Contains(languageBooks.Body.String(), "PDF Book") {
		t.Fatalf("language browse=%d %s", languageBooks.Code, languageBooks.Body.String())
	}
	acquireForm := url.Values{"csrf_token": {csrf}, "connection": {connection.ID}, "language": {"de"}, "entry_id": {"book-1"}, "title": {"Test Book"}, "href": {catalog.URL + "/book.epub"}}
	acquired := perform(t, h, "POST", "/opds/acquire", acquireForm, cookies)
	if acquired.Code != http.StatusSeeOther {
		t.Fatalf("acquire=%d %s", acquired.Code, acquired.Body.String())
	}
	if recorder.owner != alice.ID || recorder.source == "" {
		t.Fatalf("analysis wiring owner=%q source=%q", recorder.owner, recorder.source)
	}
	library := perform(t, h, "GET", "/library", nil, cookies)
	if library.Code != 200 || !strings.Contains(library.Body.String(), "Test Book") || !strings.Contains(library.Body.String(), "not analyzed") {
		t.Fatalf("library=%d %s", library.Code, library.Body.String())
	}
	bookPage := perform(t, h, "GET", "/books/"+recorder.source, nil, cookies)
	if bookPage.Code != 200 || !strings.Contains(bookPage.Body.String(), "Submit to analysis") {
		t.Fatalf("book=%d %s", bookPage.Code, bookPage.Body.String())
	}
	if got := perform(t, h, "POST", "/books/"+recorder.source+"/analyze", nil, cookies); got.Code != http.StatusForbidden {
		t.Fatalf("analysis without csrf=%d", got.Code)
	}
	resubmitted := perform(t, h, "POST", "/books/"+recorder.source+"/analyze", url.Values{"csrf_token": {csrf}}, cookies)
	if resubmitted.Code != http.StatusSeeOther || recorder.owner != alice.ID || recorder.source == "" {
		t.Fatalf("resubmit=%d owner=%q source=%q", resubmitted.Code, recorder.owner, recorder.source)
	}
	jobPage := perform(t, h, "GET", "/jobs/42", nil, cookies)
	if jobPage.Code != 200 || !strings.Contains(jobPage.Body.String(), "Succeeded") || !strings.Contains(jobPage.Body.String(), "Analysis job #1") || strings.Contains(jobPage.Body.String(), "Analysis job #42") {
		t.Fatalf("job detail=%d %s", jobPage.Code, jobPage.Body.String())
	}

	// Seed the completed pipeline boundary and exercise the authenticated book
	// workflow after analysis.
	artifactHash := "web-workflow-artifact"
	if _, err = store.Pool().Exec(ctx, `INSERT INTO normalized_corpus_artifacts(content_hash,language,schema_version,normalization_profile,normalization_version,analyzer_name,analyzer_version) VALUES($1,'de','1','test','1','test','1')`, artifactHash); err != nil {
		t.Fatal(err)
	}
	_, err = store.PutCorpus(ctx, alice.ID, recorder.source, artifactHash)
	if err != nil {
		t.Fatal(err)
	}
	library = perform(t, h, "GET", "/library", nil, cookies)
	if library.Code != 200 || !strings.Contains(library.Body.String(), "analyzed") {
		t.Fatalf("analyzed library=%d %s", library.Code, library.Body.String())
	}
	externalJobs := &recordingEnrichment{}
	insights := &recordingAnalysisInsights{coverage: domain.AnalysisCoverage{
		AnalyzableTokenCount: 200, DistinctLemmaCount: 8, KnownTokenCount: 110, KnownLemmaCount: 3, UnknownTokenCount: 90, UnknownLemmaCount: 5,
		TextProfile:          &domain.TextProfile{SentenceCount: 20, NormalizedTokenCount: 240, MedianSentenceTokenCount: 12, P90SentenceTokenCount: 38, LongSentenceCount: 2},
		TopUnknownLemmas:     []domain.LemmaOccurrence{{CanonicalLemma: "wichtig", UPOS: "ADJ", OccurrenceCount: 30}},
		UnknownConcentration: domain.CoverageProjection{TopLemmaCount: 10, SelectedLemmaCount: 5, OccurrenceCount: 60, EligibleTokenCount: 75, ProjectedTokenCount: 170},
		Projections:          []domain.CoverageProjection{{TopLemmaCount: 10, SelectedLemmaCount: 5, OccurrenceCount: 60, EligibleTokenCount: 75, ProjectedTokenCount: 170}, {TopLemmaCount: 25, SelectedLemmaCount: 5, OccurrenceCount: 60, EligibleTokenCount: 75, ProjectedTokenCount: 170}, {TopLemmaCount: 50, SelectedLemmaCount: 5, OccurrenceCount: 60, EligibleTokenCount: 75, ProjectedTokenCount: 170}},
		Thresholds:           []domain.CoverageThreshold{{TargetPercent: 95, LemmaCount: 3, Reachable: true}, {TargetPercent: 97, LemmaCount: 4, Reachable: true}, {TargetPercent: 99, LemmaCount: 5, Reachable: true}},
	}}
	h = New(Services{Auth: authService, WebAuth: webAuth, Store: store, OPDS: opdsService, Analysis: recorder, AnalysisInsights: insights, KnownVocab: knownJobs, Enrichment: externalJobs, Capabilities: readyGerman(), SessionLifetime: time.Hour})
	bookPage = perform(t, h, "GET", "/books/"+recorder.source, nil, cookies)
	if bookPage.Code != http.StatusOK || insights.owner != alice.ID || insights.corpus == "" {
		t.Fatalf("coverage request=%d owner=%q corpus=%q body=%s", bookPage.Code, insights.owner, insights.corpus, bookPage.Body.String())
	}
	for _, want := range []string{"Text profile", "20", "12.0", "38", "10.0%", "200 of 240", "55.0%", "current-known coverage", "active-campaign projected coverage", "200", "8", "110", "90", "lemmas for 95%", "lemmas for 97%", "lemmas for 99%", "graduated by completed campaigns", "legacy generated history", "deck-eligible vocabulary", "Highest-impact unknown vocabulary", "wichtig", "30 occurrences", "80.0%", "Projected token coverage", "85.0%", "after top 10 lemmas", "after top 25 lemmas", "after top 50 lemmas", "Prepare deck"} {
		if !strings.Contains(bookPage.Body.String(), want) {
			t.Errorf("coverage page missing %q", want)
		}
	}
	settingsPage := perform(t, h, "GET", "/settings?language=de", nil, cookies)
	if settingsPage.Code != 200 || !strings.Contains(settingsPage.Body.String(), "Account settings") || !strings.Contains(settingsPage.Body.String(), "German") || !strings.Contains(settingsPage.Body.String(), "Import known words") {
		t.Fatalf("settings page=%d %s", settingsPage.Code, settingsPage.Body.String())
	}
	knownPage := perform(t, h, "GET", "/known-vocab?language=de", nil, cookies)
	if knownPage.Code != 200 || !strings.Contains(knownPage.Body.String(), "Known vocabulary") || strings.Contains(knownPage.Body.String(), "textarea") || strings.Contains(knownPage.Body.String(), "Universal POS") || !strings.Contains(knownPage.Body.String(), `accept="text/plain,.txt"`) {
		t.Fatalf("known vocab page=%d %s", knownPage.Code, knownPage.Body.String())
	}
	if got := multipartUpload(t, h, "/known-vocab/import", cookies, map[string]string{"language": "de"}, "Daß\nbad\tNOPE\n"); got.Code != http.StatusForbidden {
		t.Fatalf("known vocab without csrf=%d", got.Code)
	}
	importedKnown := multipartUpload(t, h, "/known-vocab/import", cookies, map[string]string{"csrf_token": csrf, "language": "de"}, "Daß\nbad\tNOPE\n")
	if importedKnown.Code != 200 || !strings.Contains(importedKnown.Body.String(), "Queued") || !strings.Contains(importedKnown.Body.String(), "/known-vocab/imports/77/status") {
		t.Fatalf("known vocab import=%d %s", importedKnown.Code, importedKnown.Body.String())
	}
	importStatus := perform(t, h, "GET", "/known-vocab/imports/77/status", nil, cookies)
	if importStatus.Code != 200 || !strings.Contains(importStatus.Body.String(), "1 imported") || !strings.Contains(importStatus.Body.String(), "no tab-separated columns") || !strings.Contains(importStatus.Body.String(), "<td>2</td>") {
		t.Fatalf("known vocab status=%d %s", importStatus.Code, importStatus.Body.String())
	}
	bobLogin := perform(t, h, "POST", "/login", url.Values{"csrf_token": {csrf}, "username": {bob.Username}, "password": {"bob-password"}}, []*http.Cookie{csrfCookieValue})
	bobSession := cookieNamed(t, bobLogin.Result().Cookies(), webauth.CookieName)
	if got := perform(t, h, "GET", "/known-vocab/imports/77/status", nil, []*http.Cookie{csrfCookieValue, bobSession}); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner known vocab status=%d %s", got.Code, got.Body.String())
	}
	for _, route := range []struct{ method, path string }{{"GET", "/review?book=" + recorder.source}, {"POST", "/review/accept"}, {"GET", "/deck?book=" + recorder.source}, {"GET", "/deck/download"}, {"POST", "/books/" + recorder.source + "/deck"}, {"GET", "/admin/frequency"}} {
		if got := perform(t, h, route.method, route.path, nil, cookies); got.Code != http.StatusNotFound {
			t.Fatalf("removed route %s %s=%d", route.method, route.path, got.Code)
		}
	}
	bookPage = perform(t, h, "GET", "/books/"+recorder.source, nil, cookies)
	if !strings.Contains(bookPage.Body.String(), "Prepare deck") || !strings.Contains(bookPage.Body.String(), "/books/"+recorder.source+"/deck/preparations") || strings.Contains(bookPage.Body.String(), `action="/books/`+recorder.source+`/deck"`) || strings.Contains(bookPage.Body.String(), "/review?") || strings.Contains(bookPage.Body.String(), "filter_known") || strings.Contains(bookPage.Body.String(), "ranking") {
		t.Fatalf("book deck flow not unified: %s", bookPage.Body.String())
	}
	statusPage := perform(t, h, "GET", "/enrichment-jobs/88/status", nil, cookies)
	if statusPage.Code != http.StatusOK || !strings.Contains(statusPage.Body.String(), "1 of 2 completed") || !strings.Contains(statusPage.Body.String(), "attempt 2") || !strings.Contains(statusPage.Body.String(), "temporary provider failure") {
		t.Fatalf("enrichment status=%d %s", statusPage.Code, statusPage.Body.String())
	}
	if got := perform(t, h, "POST", "/enrichment-jobs/88/cancel", nil, cookies); got.Code != http.StatusForbidden {
		t.Fatalf("cancel without csrf=%d", got.Code)
	}
	cancelled := perform(t, h, "POST", "/enrichment-jobs/88/cancel", url.Values{"csrf_token": {csrf}}, cookies)
	if cancelled.Code != http.StatusOK || !strings.Contains(cancelled.Body.String(), "Cancelled") {
		t.Fatalf("cancelled=%d %s", cancelled.Code, cancelled.Body.String())
	}
	bobCookies, bobCSRF := loginCookies(t, h, "bob", "bob-password")
	if got := perform(t, h, "GET", "/enrichment-jobs/88/status", nil, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner enrichment status=%d %s", got.Code, got.Body.String())
	}
	if got := perform(t, h, "POST", "/settings/languages", url.Values{"language": {"de"}}, bobCookies); got.Code != http.StatusForbidden {
		t.Fatalf("study language without csrf=%d", got.Code)
	}
	if got := perform(t, h, "POST", "/settings/languages", url.Values{"csrf_token": {bobCSRF}, "language": {"zz"}}, bobCookies); got.Code != http.StatusBadRequest {
		t.Fatalf("unsupported study language=%d", got.Code)
	}
	if got := perform(t, h, "POST", "/settings/languages", url.Values{"csrf_token": {bobCSRF}, "language": {"de"}}, bobCookies); got.Code != http.StatusSeeOther {
		t.Fatalf("add study language=%d", got.Code)
	}
	if got := perform(t, h, "POST", "/settings/languages/remove", url.Values{"csrf_token": {bobCSRF}, "language": {"de"}}, bobCookies); got.Code != http.StatusSeeOther {
		t.Fatalf("remove study language=%d", got.Code)
	}
	if got := perform(t, h, "GET", "/known-vocab?language=de", nil, bobCookies); got.Code != 200 || strings.Contains(got.Body.String(), "dass") {
		t.Fatalf("bob known vocabulary leaked: %d %s", got.Code, got.Body.String())
	}
	if got := perform(t, h, "GET", "/books/"+recorder.source, nil, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("bob read alice book: %d", got.Code)
	}
	if got := perform(t, h, "GET", "/jobs/42", nil, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("bob read alice job: %d", got.Code)
	}
	for _, path := range []string{"/admin", "/admin/users", "/languages", "/register"} {
		if got := perform(t, h, "GET", path, nil, cookies); got.Code != http.StatusNotFound {
			t.Fatalf("removed route %s=%d", path, got.Code)
		}
	}
	if got := perform(t, h, "POST", "/languages", url.Values{"csrf_token": {csrf}, "language": {"fr"}, "display_name": {"French"}}, cookies); got.Code != http.StatusNotFound {
		t.Fatalf("removed language update=%d", got.Code)
	}
	if got := perform(t, h, "GET", "/admin/connections", nil, cookies); got.Code != http.StatusNotFound {
		t.Fatalf("removed admin connection management=%d", got.Code)
	}
	if got := perform(t, h, "POST", "/connections", url.Values{"csrf_token": {csrf}, "name": {"Personal"}, "url": {catalog.URL}, "language": {"de"}}, cookies); got.Code != http.StatusSeeOther {
		t.Fatalf("learner connection create=%d", got.Code)
	}
	aliceConnections, err := store.ListOpdsConnections(ctx, alice.ID)
	if err != nil || len(aliceConnections) != 2 {
		t.Fatalf("learner connection list=%+v err=%v", aliceConnections, err)
	}
	personal := aliceConnections[0]
	if personal.ID == connection.ID {
		personal = aliceConnections[1]
	}
	if got := perform(t, h, "POST", "/connections/"+personal.ID, url.Values{"csrf_token": {csrf}, "name": {"Renamed personal"}, "url": {catalog.URL + "/opds"}, "language": {"de"}}, cookies); got.Code != http.StatusSeeOther {
		t.Fatalf("learner connection update=%d %s", got.Code, got.Body.String())
	}
	connectionsPage := perform(t, h, "GET", "/connections", nil, cookies)
	if connectionsPage.Code != http.StatusOK || !strings.Contains(connectionsPage.Body.String(), "Renamed personal") || strings.Contains(connectionsPage.Body.String(), "Private") {
		t.Fatalf("learner connection page=%d %s", connectionsPage.Code, connectionsPage.Body.String())
	}
	if got := perform(t, h, "POST", "/connections/"+personal.ID+"/delete", url.Values{"csrf_token": {csrf}}, cookies); got.Code != http.StatusSeeOther {
		t.Fatalf("learner connection delete=%d %s", got.Code, got.Body.String())
	}
	if _, err = store.GetOpdsConnection(ctx, alice.ID, personal.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("deleted learner connection read: %v", err)
	}
	adminCookies, _ := loginCookies(t, h, "admin", "admin-password")
	adminHub := perform(t, h, "GET", "/admin", nil, adminCookies)
	if adminHub.Code != http.StatusNotFound {
		t.Fatalf("removed admin hub=%d %s", adminHub.Code, adminHub.Body.String())
	}
	adminHome := perform(t, h, "GET", "/", nil, adminCookies)
	if adminHome.Code != http.StatusSeeOther || adminHome.Header().Get("Location") != "/library" {
		t.Fatalf("admin home=%d location=%q", adminHome.Code, adminHome.Header().Get("Location"))
	}
	for _, path := range []string{"/library", "/connections", "/catalog", "/known-vocab", "/settings"} {
		if got := perform(t, h, "GET", path, nil, adminCookies); got.Code == http.StatusForbidden {
			t.Fatalf("legacy admin denied learner route %s", path)
		}
	}
	adminConnections := perform(t, h, "GET", "/admin/connections", nil, adminCookies)
	if adminConnections.Code != http.StatusNotFound {
		t.Fatalf("removed admin connections=%d %s", adminConnections.Code, adminConnections.Body.String())
	}
	if got := perform(t, h, "GET", "/admin/frequency", nil, adminCookies); got.Code != http.StatusNotFound {
		t.Fatalf("removed admin frequency route=%d", got.Code)
	}
}

func TestPreparedDeckWebLifecycleOwnershipAndPureDownload(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "prepared-deck-web-secret")
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

	if got := perform(t, h, "POST", "/books/book-1/deck/preparations", nil, aliceCookies); got.Code != http.StatusForbidden {
		t.Fatalf("create without csrf=%d", got.Code)
	}
	created := perform(t, h, "POST", "/books/book-1/deck/preparations", url.Values{"csrf_token": {aliceCSRF}, "external_translation_consent": {"on"}}, aliceCookies)
	if created.Code != http.StatusSeeOther || created.Header().Get("Location") != "/deck-preparations/prep-1/status" || !decks.consent {
		t.Fatalf("create=%d location=%q consent=%v", created.Code, created.Header().Get("Location"), decks.consent)
	}
	status := perform(t, h, "GET", created.Header().Get("Location"), nil, aliceCookies)
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
	cancelled := perform(t, h, "POST", "/deck-preparations/prep-1/cancel", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
	if cancelled.Code != http.StatusOK || !strings.Contains(cancelled.Body.String(), `"state":"cancelled"`) {
		t.Fatalf("cancel=%d %s", cancelled.Code, cancelled.Body.String())
	}
	if got := perform(t, h, "POST", "/deck-preparations/prep-1/retry", nil, aliceCookies); got.Code != http.StatusForbidden {
		t.Fatalf("retry without csrf=%d", got.Code)
	}
	retried := perform(t, h, "POST", "/deck-preparations/prep-1/retry", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
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
	t.Setenv("MOUSEION_SECRET", "campaign-web-secret")
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

	page := perform(t, h, "GET", "/campaigns", nil, aliceCookies)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Prepared books") || !strings.Contains(page.Body.String(), "Completed Book") || !strings.Contains(page.Body.String(), "Deck ready") {
		t.Fatalf("prepared campaign page=%d %s", page.Code, page.Body.String())
	}
	ready, err := store.ListUnassignedReadyDeckPreparations(ctx, alice.ID)
	if err != nil || len(ready) != 4 {
		t.Fatalf("ready preparations=%+v, %v", ready, err)
	}
	if got := perform(t, h, "POST", "/campaigns", url.Values{"deck_preparation_id": {ready[0].ID}}, aliceCookies); got.Code != http.StatusForbidden {
		t.Fatalf("queue without csrf=%d", got.Code)
	}
	for _, preparation := range ready {
		queued := perform(t, h, "POST", "/campaigns", url.Values{"csrf_token": {aliceCSRF}, "deck_preparation_id": {preparation.ID}}, aliceCookies)
		if queued.Code != http.StatusSeeOther || !strings.HasPrefix(queued.Header().Get("Location"), "/campaigns?message=") {
			t.Fatalf("queue=%d location=%q body=%s", queued.Code, queued.Header().Get("Location"), queued.Body.String())
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
		response := perform(t, h, "POST", "/campaigns/"+campaign.ID+"/activate", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("activate %s=%d %s", campaign.ID, response.Code, response.Body.String())
		}
	}
	activate(completedCampaign)
	conflict := perform(t, h, "POST", "/campaigns/"+abandonedCampaign.ID+"/activate", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
	if conflict.Code != http.StatusSeeOther || !strings.Contains(conflict.Header().Get("Location"), "Finish+or+abandon") {
		t.Fatalf("second active=%d location=%q", conflict.Code, conflict.Header().Get("Location"))
	}
	if got := perform(t, h, "POST", "/campaigns/"+completedCampaign.ID+"/book-finished", nil, aliceCookies); got.Code != http.StatusForbidden {
		t.Fatalf("finish book without csrf=%d", got.Code)
	}
	finished := perform(t, h, "POST", "/campaigns/"+completedCampaign.ID+"/book-finished", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
	if finished.Code != http.StatusSeeOther || !strings.Contains(finished.Header().Get("Location"), "Book+marked+finished") {
		t.Fatalf("finish book=%d location=%q", finished.Code, finished.Header().Get("Location"))
	}
	progressPage := perform(t, h, "GET", "/campaigns", nil, aliceCookies)
	if body := progressPage.Body.String(); !strings.Contains(body, "Book</dt><dd>Finished · ") || strings.Contains(body, "Mark book finished") || !strings.Contains(body, "Mark deck reviewed") {
		t.Fatalf("book progress page=%s", body)
	}
	if got := perform(t, h, "POST", "/campaigns/"+completedCampaign.ID+"/deck-reviewed", url.Values{"csrf_token": {bobCSRF}}, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner review=%d", got.Code)
	}
	reviewed := perform(t, h, "POST", "/campaigns/"+completedCampaign.ID+"/deck-reviewed", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
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
	repeated := perform(t, h, "POST", "/campaigns/"+completedCampaign.ID+"/deck-reviewed", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
	if repeated.Code != http.StatusSeeOther || !strings.Contains(repeated.Header().Get("Location"), "Only+the+active") {
		t.Fatalf("repeat review=%d location=%q", repeated.Code, repeated.Header().Get("Location"))
	}
	activate(abandonedCampaign)
	if got := perform(t, h, "POST", "/campaigns/"+abandonedCampaign.ID+"/abandon", nil, aliceCookies); got.Code != http.StatusForbidden {
		t.Fatalf("abandon without csrf=%d", got.Code)
	}
	if got := perform(t, h, "POST", "/campaigns/"+abandonedCampaign.ID+"/abandon", url.Values{"csrf_token": {bobCSRF}}, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner abandon=%d", got.Code)
	}
	abandoned := perform(t, h, "POST", "/campaigns/"+abandonedCampaign.ID+"/abandon", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
	if abandoned.Code != http.StatusSeeOther || !strings.Contains(abandoned.Header().Get("Location"), "Campaign+abandoned") {
		t.Fatalf("abandon=%d location=%q", abandoned.Code, abandoned.Header().Get("Location"))
	}
	repeatedAbandon := perform(t, h, "POST", "/campaigns/"+abandonedCampaign.ID+"/abandon", url.Values{"csrf_token": {aliceCSRF}}, aliceCookies)
	if repeatedAbandon.Code != http.StatusSeeOther || !strings.Contains(repeatedAbandon.Header().Get("Location"), "Campaign+abandoned") {
		t.Fatalf("repeat abandon=%d location=%q", repeatedAbandon.Code, repeatedAbandon.Header().Get("Location"))
	}
	activate(activeCampaign)

	page = perform(t, h, "GET", "/campaigns", nil, aliceCookies)
	body := page.Body.String()
	for _, expected := range []string{"Active campaign", "Queue", "History", "Completed Book", "Abandoned Book", "Active Book", "Queued Book", ">Complete<", ">Abandoned<", ">Active<", ">Queued<", "Book</dt><dd>Reading", "Deck</dt><dd>Studying", "Completed 20", "Mark book finished", "Mark deck reviewed", "Abandon campaign"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("campaign page missing %q: %s", expected, body)
		}
	}
	if got := perform(t, h, "POST", "/campaigns/"+queuedCampaign.ID+"/activate", url.Values{"csrf_token": {bobCSRF}}, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner activation=%d", got.Code)
	}
	bobPage := perform(t, h, "GET", "/campaigns", nil, bobCookies)
	if bobPage.Code != http.StatusOK || strings.Contains(bobPage.Body.String(), "Active Book") {
		t.Fatalf("bob campaign page=%d %s", bobPage.Code, bobPage.Body.String())
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
	if path == "/known-vocab/import" {
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
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	files := map[string]string{"mimetype": "application/epub+zip", "META-INF/container.xml": `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`, `OEBPS/content.opf`: `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">book-1</dc:identifier><dc:title>Test Book</dc:title></metadata><manifest><item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="chapter"/></spine></package>`, `OEBPS/chapter.xhtml`: `<html xmlns="http://www.w3.org/1999/xhtml"><body><p>Hallo Welt.</p></body></html>`}
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
