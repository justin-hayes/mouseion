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
	_, err = store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
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

	result, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
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
	repeated, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
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

	result, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Completion.SnapshotVocabularyCount)
	assert.Equal(t, 0, result.Completion.EligibleVocabularyCount)
	assert.Equal(t, 0, result.Completion.GraduatedVocabularyCount)
	assert.Equal(t, 0, result.Completion.AlreadyKnownVocabularyCount)
	assert.NotEmpty(t, result.Completion.GoalSnapshotID)
}

func TestPrimaryGoalCompletionHandlesMissingSnapshotIdempotently(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "goal-missing-snapshot", false)
	require.NoError(t, err)
	otherOwner, err := store.CreateUser(ctx, "goal-missing-snapshot-other", false)
	require.NoError(t, err)

	book, _, _ := createJourneyFixture(t, ctx, store, owner.ID, "missing-snapshot")
	italianBook, _, _ := createJourneyFixtureInLanguage(t, ctx, store, owner.ID, "it", "missing-snapshot-italian")
	otherBook, _, _ := createJourneyFixture(t, ctx, store, otherOwner.ID, "missing-snapshot-other")
	for _, item := range []struct {
		owner, language, book string
	}{
		{owner.ID, "de", book.ID},
		{owner.ID, "it", italianBook.ID},
		{otherOwner.ID, "de", otherBook.ID},
	} {
		journey, journeyErr := store.GetReadingJourney(ctx, item.owner, item.language)
		require.NoError(t, journeyErr)
		_, journeyErr = store.AddToReadingJourney(ctx, item.owner, item.language, item.book, journey.Revision)
		require.NoError(t, journeyErr)
	}
	_, err = store.Pool().Exec(ctx, `INSERT INTO primary_goals(owner_id, language, book_id) VALUES ($1, 'de', $2), ($1, 'it', $3), ($4, 'de', $5)`, owner.ID, book.ID, italianBook.ID, otherOwner.ID, otherBook.ID)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan ReadingFinishResult, 2)
	errors := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			result, finishErr := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, "")
			results <- result
			errors <- finishErr
		}()
	}
	close(start)
	for range 2 {
		require.NoError(t, <-errors)
	}
	first, second := <-results, <-results
	assert.Equal(t, first.Completion, second.Completion)
	assert.Empty(t, first.Completion.GoalSnapshotID)
	assert.Zero(t, first.Completion.SnapshotVocabularyCount)
	assert.Zero(t, first.Completion.EligibleVocabularyCount)
	assert.Zero(t, first.Completion.GraduatedVocabularyCount)
	assert.Zero(t, first.Completion.AlreadyKnownVocabularyCount)

	repeated, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, "")
	require.NoError(t, err)
	assert.Equal(t, first.Completion, repeated.Completion)
	var historyCount, knownCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND language='de' AND book_id=$2`, owner.ID, book.ID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Equal(t, 1, historyCount)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&knownCount)
	require.NoError(t, err)
	assert.Zero(t, knownCount)

	goal, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, goal.BookID)
	italianGoal, err := store.GetPrimaryGoal(ctx, owner.ID, "it")
	require.NoError(t, err)
	assert.Equal(t, italianBook.ID, italianGoal.BookID, "completion crossed the language boundary")
	otherGoal, err := store.GetPrimaryGoal(ctx, otherOwner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, otherBook.ID, otherGoal.BookID, "completion crossed the owner boundary")
}
