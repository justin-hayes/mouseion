//go:build integration

package vocabulary

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecyclePersistsAcrossCorporaAndIsolatesUsers(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "vocabulary lifecycle store", store.Close)
	alice, err := store.CreateUser(ctx, "lifecycle-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "lifecycle-bob", false)
	require.NoError(t, err)
	lifecycle := NewLifecycle(store)
	id := Identity{"de", "Haus", "NOUN"}
	_, err = lifecycle.Transition(ctx, alice.ID, id, Candidate)
	require.NoError(t, err)
	_, err = lifecycle.Transition(ctx, alice.ID, id, Ignored)
	require.NoError(t, err)
	// Rediscovery from another corpus is represented by requesting candidate and is suppressed.
	_, err = lifecycle.Transition(ctx, alice.ID, id, Candidate)
	assert.Error(t, err, "ignored item implicitly returned as candidate") //nolint:testifylint // Rejection and the independent owner-state checks both need to run.
	_, err = lifecycle.Transition(ctx, bob.ID, id, Candidate)
	require.NoError(t, err)
	aliceState, err := store.GetVocabularyStateByIdentity(ctx, alice.ID, "de", "Haus", "NOUN")
	require.NoError(t, err)
	bobState, err := store.GetVocabularyStateByIdentity(ctx, bob.ID, "de", "Haus", "NOUN")
	require.NoError(t, err)
	assert.Equal(t, "ignored", aliceState.State)
	assert.Equal(t, "candidate", bobState.State)
	_, err = lifecycle.Reset(ctx, alice.ID, id)
	require.NoError(t, err)
	_, err = lifecycle.Transition(ctx, alice.ID, id, Accepted)
	require.NoError(t, err)
	_, err = lifecycle.Transition(ctx, alice.ID, id, Generated)
	require.NoError(t, err)
	var auditCount int
	auditPool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	defer auditPool.Close()
	err = auditPool.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='vocabulary.transition'`, alice.ID).Scan(&auditCount)
	require.NoError(t, err)
	assert.Equal(t, 5, auditCount, "audit rows = %d, want 5", auditCount)
}
