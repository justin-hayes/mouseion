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
	pin := "f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7"
	var nilWriter *ReferenceTranslationWriter
	_, err := nilWriter.WriteReferenceTranslations(ctx, ancientgreek.ReferenceTranslationWriteRequest{TranslationPinSHA256: pin})
	require.ErrorContains(t, err, "store is required")

	_, err = NewReferenceTranslationWriter(nil).WriteReferenceTranslations(ctx, ancientgreek.ReferenceTranslationWriteRequest{TranslationPinSHA256: pin})
	require.ErrorContains(t, err, "store is required")

	writer := &ReferenceTranslationWriter{store: &persistence.PostgresStore{}}
	_, err = writer.WriteReferenceTranslations(ctx, ancientgreek.ReferenceTranslationWriteRequest{
		Language: "grc", Translations: []ancientgreek.ReferenceTranslation{{Citation: "1.2.17", Text: "x"}},
	})
	require.ErrorContains(t, err, "pin is required")

	_, err = writer.WriteReferenceTranslations(ctx, ancientgreek.ReferenceTranslationWriteRequest{
		TranslationPinSHA256: pin,
		Translations:         []ancientgreek.ReferenceTranslation{{Citation: "1.2.17", Text: "x"}},
	})
	require.ErrorContains(t, err, "language is required")

	_, err = writer.WriteReferenceTranslations(ctx, ancientgreek.ReferenceTranslationWriteRequest{
		Language:             "grc",
		TranslationPinSHA256: pin,
		Translations: []ancientgreek.ReferenceTranslation{
			{Citation: "1.2.17", Text: "a"},
			{Citation: "1.2.17", Text: "b"},
		},
	})
	require.ErrorContains(t, err, "duplicate reference translation citation")
}
