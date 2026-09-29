//go:build integration

package selection_test

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectionPersistsProvenanceAndIsolatesOwners(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "selection store", store.Close)
	alice, err := store.CreateUser(ctx, "selection-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "selection-bob", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "reserved-book", Title: "Reserved", MediaType: "text/plain", ContentHash: "reserved-book", Content: []byte("reserviert"), FullText: "reserviert"})
	require.NoError(t, err)
	deck, err := store.PutDeck(ctx, alice.ID, "de", "Reserved")
	require.NoError(t, err)
	_, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: "reserviert", UPOS: "ADJ", FirstDeckID: deck.ID, FirstSourceMaterialID: &source.ID})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES ($1,'de','alt','ADJ','ignored'),($1,'de','legacy','NOUN','generated')`, alice.ID)
	require.NoError(t, err)
	corpus := selectionFixture(selectionToken("Häuser", "Haus", "NOUN"), selectionToken("Haus", "Haus", "NOUN"), selectionToken("alt", "alt", "ADJ"), selectionToken("alt", "alt", "ADJ"), selectionToken("legacy", "legacy", "NOUN"), selectionToken("reserviert", "reserviert", "ADJ"))
	svc := selection.NewService(store)
	got, err := svc.Select(ctx, alice.ID, corpus, selection.DefaultConfig("book-a"))
	require.NoError(t, err)
	assert.Len(t, got, 4)
	got, err = svc.Select(ctx, bob.ID, corpus, selection.DefaultConfig("book-b"))
	require.NoError(t, err)
	assert.Len(t, got, 4)
	var aliceCount, bobCount int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1`, alice.ID).Scan(&aliceCount)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1`, bob.ID).Scan(&bobCount)
	require.NoError(t, err)
	assert.Equal(t, 4, aliceCount)
	assert.Equal(t, 4, bobCount)
	var occurrences int
	var forms, refs, provenance []byte
	err = pool.QueryRow(ctx, `SELECT occurrence_count,observed_forms,eligible_sentence_refs,provenance FROM selection_candidates WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&occurrences, &forms, &refs, &provenance)
	require.NoError(t, err)
	assert.Equal(t, 2, occurrences)
	assert.NotEmpty(t, forms)
	assert.NotEmpty(t, refs)
	assert.NotEmpty(t, provenance)
	var states int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM vocabulary_states WHERE owner_id=$1`, alice.ID).Scan(&states)
	require.NoError(t, err)
	assert.Equal(t, 2, states, "selection must not create legacy lifecycle rows")
	got, err = svc.Select(ctx, alice.ID, selectionFixture(selectionToken("reserviert", "reserviert", "ADJ")), selection.DefaultConfig("book-after-abandonment"))
	require.NoError(t, err)
	assert.Len(t, got, 1)
	var generated int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='reserviert'`, alice.ID).Scan(&generated)
	require.NoError(t, err)
	assert.Equal(t, 1, generated)
}

func selectionToken(surface, lemma, upos string) analyzer.Token {
	return analyzer.Token{Surface: surface, RawLemma: lemma, CanonicalLemma: lemma, UPOS: upos}
}

func selectionFixture(tokens ...analyzer.Token) analyzer.Result {
	return analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{{Tokens: tokens}}}
}
