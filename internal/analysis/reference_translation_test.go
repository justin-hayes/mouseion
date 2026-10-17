package analysis

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/require"
)

func TestReferenceTranslationWriterRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	var nilWriter *ReferenceTranslationWriter
	_, err := nilWriter.WriteReferenceTranslations(ctx, ancientgreek.ReferenceTranslationWriteRequest{OwnerID: "owner-1"})
	require.ErrorContains(t, err, "store is required")

	_, err = NewReferenceTranslationWriter(nil).WriteReferenceTranslations(ctx, ancientgreek.ReferenceTranslationWriteRequest{OwnerID: "owner-1"})
	require.ErrorContains(t, err, "store is required")

	writer := &ReferenceTranslationWriter{store: &persistence.PostgresStore{}}
	_, err = writer.WriteReferenceTranslations(ctx, ancientgreek.ReferenceTranslationWriteRequest{Translations: []ancientgreek.ReferenceTranslation{{Citation: "1.2.17", Text: "x"}}})
	require.ErrorContains(t, err, "owner is required")

	_, err = writer.WriteReferenceTranslations(ctx, ancientgreek.ReferenceTranslationWriteRequest{
		OwnerID: "owner-1",
		Translations: []ancientgreek.ReferenceTranslation{
			{Citation: "1.2.17", Text: "a"},
			{Citation: "1.2.17", Text: "b"},
		},
	})
	require.ErrorContains(t, err, "duplicate reference translation citation")
}
