package webapp

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strconv"
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

type journeyForecastFailureInsights struct {
	fixtures.Insights
}

func (journeyForecastFailureInsights) JourneyForecast(context.Context, string, string) (domain.JourneyForecast, error) {
	return domain.JourneyForecast{}, errors.New("forecast provider unavailable")
}

type unavailableGoalPreparedDeck struct {
	fixtures.PreparedDeck
}

func (unavailableGoalPreparedDeck) GetForGoalSnapshot(context.Context, string, string) (domain.DeckPreparation, error) {
	return domain.DeckPreparation{}, persistence.ErrNotFound
}

type existingGoalPreparedDeck struct {
	fixtures.PreparedDeck
	preparation        domain.DeckPreparation
	retries            int
	goalSubmissions    int
	genericSubmissions int
	retryConsent       bool
	cancellations      int
}

func (p *existingGoalPreparedDeck) GetForGoalSnapshot(context.Context, string, string) (domain.DeckPreparation, error) {
	return p.preparation, nil
}

func (p *existingGoalPreparedDeck) Retry(_ context.Context, _, _ string, consent bool) (prepareddeck.Handle, error) {
	p.retries++
	p.retryConsent = consent
	return prepareddeck.Handle{Preparation: p.preparation, JobID: 9}, nil
}

func (p *existingGoalPreparedDeck) SubmitForGoal(context.Context, string, string, string) (prepareddeck.Handle, error) {
	p.goalSubmissions++
	return prepareddeck.Handle{Preparation: p.preparation, JobID: 9}, nil
}

func (p *existingGoalPreparedDeck) Submit(context.Context, string, string, bool) (prepareddeck.Handle, error) {
	p.genericSubmissions++
	return prepareddeck.Handle{Preparation: p.preparation, JobID: 9}, nil
}

func (p *existingGoalPreparedDeck) Cancel(context.Context, string, string) (domain.DeckPreparation, error) {
	p.cancellations++
	return p.preparation, nil
}

func renderGoalSection(t *testing.T, goal *journeyBookView, message, pageError, focusBookID string) string {
	t.Helper()
	var output bytes.Buffer
	require.NoError(t, GoalSection(goal, "csrf-token", message, pageError, focusBookID).Render(context.Background(), &output))
	return output.String()
}

func TestGoalSectionRendersEmptyStateAndLiveFeedback(t *testing.T) {
	html := renderGoalSection(t, nil, "current reading cleared.", "", "")
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

	html = renderGoalSection(t, nil, "", "This current reading changed since this page was loaded.", "")
	assert.True(t, strings.Contains(html, `role="alert"`) && strings.Contains(html, "This current reading changed since this page was loaded"), "Goal section did not render an accessible error: %s", html)
}

func TestGoalSectionRendersReadingOnlyAndResidualStates(t *testing.T) {
	unassessed := testJourneyBook("reading-only", "Reading-only book", "ready")
	unassessed.GoalReadingOnly = true
	unassessed.GoalUnassessed = true
	readingOnlyHTML := renderGoalSection(t, &unassessed, "", "", "reading-only")
	for _, want := range []string{"Reading only", "No analysis or deck-eligible vocabulary exists yet", "Reading directly is the current path"} {
		assert.True(t, strings.Contains(readingOnlyHTML, want), "unassessed Goal missing %q: %s", want, readingOnlyHTML)
	}

	assessed := testJourneyBook("assessed", "Assessed without deck", "analyzed")
	assessed.GoalReadingOnly = true
	assessed.GoalUnassessed = false
	assessedHTML := renderGoalSection(t, &assessed, "", "", "assessed")
	assert.True(t, strings.Contains(assessedHTML, "Analysis evidence exists, but this Book has no deck-eligible vocabulary"), "assessed reading-only copy missing: %s", assessedHTML)

	goal := testJourneyBook("goal", "Goal book", "analyzed")
	goalHTML := renderGoalSection(t, &goal, "", "", "goal")
	assert.True(t, strings.Contains(goalHTML, "Clear Current reading"), "Goal omitted clear action: %s", goalHTML)
}

