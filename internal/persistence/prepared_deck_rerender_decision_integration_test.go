//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListStalePreparedDecksMatchesTheFreshnessDecision pins the SQL filter to
// the shared decision: every deck the listing returns must be one the decision
// calls stale, and every deck it omits must be current.
func TestListStalePreparedDecksMatchesTheFreshnessDecision(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "stale-decision-owner", false)
	require.NoError(t, err)

	decks := []struct {
		name                                                       string
		runPresentationVersion, runRenderInputVersion, preparation int
	}{
		{name: "both-current", runPresentationVersion: cardexport.PresentationVersion, runRenderInputVersion: cardexport.RenderInputVersion, preparation: cardexport.RenderInputVersion},
		{name: "presentation-behind", runPresentationVersion: cardexport.PresentationVersion - 1, runRenderInputVersion: cardexport.RenderInputVersion, preparation: cardexport.RenderInputVersion},
		{name: "run-input-behind-preparation-current", runPresentationVersion: cardexport.PresentationVersion, runRenderInputVersion: cardexport.RenderInputVersion - 1, preparation: cardexport.RenderInputVersion},
		{name: "input-behind-both", runPresentationVersion: cardexport.PresentationVersion, runRenderInputVersion: cardexport.RenderInputVersion - 1, preparation: cardexport.RenderInputVersion - 1},
		{name: "run-ahead", runPresentationVersion: cardexport.PresentationVersion + 1, runRenderInputVersion: cardexport.RenderInputVersion, preparation: cardexport.RenderInputVersion},
	}

	wantStale := make([]string, 0, len(decks))
	for _, deck := range decks {
		id := seedReadyPreparedDeck(t, ctx, store, owner.ID, deck.name, deck.runPresentationVersion, deck.runRenderInputVersion, deck.preparation)
		if upToDate, _ := cardexport.DecidePreparedDeckFreshness(deck.runPresentationVersion, deck.runRenderInputVersion, deck.preparation, cardexport.PresentationVersion); !upToDate {
			wantStale = append(wantStale, id)
		}
	}

	work, err := store.ListStalePreparedDecks(ctx, cardexport.PresentationVersion, 100)
	require.NoError(t, err)
	gotStale := make([]string, 0, len(work))
	for _, item := range work {
		gotStale = append(gotStale, item.PreparationID)
	}
	assert.ElementsMatch(t, wantStale, gotStale)
}

// seedReadyPreparedDeck stores one ready deck whose run and preparation carry
// the versions under test, returning the preparation identity.
func seedReadyPreparedDeck(t *testing.T, ctx context.Context, store *PostgresStore, owner, name string, runPresentationVersion, runRenderInputVersion, preparationRenderInputVersion int) string {
	t.Helper()
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{
		OwnerID: owner, Language: "de", SourceIdentifier: name, Title: name,
		MediaType: "text/plain", ContentHash: name + "-hash", Content: []byte(name), FullText: name,
	})
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner, SourceMaterialID: source.ID,
		Filename: name + ".apkg", DeckName: name, ContentHash: source.ContentHash,
	})
	require.NoError(t, err)
	deck, err := testutil.FreezePresentationDeck(ctx, owner, name, nil, testutil.PresentationProvider{})
	require.NoError(t, err)
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	frozen, err := store.FreezePreparedDeckRunTx(ctx, tx, FreezePreparedDeckRunParams{
		OwnerID: owner, PreparationID: preparation.ID, Projection: deck.StorageProjection(),
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	claimToken := "00000000-0000-0000-0000-000000000001"
	_, err = store.ClaimPreparedDeckFinalization(ctx, owner, preparation.ID, frozen.Run.ID, 0, claimToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	_, err = store.CompletePreparedDeckRun(ctx, owner, preparation.ID, frozen.Run.ID, claimToken, cardexport.Artifact{
		APKG: []byte(name), Filename: preparation.Filename, DeckName: preparation.DeckName,
	})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET presentation_version=$3, render_input_version=$4 WHERE owner_id=$1 AND id=$2`, owner, preparation.ID, runPresentationVersion, preparationRenderInputVersion)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_runs SET presentation_version=$4, render_input_version=$5 WHERE owner_id=$1 AND preparation_id=$2 AND id=$3`, owner, preparation.ID, frozen.Run.ID, runPresentationVersion, runRenderInputVersion)
	require.NoError(t, err)
	return preparation.ID
}
