//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestCatalogueAliasBackfillPersistenceIsScopedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	alice, err := store.CreateUser(ctx, "alias-backfill-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "alias-backfill-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Legacy", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.AddBookAlias(ctx, alice.ID, book.ID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, "legacy-entry"); err != nil {
		t.Fatal(err)
	}
	if err = store.AddBookAlias(ctx, alice.ID, book.ID, domain.AliasStrongBibliographic, "isbn", "978-legacy"); err != nil {
		t.Fatal(err)
	}
	aliceConnection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Alice", URL: "https://alice.example/opds"})
	if err != nil {
		t.Fatal(err)
	}
	bobConnection, err := store.CreateOpdsConnection(ctx, bob.ID, domain.OpdsConnection{Name: "Bob", URL: "https://bob.example/opds"})
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := store.ListUnscopedCatalogueEntryAliases(ctx)
	if err != nil || len(aliases) != 1 || aliases[0].Value != "legacy-entry" {
		t.Fatalf("unscoped aliases=%+v error=%v", aliases, err)
	}
	if err = store.SetCatalogueEntryAliasConnection(ctx, alice.ID, aliases[0].ID, bobConnection.ID); !errors.Is(err, ErrAliasConflict) {
		t.Fatalf("cross-owner assignment error=%v, want %v", err, ErrAliasConflict)
	}
	var strongAliasID string
	if err = store.Pool().QueryRow(ctx, `SELECT id FROM book_aliases WHERE owner_id=$1 AND alias_type=$2`, alice.ID, domain.AliasStrongBibliographic).Scan(&strongAliasID); err != nil {
		t.Fatal(err)
	}
	if err = store.SetCatalogueEntryAliasConnection(ctx, alice.ID, strongAliasID, aliceConnection.ID); !errors.Is(err, ErrAliasConflict) {
		t.Fatalf("strong-bibliographic assignment error=%v, want %v", err, ErrAliasConflict)
	}
	if err = store.SetCatalogueEntryAliasConnection(ctx, alice.ID, aliases[0].ID, aliceConnection.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.SetCatalogueEntryAliasConnection(ctx, alice.ID, aliases[0].ID, aliceConnection.ID); err != nil {
		t.Fatalf("idempotent assignment: %v", err)
	}
	if aliases, err = store.ListUnscopedCatalogueEntryAliases(ctx); err != nil || len(aliases) != 0 {
		t.Fatalf("remaining unscoped aliases=%+v error=%v", aliases, err)
	}
	alias, err := store.GetBookCatalogEntryAlias(ctx, alice.ID, book.ID)
	if err != nil || alias.ConnectionID != aliceConnection.ID {
		t.Fatalf("scoped alias=%+v error=%v", alias, err)
	}
}
