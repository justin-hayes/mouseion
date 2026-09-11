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
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderGoalSection(t *testing.T, goal *journeyBookView, message, pageError, focusBookID string) string {
	t.Helper()
	var output bytes.Buffer
	require.NoError(t, GoalSection(goal, "csrf-token", message, pageError, focusBookID).Render(context.Background(), &output))
	return output.String()
}

func TestGoalSectionRendersEmptyStateAndLiveFeedback(t *testing.T) {
	html := renderGoalSection(t, nil, "Primary Goal cleared.", "", "")
	for _, want := range []string{
		`id="primary-goal-section"`,
		`id="goal-section-status"`,
		`role="status"`,
		"Primary Goal cleared.",
		"No Primary Goal yet",
		"analyzed member as your current commitment",
	} {
		assert.True(t, strings.Contains(html, want), "Goal section missing %q: %s", want, html)
	}

	html = renderGoalSection(t, nil, "", "This Primary Goal changed since this page was loaded.", "")
	assert.True(t, strings.Contains(html, `role="alert"`) && strings.Contains(html, "This Primary Goal changed since this page was loaded"), "Goal section did not render an accessible error: %s", html)
}

func TestGoalSectionRendersReadingOnlyAndResidualStates(t *testing.T) {
	unassessed := testJourneyBook("reading-only", "Reading-only book", "ready")
	unassessed.GoalReadingOnly = true
	unassessed.GoalUnassessed = true
	readingOnlyHTML := renderGoalSection(t, &unassessed, "", "", "reading-only")
	for _, want := range []string{"Reading-only Goal", "No analysis or deck exists yet", "stands on its own", "nothing is prepared automatically"} {
		assert.True(t, strings.Contains(readingOnlyHTML, want), "unassessed Goal missing %q: %s", want, readingOnlyHTML)
	}

	assessed := testJourneyBook("assessed", "Assessed without deck", "analyzed")
	assessed.GoalReadingOnly = true
	assessed.GoalUnassessed = false
	assessedHTML := renderGoalSection(t, &assessed, "", "", "assessed")
	assert.True(t, strings.Contains(assessedHTML, "Analysis evidence exists, but no deck has been prepared"), "assessed reading-only copy missing: %s", assessedHTML)

	goal := testJourneyBook("goal", "Goal book", "analyzed")
	goalHTML := renderGoalSection(t, &goal, "", "", "goal")
	assert.True(t, strings.Contains(goalHTML, "Clear Primary Goal"), "Goal omitted clear action: %s", goalHTML)
}

func TestJourneyGoalControlsUseExpectedStateAndStaySeparated(t *testing.T) {
	goal := testJourneyBook("goal", "Goal book", "analyzed")
	first := testJourneyBook("first", "First book", "analyzed")
	second := testJourneyBook("second", "Second book", "ready")
	first.Book.Source.MediaType = "application/epub+zip"
	first.Book.Source.ContentRevisionID = "first-revision"
	first.Book.Source.ContentSnapshotID = "first-snapshot"
	first.Book.AnalysisState = "completed"
	first.Book.AnalysisRunID = "first-run"
	first.Book.CorpusID = "first-corpus"
	first.CanChooseGoal = true
	second.GoalEligibilityReason = "This book needs a successfully completed current analysis before it can become a Primary Goal."
	html := renderJourney(t, journeyPageView{Goal: &goal, Provisional: []journeyBookView{first, second}}, "", "")
	assert.Equal(t, 1, strings.Count(html, `action="/goal/books/first"`), "expected a choose form only for the eligible provisional card: %s", html)
	assert.False(t, strings.Contains(html, `action="/goal/books/second"`), "expected a choose form only for the eligible provisional card: %s", html)
	assert.True(t, strings.Contains(html, `name="expected_goal_book_id" value="goal"`), "provisional choose forms did not carry the current Goal: %s", html)
	goalStart := strings.Index(html, `id="journey-book-goal"`)
	goalEnd := strings.Index(html[goalStart:], "</article>")
	goalCard := html[goalStart : goalStart+goalEnd]
	assert.False(t, strings.Contains(goalCard, `action="/goal/books/`), "Goal card exposed a choose control: %s", goalCard)
}

