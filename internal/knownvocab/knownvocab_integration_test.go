//go:build integration

package knownvocab

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func TestImportPostgresIsolationLifecycleAndIdempotency(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("MOUSEION_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://postgres@localhost:5432/mouseion_test?sslmode=disable"
	}
	lockPool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer lockPool.Close()
	conn, err := lockPool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(90420009)`); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(90420009)`) //nolint:errcheck
	if _, err = lockPool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
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
	alice, err := store.CreateUser(ctx, "known-import-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "known-import-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store)
	first, err := service.Import(ctx, alice.ID, "de", strings.NewReader("Daß\tSCONJ\nHaus\n"))
	if err != nil || first.Imported != 2 {
		t.Fatalf("first import = %+v, %v", first, err)
	}
	second, err := service.Import(ctx, alice.ID, "de", strings.NewReader("Daß\tSCONJ\nHaus\n"))
	if err != nil || second.AlreadyKnown != 2 || second.Imported != 0 {
		t.Fatalf("second import = %+v, %v", second, err)
	}
	known, err := store.IsKnownVocabularyIdentity(ctx, alice.ID, "de", "haus", "NOUN")
	if err != nil || !known {
		t.Fatalf("alice wildcard known = %t, %v", known, err)
	}
	for _, check := range []struct{ owner, language string }{{bob.ID, "de"}, {alice.ID, "fr"}} {
		known, checkErr := store.IsKnownVocabularyIdentity(ctx, check.owner, check.language, "haus", "NOUN")
		if checkErr != nil || known {
			t.Fatalf("unexpected known for owner=%s language=%s: %t, %v", check.owner, check.language, known, checkErr)
		}
	}
	state, err := store.GetVocabularyStateByIdentity(ctx, alice.ID, "de", "dass", "SCONJ")
	if err != nil || state.State != "known" {
		t.Fatalf("exact state = %+v, %v", state, err)
	}
	wildcardState, err := store.GetVocabularyStateByIdentity(ctx, alice.ID, "de", "haus", "")
	if err != nil || wildcardState.State != "known" {
		t.Fatalf("wildcard state = %+v, %v", wildcardState, err)
	}
	if _, err = store.GetVocabularyStateByIdentity(ctx, bob.ID, "de", "dass", "SCONJ"); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("bob state error = %v", err)
	}
}
