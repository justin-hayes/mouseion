// Package sentencequality scores candidate sentences as representative
// vocabulary examples. It owns the accept/reject gate and the ranking the
// prepared deck and the Reading freeze share (ADR 0106 §2): the generic text
// criteria, each language's boundary rules and the German GDEX overlay
// (ADR 0062).
package sentencequality

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/gdex"
	"github.com/justin-hayes/mouseion/internal/textmatch"
)

// GateVersion identifies the accept/reject rules this package applies. A
// Reading snapshot records the version that admitted each frozen identity, so a
// later gate change never rewrites an active or historical snapshot: a learner
// rereads a Book to adopt a corrected gate (ADR 0106 §3).
//
// Bump GateVersion whenever a change can alter whether a sentence is accepted:
// a knock-out criterion, the minimum score, a language's boundary rule, or the
// German GDEX overlay. A change that only affects the ranking among accepted
// sentences, or only the recorded reasons, leaves every accept/reject outcome
// unchanged and must not bump it.
const GateVersion = 2

const (
	minimumSentenceWords         = 6
	maximumSentenceWords         = 50
	maximumSentenceCharacters    = 400
	minimumSentenceScore         = 70
	reasonNoFiniteVerbAndSubject = "no finite verb and subject"
)

// SentenceQuality is the deterministic result of scoring one sentence. Score is
// in [0, 100]. A sentence that fails the gate has Accepted false; a rejected
// sentence scores zero. Reasons explains every award and rejection, and
// GDEXScore carries the German GDEX gradual score when a parse was available.
type SentenceQuality struct {
	Accepted  bool
	Score     int
	Reasons   []string
	GDEXScore float64
}

// ScoreSentenceQuality applies a deliberately small, explainable gate using only
// sentence text, the selected target form, source location and, for German, the
// persisted dependency parse.
func ScoreSentenceQuality(language, sentence, target string, firstEncounter int64, tokens []analyzer.Token, sentenceStartOffset uint64) SentenceQuality {
	text := strings.TrimSpace(sentence)
	quality := SentenceQuality{Reasons: make([]string, 0, 8)}
	scoreLengthQuality(&quality, text)
	scoreTargetQuality(&quality, text, target, tokens, sentenceStartOffset)
	scoreSourceLocationQuality(&quality, firstEncounter)
	scoreBoundaryQuality(&quality, language, text)
	scoreStructuralNoiseQuality(&quality, text)
	scoreGermanGDEXQuality(language, &quality, text, target, tokens)
	quality.Accepted = quality.Score >= minimumSentenceScore && !containsRejection(quality.Reasons)
	return quality
}

// award grants points and records the matching acceptance reason.
func (quality *SentenceQuality) award(points int, reason string) {
	quality.Score += points
	quality.Reasons = append(quality.Reasons, reason)
}

// reject records a rejection reason without changing the score.
func (quality *SentenceQuality) reject(reason string) {
	quality.Reasons = append(quality.Reasons, reason)
}

// scoreLengthQuality applies the length gate: a fragmented or over-long
// sentence is rejected, otherwise it earns the base score and a context bonus
// when its word count sits in the useful window.
func scoreLengthQuality(quality *SentenceQuality, text string) {
	words := strings.Fields(text)
	if !utf8.ValidString(text) || len(words) < minimumSentenceWords {
		quality.reject("too short or fragmented")
		return
	}
	if sentenceTooLong(text) {
		quality.reject("too long")
		return
	}
	quality.award(30, "usable length")
	if len(words) >= 8 && len(words) <= 30 {
		quality.award(10, "useful context window")
	}
}

// scoreTargetQuality requires the selected target to appear as a whole word.
func scoreTargetQuality(quality *SentenceQuality, text, target string, tokens []analyzer.Token, sentenceStartOffset uint64) {
	if targetIndex(text, strings.TrimSpace(target), tokens, sentenceStartOffset) < 0 {
		quality.reject("target not present as a word")
		return
	}
	quality.award(25, "target present")
}

