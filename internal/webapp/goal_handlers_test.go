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
		"analyzed member as your current commitment",
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
	first := testJourneyBook("first", "First book", "analyzed")
	second := testJourneyBook("second", "Second book", "ready")
	first.Book.Source.MediaType = "application/epub+zip"
	first.Book.Source.ContentRevisionID = "first-revision"
	first.Book.AnalysisState = "completed"
	first.Book.AnalysisRunID = "first-run"
	first.Book.CorpusID = "first-corpus"
	first.CanChooseGoal = true
	second.GoalEligibilityReason = "This book needs a successfully completed current analysis before it can become a Primary Goal."
	html := renderJourney(t, journeyPageView{Goal: &goal, Provisional: []journeyBookView{first, second}}, "", "", "")
	if strings.Count(html, `action="/goal/books/first"`) != 1 || strings.Contains(html, `action="/goal/books/second"`) {
		t.Fatalf("expected a choose form only for the eligible provisional card: %s", html)
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
	if err := MyBooksPage(domain.User{Username: "learner"}, "csrf", books, "", "", "goal-book", true, MyBooksBrowseState{}).Render(context.Background(), &output); err != nil {
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
	if strings.Contains(otherCard, "Choose as Primary Goal") || strings.Contains(otherCard, `action="/goal/books/other-book"`) {
		t.Fatalf("My Books exposed a choose form: %s", otherCard)
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

	changed := goalRequest(t, h, "/goal/books/fixture-route-match", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID},
	}, cookies)
	if changed.Code != http.StatusSeeOther || !strings.Contains(changed.Header().Get("Location"), "reserved+vocabulary+are+unchanged") {
		t.Fatalf("residual change=%d location=%q", changed.Code, changed.Header().Get("Location"))
	}
	goal, _ := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID)
	if goal.BookID != "fixture-route-match" {
		t.Fatalf("changed Goal=%+v", goal)
	}

	cleared := goalRequest(t, h, "/goal/clear", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {"fixture-route-match"},
	}, cookies)
	if cleared.Code != http.StatusSeeOther {
		t.Fatalf("clear=%d location=%q", cleared.Code, cleared.Header().Get("Location"))
	}
	clearedAgain := goalRequest(t, h, "/goal/clear", url.Values{"csrf_token": {csrf}, "expected_goal_book_id": {""}}, cookies)
	if clearedAgain.Code != http.StatusSeeOther || !strings.Contains(clearedAgain.Header().Get("Location"), "No+Primary+Goal+was+set") {
		t.Fatalf("idempotent clear=%d location=%q", clearedAgain.Code, clearedAgain.Header().Get("Location"))
	}
}

func TestPrimaryGoalFinishRendersTruthfulOutcomeAndIsIdempotent(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	finished := goalRequest(t, h, "/goal/finish", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID},
	}, cookies)
	if finished.Code != http.StatusOK {
		t.Fatalf("finish=%d body=%s", finished.Code, finished.Body.String())
	}
	for _, want := range []string{
		"Reading finished",
		"Vocabulary transition",
		"No vocabulary was added to known vocabulary",
		"Vocabulary work remains",
		"2 ungraduated identities remain reserved",
		"Reading Journey recalculated",
		"Where next?",
		"No new Primary Goal has been selected",
		"conditional-projected coverage",
		"Choose another book from Reading Journey",
	} {
		if !strings.Contains(finished.Body.String(), want) {
			t.Errorf("finish outcome missing %q: %s", want, finished.Body.String())
		}
	}
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID)
	if err != nil || goal.ReadingFinishedAt == nil {
		t.Fatalf("finished Goal=%+v err=%v", goal, err)
	}
	if goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID); err != nil || goal.BookID != fixtures.BookID {
		t.Fatalf("finished Goal history=%+v err=%v", goal, err)
	}

	repeated := goalRequest(t, h, "/goal/finish", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID},
	}, cookies)
	if repeated.Code != http.StatusOK || !strings.Contains(repeated.Body.String(), "Reading finished") {
		t.Fatalf("idempotent finish=%d body=%s", repeated.Code, repeated.Body.String())
	}
}

func TestPrimaryGoalFinishRejectsStaleAndMissingCSRF(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	stale := goalRequest(t, h, "/goal/finish", url.Values{
		"csrf_token": {csrf}, "expected_goal_book_id": {"stale-book"},
	}, cookies)
	if stale.Code != http.StatusSeeOther || !strings.Contains(stale.Header().Get("Location"), "This+Primary+Goal+changed") {
		t.Fatalf("stale finish=%d location=%q", stale.Code, stale.Header().Get("Location"))
	}
	missingCSRF := goalRequest(t, h, "/goal/finish", url.Values{"expected_goal_book_id": {fixtures.BookID}}, cookies)
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing csrf finish=%d body=%s", missingCSRF.Code, missingCSRF.Body.String())
	}
	goal, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID)
	if err != nil || goal.ReadingFinishedAt != nil {
		t.Fatalf("rejected finish changed Goal=%+v err=%v", goal, err)
	}
}

func TestPrimaryGoalFinishOutcomeDistinguishesGraduationAndConditionalEvidence(t *testing.T) {
	count := 2
	graduated := primaryGoalFinishView{
		BookTitle: "Finished book",
		Campaign:  &domain.LearningCampaign{VocabularyGraduatedAt: timePtr(time.Now())},
		Graduated: []domain.CampaignVocabulary{{CanonicalLemma: "gehen", UPOS: "VERB"}},
		Evidence: []finishEvidenceView{{
			Book:            testJourneyBook("next", "Next book", "analyzed"),
			Changed:         true,
			BeforeLabel:     "Current evidence",
			BeforeCurrent:   "40.0%",
			BeforeProjected: "60.0%",
			AfterLabel:      "Current evidence",
			AfterCurrent:    "50.0%",
			AfterProjected:  "70.0%",
		}},
	}
	var output bytes.Buffer
	if err := PrimaryGoalFinish(graduated, "csrf").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"exactly 1 eligible vocabulary identities", "gehen (VERB)", "Before:", "Now — Current evidence", "Where next?"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("graduation outcome missing %q: %s", want, output.String())
		}
	}

	residual := primaryGoalFinishView{BookTitle: "Reading-only book", ResidualVocabulary: count, Campaign: &domain.LearningCampaign{Status: domain.CampaignActive}}
	output.Reset()
	if err := PrimaryGoalFinish(residual, "csrf").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"No vocabulary was added to known vocabulary", "future effect is conditional", "No new Primary Goal has been selected"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("residual outcome missing %q: %s", want, output.String())
		}
	}
}

func timePtr(value time.Time) *time.Time { return &value }
