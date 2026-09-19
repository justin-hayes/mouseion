//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrimaryGoalFreezesAndReleasesVocabularySnapshot(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "goal-snapshot", false)
	require.NoError(t, err)
	book, source, _ := createJourneyFixture(t, ctx, store, owner.ID, "snapshot-one")
	makeJourneyMemberAnalyzed(t, ctx, store, book, source)
	var corpusID string
	err = store.Pool().QueryRow(ctx, `SELECT corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&corpusID)
	require.NoError(t, err)
	_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{
		OwnerID: owner.ID, CorpusID: corpusID, Language: "de", CanonicalLemma: "reisen", UPOS: "VERB",
		OccurrenceCount: 5, ObservedForms: []byte(`["reisen"]`), SentenceReferences: []byte(`[{"location":{"start_offset":4}}]`), Provenance: []byte(`{"min_occurrences":3}`),
	})
	require.NoError(t, err)

	goal, err := store.CreatePrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, goal.SnapshotID)
	assert.Equal(t, corpusID, goal.CorpusID)
	assert.Equal(t, 1, goal.SnapshotSize)
	reserved, err := store.ListReservedVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.Len(t, reserved, 1)
	assert.Equal(t, "reisen", reserved[0].CanonicalLemma)

	_, err = store.Pool().Exec(ctx, `UPDATE selection_candidates SET occurrence_count=99 WHERE owner_id=$1 AND corpus_id=$2`, owner.ID, corpusID)
	require.NoError(t, err)
	stored, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, goal.SnapshotID, stored.SnapshotID)
	assert.Equal(t, 1, stored.SnapshotSize)

	err = store.ClearPrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	reserved, err = store.ListReservedVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, reserved)
	var released int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2 AND released_at IS NOT NULL`, owner.ID, goal.SnapshotID).Scan(&released)
	require.NoError(t, err)
	assert.Equal(t, 1, released)
}

func TestPrimaryGoalCompletionGraduatesFrozenVocabularyWithProvenance(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "goal-graduation", false)
	require.NoError(t, err)
	book, source, _ := createJourneyFixture(t, ctx, store, owner.ID, "graduation")
	makeJourneyMemberAnalyzed(t, ctx, store, book, source)
	var corpusID string
	err = store.Pool().QueryRow(ctx, `SELECT corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&corpusID)
	require.NoError(t, err)
	for _, candidate := range []string{"reisen", "bleiben"} {
		_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{
			OwnerID: owner.ID, CorpusID: corpusID, Language: "de", CanonicalLemma: candidate, UPOS: "VERB",
			OccurrenceCount: 5, ObservedForms: []byte(`[]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`),
		})
		require.NoError(t, err)
	}
	goal, err := store.CreatePrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	deck, err := store.PutDeck(ctx, owner.ID, "de", "Graduation provenance deck")
	require.NoError(t, err)
	_, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{
		OwnerID: owner.ID, Language: "de", CanonicalLemma: "bleiben", UPOS: "VERB",
		FirstDeckID: deck.ID, FirstSourceMaterialID: &source.ID,
	})
	require.NoError(t, err)
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "reisen", "")
	require.NoError(t, err)

	_, err = store.Pool().Exec(ctx, `
CREATE FUNCTION test_goal_graduation_failure() RETURNS trigger
LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced goal completion failure'; END; $$;
CREATE TRIGGER test_goal_graduation_failure
BEFORE DELETE ON reading_journey_membership
FOR EACH ROW EXECUTE FUNCTION test_goal_graduation_failure();`)
	require.NoError(t, err)
	_, err = store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.Error(t, err)
	var graduatedAfterRollback int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND canonical_lemma='bleiben'`, owner.ID).Scan(&graduatedAfterRollback)
	require.NoError(t, err)
	assert.Zero(t, graduatedAfterRollback)
	goalAfterRollback, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, book.ID, goalAfterRollback.BookID)
	_, err = store.Pool().Exec(ctx, `DROP TRIGGER test_goal_graduation_failure ON reading_journey_membership; DROP FUNCTION test_goal_graduation_failure();`)
	require.NoError(t, err)

	result, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	assert.Equal(t, goal.SnapshotID, result.Completion.GoalSnapshotID)
	assert.Equal(t, 2, result.Completion.SnapshotVocabularyCount)
	assert.Equal(t, 1, result.Completion.EligibleVocabularyCount)
	assert.Equal(t, 1, result.Completion.GraduatedVocabularyCount)
	assert.Equal(t, 1, result.Completion.AlreadyKnownVocabularyCount)

	known, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Len(t, known, 2)
	var completionBook, completionSnapshot, completionAnalysis string
	var completionAt, generatedAt *string
	err = store.Pool().QueryRow(ctx, `
SELECT completion_book_id::text, completion_goal_snapshot_id::text, completion_analysis_run_id::text,
       completion_at::text, generated_first_at::text
FROM known_vocabulary
WHERE owner_id=$1 AND canonical_lemma='bleiben'`, owner.ID).Scan(&completionBook, &completionSnapshot, &completionAnalysis, &completionAt, &generatedAt)
	require.NoError(t, err)
	assert.Equal(t, book.ID, completionBook)
	assert.Equal(t, goal.SnapshotID, completionSnapshot)
	assert.NotEmpty(t, completionAnalysis)
	assert.NotNil(t, completionAt)
	assert.NotNil(t, generatedAt)
	knownByLemma := make(map[string]domain.KnownVocabulary, len(known))
	for _, item := range known {
		knownByLemma[item.CanonicalLemma] = item
	}
	assert.Equal(t, "Accepted on Primary Goal completion", knownByLemma["bleiben"].Provenance)

	assert.NotEmpty(t, result.Completion.BookID)
	repeated, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	assert.Equal(t, result.Completion, repeated.Completion)
	reserved, err := store.ListReservedVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, reserved)
}

func TestPrimaryGoalCompletionAcceptsEmptySnapshot(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "goal-empty-completion", false)
	require.NoError(t, err)
	book, source, _ := createJourneyFixture(t, ctx, store, owner.ID, "empty-completion")
	makeJourneyMemberAnalyzed(t, ctx, store, book, source)
	goal, err := store.CreatePrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	assert.Zero(t, goal.SnapshotSize)

	result, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Completion.SnapshotVocabularyCount)
	assert.Equal(t, 0, result.Completion.EligibleVocabularyCount)
	assert.Equal(t, 0, result.Completion.GraduatedVocabularyCount)
	assert.Equal(t, 0, result.Completion.AlreadyKnownVocabularyCount)
	assert.NotEmpty(t, result.Completion.GoalSnapshotID)
}
