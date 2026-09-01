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
)

func renderGoalSection(t *testing.T, goal *journeyBookView, residual *goalResidualView, message, pageError, focusBookID string) string {
	t.Helper()
	var output bytes.Buffer
	if err := GoalSection(goal, residual, "csrf-token", message, pageError, focusBookID).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestGoalSectionRendersEmptyStateAndLiveFeedback(t *testing.T) {
	html := renderGoalSection(t, nil, nil, "Primary Goal cleared.", "", "")
	for _, want := range []string{
		`id="primary-goal-section"`,
		`id="goal-section-status"`,
		`role="status"`,
		"Primary Goal cleared.",
		"No Primary Goal yet",
		"provisional order or My Books",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("Goal section missing %q: %s", want, html)
		}
	}

	html = renderGoalSection(t, nil, nil, "", "This Primary Goal changed since this page was loaded.", "")
	if !strings.Contains(html, `role="alert"`) || !strings.Contains(html, "This Primary Goal changed since this page was loaded") {
		t.Fatalf("Goal section did not render an accessible error: %s", html)
	}
}

func TestGoalSectionRendersReadingOnlyAndResidualStates(t *testing.T) {
	unassessed := testJourneyBook("reading-only", "Reading-only book", "ready")
	unassessed.GoalReadingOnly = true
	unassessed.GoalUnassessed = true
	readingOnlyHTML := renderGoalSection(t, &unassessed, nil, "", "", "reading-only")
	for _, want := range []string{"Reading-only Goal", "No analysis or deck exists yet", "stands on its own", "nothing is prepared automatically"} {
		if !strings.Contains(readingOnlyHTML, want) {
			t.Errorf("unassessed Goal missing %q: %s", want, readingOnlyHTML)
		}
	}

	assessed := testJourneyBook("assessed", "Assessed without deck", "analyzed")
	assessed.GoalReadingOnly = true
	assessed.GoalUnassessed = false
	assessedHTML := renderGoalSection(t, &assessed, nil, "", "", "assessed")
	if !strings.Contains(assessedHTML, "Analysis evidence exists, but no deck has been prepared") {
		t.Fatalf("assessed reading-only copy missing: %s", assessedHTML)
	}

	count := 4
	residual := &goalResidualView{CampaignID: "campaign-1", BookTitle: "Goal book", ReservedCount: &count}
	goal := testJourneyBook("goal", "Goal book", "analyzed")
	residualHTML := renderGoalSection(t, &goal, residual, "", "", "goal")
	for _, want := range []string{
		"Vocabulary work remains",
		"4 ungraduated vocabulary identities remain reserved.",
		"active campaign and its reserved vocabulary unchanged",
		"Graduate or abandon",
		"Clear Primary Goal",
		`<details`,
	} {
		if !strings.Contains(residualHTML, want) {
			t.Errorf("residual Goal missing %q: %s", want, residualHTML)
		}
	}
}

func TestJourneyGoalControlsUseExpectedStateAndStaySeparated(t *testing.T) {
	goal := testJourneyBook("goal", "Goal book", "analyzed")
	first := testJourneyBook("first", "First book", "ready")
	second := testJourneyBook("second", "Second book", "ready")
	html := renderJourney(t, journeyPageView{Goal: &goal, Provisional: []journeyBookView{first, second}}, "", "", "")
	if strings.Count(html, `action="/goal/books/first"`) != 1 || strings.Count(html, `action="/goal/books/second"`) != 1 {
		t.Fatalf("expected one choose form per provisional card: %s", html)
	}
	if !strings.Contains(html, `name="expected_goal_book_id" value="goal"`) {
		t.Fatalf("provisional choose forms did not carry the current Goal: %s", html)
	}
	goalStart := strings.Index(html, `id="journey-book-goal"`)
	goalEnd := strings.Index(html[goalStart:], "</article>")
	goalCard := html[goalStart : goalStart+goalEnd]
	if strings.Contains(goalCard, `action="/goal/books/`) {
		t.Fatalf("Goal card exposed a choose control: %s", goalCard)
	}
}

