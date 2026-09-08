//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestAddToReadingJourneyRejectsBookWithoutChosenLanguage(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, err := store.CreateUser(ctx, "journey-language-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Awaiting language", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	journey, err := store.GetReadingJourney(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddToReadingJourney(ctx, owner.ID, book.ID, journey.Revision); !errors.Is(err, ErrBookLanguageRequired) {
		t.Fatalf("unknown-language Journey add error=%v", err)
	}
	unchanged, err := store.GetReadingJourney(ctx, owner.ID)
	if err != nil || len(unchanged.Entries) != 0 {
		t.Fatalf("unknown-language add changed Journey=%+v err=%v", unchanged, err)
	}
	chosen, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Chosen language", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddToReadingJourney(ctx, owner.ID, chosen.ID, unchanged.Revision); err != nil {
		t.Fatalf("chosen-language Journey add error=%v", err)
	}
	if _, err = store.pool.Exec(ctx, `INSERT INTO reading_journey_membership(owner_id,book_id,position) VALUES($1,$2,$3)`, owner.ID, book.ID, 2); err != nil {
		t.Fatal(err)
	}
	filtered, err := store.GetReadingJourney(ctx, owner.ID)
	if err != nil || len(filtered.Entries) != 1 || filtered.Entries[0].BookID != chosen.ID {
		t.Fatalf("unknown-language membership was visible Journey=%+v err=%v", filtered, err)
	}
}