func TestJourneyGoalControlsUseExpectedStateAndStaySeparated(t *testing.T) {
	goal := testJourneyBook("goal", "Goal book", "analyzed")
	goal.GoalSnapshotID = "goal-snapshot"
	first := testJourneyBook("first", "First book", "analyzed")
	second := testJourneyBook("second", "Second book", "ready")
	first.Book.Source.MediaType = "application/epub+zip"
	first.Book.Source.ContentRevisionID = "first-revision"
	first.Book.Source.ContentSnapshotID = "first-snapshot"
	first.Book.AnalysisState = "completed"
	first.Book.AnalysisRunID = "first-run"
	first.Book.CorpusID = "first-corpus"
	first.CanChooseGoal = true
	second.GoalEligibilityReason = "This book needs a successfully completed current analysis before it can become a current reading."
	html := renderJourney(t, journeyPageView{Goal: &goal, Provisional: []journeyBookView{first, second}}, "", "")
	assert.Equal(t, 1, strings.Count(html, `action="/goal/books/first"`), "expected a choose form only for the eligible provisional card: %s", html)
	assert.False(t, strings.Contains(html, `action="/goal/books/second"`), "expected a choose form only for the eligible provisional card: %s", html)
	assert.True(t, strings.Contains(html, `name="expected_goal_book_id" value="goal"`), "provisional choose forms did not carry the current Book: %s", html)
	assert.True(t, strings.Contains(html, `name="expected_goal_snapshot_id" value="goal-snapshot"`), "finish form did not carry the current Book snapshot: %s", html)
	goalStart := strings.Index(html, `id="journey-book-goal"`)
	goalEnd := strings.Index(html[goalStart:], "</article>")
	goalCard := html[goalStart : goalStart+goalEnd]
	assert.False(t, strings.Contains(goalCard, `action="/goal/books/`), "current Book card exposed a choose control: %s", goalCard)
}

func TestMyBooksGoalControlsAndJourneyLink(t *testing.T) {
	books := []domain.MyBook{
		{Book: domain.Book{ID: "goal-book", OwnerID: "owner", Title: "Current goal"}, JourneyGoal: true},
		{Book: domain.Book{ID: "other-book", OwnerID: "owner", Title: "Other book"}},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", "goal-book", true, MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	goalStart := strings.Index(html, `aria-labelledby="book-title-goal-book"`)
	otherStart := strings.Index(html, `aria-labelledby="book-title-other-book"`)
	require.True(t, goalStart >= 0 && otherStart >= 0, "book cards missing: %s", html)
	goalCard := html[goalStart:otherStart]
	otherCard := html[otherStart:]
	for _, want := range []string{"To Read", "View in Reading", "/reading#journey-book-goal-book"} {
		assert.True(t, strings.Contains(goalCard, want), "current Book card missing %q: %s", want, goalCard)
	}
	assert.False(t, strings.Contains(goalCard, "current reading"), "My Books exposed Goal state: %s", goalCard)
	assert.False(t, strings.Contains(otherCard, "Start reading") || strings.Contains(otherCard, `action="/goal/books/other-book"`), "My Books exposed a choose form: %s", otherCard)
}

func TestVisibleJourneyEntriesSkipAnchoredGoal(t *testing.T) {
	entries := []domain.ReadingJourneyEntry{
		{BookID: "first", Position: 1},
		{BookID: "goal", Position: 2},
		{BookID: "second", Position: 3},
	}
	visible := visibleJourneyEntries(entries, "goal")
	require.Len(t, visible, 2)
	assert.Equal(t, "first", visible[0].BookID)
	assert.Equal(t, "second", visible[1].BookID)
	assert.True(t, entries[1].BookID == "goal" && entries[0].BookID == "first" && entries[2].BookID == "second", "visible-order helper mutated source entries=%+v", entries)
}

func goalFixtureSession(t *testing.T) (http.Handler, []*http.Cookie, string, *fixtures.Store) {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "goal-unit-test-secret-0123456789")
	store := fixtures.NewStore()
	authService := auth.New(fixtures.NewAuthStore(), time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), PreparedDeck: fixtures.PreparedDeck{Store: store}, SessionLifetime: time.Hour})
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

