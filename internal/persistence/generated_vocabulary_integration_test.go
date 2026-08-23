//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestGeneratedVocabularyFirstProvenanceAndOwnerIsolation(t *testing.T) {
	ctx := context.Background()
	url := integrationDatabase(t, ctx)
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	alice, err := store.CreateUser(ctx, "generated-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "generated-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	aliceDeck, err := store.PutDeck(ctx, alice.ID, "de", "First deck")
	if err != nil {
		t.Fatal(err)
	}
	aliceSecondDeck, err := store.PutDeck(ctx, alice.ID, "de", "Second deck")
	if err != nil {
		t.Fatal(err)
	}
	bobDeck, err := store.PutDeck(ctx, bob.ID, "de", "Bob deck")
	if err != nil {
		t.Fatal(err)
	}
	aliceSource, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "alice-first", Title: "First book", MediaType: "text/plain", ContentHash: "alice-first", Content: []byte("first"), FullText: "first"})
	if err != nil {
		t.Fatal(err)
	}
	aliceSecondSource, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "alice-second", Title: "Second book", MediaType: "text/plain", ContentHash: "alice-second", Content: []byte("second"), FullText: "second"})
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", FirstDeckID: aliceDeck.ID, FirstSourceMaterialID: &aliceSource.ID})
	if err != nil {
		t.Fatal(err)
	}
	if first.FirstDeckID != aliceDeck.ID || first.FirstSourceMaterialID == nil || *first.FirstSourceMaterialID != aliceSource.ID || first.FirstGeneratedAt.IsZero() {
		t.Fatalf("first record = %+v", first)
	}
	repeated, err := store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", FirstDeckID: aliceSecondDeck.ID, FirstSourceMaterialID: &aliceSecondSource.ID})
	if err != nil {
		t.Fatal(err)
	}
	if repeated.FirstDeckID != first.FirstDeckID || repeated.FirstSourceMaterialID == nil || *repeated.FirstSourceMaterialID != *first.FirstSourceMaterialID || !repeated.FirstGeneratedAt.Equal(first.FirstGeneratedAt) {
		t.Fatalf("repeat changed first provenance: first=%+v repeated=%+v", first, repeated)
	}
	if _, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: bob.ID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", FirstDeckID: bobDeck.ID}); err != nil {
		t.Fatal(err)
	}
	aliceWords, err := store.ListGeneratedVocabulary(ctx, alice.ID, "de")
	if err != nil || len(aliceWords) != 1 || aliceWords[0].OwnerID != alice.ID {
		t.Fatalf("alice list = %+v, err=%v", aliceWords, err)
	}
	bobWords, err := store.ListGeneratedVocabulary(ctx, bob.ID, "de")
	if err != nil || len(bobWords) != 1 || bobWords[0].OwnerID != bob.ID || bobWords[0].FirstSourceMaterialID != nil {
		t.Fatalf("bob list = %+v, err=%v", bobWords, err)
	}

	if _, err = store.Pool().Exec(ctx, `DELETE FROM source_materials WHERE owner_id=$1 AND id=$2`, alice.ID, aliceSource.ID); err != nil {
		t.Fatal(err)
	}
	aliceWords, err = store.ListGeneratedVocabulary(ctx, alice.ID, "de")
	if err != nil || aliceWords[0].FirstSourceMaterialID != nil {
		t.Fatalf("source deletion did not clear provenance: %+v, err=%v", aliceWords, err)
	}
}
