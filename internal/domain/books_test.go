package domain

import "testing"

func TestBookValidationLanguageState(t *testing.T) {
	if _, err := NewBook("owner", "Metadata only", MetadataProvenanceCatalogueSync, LanguageUnknown, ""); err != nil {
		t.Fatalf("unknown language book: %v", err)
	}
	if _, err := NewBook("owner", "Chosen language", MetadataProvenanceCatalogueSync, LanguageChosen, "de"); err != nil {
		t.Fatalf("chosen language book: %v", err)
	}
	book, err := NewBook("owner", "Regional language", MetadataProvenanceCatalogueSync, LanguageChosen, "de_DE")
	if err != nil || book.LanguageTag != "de" {
		t.Fatalf("NewBook regional language = %+v, err=%v", book, err)
	}
	for _, book := range []Book{
		{OwnerID: "owner", Title: "Missing tag", MetadataProvenance: MetadataProvenanceCatalogueSync, LanguageState: LanguageChosen},
		{OwnerID: "owner", Title: "Unexpected tag", MetadataProvenance: MetadataProvenanceCatalogueSync, LanguageState: LanguageUnknown, LanguageTag: "de"},
		{OwnerID: "owner", Title: "Non-canonical tag", MetadataProvenance: MetadataProvenanceCatalogueSync, LanguageState: LanguageChosen, LanguageTag: "de-DE"},
	} {
		if err := book.Validate(); err == nil {
			t.Fatalf("invalid book accepted: %+v", book)
		}
	}
}

func TestBookMembershipValidationRemovalState(t *testing.T) {
	if err := (BookMembership{OwnerID: "owner", BookID: "book", State: "active"}).Validate(); err != nil {
		t.Fatalf("active membership: %v", err)
	}
	if err := (BookMembership{OwnerID: "owner", BookID: "book", State: "removed"}).Validate(); err == nil {
		t.Fatal("removed membership without timestamp accepted")
	}
}
