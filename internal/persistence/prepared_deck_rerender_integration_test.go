//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListStalePreparedDecksOnlyReturnsCurrentUnretiredReadyDecks(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	defer store.Close()

	owner, err := store.CreateUser(ctx, "stale-rerender-owner", false)
	require.NoError(t, err)
	createReady := func(t *testing.T, name string, presentationVersion int, retired bool) domain.DeckPreparation {
		t.Helper()
		source, sourceErr := store.PutSourceMaterial(ctx, domain.SourceMaterial{
			OwnerID: owner.ID, Language: "de", SourceIdentifier: name, Title: name,
			MediaType: "text/plain", ContentHash: name + "-hash", Content: []byte(name), FullText: name,
		})
		require.NoError(t, sourceErr)
		preparation, createErr := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
			OwnerID: owner.ID, SourceMaterialID: source.ID,
			Filename: name + ".apkg", DeckName: name, ContentHash: source.ContentHash,
		})
		require.NoError(t, createErr)
		manifest := cardexport.NewManifest(owner.ID, name, nil)
		tx, beginErr := store.Pool().Begin(ctx)
		require.NoError(t, beginErr)
		frozen, freezeErr := store.FreezePreparedDeckRunTx(ctx, tx, FreezePreparedDeckRunParams{
			OwnerID: owner.ID, PreparationID: preparation.ID, Projection: manifest.Snapshot(),
		})
		require.NoError(t, freezeErr)
		require.NoError(t, tx.Commit(ctx))
		claimToken := "00000000-0000-0000-0000-000000000001"
		_, err = store.ClaimPreparedDeckFinalization(ctx, owner.ID, preparation.ID, frozen.Run.ID, 0, claimToken, time.Now().UTC().Add(time.Minute))
		require.NoError(t, err)
		_, err = store.CompletePreparedDeckRun(ctx, owner.ID, preparation.ID, frozen.Run.ID, claimToken, cardexport.Artifact{
			APKG: []byte(name), Filename: preparation.Filename, DeckName: preparation.DeckName,
		})
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET presentation_version=$3 WHERE owner_id=$1 AND id=$2`, owner.ID, preparation.ID, presentationVersion)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_runs SET presentation_version=$3 WHERE owner_id=$1 AND preparation_id=$2`, owner.ID, preparation.ID, presentationVersion)
		require.NoError(t, err)
		if retired {
			_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET retired_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, preparation.ID)
			require.NoError(t, err)
		}
		return preparation
	}

	stale := createReady(t, "stale", 0, false)
	current := createReady(t, "current", cardexport.PresentationVersion, false)
	retired := createReady(t, "retired", 0, true)
	inputStale := createReady(t, "input-stale", cardexport.PresentationVersion, false)
	inputStaleRun, err := store.GetCurrentPreparedDeckRun(ctx, owner.ID, inputStale.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET render_input_version=0 WHERE owner_id=$1 AND id=$2`, owner.ID, inputStale.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_runs SET render_input_version=0 WHERE owner_id=$1 AND id=$2`, owner.ID, inputStaleRun.ID)
	require.NoError(t, err)
	blocked := createReady(t, "blocked", 0, false)
	blockedRun, err := store.GetCurrentPreparedDeckRun(ctx, owner.ID, blocked.ID)
	require.NoError(t, err)
	require.NoError(t, store.MarkPreparedDeckRequiresRepreparation(ctx, owner.ID, blocked.ID, blockedRun.ID))

	work, err := store.ListStalePreparedDecks(ctx, cardexport.PresentationVersion, 100)
	require.NoError(t, err)
	require.Len(t, work, 2)
	assert.Equal(t, owner.ID, work[0].OwnerID)
	ids := []string{work[0].PreparationID, work[1].PreparationID}
	assert.ElementsMatch(t, []string{stale.ID, inputStale.ID}, ids)
	assert.NotContains(t, ids, current.ID)
	assert.NotContains(t, ids, retired.ID)
	assert.NotContains(t, ids, blocked.ID)
}
