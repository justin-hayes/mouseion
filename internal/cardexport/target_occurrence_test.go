package cardexport_test

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func targetOccurrenceSnapshot(disposition cardexport.ManifestDisposition, tokens []analyzer.Token) cardexport.ManifestSnapshot {
	accepted := disposition == cardexport.ManifestAccepted
	return cardexport.ManifestSnapshot{
		SchemaVersion: cardexport.ManifestSchemaVersion, Owner: "owner-1", DeckName: "Book",
		Filename: cardexport.DownloadFilename("Book"),
		Items: []cardexport.ManifestItem{{
			Ordinal: 0, Disposition: disposition, CorpusID: "corpus-1", SentenceOrdinal: 7,
			Entry: cardexport.Entry{
				Language: "de", CanonicalLemma: "umhauen", UPOS: "VERB",
				Sentence: "Dann haut das Motorrad Piero um.", TargetWord: "Piero",
				SentenceTokens: tokens, SourceDocument: "Book", FirstEncounter: 1,
			},
			Quality: cardexport.SentenceQuality{Accepted: accepted},
		}},
	}
}

func TestFrozenDeckTargetOccurrenceResolvesTheFrozenToken(t *testing.T) {
	tokens := []analyzer.Token{
		{Surface: "Dann", Location: analyzer.SourceLocation{SourceDocumentID: "unit-1", StartOffset: 0, EndOffset: 4}},
		{Surface: "haut", Location: analyzer.SourceLocation{SourceDocumentID: "unit-1", StartOffset: 5, EndOffset: 9}},
		{Surface: "das", Location: analyzer.SourceLocation{SourceDocumentID: "unit-1", StartOffset: 10, EndOffset: 13}},
		{Surface: "Motorrad", Location: analyzer.SourceLocation{SourceDocumentID: "unit-1", StartOffset: 14, EndOffset: 22}},
		{Surface: "Piero", Location: analyzer.SourceLocation{SourceDocumentID: "unit-1", StartOffset: 23, EndOffset: 28}},
		{Surface: "um", Location: analyzer.SourceLocation{SourceDocumentID: "unit-1", StartOffset: 29, EndOffset: 31}},
	}
	deck, err := cardexport.NewPresentation(nil).Restore(targetOccurrenceSnapshot(cardexport.ManifestAccepted, tokens))
	require.NoError(t, err)

	target, ok := deck.TargetOccurrence(0)
	require.True(t, ok)
	assert.Equal(t, cardexport.TargetOccurrence{
		CorpusID: "corpus-1", SourceDocumentID: "unit-1", SentenceOrdinal: 7, StartOffset: 23, EndOffset: 28,
	}, target)
}

func TestFrozenDeckTargetOccurrenceSkipsAnUnresolvableTarget(t *testing.T) {
	tests := map[string]struct {
		disposition cardexport.ManifestDisposition
		tokens      []analyzer.Token
		targetWord  string
	}{
		"omitted item": {
			disposition: cardexport.ManifestQualityOmitted, targetWord: "Piero",
		},
		"no persisted tokens": {
			disposition: cardexport.ManifestAccepted, targetWord: "Piero",
		},
		"token span contradicts the anchor": {
			disposition: cardexport.ManifestAccepted, targetWord: "Piero",
			tokens: []analyzer.Token{
				{Surface: "Piero", Location: analyzer.SourceLocation{SourceDocumentID: "unit-1", StartOffset: 0, EndOffset: 5}},
			},
		},
		"target is absent": {
			disposition: cardexport.ManifestAccepted, targetWord: "fehlt",
			tokens: []analyzer.Token{
				{Surface: "Motorrad", Location: analyzer.SourceLocation{SourceDocumentID: "unit-1", StartOffset: 14, EndOffset: 22}},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot := targetOccurrenceSnapshot(test.disposition, test.tokens)
			snapshot.Items[0].Entry.TargetWord = test.targetWord
			deck, err := cardexport.NewPresentation(nil).Restore(snapshot)
			require.NoError(t, err)
			_, ok := deck.TargetOccurrence(0)
			require.False(t, ok)
		})
	}
}
