//go:build integration

package persistence

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestMyBooksBrowseFiltersCountsPagingAndOwnership(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	alice, err := store.CreateUser(ctx, "browse-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "browse-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	create := func(owner, title, state, tag, provenance string) domain.Book {
		input, createErr := domain.NewBook(owner, title, provenance, state, tag)
		if createErr != nil {
			t.Fatal(createErr)
		}
		book, createErr := store.CreateBook(ctx, input)
		if createErr != nil {
			t.Fatal(createErr)
		}
		return book
	}

	percent := create(alice.ID, "100% real", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)
	create(alice.ID, "German Upper", domain.LanguageChosen, "DE", domain.MetadataProvenanceCatalogueSync)
	create(alice.ID, "German Lower", domain.LanguageChosen, "de", domain.MetadataProvenanceCatalogueSync)
	metadataDampf := create(alice.ID, "Metadata Donaudampf", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)
	acquiredSource := putBookSource(t, ctx, store, alice.ID, "browse-acquired", "Acquired Donaudampf", []byte("browse-content"), "browse content")
	acquiredID, err := store.ResolveOrCreateBookForAcquisition(ctx, alice.ID, acquiredSource.SourceIdentifier, acquiredSource.Language, acquiredSource.Title)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.LinkSourceToBook(ctx, alice.ID, acquiredID, acquiredSource.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 26; i++ {
		create(alice.ID, fmt.Sprintf("Page %02d", i), domain.LanguageChosen, "it", domain.MetadataProvenanceCatalogueSync)
	}
	tieA := create(alice.ID, "Same title", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)
	tieB := create(alice.ID, "Same title", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)
	create(bob.ID, "Metadata Donaudampf", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)

	result, err := store.ListMyBooksBrowse(ctx, alice.ID, "Donaudampf", "", 0, 25)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || len(result.Items) != 2 || result.AllCount != 33 {
		t.Fatalf("Donaudampf browse result=%+v", result)
	}
	for _, item := range result.Items {
		if item.Book.OwnerID != alice.ID {
			t.Fatalf("cross-owner item leaked: %+v", item)
		}
	}

	literal, err := store.ListMyBooksBrowse(ctx, alice.ID, "%", "", 0, 25)
	if err != nil {
		t.Fatal(err)
	}
	if literal.Total != 1 || literal.Items[0].Book.ID != percent.ID {
		t.Fatalf("LIKE wildcard was not literal: %+v", literal)
	}

	deResult, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "DE", 0, 25)
	if err != nil {
		t.Fatal(err)
	}
	if deResult.Total != 3 {
		t.Fatalf("German filter total=%d want 3", deResult.Total)
	}
	if len(deResult.Counts) != 3 || deResult.Counts[0].Tag != "de" || deResult.Counts[0].Count != 3 || deResult.Counts[1].Tag != "it" || deResult.Counts[1].Count != 26 || deResult.Counts[2].Tag != "unknown" || deResult.Counts[2].Count != 4 {
		t.Fatalf("full collection language counts=%+v", deResult.Counts)
	}
	unknown, err := store.ListMyBooksBrowse(ctx, alice.ID, "Donaudampf", domain.LanguageUnknown, 0, 25)
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Total != 1 || unknown.Items[0].Book.ID != metadataDampf.ID {
		t.Fatalf("unknown combined filter=%+v", unknown)
	}

	page, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "", 25, 25)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 33 || len(page.Items) != 8 {
		t.Fatalf("page slice total=%d items=%d", page.Total, len(page.Items))
	}
	tieIDs := []string{tieA.ID, tieB.ID}
	orderedTies, err := store.ListMyBooksBrowse(ctx, alice.ID, "Same title", "", 0, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(orderedTies.Items) != 2 || orderedTies.Items[0].Book.ID != minString(tieIDs[0], tieIDs[1]) || orderedTies.Items[1].Book.ID != maxString(tieIDs[0], tieIDs[1]) {
		t.Fatalf("tie ordering=%v", []string{orderedTies.Items[0].Book.ID, orderedTies.Items[1].Book.ID})
	}

	if err = store.RemoveBookFromMyBooks(ctx, alice.ID, percent.ID); err != nil {
		t.Fatal(err)
	}
	remaining, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "", 0, 25)
	if err != nil {
		t.Fatal(err)
	}
	if remaining.AllCount != 32 || remaining.Total != 32 || strings.Contains(fmt.Sprint(remaining.Items), percent.ID) {
		t.Fatalf("removed membership remained in browse=%+v", remaining)
	}
}

func minString(left, right string) string {
	if left < right {
		return left
	}
	return right
}

func maxString(left, right string) string {
	if left > right {
		return left
	}
	return right
}
