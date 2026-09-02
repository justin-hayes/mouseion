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
	"reflect"
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
	r.owner, r.source = owner, source
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
	bob := createAccount(t, ctx, store, "scope-web-bob", "bob-password", false)
	createBook := func(owner, language, identifier, firstTitle, secondTitle string) (domain.SourceMaterial, domain.ExtractedUnits) {
		t.Helper()
		firstText, secondText := "Erstes Kapitel.", "Bibliografia finale."
		firstManifestID, secondManifestID := identifier+"-chapter", identifier+"-bibliography"
		fullText := firstText + "\n\n" + secondText
		units := domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{
			{ID: domain.EPUBUnitID(0, firstManifestID), Order: 0, SpineIndex: 0, Title: firstTitle, TitleSource: domain.UnitTitleHeading, Text: firstText, EndOffset: uint64(len([]rune(firstText))), PackagePath: "OPS/package.opf", ResolvedHref: "OPS/Text/Teil/chapter.xhtml", ManifestID: firstManifestID, MediaType: "application/xhtml+xml", Linear: true},
			{ID: domain.EPUBUnitID(1, secondManifestID), Order: 1, SpineIndex: 1, Title: secondTitle, TitleSource: domain.UnitTitleManifestID, Text: secondText, StartOffset: uint64(len([]rune(firstText)) + 2), EndOffset: uint64(len([]rune(fullText))), PackagePath: "OPS/package.opf", ResolvedHref: "OPS/Text/Teil/bibliography.xhtml", ManifestID: secondManifestID, MediaType: "application/xhtml+xml", Linear: true},
		}}
		if language == "de" {
			units.Units[0].NavigationLabels = []string{"Teil Eins"}
			units.Units[1].NavigationLabels = []string{"Teil Eins"}
		} else {
			units.Units[0].ResolvedHref = "OPS/capitolo.xhtml"
			units.Units[1].ResolvedHref = "OPS/bibliografia.xhtml"
		}
		source, putErr := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner, Language: language, SourceIdentifier: identifier, Title: firstTitle, MediaType: "application/epub+zip", ContentHash: identifier, Content: []byte(fullText), FullText: fullText}, units)
		if putErr != nil {
			t.Fatal(putErr)
		}
		return source, units
	}
	german, germanUnits := createBook(alice.ID, "de", "review-de", "Erstes Kapitel", "bibliography")
	italian, _ := createBook(alice.ID, "it", "review-it", "Capitolo primo", "bibliografia")
	_, bobUnits := createBook(bob.ID, "de", "review-bob", "Privates Kapitel", "private-bibliography")
	germanSnapshot, _, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, german.ID)
	if err != nil {
		t.Fatal(err)
	}
	italianSnapshot, _, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, italian.ID)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &recordingAnalysis{}
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: store, Analysis: recorder, Capabilities: readyGerman(), SessionLifetime: time.Hour})
	cookies, csrf := loginCookies(t, h, "scope-web-alice", "alice-password")
	bobCookies, _ := loginCookies(t, h, "scope-web-bob", "bob-password")
	for _, test := range []struct{ id, title string }{{german.ID, "Erstes Kapitel"}, {italian.ID, "Capitolo primo"}} {
		page := perform(t, h, "GET", "/books/"+test.id+"/scope", nil, cookies)
		body := page.Body.String()
		for _, want := range []string{test.title, "Check all", "Uncheck all", "All units:", "Selected:", `aria-live="polite"`} {
			if page.Code != http.StatusOK || !strings.Contains(body, want) {
				t.Fatalf("scope page %s missing %q: status=%d body=%s", test.id, want, page.Code, body)
			}
		}
		for _, forbidden := range []string{"Category", "Confidence", "Recommendation", "Classification evidence", "High-confidence exclusion", "scope-groups"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("classifier-era scope UI contains %q", forbidden)
			}
		}
	}
	if got := perform(t, h, "GET", "/books/"+german.ID+"/scope", nil, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner scope review=%d %s", got.Code, got.Body.String())
	}
	if got := perform(t, h, "POST", "/books/"+german.ID+"/scope", url.Values{"unit_id": {germanUnits.Units[0].ID}}, cookies); got.Code != http.StatusForbidden {
		t.Fatalf("scope without csrf=%d", got.Code)
	}
	stale := perform(t, h, "POST", "/books/"+german.ID+"/scope", url.Values{"csrf_token": {csrf}, "snapshot_id": {"stale-snapshot"}, "unit_id": {germanUnits.Units[0].ID}}, cookies)
	if stale.Code != http.StatusBadRequest || !strings.Contains(stale.Body.String(), "snapshot changed") {
		t.Fatalf("stale scope=%d %s", stale.Code, stale.Body.String())
	}
	empty := perform(t, h, "POST", "/books/"+german.ID+"/scope", url.Values{"csrf_token": {csrf}, "snapshot_id": {germanSnapshot}}, cookies)
	if empty.Code != http.StatusBadRequest || !strings.Contains(empty.Body.String(), "Select at least one readable unit") {
		t.Fatalf("empty scope=%d %s", empty.Code, empty.Body.String())
	}
	foreign := perform(t, h, "POST", "/books/"+german.ID+"/scope", url.Values{"csrf_token": {csrf}, "snapshot_id": {germanSnapshot}, "unit_id": {bobUnits.Units[0].ID}}, cookies)
	if foreign.Code != http.StatusBadRequest || !strings.Contains(foreign.Body.String(), "does not belong to this book") {
		t.Fatalf("foreign scope=%d %s", foreign.Code, foreign.Body.String())
	}
	override := perform(t, h, "POST", "/books/"+german.ID+"/scope", url.Values{"csrf_token": {csrf}, "snapshot_id": {germanSnapshot}, "unit_id": {germanUnits.Units[0].ID, germanUnits.Units[1].ID}}, cookies)
	if override.Code != http.StatusSeeOther || !strings.Contains(override.Header().Get("Location"), "Analysis+scope+saved+with+2+selected+units") {
		t.Fatalf("override=%d location=%q body=%s", override.Code, override.Header().Get("Location"), override.Body.String())
	}
	var classifierName, classifierVersion, mode string
	var selected int
	if err = store.Pool().QueryRow(ctx, `SELECT s.classifier_name,s.classifier_version,s.selection_mode,count(u.unit_id) FROM epub_reviewed_scopes s JOIN epub_reviewed_scope_units u USING(scope_id) WHERE s.owner_id=$1 AND s.source_material_id=$2 GROUP BY s.classifier_name,s.classifier_version,s.selection_mode`, alice.ID, german.ID).Scan(&classifierName, &classifierVersion, &mode, &selected); err != nil || classifierName != "none" || classifierVersion != "none" || mode != "overridden" || selected != 2 {
		t.Fatalf("persisted classifier=%q/%q mode=%q selected=%d err=%v", classifierName, classifierVersion, mode, selected, err)
	}
	if recorder.owner != "" || recorder.source != "" || recorder.scope != "" {
		t.Fatalf("scope confirmation enqueued analysis owner=%q source=%q scope=%q", recorder.owner, recorder.source, recorder.scope)
	}
	var analysisJobs int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, german.ID).Scan(&analysisJobs); err != nil || analysisJobs != 0 {
		t.Fatalf("scope confirmation created analysis jobs=%d err=%v", analysisJobs, err)
	}
	var priorScopeID string
	if err = store.Pool().QueryRow(ctx, `SELECT scope_id::text FROM epub_reviewed_scopes WHERE owner_id=$1 AND source_material_id=$2 ORDER BY created_at DESC,scope_id DESC LIMIT 1`, alice.ID, german.ID).Scan(&priorScopeID); err != nil {
		t.Fatal(err)
	}
	if analyzed := perform(t, h, "POST", "/books/"+german.ID+"/analyze", url.Values{"csrf_token": {csrf}}, cookies); analyzed.Code != http.StatusSeeOther || recorder.scope != priorScopeID {
		t.Fatalf("explicit scoped analysis=%d scope=%q want %q body=%s", analyzed.Code, recorder.scope, priorScopeID, analyzed.Body.String())
	}
	expanded, err := store.GetEPUBReviewedScope(ctx, alice.ID, german.ID, priorScopeID)
	if err != nil || len(expanded.SelectedUnits) != 2 || expanded.SelectedUnits[0].UnitID != germanUnits.Units[0].ID || expanded.SelectedUnits[1].UnitID != germanUnits.Units[1].ID {
		t.Fatalf("group expansion selected unintended or unordered units: scope=%+v err=%v", expanded, err)
	}
	checklistReview := perform(t, h, "GET", "/books/"+german.ID+"/scope?preset=prior&prior_scope_id="+priorScopeID, nil, cookies)
	if checklistReview.Code != http.StatusOK || !strings.Contains(checklistReview.Body.String(), "Check all") || strings.Contains(checklistReview.Body.String(), "Scope comparison") {
		t.Fatalf("scope query did not remain classifier-free: status=%d body=%s", checklistReview.Code, checklistReview.Body.String())
	}
	if got := perform(t, h, "GET", "/books/"+german.ID+"/scope", nil, bobCookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner scope review=%d %s", got.Code, got.Body.String())
	}
	clone := perform(t, h, "POST", "/books/"+german.ID+"/scope", url.Values{"csrf_token": {csrf}, "snapshot_id": {germanSnapshot}, "unit_id": {germanUnits.Units[0].ID, germanUnits.Units[1].ID}}, cookies)
	if clone.Code != http.StatusSeeOther {
		t.Fatalf("clone=%d location=%q body=%s", clone.Code, clone.Header().Get("Location"), clone.Body.String())
	}
	var cloneScopeID string
	if err = store.Pool().QueryRow(ctx, `SELECT scope_id::text FROM epub_reviewed_scopes WHERE owner_id=$1 AND source_material_id=$2 ORDER BY created_at DESC,scope_id DESC LIMIT 1`, alice.ID, german.ID).Scan(&cloneScopeID); err != nil {
		t.Fatal(err)
	}
	if cloneScopeID != priorScopeID {
		t.Fatalf("idempotent clone created a new scope %q instead of reusing %q", cloneScopeID, priorScopeID)
	}
	var cloneCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM epub_reviewed_scopes WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, german.ID).Scan(&cloneCount); err != nil || cloneCount != 1 {
		t.Fatalf("immutable clone history count=%d err=%v", cloneCount, err)
	}
	cloned, err := store.GetEPUBReviewedScope(ctx, alice.ID, german.ID, cloneScopeID)
	if err != nil || !reflect.DeepEqual(cloned.SelectedUnits, expanded.SelectedUnits) || cloned.SourceUnitSnapshot != expanded.SourceUnitSnapshot || cloned.Classifier != expanded.Classifier {
		t.Fatalf("same-scope clone changed deterministic inputs: original=%+v clone=%+v err=%v", expanded, cloned, err)
	}
	_, italianUnits, err := store.GetExtractedUnitSnapshot(ctx, alice.ID, italian.ID)
	if err != nil {
		t.Fatal(err)
	}
	recommended := perform(t, h, "POST", "/books/"+italian.ID+"/scope", url.Values{"csrf_token": {csrf}, "snapshot_id": {italianSnapshot}, "unit_id": {italianUnits.Units[0].ID}}, cookies)
	if recommended.Code != http.StatusSeeOther {
		t.Fatalf("recommended=%d location=%q body=%s", recommended.Code, recommended.Header().Get("Location"), recommended.Body.String())
	}
	if err = store.Pool().QueryRow(ctx, `SELECT s.classifier_name,s.classifier_version,s.selection_mode,count(u.unit_id) FROM epub_reviewed_scopes s JOIN epub_reviewed_scope_units u USING(scope_id) WHERE s.owner_id=$1 AND s.source_material_id=$2 GROUP BY s.classifier_name,s.classifier_version,s.selection_mode`, alice.ID, italian.ID).Scan(&classifierName, &classifierVersion, &mode, &selected); err != nil || classifierName != "none" || classifierVersion != "none" || mode != "overridden" || selected != 1 {
		t.Fatalf("classifier=%q/%q mode=%q selected=%d err=%v", classifierName, classifierVersion, mode, selected, err)
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

func TestNLPCapabilityDiscoveryAndDegradedBehavior(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "nlp-web-integration-secret-0123456789")
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
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), "French") || !strings.Contains(settings.Body.String(), "(fr)") || strings.Contains(settings.Body.String(), "Italian") {
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
	t.Setenv("MOUSEION_SECRET", "language-web-integration-secret-0123456789")
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
	t.Setenv("MOUSEION_SECRET", "webapp-integration-secret-0123456789")
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
	createAccount(t, ctx, store, "empty", "empty-password", false)
	if _, err = store.PutSupportedLanguage(ctx, "de", "German"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutLanguageProfile(ctx, alice.ID, "de", "German"); err != nil {
		t.Fatal(err)
	}
	epubBytes := testEPUB(t)
	secondEPUBBytes := testEPUBVariant(t, "book-2", "Second Book", "Guten Tag Welt.")
	invalidEPUBBytes := []byte("not an epub")
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
			fmt.Fprintf(w, `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><title>German</title><entry><id>book-pdf</id><title>PDF Book</title><link rel="%s" type="application/pdf" href="/book.pdf"/></entry><entry><id>book-1</id><title>Test Book</title><link rel="%s" type="%s" href="/book.epub"/></entry><entry><id>book-2</id><title>Second Book</title><link rel="%s" type="%s" href="/book-2.epub"/></entry></feed>`, opds.AcquisitionRel, opds.AcquisitionRel, opds.EPUBMediaType, opds.AcquisitionRel, opds.EPUBMediaType)
		case "/book.epub":
			w.Header().Set("Content-Type", opds.EPUBMediaType)
			_, _ = w.Write(epubBytes)
		case "/book-2.epub":
			w.Header().Set("Content-Type", opds.EPUBMediaType)
			_, _ = w.Write(secondEPUBBytes)
		case "/invalid.epub":
			w.Header().Set("Content-Type", opds.EPUBMediaType)
			_, _ = w.Write(invalidEPUBBytes)
		case "/private.epub":
			w.WriteHeader(http.StatusUnauthorized)
		case "/upstream":
			w.WriteHeader(http.StatusBadGateway)
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
	emptyCookies, _ := loginCookies(t, h, "empty", "empty-password")
	firstHub := perform(t, h, "GET", "/connections", nil, emptyCookies)
	if firstHub.Code != http.StatusOK || !strings.Contains(firstHub.Body.String(), "Add your first catalog connection") || !strings.Contains(firstHub.Body.String(), "Add catalog connection") {
		t.Fatalf("empty acquisition hub=%d %s", firstHub.Code, firstHub.Body.String())
	}
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
	unsupportedAcquire := perform(t, h, "POST", "/opds/acquire", url.Values{"csrf_token": {csrf}, "connection": {connection.ID}, "language": {"xx"}, "acquisition": {h.clientTargetToken(connection.ID, "xx", &opds.Entry{ID: "book-1", Title: "Test Book"}, catalog.URL+"/book.epub")}}, cookies)
	if unsupportedAcquire.Code != http.StatusBadRequest || catalogRequests != requestsBeforeUnsupported {
		t.Fatalf("unsupported acquire=%d requests=%d want %d", unsupportedAcquire.Code, catalogRequests, requestsBeforeUnsupported)
	}
	browse := perform(t, h, "GET", "/opds/browse?connection="+connection.ID+"&language=de", nil, cookies)
	if browse.Code != 200 || !strings.Contains(browse.Body.String(), "Test Book") || !strings.Contains(browse.Body.String(), "Search is not available") {
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
	if got := perform(t, h, "POST", "/opds/acquire", url.Values{"csrf_token": {csrf}, "connection": {bobConnection.ID}, "language": {"de"}, "acquisition": {h.clientTargetToken(bobConnection.ID, "de", &opds.Entry{ID: "book-1", Title: "Test Book"}, catalog.URL+"/book.epub")}}, cookies); got.Code != http.StatusNotFound {
		t.Fatalf("cross-owner acquire=%d %s", got.Code, got.Body.String())
	}
	catalogPage := perform(t, h, "GET", "/catalog?connection="+connection.ID, nil, cookies)
	if catalogPage.Code != 200 || !strings.Contains(catalogPage.Body.String(), `value="de"`) {
		t.Fatalf("catalog=%d %s", catalogPage.Code, catalogPage.Body.String())
	}
	languageBooks := perform(t, h, "GET", "/opds/language?connection="+connection.ID+"&language=de", nil, cookies)
	if languageBooks.Code != 200 || !strings.Contains(languageBooks.Body.String(), "<!doctype html>") || !strings.Contains(languageBooks.Body.String(), "Test Book") || strings.Contains(languageBooks.Body.String(), "PDF Book") {
		t.Fatalf("language browse=%d %s", languageBooks.Code, languageBooks.Body.String())
	}
	invalidRequest := url.Values{"csrf_token": {csrf}, "connection": {connection.ID}, "language": {"de"}, "acquisition": {h.clientTargetToken(connection.ID, "de", &opds.Entry{ID: "invalid", Title: "Broken EPUB"}, catalog.URL+"/invalid.epub")}}
	invalid := httptest.NewRecorder()
	invalidHTTP := httptest.NewRequest("POST", "/opds/acquire", strings.NewReader(invalidRequest.Encode()))
	invalidHTTP.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	invalidHTTP.Header.Set("HX-Request", "true")
	for _, cookie := range cookies {
		invalidHTTP.AddCookie(cookie)
	}
	h.ServeHTTP(invalid, invalidHTTP)
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "not a valid EPUB") || !strings.Contains(invalid.Body.String(), "No book was added") {
		t.Fatalf("invalid acquisition=%d %s", invalid.Code, invalid.Body.String())
	}
	authFailure := url.Values{"csrf_token": {csrf}, "connection": {connection.ID}, "language": {"de"}, "acquisition": {h.clientTargetToken(connection.ID, "de", &opds.Entry{ID: "private", Title: "Private EPUB"}, catalog.URL+"/private.epub")}}
	authFailureResponse := httptest.NewRecorder()
	authHTTP := httptest.NewRequest("POST", "/opds/acquire", strings.NewReader(authFailure.Encode()))
	authHTTP.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	authHTTP.Header.Set("HX-Request", "true")
	for _, cookie := range cookies {
		authHTTP.AddCookie(cookie)
	}
	h.ServeHTTP(authFailureResponse, authHTTP)
	if authFailureResponse.Code != http.StatusBadGateway || !strings.Contains(authFailureResponse.Body.String(), "rejected the credentials") || !strings.Contains(authFailureResponse.Body.String(), "Library") || !strings.Contains(authFailureResponse.Body.String(), "Edit connection") {
		t.Fatalf("authentication acquisition=%d %s", authFailureResponse.Code, authFailureResponse.Body.String())
	}
	upstream := perform(t, h, "GET", "/opds/browse?connection="+connection.ID+"&language=de&url="+url.QueryEscape(catalog.URL+"/upstream"), nil, cookies)
	if upstream.Code != http.StatusBadGateway || !strings.Contains(upstream.Body.String(), "Library") || !strings.Contains(upstream.Body.String(), "Retry") || !strings.Contains(upstream.Body.String(), "<!doctype html>") {
		t.Fatalf("upstream browse=%d %s", upstream.Code, upstream.Body.String())
	}
	validToken := hiddenInputValue(t, languageBooks.Body.String(), "acquisition")
	csrfMissing := url.Values{"connection": {connection.ID}, "language": {"de"}, "acquisition": {validToken}}
	if got := perform(t, h, "POST", "/opds/acquire", csrfMissing, cookies); got.Code != http.StatusForbidden {
		t.Fatalf("acquire without csrf=%d", got.Code)
	}
	acquireForm := url.Values{"csrf_token": {csrf}, "connection": {connection.ID}, "language": {"de"}, "acquisition": {validToken}, "return_to": {"/opds/language?connection=" + connection.ID + "&language=de"}}
	acquired := perform(t, h, "POST", "/opds/acquire", acquireForm, cookies)
	if acquired.Code != http.StatusSeeOther {
		t.Fatalf("acquire=%d %s", acquired.Code, acquired.Body.String())
	}
	if recorder.owner != "" || recorder.source != "" {
		t.Fatalf("OPDS acquisition started analysis owner=%q source=%q", recorder.owner, recorder.source)
	}
	if acquired.Header().Get("Location") == "" || !strings.Contains(acquired.Header().Get("Location"), "/opds/language") {
		t.Fatalf("acquire did not preserve browse context: %q", acquired.Header().Get("Location"))
	}
	acquisitionCookieValue := cookieNamed(t, acquired.Result().Cookies(), acquisitionCookie)
	if len(acquisitionCookieValue.Value) > maxAcquisitionCookieBytes || strings.Contains(acquisitionCookieValue.Value, "book.epub") {
		t.Fatalf("acquisition cookie is too large or contains catalog URL: %d %q", len(acquisitionCookieValue.Value), acquisitionCookieValue.Value)
	}
	contextCookies := append(append([]*http.Cookie{}, cookies...), acquisitionCookieValue)
	addedReturn := perform(t, h, "GET", acquired.Header().Get("Location"), nil, contextCookies)
	if addedReturn.Code != http.StatusOK || !strings.Contains(addedReturn.Body.String(), "Added to My Books") || !strings.Contains(addedReturn.Body.String(), "Already in My Books") {
		t.Fatalf("added full-page return=%d %s", addedReturn.Code, addedReturn.Body.String())
	}
	rememberedFeed := perform(t, h, "GET", "/opds/language?connection="+connection.ID+"&language=de", nil, contextCookies)
	if rememberedFeed.Code != http.StatusOK || !strings.Contains(rememberedFeed.Body.String(), "Already in My Books") || !strings.Contains(rememberedFeed.Body.String(), "Open owned book") {
		t.Fatalf("remembered acquisition state=%d %s", rememberedFeed.Code, rememberedFeed.Body.String())
	}
	duplicateFullPage := perform(t, h, "POST", "/opds/acquire", acquireForm, contextCookies)
	duplicateAcquisitionCookie := cookieNamed(t, duplicateFullPage.Result().Cookies(), acquisitionCookie)
	duplicateReturn := perform(t, h, "GET", duplicateFullPage.Header().Get("Location"), nil, append(append([]*http.Cookie{}, cookies...), duplicateAcquisitionCookie))
	if duplicateFullPage.Code != http.StatusSeeOther || duplicateReturn.Code != http.StatusOK || !strings.Contains(duplicateReturn.Body.String(), "That book is already in My Books") || !strings.Contains(duplicateReturn.Body.String(), "Already in My Books") {
		t.Fatalf("duplicate full-page return=%d location=%q body=%s", duplicateFullPage.Code, duplicateFullPage.Header().Get("Location"), duplicateReturn.Body.String())
	}
	logout := perform(t, h, "POST", "/logout", url.Values{"csrf_token": {csrf}}, cookies)
	if logout.Code != http.StatusSeeOther || cookieNamed(t, logout.Result().Cookies(), acquisitionCookie).MaxAge != -1 {
		t.Fatalf("logout did not clear acquisition state: %d cookies=%v", logout.Code, logout.Result().Cookies())
	}
	cookies, csrf = loginCookies(t, h, "alice", "alice-password")
	jsonLogoutRequest := httptest.NewRequest("POST", "/logout", strings.NewReader(`{}`))
	jsonLogoutRequest.Header.Set("Content-Type", "application/json")
	for _, cookie := range append(append([]*http.Cookie{}, cookies...), acquisitionCookieValue) {
		jsonLogoutRequest.AddCookie(cookie)
	}
	jsonLogout := httptest.NewRecorder()
	h.ServeHTTP(jsonLogout, jsonLogoutRequest)
	if jsonLogout.Code != http.StatusNoContent || cookieNamed(t, jsonLogout.Result().Cookies(), webauth.CookieName).MaxAge != -1 || cookieNamed(t, jsonLogout.Result().Cookies(), acquisitionCookie).MaxAge != -1 {
		t.Fatalf("JSON logout did not clear session and acquisition state: %d cookies=%v", jsonLogout.Code, jsonLogout.Result().Cookies())
	}
	cookies, csrf = loginCookies(t, h, "alice", "alice-password")
	jsonLogoutAllRequest := httptest.NewRequest("POST", "/logout-all", strings.NewReader(`{}`))
	jsonLogoutAllRequest.Header.Set("Content-Type", "application/json; charset=utf-8")
	for _, cookie := range append(append([]*http.Cookie{}, cookies...), acquisitionCookieValue) {
		jsonLogoutAllRequest.AddCookie(cookie)
	}
	jsonLogoutAll := httptest.NewRecorder()
	h.ServeHTTP(jsonLogoutAll, jsonLogoutAllRequest)
	if jsonLogoutAll.Code != http.StatusNoContent || cookieNamed(t, jsonLogoutAll.Result().Cookies(), webauth.CookieName).MaxAge != -1 || cookieNamed(t, jsonLogoutAll.Result().Cookies(), acquisitionCookie).MaxAge != -1 {
		t.Fatalf("JSON logout-all did not clear session and acquisition state: %d cookies=%v", jsonLogoutAll.Code, jsonLogoutAll.Result().Cookies())
	}
	cookies, csrf = loginCookies(t, h, "alice", "alice-password")
	htmlLogoutAll := perform(t, h, "POST", "/logout-all", url.Values{"csrf_token": {csrf}}, append(cookies, acquisitionCookieValue))
	if htmlLogoutAll.Code != http.StatusSeeOther || htmlLogoutAll.Header().Get("Location") != "/login" || cookieNamed(t, htmlLogoutAll.Result().Cookies(), webauth.CookieName).MaxAge != -1 || cookieNamed(t, htmlLogoutAll.Result().Cookies(), acquisitionCookie).MaxAge != -1 {
		t.Fatalf("HTML logout-all changed CSRF/redirect behavior: %d location=%q cookies=%v", htmlLogoutAll.Code, htmlLogoutAll.Header().Get("Location"), htmlLogoutAll.Result().Cookies())
	}
	cookies, csrf = loginCookies(t, h, "alice", "alice-password")
	acquireForm.Set("csrf_token", csrf)
	stale := append(append([]*http.Cookie{}, cookies...), acquisitionCookieValue)
	if stalePage := perform(t, h, "GET", "/opds/language?connection="+connection.ID+"&language=de", nil, stale); stalePage.Code != http.StatusOK || strings.Contains(stalePage.Body.String(), "Already in My Books") {
		t.Fatalf("stale acquisition state survived session rotation: %d %s", stalePage.Code, stalePage.Body.String())
	}
	library := perform(t, h, "GET", "/library", nil, cookies)
	if library.Code != 200 || !strings.Contains(library.Body.String(), "Test Book") || !strings.Contains(library.Body.String(), "Scope review required") {
		t.Fatalf("library=%d %s", library.Code, library.Body.String())
	}
	metadataForm := url.Values{"csrf_token": {csrf}, "title": {"Metadata-only web book"}, "language_state": {domain.LanguageUnknown}}
	metadataResponse := perform(t, h, "POST", "/library/books", metadataForm, cookies)
	if metadataResponse.Code != http.StatusSeeOther || !strings.Contains(metadataResponse.Header().Get("Location"), "/library?message=") {
		t.Fatalf("metadata create=%d location=%q body=%s", metadataResponse.Code, metadataResponse.Header().Get("Location"), metadataResponse.Body.String())
	}
	metadataBooks, metadataErr := store.ListMyBooksWithEvidence(ctx, alice.ID)
	if metadataErr != nil {
		t.Fatal(metadataErr)
	}
	var metadataBook domain.MyBook
	for _, candidate := range metadataBooks {
		if candidate.Book.Title == "Metadata-only web book" {
			metadataBook = candidate
			break
		}
	}
	if metadataBook.Book.ID == "" || metadataBook.Acquired != nil || metadataBook.EvidenceState != domain.MyBookNotAcquired {
		t.Fatalf("metadata handler read model=%+v", metadataBook)
	}
	repeatedMetadata := perform(t, h, "POST", "/library/books", metadataForm, cookies)
	if repeatedMetadata.Code != http.StatusSeeOther {
		t.Fatalf("repeated metadata create=%d %s", repeatedMetadata.Code, repeatedMetadata.Body.String())
	}
	metadataBooks, metadataErr = store.ListMyBooksWithEvidence(ctx, alice.ID)
	if metadataErr != nil {
		t.Fatal(metadataErr)
	}
	metadataCount := 0
	for _, candidate := range metadataBooks {
		if candidate.Book.Title == "Metadata-only web book" {
			metadataCount++
		}
	}
	if metadataCount != 1 {
		t.Fatalf("repeated metadata create count=%d", metadataCount)
	}
	removeMetadata := url.Values{"csrf_token": {csrf}}
	if removed := perform(t, h, "POST", "/library/books/"+metadataBook.Book.ID+"/remove", removeMetadata, cookies); removed.Code != http.StatusSeeOther {
		t.Fatalf("metadata removal=%d %s", removed.Code, removed.Body.String())
	}
	if repeatedRemoval := perform(t, h, "POST", "/library/books/"+metadataBook.Book.ID+"/remove", removeMetadata, cookies); repeatedRemoval.Code != http.StatusSeeOther {
		t.Fatalf("repeated metadata removal=%d %s", repeatedRemoval.Code, repeatedRemoval.Body.String())
	}
	books, err := store.ListSourceMaterials(ctx, alice.ID)
	if err != nil || len(books) != 1 {
		t.Fatalf("OPDS acquisition books=%d err=%v", len(books), err)
	}
	bookID := books[0].Source.ID
	var analysisJobs int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id=$1`, alice.ID).Scan(&analysisJobs); err != nil || analysisJobs != 0 {
		t.Fatalf("OPDS acquisition analysis jobs=%d err=%v", analysisJobs, err)
	}
	secondToken := hiddenInputValues(t, languageBooks.Body.String(), "acquisition")[1]
	secondForm := url.Values{"csrf_token": {csrf}, "connection": {connection.ID}, "language": {"de"}, "acquisition": {secondToken}}
	secondRequest := httptest.NewRequest("POST", "/opds/acquire", strings.NewReader(secondForm.Encode()))
	secondRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	secondRequest.Header.Set("HX-Request", "true")
	for _, cookie := range cookies {
		secondRequest.AddCookie(cookie)
	}
	secondResponse := httptest.NewRecorder()
	h.ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusOK || !strings.Contains(secondResponse.Body.String(), "Added to My Books") || !strings.Contains(secondResponse.Body.String(), "/books/") {
		t.Fatalf("second OPDS acquisition=%d %s", secondResponse.Code, secondResponse.Body.String())
	}
	duplicateResponse := httptest.NewRecorder()
	duplicateRequest := httptest.NewRequest("POST", "/opds/acquire", strings.NewReader(acquireForm.Encode()))
	duplicateRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	duplicateRequest.Header.Set("HX-Request", "true")
	for _, cookie := range cookies {
		duplicateRequest.AddCookie(cookie)
	}
	h.ServeHTTP(duplicateResponse, duplicateRequest)
	if duplicateResponse.Code != http.StatusOK || !strings.Contains(duplicateResponse.Body.String(), "Already in My Books") {
		t.Fatalf("duplicate OPDS acquisition=%d %s", duplicateResponse.Code, duplicateResponse.Body.String())
	}
	books, err = store.ListSourceMaterials(ctx, alice.ID)
	if err != nil || len(books) != 2 {
		t.Fatalf("multi-add books=%d err=%v", len(books), err)
	}
	bookPage := perform(t, h, "GET", "/books/"+bookID, nil, cookies)
	if bookPage.Code != 200 || !strings.Contains(bookPage.Body.String(), "Review scope") {
		t.Fatalf("book=%d %s", bookPage.Code, bookPage.Body.String())
	}
	snapshotID, units, snapshotErr := store.GetExtractedUnitSnapshot(ctx, alice.ID, bookID)
	if snapshotErr != nil || len(units.Units) == 0 {
		t.Fatalf("acquired EPUB units=%d snapshot=%q err=%v", len(units.Units), snapshotID, snapshotErr)
	}
	scopeForm := url.Values{"csrf_token": {csrf}, "snapshot_id": {snapshotID}}
	for _, unit := range units.Units {
		scopeForm.Add("unit_id", unit.ID)
	}
	scope := perform(t, h, "POST", "/books/"+bookID+"/scope", scopeForm, cookies)
	if scope.Code != http.StatusSeeOther {
		t.Fatalf("scope confirmation=%d %s", scope.Code, scope.Body.String())
	}
	if got := perform(t, h, "POST", "/books/"+bookID+"/analyze", nil, cookies); got.Code != http.StatusForbidden {
		t.Fatalf("analysis without csrf=%d", got.Code)
	}
	resubmitted := perform(t, h, "POST", "/books/"+bookID+"/analyze", url.Values{"csrf_token": {csrf}}, cookies)
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
	var scopeID, scopeSnapshotID string
	if err = store.Pool().QueryRow(ctx, `SELECT scope_id::text,snapshot_id::text FROM epub_reviewed_scopes WHERE owner_id=$1 AND source_material_id=$2 ORDER BY created_at DESC,scope_id DESC LIMIT 1`, alice.ID, recorder.source).Scan(&scopeID, &scopeSnapshotID); err != nil {
		t.Fatal(err)
	}
	acquiredSource, sourceErr := store.GetSourceMaterial(ctx, alice.ID, recorder.source)
	if sourceErr != nil {
		t.Fatal(sourceErr)
	}
	var runID string
	if err = store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,scope_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,$5,'test-analyzer','1','test-config','completed',now()) RETURNING id::text`, alice.ID, recorder.source, acquiredSource.ContentRevisionID, scopeID, scopeSnapshotID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	_, err = store.PutCorpus(ctx, alice.ID, recorder.source, artifactHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE corpora SET reviewed_scope_id=$1,analysis_run_id=$2,status='complete' WHERE owner_id=$3 AND source_material_id=$4`, scopeID, runID, alice.ID, recorder.source); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=(SELECT id FROM corpora WHERE owner_id=$1 AND source_material_id=$2) WHERE owner_id=$1 AND id=$3`, alice.ID, recorder.source, runID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,display_number,owner_id,source_material_id,content_hash,corpus_id,reviewed_scope_id,analysis_run_id,progress) VALUES(42,1,$1,$2,$3,(SELECT id FROM corpora WHERE owner_id=$1 AND source_material_id=$2),$4,$5,100)`, alice.ID, recorder.source, acquiredSource.ContentHash, scopeID, runID); err != nil {
		t.Fatal(err)
	}
	library = perform(t, h, "GET", "/library", nil, cookies)
	if library.Code != 200 || !strings.Contains(library.Body.String(), "Analysis result ready") {
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
	for _, want := range []string{"Text profile", "20", "12.0", "38", "10.0%", "200 of 240", "55.0%", "current-known coverage", "active-campaign projected coverage", "200", "8", "110", "90", "lemmas for 95%", "lemmas for 97%", "lemmas for 99%", "graduated by completed campaigns", "legacy generated history", "deck-eligible vocabulary", "Highest-impact unknown vocabulary", "wichtig", "30 occurrences", "80.0%", "Projected token coverage", "85.0%", "after top 10 lemmas", "after top 25 lemmas", "after top 50 lemmas", "View analysis result"} {
		if !strings.Contains(bookPage.Body.String(), want) {
			t.Errorf("coverage page missing %q", want)
		}
	}
	settingsPage := perform(t, h, "GET", "/settings?language=de", nil, cookies)
	if settingsPage.Code != 200 || !strings.Contains(settingsPage.Body.String(), "Account settings") || !strings.Contains(settingsPage.Body.String(), "German") || !strings.Contains(settingsPage.Body.String(), "Import known vocabulary") || !strings.Contains(settingsPage.Body.String(), `id="known-vocabulary"`) {
		t.Fatalf("settings page=%d %s", settingsPage.Code, settingsPage.Body.String())
	}
	knownPage := perform(t, h, "GET", "/known-vocab?language=de", nil, cookies)
	if knownPage.Code != http.StatusSeeOther || knownPage.Header().Get("Location") != "/settings?language=de#known-vocabulary" {
		t.Fatalf("known vocab redirect=%d location=%q", knownPage.Code, knownPage.Header().Get("Location"))
	}
	if got := multipartUpload(t, h, "/known-vocab/import", cookies, map[string]string{"language": "de"}, "Daß\nbad\tNOPE\n"); got.Code != http.StatusForbidden {
		t.Fatalf("known vocab without csrf=%d", got.Code)
	}
	importedKnown := multipartUpload(t, h, "/known-vocab/import", cookies, map[string]string{"csrf_token": csrf, "language": "de"}, "Daß\nbad\tNOPE\n")
	if importedKnown.Code != http.StatusSeeOther || importedKnown.Header().Get("Location") != "/known-vocab/imports/77/status" {
		t.Fatalf("known vocab import=%d %s", importedKnown.Code, importedKnown.Body.String())
	}
	importStatus := perform(t, h, "GET", "/known-vocab/imports/77/status", nil, cookies)
	if importStatus.Code != 200 || !strings.Contains(importStatus.Body.String(), "1 new") || !strings.Contains(importStatus.Body.String(), "no tab-separated columns") || !strings.Contains(importStatus.Body.String(), "<td>2</td>") {
		t.Fatalf("known vocab status=%d %s", importStatus.Code, importStatus.Body.String())
	}
	bobLogin := perform(t, h, "POST", "/login", url.Values{"csrf_token": {csrf}, "username": {bob.Username}, "password": {"bob-password"}}, []*http.Cookie{cookies[0]})
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
	if !strings.Contains(bookPage.Body.String(), "View analysis result") || !strings.Contains(bookPage.Body.String(), "/analyses/") || strings.Contains(bookPage.Body.String(), `action="/books/`+recorder.source+`/deck/preparations"`) || strings.Contains(bookPage.Body.String(), "/review?") || strings.Contains(bookPage.Body.String(), "filter_known") || strings.Contains(bookPage.Body.String(), "ranking") {
		t.Fatalf("book deck flow not unified: %s", bookPage.Body.String())
	}
	if _, err = externalJobs.SubmitEnrichment(ctx, alice.ID, []enrichment.Candidate{{Identity: enrichment.Identity{Language: "de", CanonicalLemma: "wichtig", UPOS: "ADJ"}}, {Identity: enrichment.Identity{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB"}}}); err != nil {
		t.Fatal(err)
	}
	statusPage := perform(t, h, "GET", "/enrichment-jobs/88/status", nil, cookies)
	if statusPage.Code != http.StatusOK || !strings.Contains(statusPage.Body.String(), "Contextual translations: Running") || !strings.Contains(statusPage.Body.String(), "1 of 2 translations complete. Attempt 2.") || !strings.Contains(statusPage.Body.String(), "Translation needs attention") || !strings.Contains(statusPage.Body.String(), `aria-busy="true"`) {
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
	if got := perform(t, h, "GET", "/known-vocab?language=de", nil, bobCookies); got.Code != http.StatusSeeOther || got.Header().Get("Location") != "/settings#known-vocabulary" {
		t.Fatalf("bob known vocabulary redirect=%d location=%q", got.Code, got.Header().Get("Location"))
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
	uppercaseCatalogURL := "HTTP" + strings.TrimPrefix(catalog.URL, "http")
	if got := perform(t, h, "POST", "/connections", url.Values{"csrf_token": {csrf}, "name": {"Personal"}, "url": {uppercaseCatalogURL}, "language": {"de"}}, cookies); got.Code != http.StatusSeeOther {
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
	if personal.URL != catalog.URL {
		t.Fatalf("connection form did not normalize scheme: got %q want %q", personal.URL, catalog.URL)
	}
	credentialURL := catalog.URL + "/opds?access_token=keep-this-secret&view=books"
	if got := perform(t, h, "POST", "/connections/"+personal.ID, url.Values{"csrf_token": {csrf}, "name": {"Renamed personal"}, "url": {credentialURL}, "language": {"de"}}, cookies); got.Code != http.StatusSeeOther {
		t.Fatalf("learner connection update=%d %s", got.Code, got.Body.String())
	}
	connectionsPage := perform(t, h, "GET", "/connections", nil, cookies)
	if connectionsPage.Code != http.StatusOK || !strings.Contains(connectionsPage.Body.String(), "Renamed personal") || !strings.Contains(connectionsPage.Body.String(), "Choose a catalog to browse") || !strings.Contains(connectionsPage.Body.String(), "Catalog maintenance") || strings.Contains(connectionsPage.Body.String(), "Private") || strings.Contains(connectionsPage.Body.String(), "keep-this-secret") || strings.Contains(connectionsPage.Body.String(), "[redacted]") {
		t.Fatalf("learner connection page=%d %s", connectionsPage.Code, connectionsPage.Body.String())
	}
	if got, err := store.GetOpdsConnection(ctx, alice.ID, personal.ID); err != nil || got.URL != credentialURL {
		t.Fatalf("explicit credential URL=%q err=%v", got.URL, err)
	}
	if got := perform(t, h, "POST", "/connections/"+personal.ID, url.Values{"csrf_token": {csrf}, "name": {"Renamed personal"}, "url": {""}, "language": {"de"}}, cookies); got.Code != http.StatusSeeOther {
		t.Fatalf("keep-current connection update=%d %s", got.Code, got.Body.String())
	}
	if got, err := store.GetOpdsConnection(ctx, alice.ID, personal.ID); err != nil || got.URL != credentialURL {
		t.Fatalf("blank URL changed credential URL=%q err=%v", got.URL, err)
	}
	replacementURL := catalog.URL + "/opds?access_token=replacement-secret&view=authors"
	if got := perform(t, h, "POST", "/connections/"+personal.ID, url.Values{"csrf_token": {csrf}, "name": {"Renamed personal"}, "url": {replacementURL}, "language": {"de"}}, cookies); got.Code != http.StatusSeeOther {
		t.Fatalf("replacement connection update=%d %s", got.Code, got.Body.String())
	}
	if got, err := store.GetOpdsConnection(ctx, alice.ID, personal.ID); err != nil || got.URL != replacementURL {
		t.Fatalf("replacement URL=%q err=%v", got.URL, err)
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
		book, createErr := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown})
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
