//go:build integration

package analysis

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSnapshotAnalysisSendsV2UnitTextUnchanged proves that a v2 snapshot's exact
// unit text, including authorial newlines, reaches the analyzer through the
// existing RPC under per-unit size caps and the whole-snapshot main-text
// fallback.
func TestSnapshotAnalysisSendsV2UnitTextUnchanged(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	owner, err := store.CreateUser(ctx, "v2-analysis-text", false)
	require.NoError(t, err)

	data := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "faust-excerpt"))
	extraction, err := epub.ExtractV2(data)
	require.NoError(t, err)
	source, err := store.PutSourceMaterialWithExtractedSnapshot(ctx, domain.SourceMaterial{
		OwnerID: owner.ID, Language: "de", SourceIdentifier: extraction.Book.SourceIdentifier, Title: extraction.Book.Title,
		MediaType: "application/epub+zip", Content: data, FullText: extraction.Book.FullText,
	}, extraction.Book.ExtractedUnits, extraction.Structure)
	require.NoError(t, err)

	var revisionID, snapshotID, runID string
	require.NoError(t, pool.QueryRow(ctx, `
SELECT r.revision_id::text, s.current_snapshot_id::text
FROM source_materials s
JOIN source_content_revisions r ON r.owner_id=s.owner_id AND r.source_material_id=s.id AND r.revision_id=s.current_content_revision_id
WHERE s.owner_id=$1 AND s.id=$2`, owner.ID, source.ID).Scan(&revisionID, &snapshotID))
	require.NoError(t, pool.QueryRow(ctx, `
INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state)
VALUES($1,$2,$3,$4,$5,$6,$7,'queued') RETURNING id::text`,
		owner.ID, source.ID, revisionID, snapshotID, snapshotAnalyzerName, snapshotAnalyzerVersion, snapshotConfigIdentity).Scan(&runID))

	var requests []analyzer.AnalyzeRequest
	fake := &analyzertest.Fake{AnalyzeFunc: func(_ context.Context, request analyzer.AnalyzeRequest) (analyzer.Result, error) {
		requests = append(requests, request)
		return analyzer.Result{}, nil
	}}
	worker := &Worker{
		Pool: pool, Analyzer: fake, Capabilities: analyzertest.ReadyDepparseCapabilityProvider(),
		Selection: selection.NewService(store), MaxChunkChars: 80,
	}
	args := JobArgs{
		OwnerID: owner.ID, SourceMaterialID: source.ID, ContentRevisionID: revisionID, SnapshotID: snapshotID,
		Language: "de", ContentHash: source.ContentHash, ConfigIdentity: snapshotConfigIdentity, RunID: runID,
		AnalyzerName: snapshotAnalyzerName, AnalyzerVersion: snapshotAnalyzerVersion,
	}
	plan, err := worker.fetchSnapshotWork(ctx, args)
	require.NoError(t, err)
	require.Len(t, plan.inputs, len(extraction.Book.ExtractedUnits.Units), "the fixture declares no main-text landmark, so the whole snapshot is analyzed")
	for index, input := range plan.inputs {
		assert.Equal(t, extraction.Book.ExtractedUnits.Units[index].Text, input.text)
	}

	var merge snapshotChunkMerge
	for _, input := range plan.inputs {
		merge.total += len(chunkText(input.text, worker.MaxChunkChars))
	}
	require.NotZero(t, merge.total)
	for _, input := range plan.inputs {
		require.NoError(t, worker.analyzeDocumentChunks(ctx, args, input, &merge))
	}
	require.Len(t, requests, merge.total)

	byDocument := make(map[string]string, len(plan.inputs))
	chunks := make(map[string]int, len(plan.inputs))
	for _, request := range requests {
		byDocument[request.Document.ID] += request.Document.Text
		chunks[request.Document.ID]++
		assert.LessOrEqual(t, len([]rune(request.Document.Text)), worker.MaxChunkChars)
	}
	for _, unit := range extraction.Book.ExtractedUnits.Units {
		assert.Equal(t, unit.Text, byDocument[unit.ID], "the analyzer received text other than the exact v2 unit text")
	}
	assert.Greater(t, chunks[extraction.Book.ExtractedUnits.Units[0].ID], 1, "the size cap should split the long Director unit")
}
