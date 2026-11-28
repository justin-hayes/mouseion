package sentencequality

import (
	"strings"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/textmatch"
)

func targetIndex(sentence, target string, tokens []analyzer.Token, sentenceStartOffset uint64) int {
	start, _, ok := targetSpan(sentence, textmatch.CleanLexicalSurface(target), tokens, sentenceStartOffset)
	if ok {
		return start
	}
	return -1
}

func targetSpan(sentence, target string, tokens []analyzer.Token, sentenceStartOffset uint64) (int, int, bool) {
	if start, end, ok := textmatch.FoldedWordSpan(sentence, target); ok {
		return start, end, true
	}
	return tokenAnchorSpan(sentence, target, tokens, sentenceStartOffset)
}

// tokenAnchorSpan anchors target on the first persisted token, in sentence
// order, whose cleaned surface case-folds equal to it. Token offsets are
// document-absolute Unicode code points; the span is taken relative to the
// sentence's persisted start offset and converted to byte offsets in the
// sentence text. An elided or inversion-hyphen target (l'homme, d'accord,
// va-t-il) has no word boundary for the word-bounded matcher, but its own token
// does carry the exact span (ADR 0102).
//
// The span is accepted only when the sentence text at that span, cleaned the
// same way, case-folds equal to the target. That check rejects stale or
// inconsistent offsets and multiword-token parts, which carry the whole
// contraction's span.
func tokenAnchorSpan(sentence, target string, tokens []analyzer.Token, sentenceStartOffset uint64) (int, int, bool) {
	if target == "" || len(tokens) == 0 {
		return 0, 0, false
	}
	for _, token := range tokens {
		if !strings.EqualFold(textmatch.CleanLexicalSurface(token.Surface), target) {
			continue
		}
		start, end, ok := tokenSpanBytes(sentence, token.Location.StartOffset, token.Location.EndOffset, sentenceStartOffset)
		if !ok || !strings.EqualFold(textmatch.CleanLexicalSurface(sentence[start:end]), target) {
			return 0, 0, false
		}
		return start, end, true
	}
	return 0, 0, false
}

// tokenSpanBytes converts a token's document-absolute code-point span to byte
// offsets within sentence, relative to sentenceStartOffset. It reports false
// when the offsets do not describe a span inside the sentence text.
func tokenSpanBytes(sentence string, start, end, sentenceStartOffset uint64) (int, int, bool) {
	if start < sentenceStartOffset || end < start {
		return 0, 0, false
	}
	byteOffsets := make([]int, 0, utf8.RuneCountInString(sentence)+1)
	for offset := range sentence {
		byteOffsets = append(byteOffsets, offset)
	}
	byteOffsets = append(byteOffsets, len(sentence))
	relativeStart, relativeEnd := start-sentenceStartOffset, end-sentenceStartOffset
	if relativeStart >= uint64(len(byteOffsets)) || relativeEnd >= uint64(len(byteOffsets)) {
		return 0, 0, false
	}
	return byteOffsets[relativeStart], byteOffsets[relativeEnd], true
}
