//go:build integration

package webapp

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

// greekTestDefinition is the synthetic fixture's reviewed Reading-unit
// definition. Tests use it instead of compiling the retired Anabasis value
// into the production binary.
func greekTestDefinition() domain.ReadingUnitDefinition {
	return domain.ReadingUnitDefinition{
		Key:                   "xenophon-anabasis-1",
		Language:              ancientgreek.Language,
		WorkTitle:             "Anabasis",
		WorkAuthor:            "Xenophon",
		EditionLabel:          "Marchant 1904",
		CitationStart:         "1.1.1",
		CitationEnd:           "1.10.19",
		SourceFileName:        "tlg0032.tlg006.perseus-grc2_oga020_src.conllu",
		SourceFileSize:        7438037,
		SourceFileSHA256:      "c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61",
		InterpretationVersion: "oga-agdt-upos/5",
	}
}

// registerGreekDefinition registers one synthetic Reading unit for a learner
// through the persistence seam the retired operator command used.
func registerGreekDefinition(t *testing.T, ctx context.Context, store *persistence.PostgresStore, ownerID string, definition domain.ReadingUnitDefinition) (domain.ReadingUnitBookRegistration, error) {
	t.Helper()
	if _, err := store.PutSupportedLanguage(ctx, ancientgreek.Language, ancientgreek.DisplayName); err != nil {
		return domain.ReadingUnitBookRegistration{}, err
	}
	return store.RegisterReadingUnitBook(ctx, ownerID, definition)
}

// brownsonTestSource is the synthetic Brownson fixture's source identity.
func brownsonTestSource() ancientgreek.BrownsonSource {
	return ancientgreek.BrownsonSource{
		Language:      ancientgreek.Language,
		EditionLabel:  "Brownson (1922)",
		WorkLabel:     "Xen. Anab.",
		FileName:      "tlg0032.tlg006.perseus-eng2.xml",
		FileSize:      1101217,
		FileSHA256:    "f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7",
		CitationStart: "1.1.1",
		CitationEnd:   "1.10.19",
	}
}
