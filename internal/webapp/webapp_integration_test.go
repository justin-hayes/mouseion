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
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/frequency"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/review"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/vocabulary"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/justin-hayes/mouseion/internal/webworkflow"
	"github.com/riverqueue/river/rivertype"
)

type recordingAnalysis struct{ owner, source string }

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
	_, err = authService.CreateUser(ctx, admin.ID, "bob", "bob-password", auth.RoleUser)
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
	h := New(Services{Auth: authService, WebAuth: webAuth, Store: store, OPDS: opdsService, Analysis: recorder, KnownVocab: knownvocab.NewService(store), SessionLifetime: time.Hour})
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
	if browse.Code != 200 || !strings.Contains(browse.Body.String(), "Test Book") {
		t.Fatalf("browse=%d %s", browse.Code, browse.Body.String())
	}
	shared := perform(t, h, "GET", "/opds/browse?connection="+bobConnection.ID, nil, cookies)
	if shared.Code != 200 || !strings.Contains(shared.Body.String(), "Test Book") {
		t.Fatalf("shared browse=%d %s", shared.Code, shared.Body.String())
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

	// Seed the completed pipeline boundary and exercise the shared review/export
	// services through the authenticated web workflow.
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
	// Ranking is populated asynchronously after selection. Leave ranking columns
	// NULL to verify that review is available at this pipeline boundary.
	if _, err = store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,'de','haus','NOUN',2,'["Haus"]','[{"Location":{"StartOffset":0}}]','{"min_occurrences":2,"occurrence_count":2,"frequency_cutoff":0.05}')`, alice.ID, corpus.ID); err != nil {
		t.Fatal(err)
	}
	examples := []domain.ExampleSentence{{SentenceKey: "haus:1", Text: "Das Haus ist heute sehr ruhig.", SourceLocation: []byte(`{"source_document_id":"book-1","start_offset":0,"end_offset":4}`), SelectionReasons: []byte(`["preferred length"]`), SelectionRank: 1, SelectionScore: 90, Chosen: true}}
	if err = store.ReplaceSelectedSentences(ctx, alice.ID, corpus.ID, "de", "haus", "NOUN", examples); err != nil {
		t.Fatal(err)
	}
	workflow := webworkflow.NewReview(store.Pool(), review.NewService(vocabulary.NewLifecycle(store), store))
	h = New(Services{Auth: authService, WebAuth: webAuth, Store: store, OPDS: opdsService, Analysis: recorder, Review: workflow, Frequency: frequency.NewService(store), KnownVocab: knownvocab.NewService(store), CardExport: cardexport.NewService(store), SessionLifetime: time.Hour})
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
	if importedKnown.Code != 200 || !strings.Contains(importedKnown.Body.String(), "1 imported") || !strings.Contains(importedKnown.Body.String(), "invalid UPOS") || !strings.Contains(importedKnown.Body.String(), "dass") {
		t.Fatalf("known vocab import=%d %s", importedKnown.Code, importedKnown.Body.String())
	}
	reviewPage := perform(t, h, "GET", "/review", nil, cookies)
	if reviewPage.Code != 200 || !strings.Contains(reviewPage.Body.String(), "haus") {
		t.Fatalf("review=%d %s", reviewPage.Code, reviewPage.Body.String())
	}
	edit := perform(t, h, "POST", "/review/edit", url.Values{"csrf_token": {csrf}, "language": {"de"}, "lemma": {"haus"}, "upos": {"NOUN"}, "sentence": {"Das Haus ist heute ganz ruhig."}}, cookies)
	if edit.Code != http.StatusSeeOther {
		t.Fatalf("edit=%d %s", edit.Code, edit.Body.String())
	}
	accept := perform(t, h, "POST", "/review/accept", url.Values{"csrf_token": {csrf}, "language": {"de"}, "lemma": {"haus"}, "upos": {"NOUN"}}, cookies)
	if accept.Code != http.StatusSeeOther {
		t.Fatalf("accept=%d %s", accept.Code, accept.Body.String())
	}
	deckPage := perform(t, h, "GET", "/deck?book="+recorder.source, nil, cookies)
	if deckPage.Code != 200 || !strings.Contains(deckPage.Body.String(), "Filter known words") || !strings.Contains(deckPage.Body.String(), "First encounter") || !strings.Contains(deckPage.Body.String(), "Frequency in this book") {
		t.Fatalf("deck config=%d %s", deckPage.Code, deckPage.Body.String())
	}
	bookPage = perform(t, h, "GET", "/books/"+recorder.source, nil, cookies)
	if !strings.Contains(bookPage.Body.String(), "/settings?language=de") || strings.Contains(bookPage.Body.String(), "/known-vocab?language=de") {
		t.Fatalf("book known-vocabulary link not relocated: %s", bookPage.Body.String())
	}
	configured := url.Values{"book": {recorder.source}, "name": {"German"}, "filter_known": {"true"}, "ranking": {"encounter"}}
	if got := perform(t, h, "POST", "/deck/download", configured, cookies); got.Code != http.StatusForbidden {
		t.Fatalf("deck export without csrf=%d", got.Code)
	}
	configured.Set("csrf_token", csrf)
	exported := perform(t, h, "POST", "/deck/download", configured, cookies)
	if exported.Code != 200 || !strings.Contains(exported.Header().Get("Content-Disposition"), "attachment") || !strings.Contains(exported.Body.String(), "{{c1::Haus") {
		t.Fatalf("export=%d headers=%v body=%s", exported.Code, exported.Header(), exported.Body.String())
	}

	bobCookies, bobCSRF := loginCookies(t, h, "bob", "bob-password")
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
	if got := perform(t, h, "GET", "/deck?book="+recorder.source, nil, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("bob configured alice deck: %d", got.Code)
	}
	if got := perform(t, h, "GET", "/jobs/42", nil, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("bob read alice job: %d", got.Code)
	}
	if got := perform(t, h, "GET", "/review", nil, bobCookies); got.Code != 200 || strings.Contains(got.Body.String(), "haus") {
		t.Fatalf("bob review leaked: %d %s", got.Code, got.Body.String())
	}
	if got := perform(t, h, "POST", "/review/ignore", url.Values{"csrf_token": {bobCSRF}, "language": {"de"}, "lemma": {"haus"}, "upos": {"NOUN"}}, bobCookies); got.Code == 200 || got.Code == http.StatusSeeOther {
		t.Fatalf("bob acted on alice item: %d", got.Code)
	}
	if got := perform(t, h, "GET", "/admin/frequency?language=de", nil, cookies); got.Code != http.StatusForbidden {
		t.Fatalf("non-admin frequency=%d", got.Code)
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
	adminCookies, adminCSRF := loginCookies(t, h, "admin", "admin-password")
	adminHub := perform(t, h, "GET", "/admin", nil, adminCookies)
	if adminHub.Code != http.StatusOK || !strings.Contains(adminHub.Body.String(), "Configure languages") || !strings.Contains(adminHub.Body.String(), "Configure connections") || strings.Contains(adminHub.Body.String(), "My Library") {
		t.Fatalf("admin hub=%d %s", adminHub.Code, adminHub.Body.String())
	}
	adminHome := perform(t, h, "GET", "/", nil, adminCookies)
	if adminHome.Code != http.StatusSeeOther || adminHome.Header().Get("Location") != "/admin" {
		t.Fatalf("admin home=%d location=%q", adminHome.Code, adminHome.Header().Get("Location"))
	}
	for _, path := range []string{"/library", "/connections", "/catalog", "/known-vocab", "/settings", "/review", "/deck"} {
		if got := perform(t, h, "GET", path, nil, adminCookies); got.Code != http.StatusForbidden {
			t.Fatalf("admin learner route %s=%d", path, got.Code)
		}
	}
	adminConnections := perform(t, h, "GET", "/admin/connections", nil, adminCookies)
	if adminConnections.Code != http.StatusOK || !strings.Contains(adminConnections.Body.String(), "Library") || !strings.Contains(adminConnections.Body.String(), "Add connection") {
		t.Fatalf("admin connections=%d %s", adminConnections.Code, adminConnections.Body.String())
	}
	upload := multipartUpload(t, h, "/admin/frequency", adminCookies, map[string]string{"csrf_token": adminCSRF, "language": "de", "version": "web-v1", "replace": "on"}, "lemma,wortklasse,frequenzklasse\nHaus,Substantiv,2\n")
	if upload.Code != http.StatusSeeOther {
		t.Fatalf("admin upload=%d %s", upload.Code, upload.Body.String())
	}
	adminPage := perform(t, h, "GET", "/admin/frequency?language=de", nil, adminCookies)
	if adminPage.Code != 200 || !strings.Contains(adminPage.Body.String(), "web-v1") || !strings.Contains(adminPage.Body.String(), "Active") {
		t.Fatalf("admin frequency=%d %s", adminPage.Code, adminPage.Body.String())
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
