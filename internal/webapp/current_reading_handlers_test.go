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
		"Choose Read next on a To Read book when you are ready to begin reading.",
	} {
		assert.Contains(t, html, want, "Goal section missing %q: %s", want, html)
	}

	html = renderCurrentReadingSection(t, nil, "", "This current reading changed since this page was loaded.", "")
	assert.True(t, strings.Contains(html, `role="alert"`) && strings.Contains(html, "This current reading changed since this page was loaded"), "Goal section did not render an accessible error: %s", html)
}

func TestCurrentReadingSectionRendersWithoutReadingOnlyOrReservedCopy(t *testing.T) {
	currentReading := testReadingBook("goal", "Goal book", testAnalyzed)
	currentReadingHTML := renderCurrentReadingSection(t, &currentReading, "", "", "goal")
	assert.NotContains(t, currentReadingHTML, "Clear Current reading", "retired Goal clear action remained visible: %s", currentReadingHTML)
	assert.NotContains(t, currentReadingHTML, "Reading only", "the reading-only state needs a vocabulary snapshot, which no longer exists")
	assert.NotContains(t, currentReadingHTML, "Reserved vocabulary", "nothing is reserved while reading")
	assert.NotContains(t, currentReadingHTML, "set aside from vocabulary selection")
}

func TestIsCurrentReadingControlsCarryNoCommitmentAndStaySeparated(t *testing.T) {
	currentReading := testReadingBook("goal", "Goal book", testAnalyzed)
	html := renderReading(t, readingPageView{CurrentReading: &currentReading})
	assert.NotContains(t, html, `action="/goal/books/`)
	assert.Contains(t, html, `action="/reading/finish"`)
	assert.Contains(t, html, `action="/reading/end"`)
	assert.NotContains(t, html, `expected_current_book_id`, "Finish and End carry no commitment token")
	assert.NotContains(t, html, `expected_current_snapshot_id`)
	currentReadingStart := strings.Index(html, `id="journey-book-goal"`)
	currentReadingEnd := strings.Index(html[currentReadingStart:], "</article>")
	currentReadingCard := html[currentReadingStart : currentReadingStart+currentReadingEnd]
	assert.NotContains(t, currentReadingCard, `action="/goal/books/`, "current Book card exposed a legacy Goal control: %s", currentReadingCard)
}

func TestMyBooksCurrentReadingControlsAndReadingLink(t *testing.T) {
	books := []domain.MyBook{
		{Book: domain.Book{ID: "goal-book", OwnerID: "owner", Title: "Current goal"}, IsCurrentReading: true},
		{Book: domain.Book{ID: "other-book", OwnerID: "owner", Title: "Other book"}},
	}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", MyBooksBrowseState{}).Render(context.Background(), &output))
	html := output.String()
	currentReadingStart := strings.Index(html, `aria-labelledby="book-title-goal-book"`)
	otherStart := strings.Index(html, `aria-labelledby="book-title-other-book"`)
	require.True(t, currentReadingStart >= 0 && otherStart >= 0, "book cards missing: %s", html)
	currentReadingCard := html[currentReadingStart:otherStart]
	otherCard := html[otherStart:]
	assert.Contains(t, currentReadingCard, "Currently reading")
	assert.NotContains(t, currentReadingCard, "View in Reading", "Reading is not a page")
	assert.NotContains(t, currentReadingCard, "/reading#", "Reading is not a page")
	assert.NotContains(t, currentReadingCard, "To Read", "current Book was also presented as To Read: %s", currentReadingCard)
	assert.False(t, strings.Contains(otherCard, "Start reading") || strings.Contains(otherCard, `action="/goal/books/other-book"`), "Reading exposed a choose form: %s", otherCard)
}

