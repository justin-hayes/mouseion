package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type unavailableCurrentReadingPreparedDeck struct {
	fixtures.PreparedDeck
}

func (unavailableCurrentReadingPreparedDeck) GetForGoalSnapshot(context.Context, string, string) (domain.DeckPreparation, error) {
	return domain.DeckPreparation{}, persistence.ErrNotFound
}

type existingCurrentReadingPreparedDeck struct {
	fixtures.PreparedDeck
	preparation               domain.DeckPreparation
	retries                   int
	currentReadingSubmissions int
	genericSubmissions        int
	cancellations             int
}

func (p *existingCurrentReadingPreparedDeck) GetForGoalSnapshot(context.Context, string, string) (domain.DeckPreparation, error) {
	return p.preparation, nil
}

func (p *existingCurrentReadingPreparedDeck) Retry(_ context.Context, _, _ string) (prepareddeck.Handle, error) {
	p.retries++
	return prepareddeck.Handle{Preparation: p.preparation, JobID: 9}, nil
}

func (p *existingCurrentReadingPreparedDeck) SubmitForGoal(context.Context, string, string, string) (prepareddeck.Handle, error) {
	p.currentReadingSubmissions++
	return prepareddeck.Handle{Preparation: p.preparation, JobID: 9}, nil
}

func (p *existingCurrentReadingPreparedDeck) Submit(context.Context, string, string) (prepareddeck.Handle, error) {
	p.genericSubmissions++
	return prepareddeck.Handle{Preparation: p.preparation, JobID: 9}, nil
}

func (p *existingCurrentReadingPreparedDeck) Cancel(context.Context, string, string) (domain.DeckPreparation, error) {
	p.cancellations++
	return p.preparation, nil
}

func renderCurrentReadingSection(t *testing.T, currentReading *readingBookView, message, pageError, focusBookID string) string {
	t.Helper()
	var output bytes.Buffer
	require.NoError(t, CurrentReadingSection(currentReading, "csrf-token", message, pageError, focusBookID).Render(context.Background(), &output))
	return output.String()
}

func TestCurrentReadingSectionRendersEmptyStateAndLiveFeedback(t *testing.T) {
	html := renderCurrentReadingSection(t, nil, "current reading cleared.", "", "")
	for _, want := range []string{
		`id="primary-goal-section"`,
		`id="goal-section-status"`,
		`role="status"`,
		"current reading cleared.",
		"No current Book yet",
		"Start an analyzed To Read book when you are ready to begin reading.",
	} {
		assert.True(t, strings.Contains(html, want), "Goal section missing %q: %s", want, html)
	}

	html = renderCurrentReadingSection(t, nil, "", "This current reading changed since this page was loaded.", "")
	assert.True(t, strings.Contains(html, `role="alert"`) && strings.Contains(html, "This current reading changed since this page was loaded"), "Goal section did not render an accessible error: %s", html)
}

func TestCurrentReadingSectionRendersReadingOnlyAndResidualStates(t *testing.T) {
	unassessed := testReadingBook("reading-only", "Reading-only book", testNoContent)
	unassessed.ReadingOnly = true
	unassessed.CurrentReadingUnassessed = true
	readingOnlyHTML := renderCurrentReadingSection(t, &unassessed, "", "", "reading-only")
	for _, want := range []string{"Reading only", "No analysis or deck-eligible vocabulary exists yet", "Reading directly is the current path"} {
		assert.True(t, strings.Contains(readingOnlyHTML, want), "unassessed Goal missing %q: %s", want, readingOnlyHTML)
	}

	assessed := testReadingBook("assessed", "Assessed without deck", testAnalyzed)
	assessed.ReadingOnly = true
	assessed.CurrentReadingUnassessed = false
	assessedHTML := renderCurrentReadingSection(t, &assessed, "", "", "assessed")
	assert.True(t, strings.Contains(assessedHTML, "Analysis evidence exists, but this Book has no deck-eligible vocabulary"), "assessed reading-only copy missing: %s", assessedHTML)

	currentReading := testReadingBook("goal", "Goal book", testAnalyzed)
	currentReadingHTML := renderCurrentReadingSection(t, &currentReading, "", "", "goal")
	assert.False(t, strings.Contains(currentReadingHTML, "Clear Current reading"), "retired Goal clear action remained visible: %s", currentReadingHTML)
}