func goalRequest(t *testing.T, h http.Handler, path string, form url.Values, cookies []*http.Cookie) *httptest.ResponseRecorder {
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

func TestGoalMutationRoutesAreIdempotent(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	idempotent := goalRequest(t, h, "/goal/books/"+fixtures.BookID, url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, idempotent.Code)
	assert.True(t, strings.Contains(idempotent.Header().Get("Location"), "already+your+current+reading"), "idempotent choose location=%q", idempotent.Header().Get("Location"))

	changed := goalRequest(t, h, "/goal/books/fixture-route-match", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, changed.Code)
	assert.True(t, strings.Contains(changed.Header().Get("Location"), "is+your+current+reading"), "change location=%q", changed.Header().Get("Location"))
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, "fixture-route-match", goal.BookID)

	cleared := goalRequest(t, h, "/goal/clear", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {"fixture-route-match"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, cleared.Code)
	clearedAgain := goalRequest(t, h, "/goal/clear", url.Values{"csrf_token": {csrf}, "expected_goal_book_id": {""}}, cookies)
	assert.Equal(t, http.StatusSeeOther, clearedAgain.Code)
	assert.True(t, strings.Contains(clearedAgain.Header().Get("Location"), "No+current+reading+was+set"), "idempotent clear location=%q", clearedAgain.Header().Get("Location"))
}

func TestGoalSelectionBindsAutomaticPreparationToFrozenSnapshot(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	store.SetGoalSnapshotVocabulary("fixture-goal-de-fixture-route-match", []domain.DeckPreparationVocabulary{{OwnerID: fixtures.OwnerID, Language: "de", CanonicalLemma: "snapshot-word", UPOS: "NOUN"}})

	cleared := goalRequest(t, h, "/goal/clear", url.Values{"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID}}, cookies)
	require.Equal(t, http.StatusSeeOther, cleared.Code)
	chosen := goalRequest(t, h, "/goal/books/fixture-route-match", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {""}, "external_translation_consent": {"on"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, chosen.Code)

	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	preparation, err := store.GetDeckPreparationForGoalSnapshot(context.Background(), fixtures.OwnerID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, goal.SnapshotID, preparation.GoalSnapshotID)
	assert.Equal(t, goal.AnalysisRunID, preparation.AnalysisRunID)
}

func TestJourneyPageScopesHeadingGoalAndActionsToActiveLanguage(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
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
		"Reading in Italian",
		`id="journey-book-fixture-empty"`,
		`name="expected_revision" value="1"`,
		`action="/reading/entries/fixture-empty/move-later"`,
		"Reserved vocabulary</strong>: <span class=\"numeric\">0</span> frozen identities.",
	} {
		assert.True(t, strings.Contains(body, want), "Italian Journey page missing %q: %s", want, body)
	}
	assert.True(t, strings.Contains(body, `id="journey-book-fixture-italian-goal"`) && !strings.Contains(body, `id="journey-book-fixture-book"`), "Italian Journey page exposed the German Goal: %s", body)
	cleared := goalRequest(t, h, "/goal/clear", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.ItalianGoalBookID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, cleared.Code)
	chosen := goalRequest(t, h, "/goal/books/"+fixtures.ItalianGoalBookID, url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {""},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, chosen.Code)
	assert.True(t, strings.Contains(chosen.Header().Get("Location"), "is+your+current+reading"), "choose Italian Goal location=%q", chosen.Header().Get("Location"))
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, fixtures.BookID, goal.BookID)

	moved := goalRequest(t, h, "/reading/entries/fixture-edge-content/move-earlier", url.Values{
		"csrf_token": {csrf}, "expected_revision": {"1"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, moved.Code)
	italianJourney, err := store.GetReadingJourney(context.Background(), fixtures.OwnerID, "it")
	require.NoError(t, err)
	assert.Equal(t, "fixture-edge-content", italianJourney.Entries[0].BookID, "Italian Journey after move=%+v", italianJourney.Entries)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "de"))
	request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	body = response.Body.String()
	assert.True(t, strings.Contains(body, "Reading in German") && strings.Contains(body, `id="journey-book-fixture-book"`) && !strings.Contains(body, `id="journey-book-fixture-empty"`), "German Journey did not remain isolated after Italian move: %s", body)
	assert.Contains(t, body, "Reserved vocabulary</strong>: <span class=\"numeric\">2</span> frozen identities.")
}

func TestJourneyPageShowsReservedCountWhenGoalArtifactIsUnavailable(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	handler := requireHandler(t, h)
	handler.services.PreparedDeck = unavailableGoalPreparedDeck{PreparedDeck: fixtures.PreparedDeck{Store: store}}

	page := performJourneyRequest(t, h, http.MethodGet, "/reading", nil, cookies, false)
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, "Reserved vocabulary</strong>: <span class=\"numeric\">2</span> frozen identities.")
	assert.Contains(t, body, "Deck state: Missing")
	assert.Contains(t, body, "Prepare deck")
}

func TestGoalDeckRetryRequiresTheRenderedSnapshot(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)

	response := goalRequest(t, h, "/goal/books/"+goal.BookID+"/deck/retry", url.Values{
		"csrf_token":                {csrf},
		"expected_goal_snapshot_id": {"stale-snapshot"},
	}, cookies)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	location, err := url.QueryUnescape(response.Header().Get("Location"))
	require.NoError(t, err)
	assert.Contains(t, location, goalStaleMessage)
}

