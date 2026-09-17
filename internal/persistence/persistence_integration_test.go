//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateFirstUserAndSessionIsAtomicAndOwnerReady(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	exists, err := store.HasUsers(ctx)
	require.NoError(t, err)
	assert.False(t, exists, "fresh users")
	u, created, err := store.CreateFirstUserAndSession(ctx, "alice", "hash", "token-hash", time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.True(t, created, "create first")
	assert.Equal(t, "alice", u.Username)
	_, _, err = store.GetSession(ctx, "token-hash")
	require.NoError(t, err, "initial session")
	_, err = store.PutSupportedLanguage(ctx, "de", "German")
	require.NoError(t, err, "supported language")
	_, created, err = store.CreateFirstUserAndSession(ctx, "bob", "hash", "other-token", time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.False(t, created, "second create")
}

func integrationDatabase(t *testing.T, ctx context.Context) string {
	t.Helper()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	return url
}

func TestPostgresOwnershipAndSharedArtifactBoundaries(t *testing.T) {
	ctx := context.Background()
	url := integrationDatabase(t, ctx)
	store := openIntegrationStore(t, ctx, url)

	alice, err := store.CreateUser(ctx, "alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "bob", false)
	require.NoError(t, err)
	_, err = store.PutSupportedLanguage(ctx, "de", "German")
	require.NoError(t, err)

	artifact := domain.NormalizedArtifact{ContentHash: "sha256:shared", Language: "de", SchemaVersion: "1.0.0", NormalizationProfile: "de-standard", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
	err = store.PutArtifact(ctx, artifact, []domain.SharedLemma{{CanonicalLemma: "Haus", UPOS: "NOUN", Morphology: []byte(`{"Gender":"Neut"}`), Frequency: 3}})
	require.NoError(t, err)
	_, lemmas, err := store.GetArtifact(ctx, artifact.ContentHash)
	require.NoError(t, err)
	assert.Len(t, lemmas, 1, "shared artifact")

	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "book-1", Title: "Private", MediaType: "application/epub+zip", ContentHash: "sha256:shared", Content: []byte("epub"), FullText: "private sentence"})
	require.NoError(t, err)
	_, err = store.GetSourceMaterial(ctx, bob.ID, source.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	library, err := store.ListSourceMaterials(ctx, alice.ID)
	require.NoError(t, err)
	require.Len(t, library, 1)
	assert.Equal(t, "not analyzed", library[0].AnalysisStatus, "alice library before analysis")
	bobLibrary, err := store.ListSourceMaterials(ctx, bob.ID)
	require.NoError(t, err)
	assert.Empty(t, bobLibrary, "bob library leaked alice source")
	_, err = store.Pool().Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,display_number,owner_id,source_material_id,content_hash) VALUES(84001,1,$1,$2,$3)`, alice.ID, source.ID, source.ContentHash)
	require.NoError(t, err)
	library, err = store.ListSourceMaterials(ctx, alice.ID)
	require.NoError(t, err)
	require.Len(t, library, 1)
	assert.Equal(t, "analyzing", library[0].AnalysisStatus, "alice library during analysis")
	assert.Equal(t, int64(84001), library[0].AnalysisJobID, "alice library during analysis")
	corpus, err := store.PutCorpus(ctx, alice.ID, source.ID, artifact.ContentHash)
	require.NoError(t, err)
	legacyCorpus, err := store.GetCorpus(ctx, alice.ID, corpus.ID)
	require.NoError(t, err)
	assert.Nil(t, legacyCorpus.Statistics, "legacy corpus statistics want unavailable")
	library, err = store.ListSourceMaterials(ctx, alice.ID)
	require.NoError(t, err)
	require.Len(t, library, 1)
	assert.Equal(t, "analyzing", library[0].AnalysisStatus, "alice library after legacy corpus")
	assert.Empty(t, library[0].AnalysisRunID, "alice library after legacy corpus")
	assert.Empty(t, library[0].CorpusID, "alice library after legacy corpus")
	_, err = store.GetCorpus(ctx, bob.ID, corpus.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = store.PutExampleSentence(ctx, bob.ID, corpus.ID, "s1", "stolen", []byte(`{}`))
	assert.Error(t, err, "bob inserted a sentence into alice corpus")

	state, err := store.PutVocabularyState(ctx, alice.ID, "de", "Haus", "NOUN", "accepted")
	require.NoError(t, err)
	_, err = store.GetVocabularyState(ctx, bob.ID, state.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	err = store.DeleteVocabularyState(ctx, bob.ID, state.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	got, err := store.GetVocabularyState(ctx, alice.ID, state.ID)
	require.NoError(t, err)
	assert.Equal(t, "accepted", got.State, "alice state changed")

	known, err := store.PutKnownVocabulary(ctx, alice.ID, "de", "gehen", "VERB")
	require.NoError(t, err)
	_, err = store.GetKnownVocabulary(ctx, bob.ID, known.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	deck, err := store.PutDeck(ctx, alice.ID, "de", "Study")
	require.NoError(t, err)
	_, err = store.PutCard(ctx, domain.Card{OwnerID: bob.ID, DeckID: deck.ID, DedupKey: "x", CanonicalLemma: "Haus", UPOS: "NOUN", Front: "x", Back: "y"})
	assert.Error(t, err, "bob inserted a card into alice deck")
}
