package persistence

import (
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/stretchr/testify/assert"
)

func contentTestDefinition() classical.Definition {
	book := 1
	return classical.Definition{
		Key:                   "unit-1",
		WorkKey:               "work",
		WorkTitle:             "Work",
		WorkAuthor:            "Author",
		EditionKey:            "edition",
		EditionLabel:          "Edition",
		Language:              classical.Language,
		ConstituentBook:       &book,
		ConstituentBooks:      1,
		CitationStart:         "1.1.1",
		CitationEnd:           "1.1.2",
		WordCount:             2,
		Source:                classical.SourcePin{Layer: "source", FileName: "work.conllu", Size: 10, SHA256: strings.Repeat("a", 64), Format: classical.SourceFormatOGACoNLLU},
		InterpretationVersion: "oga-agdt-upos/5",
		Revision:              1,
	}
}

func recordedUnit(definition classical.Definition) storedClassicalUnit {
	return storedClassicalUnit{
		SourceLayer:           definition.Source.Layer,
		SourceFileName:        definition.Source.FileName,
		SourceFileSize:        definition.Source.Size,
		SourceFileSHA256:      definition.Source.SHA256,
		SourceFormat:          definition.Source.Format,
		InterpretationVersion: definition.InterpretationVersion,
		Translation:           definition.Translation,
		Revision:              definition.Revision,
	}
}

func TestStoredContentChangedIgnoresAnUnrecordedLegacyRow(t *testing.T) {
	definition := contentTestDefinition()
	legacy := recordedUnit(definition)
	legacy.SourceLayer = ""
	legacy.SourceFormat = ""

	translated := definition
	translated.Translation = &classical.TranslationPin{
		Layer: "translation", FileName: "work.xml", Size: 20,
		SHA256: strings.Repeat("b", 64), CitationStart: "1.1.1", CitationEnd: "1.1.2",
	}
	assert.False(t, storedContentChanged(legacy, translated), "a legacy row never recorded a translation pin")

	legacy.SourceFileSHA256 = strings.Repeat("c", 64)
	assert.True(t, storedContentChanged(legacy, definition), "a recorded source pin still compares")
}

func TestStoredContentChangedComparesEveryRecordedContentBearingField(t *testing.T) {
	definition := contentTestDefinition()
	recorded := recordedUnit(definition)
	assert.False(t, storedContentChanged(recorded, definition))

	source := definition
	source.Source.Layer = "another-source"
	assert.True(t, storedContentChanged(recorded, source))

	source = definition
	source.Source.Format = "another-format"
	assert.True(t, storedContentChanged(recorded, source))

	source = definition
	source.InterpretationVersion = "oga-agdt-upos/6"
	assert.True(t, storedContentChanged(recorded, source))

	translated := definition
	translated.Translation = &classical.TranslationPin{
		Layer: "translation", FileName: "work.xml", Size: 20,
		SHA256: strings.Repeat("b", 64), CitationStart: "1.1.1", CitationEnd: "1.1.2",
	}
	assert.True(t, storedContentChanged(recorded, translated), "adding a translation pin is a content change")
	assert.True(t, storedContentChanged(recordedUnit(translated), definition), "removing a translation pin is a content change")

	changedRange := translated
	changedRange.Translation = &classical.TranslationPin{
		Layer: "translation", FileName: "work.xml", Size: 20,
		SHA256: strings.Repeat("b", 64), CitationStart: "1.1.2", CitationEnd: "1.1.3",
	}
	assert.True(t, storedContentChanged(recordedUnit(translated), changedRange), "a changed aligned range is a content change")
}