// scoreSourceLocationQuality requires a valid first-encounter offset.
func scoreSourceLocationQuality(quality *SentenceQuality, firstEncounter int64) {
	if firstEncounter < 0 || firstEncounter == math.MaxInt64 {
		quality.reject("invalid source location")
		return
	}
	quality.award(15, "valid source location")
}

// scoreBoundaryQuality requires a terminal sentence boundary and, for
// languages that capitalise sentences, a capitalised or quoted start.
func scoreBoundaryQuality(quality *SentenceQuality, language, text string) {
	if !hasCompleteBoundary(language, text) {
		quality.reject("incomplete sentence boundaries")
		return
	}
	quality.award(20, "complete sentence boundaries")
}

// scoreStructuralNoiseQuality rejects headings, boilerplate and extraction
// anomalies that pass the length gate.
func scoreStructuralNoiseQuality(quality *SentenceQuality, text string) {
	if structuralNoise(text) {
		quality.reject("structural noise or boilerplate")
		return
	}
	quality.award(10, "no obvious structural noise")
}

// scoreGermanGDEXQuality overlays the German GDEX grader. It contributes the
// grader score and merges its reasons, and a rejected grade zeroes the score.
// Other languages, and calls without a dependency parse, keep only the text
// criteria scored above.
func scoreGermanGDEXQuality(language string, quality *SentenceQuality, text, target string, tokens []analyzer.Token) {
	if tokens == nil || !isGerman(language) {
		return
	}
	gdexQuality := gdex.ScoreSentenceQuality(analyzer.Sentence{Text: text, Tokens: tokens}, gdexTargetIndices(target, tokens))
	quality.GDEXScore = gdexQuality.Score
	for _, reason := range gdexQuality.Reasons {
		if !slices.Contains(quality.Reasons, reason) {
			quality.Reasons = append(quality.Reasons, reason)
		}
	}
	if !gdexQuality.Accepted {
		quality.Score = 0
		if !slices.Contains(quality.Reasons, reasonNoFiniteVerbAndSubject) {
			quality.Reasons = append(quality.Reasons, reasonNoFiniteVerbAndSubject)
		}
	}
}

// gdexTargetIndices reports the token ordinals whose lexical surface matches the
// selected target.
func gdexTargetIndices(target string, tokens []analyzer.Token) []int {
	targetIndices := make([]int, 0, 1)
	cleanTarget := textmatch.CleanLexicalSurface(target)
	for index, token := range tokens {
		if cleanTarget != "" && strings.EqualFold(textmatch.CleanLexicalSurface(token.Surface), cleanTarget) {
			targetIndices = append(targetIndices, index)
		}
	}
	return targetIndices
}

func isGerman(language string) bool {
	return canonicalization.NormalizeLanguage(language) == "de"
}

func sentenceTooLong(text string) bool {
	text = strings.TrimSpace(text)
	return len(strings.Fields(text)) > maximumSentenceWords || utf8.RuneCountInString(text) > maximumSentenceCharacters
}

func containsRejection(reasons []string) bool {
	for _, reason := range reasons {
		switch reason {
		case "too short or fragmented", "too long", "target not present as a word", "invalid source location", "incomplete sentence boundaries", "structural noise or boilerplate", reasonNoFiniteVerbAndSubject:
			return true
		}
	}
	return false
}

func hasCompleteBoundary(language, text string) bool {
	if text == "" {
		return false
	}
	if IsAncientGreek(language) {
		// Greek editions start sentences in lowercase, so only the terminal
		// boundary is required. The source terminators are the full stop, the
		// semicolon / Greek question mark (U+037E) and the raised dot / ano
		// teleia (U+00B7 / U+0387). The generic dialogue and closing-quote forms
		// are deliberately outside this rule.
		last, _ := utf8.DecodeLastRuneInString(text)
		return strings.ContainsRune(".;\u037e·\u0387", last)
	}
	if !hasCompleteEnding(text) {
		return false
	}
	return acceptedSentenceStart(text)
}

