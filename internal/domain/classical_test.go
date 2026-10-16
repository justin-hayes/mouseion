package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validDefinition() ReadingUnitDefinition {
	return ReadingUnitDefinition{
		Key:                   "example-1",
		Language:              "grc",
		WorkTitle:             "Anabasis",
		WorkAuthor:            "Xenophon",
		EditionLabel:          "Marchant 1904",
		CitationStart:         "1.1.1",
		CitationEnd:           "1.10.19",
		SourceFileName:        "example.conllu",
		SourceFileSize:        7438037,
		SourceFileSHA256:      "c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61",
		InterpretationVersion: "oga-agdt-upos/4",
	}
}

func TestReadingUnitDefinitionValidate(t *testing.T) {
	t.Parallel()
	require.NoError(t, validDefinition().Validate(), "complete definition")
	missing := func(mutate func(*ReadingUnitDefinition)) ReadingUnitDefinition {
		definition := validDefinition()
		mutate(&definition)
		return definition
	}
	for name, definition := range map[string]ReadingUnitDefinition{
		"missing key":      missing(func(d *ReadingUnitDefinition) { d.Key = "" }),
		"missing language": missing(func(d *ReadingUnitDefinition) { d.Language = "" }),
		"padded language":  missing(func(d *ReadingUnitDefinition) { d.Language = " grc" }),
		"non-canonical":    missing(func(d *ReadingUnitDefinition) { d.Language = "GRC" }),
		"missing title":    missing(func(d *ReadingUnitDefinition) { d.WorkTitle = "" }),
		"missing author":   missing(func(d *ReadingUnitDefinition) { d.WorkAuthor = "" }),
		"missing edition":  missing(func(d *ReadingUnitDefinition) { d.EditionLabel = "" }),
		"missing citation": missing(func(d *ReadingUnitDefinition) { d.CitationStart = "" }),
		"missing end":      missing(func(d *ReadingUnitDefinition) { d.CitationEnd = "" }),
		"missing file":     missing(func(d *ReadingUnitDefinition) { d.SourceFileName = "" }),
		"zero size":        missing(func(d *ReadingUnitDefinition) { d.SourceFileSize = 0 }),
		"short sha":        missing(func(d *ReadingUnitDefinition) { d.SourceFileSHA256 = "abc" }),
		"uppercase sha": missing(func(d *ReadingUnitDefinition) {
			d.SourceFileSHA256 = "C51BBA2DF6E80E2EB8DD2543D00CF6D6E6B8C65CC9ED03C7D27176FED063EB61"
		}),
		"missing interpret":  missing(func(d *ReadingUnitDefinition) { d.InterpretationVersion = "" }),
		"non-hex sha suffix": missing(func(d *ReadingUnitDefinition) { d.SourceFileSHA256 = d.SourceFileSHA256[:63] + "g" }),
	} {
		assert.Error(t, definition.Validate(), "invalid definition accepted: %s", name)
	}
}

func TestMyBookAwaitingPreparation(t *testing.T) {
	t.Parallel()
	ordinary := MyBook{Book: Book{LanguageState: LanguageChosen, LanguageTag: "de"}}
	assert.False(t, ordinary.AwaitingPreparation(), "ordinary book never awaits preparation")

	registered := MyBook{
		Book:        Book{LanguageState: LanguageChosen, LanguageTag: "grc"},
		ReadingUnit: &BookReadingUnit{ReadingUnitID: "unit", EditionLabel: "Marchant 1904"},
	}
	assert.True(t, registered.AwaitingPreparation(), "registered book without analysis awaits preparation")

	analyzed := registered
	analyzed.Acquired = &SourceMaterialSummary{
		Source:  SourceMaterial{ID: "source", MediaType: "application/epub+zip", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"},
		Signals: AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: RunCompleted},
	}
	assert.False(t, analyzed.AwaitingPreparation(), "published analysis ends preparation")
}
