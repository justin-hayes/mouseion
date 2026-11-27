//go:build integration

package analysisinsights

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverageEndToEndOwnerIsolationAndLegacyReanalysis(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis insights store", store.Close)

	alice, err := store.CreateUser(ctx, "insights-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "insights-bob", false)
	require.NoError(t, err)
	artifact := domain.NormalizedArtifact{ContentHash: "sha256:insights-e2e", Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
	err = store.PutArtifact(ctx, artifact, []domain.SharedLemma{
		{CanonicalLemma: "eins", UPOS: "NOUN", Morphology: []byte(`{}`), Frequency: 70},
		{CanonicalLemma: "zwei", UPOS: "VERB", Morphology: []byte(`{}`), Frequency: 20},
		{CanonicalLemma: "drei", UPOS: "ADJ", Morphology: []byte(`{}`), Frequency: 10},
	})
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Insights", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	text := "eins zwei drei"
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "insights-e2e", Title: "Insights", MediaType: "application/epub+zip", ContentHash: artifact.ContentHash, Content: []byte(text), FullText: text},
		domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "insights"), Order: 0, SpineIndex: 0, ManifestID: "insights", Text: text, EndOffset: uint64(len(text)), MediaType: "application/xhtml+xml", Linear: true}}})
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, alice.ID, book.ID, source.ID))
	corpus, err := store.PutCorpus(ctx, alice.ID, source.ID, artifact.ContentHash)
	require.NoError(t, err)
	service := NewService(store)
	_, err = service.Coverage(ctx, alice.ID, corpus.ID)
	t.Run("legacy coverage reports statistics unavailable", func(t *testing.T) {
		require.ErrorIs(t, err, ErrStatisticsUnavailable)
	})
	seedStudyWord(t, ctx, store, alice.ID, "eins", "NOUN", true)
	seedStudyWord(t, ctx, store, bob.ID, "zwei", "VERB", true)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analyzable_token_count=100,distinct_lemma_count=3,sentence_count=4,normalized_token_count=120,empty_sentence_count=1,median_sentence_token_count=30,p90_sentence_token_count=40,long_sentence_count=1 WHERE id=$1`, corpus.ID)
	require.NoError(t, err)

	var snapshotID, runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, alice.ID, source.ID).Scan(&snapshotID))
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'test','1','insights','completed',now()) RETURNING id::text`, alice.ID, source.ID, source.ContentRevisionID, snapshotID).Scan(&runID))
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, runID, alice.ID, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, alice.ID, runID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, alice.ID, book.ID, source.ID, runID)
	require.NoError(t, err)

	// Until the Book's count projection is ready, coverage is withheld rather
	// than derived from raw corpus counts.
	_, err = service.Coverage(ctx, alice.ID, corpus.ID)
	require.ErrorIs(t, err, ErrCountsUpdating)
	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_count_readiness(owner_id,book_id,language,analysis_run_id,corpus_id,builder_version) VALUES($1,$2,'de',$3,$4,5)`, alice.ID, book.ID, runID, corpus.ID)
	require.NoError(t, err)
	for _, count := range []struct {
		lemma, upos string
		n           int
	}{{"eins", "NOUN", 70}, {"zwei", "VERB", 20}, {"drei", "ADJ", 10}} {
		_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count) VALUES($1,$2,'de',$3,$4,$5,$6,$7)`, alice.ID, book.ID, runID, corpus.ID, count.lemma, count.upos, count.n)
		require.NoError(t, err)
	}

	got, err := service.Coverage(ctx, alice.ID, corpus.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(70), got.MatureTokenCount)
	assert.Equal(t, int64(30), got.UnknownTokenCount)
	assert.Equal(t, int64(1), got.MatureLemmaCount)
	assert.Equal(t, int64(2), got.UnknownLemmaCount)
	require.NotNil(t, got.TextProfile)
	assert.Equal(t, int64(120), got.TextProfile.NormalizedTokenCount)
	assert.Equal(t, int64(1), got.TextProfile.EmptySentenceCount)
	require.Len(t, got.Thresholds, 3)
	assert.Equal(t, int64(2), got.Thresholds[0].LemmaCount)
	assert.Equal(t, int64(30), got.Thresholds[1].OccurrenceCount)
	assert.Equal(t, int64(2), got.Thresholds[2].LemmaCount)
	require.Len(t, got.TopUnknownLemmas, 2)
	assert.Equal(t, "zwei", got.TopUnknownLemmas[0].CanonicalLemma)
	assert.Equal(t, int64(30), got.UnknownConcentration.OccurrenceCount)
	require.Len(t, got.Projections, 3)
	assert.Equal(t, int64(100), got.Projections[0].ProjectedTokenCount)
	_, err = service.Coverage(ctx, bob.ID, corpus.ID)
	assert.Error(t, err, "bob read Alice's analysis insights")
}

// seedStudyWord puts a German word in the learner's study: Mature (in review at
// the maturity interval) or still learning.
func seedStudyWord(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, lemma, upos string, mature bool) {
	t.Helper()
	state, days := "learning", int64(0)
	if mature {
		state, days = "review", domain.FluencyMaturityDays
	}
	_, err := store.Pool().Exec(ctx, `
WITH m AS (
  INSERT INTO study_material (owner_id, language, canonical_lemma, upos, state, omission_reason)
  VALUES ($1, 'de', $2, $3, 'unpreparable', 'sentence-quality') RETURNING id)
INSERT INTO study_progress (owner_id, language, canonical_lemma, upos, material_id, introduced_at, learned_meaning, state, due, stability, difficulty, scheduled_days, reps, last_review)
SELECT $1, 'de', $2, $3, id, now(), 'seeded', $4, now(), 5, 5, $5, 1, now() FROM m`, owner, lemma, upos, state, days)
	require.NoError(t, err)
}