func TestIsCurrentReadingControlsUseExpectedStateAndStaySeparated(t *testing.T) {
	currentReading := testReadingBook("goal", "Goal book", testAnalyzed)
	currentReading.CurrentReadingSnapshotID = "goal-snapshot"
	html := renderReading(t, readingPageView{CurrentReading: &currentReading}, "", "")
	assert.NotContains(t, html, `action="/goal/books/`)
	assert.Contains(t, html, `action="/reading/finish"`)
	assert.True(t, strings.Contains(html, `name="expected_current_book_id" value="goal"`), "finish form did not carry the current Book: %s", html)
	assert.True(t, strings.Contains(html, `name="expected_current_snapshot_id" value="goal-snapshot"`), "finish form did not carry the current Book snapshot: %s", html)
	currentReadingStart := strings.Index(html, `id="journey-book-goal"`)
	currentReadingEnd := strings.Index(html[currentReadingStart:], "</article>")
	currentReadingCard := html[currentReadingStart : currentReadingStart+currentReadingEnd]
	assert.False(t, strings.Contains(currentReadingCard, `action="/goal/books/`), "current Book card exposed a legacy Goal control: %s", currentReadingCard)
}

func TestMyBooksCurrentReadingControlsAndReadingLink(t *testing.T) {
	books := []domain.MyBook{
		{Book: domain.Book{ID: "goal-book", OwnerID: "owner", Title: "Current goal"}, IsCurrentReading: true},
		{Book: domain.Book{ID: "other-book", OwnerID: "owner", Title: "Other book"}},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", "goal-book", true, MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	currentReadingStart := strings.Index(html, `aria-labelledby="book-title-goal-book"`)
	otherStart := strings.Index(html, `aria-labelledby="book-title-other-book"`)
	require.True(t, currentReadingStart >= 0 && otherStart >= 0, "book cards missing: %s", html)
	currentReadingCard := html[currentReadingStart:otherStart]
	otherCard := html[otherStart:]
	for _, want := range []string{"Currently reading", "View in Reading", "/reading#journey-book-goal-book"} {
		assert.True(t, strings.Contains(currentReadingCard, want), "current Book card missing %q: %s", want, currentReadingCard)
	}
	assert.False(t, strings.Contains(currentReadingCard, "To Read"), "current Book was also presented as To Read: %s", currentReadingCard)
	assert.False(t, strings.Contains(otherCard, "Start reading") || strings.Contains(otherCard, `action="/goal/books/other-book"`), "My Books exposed a choose form: %s", otherCard)
}

func readingFixtureSession(t *testing.T) (http.Handler, []*http.Cookie, string, *fixtures.Store) {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "goal-unit-test-secret-0123456789")
	store := fixtures.NewStore()
	authService := auth.New(fixtures.NewAuthStore(), time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), Analysis: fixtures.Analysis{Store: store}, CatalogueSync: fixtures.NewCatalogueSync(store), PreparedDeck: fixtures.PreparedDeck{Store: store}, SessionLifetime: time.Hour})
	loginPage := httptest.NewRecorder()
	h.ServeHTTP(loginPage, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login", nil))
	initialCSRFCookie := cookieByName(t, loginPage.Result().Cookies(), csrfCookie)
	token := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(loginPage.Body.String())[1]
	loginRequest := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/login", strings.NewReader(url.Values{"csrf_token": {token}, "username": {fixtures.Username}, "password": {fixtures.Password}}.Encode()))
	loginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRequest.AddCookie(initialCSRFCookie)
	loginResponse := httptest.NewRecorder()
	h.ServeHTTP(loginResponse, loginRequest)
	require.Equal(t, http.StatusSeeOther, loginResponse.Code)
	cookies := loginResponse.Result().Cookies()
	return h, cookies, cookieByName(t, cookies, csrfCookie).Value, store
}

func cookieByName(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	require.FailNow(t, "cookie %q not found", name)
	return nil
}

