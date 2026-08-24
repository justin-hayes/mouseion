//go:build integration

package webapp

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/enrichmentjob"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river/rivertype"
)

type recordingAnalysis struct{ owner, source string }
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
	return analysis.Handle{ID: 42}, nil
}
func (r *recordingAnalysis) Get(_ context.Context, owner string, id int64) (analysis.Status, error) {
	if owner != r.owner || id != 42 {
		return analysis.Status{}, analysis.ErrNotFound
	}
	return analysis.Status{ID: 42, State: rivertype.JobStateCompleted, Progress: 100, CorpusID: "corpus-result", Attempt: 1}, nil
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
	admin, err := authService.BootstrapAdmin(ctx, "admin", "admin-password")
	if err != nil {
		t.Fatal(err)
	}
	alice, err := authService.CreateUser(ctx, admin.ID, "alice", "alice-password", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := authService.CreateUser(ctx, admin.ID, "bob", "bob-password", auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutSupportedLanguage(ctx, "de", "German"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutLanguageProfile(ctx, alice.ID, "de", "German"); err != nil {
		t.Fatal(err)
	}
	epubBytes := testEPUB(t)
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	connection, err := store.CreateOpdsConnection(ctx, domain.OpdsConnection{Name: "Library", URL: catalog.URL + "/opds", Language: "de"})
	if err != nil {
		t.Fatal(err)
	}
	bobConnection, err := store.CreateOpdsConnection(ctx, domain.OpdsConnection{Name: "Private", URL: catalog.URL + "/opds", Language: "de"})
	if err != nil {
		t.Fatal(err)
	}
	recorder := &recordingAnalysis{}
	opdsService := opds.NewService(store, epub.NewService(store), catalog.Client())
	webAuth := webauth.New(authService, false, time.Hour)
	knownJobs := &recordingKnownVocab{service: knownvocab.NewService(store)}
	h := New(Services{Auth: authService, WebAuth: webAuth, Store: store, OPDS: opdsService, Analysis: recorder, KnownVocab: knownJobs, SessionLifetime: time.Hour})
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
	browse := perform(t, h, "GET", "/opds/browse?connection="+connection.ID, nil, cookies)
	if browse.Code != 200 || strings.Contains(browse.Body.String(), "Test Book") || !strings.Contains(browse.Body.String(), "Search is not available") {
		t.Fatalf("browse=%d %s", browse.Code, browse.Body.String())
	}
	shared := perform(t, h, "GET", "/opds/browse?connection="+bobConnection.ID, nil, cookies)
	if shared.Code != 200 || strings.Contains(shared.Body.String(), "Test Book") || !strings.Contains(shared.Body.String(), "Search is not available") {
		t.Fatalf("shared browse=%d %s", shared.Code, shared.Body.String())
	}
	catalogPage := perform(t, h, "GET", "/catalog?connection="+connection.ID, nil, cookies)
	if catalogPage.Code != 200 || !strings.Contains(catalogPage.Body.String(), "German — study language") {
		t.Fatalf("catalog=%d %s", catalogPage.Code, catalogPage.Body.String())
	}
	languageBooks := perform(t, h, "GET", "/opds/language?connection="+connection.ID+"&language=1", nil, cookies)
	if languageBooks.Code != 200 || !strings.Contains(languageBooks.Body.String(), "Test Book") || strings.Contains(languageBooks.Body.String(), "PDF Book") {
		t.Fatalf("language browse=%d %s", languageBooks.Code, languageBooks.Body.String())
	}
	acquireForm := url.Values{"csrf_token": {csrf}, "connection": {connection.ID}, "entry_id": {"book-1"}, "title": {"Test Book"}, "href": {catalog.URL + "/book.epub"}}
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
	if jobPage.Code != 200 || !strings.Contains(jobPage.Body.String(), "Succeeded") {
		t.Fatalf("job detail=%d %s", jobPage.Code, jobPage.Body.String())
	}

	// Seed the completed pipeline boundary and exercise coverage export through
	// the authenticated one-button workflow.
	artifactHash := "web-workflow-artifact"
	if _, err = store.Pool().Exec(ctx, `INSERT INTO normalized_corpus_artifacts(content_hash,language,schema_version,normalization_profile,normalization_version,analyzer_name,analyzer_version) VALUES($1,'de','1','test','1','test','1')`, artifactHash); err != nil {
		t.Fatal(err)
	}
	corpus, err := store.PutCorpus(ctx, alice.ID, recorder.source, artifactHash)
	if err != nil {
		t.Fatal(err)
	}
	library = perform(t, h, "GET", "/library", nil, cookies)
	if library.Code != 200 || !strings.Contains(library.Body.String(), "analyzed") {
		t.Fatalf("analyzed library=%d %s", library.Code, library.Body.String())
	}
	if _, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES($1,'de','haus','NOUN','candidate')`, alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,'de','haus','NOUN',1,'["Haus"]','[{"Location":{"StartOffset":0},"Text":"Das Haus ist heute sehr ruhig."}]','{"min_occurrences":1,"occurrence_count":1}')`, alice.ID, corpus.ID); err != nil {
		t.Fatal(err)
	}
	examples := []domain.ExampleSentence{{SentenceKey: "haus:1", Text: "Das Haus ist heute sehr ruhig.", SourceLocation: []byte(`{"source_document_id":"book-1","start_offset":0,"end_offset":4}`), SelectionReasons: []byte(`["preferred length"]`), SelectionRank: 1, SelectionScore: 90, Chosen: true}}
	if err = store.ReplaceSelectedSentences(ctx, alice.ID, corpus.ID, "de", "haus", "NOUN", examples); err != nil {
		t.Fatal(err)
	}
	externalJobs := &recordingEnrichment{}
	h = New(Services{Auth: authService, WebAuth: webAuth, Store: store, OPDS: opdsService, Analysis: recorder, KnownVocab: knownJobs, CardExport: cardexport.NewService(store), Enrichment: externalJobs, SessionLifetime: time.Hour})
	settingsPage := perform(t, h, "GET", "/settings?language=de", nil, cookies)
	if settingsPage.Code != 200 || !strings.Contains(settingsPage.Body.String(), "Account settings") || !strings.Contains(settingsPage.Body.String(), "German") || !strings.Contains(settingsPage.Body.String(), "Import known words") {
		t.Fatalf("settings page=%d %s", settingsPage.Code, settingsPage.Body.String())
	}
	knownPage := perform(t, h, "GET", "/known-vocab?language=de", nil, cookies)
	if knownPage.Code != 200 || !strings.Contains(knownPage.Body.String(), "Known vocabulary") {
		t.Fatalf("known vocab page=%d %s", knownPage.Code, knownPage.Body.String())
	}
	if got := multipartUpload(t, h, "/known-vocab/import", cookies, map[string]string{"language": "de", "vocabulary": "Daß\tSCONJ\nbad\tNOPE"}, ""); got.Code != http.StatusForbidden {
		t.Fatalf("known vocab without csrf=%d", got.Code)
	}
	importedKnown := multipartUpload(t, h, "/known-vocab/import", cookies, map[string]string{"csrf_token": csrf, "language": "de", "vocabulary": "Daß\tSCONJ\nbad\tNOPE"}, "")
	if importedKnown.Code != 200 || !strings.Contains(importedKnown.Body.String(), "Queued") || !strings.Contains(importedKnown.Body.String(), "/known-vocab/imports/77/status") {
		t.Fatalf("known vocab import=%d %s", importedKnown.Code, importedKnown.Body.String())
	}
	importStatus := perform(t, h, "GET", "/known-vocab/imports/77/status", nil, cookies)
	if importStatus.Code != 200 || !strings.Contains(importStatus.Body.String(), "1 imported") || !strings.Contains(importStatus.Body.String(), "invalid UPOS") {
		t.Fatalf("known vocab status=%d %s", importStatus.Code, importStatus.Body.String())
	}
	bobLogin := perform(t, h, "POST", "/login", url.Values{"csrf_token": {csrf}, "username": {bob.Username}, "password": {"bob-password"}}, []*http.Cookie{csrfCookieValue})
	bobSession := cookieNamed(t, bobLogin.Result().Cookies(), webauth.CookieName)
	if got := perform(t, h, "GET", "/known-vocab/imports/77/status", nil, []*http.Cookie{csrfCookieValue, bobSession}); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner known vocab status=%d %s", got.Code, got.Body.String())
	}
	for _, route := range []struct{ method, path string }{{"GET", "/review?book=" + recorder.source}, {"POST", "/review/accept"}, {"GET", "/deck?book=" + recorder.source}, {"GET", "/deck/download"}, {"GET", "/admin/frequency"}} {
		if got := perform(t, h, route.method, route.path, nil, cookies); got.Code != http.StatusNotFound {
			t.Fatalf("removed route %s %s=%d", route.method, route.path, got.Code)
		}
	}
	bookPage = perform(t, h, "GET", "/books/"+recorder.source, nil, cookies)
	if !strings.Contains(bookPage.Body.String(), "Generate deck") || !strings.Contains(bookPage.Body.String(), "/books/"+recorder.source+"/deck") || strings.Contains(bookPage.Body.String(), "/review?") || strings.Contains(bookPage.Body.String(), "filter_known") || strings.Contains(bookPage.Body.String(), "ranking") {
		t.Fatalf("book deck flow not unified: %s", bookPage.Body.String())
	}
	generate := url.Values{}
	if got := perform(t, h, "POST", "/books/"+recorder.source+"/deck", generate, cookies); got.Code != http.StatusForbidden {
		t.Fatalf("deck export without csrf=%d", got.Code)
	}
	generate.Set("csrf_token", csrf)
	exported := perform(t, h, "POST", "/books/"+recorder.source+"/deck", generate, cookies)
	if exported.Code != 200 || exported.Header().Get("Content-Type") != "application/vnd.anki" || !strings.Contains(exported.Header().Get("Content-Disposition"), `filename="Test Book.apkg"`) {
		t.Fatalf("export=%d headers=%v", exported.Code, exported.Header())
	}
	if len(externalJobs.candidates) != 0 || exported.Header().Get("X-Mouseion-Enrichment-Job") != "" {
		t.Fatalf("translation queued without consent: %+v", externalJobs.candidates)
	}
	generate.Set("external_translation_consent", "on")
	withConsent := perform(t, h, "POST", "/books/"+recorder.source+"/deck", generate, cookies)
	if withConsent.Code != http.StatusOK || withConsent.Header().Get("X-Mouseion-Enrichment-Job") != "88" || len(externalJobs.candidates) != 1 || externalJobs.candidates[0].ExampleSentence != "Das Haus ist heute sehr ruhig." {
		t.Fatalf("consented translation queue: code=%d header=%q candidates=%+v", withConsent.Code, withConsent.Header().Get("X-Mouseion-Enrichment-Job"), externalJobs.candidates)
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
	zr, err := zip.NewReader(bytes.NewReader(exported.Body.Bytes()), int64(exported.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var collection *zip.File
	for _, member := range zr.File {
		if member.Name == "collection.anki2" {
			collection = member
		}
	}
	if collection == nil || len(zr.File) != 2 {
		t.Fatalf("package members=%v", zr.File)
	}
	r, err := collection.Open()
	if err != nil {
		t.Fatal(err)
	}
	dbBytes, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	dbPath := t.TempDir() + "/collection.anki2"
	if err = os.WriteFile(dbPath, dbBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	ankidb, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ankidb.Close()
	var models, decks, fields string
	var cards int
	if err = ankidb.QueryRow(`SELECT models,decks FROM col`).Scan(&models, &decks); err != nil {
		t.Fatal(err)
	}
	if err = ankidb.QueryRow(`SELECT flds FROM notes`).Scan(&fields); err != nil {
		t.Fatal(err)
	}
	if err = ankidb.QueryRow(`SELECT count(*) FROM cards`).Scan(&cards); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(models, `"name":"Mouseion Vocab Cloze"`) || !strings.Contains(decks, `Mouseion::de::Test Book`) || !strings.Contains(fields, `{{c1::Haus`) || len(strings.Split(fields, "\x1f")) != 8 || cards != 1 {
		t.Fatalf("models=%s decks=%s fields=%q cards=%d", models, decks, fields, cards)
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
	if got := perform(t, h, "POST", "/books/"+recorder.source+"/deck", url.Values{"csrf_token": {bobCSRF}}, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("bob generated alice deck: %d", got.Code)
	}
	if got := perform(t, h, "GET", "/jobs/42", nil, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("bob read alice job: %d", got.Code)
	}
	for _, path := range []string{"/admin", "/admin/users", "/languages"} {
		if got := perform(t, h, "GET", path, nil, cookies); got.Code != http.StatusForbidden {
			t.Fatalf("non-admin %s=%d", path, got.Code)
		}
	}
	if got := perform(t, h, "POST", "/languages", url.Values{"csrf_token": {csrf}, "language": {"fr"}, "display_name": {"French"}}, cookies); got.Code != http.StatusForbidden {
		t.Fatalf("non-admin language update=%d", got.Code)
	}
	if got := perform(t, h, "GET", "/admin/connections", nil, cookies); got.Code != http.StatusForbidden {
		t.Fatalf("non-admin connection management=%d", got.Code)
	}
	if got := perform(t, h, "POST", "/connections", url.Values{"csrf_token": {csrf}, "name": {"Denied"}, "url": {catalog.URL}, "language": {"de"}}, cookies); got.Code != http.StatusForbidden {
		t.Fatalf("non-admin connection create=%d", got.Code)
	}
	adminCookies, _ := loginCookies(t, h, "admin", "admin-password")
	adminHub := perform(t, h, "GET", "/admin", nil, adminCookies)
	if adminHub.Code != http.StatusOK || !strings.Contains(adminHub.Body.String(), "Configure languages") || !strings.Contains(adminHub.Body.String(), "Configure connections") || strings.Contains(adminHub.Body.String(), "My Library") {
		t.Fatalf("admin hub=%d %s", adminHub.Code, adminHub.Body.String())
	}
	adminHome := perform(t, h, "GET", "/", nil, adminCookies)
	if adminHome.Code != http.StatusSeeOther || adminHome.Header().Get("Location") != "/admin" {
		t.Fatalf("admin home=%d location=%q", adminHome.Code, adminHome.Header().Get("Location"))
	}
	for _, path := range []string{"/library", "/connections", "/catalog", "/known-vocab", "/settings"} {
		if got := perform(t, h, "GET", path, nil, adminCookies); got.Code != http.StatusForbidden {
			t.Fatalf("admin learner route %s=%d", path, got.Code)
		}
	}
	adminConnections := perform(t, h, "GET", "/admin/connections", nil, adminCookies)
	if adminConnections.Code != http.StatusOK || !strings.Contains(adminConnections.Body.String(), "Library") || !strings.Contains(adminConnections.Body.String(), "Add connection") {
		t.Fatalf("admin connections=%d %s", adminConnections.Code, adminConnections.Body.String())
	}
	if got := perform(t, h, "GET", "/admin/frequency", nil, adminCookies); got.Code != http.StatusNotFound {
		t.Fatalf("removed admin frequency route=%d", got.Code)
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
	part, err := writer.CreateFormFile("dataset", "frequency.csv")
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
