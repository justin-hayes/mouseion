//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAnalysisCorpusVocabularyIsOwnerScopedAndAggregatesMorphology(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	alice, _ := store.CreateUser(ctx, "alice", false)
	bob, _ := store.CreateUser(ctx, "bob", false)
	artifact := domain.NormalizedArtifact{ContentHash: "sha256:insights", Language: "de", SchemaVersion: "1", NormalizationProfile: "de", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
	err := store.PutArtifact(ctx, artifact, []domain.SharedLemma{
		{CanonicalLemma: "haus", UPOS: "NOUN", Morphology: []byte(`{"Number":"Sing"}`), Frequency: 3},
		{CanonicalLemma: "haus", UPOS: "NOUN", Morphology: []byte(`{"Number":"Plur"}`), Frequency: 2},
		{CanonicalLemma: "gehen", UPOS: "VERB", Morphology: []byte(`{}`), Frequency: 1},
		{CanonicalLemma: "5", UPOS: "NOUN", Morphology: []byte(`{}`), Frequency: 7},
		{CanonicalLemma: ".", UPOS: "PUNCT", Morphology: []byte(`{}`), Frequency: 20},
		{CanonicalLemma: "der", UPOS: "DET", Morphology: []byte(`{}`), Frequency: 10},
	})
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "insights", Title: "Book", MediaType: "text/plain", ContentHash: artifact.ContentHash, Content: []byte("book"), FullText: "book"})
	require.NoError(t, err)
	corpus, err := store.PutCorpus(ctx, alice.ID, source.ID, artifact.ContentHash)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analyzable_token_count=13,distinct_lemma_count=3,sentence_count=2,normalized_token_count=15,empty_sentence_count=0,median_sentence_token_count=4,p90_sentence_token_count=5,long_sentence_count=0 WHERE id=$1`, corpus.ID)
	require.NoError(t, err)
	got, err := store.GetAnalysisCorpusVocabulary(ctx, alice.ID, corpus.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Statistics)
	require.NotNil(t, got.Statistics.TextProfile)
	assert.Equal(t, int64(5), got.Statistics.TextProfile.P90SentenceTokenCount)
	assert.Equal(t, int64(6), got.Statistics.AnalyzableTokenCount)
	require.Len(t, got.Lemmas, 2)
	assert.Equal(t, "haus", got.Lemmas[1].CanonicalLemma)
	assert.Equal(t, int64(5), got.Lemmas[1].OccurrenceCount)
	_, err = store.GetAnalysisCorpusVocabulary(ctx, bob.ID, corpus.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}
