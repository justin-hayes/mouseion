//go:build integration

package vocabulary

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestLifecyclePersistsAcrossCorporaAndIsolatesUsers(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, err := store.CreateUser(ctx, "lifecycle-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "lifecycle-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := NewLifecycle(store)
	id := Identity{"de", "Haus", "NOUN"}
	if _, err = lifecycle.Transition(ctx, alice.ID, id, Candidate); err != nil {
		t.Fatal(err)
	}
	if _, err = lifecycle.Transition(ctx, alice.ID, id, Ignored); err != nil {
		t.Fatal(err)
	}
	// Rediscovery from another corpus is represented by requesting candidate and is suppressed.
	if _, err = lifecycle.Transition(ctx, alice.ID, id, Candidate); err == nil {
		t.Fatal("ignored item implicitly returned as candidate")
	}
	if _, err = lifecycle.Transition(ctx, bob.ID, id, Candidate); err != nil {
		t.Fatal(err)
	}
	aliceState, err := store.GetVocabularyStateByIdentity(ctx, alice.ID, "de", "Haus", "NOUN")
	if err != nil {
		t.Fatal(err)
	}
	bobState, err := store.GetVocabularyStateByIdentity(ctx, bob.ID, "de", "Haus", "NOUN")
	if err != nil {
		t.Fatal(err)
	}
	if aliceState.State != "ignored" || bobState.State != "candidate" {
		t.Fatalf("states leaked: alice=%s bob=%s", aliceState.State, bobState.State)
	}
	if _, err = lifecycle.Reset(ctx, alice.ID, id); err != nil {
		t.Fatal(err)
	}
	if _, err = lifecycle.Transition(ctx, alice.ID, id, Accepted); err != nil {
		t.Fatal(err)
	}
	if _, err = lifecycle.Transition(ctx, alice.ID, id, Generated); err != nil {
		t.Fatal(err)
	}
	var auditCount int
	auditPool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer auditPool.Close()
	if err = auditPool.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='vocabulary.transition'`, alice.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 5 {
		t.Fatalf("audit rows = %d, want 5", auditCount)
	}
}