func readingTestRequest(t *testing.T, h http.Handler, path string, form url.Values, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

func TestCanonicalCurrentReadingStartSwitchAndStopAreIdempotent(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	idempotent := readingTestRequest(t, h, "/reading/books/"+fixtures.BookID+"/start", url.Values{
		"csrf_token": {csrf},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, idempotent.Code)
	assert.Contains(t, idempotent.Header().Get("Location"), "is+now+your+current+reading")

	current, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	changed := readingTestRequest(t, h, "/reading/books/fixture-route-match/switch", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {fixtures.BookID}, "expected_current_snapshot_id": {current.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, changed.Code)
	assert.Contains(t, changed.Header().Get("Location"), "is+now+your+current+reading")
	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, "fixture-route-match", currentReading.BookID)

	current, err = store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	cleared := readingTestRequest(t, h, "/reading/end", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {"fixture-route-match"}, "expected_current_snapshot_id": {current.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, cleared.Code)
	clearedAgain := readingTestRequest(t, h, "/reading/end", url.Values{"csrf_token": {csrf}, "expected_current_book_id": {"fixture-route-match"}, "expected_current_snapshot_id": {current.SnapshotID}}, cookies)
	assert.Equal(t, http.StatusSeeOther, clearedAgain.Code)
	assert.NotContains(t, clearedAgain.Header().Get("Location"), "error=")
}

func TestReadingStartLeavesOptionalDeckPreparationUnsubmitted(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	store.SetGoalSnapshotVocabulary("fixture-goal-de-fixture-route-match", []domain.DeckPreparationVocabulary{{OwnerID: fixtures.OwnerID, Language: "de", CanonicalLemma: "snapshot-word", UPOS: "NOUN"}})

	current, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	cleared := readingTestRequest(t, h, "/reading/end", url.Values{"csrf_token": {csrf}, "expected_current_book_id": {fixtures.BookID}, "expected_current_snapshot_id": {current.SnapshotID}}, cookies)
	require.Equal(t, http.StatusSeeOther, cleared.Code)
	chosen := readingTestRequest(t, h, "/reading/books/fixture-route-match/start", url.Values{
		"csrf_token": {csrf},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, chosen.Code)

	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	_, err = store.GetDeckPreparationForSnapshot(context.Background(), fixtures.OwnerID, currentReading.SnapshotID)
	require.ErrorIs(t, err, persistence.ErrNotFound, "starting Reading does not auto-submit optional deck preparation")
	page := performReadingRequest(t, h, http.MethodGet, "/reading", nil, cookies, false)
	assert.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), `action="/reading/books/fixture-route-match/deck/retry"`, "the current reading offers explicit preparation from its frozen snapshot")
}

func TestReadingPageScopesCurrentReadingToActiveLanguage(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.NotContains(t, body, "Reading Journey")
	assert.NotContains(t, body, "Primary Goal")
	for _, want := range []string{
		"Italiano",
		"0 lemmas are set aside from vocabulary selection while you read this Book.",
	} {
		assert.True(t, strings.Contains(body, want), "Italian Journey page missing %q: %s", want, body)
	}
	assert.True(t, strings.Contains(body, `id="journey-book-fixture-italian-goal"`) && !strings.Contains(body, `id="journey-book-fixture-book"`), "Italian Journey page exposed the German Goal: %s", body)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "de"))
	request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	body = response.Body.String()
	assert.True(t, strings.Contains(body, "Deutsch") && strings.Contains(body, `id="journey-book-fixture-book"`) && !strings.Contains(body, `id="journey-book-fixture-empty"`), "German Reading did not remain isolated after Italian move: %s", body)
	assert.Contains(t, body, "2 lemmas are set aside from vocabulary selection while you read this Book.")
}

func TestReadingPageShowsReservedCountWhenCurrentReadingArtifactIsUnavailable(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	handler := requireHandler(t, h)
	handler.services.PreparedDeck = unavailableCurrentReadingPreparedDeck{PreparedDeck: fixtures.PreparedDeck{Store: store}}

	page := performReadingRequest(t, h, http.MethodGet, "/reading", nil, cookies, false)
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, "2 lemmas are set aside from vocabulary selection while you read this Book.")
	assert.Contains(t, body, "Deck missing.")
	assert.Contains(t, body, "Prepare deck")
}