func TestMyBooksPinsTheCurrentReadingAboveItsBooks(t *testing.T) {
	current := testReadingBook("goal", "Goal book", testAnalyzed)
	current.Book.Source.Language = "de"
	current.Coverage = &domain.AnalysisCoverage{
		AnalyzableTokenCount: 1000, MatureTokenCount: 925, InStudyTokenCount: 950,
		ToTarget: domain.CoverageThreshold{TargetPercent: 98, LemmaCount: 1234, Reachable: true},
	}
	books := []domain.MyBook{{Book: domain.Book{ID: "other-book", OwnerID: "owner", Title: "Other book"}}}
	var output bytes.Buffer
	require.NoError(t, MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", MyBooksBrowseState{CurrentReading: &current}).Render(context.Background(), &output))
	html := output.String()

	pinned, other := strings.Index(html, `id="primary-goal-section"`), strings.Index(html, `aria-labelledby="book-title-other-book"`)
	require.True(t, pinned >= 0 && other > pinned, "the Current reading is pinned above the Books: %s", html)
	for _, want := range []string{
		"data-current-reading-mature>92.5%</span>", "data-current-reading-in-study>95.0%</span>", "925 of 1,000 word occurrences are Mature.",
		"About 1,234 more words to reach Target coverage (98%).",
		`action="/reading/finish"`, `action="/reading/end"`,
	} {
		assert.Contains(t, html, want)
	}
	assert.NotContains(t, html, "/reading/switch", "Read next replaces Switch")
}

func readingFixtureSession(t *testing.T) (http.Handler, []*http.Cookie, string, *fixtures.Store) {
	t.Helper()
	return readingFixtureSessionWith(t, nil)
}

// readingFixtureSessionWith builds the fixture session and lets a test supply
// one service before the handler is constructed, such as a small catalog.
func readingFixtureSessionWith(t *testing.T, configure func(*Services)) (http.Handler, []*http.Cookie, string, *fixtures.Store) {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "goal-unit-test-secret-0123456789")
	store := fixtures.NewStore()
	authService := auth.New(fixtures.NewAuthStore(), time.Hour)
	services := Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), Analysis: fixtures.Analysis{Store: store}, CatalogueSync: fixtures.NewCatalogueSync(store), SessionLifetime: time.Hour}
	if configure != nil {
		configure(&services)
	}
	h := New(services)
	loginPage := httptest.NewRecorder()
	h.ServeHTTP(loginPage, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/login", nil))
	initialCSRFCookie := findCSRFCookie(t, loginPage.Result().Cookies())
	token := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(loginPage.Body.String())[1]
	loginRequest := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/login", strings.NewReader(url.Values{"csrf_token": {token}, "username": {fixtures.Username}, "password": {fixtures.Password}}.Encode()))
	loginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRequest.AddCookie(initialCSRFCookie)
	loginResponse := httptest.NewRecorder()
	h.ServeHTTP(loginResponse, loginRequest)
	require.Equal(t, http.StatusSeeOther, loginResponse.Code)
	cookies := loginResponse.Result().Cookies()
	return h, cookies, findCSRFCookie(t, cookies).Value, store
}

func findCSRFCookie(t *testing.T, cookies []*http.Cookie) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == csrfCookie {
			return cookie
		}
	}
	require.FailNowf(t, "cookie not found", "cookie %q not found", csrfCookie)
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

func TestCanonicalCurrentReadingReadNextAndEndNeedNoCommitment(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	idempotent := readingTestRequest(t, h, "/reading/books/"+fixtures.BookID+"/start", url.Values{
		"csrf_token": {csrf},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, idempotent.Code)
	assert.Contains(t, idempotent.Header().Get("Location"), "is+already+your+current+reading")

	changed := readingTestRequest(t, h, "/reading/books/fixture-route-match/switch", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, changed.Code)
	assert.Contains(t, changed.Header().Get("Location"), "is+now+your+current+reading")
	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, "fixture-route-match", currentReading.BookID)

	cleared := readingTestRequest(t, h, "/reading/end", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, cleared.Code)
	assert.NotContains(t, cleared.Header().Get("Location"), "error=")
	clearedAgain := readingTestRequest(t, h, "/reading/end", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, clearedAgain.Code)
	assert.Contains(t, clearedAgain.Header().Get("Location"), "error=", "a second End has no reading to end")
}