func TestMyBooksGoalControlsAndJourneyLink(t *testing.T) {
	books := []domain.MyBook{
		{Book: domain.Book{ID: "goal-book", OwnerID: "owner", Title: "Current goal"}},
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
	for _, want := range []string{"Current Primary Goal", "View Primary Goal in Reading Journey", "/journey#journey-book-goal-book"} {
		assert.True(t, strings.Contains(goalCard, want), "current Goal card missing %q: %s", want, goalCard)
	}
	assert.False(t, strings.Contains(goalCard, "Choose as Primary Goal"), "current Goal card exposed choose control: %s", goalCard)
	assert.False(t, strings.Contains(otherCard, "Choose as Primary Goal") || strings.Contains(otherCard, `action="/goal/books/other-book"`), "My Books exposed a choose form: %s", otherCard)
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
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: store, SessionLifetime: time.Hour})
	loginPage := httptest.NewRecorder()
	h.ServeHTTP(loginPage, httptest.NewRequest(http.MethodGet, "/login", nil))
	initialCSRFCookie := cookieByName(t, loginPage.Result().Cookies(), csrfCookie)
	token := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(loginPage.Body.String())[1]
	loginRequest := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{"csrf_token": {token}, "username": {fixtures.Username}, "password": {fixtures.Password}}.Encode()))
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
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
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
	assert.True(t, strings.Contains(idempotent.Header().Get("Location"), "already+your+Primary+Goal"), "idempotent choose location=%q", idempotent.Header().Get("Location"))

	changed := goalRequest(t, h, "/goal/books/fixture-route-match", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, changed.Code)
	assert.True(t, strings.Contains(changed.Header().Get("Location"), "is+your+Primary+Goal"), "change location=%q", changed.Header().Get("Location"))
	goal, _ := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	assert.Equal(t, "fixture-route-match", goal.BookID)

	cleared := goalRequest(t, h, "/goal/clear", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {"fixture-route-match"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, cleared.Code)
	clearedAgain := goalRequest(t, h, "/goal/clear", url.Values{"csrf_token": {csrf}, "expected_goal_book_id": {""}}, cookies)
	assert.Equal(t, http.StatusSeeOther, clearedAgain.Code)
	assert.True(t, strings.Contains(clearedAgain.Header().Get("Location"), "No+Primary+Goal+was+set"), "idempotent clear location=%q", clearedAgain.Header().Get("Location"))
}

