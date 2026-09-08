//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

func TestGoalInteractionIntegrationKeepsReadingOnlyBooksAndOwnerBoundaries(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "goal-interaction-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "goal-integration-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "goal-integration-bob", "bob-password", false)
	newBook := func(owner domain.User, title string) domain.Book {
		t.Helper()
		book, createErr := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return book
	}
	readingOnly := newBook(alice, "Reading-only integration book")
	replacement := newBook(alice, "Replacement integration book")

	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: store, SessionLifetime: time.Hour})
	aliceCookies, aliceCSRF := loginCookies(t, h, alice.Username, "alice-password")
	bobCookies, bobCSRF := loginCookies(t, h, bob.Username, "bob-password")

	initialJourney, err := store.GetReadingJourney(ctx, alice.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	chosen := perform(t, h, http.MethodPost, "/goal/books/"+readingOnly.ID, url.Values{
		"csrf_token":            {aliceCSRF},
		"expected_goal_book_id": {""},
	}, aliceCookies)
	if chosen.Code != http.StatusSeeOther || !strings.Contains(chosen.Header().Get("Location"), "active+Reading+Journey+member") {
		t.Fatalf("reading-only Goal response=%d location=%q body=%s", chosen.Code, chosen.Header().Get("Location"), chosen.Body.String())
	}
	goal, err := store.GetPrimaryGoal(ctx, alice.ID)
	if err != nil || goal.BookID != "" {
		t.Fatalf("Goal=%+v err=%v", goal, err)
	}
	journey, err := store.GetReadingJourney(ctx, alice.ID, "de")
	if err != nil || len(journey.Entries) != len(initialJourney.Entries) || journey.Revision != initialJourney.Revision {
		t.Fatalf("choosing Goal changed Journey: before=%+v after=%+v err=%v", initialJourney, journey, err)
	}
	if campaigns, listErr := store.ListLearningCampaigns(ctx, alice.ID); listErr != nil || len(campaigns) != 0 {
		t.Fatalf("choosing Goal created campaign rows=%d err=%v", len(campaigns), listErr)
	}
	if jobs, listErr := store.ListAnalysisJobs(ctx, alice.ID); listErr != nil || len(jobs) != 0 {
		t.Fatalf("choosing Goal created analysis jobs=%d err=%v", len(jobs), listErr)
	}
	if _, err = store.Pool().Exec(ctx, `INSERT INTO primary_goals(owner_id,book_id) VALUES($1,$2)`, alice.ID, readingOnly.ID); err != nil {
		t.Fatal(err)
	}

	stale := perform(t, h, http.MethodPost, "/goal/books/"+replacement.ID, url.Values{
		"csrf_token":            {aliceCSRF},
		"expected_goal_book_id": {"stale-goal"},
	}, aliceCookies)
	if stale.Code != http.StatusSeeOther || !strings.Contains(stale.Header().Get("Location"), "This+Primary+Goal+changed") {
		t.Fatalf("stale Goal response=%d location=%q", stale.Code, stale.Header().Get("Location"))
	}
	goal, _ = store.GetPrimaryGoal(ctx, alice.ID)
	if goal.BookID != readingOnly.ID {
		t.Fatalf("stale request changed Goal=%+v", goal)
	}

	csrfFailure := perform(t, h, http.MethodPost, "/goal/books/"+replacement.ID, url.Values{"expected_goal_book_id": {readingOnly.ID}}, aliceCookies)
	if csrfFailure.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d body=%s", csrfFailure.Code, csrfFailure.Body.String())
	}
	goal, _ = store.GetPrimaryGoal(ctx, alice.ID)
	if goal.BookID != readingOnly.ID {
		t.Fatalf("CSRF failure changed Goal=%+v", goal)
	}

	foreign := perform(t, h, http.MethodPost, "/goal/books/"+readingOnly.ID, url.Values{
		"csrf_token":            {bobCSRF},
		"expected_goal_book_id": {""},
	}, bobCookies)
	if foreign.Code != http.StatusSeeOther || !strings.Contains(foreign.Header().Get("Location"), "not+available+in+My+Books") {
		t.Fatalf("cross-owner Goal response=%d location=%q", foreign.Code, foreign.Header().Get("Location"))
	}
	if bobGoal, getErr := store.GetPrimaryGoal(ctx, bob.ID); getErr != nil || bobGoal.BookID != "" {
		t.Fatalf("cross-owner request changed Bob's Goal=%+v err=%v", bobGoal, getErr)
	}

	cleared := perform(t, h, http.MethodPost, "/goal/clear", url.Values{
		"csrf_token":            {aliceCSRF},
		"expected_goal_book_id": {readingOnly.ID},
	}, aliceCookies)
	if cleared.Code != http.StatusSeeOther || !strings.Contains(cleared.Header().Get("Location"), "Primary+Goal+cleared") {
		t.Fatalf("clear response=%d location=%q", cleared.Code, cleared.Header().Get("Location"))
	}
	clearedAgain := perform(t, h, http.MethodPost, "/goal/clear", url.Values{
		"csrf_token":            {aliceCSRF},
		"expected_goal_book_id": {""},
	}, aliceCookies)
	if clearedAgain.Code != http.StatusSeeOther || !strings.Contains(clearedAgain.Header().Get("Location"), "No+Primary+Goal+was+set") {
		t.Fatalf("idempotent clear response=%d location=%q", clearedAgain.Code, clearedAgain.Header().Get("Location"))
	}
}
