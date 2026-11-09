package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
)

func validClassicalSourceConversion() domain.ClassicalSourceConversion {
	return domain.ClassicalSourceConversion{
		OwnerID:                   "owner",
		BookID:                    "book",
		SourceMaterialID:          "source",
		ReadingUnitKey:            "xenophon-anabasis-1",
		UnitID:                    "classical-unit-v1:xenophon-anabasis-1",
		UnitText:                  "λέγει",
		SourceFileName:            "anabasis.conllu",
		DefinitionRevision:        1,
		SourceFileSHA256:          "c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61",
		InterpretationVersion:     "oga-agdt-upos/5",
		AnalysisRunID:             "run",
		ExpectedContentRevisionID: "revision",
		ExpectedSnapshotID:        "snapshot",
	}
}

func TestValidateClassicalSourceConversion(t *testing.T) {
	t.Run("complete conversion", func(t *testing.T) {
		require.NoError(t, validateClassicalSourceConversion(validClassicalSourceConversion()))
	})

	tests := []struct {
		name    string
		mutate  func(*domain.ClassicalSourceConversion)
		message string
	}{
		{"owner", func(c *domain.ClassicalSourceConversion) { c.OwnerID = "" }, "owner is required"},
		{"Book", func(c *domain.ClassicalSourceConversion) { c.BookID = "" }, "Book is required"},
		{"source", func(c *domain.ClassicalSourceConversion) { c.SourceMaterialID = "" }, "source is required"},
		{"Reading unit", func(c *domain.ClassicalSourceConversion) { c.ReadingUnitKey = "" }, "Reading unit is required"},
		{"unit", func(c *domain.ClassicalSourceConversion) { c.UnitID = "" }, "unit is required"},
		{"text", func(c *domain.ClassicalSourceConversion) { c.UnitText = "" }, "text is required"},
		{"revision", func(c *domain.ClassicalSourceConversion) { c.DefinitionRevision = 0 }, "revision must be positive"},
		{"source SHA-256", func(c *domain.ClassicalSourceConversion) { c.SourceFileSHA256 = "short" }, "SHA-256 is invalid"},
		{"interpretation", func(c *domain.ClassicalSourceConversion) { c.InterpretationVersion = "" }, "interpretation version is required"},
		{"analysis run", func(c *domain.ClassicalSourceConversion) { c.AnalysisRunID = "" }, "analysis run is required"},
		{"expected revision", func(c *domain.ClassicalSourceConversion) { c.ExpectedContentRevisionID = "" }, "expected evidence is required"},
		{"expected snapshot", func(c *domain.ClassicalSourceConversion) { c.ExpectedSnapshotID = "" }, "expected evidence is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conversion := validClassicalSourceConversion()
			tt.mutate(&conversion)
			require.ErrorContains(t, validateClassicalSourceConversion(conversion), tt.message)
		})
	}
}
