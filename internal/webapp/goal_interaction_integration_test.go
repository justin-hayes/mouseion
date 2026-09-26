//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoalInteractionIntegrationKeepsReadingOnlyBooksAndOwnerBoundaries(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "goal-interaction-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "goal-integration-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "goal-integration-bob", "bob-password", false)
	newBook := func(owner domain.User, title string) domain.Book {
		t.Helper()
		book, createErr := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
		require.NoError(t, createErr)
		return book
	}
	readingOnly := newBook(alice, "Reading-only integration book")
	replacement := newBook(alice, "Replacement integration book")

	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	aliceCookies, aliceCSRF := loginCookies(t, h, alice.Username, "alice-password")
	bobCookies, bobCSRF := loginCookies(t, h, bob.Username, "bob-password")

	initialDisposition, err := store.GetBookDisposition(ctx, alice.ID, readingOnly.ID)
	require.NoError(t, err)
	chosen := perform(t, h, http.MethodPost, "/goal/books/"+readingOnly.ID, url.Values{
		"csrf_token":            {aliceCSRF},
		"expected_goal_book_id": {""},
	}, aliceCookies)
	assert.Equal(t, http.StatusSeeOther, chosen.Code)
	assert.True(t, strings.Contains(chosen.Header().Get("Location"), "must+be+in+To+Read"), "location=%q body=%s", chosen.Header().Get("Location"), chosen.Body.String())
	goal, err := store.GetPrimaryGoal(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, goal.BookID)
	actualDisposition, err := store.GetBookDisposition(ctx, alice.ID, readingOnly.ID)
	require.NoError(t, err)
	assert.Equal(t, initialDisposition, actualDisposition)
	jobs, listErr := store.ListAnalysisJobs(ctx, alice.ID)
	require.NoError(t, listErr)
	assert.Empty(t, jobs)
	_, err = store.Pool().Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3)`, alice.ID, "de", readingOnly.ID)
	require.NoError(t, err)
	readingOnlyState, err := store.GetBookDetail(ctx, alice.ID, readingOnly.ID)
	require.NoError(t, err)
	setAsideGoal := perform(t, h, http.MethodPost, "/library/books/"+readingOnly.ID+"/set-aside", url.Values{
		"csrf_token": {aliceCSRF}, "expected_revision": {strconv.FormatInt(readingOnlyState.DispositionRevision, 10)},
	}, aliceCookies)
	assert.Equal(t, http.StatusSeeOther, setAsideGoal.Code)
	assert.Contains(t, setAsideGoal.Header().Get("Location"), "cannot+be+set+aside")
	disposition, dispositionErr := store.GetBookDisposition(ctx, alice.ID, readingOnly.ID)
	require.NoError(t, dispositionErr)
	assert.Equal(t, domain.BookDispositionInbox, disposition, "current Goal disposition was changed by a set-aside action")

	stale := perform(t, h, http.MethodPost, "/goal/books/"+replacement.ID, url.Values{
		"csrf_token":            {aliceCSRF},
		"expected_goal_book_id": {"stale-goal"},
	}, aliceCookies)
	assert.Equal(t, http.StatusSeeOther, stale.Code)
	assert.True(t, strings.Contains(stale.Header().Get("Location"), "current+reading+changed"), "location=%q", stale.Header().Get("Location"))
	goal, err = store.GetPrimaryGoal(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, readingOnly.ID, goal.BookID)

	csrfFailure := perform(t, h, http.MethodPost, "/goal/books/"+replacement.ID, url.Values{"expected_goal_book_id": {readingOnly.ID}}, aliceCookies)
	assert.Equal(t, http.StatusForbidden, csrfFailure.Code)
	goal, err = store.GetPrimaryGoal(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, readingOnly.ID, goal.BookID)

	foreign := perform(t, h, http.MethodPost, "/goal/books/"+readingOnly.ID, url.Values{
		"csrf_token":            {bobCSRF},
		"expected_goal_book_id": {""},
	}, bobCookies)
	assert.Equal(t, http.StatusSeeOther, foreign.Code)
	assert.True(t, strings.Contains(foreign.Header().Get("Location"), "not+available+in+My+Books"), "location=%q", foreign.Header().Get("Location"))
	bobGoal, getErr := store.GetPrimaryGoal(ctx, bob.ID, "de")
	require.NoError(t, getErr)
	assert.Empty(t, bobGoal.BookID)

	cleared := perform(t, h, http.MethodPost, "/goal/clear", url.Values{
		"csrf_token":            {aliceCSRF},
		"expected_goal_book_id": {readingOnly.ID},
	}, aliceCookies)
	assert.Equal(t, http.StatusSeeOther, cleared.Code)
	assert.True(t, strings.Contains(cleared.Header().Get("Location"), "Current+reading+cleared"), "location=%q", cleared.Header().Get("Location"))
	clearedAgain := perform(t, h, http.MethodPost, "/goal/clear", url.Values{
		"csrf_token":            {aliceCSRF},
		"expected_goal_book_id": {""},
	}, aliceCookies)
	assert.Equal(t, http.StatusSeeOther, clearedAgain.Code)
	assert.True(t, strings.Contains(clearedAgain.Header().Get("Location"), "No+current+reading+was+set"), "location=%q", clearedAgain.Header().Get("Location"))
}
