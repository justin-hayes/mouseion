// Package textmatch provides Unicode-safe matching for learner-facing text.
package textmatch

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// CleanLexicalSurface removes surrounding Unicode punctuation and symbols
// from a tested surface while preserving lexical internals and apostrophes.
// Standalone punctuation is returned unchanged.
//
// The analyzer applies the same rule to new tokens. Card export reapplies it
// before matching so persisted candidates from older analyzer versions behave
// identically without rewriting their stored source text, offsets, or forms.
func CleanLexicalSurface(surface string) string {
	surface = strings.TrimSpace(surface)
	if surface == "" {
		return ""
	}
	runes := []rune(surface)
	start, end := 0, len(runes)
	isEdgeDecoration := func(r rune) bool {
		return r != '\'' && r != '\u2019' && (unicode.IsPunct(r) || unicode.IsSymbol(r))
	}
	for start < end && isEdgeDecoration(runes[start]) {
		start++
	}
	for end > start && isEdgeDecoration(runes[end-1]) {
		end--
	}
	if start == end {
		return surface
	}
	return string(runes[start:end])
}

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
