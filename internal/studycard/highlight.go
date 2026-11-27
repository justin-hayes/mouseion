// Package studycard holds the small text helpers Study's card faces share: the
// English translation highlight and the Ancient Greek passage-context label.
package studycard

import (
	"html"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// HighlightEnglishTarget renders an optional provider alignment only when it
// is a unique, word-bounded match in the complete translation. The provider
// supplies plain text; all HTML is owned and escaped by Mouseion.
func HighlightEnglishTarget(translation, target string) string {
	start, end, ok := uniqueTargetMatch(translation, target)
	if !ok {
		return escapeField(translation)
	}
	wordCount := lexicalWordCount(translation)
	if wordCount > 1 && lexicalWordCount(translation[start:end]) == wordCount {
		return escapeField(translation)
	}
	return escapeField(translation[:start]) + "<b>" + escapeField(translation[start:end]) + "</b>" + escapeField(translation[end:])
}

// HighlightEnglishTargets renders validated structured excerpts. Historical
// cached single-phrase targets retain their legacy presentation.
func HighlightEnglishTargets(translation string, targets []string, legacyTarget string) string {
	validated := enrichment.ValidateSentenceTranslationTargets(translation, targets)
	if len(validated) == 0 {
		return HighlightEnglishTarget(translation, legacyTarget)
	}
	var output strings.Builder
	cursor := 0
	for _, target := range validated {
		start, end, ok := uniqueExactTargetMatch(translation, target)
		if !ok || start < cursor {
			return escapeField(translation)
		}
		output.WriteString(escapeField(translation[cursor:start]))
		output.WriteString("<b>")
		output.WriteString(escapeField(translation[start:end]))
		output.WriteString("</b>")
		cursor = end
	}
	output.WriteString(escapeField(translation[cursor:]))
	return output.String()
}

func lexicalWordCount(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		isWord := isWordRune(r)
		if isWord && !inWord {
			count++
		}
		inWord = isWord
	}
	return count
}

func uniqueTargetMatch(text, target string) (int, int, bool) {
	return uniqueTargetMatchUsing(text, target, strings.EqualFold)
}

func uniqueExactTargetMatch(text, target string) (int, int, bool) {
	return uniqueTargetMatchUsing(text, target, func(text, target string) bool { return text == target })
}

func uniqueTargetMatchUsing(text, target string, matches func(string, string) bool) (int, int, bool) {
	target = strings.TrimSpace(target)
	if target == "" || !utf8.ValidString(text) || !utf8.ValidString(target) || !hasWordOrNumber(target) {
		return 0, 0, false
	}
	start, end := -1, -1
	for offset := range text {
		matchEnd := offset + len(target)
		if matchEnd > len(text) || (matchEnd < len(text) && !utf8.RuneStart(text[matchEnd])) || !wordBoundaryBefore(text, offset) || !wordBoundaryAfter(text, matchEnd) {
			continue
		}
		if !matches(text[offset:matchEnd], target) {
			continue
		}
		if start >= 0 {
			return 0, 0, false
		}
		start, end = offset, matchEnd
	}
	return start, end, start >= 0
}

func hasWordOrNumber(value string) bool {
	for _, r := range value {
		if isWordRune(r) {
			return true
		}
	}
	return false
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

func wordBoundaryBefore(text string, index int) bool {
	if index == 0 {
		return true
	}
	if index < 0 || index > len(text) || !utf8.RuneStart(text[index]) {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(text[:index])
	return !unicode.IsLetter(r) && !unicode.IsNumber(r)
}

func wordBoundaryAfter(text string, index int) bool {
	if index == len(text) {
		return true
	}
	if index < 0 || index > len(text) || !utf8.RuneStart(text[index]) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text[index:])
	return !unicode.IsLetter(r) && !unicode.IsNumber(r)
}

func escapeField(value string) string {
	value = html.EscapeString(value)
	value = strings.ReplaceAll(value, "\x1f", "&#31;")
	value = strings.ReplaceAll(value, "{", "&#123;")
	return strings.ReplaceAll(value, "}", "&#125;")
}
