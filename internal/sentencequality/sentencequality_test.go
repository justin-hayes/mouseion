package sentencequality

import (
	"slices"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestScoreSentenceQuality(t *testing.T) {
	tests := []struct {
		name, sentence, target, reason string
		location                       int64
		accepted                       bool
	}{
		{"long contextual", "Vor dem alten Haus spielen heute mehrere fröhliche Kinder, während ihre Eltern im sonnigen Garten gemeinsam das Abendessen vorbereiten.", "Haus", "usable length", 42, true},
		{"too short", "Altes Haus.", "Haus", "too short or fragmented", 42, false},
		{"too long", "Das Haus " + strings.Repeat("steht weit außerhalb der alten Stadt ", 9) + "still.", "Haus", "too long", 42, false},
		{"missing target", "Vor dem alten Gebäude spielen heute mehrere fröhliche Kinder.", "Haus", "target not present as a word", 42, false},
		{"fragmented boundary", "Vor dem alten Haus spielen heute mehrere Kinder", "Haus", "incomplete sentence boundaries", 42, false},
		{"contents", "Inhaltsverzeichnis: Das Haus und seine lange Geschichte ..... 12.", "Haus", "structural noise or boilerplate", 42, false},
		{"bibliography", "Bibliography: Das Haus in der europäischen Literatur.", "Haus", "structural noise or boilerplate", 42, false},
		{"numbered list", "12. Das Haus steht oberhalb des alten Dorfes.", "Haus", "structural noise or boilerplate", 42, false},
		{"chapter heading", "Kapitel 3: Das Haus und seine lange Geschichte.", "Haus", "structural noise or boilerplate", 42, false},
		{"citation density", "Das Haus wird bei Müller [1999] und Schmidt [2004] ausführlich beschrieben.", "Haus", "structural noise or boilerplate", 42, false},
		{"extraction anomaly", "Das Haus Haus Haus Haus steht heute am See.", "Haus", "structural noise or boilerplate", 42, false},
		{"boilerplate", "All rights reserved for this edition of Haus und Garten.", "Haus", "structural noise or boilerplate", 42, false},
		{"invalid location", "Vor dem alten Haus spielen heute mehrere fröhliche Kinder.", "Haus", "invalid source location", -1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ScoreSentenceQuality("", tc.sentence, tc.target, tc.location, nil, 0)
			assert.Equal(t, tc.accepted, got.Accepted)
			assert.True(t, slices.Contains(got.Reasons, tc.reason), "quality=%+v", got)
		})
	}
}

// TestScoreSentenceQualityGermanOverlayGatesOnFiniteVerbAndSubject pins that a
// language with a rubric overlay applies it: a German sentence with a parse
// whose root has no finite verb and subject is rejected, and a well-formed one
// carries the GDEX score.
func TestScoreSentenceQualityGermanOverlayGatesOnFiniteVerbAndSubject(t *testing.T) {
	tokens := []analyzer.Token{
		{Surface: "Das", UPOS: "DET", Dependency: "det", Head: 1},
		{Surface: "Haus", UPOS: "NOUN", Dependency: "root", Head: 1},
		{Surface: "steht", UPOS: "VERB", Dependency: "xcomp", Head: 1, Morphology: map[string]string{"VerbForm": "Inf"}},
	}
	sentence := strings.Join([]string{"Das", "Haus", "steht", "heute", "am", "alten", "See", "ruhig", "."}, " ")

	rejected := ScoreSentenceQuality("de", sentence, "Haus", 10, tokens, 0)
	assert.False(t, rejected.Accepted)
	assert.Contains(t, rejected.Reasons, reasonNoFiniteVerbAndSubject)

	accepted, ok := BestSentenceEvidenceFromCorpus(domain.SelectionCandidate{
		Language: "de", CanonicalLemma: "haus", ObservedForms: []byte(`["See"]`),
		SentenceReferences: []byte(`[{"sentence_index":0,"text":"x","location":{"start_offset":10}}]`),
	}, map[int64]analyzer.Sentence{0: {
		Text: "Der See liegt neben dem alten Haus.",
		Tokens: []analyzer.Token{
			{Surface: "Der", UPOS: "DET", Dependency: "det", Head: 1},
			{Surface: "See", UPOS: "NOUN", Dependency: "nsubj", Head: 2},
			{Surface: "liegt", UPOS: "VERB", Dependency: "root", Head: 2, Morphology: map[string]string{"VerbForm": "Fin"}},
			{Surface: "neben", UPOS: "ADP", Dependency: "case", Head: 6},
			{Surface: "dem", UPOS: "DET", Dependency: "det", Head: 6},
			{Surface: "alten", UPOS: "ADJ", Dependency: "amod", Head: 6},
			{Surface: "Haus", UPOS: "NOUN", Dependency: "obl", Head: 2},
		},
	}})
	assert.True(t, ok)
	assert.True(t, accepted.Quality.Accepted)
	assert.Greater(t, accepted.Quality.GDEXScore, float64(0))
}

