//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseDigest fingerprints every stored parse table of a run, so a rebuild can
// prove it never touches the frozen parse.
func parseDigest(t *testing.T, ctx context.Context, pool *pgxpool.Pool, run string) string {
	t.Helper()
	var digest string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT md5(
		  COALESCE((SELECT string_agg(t::text, '|' ORDER BY t.sentence_ordinal, t.token_ordinal) FROM corpus_tokens t WHERE t.analysis_run_id=$1), '') ||
		  COALESCE((SELECT string_agg(n::text, '|' ORDER BY n.sentence_ordinal, n.source_id) FROM corpus_unprinted_syntax_nodes n WHERE n.analysis_run_id=$1), '') ||
		  COALESCE((SELECT string_agg(b::text, '|' ORDER BY b::text) FROM vocabulary_browse_counts b WHERE b.analysis_run_id=$1), ''))`, run).Scan(&digest))
	return digest
}

// TestConstructionRebuildRepairsRunsBehindTheDetectorVersion proves a
// detector-version bump is repaired by the durable worker: rows return without a
// reimport, the parse is untouched, a failed rebuild keeps the old state, and
// the startup scan finds exactly the runs that are missing or behind.
func TestConstructionRebuildRepairsRunsBehindTheDetectorVersion(t *testing.T) {
	ctx, store, pool, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-construction-rebuild", "learner-password")
	registration, err := registerGreekDefinition(t, ctx, store, learner.ID, greekTestDefinition())
	require.NoError(t, err)
	definition, sourcePath := fixtureImportFrom(t, "oga-src-constructions.conllu")
	_, err = ancientgreek.Import(ctx, ancientgreek.ImportRequest{
		OwnerID: learner.ID, BookID: registration.BookID, SourcePath: sourcePath, Definition: definition,
	}, analysis.NewGreekImportWriter(store))
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))
	cookies, _ := loginCookies(t, h, learner.Username, "learner-password")

	var run, corpus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text, corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, learner.ID, registration.BookID).Scan(&run, &corpus))
	studyBody := func(t *testing.T) string {
		t.Helper()
		query := url.Values{
			"book": {registration.BookID}, "run": {run}, "corpus": {corpus}, "unit": {domain.ClassicalUnitID(definition.Key)},
			"sentence": {strconv.Itoa(0)}, "target": {"1"},
		}
		response := perform(t, h, http.MethodGet, "/vocabulary/concordance/sentence?"+query.Encode(), nil, cookies)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		return response.Body.String()
	}
	worker := &analysis.ConstructionRebuildWorker{Pool: pool}
	work := func() error {
		return worker.Work(ctx, &river.Job[analysis.ConstructionRebuildArgs]{Args: analysis.ConstructionRebuildArgs{OwnerID: learner.ID, RunID: run}})
	}
	before := constructionRows(t, ctx, pool, run)
	require.NotEmpty(t, before)
	digest := parseDigest(t, ctx, pool, run)
	versionOf := func() int {
		var v int
		require.NoError(t, pool.QueryRow(ctx, `SELECT detector_version FROM construction_run_markers WHERE analysis_run_id=$1`, run).Scan(&v))
		return v
	}

	t.Run("a failed rebuild keeps the old rows and marker", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE construction_run_markers SET detector_version=1 WHERE analysis_run_id=$1`, run)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `
			CREATE FUNCTION fail_construction_rebuild() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN RAISE EXCEPTION 'rebuild failed after delete'; END $$;
			CREATE TRIGGER fail_construction_rebuild BEFORE INSERT ON construction_occurrences
			FOR EACH ROW EXECUTE FUNCTION fail_construction_rebuild();`)
		require.NoError(t, err)
		require.ErrorContains(t, work(), "rebuild failed after delete")
		assert.Equal(t, before, constructionRows(t, ctx, pool, run), "the delete rolled back")
		assert.Equal(t, 1, versionOf(), "the marker stays behind")
		_, err = pool.Exec(ctx, `DROP TRIGGER fail_construction_rebuild ON construction_occurrences; DROP FUNCTION fail_construction_rebuild()`)
		require.NoError(t, err)
	})

	t.Run("the worker restores rows and marker, and the page shows them again", func(t *testing.T) {
		_, err := pool.Exec(ctx, `DELETE FROM construction_occurrences WHERE analysis_run_id=$1`, run)
		require.NoError(t, err)
		assert.NotContains(t, studyBody(t), "Constructions", "reads stay silent while the marker is behind")
		require.NoError(t, work())
		assert.Equal(t, before, constructionRows(t, ctx, pool, run))
		assert.Equal(t, constructions.AGDTDetectorVersion, versionOf())
		assert.Contains(t, studyBody(t), "Constructions")
		assert.Equal(t, digest, parseDigest(t, ctx, pool, run), "the frozen parse is byte-identical")
	})

	t.Run("a repeat changes nothing", func(t *testing.T) {
		var built1 string
		require.NoError(t, pool.QueryRow(ctx, `SELECT built_at::text FROM construction_run_markers WHERE analysis_run_id=$1`, run).Scan(&built1))
		require.NoError(t, work())
		var built2 string
		require.NoError(t, pool.QueryRow(ctx, `SELECT built_at::text FROM construction_run_markers WHERE analysis_run_id=$1`, run).Scan(&built2))
		assert.Equal(t, built1, built2)
		assert.Equal(t, before, constructionRows(t, ctx, pool, run))
	})

	t.Run("a run that is not native_agdt is left alone", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE analysis_runs SET syntax_state='absent' WHERE id=$1`, run)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `UPDATE construction_run_markers SET detector_version=1 WHERE analysis_run_id=$1`, run)
		require.NoError(t, err)
		require.NoError(t, work())
		assert.Equal(t, 1, versionOf())
		_, err = pool.Exec(ctx, `UPDATE analysis_runs SET syntax_state='native_agdt' WHERE id=$1`, run)
		require.NoError(t, err)
	})

	t.Run("startup scheduling enqueues exactly the missing or behind runs", func(t *testing.T) {
		riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
		require.NoError(t, err)
		service := analysis.NewService(pool, riverClient)
		queued := func() int {
			var n int
			require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='rebuild_construction_occurrences' AND args->>'run_id'=$1`, run).Scan(&n))
			return n
		}
		require.NoError(t, work()) // marker is at 1 (behind): bring it current
		require.NoError(t, service.EnqueueMissingConstructionRebuilds(ctx))
		assert.Zero(t, queued(), "a current run is not queued")

		_, err = pool.Exec(ctx, `DELETE FROM construction_run_markers WHERE analysis_run_id=$1`, run)
		require.NoError(t, err)
		require.NoError(t, service.EnqueueMissingConstructionRebuilds(ctx))
		require.NoError(t, service.EnqueueMissingConstructionRebuilds(ctx))
		assert.Equal(t, 1, queued(), "a missing marker is queued once, however often the scan repeats")
	})
}