// End and Finish return to the study language of the form that posted them
// (ADR 0131), not the Last study language another tab may have moved.
func TestEndAndFinishReturnToThePostedStudyLanguage(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))

	ended := readingTestRequest(t, h, "/reading/end", url.Values{"csrf_token": {csrf}, "language": {"de"}}, cookies)
	require.Equal(t, http.StatusSeeOther, ended.Code)
	require.True(t, strings.HasPrefix(ended.Header().Get("Location"), "/de/reading"), ended.Header().Get("Location"))

	finished := readingTestRequest(t, h, "/reading/finish", url.Values{"csrf_token": {csrf}, "language": {"de"}}, cookies)
	require.Equal(t, http.StatusSeeOther, finished.Code, "no reading is left to finish")
	require.True(t, strings.HasPrefix(finished.Header().Get("Location"), "/de/reading"), finished.Header().Get("Location"))
}

func TestReadingPageScopesCurrentReadingToActiveLanguage(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/it/reading", nil)
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
	} {
		assert.Contains(t, body, want, "Italian Journey page missing %q: %s", want, body)
	}
	assert.True(t, strings.Contains(body, `id="journey-book-fixture-italian-goal"`) && !strings.Contains(body, `id="journey-book-fixture-book"`), "Italian Journey page exposed the German Goal: %s", body)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "de"))
	request = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/de/reading", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	body = response.Body.String()
	assert.True(t, strings.Contains(body, "Deutsch") && strings.Contains(body, `id="journey-book-fixture-book"`) && !strings.Contains(body, `id="journey-book-fixture-empty"`), "German Reading did not remain isolated after Italian move: %s", body)
	assert.NotContains(t, body, "set aside from vocabulary selection", "a reading reserves nothing")
}

func TestReadingPageReservesNothingAndOffersNoDeck(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)

	page := performReadingRequest(t, h, http.MethodGet, "/de/reading", nil, cookies)
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.NotContains(t, body, "set aside from vocabulary selection")
	assert.NotContains(t, body, "Reserved vocabulary")
	assert.NotContains(t, body, "Prepare deck")
	assert.NotContains(t, body, "/deck/")
	assert.NotContains(t, body, "deck-preparations")
}

func TestReadingPageShowsEmptyActiveLanguageReading(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	ctx := context.Background()
	books, err := store.ListMyBooksWithEvidence(ctx, fixtures.OwnerID)
	require.NoError(t, err)
	for _, book := range books {
		if book.Book.LanguageTag == "it" && book.Book.ID != fixtures.ItalianGoalBookID && book.Disposition == domain.BookDispositionToRead {
			require.NoError(t, store.MarkBookPreviouslyRead(ctx, fixtures.OwnerID, book.Book.ID))
		}
	}
	require.NoError(t, store.SetActiveStudyLanguage(ctx, fixtures.OwnerID, "it"))

	// The working desk is gone: an address that names a Book moves to that Book's
	// detail page, which serves the word list.
	moved := performReadingRequest(t, h, http.MethodGet, "/de/reading?reading="+fixtures.BookID, nil, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code)
	require.Equal(t, bookDetailURL(fixtures.BookID), moved.Header().Get("Location"))
	response := performReadingRequest(t, h, http.MethodGet, moved.Header().Get("Location"), nil, cookies)
	assert.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, `id="vocabulary-workflow"`, "the Book's words belong to its detail page")
	for _, retired := range []string{"Other To Read", "No other To Read books", "provisional-journey"} {
		assert.NotContains(t, body, retired)
	}
	assert.False(t, strings.Contains(body, `id="journey-book-fixture-empty"`) || strings.Contains(body, `id="journey-book-fixture-edge-content"`), "empty Italian Journey page exposed a member: %s", body)
}