func TestJourneyPageScopesHeadingGoalAndActionsToActiveLanguage(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))

	request := httptest.NewRequest(http.MethodGet, "/journey", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	for _, want := range []string{
		"Reading Journey in Italian",
		`id="journey-book-fixture-empty"`,
		`name="expected_revision" value="1"`,
		`action="/journey/entries/fixture-empty/move-later"`,
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
	assert.True(t, strings.Contains(chosen.Header().Get("Location"), "is+your+Primary+Goal"), "choose Italian Goal location=%q", chosen.Header().Get("Location"))
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, fixtures.BookID, goal.BookID)

	moved := goalRequest(t, h, "/journey/entries/fixture-edge-content/move-earlier", url.Values{
		"csrf_token": {csrf}, "expected_revision": {"1"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, moved.Code)
	italianJourney, err := store.GetReadingJourney(context.Background(), fixtures.OwnerID, "it")
	require.NoError(t, err)
	assert.Equal(t, "fixture-edge-content", italianJourney.Entries[0].BookID, "Italian Journey after move=%+v", italianJourney.Entries)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "de"))
	request = httptest.NewRequest(http.MethodGet, "/journey", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	body = response.Body.String()
	assert.True(t, strings.Contains(body, "Reading Journey in German") && strings.Contains(body, `id="journey-book-fixture-book"`) && !strings.Contains(body, `id="journey-book-fixture-empty"`), "German Journey did not remain isolated after Italian move: %s", body)
}

func TestJourneyPageShowsEmptyActiveLanguageJourney(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	ctx := context.Background()
	journey, err := store.GetReadingJourney(ctx, fixtures.OwnerID, "it")
	require.NoError(t, err)
	for i := len(journey.Entries) - 1; i >= 0; i-- {
		_, err = store.RemoveFromReadingJourney(ctx, fixtures.OwnerID, "it", journey.Entries[i].BookID, journey.Revision)
		require.NoError(t, err)
		journey, err = store.GetReadingJourney(ctx, fixtures.OwnerID, "it")
		require.NoError(t, err)
	}
	require.NoError(t, store.SetActiveStudyLanguage(ctx, fixtures.OwnerID, "it"))

	request := httptest.NewRequest(http.MethodGet, "/journey", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	for _, want := range []string{"Reading Journey in Italian", "No provisional books yet", "Browse My Books"} {
		assert.True(t, strings.Contains(body, want), "empty Italian Journey page missing %q: %s", want, body)
	}
	assert.False(t, strings.Contains(body, `id="journey-book-fixture-empty"`) || strings.Contains(body, `id="journey-book-fixture-edge-content"`), "empty Italian Journey page exposed a member: %s", body)
}

func TestPrimaryGoalFinishRendersTruthfulOutcomeAndIsIdempotent(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	finished := goalRequest(t, h, "/goal/finish", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID},
	}, cookies)
	assert.Equal(t, http.StatusOK, finished.Code)
	for _, want := range []string{
		"Reading finished",
		"Vocabulary transition",
		"No vocabulary was added to known vocabulary",
		"Reading Journey recalculated",
		"Where next?",
		"No new Primary Goal has been selected",
		"conditional-projected coverage",
		"Choose another book from Reading Journey",
	} {
		assert.True(t, strings.Contains(finished.Body.String(), want), "finish outcome missing %q: %s", want, finished.Body.String())
	}
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.NotNil(t, goal.ReadingFinishedAt, "finished Goal=%+v", goal)
	goal, err = store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, fixtures.BookID, goal.BookID)
	repeated := goalRequest(t, h, "/goal/finish", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID},
	}, cookies)
	assert.Equal(t, http.StatusOK, repeated.Code)
	assert.True(t, strings.Contains(repeated.Body.String(), "Reading finished"), "idempotent finish body=%s", repeated.Body.String())
}

func TestPrimaryGoalFinishRejectsStaleAndMissingCSRF(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	stale := goalRequest(t, h, "/goal/finish", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {"stale-book"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, stale.Code)
	assert.True(t, strings.Contains(stale.Header().Get("Location"), "This+Primary+Goal+changed"), "stale finish location=%q", stale.Header().Get("Location"))
	missingCSRF := goalRequest(t, h, "/goal/finish", url.Values{"expected_goal_book_id": {fixtures.BookID}}, cookies)
	assert.Equal(t, http.StatusForbidden, missingCSRF.Code)
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Nil(t, goal.ReadingFinishedAt, "rejected finish changed Goal=%+v", goal)
}

func TestPrimaryGoalFinishOutcomeShowsConditionalVocabularyEvidence(t *testing.T) {
	residual := primaryGoalFinishView{BookTitle: "Reading-only book", ResidualVocabulary: 2}
	var output bytes.Buffer
	require.NoError(t, PrimaryGoalFinish(residual, "csrf").Render(context.Background(), &output))
	for _, want := range []string{"No vocabulary was added to known vocabulary", "future effect is conditional", "No new Primary Goal has been selected"} {
		assert.True(t, strings.Contains(output.String(), want), "residual outcome missing %q: %s", want, output.String())
	}
}

func timePtr(value time.Time) *time.Time { return &value }
