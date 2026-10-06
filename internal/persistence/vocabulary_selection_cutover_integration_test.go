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

func TestVocabularyBrowseSelectionCutoverOnlyRemovesSavedSelections(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	ownerA, err := store.CreateUser(ctx, "selection-cutover-a", false)
	require.NoError(t, err)
	ownerB, err := store.CreateUser(ctx, "selection-cutover-b", false)
	require.NoError(t, err)
	emptyOwner, err := store.CreateUser(ctx, "selection-cutover-empty", false)
	require.NoError(t, err)

	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_selections(owner_id,language,canonical_lemma,upos) VALUES
		($1,'de','haus','NOUN'),($1,'de','gehen','VERB'),($1,'it','casa','NOUN'),($2,'el','λόγος','NOUN')`, ownerA.ID, ownerB.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos) VALUES($1,'de','bekannt','NOUN')`, ownerA.ID)
	require.NoError(t, err)

	activeBook, activeSource, activePreparation := createReadingFixture(t, ctx, store, ownerA.ID, "cutover-active")
	makeAnalyzedToReadBook(t, ctx, store, activeBook, activeSource)
	activeReading, err := store.StartCurrentReading(ctx, ownerA.ID, "de", activeBook.ID)
	require.NoError(t, err)
	historicalBook, historicalSource, historicalPreparation := createReadingFixtureInLanguage(t, ctx, store, ownerA.ID, "it", "cutover-history")
	makeAnalyzedToReadBook(t, ctx, store, historicalBook, historicalSource)
	historicalReading, err := store.StartCurrentReading(ctx, ownerA.ID, "it", historicalBook.ID)
	require.NoError(t, err)
	_, err = store.FinishCurrentReading(ctx, ownerA.ID, "it", historicalBook.ID, historicalReading.SnapshotID)
	require.NoError(t, err)

	var customDeckID, customPreparationID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO custom_vocabulary_decks(owner_id,language,name,creation_key)
		VALUES($1,'de','Retained custom deck','0786c507-8476-4aa3-ad96-75b096b482df') RETURNING id::text`, ownerA.ID).Scan(&customDeckID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO custom_vocabulary_deck_identities(owner_id,deck_id,language,canonical_lemma,upos)
		VALUES($1,$2,'de','custom','NOUN')`, ownerA.ID, customDeckID)
	require.NoError(t, err)
	const customArtifact = "retained custom APKG bytes"
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO custom_vocabulary_deck_preparations(owner_id,custom_deck_id,submission_key,language,deck_name,filename,state,artifact,total_cards,selected_identities,completed_at)
		VALUES($1,$2,'d16a563f-0e84-49b6-a26b-600c4f65753f','de','Retained custom deck','retained.apkg','ready',$3,1,1,now()) RETURNING id::text`, ownerA.ID, customDeckID, []byte(customArtifact)).Scan(&customPreparationID))

	before, err := store.CutoverVocabularyBrowseSelections(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(4), before.Deleted)
	assert.Len(t, before.Extents, 3, "report includes each populated owner/language pair")
	var remaining int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM vocabulary_browse_selections`).Scan(&remaining))
	assert.Zero(t, remaining)

	var known int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma='bekannt'`, ownerA.ID).Scan(&known))
	assert.Equal(t, 1, known, "selection identities were not converted or removed from Known vocabulary")
	currentReading, err := store.GetCurrentReading(ctx, ownerA.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, activeReading, currentReading, "current-reading reservation and snapshot remain unchanged")
	var historyRows int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND language='it' AND book_id=$2 AND goal_snapshot_id=$3`, ownerA.ID, historicalBook.ID, historicalReading.SnapshotID).Scan(&historyRows))
	assert.Equal(t, 1, historyRows, "historical Reading snapshot remains unchanged")
	for _, preparation := range []domain.DeckPreparation{activePreparation, historicalPreparation} {
		downloaded, downloadErr := store.DownloadDeckPreparation(ctx, ownerA.ID, preparation.ID)
		require.NoError(t, downloadErr)
		assert.Equal(t, []byte("apkg-"+preparation.Filename[:len(preparation.Filename)-5]), downloaded.Artifact)
	}
	var customDecks, customIdentities, customPreparations int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT
		(SELECT count(*) FROM custom_vocabulary_decks WHERE owner_id=$1 AND id=$2),
		(SELECT count(*) FROM custom_vocabulary_deck_identities WHERE owner_id=$1 AND deck_id=$2),
		(SELECT count(*) FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND id=$3 AND artifact=$4)`, ownerA.ID, customDeckID, customPreparationID, []byte(customArtifact)).Scan(&customDecks, &customIdentities, &customPreparations))
	assert.Equal(t, 1, customDecks)
	assert.Equal(t, 1, customIdentities)
	assert.Equal(t, 1, customPreparations, "Custom deck APKG bytes remain intact")

	// An empty-selection owner is unaffected, and retries are a no-op.
	assert.NotEmpty(t, emptyOwner.ID)
	after, err := store.CutoverVocabularyBrowseSelections(ctx)
	require.NoError(t, err)
	assert.Empty(t, after.Extents)
	assert.Zero(t, after.Deleted)
}
