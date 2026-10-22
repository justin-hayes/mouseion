package sentencequality

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"
)

// directorSentence is the first complete sentence of the Director speech in
// Faust I. Its four source lines are one analyzer sentence that carries the
// authorial line breaks as LF, not a list or page fragment.
const directorSentence = "Ihr beyden die ihr mir so oft,\nIn Noth und Trübsal, beygestanden,\nSagt was ihr wohl, in deutschen Landen,\nVon unsrer Unternehmung hofft?" //nolint:misspell // Goethe source text

// TestMultilineSourceLineIsNotStructuralNoise pins the only quality-policy
// change: LF alone no longer rejects an otherwise complete sentence, and the
// same sentence keeps every existing award.
func TestMultilineSourceLineIsNotStructuralNoise(t *testing.T) {
	quality := ScoreSentenceQuality("de", directorSentence, "Unternehmung", 42, nil, 0)
	require.True(t, quality.Accepted, "quality=%+v", quality)

	t.Run("keeps the existing score", func(t *testing.T) {
		require.Equal(t, 110, quality.Score)
	})
	t.Run("keeps every existing award", func(t *testing.T) {
		require.Contains(t, quality.Reasons, "no obvious structural noise")
		require.NotContains(t, quality.Reasons, "structural noise or boilerplate")
	})
}

// TestBlankSourceLinesAreNotListOrPageMarkers pins that the per-line checks
// look at content, so an empty line contributes no marker of its own.
func TestBlankSourceLinesAreNotListOrPageMarkers(t *testing.T) {
	require.False(t, listOrPageFragment("Ihr beyden die ihr mir so oft,\n\nVon unsrer Unternehmung hofft?"))
}

// TestListAndPageMarkersRejectOnAnySourceLine keeps the existing list and
// page-marker content checks while applying them to every source line, not
// only the first.
func TestListAndPageMarkersRejectOnAnySourceLine(t *testing.T) {
	for _, test := range []struct{ name, sentence string }{
		{"bullet on the first line", "• " + directorSentence},
		{"dialogue-dash bullet on the first line", "- " + directorSentence},
		{"asterisk bullet on the first line", "* " + directorSentence},
		{"bullet on a later line", "Ihr beyden die ihr mir so oft,\n• In Noth und Trübsal, beygestanden,\nSagt was ihr wohl, in deutschen Landen,\nVon unsrer Unternehmung hofft?"}, //nolint:misspell // Goethe source text
		{"numbering on the first line", "12. " + directorSentence},
		{"numbering on a later line", "Ihr beyden die ihr mir so oft,\n12. In Noth und Trübsal, beygestanden,\nSagt was ihr wohl, in deutschen Landen,\nVon unsrer Unternehmung hofft?"}, //nolint:misspell // Goethe source text
		{"page number on the first line", "42\n" + directorSentence},
		{"page fragment on a later line", directorSentence + "\n42"},
		{"decorated page fragment on a later line", directorSentence + "\n— 42 —"},
	} {
		t.Run(test.name, func(t *testing.T) {
			quality := ScoreSentenceQuality("de", test.sentence, "Unternehmung", 42, nil, 0)
			require.False(t, quality.Accepted, "quality=%+v", quality)
			require.Contains(t, quality.Reasons, "structural noise or boilerplate")
		})
	}
}

// TestDeliberatePoetryOmissionsFailHonestly pins the measured Faust excerpts
// the specification deliberately omits: short exclamations and an interrupted
// dash ending stay sentence-quality omissions rather than padded or invented
// cards. The quoted lines are the packaged-XHTML excerpts measured in
// doc/features/basic-poetry-support.md; 4:580 is also committed in
// internal/epub/testfixtures/faust-excerpt.
func TestDeliberatePoetryOmissionsFailHonestly(t *testing.T) {
	for _, test := range []struct{ name, sentence, target, reason string }{
		{"4:368 short exclamation", "Ach!", "Ach", "too short or fragmented"},
		{"4:580 short exclamation", "O weh!", "weh", "too short or fragmented"},
		{"0:599-602 interrupted Geist speech", "Du hast mich mächtig angezogen,\nAn meiner Sphäre lang’ gesogen,\nUnd nun –", "mächtig", "incomplete sentence boundaries"},
	} {
		t.Run(test.name, func(t *testing.T) {
			quality := ScoreSentenceQuality("de", test.sentence, test.target, 42, nil, 0)
			require.False(t, quality.Accepted, "quality=%+v", quality)
			require.Contains(t, quality.Reasons, test.reason)
		})
	}
}

// TestNoBreakVerdictsMatchTheV1Policy is the comparison test: for every text
// without a source line break, the line-aware policy returns exactly the v1
// verdict. Only a newline may change an outcome.
func TestNoBreakVerdictsMatchTheV1Policy(t *testing.T) {
	for _, test := range []struct {
		name, text string
		v1         bool
	}{
		{"empty", "", true},
		{"whitespace", " ", true},
		{"ordinary prose", "Das Haus steht an der Straße.", false},
		{"numbered list with a period", "12. Das Haus steht an der Straße.", true},
		{"numbered list with a parenthesis", "12) Das Haus steht an der Straße.", true},
		{"bullet list", "• Das Haus steht an der Straße.", true},
		{"dialogue-dash list", "- Das Haus steht an der Straße.", true},
		{"asterisk list", "* Das Haus steht an der Straße.", true},
		{"page number", "42", true},
		{"decorated page number", "— 42 —", true},
		{"lone dash", "-", true},
		{"chapter heading", "Chapter 12: Das Haus steht an der Straße.", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.v1, v1ListOrPageFragment(test.text), "the v1 oracle and expected verdict disagree")
			require.Equal(t, test.v1, listOrPageFragment(test.text), "the line-aware policy changed a no-break verdict")
		})
	}

	t.Run("a newline alone changes the verdict", func(t *testing.T) {
		// The v1 policy rejected every text containing a newline. The line-aware
		// policy keeps that verdict only when a whole-line marker is present.
		require.True(t, v1ListOrPageFragment(directorSentence))
		require.False(t, listOrPageFragment(directorSentence))
	})
}

// v1ListOrPageFragment is a deliberately frozen copy of the superseded policy.
// It is the comparison oracle for this test, not production code.
func v1ListOrPageFragment(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return true
	}
	firstLine := strings.TrimSpace(strings.SplitN(trimmed, "\n", 2)[0])
	if strings.Contains(trimmed, "\n") || strings.HasPrefix(firstLine, "•") || strings.HasPrefix(firstLine, "- ") || strings.HasPrefix(firstLine, "* ") {
		return true
	}
	fields := strings.Fields(firstLine)
	if len(fields) <= 12 && len(fields) > 0 {
		prefix := strings.TrimRight(fields[0], ".)")
		if _, err := fmt.Sscan(prefix, new(int)); err == nil {
			return true
		}
	}
	allDigits := true
	for _, r := range strings.Trim(trimmed, "-–— ") {
		if !unicode.IsDigit(r) {
			allDigits = false
			break
		}
	}
	return allDigits
}