func TestCurrentReadingDeckRetryRequiresTheRenderedSnapshot(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)

	response := readingTestRequest(t, h, "/reading/books/"+currentReading.BookID+"/deck/retry", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_snapshot_id": {"stale-snapshot"},
	}, cookies)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	location, err := url.QueryUnescape(response.Header().Get("Location"))
	require.NoError(t, err)
	assert.Contains(t, location, currentReadingStaleMessage)
}

func TestCurrentReadingDeckRetryReusesTheCurrentSnapshotPreparation(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	preparedDeck := &existingCurrentReadingPreparedDeck{
		PreparedDeck: fixtures.PreparedDeck{Store: store},
		preparation: domain.DeckPreparation{
			ID: "current-goal-preparation", OwnerID: fixtures.OwnerID,
			SourceMaterialID: currentReading.SourceMaterialID, AnalysisRunID: currentReading.AnalysisRunID,
			SnapshotID: currentReading.SnapshotID, State: domain.DeckPreparationQueued,
		},
	}
	handler := requireHandler(t, h)
	handler.services.Analysis = fixtures.Analysis{}
	handler.services.PreparedDeck = preparedDeck

	response := readingTestRequest(t, h, "/reading/books/"+currentReading.BookID+"/deck/retry", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_snapshot_id": {currentReading.SnapshotID},
	}, cookies)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, 1, preparedDeck.retries)
	assert.Zero(t, preparedDeck.currentReadingSubmissions, "repeated recovery must not retire the current Book preparation")
}

