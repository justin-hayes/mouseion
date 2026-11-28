package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookValidationLanguageState(t *testing.T) {
	for _, book := range []Book{
		{OwnerID: "owner", Title: "Metadata only", MetadataProvenance: MetadataProvenanceCatalogueSync, LanguageState: LanguageUnknown},
		{OwnerID: "owner", Title: "Chosen language", MetadataProvenance: MetadataProvenanceCatalogueSync, LanguageState: LanguageChosen, LanguageTag: "de"},
	} {
		require.NoError(t, book.Validate(), "valid book rejected: %+v", book)
	}
	for _, book := range []Book{
		{OwnerID: "owner", Title: "Missing tag", MetadataProvenance: MetadataProvenanceCatalogueSync, LanguageState: LanguageChosen},
		{OwnerID: "owner", Title: "Unexpected tag", MetadataProvenance: MetadataProvenanceCatalogueSync, LanguageState: LanguageUnknown, LanguageTag: "de"},
		{OwnerID: "owner", Title: "Non-canonical tag", MetadataProvenance: MetadataProvenanceCatalogueSync, LanguageState: LanguageChosen, LanguageTag: "de-DE"},
	} {
		assert.Error(t, book.Validate(), "invalid book accepted: %+v", book)
	}
}
