//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaleOldLanguageRequestsRecoverWithoutDurableMutation(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "language-context-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	owner := createAccount(t, ctx, store, "language-context", "learner-password", false)
	book, _, _, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "language-context", "Language context", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 3}})
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	// A second study language makes a deliberate switch possible.
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Italian catalog", URL: "https://italian.example/opds"})
	require.NoError(t, err)
	_, err = store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, "it-entry", "Un libro", "Autrice", "it")
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, owner.ID, "de"))

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), Analysis: analysis.NewService(store.Pool(), nil), SessionLifetime: time.Hour})
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")

	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	oldToken := csrf + ".de"
	post := func(path string, form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		return goalRequest(t, h, path, form, cookies)
	}

	switched := post("/active-study-language", url.Values{"csrf_token": {oldToken}, "language": {"it"}, "return_to": {"/reading?language=de"}})
	require.Equal(t, http.StatusSeeOther, switched.Code)
	assert.Equal(t, "/library", switched.Header().Get("Location"))

	for _, path := range []string{"/reading/finish", "/reading/stop", "/reading/books/" + book.ID + "/switch", "/reading/books/" + book.ID + "/start", "/library/books/" + book.ID + "/previously-read"} {
		response := post(path, url.Values{"csrf_token": {oldToken}, "expected_current_book_id": {book.ID}, "expected_current_snapshot_id": {reading.SnapshotID}})
		assert.Equal(t, http.StatusSeeOther, response.Code, path)
		assert.Equal(t, languageChangedPath(), response.Header().Get("Location"), path)
	}
	for _, path := range []string{"/reading?language=de", "/vocabulary/concordance?language=de&mode=effective&term=haus&upos=NOUN", "/vocabulary?language=de&reading=" + book.ID} {
		response := getAs(t, h, path, cookies)
		assert.Equal(t, languageChangedPath(), response.Header().Get("Location"), path)
	}

	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, reading.SnapshotID, current.SnapshotID, "stale submissions must leave the old-language reading untouched")
	var history, known, reserved int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1`, owner.ID).Scan(&history))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&known))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND released_at IS NOT NULL`, owner.ID).Scan(&reserved))
	assert.Zero(t, history)
	assert.Zero(t, known)
	assert.Zero(t, reserved)
	stored, err := store.GetStoredActiveStudyLanguage(ctx, owner.ID)
	require.NoError(t, err)
	assert.Equal(t, "it", stored, "recovery never switches the selection back")

	// The notice page is My Books in the active language.
	notice := getAs(t, h, languageChangedPath(), cookies)
	assert.Equal(t, http.StatusOK, notice.Code)
	assert.Contains(t, notice.Body.String(), "Your study language changed")

	// Another owner's session is unaffected by, and cannot reuse, this state.
	other := createAccount(t, ctx, store, "language-context-other", "other-learner-password", false)
	otherCookies, otherCSRF := loginCookies(t, h, other.Username, "other-learner-password")
	response := goalRequest(t, h, "/reading/finish", url.Values{"csrf_token": {otherCSRF}, "expected_current_book_id": {book.ID}, "expected_current_snapshot_id": {reading.SnapshotID}}, otherCookies)
	assert.NotEqual(t, http.StatusOK, response.Code)
	current, err = store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, reading.SnapshotID, current.SnapshotID)
}
