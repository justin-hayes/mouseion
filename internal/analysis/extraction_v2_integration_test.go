//go:build integration

package analysis

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/checked"
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
		tokens := tokenizeIntegrationWords(request.Document.ID, request.Document.Text)
		return analyzer.Result{
			SchemaVersion:        "1",
			Language:             request.Language,
			Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "fixture", AnalyzerVersion: "1"},
			NormalizationProfile: analyzer.NormalizationProfile{Name: "identity", Version: "1"},
			Sentences: []analyzer.Sentence{{
				Text: request.Document.Text, Tokens: tokens,
				Location: analyzer.SourceLocation{SourceDocumentID: request.Document.ID, EndOffset: uint64(len([]rune(request.Document.Text)))},
			}},
		}, nil
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
	assert.Equal(t, extraction.Structure, plan.structure, "the worker loads the exact snapshot's recorded structure")
	selectionCfg, err := worker.selectionConfig(ctx, args, "corpus")
	require.NoError(t, err)
	assert.Equal(t, extraction.Structure, selectionCfg.Structure, "candidate generation uses the same exact-snapshot structure")
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

	analyzed, err := merge.analysis(args, plan.structure)
	require.NoError(t, err)
	wantAnalyzable := eligibleTokensOutsideSpeakerSpans(merge.result, plan.structure)
	assert.Positive(t, wantAnalyzable)
	assert.Less(t, wantAnalyzable, allIntegrationTokens(merge.result), "the fixture must contain excluded label tokens")
	assert.Equal(t, wantAnalyzable, analyzed.statistics.AnalyzableTokenCount, "the snapshot's speaker spans keep label tokens out of the analysis aggregate")
}

var integrationWordPattern = regexp.MustCompile(`[\p{L}\p{M}]+`)

// tokenizeIntegrationWords turns a chunk into one eligible noun per letter run
// using chunk-relative code-point offsets, so the merge's offset adjustment
// lines up with the exact snapshot units.
func tokenizeIntegrationWords(documentID, text string) []analyzer.Token {
	var tokens []analyzer.Token
	for _, index := range integrationWordPattern.FindAllStringIndex(text, -1) {
		surface := text[index[0]:index[1]]
		start, err := checked.Uint64FromInt(utf8.RuneCountInString(text[:index[0]]))
		if err != nil {
			panic(err)
		}
		end, err := checked.Uint64FromInt(utf8.RuneCountInString(text[:index[1]]))
		if err != nil {
			panic(err)
		}
		tokens = append(tokens, analyzer.Token{
			Surface: surface, RawLemma: strings.ToLower(surface), CanonicalLemma: strings.ToLower(surface),
			UPOS: "NOUN", Dependency: "root", Location: analyzer.SourceLocation{SourceDocumentID: documentID, StartOffset: start, EndOffset: end},
		})
	}
	return tokens
}

// eligibleTokensOutsideSpeakerSpans counts merged tokens that do not overlap a
// recorded speaker span, independently of selection's projection.
func eligibleTokensOutsideSpeakerSpans(result analyzer.Result, structure domain.SnapshotStructure) int64 {
	var count int64
	for _, sentence := range result.Sentences {
		for _, token := range sentence.Tokens {
			if !structure.OverlapsSpeakerLabel(token.Location.SourceDocumentID, token.Location.StartOffset, token.Location.EndOffset) {
				count++
			}
		}
	}
	return count
}

// allIntegrationTokens counts every analyzed token, so a passing comparison
// proves at least one label token was excluded rather than none.
func allIntegrationTokens(result analyzer.Result) int64 {
	var count int64
	for _, sentence := range result.Sentences {
		count += int64(len(sentence.Tokens))
	}
	return count
}