func performReadingRequest(t *testing.T, h http.Handler, method, path string, form url.Values, cookies []*http.Cookie) *httptest.ResponseRecorder {
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
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

func TestCurrentReadingFinishRendersTruthfulOutcomeWithNoVocabularyEffect(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	finished := readingTestRequest(t, h, "/reading/finish", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusOK, finished.Code)
	for _, want := range []string{
		"Reading finished",
		`href="/de/reading">Choose a To Read book in Reading</a>`,
	} {
		assert.Contains(t, finished.Body.String(), want, "finish outcome missing %q: %s", want, finished.Body.String())
	}
	for _, unwanted := range []string{"Vocabulary transition", "Known", "achievement", "Where next?"} {
		assert.NotContains(t, finished.Body.String(), unwanted)
	}
	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Empty(t, currentReading.BookID, "finished Goal=%+v", currentReading)
	disposition, err := store.GetBookDisposition(context.Background(), fixtures.OwnerID, fixtures.BookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, disposition)

	repeated := readingTestRequest(t, h, "/reading/finish", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, repeated.Code, "a second Finish has no reading to finish")
	assert.Contains(t, repeated.Header().Get("Location"), "error=")
}

func TestCurrentReadingFinishRejectsMissingCSRF(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	missingCSRF := readingTestRequest(t, h, "/reading/finish", url.Values{}, cookies)
	assert.Equal(t, http.StatusForbidden, missingCSRF.Code)
	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, fixtures.BookID, currentReading.BookID, "rejected finish changed Goal=%+v", currentReading)
}

func TestCurrentReadingFinishOutcomeShowsTheBookAndNoVocabulary(t *testing.T) {
	residual := currentReadingFinishView{BookTitle: "Reading-only book"}
	var output bytes.Buffer
	require.NoError(t, CurrentReadingFinish(residual).Render(context.Background(), &output))
	for _, want := range []string{"Reading finished for", "Reading-only book", "Choose a To Read book"} {
		assert.Contains(t, output.String(), want, "outcome missing %q: %s", want, output.String())
	}
	assert.NotContains(t, output.String(), "Vocabulary")
	assert.NotContains(t, output.String(), "achievement")
}

func TestCurrentReadingFinishThroughAuthenticatedHandlerAfterRestart(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	cleared := readingTestRequest(t, h, "/reading/end", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, cleared.Code)
	chosen := readingTestRequest(t, h, "/reading/books/"+fixtures.BookID+"/start", url.Values{
		"csrf_token": {csrf},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, chosen.Code)
	currentReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, fixtures.BookID, currentReading.BookID)

	finished := readingTestRequest(t, h, "/reading/finish", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusOK, finished.Code)
	for _, want := range []string{"Reading finished", "Choose a To Read book"} {
		assert.Contains(t, finished.Body.String(), want)
	}
	assert.NotContains(t, finished.Body.String(), `<article role="alert" class=`)
	assert.NotContains(t, finished.Body.String(), "celebrat")
}

func TestEndCurrentReadingNeedsOnlyCSRFThenEndsTheCurrentReading(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)

	missingCSRF := readingTestRequest(t, h, "/reading/end", url.Values{}, cookies)
	assert.Equal(t, http.StatusForbidden, missingCSRF.Code)
	current, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, fixtures.BookID, current.BookID, "rejected End leaves the reading")

	ended := readingTestRequest(t, h, "/reading/end", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, ended.Code)
	assert.NotContains(t, ended.Header().Get("Location"), "error=")
	current, err = store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive())

	for _, path := range []string{"/reading/stop", "/reading/set-aside"} {
		retired := readingTestRequest(t, h, path, url.Values{"csrf_token": {csrf}}, cookies)
		assert.GreaterOrEqual(t, retired.Code, http.StatusBadRequest, path)
	}
}
