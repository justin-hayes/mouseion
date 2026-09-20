//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrimaryGoalSnapshotBackfillPreservesLegacyStudies(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	moveApplicationMigrations(t, databaseURL, -7)
	matchingOwner, err := store.CreateUser(ctx, "snapshot-migration-matching", false)
	require.NoError(t, err)
	emptyOwner, err := store.CreateUser(ctx, "snapshot-migration-empty", false)
	require.NoError(t, err)
	missingSnapshotOwner, err := store.CreateUser(ctx, "snapshot-migration-missing", false)
	require.NoError(t, err)
	otherOwner, err := store.CreateUser(ctx, "snapshot-migration-other", false)
	require.NoError(t, err)
	languageOwner, err := store.CreateUser(ctx, "snapshot-migration-language", false)
	require.NoError(t, err)

	matchingBook, matchingSource, matchingRun, _ := insertMigrationAnalysisFixture(t, ctx, pool, store, matchingOwner.ID, "de", "matching")
	emptyBook, emptySource, emptyRun, emptyCorpus := insertMigrationAnalysisFixture(t, ctx, pool, store, emptyOwner.ID, "de", "empty")
	missingSnapshotBook, err := store.CreateBook(ctx, domain.Book{OwnerID: missingSnapshotOwner.ID, Title: "Migration missing snapshot", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	otherBook, otherSource, otherRun, _ := insertMigrationAnalysisFixture(t, ctx, pool, store, otherOwner.ID, "de", "other")
	languageBook, languageSource, languageRun, languageCorpus := insertMigrationAnalysisFixture(t, ctx, pool, store, languageOwner.ID, "de", "language")
	italianBook, _, _, _ := insertMigrationAnalysisFixture(t, ctx, pool, store, matchingOwner.ID, "it", "italian")

	_, err = pool.Exec(ctx, `
INSERT INTO primary_goals(owner_id, language, book_id) VALUES
  ($1, 'de', $2),
  ($1, 'it', $3),
  ($4, 'de', $5),
  ($6, 'de', $7),
  ($8, 'de', $9),
  ($10, 'de', $11)`, matchingOwner.ID, matchingBook, italianBook, otherOwner.ID, otherBook, languageOwner.ID, languageBook, emptyOwner.ID, emptyBook, missingSnapshotOwner.ID, missingSnapshotBook.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO selection_candidates(owner_id, corpus_id, language, canonical_lemma, upos, occurrence_count, observed_forms, eligible_sentence_refs, provenance) VALUES ($1,$2,'de','must-not-widen','NOUN',5,'[]','[]','{}')`, emptyOwner.ID, emptyCorpus)
	require.NoError(t, err)

	otherPreparation := insertLegacyPreparation(t, ctx, pool, otherOwner.ID, otherBook, otherSource, otherRun, "other-artifact")
	insertLegacyGeneratedVocabulary(t, ctx, pool, otherOwner.ID, otherPreparation, "de", "other", "NOUN")
	insertLegacyPreparationVocabulary(t, ctx, pool, otherOwner.ID, otherPreparation, "de", "other", "NOUN")

	matchingPreparation := insertLegacyPreparation(t, ctx, pool, matchingOwner.ID, matchingBook, matchingSource, matchingRun, "matching-artifact")
	insertLegacyGeneratedVocabulary(t, ctx, pool, matchingOwner.ID, matchingPreparation, "de", "legacy", "NOUN")
	insertLegacyPreparationVocabulary(t, ctx, pool, matchingOwner.ID, matchingPreparation, "de", "legacy", "NOUN")
	emptyPreparation := insertLegacyPreparation(t, ctx, pool, emptyOwner.ID, emptyBook, emptySource, emptyRun, "empty-artifact")

	// This reservation has the right owner, Book, and current source but the
	// wrong study language, so it must not be adopted by the German Goal.
	languagePreparation := insertLegacyPreparation(t, ctx, pool, languageOwner.ID, languageBook, languageSource, languageRun, "language-artifact")
	insertLegacyGeneratedVocabulary(t, ctx, pool, languageOwner.ID, languagePreparation, "it", "falsely-matching", "NOUN")
	insertLegacyPreparationVocabulary(t, ctx, pool, languageOwner.ID, languagePreparation, "it", "falsely-matching", "NOUN")
	_, err = pool.Exec(ctx, `INSERT INTO processing_history(owner_id, corpus_id, operation, status, details) VALUES ($1,$2,'prepared-deck','completed','{}')`, languageOwner.ID, languageCorpus)
	require.NoError(t, err)

	moveApplicationMigrations(t, databaseURL, 7)

	var matchingSnapshot, matchingPreparationSnapshot string
	err = pool.QueryRow(ctx, `SELECT snapshot_id::text FROM primary_goals WHERE owner_id=$1 AND language='de'`, matchingOwner.ID).Scan(&matchingSnapshot)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT COALESCE(goal_snapshot_id::text, '') FROM deck_preparations WHERE owner_id=$1 AND id=$2`, matchingOwner.ID, matchingPreparation).Scan(&matchingPreparationSnapshot)
	require.NoError(t, err)
	assert.Equal(t, matchingSnapshot, matchingPreparationSnapshot, "matching preparation was not bound to the Goal snapshot")
	var snapshotSource, snapshotAnalysis, snapshotRevision, snapshotContent, snapshotCorpus string
	err = pool.QueryRow(ctx, `SELECT source_material_id::text, analysis_run_id::text, content_revision_id::text, content_snapshot_id::text, corpus_id::text FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2`, matchingOwner.ID, matchingSnapshot).Scan(&snapshotSource, &snapshotAnalysis, &snapshotRevision, &snapshotContent, &snapshotCorpus)
	require.NoError(t, err)
	var currentRevision, currentContent, currentCorpus string
	err = pool.QueryRow(ctx, `SELECT content_revision_id::text, snapshot_id::text, corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, matchingOwner.ID, matchingBook).Scan(&currentRevision, &currentContent, &currentCorpus)
	require.NoError(t, err)
	assert.Equal(t, matchingSource, snapshotSource)
	assert.Equal(t, matchingRun, snapshotAnalysis)
	assert.Equal(t, currentRevision, snapshotRevision)
	assert.Equal(t, currentContent, snapshotContent)
	assert.Equal(t, currentCorpus, snapshotCorpus)

	var snapshotLemma string
	err = pool.QueryRow(ctx, `SELECT canonical_lemma FROM primary_goal_snapshot_vocabulary WHERE owner_id=$1 AND snapshot_id=$2`, matchingOwner.ID, matchingSnapshot).Scan(&snapshotLemma)
	require.NoError(t, err)
	assert.Equal(t, "legacy", snapshotLemma)

	var matchingReleased, artifactCount, generatedCount, preparationVocabularyCount int
	var matchingArtifact []byte
	var generatedDeck, generatedSource string
	err = pool.QueryRow(ctx, `
SELECT count(*) FILTER (WHERE p.released_at IS NOT NULL), count(*) FILTER (WHERE p.artifact IS NOT NULL),
       (SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='legacy'),
       (SELECT count(*) FROM deck_preparation_vocabulary WHERE owner_id=$1 AND deck_preparation_id=p.id),
       p.artifact, gv.first_deck_id::text, gv.first_source_material_id::text
FROM deck_preparations p
JOIN generated_vocabulary gv ON gv.owner_id=p.owner_id AND gv.canonical_lemma='legacy'
WHERE p.owner_id=$1 AND p.id=$2
GROUP BY p.id, gv.first_deck_id, gv.first_source_material_id`, matchingOwner.ID, matchingPreparation).Scan(&matchingReleased, &artifactCount, &generatedCount, &preparationVocabularyCount, &matchingArtifact, &generatedDeck, &generatedSource)
	require.NoError(t, err)
	assert.Zero(t, matchingReleased, "matching active reservation was released")
	assert.Equal(t, 1, artifactCount, "matching deck artifact was not retained")
	assert.Equal(t, []byte("matching-artifact"), matchingArtifact, "matching deck artifact changed")
	assert.Equal(t, 1, generatedCount, "matching generated-vocabulary history was not retained")
	assert.Equal(t, 1, preparationVocabularyCount, "matching preparation vocabulary was not retained")
	assert.NotEmpty(t, generatedDeck, "matching generated deck provenance was lost")
	assert.Equal(t, matchingSource, generatedSource, "matching generated source provenance changed")
	var knownCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, matchingOwner.ID).Scan(&knownCount)
	require.NoError(t, err)
	assert.Zero(t, knownCount, "migration graduated vocabulary")

	var emptySnapshot, emptyPreparationSnapshot string
	err = pool.QueryRow(ctx, `SELECT snapshot_id::text FROM primary_goals WHERE owner_id=$1 AND language='de'`, emptyOwner.ID).Scan(&emptySnapshot)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT COALESCE(goal_snapshot_id::text, '') FROM deck_preparations WHERE owner_id=$1 AND id=$2`, emptyOwner.ID, emptyPreparation).Scan(&emptyPreparationSnapshot)
	require.NoError(t, err)
	assert.Empty(t, emptyPreparationSnapshot, "ambiguous empty preparation was adopted without durable activity evidence")
	var emptyOwnerSnapshotSize, emptyReleased, emptyArtifactCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshot_vocabulary WHERE owner_id=$1 AND snapshot_id=$2`, emptyOwner.ID, emptySnapshot).Scan(&emptyOwnerSnapshotSize)
	require.NoError(t, err)
	assert.Zero(t, emptyOwnerSnapshotSize, "empty matching preparation was widened from current candidates")
	err = pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE released_at IS NOT NULL), count(*) FILTER (WHERE artifact IS NOT NULL) FROM deck_preparations WHERE owner_id=$1 AND id=$2`, emptyOwner.ID, emptyPreparation).Scan(&emptyReleased, &emptyArtifactCount)
	require.NoError(t, err)
	assert.Equal(t, 1, emptyReleased, "ambiguous empty preparation was not released")
	assert.Equal(t, 1, emptyArtifactCount, "ambiguous empty preparation artifact was deleted")

	var emptySnapshotSize int
	err = pool.QueryRow(ctx, `
SELECT count(*) FROM primary_goal_snapshot_vocabulary v
JOIN primary_goals g ON g.owner_id=v.owner_id AND g.snapshot_id=v.snapshot_id
WHERE g.owner_id=$1 AND g.language='it'`, matchingOwner.ID).Scan(&emptySnapshotSize)
	require.NoError(t, err)
	assert.Zero(t, emptySnapshotSize, "empty snapshot was widened by another language")

	var isolatedSnapshotSize int
	err = pool.QueryRow(ctx, `
SELECT count(*) FROM primary_goal_snapshot_vocabulary v
JOIN primary_goals g ON g.owner_id=v.owner_id AND g.snapshot_id=v.snapshot_id
WHERE g.owner_id=$1 AND g.language='de'`, otherOwner.ID).Scan(&isolatedSnapshotSize)
	require.NoError(t, err)
	assert.Equal(t, 1, isolatedSnapshotSize, "owner's own vocabulary was not adopted")
	var isolatedLemma string
	err = pool.QueryRow(ctx, `
SELECT v.canonical_lemma FROM primary_goal_snapshot_vocabulary v
JOIN primary_goals g ON g.owner_id=v.owner_id AND g.snapshot_id=v.snapshot_id
WHERE g.owner_id=$1 AND g.language='de'`, otherOwner.ID).Scan(&isolatedLemma)
	require.NoError(t, err)
	assert.Equal(t, "other", isolatedLemma, "another owner's vocabulary crossed the snapshot boundary")

	var languageSnapshotSize, languageReleased, languageArtifactCount, languageGeneratedCount, operationalHistoryCount int
	err = pool.QueryRow(ctx, `
SELECT count(*) FROM primary_goal_snapshot_vocabulary v
JOIN primary_goals g ON g.owner_id=v.owner_id AND g.snapshot_id=v.snapshot_id
WHERE g.owner_id=$1 AND g.language='de'`, languageOwner.ID).Scan(&languageSnapshotSize)
	require.NoError(t, err)
	assert.Zero(t, languageSnapshotSize, "wrong-language reservation entered the German snapshot")
	err = pool.QueryRow(ctx, `
SELECT count(*) FILTER (WHERE released_at IS NOT NULL), count(*) FILTER (WHERE artifact IS NOT NULL),
       (SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='falsely-matching')
FROM deck_preparations WHERE owner_id=$1 AND id=$2`, languageOwner.ID, languagePreparation).Scan(&languageReleased, &languageArtifactCount, &languageGeneratedCount)
	require.NoError(t, err)
	assert.Equal(t, 1, languageReleased, "wrong-language reservation was not released")
	assert.Equal(t, 1, languageArtifactCount, "unmatched deck artifact was deleted")
	assert.Equal(t, 1, languageGeneratedCount, "unmatched generated-vocabulary history was deleted")
	err = pool.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND corpus_id=$2`, languageOwner.ID, languageCorpus).Scan(&operationalHistoryCount)
	require.NoError(t, err)
	assert.Equal(t, 1, operationalHistoryCount, "unmatched operational history was deleted")

	var missingSnapshot string
	err = pool.QueryRow(ctx, `SELECT COALESCE(snapshot_id::text, '') FROM primary_goals WHERE owner_id=$1 AND language='de'`, missingSnapshotOwner.ID).Scan(&missingSnapshot)
	require.NoError(t, err)
	assert.Empty(t, missingSnapshot, "a Goal without current analysis unexpectedly received a snapshot")
	firstCompletion, err := store.RecordReadingFinishedPrimaryGoal(ctx, missingSnapshotOwner.ID, "de", missingSnapshotBook.ID, "")
	require.NoError(t, err)
	secondCompletion, err := store.RecordReadingFinishedPrimaryGoal(ctx, missingSnapshotOwner.ID, "de", missingSnapshotBook.ID, "")
	require.NoError(t, err)
	assert.Equal(t, firstCompletion.Completion, secondCompletion.Completion, "missing-snapshot completion retry changed the outcome")
	var missingSnapshotHistoryCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND language='de' AND book_id=$2`, missingSnapshotOwner.ID, missingSnapshotBook.ID).Scan(&missingSnapshotHistoryCount)
	require.NoError(t, err)
	assert.Equal(t, 1, missingSnapshotHistoryCount, "missing-snapshot completion retry duplicated history")

	emptyCompletion, err := store.RecordReadingFinishedPrimaryGoal(ctx, emptyOwner.ID, "de", emptyBook, emptySnapshot)
	require.NoError(t, err)
	emptyCompletionRetry, err := store.RecordReadingFinishedPrimaryGoal(ctx, emptyOwner.ID, "de", emptyBook, emptySnapshot)
	require.NoError(t, err)
	assert.Equal(t, emptyCompletion.Completion, emptyCompletionRetry.Completion, "empty Goal completion retry changed the outcome")
	var emptyHistoryCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND language='de' AND book_id=$2`, emptyOwner.ID, emptyBook).Scan(&emptyHistoryCount)
	require.NoError(t, err)
	assert.Equal(t, 1, emptyHistoryCount, "empty Goal completion retry duplicated history")

	migratedCompletion, err := store.RecordReadingFinishedPrimaryGoal(ctx, matchingOwner.ID, "de", matchingBook, matchingSnapshot)
	require.NoError(t, err)
	migratedRetry, err := store.RecordReadingFinishedPrimaryGoal(ctx, matchingOwner.ID, "de", matchingBook, matchingSnapshot)
	require.NoError(t, err)
	assert.Equal(t, migratedCompletion.Completion, migratedRetry.Completion, "migrated completion retry changed the outcome")
	var migratedHistoryCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND language='de' AND book_id=$2`, matchingOwner.ID, matchingBook).Scan(&migratedHistoryCount)
	require.NoError(t, err)
	assert.Equal(t, 1, migratedHistoryCount, "migrated completion retry duplicated history")
}

