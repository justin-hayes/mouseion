//go:build integration

package persistence

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestCatalogueMetadataRefreshPreservesAcquiredEvidence(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, err := store.CreateUser(ctx, "refresh-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Refresh catalog", URL: "https://catalog.example/opds"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateUser(ctx, "refresh-other-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	source := putBookSource(t, ctx, store, owner.ID, "refresh-entry", "Old title", []byte("acquired content"), "readable text")
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: source.Title, MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	bookID := book.ID
	if err = store.LinkSourceToBook(ctx, owner.ID, bookID, source.ID); err != nil {
		t.Fatal(err)
	}

	beforeSource, err := store.GetSourceMaterial(ctx, owner.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	var beforeRevisions, beforeAliases, beforeMemberships int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID).Scan(&beforeRevisions); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&beforeAliases); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_membership WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&beforeMemberships); err != nil {
		t.Fatal(err)
	}

	result, err := store.ReconcileCatalogueEntry(ctx, owner.ID, connection.ID, source.SourceIdentifier, "New title", source.Language)
	if err != nil || !result.TitleChanged || result.Book.Title != "New title" {
		t.Fatalf("refresh result=%+v err=%v", result, err)
	}
	alias, err := store.GetBookCatalogEntryAlias(ctx, owner.ID, bookID)
	if err != nil || alias.ConnectionID != connection.ID {
		t.Fatalf("catalogue alias=%+v err=%v", alias, err)
	}
	afterSource, err := store.GetSourceMaterial(ctx, owner.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	var afterRevisions, afterAliases, afterMemberships int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID).Scan(&afterRevisions); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&afterAliases); err != nil {
		t.Fatal(err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_membership WHERE owner_id=$1 AND book_id=$2`, owner.ID, bookID).Scan(&afterMemberships); err != nil {
		t.Fatal(err)
	}
	if beforeSource.Title != afterSource.Title || beforeSource.ContentHash != afterSource.ContentHash || beforeSource.ContentDigest != afterSource.ContentDigest || beforeSource.ContentRevisionID != afterSource.ContentRevisionID || !bytes.Equal(beforeSource.Content, afterSource.Content) || beforeRevisions != afterRevisions || beforeAliases+1 != afterAliases || beforeMemberships != afterMemberships {
		t.Fatalf("refresh changed acquired evidence before=%+v/%d/%d/%d after=%+v/%d/%d/%d", beforeSource, beforeRevisions, beforeAliases, beforeMemberships, afterSource, afterRevisions, afterAliases, afterMemberships)
	}

	if _, err = store.GetBookCatalogEntryAlias(ctx, other.ID, bookID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner alias lookup err=%v", err)
	}
	if _, err = store.GetBook(ctx, other.ID, bookID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner book lookup err=%v", err)
	}
}