func TestCurrentReadingDeckCancelRequiresAndUsesTheCurrentSnapshot(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	preparedDeck := &existingCurrentReadingPreparedDeck{
		PreparedDeck: fixtures.PreparedDeck{Store: store},
		preparation: domain.DeckPreparation{
			ID: "current-goal-preparation", OwnerID: fixtures.OwnerID,
			SourceMaterialID: currentReading.SourceMaterialID, AnalysisRunID: currentReading.AnalysisRunID,
			SnapshotID: currentReading.SnapshotID, State: domain.DeckPreparationPreparing,
		},
	}
	handler := requireHandler(t, h)
	handler.services.PreparedDeck = preparedDeck

	stale := readingTestRequest(t, h, "/reading/books/"+currentReading.BookID+"/deck/cancel", url.Values{
		"csrf_token": {csrf}, "expected_current_snapshot_id": {"stale-snapshot"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, stale.Code)
	assert.Zero(t, preparedDeck.cancellations)

	current := readingTestRequest(t, h, "/reading/books/"+currentReading.BookID+"/deck/cancel", url.Values{
		"csrf_token": {csrf}, "expected_current_snapshot_id": {currentReading.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, current.Code)
	assert.Equal(t, 1, preparedDeck.cancellations)
}

func TestReadingDeckSubmissionKeepsCurrentReadingPreparationLocal(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	preparedDeck := &existingCurrentReadingPreparedDeck{
		PreparedDeck: fixtures.PreparedDeck{Store: store},
		preparation: domain.DeckPreparation{
			ID: "current-goal-preparation", OwnerID: fixtures.OwnerID,
			SourceMaterialID: currentReading.SourceMaterialID, AnalysisRunID: currentReading.AnalysisRunID,
			SnapshotID: currentReading.SnapshotID, State: domain.DeckPreparationQueued,
		},
	}
	handler := requireHandler(t, h)
	handler.services.Analysis = fixtures.Analysis{}
	handler.services.PreparedDeck = preparedDeck

	response := readingTestRequest(t, h, "/reading/books/"+currentReading.BookID+"/deck/preparations", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_snapshot_id": {currentReading.SnapshotID},
		"external_translation_consent": {"on"},
	}, cookies)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, 1, preparedDeck.retries)
	assert.Equal(t, 1, preparedDeck.retries, "Goal retry must queue the same contextual preparation")
	assert.Zero(t, preparedDeck.genericSubmissions, "Goal preparation remains bound to its frozen snapshot")
}

func TestReadingPageShowsEmptyActiveLanguageReading(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	ctx := context.Background()
	books, err := store.ListMyBooksWithEvidence(ctx, fixtures.OwnerID)
	require.NoError(t, err)
	for _, book := range books {
		if book.Book.LanguageTag == "it" && book.Book.ID != fixtures.ItalianGoalBookID && book.Disposition == domain.BookDispositionToRead {
			require.NoError(t, store.SetBookDisposition(ctx, fixtures.OwnerID, book.Book.ID, domain.BookDispositionInbox))
		}
	}
	require.NoError(t, store.SetActiveStudyLanguage(ctx, fixtures.OwnerID, "it"))

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, `id="vocabulary-workflow"`, "the current Book's Browse belongs to Reading")
	for _, retired := range []string{"Other To Read", "No other To Read books", "provisional-journey"} {
		assert.NotContains(t, body, retired)
	}
	assert.False(t, strings.Contains(body, `id="journey-book-fixture-empty"`) || strings.Contains(body, `id="journey-book-fixture-edge-content"`), "empty Italian Journey page exposed a member: %s", body)
}

func performReadingRequest(t *testing.T, h http.Handler, method, path string, form url.Values, cookies []*http.Cookie, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if form == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(form.Encode())
	}
	request := httptest.NewRequestWithContext(t.Context(), method, path, body)
	if form != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if htmx {
		request.Header.Set("Hx-Request-Type", "partial")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

func TestCurrentReadingFinishRendersTruthfulOutcomeAndIsIdempotent(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	currentReadingBefore, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	knownBefore, err := store.ListKnownVocabulary(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	finished := readingTestRequest(t, h, "/reading/finish", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {fixtures.BookID}, "expected_current_snapshot_id": {currentReadingBefore.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusOK, finished.Code)
	for _, want := range []string{
		"Reading finished",
		"Vocabulary transition",
		"Vocabulary: 2 identities newly Known; 0 identities already Known",
		`href="/reading">Choose a To Read book</a>`,
	} {
		assert.True(t, strings.Contains(finished.Body.String(), want), "finish outcome missing %q: %s", want, finished.Body.String())
	}
	assert.NotContains(t, finished.Body.String(), "achievement")
	assert.NotContains(t, finished.Body.String(), "Where next?")
	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Empty(t, currentReading.BookID, "finished Goal=%+v", currentReading)
	disposition, err := store.GetBookDisposition(context.Background(), fixtures.OwnerID, fixtures.BookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, disposition)
	known, err := store.ListKnownVocabulary(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, 2, len(known)-len(knownBefore))
	repeated := readingTestRequest(t, h, "/reading/finish", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {fixtures.BookID}, "expected_current_snapshot_id": {currentReadingBefore.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusOK, repeated.Code)
	assert.True(t, strings.Contains(repeated.Body.String(), "Reading finished"), "idempotent finish body=%s", repeated.Body.String())
	assert.True(t, strings.Contains(repeated.Body.String(), "Der lange Weg nach Hause"), "idempotent finish lost Book title: %s", repeated.Body.String())
}

func TestCurrentReadingFinishRejectsStaleAndMissingCSRF(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	stale := readingTestRequest(t, h, "/reading/finish", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {"stale-book"}, "expected_current_snapshot_id": {currentReading.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, stale.Code)
	assert.True(t, strings.Contains(stale.Header().Get("Location"), "This+current+reading+changed"), "stale finish location=%q", stale.Header().Get("Location"))
	missingCSRF := readingTestRequest(t, h, "/reading/finish", url.Values{"expected_current_book_id": {fixtures.BookID}, "expected_current_snapshot_id": {currentReading.SnapshotID}}, cookies)
	assert.Equal(t, http.StatusForbidden, missingCSRF.Code)
	assert.Equal(t, fixtures.BookID, currentReading.BookID, "rejected finish changed Goal=%+v", currentReading)
}

func TestCurrentReadingFinishOutcomeShowsStructuredVocabularyCounts(t *testing.T) {
	residual := currentReadingFinishView{BookTitle: "Reading-only book", GraduatedVocabularyCount: 2, AlreadyKnownCount: 0}
	var output bytes.Buffer
	require.NoError(t, CurrentReadingFinish(residual).Render(context.Background(), &output))
	for _, want := range []string{"Vocabulary: 2 identities newly Known; 0 identities already Known", "Choose a To Read book"} {
		assert.True(t, strings.Contains(output.String(), want), "residual outcome missing %q: %s", want, output.String())
	}
	assert.NotContains(t, output.String(), "achievement")
}

func TestCurrentReadingFinishOutcomeExplainsEmptySnapshot(t *testing.T) {
	outcome := currentReadingFinishView{BookTitle: "Empty snapshot book"}
	var output bytes.Buffer
	require.NoError(t, CurrentReadingFinish(outcome).Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{
		"Reading finished",
		"Vocabulary: 0 identities newly Known; 0 identities already Known",
		"Choose a To Read book",
	} {
		assert.Contains(t, html, want)
	}
	assert.NotContains(t, html, `<article role="alert" class=`)
	assert.NotContains(t, html, "celebrat")
}

func TestCurrentReadingFinishEmptySnapshotThroughAuthenticatedHandler(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	current, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	cleared := readingTestRequest(t, h, "/reading/end", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {fixtures.BookID}, "expected_current_snapshot_id": {current.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, cleared.Code)
	chosen := readingTestRequest(t, h, "/reading/books/"+fixtures.BookID+"/start", url.Values{
		"csrf_token": {csrf},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, chosen.Code)
	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Zero(t, currentReading.SnapshotSize)

	finished := readingTestRequest(t, h, "/reading/finish", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {fixtures.BookID}, "expected_current_snapshot_id": {currentReading.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusOK, finished.Code)
	for _, want := range []string{
		"Reading finished",
		"Vocabulary: 0 identities newly Known; 0 identities already Known",
		"Choose a To Read book",
	} {
		assert.Contains(t, finished.Body.String(), want)
	}
	assert.NotContains(t, finished.Body.String(), `<article role="alert" class=`)
	assert.NotContains(t, finished.Body.String(), "celebrat")
}

func TestEndCurrentReadingRejectsStaleMissingAndCSRFThenEndsExactCommitment(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)

	for name, form := range map[string]url.Values{
		"stale book":       {"csrf_token": {csrf}, "expected_current_book_id": {"stale-book"}, "expected_current_snapshot_id": {currentReading.SnapshotID}},
		"stale snapshot":   {"csrf_token": {csrf}, "expected_current_book_id": {currentReading.BookID}, "expected_current_snapshot_id": {"stale-snapshot"}},
		"missing snapshot": {"csrf_token": {csrf}, "expected_current_book_id": {currentReading.BookID}},
	} {
		rejected := readingTestRequest(t, h, "/reading/end", form, cookies)
		assert.Equal(t, http.StatusSeeOther, rejected.Code, name)
		assert.Contains(t, rejected.Header().Get("Location"), "error=", name)
	}
	missingCSRF := readingTestRequest(t, h, "/reading/end", url.Values{"expected_current_book_id": {currentReading.BookID}, "expected_current_snapshot_id": {currentReading.SnapshotID}}, cookies)
	assert.Equal(t, http.StatusForbidden, missingCSRF.Code)
	current, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, currentReading.SnapshotID, current.SnapshotID, "rejected End leaves the commitment")

	exact := url.Values{"csrf_token": {csrf}, "expected_current_book_id": {currentReading.BookID}, "expected_current_snapshot_id": {currentReading.SnapshotID}}
	ended := readingTestRequest(t, h, "/reading/end", exact, cookies)
	assert.Equal(t, http.StatusSeeOther, ended.Code)
	assert.NotContains(t, ended.Header().Get("Location"), "error=")
	current, err = store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive())
	replay := readingTestRequest(t, h, "/reading/end", exact, cookies)
	assert.NotContains(t, replay.Header().Get("Location"), "error=", "verified replay is harmless")

	for _, path := range []string{"/reading/stop", "/reading/set-aside"} {
		retired := readingTestRequest(t, h, path, exact, cookies)
		assert.GreaterOrEqual(t, retired.Code, http.StatusBadRequest, path)
	}
}
