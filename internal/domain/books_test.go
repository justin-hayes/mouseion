package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBookValidationLanguageState(t *testing.T) {
	const provenance = MetadataProvenanceCatalogueSync
	valid := []struct {
		name string
		book Book
	}{
		{"unknown language", Book{OwnerID: "owner", Title: "Metadata only", MetadataProvenance: provenance, LanguageState: LanguageUnknown}},
		{"chosen language", Book{OwnerID: "owner", Title: "Chosen language", MetadataProvenance: provenance, LanguageState: LanguageChosen, LanguageTag: "de"}},
	}
	for _, tt := range valid {
		t.Run("accepts "+tt.name, func(t *testing.T) {
			require.NoError(t, tt.book.Validate())
		})
	}
	invalid := []struct {
		name string
		book Book
	}{
		{"missing tag", Book{OwnerID: "owner", Title: "Missing tag", MetadataProvenance: provenance, LanguageState: LanguageChosen}},
		{"unexpected tag", Book{OwnerID: "owner", Title: "Unexpected tag", MetadataProvenance: provenance, LanguageState: LanguageUnknown, LanguageTag: "de"}},
		{"non-canonical tag", Book{OwnerID: "owner", Title: "Non-canonical tag", MetadataProvenance: provenance, LanguageState: LanguageChosen, LanguageTag: "de-DE"}},
	}
	for _, tt := range invalid {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			require.Error(t, tt.book.Validate())
		})
	}
}
