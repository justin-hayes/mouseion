//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPublishedAnalysisStaysInEffectDuringReanalysis re-analyses an analyzed
// Book's unchanged revision and drives the newer run through running and
// failed. The published analysis must stay the evidence in My Books and Reading
// throughout, and the Book must still start as the current reading.
func TestPublishedAnalysisStaysInEffectDuringReanalysis(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "reanalysis-evidence-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "reanalysis-alice", "alice-password", false)
	capabilities := staticCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
		{Language: "de", DisplayName: "German", Ready: true},
	}}}
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Analysis: fixtures.Analysis{}, AnalysisInsights: analysisinsights.NewService(store), Capabilities: capabilities,
		CatalogueSync:   fixtures.NewCatalogueSync(fixtures.NewStore()),
		PreparedDeck:    &recordingPreparedDeck{preparations: make(map[string]domain.DeckPreparation)},
		SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, "reanalysis-alice", "alice-password")

	book, source, _, _ := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "reanalysis", "Reanalysis evidence book", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 2},
		{Language: "de", CanonicalLemma: "Baum", UPOS: "NOUN", OccurrenceCount: 1},
	})
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, book.ID, domain.BookDispositionToRead))
	published := migrationMyBook(t, ctx, store, alice.ID, book.ID)
	require.Equal(t, domain.BookAnalyzed, published.EvidenceState())

	// The re-analysis of the same revision is queued and then running. The
	// published analysis still matches the current content, so it stays in effect.
	newerRunID := seedNewerAnalysisRun(t, ctx, store, alice.ID, source)
	setNewerRunState(t, ctx, store, alice.ID, newerRunID, "running")
	running := migrationMyBook(t, ctx, store, alice.ID, book.ID)
	assert.Equal(t, domain.BookAnalyzed, running.EvidenceState())
	assert.Equal(t, domain.RunRunning, running.Classification().Run)
	assert.Equal(t, domain.CurrentReadingEligible, running.Classification().Eligibility)
	libraryRunning := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	require.Equal(t, http.StatusOK, libraryRunning.Code)
	assert.Contains(t, libraryRunning.Body.String(), "Re-analysis running.")
	assert.NotContains(t, libraryRunning.Body.String(), "Analysis running.")

	// The newer run then fails. The published analysis is unchanged and retry is
	// offered for the failed run.
	setNewerRunState(t, ctx, store, alice.ID, newerRunID, "failed")
	failed := migrationMyBook(t, ctx, store, alice.ID, book.ID)
	assert.Equal(t, domain.BookAnalyzed, failed.EvidenceState())
	assert.Equal(t, domain.RunFailed, failed.Classification().Run)
	assert.Equal(t, domain.CurrentReadingEligible, failed.Classification().Eligibility)
	assert.Equal(t, domain.RecoveryRetryAnalysis, failed.Classification().Recovery)
	libraryFailed := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	require.Equal(t, http.StatusOK, libraryFailed.Code)
	assert.Contains(t, libraryFailed.Body.String(), "Re-analysis failed.")
	assert.NotContains(t, libraryFailed.Body.String(), "Analysis failed.")

	started := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code)
	assert.Contains(t, started.Header().Get("Location"), "/reading?message=")
	current, err := store.GetCurrentReading(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, book.ID, current.BookID)

	readingFailed := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, readingFailed.Code)
	assert.Contains(t, readingFailed.Body.String(), "Reanalysis evidence book")
	assert.Contains(t, readingFailed.Body.String(), "Re-analysis failed")
	assert.NotContains(t, readingFailed.Body.String(), "Analysis failed")
	assert.NotContains(t, readingFailed.Body.String(), "Analysis incomplete")
}

// seedNewerAnalysisRun records a re-analysis of the Book's current revision as
// the Book's latest analysis job. The run starts queued; the caller moves its
// state to exercise the lifecycle, which is how the analysis job reports it.
func seedNewerAnalysisRun(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner string, source domain.SourceMaterial) string {
	t.Helper()
	var snapshotID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, source.ID).Scan(&snapshotID))
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state) VALUES($1,$2,$3,$4,'test','1','reanalysis-newer','queued') RETURNING id::text`, owner, source.ID, source.ContentRevisionID, snapshotID).Scan(&runID))
	var jobID, displayNumber int64
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT COALESCE(MAX(river_job_id),0)+1,COALESCE(MAX(display_number),0)+1 FROM analysis_jobs`).Scan(&jobID, &displayNumber))
	_, err := store.Pool().Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash,progress,display_number,analysis_run_id) VALUES($1,$2,$3,$4,0,$5,$6)`, jobID, owner, source.ID, source.ContentHash, displayNumber, runID)
	require.NoError(t, err)
	return runID
}

func setNewerRunState(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, runID, state string) {
	t.Helper()
	_, err := store.Pool().Exec(ctx, `UPDATE analysis_runs SET state=$3 WHERE owner_id=$1 AND id=$2`, owner, runID, state)
	require.NoError(t, err)
}
