// Package textmatch provides Unicode-safe matching for learner-facing text.
package textmatch

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// FoldedWordSpan returns the byte offsets of the first case-insensitive target
// occurrence that is bounded by non-word runes. It compares at rune boundaries
// so case folding that changes UTF-8 byte length cannot corrupt the returned
// offsets.
func FoldedWordSpan(text, target string) (int, int, bool) {
	if target == "" || !utf8.ValidString(text) || !utf8.ValidString(target) {
		return 0, 0, false
	}
	targetRunes := []rune(target)
	textRunes := []rune(text)
	if len(targetRunes) > len(textRunes) {
		return 0, 0, false
	}

	offsets := make([]int, 0, len(textRunes)+1)
	for offset := range text {
		offsets = append(offsets, offset)
	}
	offsets = append(offsets, len(text))
	for i := 0; i+len(targetRunes) <= len(textRunes); i++ {
		endRune := i + len(targetRunes)
		if (i > 0 && isWordRune(textRunes[i-1])) || (endRune < len(textRunes) && isWordRune(textRunes[endRune])) {
			continue
		}
		start, end := offsets[i], offsets[endRune]
		if strings.EqualFold(text[start:end], target) {
			return start, end, true
		}
	}
	return 0, 0, false
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) ||
		strings.ContainsRune("'’‐‑-", r)
}
