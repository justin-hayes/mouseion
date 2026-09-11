//go:build integration

package selection

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectionPersistsProvenanceAndIsolatesOwners(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()
	alice, err := store.CreateUser(ctx, "selection-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "selection-bob", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "reserved-book", Title: "Reserved", MediaType: "text/plain", ContentHash: "reserved-book", Content: []byte("reserviert"), FullText: "reserviert"})
	require.NoError(t, err)
	prep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: alice.ID, SourceMaterialID: source.ID, Filename: "reserved.apkg", DeckName: "Reserved", ContentHash: source.ContentHash})
	require.NoError(t, err)
	prep, err = store.ClaimDeckPreparation(ctx, alice.ID, prep.ID)
	require.NoError(t, err)
	prep, err = store.CompleteDeckPreparation(ctx, alice.ID, prep.ID, domain.DeckPreparation{Artifact: []byte("apkg"), Filename: prep.Filename, DeckName: prep.DeckName, TotalCards: 1})
	require.NoError(t, err)
	deck, err := store.PutDeck(ctx, alice.ID, "de", "Reserved")
	require.NoError(t, err)
	_, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: "reserviert", UPOS: "ADJ", FirstDeckID: deck.ID, FirstSourceMaterialID: &source.ID})
	require.NoError(t, err)
	_, err = store.StartDeckVocabularyStudy(ctx, alice.ID, prep.ID)
	require.NoError(t, err)
	_, err = store.PutVocabularyState(ctx, alice.ID, "de", "alt", "ADJ", "ignored")
	require.NoError(t, err)
	_, err = store.PutVocabularyState(ctx, alice.ID, "de", "legacy", "NOUN", "generated")
	require.NoError(t, err)
	corpus := fixture(tok("Häuser", "Haus", "NOUN", false), tok("Haus", "Haus", "NOUN", false), tok("alt", "alt", "ADJ", false), tok("alt", "alt", "ADJ", false), tok("legacy", "legacy", "NOUN", false), tok("reserviert", "reserviert", "ADJ", false))
	svc := NewService(store)
	got, err := svc.Select(ctx, alice.ID, corpus, DefaultConfig("book-a"))
	require.NoError(t, err)
	assert.Len(t, got, 2)
	got, err = svc.Select(ctx, bob.ID, corpus, DefaultConfig("book-b"))
	require.NoError(t, err)
	assert.Len(t, got, 4)
	var aliceCount, bobCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1`, alice.ID).Scan(&aliceCount)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1`, bob.ID).Scan(&bobCount)
	require.NoError(t, err)
	assert.Equal(t, 2, aliceCount)
	assert.Equal(t, 4, bobCount)
	var occurrences int
	var forms, refs, provenance []byte
	err = pool.QueryRow(ctx, `SELECT occurrence_count,observed_forms,eligible_sentence_refs,provenance FROM selection_candidates WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&occurrences, &forms, &refs, &provenance)
	require.NoError(t, err)
	assert.Equal(t, 2, occurrences)
	assert.NotEmpty(t, forms)
	assert.NotEmpty(t, refs)
	assert.NotEmpty(t, provenance)
	state, err := store.GetVocabularyStateByIdentity(ctx, alice.ID, "de", "Haus", "NOUN")
	require.NoError(t, err)
	assert.Equal(t, "candidate", state.State)
	_, err = store.ReleaseDeckVocabularyStudy(ctx, alice.ID, prep.ID)
	require.NoError(t, err)
	got, err = svc.Select(ctx, alice.ID, fixture(tok("reserviert", "reserviert", "ADJ", false)), DefaultConfig("book-after-abandonment"))
	require.NoError(t, err)
	assert.Len(t, got, 1)
	var generated int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='reserviert'`, alice.ID).Scan(&generated)
	require.NoError(t, err)
	assert.Equal(t, 1, generated)
}
