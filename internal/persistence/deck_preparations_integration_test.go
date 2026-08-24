//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestDeckPreparationPersistence(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	alice, err := store.CreateUser(ctx, "prep-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "prep-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "prep-book", Title: "Book", MediaType: "text/plain", ContentHash: "prep-hash", Content: []byte("Buch"), FullText: "Buch"})
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: alice.ID, SourceMaterialID: source.ID, Filename: "book.apkg", DeckName: "Mouseion::de::Book", ContentHash: source.ContentHash})
	if err != nil || created.State != domain.DeckPreparationQueued {
		t.Fatalf("create: %+v, %v", created, err)
	}
	repeated, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: alice.ID, SourceMaterialID: source.ID, Filename: "changed.apkg", DeckName: "changed", ContentHash: source.ContentHash})
	if err != nil || repeated.ID != created.ID || repeated.Filename != created.Filename {
		t.Fatalf("idempotent create: %+v, %v", repeated, err)
	}
	if _, err = store.GetDeckPreparation(ctx, bob.ID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner status: %v", err)
	}
	if _, err = store.DownloadDeckPreparation(ctx, alice.ID, created.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("queued download: %v", err)
	}
	if _, err = store.DownloadDeckPreparation(ctx, bob.ID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner download: %v", err)
	}

	claimed, err := store.ClaimDeckPreparation(ctx, alice.ID, created.ID)
	if err != nil || claimed.State != domain.DeckPreparationPreparing || claimed.StartedAt == nil {
		t.Fatalf("claim: %+v, %v", claimed, err)
	}
	if _, err = store.ClaimDeckPreparation(ctx, alice.ID, created.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("second claim: %v", err)
	}
	readyInput := domain.DeckPreparation{Artifact: []byte("apkg"), Filename: "book.apkg", DeckName: "Mouseion::de::Book", TotalCards: 4, CardsWithEnglish: 3, CardsWithContextualSentenceTranslations: 2, QualityOmissions: 1}
	ready, err := store.CompleteDeckPreparation(ctx, alice.ID, created.ID, readyInput)
	if err != nil || ready.State != domain.DeckPreparationReady || ready.CompletedAt == nil {
		t.Fatalf("complete: %+v, %v", ready, err)
	}
	downloaded, err := store.DownloadDeckPreparation(ctx, alice.ID, created.ID)
	if err != nil || string(downloaded.Artifact) != "apkg" || downloaded.TotalCards != 4 {
		t.Fatalf("download: %+v, %v", downloaded, err)
	}
	if _, err = store.CompleteDeckPreparation(ctx, alice.ID, created.ID, readyInput); err != nil {
		t.Fatalf("idempotent complete: %v", err)
	}
	changed := readyInput
	changed.Artifact = []byte("different")
	if _, err = store.CompleteDeckPreparation(ctx, alice.ID, created.ID, changed); !errors.Is(err, ErrImmutable) {
		t.Fatalf("mutate ready artifact: %v", err)
	}
	if _, err = store.RetryDeckPreparation(ctx, alice.ID, created.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("retry ready: %v", err)
	}

	failed := createPreparation(t, ctx, store, alice.ID, source.ID, "failed-hash")
	if _, err = store.ClaimDeckPreparation(ctx, alice.ID, failed.ID); err != nil {
		t.Fatal(err)
	}
	failed, err = store.FailDeckPreparation(ctx, alice.ID, failed.ID, "provider unavailable")
	if err != nil || failed.State != domain.DeckPreparationFailed || failed.Error == "" {
		t.Fatalf("fail: %+v, %v", failed, err)
	}
	retried, err := store.RetryDeckPreparation(ctx, alice.ID, failed.ID)
	if err != nil || retried.State != domain.DeckPreparationQueued || retried.Error != "" || retried.StartedAt != nil || retried.CompletedAt != nil {
		t.Fatalf("retry: %+v, %v", retried, err)
	}
	cancelled, err := store.CancelDeckPreparation(ctx, alice.ID, retried.ID)
	if err != nil || cancelled.State != domain.DeckPreparationCancelled {
		t.Fatalf("cancel: %+v, %v", cancelled, err)
	}
	if _, err = store.RetryDeckPreparation(ctx, alice.ID, cancelled.ID); err != nil {
		t.Fatalf("retry cancelled: %v", err)
	}

	var generated int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1`, alice.ID).Scan(&generated); err != nil || generated != 0 {
		t.Fatalf("terminal preparations created exclusions: count=%d err=%v", generated, err)
	}
}

func TestCompletePreparedDeckAtomicallyPersistsArtifactAndProvenance(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, err := store.CreateUser(ctx, "atomic-prep", false)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "atomic-book", Title: "Atomic Book", MediaType: "text/plain", ContentHash: "atomic-hash", Content: []byte("Haus"), FullText: "Haus"})
	if err != nil {
		t.Fatal(err)
	}
	for _, lemma := range []string{"Haus", "Baum"} {
		if _, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES($1,'de',$2,'NOUN','candidate')`, owner.ID, lemma); err != nil {
			t.Fatal(err)
		}
	}
	p := createPreparation(t, ctx, store, owner.ID, source.ID, source.ContentHash)
	if _, err = store.ClaimDeckPreparation(ctx, owner.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	record := func(lemma string) cardexport.GeneratedRecord {
		return cardexport.GeneratedRecord{Entry: cardexport.Entry{Language: "de", CanonicalLemma: lemma, UPOS: "NOUN"}, Note: cardexport.Note{Key: lemma, Text: lemma + " front", BackExtra: lemma + " back", BookTitle: "Atomic Book"}}
	}
	bad := cardexport.Artifact{APKG: []byte("bad"), Filename: "bad.apkg", DeckName: "Mouseion::de::Atomic Book", Generated: []cardexport.GeneratedRecord{record("Haus"), record("Missing")}, Completeness: cardexport.Completeness{TotalCards: 2}}
	if _, err = store.CompletePreparedDeck(ctx, owner.ID, p.ID, bad); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected rollback error, got %v", err)
	}
	var cards, generated int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, owner.ID).Scan(&cards); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&generated); err != nil {
		t.Fatal(err)
	}
	if cards != 0 || generated != 0 {
		t.Fatalf("partial provenance after failure: cards=%d generated=%d", cards, generated)
	}
	artifact := cardexport.Artifact{APKG: []byte("apkg"), Filename: "atomic.apkg", DeckName: "Mouseion::de::Atomic Book", Generated: []cardexport.GeneratedRecord{record("Haus"), record("Baum")}, Completeness: cardexport.Completeness{TotalCards: 2, CardsWithEnglish: 2, CardsWithEnglishSentence: 1}}
	ready, err := store.CompletePreparedDeck(ctx, owner.ID, p.ID, artifact)
	if err != nil || ready.State != domain.DeckPreparationReady || ready.TotalCards != 2 {
		t.Fatalf("complete: %+v %v", ready, err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, owner.ID).Scan(&cards); err != nil || cards != 2 {
		t.Fatalf("cards=%d err=%v", cards, err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND first_source_material_id=$2`, owner.ID, source.ID).Scan(&generated); err != nil || generated != 2 {
		t.Fatalf("generated=%d err=%v", generated, err)
	}
	if _, err = store.CompletePreparedDeck(ctx, owner.ID, p.ID, artifact); err != nil {
		t.Fatalf("idempotent completion: %v", err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, owner.ID).Scan(&cards); err != nil || cards != 2 {
		t.Fatalf("duplicate cards=%d err=%v", cards, err)
	}
}

func createPreparation(t *testing.T, ctx context.Context, store *PostgresStore, owner, source, hash string) domain.DeckPreparation {
	t.Helper()
	p, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner, SourceMaterialID: source, Filename: hash + ".apkg", DeckName: hash, ContentHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