// The Ancient Greek rubric keeps the generic text criteria but accepts the
// source's own sentence terminators and does not require a capitalised first
// letter, because Greek editions start sentences in lowercase.
func TestAncientGreekSentenceBoundariesAcceptSourceTerminators(t *testing.T) {
	const body = "ἔδει γὰρ αὐτὸν δῆσαι τὸν ἵππον πρὸς τὸ δένδρον"
	tests := []struct {
		name     string
		sentence string
		accepted bool
	}{
		{name: "full stop", sentence: body + ".", accepted: true},
		{name: "ascii question mark", sentence: body + ";", accepted: true},
		{name: "greek question mark", sentence: body + "\u037e", accepted: true},
		{name: "raised dot", sentence: body + "·", accepted: true},
		{name: "ano teleia", sentence: body + "\u0387", accepted: true},
		{name: "no terminator", sentence: body, accepted: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			quality := ScoreSentenceQuality("grc", tc.sentence, "δῆσαι", 10, nil, 0)
			assert.Equal(t, tc.accepted, quality.Accepted, "quality=%+v", quality)
			if tc.accepted {
				assert.NotContains(t, quality.Reasons, "incomplete sentence boundaries")
			}
		})
	}
}

// The capitalised-start requirement stays in force for every other language.
func TestLowercaseStartStillRejectedOutsideAncientGreek(t *testing.T) {
	const sentence = "unter dem alten haus spielen heute viele fröhliche kinder."
	quality := ScoreSentenceQuality("de", sentence, "haus", 10, nil, 0)
	assert.False(t, quality.Accepted)
	assert.Contains(t, quality.Reasons, "incomplete sentence boundaries")
}

// TestSentenceQualityAcceptsElidedTarget pins the sentence-quality "target
// present" check, which otherwise rejects an elided sentence before rendering.
func TestSentenceQualityAcceptsElidedTarget(t *testing.T) {
	sentence := "Je vois l'homme qui marche vers la gare."
	tokens := []analyzer.Token{
		tokenAt("Je", 0, 2), tokenAt("vois", 3, 7), tokenAt("l'", 8, 10), tokenAt("homme", 10, 15),
		tokenAt("qui", 16, 19), tokenAt("marche", 20, 26), tokenAt("vers", 27, 31), tokenAt("la", 32, 34), tokenAt("gare", 35, 39), tokenAt(".", 39, 40),
	}

	quality := ScoreSentenceQuality("fr", sentence, "homme", 0, tokens, 0)
	assert.True(t, quality.Accepted, "quality=%+v", quality)
	assert.Contains(t, quality.Reasons, "target present")

	assert.Equal(t, "homme", MatchingSentenceTarget(sentence, []string{"homme"}, tokens, 0))
	assert.Equal(t, -1, targetIndex(sentence, "homme", nil, 0))
	assert.NotEqual(t, -1, targetIndex(sentence, "homme", tokens, 0))
}
