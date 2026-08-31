package domain

import "testing"

func TestBookValidationLanguageState(t *testing.T) {
	if _, err := NewBook("owner", "Metadata only", "manual", LanguageUnknown, ""); err != nil {
		t.Fatalf("unknown language book: %v", err)
	}
	if _, err := NewBook("owner", "Chosen language", "catalog", LanguageChosen, "de"); err != nil {
		t.Fatalf("chosen language book: %v", err)
	}
	for _, book := range []Book{
		{OwnerID: "owner", Title: "Missing tag", MetadataProvenance: "manual", LanguageState: LanguageChosen},
		{OwnerID: "owner", Title: "Unexpected tag", MetadataProvenance: "manual", LanguageState: LanguageUnknown, LanguageTag: "de"},
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
