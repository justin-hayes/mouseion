//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedVocabularyFirstProvenanceAndOwnerIsolation(t *testing.T) {
	ctx := context.Background()
	url := integrationDatabase(t, ctx)
	store := openIntegrationStore(t, ctx, url)

	alice, err := store.CreateUser(ctx, "generated-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "generated-bob", false)
	require.NoError(t, err)
	aliceDeck, err := store.PutDeck(ctx, alice.ID, "de", "First deck")
	require.NoError(t, err)
	aliceSecondDeck, err := store.PutDeck(ctx, alice.ID, "de", "Second deck")
	require.NoError(t, err)
	bobDeck, err := store.PutDeck(ctx, bob.ID, "de", "Bob deck")
	require.NoError(t, err)
	aliceSource, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "alice-first", Title: "First book", MediaType: "text/plain", ContentHash: "alice-first", Content: []byte("first"), FullText: "first"})
	require.NoError(t, err)
	aliceSecondSource, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "alice-second", Title: "Second book", MediaType: "text/plain", ContentHash: "alice-second", Content: []byte("second"), FullText: "second"})
	require.NoError(t, err)

	first, err := store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", FirstDeckID: aliceDeck.ID, FirstSourceMaterialID: &aliceSource.ID})
	require.NoError(t, err)
	assert.Equal(t, aliceDeck.ID, first.FirstDeckID, "first record")
	require.NotNil(t, first.FirstSourceMaterialID, "first record")
	assert.Equal(t, aliceSource.ID, *first.FirstSourceMaterialID, "first record")
	assert.False(t, first.FirstGeneratedAt.IsZero(), "first record")
	repeated, err := store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", FirstDeckID: aliceSecondDeck.ID, FirstSourceMaterialID: &aliceSecondSource.ID})
	require.NoError(t, err)
	assert.Equal(t, first.FirstDeckID, repeated.FirstDeckID, "repeat changed first provenance")
	require.NotNil(t, repeated.FirstSourceMaterialID, "repeat changed first provenance")
	assert.Equal(t, *first.FirstSourceMaterialID, *repeated.FirstSourceMaterialID, "repeat changed first provenance")
	assert.True(t, repeated.FirstGeneratedAt.Equal(first.FirstGeneratedAt), "repeat changed first provenance")
	_, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: bob.ID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", FirstDeckID: bobDeck.ID})
	require.NoError(t, err)
	aliceWords, err := store.ListGeneratedVocabulary(ctx, alice.ID, "de")
	require.NoError(t, err)
	require.Len(t, aliceWords, 1)
	assert.Equal(t, alice.ID, aliceWords[0].OwnerID, "alice list")
	bobWords, err := store.ListGeneratedVocabulary(ctx, bob.ID, "de")
	require.NoError(t, err)
	require.Len(t, bobWords, 1)
	assert.Equal(t, bob.ID, bobWords[0].OwnerID, "bob list")
	assert.Nil(t, bobWords[0].FirstSourceMaterialID, "bob list")

	_, err = store.Pool().Exec(ctx, `DELETE FROM source_materials WHERE owner_id=$1 AND id=$2`, alice.ID, aliceSource.ID)
	require.NoError(t, err)
	aliceWords, err = store.ListGeneratedVocabulary(ctx, alice.ID, "de")
	require.NoError(t, err)
	require.Len(t, aliceWords, 1)
	assert.Nil(t, aliceWords[0].FirstSourceMaterialID, "source deletion did not clear provenance")
}
