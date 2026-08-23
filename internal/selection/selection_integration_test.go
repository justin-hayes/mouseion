//go:build integration

package selection

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func TestSelectionPersistsProvenanceAndIsolatesOwners(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("MOUSEION_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://postgres@localhost:5432/mouseion_test?sslmode=disable"
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(90420009)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(90420009)`)
		conn.Release()
		pool.Close()
	})
	if _, err = conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err = persistence.Migrate(url); err != nil {
		t.Fatal(err)
	}
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, err := store.CreateUser(ctx, "selection-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "selection-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutVocabularyState(ctx, alice.ID, "de", "alt", "ADJ", "ignored"); err != nil {
		t.Fatal(err)
	}
	corpus := fixture(tok("Häuser", "Haus", "NOUN", false), tok("Haus", "Haus", "NOUN", false), tok("alt", "alt", "ADJ", false), tok("alt", "alt", "ADJ", false))
	svc := NewService(store)
	got, err := svc.Select(ctx, alice.ID, corpus, DefaultConfig("book-a"))
	if err != nil || len(got) != 1 {
		t.Fatalf("alice candidates=%+v err=%v", got, err)
	}
	got, err = svc.Select(ctx, bob.ID, corpus, DefaultConfig("book-b"))
	if err != nil || len(got) != 2 {
		t.Fatalf("bob candidates=%+v err=%v", got, err)
	}
	var aliceCount, bobCount int
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1`, alice.ID).Scan(&aliceCount); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1`, bob.ID).Scan(&bobCount); err != nil {
		t.Fatal(err)
	}
	if aliceCount != 1 || bobCount != 2 {
		t.Fatalf("persisted alice=%d bob=%d", aliceCount, bobCount)
	}
	var occurrences int
	var forms, refs, provenance []byte
	if err = conn.QueryRow(ctx, `SELECT occurrence_count,observed_forms,eligible_sentence_refs,provenance FROM selection_candidates WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&occurrences, &forms, &refs, &provenance); err != nil {
		t.Fatal(err)
	}
	if occurrences != 2 || len(forms) == 0 || len(refs) == 0 || len(provenance) == 0 {
		t.Fatalf("provenance occurrences=%d forms=%s refs=%s provenance=%s", occurrences, forms, refs, provenance)
	}
	state, err := store.GetVocabularyStateByIdentity(ctx, alice.ID, "de", "Haus", "NOUN")
	if err != nil || state.State != "candidate" {
		t.Fatalf("state=%+v err=%v", state, err)
	}
}
