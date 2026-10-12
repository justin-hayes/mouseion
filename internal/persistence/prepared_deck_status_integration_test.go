//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A preparation without a durable run reports a phase derived from its state
// alone.
func TestDeckPreparationStatusPhaseWithoutDurableRun(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "status-phase-owner", false)
	require.NoError(t, err)
	create := func(t *testing.T, name string) domain.DeckPreparation {
		t.Helper()
		source, sourceErr := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: name, Title: name, MediaType: "text/plain", ContentHash: name + "-hash", Content: []byte("Buch"), FullText: "Buch"})
		require.NoError(t, sourceErr)
		created, createErr := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: name + ".apkg", DeckName: name, ContentHash: source.ContentHash})
		require.NoError(t, createErr)
		return created
	}
	claim := func(t *testing.T, p domain.DeckPreparation) {
		t.Helper()
		_, claimErr := store.ClaimDeckPreparation(ctx, owner.ID, p.ID)
		require.NoError(t, claimErr)
	}
	cases := []struct {
		name    string
		prepare func(*testing.T, domain.DeckPreparation)
		state   domain.DeckPreparationState
		phase   string
	}{
		{name: "queued", prepare: func(*testing.T, domain.DeckPreparation) {}, state: domain.DeckPreparationQueued, phase: "queued"},
		{name: "preparing", prepare: claim, state: domain.DeckPreparationPreparing, phase: "freezing"},
		{name: "ready", prepare: func(t *testing.T, p domain.DeckPreparation) {
			t.Helper()
			claim(t, p)
			_, completeErr := store.CompleteDeckPreparation(ctx, owner.ID, p.ID, domain.DeckPreparation{Artifact: []byte("apkg"), Filename: "ready.apkg", DeckName: "ready", TotalCards: 1})
			require.NoError(t, completeErr)
		}, state: domain.DeckPreparationReady, phase: "completed"},
		{name: "failed", prepare: func(t *testing.T, p domain.DeckPreparation) {
			t.Helper()
			claim(t, p)
			_, failErr := store.FailDeckPreparation(ctx, owner.ID, p.ID, "failed for the test")
			require.NoError(t, failErr)
		}, state: domain.DeckPreparationFailed, phase: "failed"},
		{name: "cancelled", prepare: func(t *testing.T, p domain.DeckPreparation) {
			t.Helper()
			_, cancelErr := store.CancelDeckPreparation(ctx, owner.ID, p.ID)
			require.NoError(t, cancelErr)
		}, state: domain.DeckPreparationCancelled, phase: "cancelled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := create(t, "status-phase-"+tc.name)
			tc.prepare(t, p)
			status, statusErr := store.GetDeckPreparationStatus(ctx, owner.ID, p.ID)
			require.NoError(t, statusErr)
			assert.Equal(t, tc.state, status.State)
			assert.Equal(t, tc.phase, status.Phase)
			assert.Empty(t, status.CurrentRunID)
			assert.Zero(t, status.TranslationEligible, "a run-less preparation has no translation progress")
		})
	}
	t.Run("unknown preparation is not found", func(t *testing.T) {
		_, statusErr := store.GetDeckPreparationStatus(ctx, owner.ID, "00000000-0000-0000-0000-000000000000")
		require.ErrorIs(t, statusErr, ErrNotFound)
	})
}