func TestGoalDeckRetryReusesTheCurrentSnapshotPreparation(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	preparedDeck := &existingGoalPreparedDeck{
		PreparedDeck: fixtures.PreparedDeck{Store: store},
		preparation: domain.DeckPreparation{
			ID: "current-goal-preparation", OwnerID: fixtures.OwnerID,
			SourceMaterialID: goal.SourceMaterialID, AnalysisRunID: goal.AnalysisRunID,
			GoalSnapshotID: goal.SnapshotID, State: domain.DeckPreparationQueued,
		},
	}
	handler := requireHandler(t, h)
	handler.services.Analysis = fixtures.Analysis{}
	handler.services.PreparedDeck = preparedDeck

	response := goalRequest(t, h, "/goal/books/"+goal.BookID+"/deck/retry", url.Values{
		"csrf_token":                {csrf},
		"expected_goal_snapshot_id": {goal.SnapshotID},
	}, cookies)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, 1, preparedDeck.retries)
	assert.Zero(t, preparedDeck.goalSubmissions, "repeated recovery must not retire the current Book preparation")
}

func TestGoalDeckCancelRequiresAndUsesTheCurrentSnapshot(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	preparedDeck := &existingGoalPreparedDeck{
		PreparedDeck: fixtures.PreparedDeck{Store: store},
		preparation: domain.DeckPreparation{
			ID: "current-goal-preparation", OwnerID: fixtures.OwnerID,
			SourceMaterialID: goal.SourceMaterialID, AnalysisRunID: goal.AnalysisRunID,
			GoalSnapshotID: goal.SnapshotID, State: domain.DeckPreparationPreparing,
		},
	}
	handler := requireHandler(t, h)
	handler.services.PreparedDeck = preparedDeck

	stale := goalRequest(t, h, "/goal/books/"+goal.BookID+"/deck/cancel", url.Values{
		"csrf_token": {csrf}, "expected_goal_snapshot_id": {"stale-snapshot"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, stale.Code)
	assert.Zero(t, preparedDeck.cancellations)

	current := goalRequest(t, h, "/goal/books/"+goal.BookID+"/deck/cancel", url.Values{
		"csrf_token": {csrf}, "expected_goal_snapshot_id": {goal.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, current.Code)
	assert.Equal(t, 1, preparedDeck.cancellations)
}

func TestJourneyDeckSubmissionKeepsGoalPreparationLocal(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	preparedDeck := &existingGoalPreparedDeck{
		PreparedDeck: fixtures.PreparedDeck{Store: store},
		preparation: domain.DeckPreparation{
			ID: "current-goal-preparation", OwnerID: fixtures.OwnerID,
			SourceMaterialID: goal.SourceMaterialID, AnalysisRunID: goal.AnalysisRunID,
			GoalSnapshotID: goal.SnapshotID, State: domain.DeckPreparationQueued,
		},
	}
	handler := requireHandler(t, h)
	handler.services.Analysis = fixtures.Analysis{}
	handler.services.PreparedDeck = preparedDeck

	response := goalRequest(t, h, "/reading/books/"+goal.BookID+"/deck/preparations", url.Values{
		"csrf_token":                   {csrf},
		"external_translation_consent": {"on"},
	}, cookies)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, 1, preparedDeck.retries)
	assert.False(t, preparedDeck.retryConsent)
	assert.Zero(t, preparedDeck.genericSubmissions, "Goal preparation must not use the consent-bearing generic path")
}

func TestJourneyPageShowsEmptyActiveLanguageJourney(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	ctx := context.Background()
	journey, err := store.GetReadingJourney(ctx, fixtures.OwnerID, "it")
	require.NoError(t, err)
	for _, v := range slices.Backward(journey.Entries) {
		_, err = store.RemoveFromReadingJourney(ctx, fixtures.OwnerID, "it", v.BookID, journey.Revision)
		require.NoError(t, err)
		journey, err = store.GetReadingJourney(ctx, fixtures.OwnerID, "it")
		require.NoError(t, err)
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
	for _, want := range []string{"Choose your next book in Italian", "No To Read books yet", "Browse My Books"} {
		assert.True(t, strings.Contains(body, want), "empty Italian Reading page missing %q: %s", want, body)
	}
	assert.False(t, strings.Contains(body, `id="journey-book-fixture-empty"`) || strings.Contains(body, `id="journey-book-fixture-edge-content"`), "empty Italian Journey page exposed a member: %s", body)
}

func TestJourneyReorderRecalculatesForecastWithoutChangingLearnerState(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	handler := requireHandler(t, h)
	handler.services.AnalysisInsights = fixtures.Insights{JourneyStore: store}
	ctx := context.Background()

	knownBefore, err := store.ListKnownVocabulary(ctx, fixtures.OwnerID, "de")
	require.NoError(t, err)
	reservedBefore, err := store.ListReservedVocabulary(ctx, fixtures.OwnerID, "de")
	require.NoError(t, err)
	goalBefore, err := store.GetPrimaryGoal(ctx, fixtures.OwnerID, "de")
	require.NoError(t, err)

	page := performJourneyRequest(t, h, http.MethodGet, "/reading", nil, cookies, false)
	require.Equal(t, http.StatusOK, page.Code)
	revision := journeyHiddenInputValue(t, page.Body.String(), "expected_revision")
	before := page.Body.String()

	moved := performJourneyRequest(t, h, http.MethodPost, "/reading/entries/fixture-route-differs/move-earlier", url.Values{
		"csrf_token": {csrf}, "expected_revision": {revision},
	}, cookies, false)
	require.Equal(t, http.StatusSeeOther, moved.Code)
	assert.Contains(t, moved.Header().Get("Location"), "Coverage+forecast+recalculated")

	page = performJourneyRequest(t, h, http.MethodGet, "/reading", nil, cookies, false)
	require.Equal(t, http.StatusOK, page.Code)
	after := page.Body.String()
	assertJourneyOrder(t, after, "fixture-route-differs", "fixture-route-match")
	assert.Contains(t, after, "Current coverage")
	assert.Contains(t, after, "After current reading")
	assert.Contains(t, after, "On arrival")
	assert.NotEqual(t, journeyCardForecast(t, before, "fixture-route-match"), journeyCardForecast(t, after, "fixture-route-match"), "downstream forecast did not change after reorder")

	journey, err := store.GetReadingJourney(ctx, fixtures.OwnerID, "de")
	require.NoError(t, err)
	moveRevision := strconv.FormatInt(journey.Revision, 10)
	htmx := performJourneyRequest(t, h, http.MethodPost, "/reading/entries/fixture-route-differs/move-later", url.Values{
		"csrf_token": {csrf}, "expected_revision": {moveRevision},
	}, cookies, true)
	require.Equal(t, http.StatusOK, htmx.Code)
	assert.Contains(t, htmx.Body.String(), "Coverage forecast recalculated for the saved order.")
	assertJourneyOrder(t, htmx.Body.String(), "fixture-route-match", "fixture-route-differs")

	knownAfter, err := store.ListKnownVocabulary(ctx, fixtures.OwnerID, "de")
	require.NoError(t, err)
	reservedAfter, err := store.ListReservedVocabulary(ctx, fixtures.OwnerID, "de")
	require.NoError(t, err)
	goalAfter, err := store.GetPrimaryGoal(ctx, fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, knownBefore, knownAfter)
	assert.Equal(t, reservedBefore, reservedAfter)
	assert.Equal(t, goalBefore, goalAfter)
}

func TestJourneyReorderForecastFailureDoesNotUndoSavedOrder(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	handler := requireHandler(t, h)
	handler.services.AnalysisInsights = journeyForecastFailureInsights{Insights: fixtures.Insights{JourneyStore: store}}
	page := performJourneyRequest(t, h, http.MethodGet, "/reading", nil, cookies, false)
	require.Equal(t, http.StatusOK, page.Code)
	revision := journeyHiddenInputValue(t, page.Body.String(), "expected_revision")

	moved := performJourneyRequest(t, h, http.MethodPost, "/reading/entries/fixture-route-differs/move-earlier", url.Values{
		"csrf_token": {csrf}, "expected_revision": {revision},
	}, cookies, true)
	require.Equal(t, http.StatusOK, moved.Code)
	assert.Contains(t, moved.Body.String(), "saved order remains in place")
	assert.Contains(t, moved.Body.String(), `href="/reading">Retry forecast</a>`)
	assert.Contains(t, moved.Body.String(), "On arrival")
	assert.Contains(t, moved.Body.String(), "Unavailable")
	assertJourneyOrder(t, moved.Body.String(), "fixture-route-differs", "fixture-route-match")
}

func performJourneyRequest(t *testing.T, h http.Handler, method, path string, form url.Values, cookies []*http.Cookie, htmx bool) *httptest.ResponseRecorder {
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
		request.Header.Set("Hx-Request", "true")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

func assertJourneyOrder(t *testing.T, html string, first, second string) {
	t.Helper()
	firstIndex := strings.Index(html, `id="journey-book-`+first+`" tabindex`)
	secondIndex := strings.Index(html, `id="journey-book-`+second+`" tabindex`)
	require.GreaterOrEqual(t, firstIndex, 0, "first Journey book %q missing: %s", first, html)
	require.GreaterOrEqual(t, secondIndex, 0, "second Journey book %q missing: %s", second, html)
	assert.Less(t, firstIndex, secondIndex, "Journey order did not place %q before %q: %s", first, second, html)
}

func journeyHiddenInputValue(t *testing.T, html, name string) string {
	t.Helper()
	match := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]+)"`).FindStringSubmatch(html)
	require.Len(t, match, 2, "hidden input %q missing: %s", name, html)
	return match[1]
}

func journeyCardForecast(t *testing.T, html, bookID string) string {
	t.Helper()
	start := strings.Index(html, `id="journey-book-`+bookID+`"`)
	require.GreaterOrEqual(t, start, 0, "Journey book %q missing: %s", bookID, html)
	end := strings.Index(html[start:], "</article>")
	require.GreaterOrEqual(t, end, 0, "Journey book %q did not close: %s", bookID, html[start:])
	card := html[start : start+end]
	forecastStart := strings.Index(card, `aria-label="Reading coverage forecast"`)
	require.GreaterOrEqual(t, forecastStart, 0, "Journey book %q has no forecast: %s", bookID, card)
	forecastEnd := strings.Index(card[forecastStart:], "</section>")
	require.GreaterOrEqual(t, forecastEnd, 0, "Journey book %q forecast did not close: %s", bookID, card[forecastStart:])
	return card[forecastStart : forecastStart+forecastEnd]
}

func TestPrimaryGoalFinishRendersTruthfulOutcomeAndIsIdempotent(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	goalBefore, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	finished := goalRequest(t, h, "/goal/finish", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID}, "expected_goal_snapshot_id": {goalBefore.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusOK, finished.Code)
	for _, want := range []string{
		"Reading finished",
		"Vocabulary transition",
		"Vocabulary: 2 identities newly Known; 0 identities already Known",
		`href="/reading">Choose what to read next</a>`,
	} {
		assert.True(t, strings.Contains(finished.Body.String(), want), "finish outcome missing %q: %s", want, finished.Body.String())
	}
	assert.NotContains(t, finished.Body.String(), "achievement")
	assert.NotContains(t, finished.Body.String(), "Where next?")
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Empty(t, goal.BookID, "finished Goal=%+v", goal)
	journey, err := store.GetReadingJourney(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	for _, entry := range journey.Entries {
		assert.NotEqual(t, fixtures.BookID, entry.BookID, "finished Book remained in Journey")
	}
	known, err := store.ListKnownVocabulary(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	graduated := 0
	for _, item := range known {
		if item.Provenance == "Accepted on Primary Goal completion" {
			graduated++
		}
	}
	assert.Equal(t, 2, graduated)
	repeated := goalRequest(t, h, "/goal/finish", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID}, "expected_goal_snapshot_id": {goalBefore.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusOK, repeated.Code)
	assert.True(t, strings.Contains(repeated.Body.String(), "Reading finished"), "idempotent finish body=%s", repeated.Body.String())
	assert.True(t, strings.Contains(repeated.Body.String(), "Der lange Weg nach Hause"), "idempotent finish lost Book title: %s", repeated.Body.String())
}

func TestPrimaryGoalFinishRejectsStaleAndMissingCSRF(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	stale := goalRequest(t, h, "/goal/finish", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {"stale-book"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, stale.Code)
	assert.True(t, strings.Contains(stale.Header().Get("Location"), "This+current+reading+changed"), "stale finish location=%q", stale.Header().Get("Location"))
	missingCSRF := goalRequest(t, h, "/goal/finish", url.Values{"expected_goal_book_id": {fixtures.BookID}}, cookies)
	assert.Equal(t, http.StatusForbidden, missingCSRF.Code)
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, fixtures.BookID, goal.BookID, "rejected finish changed Goal=%+v", goal)
}

func TestPrimaryGoalFinishOutcomeShowsStructuredVocabularyCounts(t *testing.T) {
	residual := primaryGoalFinishView{BookTitle: "Reading-only book", GraduatedVocabularyCount: 2, AlreadyKnownCount: 0}
	var output bytes.Buffer
	require.NoError(t, PrimaryGoalFinish(residual).Render(context.Background(), &output))
	for _, want := range []string{"Vocabulary: 2 identities newly Known; 0 identities already Known", "Choose what to read next"} {
		assert.True(t, strings.Contains(output.String(), want), "residual outcome missing %q: %s", want, output.String())
	}
	assert.NotContains(t, output.String(), "achievement")
}

func TestPrimaryGoalFinishOutcomeExplainsEmptySnapshot(t *testing.T) {
	outcome := primaryGoalFinishView{BookTitle: "Empty snapshot book"}
	var output bytes.Buffer
	require.NoError(t, PrimaryGoalFinish(outcome).Render(context.Background(), &output))
	html := output.String()
	for _, want := range []string{
		"Reading finished",
		"Vocabulary: 0 identities newly Known; 0 identities already Known",
		"Choose what to read next",
	} {
		assert.Contains(t, html, want)
	}
	assert.NotContains(t, html, `<article role="alert" class=`)
	assert.NotContains(t, html, "celebrat")
}

func TestPrimaryGoalFinishEmptySnapshotThroughAuthenticatedHandler(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	cleared := goalRequest(t, h, "/goal/clear", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, cleared.Code)
	chosen := goalRequest(t, h, "/goal/books/"+fixtures.BookID, url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {""},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, chosen.Code)
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Zero(t, goal.SnapshotSize)

	finished := goalRequest(t, h, "/goal/finish", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID}, "expected_goal_snapshot_id": {goal.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusOK, finished.Code)
	for _, want := range []string{
		"Reading finished",
		"Vocabulary: 0 identities newly Known; 0 identities already Known",
		"Choose what to read next",
	} {
		assert.Contains(t, finished.Body.String(), want)
	}
	assert.NotContains(t, finished.Body.String(), `<article role="alert" class=`)
	assert.NotContains(t, finished.Body.String(), "celebrat")
}
