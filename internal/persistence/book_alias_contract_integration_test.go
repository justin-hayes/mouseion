//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestBookAliasConnectionContract(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, err := store.CreateUser(ctx, "alias-contract-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Contract", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.AddBookAlias(ctx, owner.ID, book.ID, domain.AliasStrongBibliographic, "isbn", "978-contract"); err != nil {
		t.Fatal(err)
	}
	if err = store.AddBookAlias(ctx, owner.ID, book.ID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, "connection-required"); err == nil {
		t.Fatal("connectionless catalogue alias was accepted")
	}
	var aliasCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1`, owner.ID).Scan(&aliasCount); err != nil {
		t.Fatal(err)
	}
	if aliasCount != 1 {
		t.Fatalf("alias count=%d, want 1", aliasCount)
	}
	if err = store.SetCatalogueEntryAliasConnection(ctx, owner.ID, "00000000-0000-0000-0000-000000000000", "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing alias assignment error=%v", err)
	}
}
