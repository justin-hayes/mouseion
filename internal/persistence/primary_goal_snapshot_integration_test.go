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
