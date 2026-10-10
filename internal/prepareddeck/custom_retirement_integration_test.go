//go:build integration

package prepareddeck

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A preparation queued before the Custom deck cutover must still be drained by
// River under its retained job kind, and must end failed without a generated
// artifact.
func TestQueuedCustomDeckPreparationFailsAfterRetirementWithoutUnknownJobKind(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "retired-custom-job-owner", false)
	require.NoError(t, err)
	var deckID, preparationID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO custom_vocabulary_decks(owner_id,language,name,creation_key) VALUES($1,'de','Queued deck','e5b4d9a2-4c4e-4b1f-9d7a-2f1c0a6e8b31') RETURNING id::text`, owner.ID).Scan(&deckID))
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO custom_vocabulary_deck_preparations(owner_id,custom_deck_id,submission_key,language,deck_name,filename,state,selected_identities) VALUES($1,$2,'c2b1d1f0-6a5e-4f7e-8d2c-9e3b4a5f6071','de','Queued deck','queued-deck.apkg','queued',1) RETURNING id::text`, owner.ID, deckID).Scan(&preparationID))

	workers := river.NewWorkers()
	AddCustomDeckPreparationWorker(workers, store)
	client, err := river.NewClient[pgx.Tx](riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "River client", client.Stop)
	_, err = client.Insert(ctx, CustomDeckPreparationJobArgs{OwnerID: owner.ID, PreparationID: preparationID}, &river.InsertOpts{Queue: Queue})
	require.NoError(t, err)

	var jobState string
	testutil.Eventually(t, testutil.DefaultWait, "retained Custom deck job reaches a terminal River state", func() (bool, string) {
		if err := store.Pool().QueryRow(ctx, `SELECT state FROM river_job WHERE kind=$1 AND args->>'PreparationID'=$2`, (CustomDeckPreparationJobArgs{}).Kind(), preparationID).Scan(&jobState); err != nil {
			return false, err.Error()
		}
		return jobState == "completed" || jobState == "discarded" || jobState == "cancelled", jobState
	})
	assert.Equal(t, "completed", jobState, "River must recognize the retained job kind rather than discard it as unknown")

	var state, message string
	var artifactAbsent bool
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT state,error,artifact IS NULL FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND id=$2`, owner.ID, preparationID).Scan(&state, &message, &artifactAbsent))
	assert.Equal(t, "failed", state)
	assert.Equal(t, customDeckRetiredMessage, message)
	assert.True(t, artifactAbsent, "a retired preparation generates no artifact")
}
