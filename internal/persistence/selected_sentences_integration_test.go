//go:build integration

package persistence

import (
	"context"
	"math"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReplaceSelectedSentencesReplacesOneIdentity covers the public replacement
// seam: it rejects a corpus the owner does not own, validates each example's
// rank and score, and atomically swaps the stored ranked set for one identity.
func TestReplaceSelectedSentencesReplacesOneIdentity(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "selected-sentences", false)
	require.NoError(t, err)
	otherOwner, err := store.CreateUser(ctx, "selected-sentences-other", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "selected", Title: "Selected", MediaType: "text/plain", ContentHash: "selected-hash", Content: []byte("Haus"), FullText: "Haus"})
	require.NoError(t, err)
	require.NoError(t, store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: "selected-artifact", Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}, nil))
	corpus, err := store.PutCorpus(ctx, owner.ID, source.ID, "selected-artifact")
	require.NoError(t, err)

	require.NoError(t, store.ReplaceSelectedSentences(ctx, owner.ID, corpus.ID, "de", "haus", "NOUN", []domain.ExampleSentence{
		{SentenceKey: "one", Text: "Das Haus ist groß.", SourceLocation: []byte(`{"source":"one"}`), SelectionRank: 1, SelectionScore: 10, SelectionReasons: []byte(`["one"]`), Chosen: true},
		{SentenceKey: "two", Text: "Ein Haus am See.", SourceLocation: []byte(`{"source":"two"}`), SelectionRank: 2, SelectionScore: 5, SelectionReasons: []byte(`["two"]`)},
	}))

	t.Run("the stored set keeps the given rank and choice", func(t *testing.T) {
		stored, err := store.ListSelectedSentences(ctx, owner.ID, corpus.ID, "de", "haus", "NOUN")
		require.NoError(t, err)
		require.Len(t, stored, 2)
		assert.Equal(t, []string{"one", "two"}, []string{stored[0].SentenceKey, stored[1].SentenceKey})
		assert.Equal(t, "Das Haus ist groß.", stored[0].Text)
		assert.Equal(t, 10, stored[0].SelectionScore)
		assert.True(t, stored[0].Chosen)
		assert.False(t, stored[1].Chosen)
	})

	t.Run("a replacement clears the identity's previous rows", func(t *testing.T) {
		require.NoError(t, store.ReplaceSelectedSentences(ctx, owner.ID, corpus.ID, "de", "haus", "NOUN", []domain.ExampleSentence{
			{SentenceKey: "three", Text: "Häuser.", SourceLocation: []byte(`{"source":"three"}`), SelectionRank: 1, SelectionScore: 1, SelectionReasons: []byte(`[]`), Chosen: true},
		}))
		replaced, listErr := store.ListSelectedSentences(ctx, owner.ID, corpus.ID, "de", "haus", "NOUN")
		require.NoError(t, listErr)
		require.Len(t, replaced, 1)
		assert.Equal(t, "three", replaced[0].SentenceKey)
	})

	t.Run("a corpus the owner does not own is not found", func(t *testing.T) {
		err := store.ReplaceSelectedSentences(ctx, otherOwner.ID, corpus.ID, "de", "haus", "NOUN", []domain.ExampleSentence{
			{SentenceKey: "forged", Text: "Haus", SelectionRank: 1, SelectionScore: 1, SelectionReasons: []byte(`[]`)},
		})
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("an invalid rank is rejected", func(t *testing.T) {
		err := store.ReplaceSelectedSentences(ctx, owner.ID, corpus.ID, "de", "haus", "NOUN", []domain.ExampleSentence{
			{SentenceKey: "bad-rank", Text: "Haus", SelectionRank: math.MaxInt32 + 1, SelectionScore: 1, SelectionReasons: []byte(`[]`)},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid selected sentence rank")
	})

	t.Run("an invalid score is rejected", func(t *testing.T) {
		err := store.ReplaceSelectedSentences(ctx, owner.ID, corpus.ID, "de", "haus", "NOUN", []domain.ExampleSentence{
			{SentenceKey: "bad-score", Text: "Haus", SelectionRank: 1, SelectionScore: math.MaxInt32 + 1, SelectionReasons: []byte(`[]`)},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid selected sentence score")
	})
}
