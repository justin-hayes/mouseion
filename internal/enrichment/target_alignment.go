package enrichment

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidateSentenceTranslationTargets keeps an alignment only when every exact
// excerpt identifies one safe, ordered, non-overlapping, word-bounded span.
// Invalid optional alignment is discarded as a whole without invalidating the
// translation or contextual Gloss.
func ValidateSentenceTranslationTargets(sentence string, targets []string) []string {
	if len(targets) == 0 || !utf8.ValidString(sentence) {
		return nil
	}
	type span struct{ start, end, words int }
	spans := make([]span, 0, len(targets))
	lastEnd := -1
	selectedWords := 0
	for _, target := range targets {
		if target == "" || strings.TrimSpace(target) != target || !utf8.ValidString(target) || strings.ContainsAny(target, "<>") || hasUnsafeAlignmentRune(target) {
			return nil
		}
		start, end, ok := uniqueWordBoundedExcerpt(sentence, target)
		if !ok || start < lastEnd {
			return nil
		}
		words := lexicalAlignmentWordCount(sentence[start:end])
		if words == 0 {
			return nil
		}
		spans = append(spans, span{start: start, end: end, words: words})
		selectedWords += words
		lastEnd = end
	}
	if len(spans) == 0 {
		return nil
	}
	allWords := lexicalAlignmentWordCount(sentence)
	if allWords > 1 && selectedWords >= allWords {
		return nil
	}
	return append([]string(nil), targets...)
}

func uniqueWordBoundedExcerpt(sentence, excerpt string) (int, int, bool) {
	start, end := -1, -1
	for offset := range sentence {
		matchEnd := offset + len(excerpt)
		if matchEnd > len(sentence) || (matchEnd < len(sentence) && !utf8.RuneStart(sentence[matchEnd])) || !alignmentBoundaryBefore(sentence, offset) || !alignmentBoundaryAfter(sentence, matchEnd) {
			continue
		}
		if sentence[offset:matchEnd] != excerpt {
			continue
		}
		if start >= 0 {
			return 0, 0, false
		}
		start, end = offset, matchEnd
	}
	return start, end, start >= 0
}

func alignmentBoundaryBefore(text string, index int) bool {
	if index == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(text[:index])
	return !alignmentWordRune(r)
}

func alignmentBoundaryAfter(text string, index int) bool {
	if index == len(text) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(text[index:])
	return !alignmentWordRune(r)
}

func alignmentWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r)
}

func lexicalAlignmentWordCount(text string) int {
	count, inside := 0, false
	for _, r := range text {
		word := alignmentWordRune(r)
		if word && !inside {
			count++
		}
		inside = word
	}
	return count
}

func hasUnsafeAlignmentRune(text string) bool {
	for _, r := range text {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return true
		}
	}
	return false
}
