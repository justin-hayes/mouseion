//go:build integration

package knownvocab

import (
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportPostgresIsolationLifecycleAndIdempotency(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "known vocabulary store", store.Close)
	alice, err := store.CreateUser(ctx, "known-import-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "known-import-bob", false)
	require.NoError(t, err)
	service := NewService(store)
	first, err := service.Import(ctx, alice.ID, "de", strings.NewReader("Daß\nHaus\n"))
	require.NoError(t, err)
	assert.Equal(t, 2, first.Imported)
	second, err := service.Import(ctx, alice.ID, "de", strings.NewReader("Daß\nHaus\n"))
	require.NoError(t, err)
	assert.Equal(t, 2, second.AlreadyKnown)
	assert.Zero(t, second.Imported)
	known, err := store.IsKnownVocabularyIdentity(ctx, alice.ID, "de", "haus", "NOUN")
	require.NoError(t, err)
	assert.True(t, known, "alice wildcard known = %t, %v", known, err)
	for _, check := range []struct{ owner, language string }{{bob.ID, "de"}, {alice.ID, "fr"}} {
		known, checkErr := store.IsKnownVocabularyIdentity(ctx, check.owner, check.language, "haus", "NOUN")
		require.NoError(t, checkErr)
		assert.False(t, known, "unexpected known for owner=%s language=%s", check.owner, check.language)
	}
	var legacyStates int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM vocabulary_states`).Scan(&legacyStates)
	require.NoError(t, err)
	assert.Zero(t, legacyStates, "imports must not create legacy lifecycle rows")
}
