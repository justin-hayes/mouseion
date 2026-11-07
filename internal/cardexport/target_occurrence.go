package cardexport

import (
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
)

// TargetOccurrence is the exact persisted source token one accepted manifest
// item's representative sentence targets.
type TargetOccurrence struct {
	CorpusID, SourceDocumentID              string
	SentenceOrdinal, StartOffset, EndOffset int64
}

// TargetOccurrence resolves the exact target token of the accepted manifest item
// at ordinal, using the same anchor card presentation uses. It reports false
// when the item is not accepted or the persisted token cannot be resolved.
func (d FrozenDeck) TargetOccurrence(ordinal int) (TargetOccurrence, bool) {
	for _, item := range d.manifest.decisions {
		if item.Ordinal != ordinal || item.Disposition != ManifestAccepted {
			continue
		}
		return manifestTargetOccurrence(item)
	}
	return TargetOccurrence{}, false
}

// manifestTargetOccurrence anchors the item's tested target on the frozen
// dependency token that carries it and converts that token's document offsets
// to the persisted occurrence identity.
func manifestTargetOccurrence(item ManifestItem) (TargetOccurrence, bool) {
	entry := item.Entry
	target := testedEntryTarget(entry)
	corpusID := item.CorpusID
	if corpusID == "" {
		corpusID = entry.CorpusID
	}
	if corpusID == "" || target == "" || len(entry.SentenceTokens) == 0 {
		return TargetOccurrence{}, false
	}
	token, ok := sentencequality.TargetToken(entry.Sentence, target, entry.SentenceTokens, entry.SentenceStartOffset)
	if !ok || token.Location.SourceDocumentID == "" {
		return TargetOccurrence{}, false
	}
	start, err := checked.Int64FromUint64(token.Location.StartOffset)
	if err != nil {
		return TargetOccurrence{}, false
	}
	end, err := checked.Int64FromUint64(token.Location.EndOffset)
	if err != nil || end <= start {
		return TargetOccurrence{}, false
	}
	return TargetOccurrence{
		CorpusID: corpusID, SourceDocumentID: token.Location.SourceDocumentID,
		SentenceOrdinal: item.SentenceOrdinal, StartOffset: start, EndOffset: end,
	}, true
}