func TestMyBooksGoalControlsAndJourneyLink(t *testing.T) {
	books := []domain.MyBook{
		{Book: domain.Book{ID: "goal-book", OwnerID: "owner", Title: "Current goal"}},
		{Book: domain.Book{ID: "other-book", OwnerID: "owner", Title: "Other book"}},
	}
	var output bytes.Buffer
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", "goal-book").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	goalStart := strings.Index(html, `aria-labelledby="book-title-goal-book"`)
	otherStart := strings.Index(html, `aria-labelledby="book-title-other-book"`)
	if goalStart < 0 || otherStart < 0 {
		t.Fatalf("book cards missing: %s", html)
	}
	goalCard := html[goalStart:otherStart]
	otherCard := html[otherStart:]
	for _, want := range []string{"Current Primary Goal", "View Primary Goal in Reading Journey", "/journey#journey-book-goal-book"} {
		if !strings.Contains(goalCard, want) {
			t.Errorf("current Goal card missing %q: %s", want, goalCard)
		}
	}
	if strings.Contains(goalCard, "Choose as Primary Goal") {
		t.Fatalf("current Goal card exposed choose control: %s", goalCard)
	}
	if !strings.Contains(otherCard, `action="/goal/books/other-book"`) || !strings.Contains(otherCard, `name="expected_goal_book_id" value="goal-book"`) {
		t.Fatalf("other book did not expose expected choose form: %s", otherCard)
	}
}

func TestVisibleJourneyEntriesSkipAnchoredGoal(t *testing.T) {
	entries := []domain.ReadingJourneyEntry{
		{BookID: "first", Position: 1},
		{BookID: "goal", Position: 2},
		{BookID: "second", Position: 3},
	}
	visible := visibleJourneyEntries(entries, "goal")
	if len(visible) != 2 || visible[0].BookID != "first" || visible[1].BookID != "second" {
		t.Fatalf("visible Journey order=%+v", visible)
	}
	if entries[1].BookID != "goal" || entries[0].BookID != "first" || entries[2].BookID != "second" {
		t.Fatalf("visible-order helper mutated source entries=%+v", entries)
	}
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
	if loginResponse.Code != http.StatusSeeOther {
		t.Fatalf("fixture login status=%d body=%s", loginResponse.Code, loginResponse.Body.String())
	}
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
	t.Fatalf("cookie %q not found", name)
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

func TestGoalMutationRoutesAreIdempotentAndPreserveResidualCampaigns(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	idempotent := goalRequest(t, h, "/goal/books/"+fixtures.BookID, url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID},
	}, cookies)
	if idempotent.Code != http.StatusSeeOther || !strings.Contains(idempotent.Header().Get("Location"), "already+your+Primary+Goal") {
		t.Fatalf("idempotent choose=%d location=%q", idempotent.Code, idempotent.Header().Get("Location"))
	}

	changed := goalRequest(t, h, "/goal/books/fixture-empty", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID},
	}, cookies)
	if changed.Code != http.StatusSeeOther || !strings.Contains(changed.Header().Get("Location"), "reserved+vocabulary+are+unchanged") {
		t.Fatalf("residual change=%d location=%q", changed.Code, changed.Header().Get("Location"))
	}
	goal, _ := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID)
	if goal.BookID != "fixture-empty" {
		t.Fatalf("changed Goal=%+v", goal)
	}

	cleared := goalRequest(t, h, "/goal/clear", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {"fixture-empty"},
	}, cookies)
	if cleared.Code != http.StatusSeeOther {
		t.Fatalf("clear=%d location=%q", cleared.Code, cleared.Header().Get("Location"))
	}
	clearedAgain := goalRequest(t, h, "/goal/clear", url.Values{"csrf_token": {csrf}, "expected_goal_book_id": {""}}, cookies)
	if clearedAgain.Code != http.StatusSeeOther || !strings.Contains(clearedAgain.Header().Get("Location"), "No+Primary+Goal+was+set") {
		t.Fatalf("idempotent clear=%d location=%q", clearedAgain.Code, clearedAgain.Header().Get("Location"))
	}
}