const (
	dialogueDashes = "—–―"
	dialogueSpaces = " \u00a0\u202f"
	openingStarts  = "\"'“„«"
	closingQuotes  = "»”\"’"
	terminalMarks  = ".!?…"
)

// hasCompleteEnding reports whether text closes with a terminal mark, or a
// terminal mark followed by one or more closing quotation marks that may be
// separated by dialogue spaces.
func hasCompleteEnding(text string) bool {
	end := len(text)
	for end > 0 {
		quote, size := utf8.DecodeLastRuneInString(text[:end])
		if !strings.ContainsRune(closingQuotes, quote) {
			break
		}
		end -= size
		for end > 0 {
			space, spaceSize := utf8.DecodeLastRuneInString(text[:end])
			if !strings.ContainsRune(dialogueSpaces, space) {
				break
			}
			end -= spaceSize
		}
	}
	if end == 0 {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(text[:end])
	return strings.ContainsRune(terminalMarks, last)
}

// acceptedSentenceStart reports whether text starts with an accepted sentence
// opening: a capital, a digit or an opening quotation mark. A dialogue dash may
// precede it when a dialogue space separates the two.
func acceptedSentenceStart(text string) bool {
	first, size := utf8.DecodeRuneInString(text)
	if strings.ContainsRune(dialogueDashes, first) {
		space, spaceSize := utf8.DecodeRuneInString(text[size:])
		if !strings.ContainsRune(dialogueSpaces, space) {
			return false
		}
		first, _ = utf8.DecodeRuneInString(text[size+spaceSize:])
	}
	return unicode.IsUpper(first) || unicode.IsNumber(first) || strings.ContainsRune(openingStarts, first)
}

// IsAncientGreek reports whether a language tag is Ancient Greek, whose
// editions do not capitalise sentence starts.
func IsAncientGreek(language string) bool {
	return canonicalization.NormalizeLanguage(language) == "grc"
}

func structuralNoise(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, marker := range []string{"table of contents", "inhaltsverzeichnis", "bibliography", "bibliografie", "references", "literaturverzeichnis", "notes", "anmerkungen", "footnote", "endnote", "chapter", "kapitel", "section", "abschnitt"} {
		if structuralHeading(lower, marker) {
			return true
		}
	}
	return strings.Contains(lower, "all rights reserved") || strings.Contains(lower, "project gutenberg") ||
		strings.Contains(lower, ".....") || strings.Count(text, "\t") >= 2 ||
		listOrPageFragment(text) || citationDense(text) || extractionAnomaly(text)
}

func listOrPageFragment(text string) bool {
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

func citationDense(text string) bool {
	brackets := strings.Count(text, "[") + strings.Count(text, "]")
	years := 0
	for year := 1500; year <= 2099; year++ {
		if strings.Contains(text, strconv.Itoa(year)) {
			years++
		}
	}
	lower := strings.ToLower(text)
	citationMarkers := strings.Count(lower, " et al.") + strings.Count(lower, "doi:") + strings.Count(lower, " pp.") + strings.Count(lower, " vol.")
	return brackets >= 4 || years >= 2 || citationMarkers >= 2 || strings.Count(lower, "doi:") >= 1
}

func extractionAnomaly(text string) bool {
	controlOrReplacement := 0
	for _, r := range text {
		if r == unicode.ReplacementChar || (unicode.IsControl(r) && r != '\n' && r != '\t') {
			controlOrReplacement++
		}
	}
	words := strings.Fields(strings.ToLower(text))
	repeated := 0
	for i := 1; i < len(words); i++ {
		if strings.Trim(words[i], ".,;:!?") == strings.Trim(words[i-1], ".,;:!?") {
			repeated++
		}
	}
	return controlOrReplacement > 0 || repeated >= 3 || strings.Contains(text, "\u00ad \u00ad")
}

func structuralHeading(text, marker string) bool {
	if !strings.HasPrefix(text, marker) {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(text, marker))
	return rest == "" || strings.ContainsRune(":.-–—0123456789", []rune(rest)[0])
}
