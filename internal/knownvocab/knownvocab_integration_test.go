//go:build integration

package knownvocab

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestImportPostgresIsolationLifecycleAndIdempotency(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
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
	first, err := service.Import(ctx, alice.ID, "de", strings.NewReader("Daß\nHaus\n"))
	if err != nil || first.Imported != 2 {
		t.Fatalf("first import = %+v, %v", first, err)
	}
	second, err := service.Import(ctx, alice.ID, "de", strings.NewReader("Daß\nHaus\n"))
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
	state, err := store.GetVocabularyStateByIdentity(ctx, alice.ID, "de", "dass", "")
	if err != nil || state.State != "known" {
		t.Fatalf("exact state = %+v, %v", state, err)
	}
	wildcardState, err := store.GetVocabularyStateByIdentity(ctx, alice.ID, "de", "haus", "")
	if err != nil || wildcardState.State != "known" {
		t.Fatalf("wildcard state = %+v, %v", wildcardState, err)
	}
	if _, err = store.GetVocabularyStateByIdentity(ctx, bob.ID, "de", "dass", ""); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("bob state error = %v", err)
	}
}
