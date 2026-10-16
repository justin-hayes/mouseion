//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAncientGreekRegistrationAppearsInMyBooks is the primary seam for the
// first Ancient Greek slice: after the operator command registers Anabasis
// Book 1, the language appears in the drop-down, the unprepared Book appears in
// My Books, and To Read refuses without enqueuing analysis.
func TestAncientGreekRegistrationAppearsInMyBooks(t *testing.T) {
	ctx, store, pool, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-learner", "learner-password")
	service := ancientgreek.NewService(store)

	report, err := service.Register(ctx, learner.Username, ancientgreek.AnabasisBook1)
	require.NoError(t, err)
	require.True(t, report.Created)
	require.NotEmpty(t, report.BookID)

	var displayName string
	require.NoError(t, pool.QueryRow(ctx, `SELECT display_name FROM supported_languages WHERE language = 'grc'`).Scan(&displayName))
	assert.Equal(t, ancientgreek.DisplayName, displayName)

	book, err := store.GetBook(ctx, learner.ID, report.BookID)
	require.NoError(t, err)
	assert.Equal(t, "Anabasis", book.Title)
	assert.Equal(t, "Xenophon", book.Author)
	assert.Equal(t, domain.MetadataProvenanceOperatorRegistration, book.MetadataProvenance)
	assert.Equal(t, "grc", book.LanguageTag)

	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	library := perform(t, h, "GET", "/library", nil, cookies)
	require.Equal(t, http.StatusOK, library.Code, "library body=%s", library.Body.String())
	for _, want := range []string{"Ancient Greek (grc)", "Anabasis", "By Xenophon", "Marchant 1904", notPreparedYetNote} {
		assert.Contains(t, library.Body.String(), want, "My Books missing %q", want)
	}

	// To Read is refused before the disposition changes and before any analysis
	// job is submitted.
	detail, err := store.GetBookDetail(ctx, learner.ID, report.BookID)
	require.NoError(t, err)
	require.Equal(t, domain.BookDispositionInbox, detail.Disposition)
	refused := perform(t, h, "POST", "/library/books/"+report.BookID+"/to-read", url.Values{
		"csrf_token":        {csrf},
		"expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, refused.Code)
	assert.Contains(t, refused.Header().Get("Location"), url.QueryEscape(notPreparedYetNote))
	after, err := store.GetBookDetail(ctx, learner.ID, report.BookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, after.Disposition, "refusal leaves the disposition unchanged")
	var jobs int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id = $1`, learner.ID).Scan(&jobs))
	assert.Zero(t, jobs, "refusal enqueues no analysis job")

	// Re-running creates no second Book for the learner.
	again, err := service.Register(ctx, learner.Username, ancientgreek.AnabasisBook1)
	require.NoError(t, err)
	assert.False(t, again.Created)
	assert.Equal(t, report.BookID, again.BookID)

	// A second learner gets their own Book for the same Reading unit.
	other := createAccount(t, ctx, store, "greek-learner-two", "learner-password")
	second, err := service.Register(ctx, other.Username, ancientgreek.AnabasisBook1)
	require.NoError(t, err)
	require.True(t, second.Created)
	assert.NotEqual(t, report.BookID, second.BookID)

	var books, associations int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM books WHERE language_tag = 'grc'`).Scan(&books))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM book_reading_units`).Scan(&associations))
	assert.Equal(t, 2, books)
	assert.Equal(t, 2, associations)
}

// TestAncientGreekRegistrationRefusesLegacyGreekRows proves the preflight: an
// existing grc row stops registration and nothing is written.
func TestAncientGreekRegistrationRefusesLegacyGreekRows(t *testing.T) {
	ctx, store, pool, _ := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-learner-legacy", "learner-password")
	_, err := pool.Exec(ctx, `INSERT INTO books(owner_id, title, metadata_provenance, language_state, language_tag) VALUES ($1, 'Legacy Greek', 'catalog_sync', 'chosen', 'grc')`, learner.ID)
	require.NoError(t, err)

	_, err = ancientgreek.NewService(store).Register(ctx, learner.Username, ancientgreek.AnabasisBook1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "legacy grc rows exist")
	assert.Contains(t, err.Error(), "owner="+learner.ID)
	assert.Contains(t, err.Error(), "surface=books count=1")

	var registered, languages int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM books WHERE metadata_provenance = $1`, domain.MetadataProvenanceOperatorRegistration).Scan(&registered))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM supported_languages WHERE language = 'grc'`).Scan(&languages))
	assert.Zero(t, registered, "refusal writes no Book")
	assert.Zero(t, languages, "refusal writes no display name")
}

// ancientGreekHarness provisions one migrated PostgreSQL database, its store,
// and a web handler wired to it.
func ancientGreekHarness(t *testing.T) (context.Context, *persistence.PostgresStore, *pgxpool.Pool, *Handler) {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "ancient-greek-secret-012345678900")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))
	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth:            authService,
		WebAuth:         webauth.New(authService, false, time.Hour),
		Store:           storeDependencies(store),
		Analysis:        analysis.NewService(store.Pool(), nil),
		SessionLifetime: time.Hour,
	})
	return ctx, store, pool, h
}
