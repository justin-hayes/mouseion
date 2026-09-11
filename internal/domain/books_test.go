package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookValidationLanguageState(t *testing.T) {
	_, err := NewBook("owner", "Metadata only", MetadataProvenanceCatalogueSync, LanguageUnknown, "")
	require.NoError(t, err, "unknown language book")
	_, err = NewBook("owner", "Chosen language", MetadataProvenanceCatalogueSync, LanguageChosen, "de")
	require.NoError(t, err, "chosen language book")
	book, err := NewBook("owner", "Regional language", MetadataProvenanceCatalogueSync, LanguageChosen, "de_DE")
	require.NoError(t, err)
	assert.Equal(t, "de", book.LanguageTag)
	for _, book := range []Book{
		{OwnerID: "owner", Title: "Missing tag", MetadataProvenance: MetadataProvenanceCatalogueSync, LanguageState: LanguageChosen},
		{OwnerID: "owner", Title: "Unexpected tag", MetadataProvenance: MetadataProvenanceCatalogueSync, LanguageState: LanguageUnknown, LanguageTag: "de"},
		{OwnerID: "owner", Title: "Non-canonical tag", MetadataProvenance: MetadataProvenanceCatalogueSync, LanguageState: LanguageChosen, LanguageTag: "de-DE"},
	} {
		assert.Error(t, book.Validate(), "invalid book accepted: %+v", book)
	}
}

func TestBookMembershipValidationRemovalState(t *testing.T) {
	require.NoError(t, (BookMembership{OwnerID: "owner", BookID: "book", State: "active"}).Validate(), "active membership")
	assert.Error(t, (BookMembership{OwnerID: "owner", BookID: "book", State: "removed"}).Validate(), "removed membership without timestamp accepted")
}