func TestPrimaryGoalSnapshotEmptyPreparationCorrectionIsConservative(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	moveApplicationMigrations(t, databaseURL, -7)
	activeOwner, err := store.CreateUser(ctx, "snapshot-empty-active", false)
	require.NoError(t, err)
	inactiveOwner, err := store.CreateUser(ctx, "snapshot-empty-inactive", false)
	require.NoError(t, err)
	releasedOwner, err := store.CreateUser(ctx, "snapshot-empty-released", false)
	require.NoError(t, err)
	foreignOwner, err := store.CreateUser(ctx, "snapshot-empty-foreign", false)
	require.NoError(t, err)
	languageOwner, err := store.CreateUser(ctx, "snapshot-empty-language", false)
	require.NoError(t, err)

	activeBook, activeSource, activeRun, activeCorpus := insertMigrationAnalysisFixture(t, ctx, pool, store, activeOwner.ID, "de", "empty-active")
	inactiveBook, inactiveSource, inactiveRun, inactiveCorpus := insertMigrationAnalysisFixture(t, ctx, pool, store, inactiveOwner.ID, "de", "empty-inactive")
	releasedBook, releasedSource, releasedRun, releasedCorpus := insertMigrationAnalysisFixture(t, ctx, pool, store, releasedOwner.ID, "de", "empty-released")
	foreignBook, foreignSource, foreignRun, foreignCorpus := insertMigrationAnalysisFixture(t, ctx, pool, store, foreignOwner.ID, "de", "empty-foreign")
	languageBook, languageSource, languageRun, languageCorpus := insertMigrationAnalysisFixture(t, ctx, pool, store, languageOwner.ID, "de", "empty-language")

	_, err = pool.Exec(ctx, `
INSERT INTO primary_goals(owner_id, language, book_id) VALUES
  ($1, 'de', $2),
  ($3, 'de', $4),
  ($5, 'de', $6),
  ($7, 'de', $8),
  ($9, 'de', $10),
  ($9, 'it', $10)`,
		activeOwner.ID, activeBook,
		inactiveOwner.ID, inactiveBook,
		releasedOwner.ID, releasedBook,
		foreignOwner.ID, foreignBook,
		languageOwner.ID, languageBook)
	require.NoError(t, err)

	activePreparation := insertLegacyPreparation(t, ctx, pool, activeOwner.ID, activeBook, activeSource, activeRun, "empty-active-artifact")
	inactivePreparation := insertLegacyPreparation(t, ctx, pool, inactiveOwner.ID, inactiveBook, inactiveSource, inactiveRun, "empty-inactive-artifact")
	_, err = pool.Exec(ctx, `UPDATE deck_preparations SET studying_at=NULL WHERE owner_id=$1 AND id=$2`, inactiveOwner.ID, inactivePreparation)
	require.NoError(t, err)
	releasedPreparation := insertLegacyPreparation(t, ctx, pool, releasedOwner.ID, releasedBook, releasedSource, releasedRun, "empty-released-artifact")
	_, err = pool.Exec(ctx, `UPDATE deck_preparations SET studying_at=NULL,released_at=now() WHERE owner_id=$1 AND id=$2`, releasedOwner.ID, releasedPreparation)
	require.NoError(t, err)
	foreignPreparation := insertLegacyPreparation(t, ctx, pool, foreignOwner.ID, foreignBook, foreignSource, foreignRun, "empty-foreign-artifact")
	languagePreparation := insertLegacyPreparation(t, ctx, pool, languageOwner.ID, languageBook, languageSource, languageRun, "empty-language-artifact")

	for _, candidate := range []struct {
		owner  string
		corpus string
		lemma  string
	}{
		{activeOwner.ID, activeCorpus, "must-not-widen-active"},
		{inactiveOwner.ID, inactiveCorpus, "must-not-widen-inactive"},
		{releasedOwner.ID, releasedCorpus, "must-not-widen-released"},
		{foreignOwner.ID, foreignCorpus, "must-not-widen-foreign"},
		{languageOwner.ID, languageCorpus, "must-not-widen-language"},
	} {
		_, err = pool.Exec(ctx, `INSERT INTO selection_candidates(owner_id, corpus_id, language, canonical_lemma, upos, occurrence_count, observed_forms, eligible_sentence_refs, provenance) VALUES ($1,$2,'de',$3,'NOUN',5,'[]','[]','{}')`, candidate.owner, candidate.corpus, candidate.lemma)
		require.NoError(t, err)
	}

	moveApplicationMigrations(t, databaseURL, 7)

	for _, preparation := range []struct {
		owner string
		id    string
	}{
		{activeOwner.ID, activePreparation},
		{inactiveOwner.ID, inactivePreparation},
		{releasedOwner.ID, releasedPreparation},
		{foreignOwner.ID, foreignPreparation},
		{languageOwner.ID, languagePreparation},
	} {
		var snapshotID string
		var studyingAt, releasedAt *string
		err = pool.QueryRow(ctx, `SELECT COALESCE(goal_snapshot_id::text, ''), studying_at::text, released_at::text FROM deck_preparations WHERE owner_id=$1 AND id=$2`, preparation.owner, preparation.id).Scan(&snapshotID, &studyingAt, &releasedAt)
		require.NoError(t, err)
		assert.Empty(t, snapshotID, "empty preparation %s was adopted", preparation.id)
		assert.Nil(t, studyingAt, "empty preparation %s remained active", preparation.id)
		assert.NotNil(t, releasedAt, "empty preparation %s was not released", preparation.id)
	}

	var activeSnapshotID string
	err = pool.QueryRow(ctx, `SELECT snapshot_id::text FROM primary_goals WHERE owner_id=$1 AND language='de'`, activeOwner.ID).Scan(&activeSnapshotID)
	require.NoError(t, err)
	var snapshotVocabularyCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshot_vocabulary WHERE owner_id=$1 AND snapshot_id=$2`, activeOwner.ID, activeSnapshotID).Scan(&snapshotVocabularyCount)
	require.NoError(t, err)
	assert.Zero(t, snapshotVocabularyCount, "ambiguous active empty preparation widened the Goal snapshot")

	var languageSnapshotVocabularyCount int
	err = pool.QueryRow(ctx, `
SELECT count(*) FROM primary_goal_snapshot_vocabulary v
JOIN primary_goals g ON g.owner_id=v.owner_id AND g.snapshot_id=v.snapshot_id
WHERE g.owner_id=$1`, languageOwner.ID).Scan(&languageSnapshotVocabularyCount)
	require.NoError(t, err)
	assert.Zero(t, languageSnapshotVocabularyCount, "language-isolated empty preparation widened a Goal snapshot")
}

func insertMigrationAnalysisFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *PostgresStore, owner, language, suffix string) (bookID, sourceID, runID, corpusID string) {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: "Migration " + suffix, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: language})
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner, Language: language, SourceIdentifier: "migration-" + suffix, Title: "Migration " + suffix, MediaType: "text/plain", ContentHash: "legacy:" + suffix, Content: []byte(suffix), FullText: suffix})
	require.NoError(t, err)
	err = store.LinkSourceToBook(ctx, owner, book.ID, source.ID)
	require.NoError(t, err)
	revisionID := uuid.NewString()
	snapshotID := uuid.NewString()
	_, err = pool.Exec(ctx, `INSERT INTO source_content_revisions(revision_id, owner_id, source_material_id, digest_version, content_digest, content, full_text) VALUES ($1,$2,$3,0,$4,$5,$6)`, revisionID, owner, source.ID, "legacy:"+suffix, []byte(suffix), suffix)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO source_material_unit_snapshots(owner_id, source_material_id, schema_version, snapshot_id, content_revision_id) VALUES ($1,$2,1,$3,$4)`, owner, source.ID, snapshotID, revisionID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE source_materials SET current_content_revision_id=$1, current_snapshot_id=$2 WHERE owner_id=$3 AND id=$4`, revisionID, snapshotID, owner, source.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO analysis_runs(owner_id, source_material_id, content_revision_id, snapshot_id, analyzer_name, analyzer_version, config_identity, state, completed_at) VALUES ($1,$2,$3,$4,'migration-test','1',$5,'completed',now())`, owner, source.ID, revisionID, snapshotID, suffix)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT id::text FROM analysis_runs WHERE owner_id=$1 AND source_material_id=$2`, owner, source.ID).Scan(&runID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO normalized_corpus_artifacts(content_hash, language, schema_version, normalization_profile, normalization_version, analyzer_name, analyzer_version) VALUES ($1,$2,'1',$2,'1','migration-test','1')`, "corpus-"+suffix, language)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO corpora(owner_id, source_material_id, artifact_hash, status, analysis_run_id) VALUES ($1,$2,$3,'complete',$4)`, owner, source.ID, "corpus-"+suffix, runID)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT id::text FROM corpora WHERE owner_id=$1 AND source_material_id=$2`, owner, source.ID).Scan(&corpusID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpusID, owner, runID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO book_current_analyses(owner_id, book_id, source_material_id, analysis_run_id) VALUES ($1,$2,$3,$4)`, owner, book.ID, source.ID, runID)
	require.NoError(t, err)
	return book.ID, source.ID, runID, corpusID
}

func insertLegacyPreparation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, bookID, sourceID, runID, artifact string) string {
	t.Helper()
	preparationID := uuid.NewString()
	_, err := pool.Exec(ctx, `
INSERT INTO deck_preparations(id, owner_id, source_material_id, state, artifact, filename, deck_name, content_hash, total_cards, completed_at, analysis_run_id, studying_at, book_id)
VALUES ($1,$2,$3,'ready',$4,$5,$6,$7,1,now(),$8,now(),$9)`, preparationID, owner, sourceID, []byte(artifact), artifact+".apkg", "Migration deck", "legacy:"+artifact, runID, bookID)
	require.NoError(t, err)
	return preparationID
}

func insertLegacyPreparationVocabulary(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, preparationID, language, lemma, upos string) {
	t.Helper()
	_, err := pool.Exec(ctx, `INSERT INTO deck_preparation_vocabulary(owner_id, deck_preparation_id, language, canonical_lemma, upos, generated_at) VALUES ($1,$2,$3,$4,$5,now())`, owner, preparationID, language, lemma, upos)
	require.NoError(t, err)
}

func insertLegacyGeneratedVocabulary(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, preparationID, language, lemma, upos string) {
	t.Helper()
	deckID := uuid.NewString()
	_, err := pool.Exec(ctx, `INSERT INTO decks(id, owner_id, language, name) VALUES ($1,$2,$3,$4)`, deckID, owner, language, "Migration deck")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO generated_vocabulary(owner_id, language, canonical_lemma, upos, first_deck_id, first_source_material_id) SELECT $1,$2,$3,$4,$5,source_material_id FROM deck_preparations WHERE owner_id=$1 AND id=$6`, owner, language, lemma, upos, deckID, preparationID)
	require.NoError(t, err)
}
