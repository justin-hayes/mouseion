package webapp

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// studyWord builds one analyzed word at a code-point span, offset by start.
func studyWord(ordinal, start, end int64, surface, upos string, morphology map[string]string) domain.SentenceStudyToken {
	return domain.SentenceStudyToken{Surface: surface, RawLemma: surface, EffectiveLemma: surface, UPOS: upos,
		Dependency: "dep", Ordinal: ordinal, StartOffset: start, EndOffset: end, Morphology: morphology}
}

// studySentence assembles a sentence whose authoritative source slice is the
// whole unit text.
func studySentence(text string, end int64, tokens ...domain.SentenceStudyToken) domain.SentenceStudy {
	return domain.SentenceStudy{SentenceText: text, UnitText: text, SentenceStartOffset: 0, SentenceEndOffset: end, Tokens: tokens}
}

// studyWords returns the aligned word units in source order.
func studyWords(line sentenceStudyLine) []sentenceStudyWord {
	var words []sentenceStudyWord
	for _, segment := range line.Segments {
		if segment.Word != nil {
			words = append(words, *segment.Word)
		}
	}
	return words
}

// studySource returns the line's exact source text as it will be read.
func studySource(line sentenceStudyLine) string {
	var builder strings.Builder
	for _, segment := range line.Segments {
		if segment.Word != nil {
			builder.WriteString(segment.Word.Text)
		} else {
			builder.WriteString(segment.Text)
		}
	}
	return builder.String()
}

func TestSentenceStudyLineAlignsOneToOne(t *testing.T) {
	study := studySentence("Das Haus steht.", 15,
		studyWord(0, 0, 3, "Das", "DET", map[string]string{"Case": "Nom", "Number": "Sing"}),
		studyWord(1, 4, 8, "Haus", "NOUN", map[string]string{"Case": "Nom"}),
		studyWord(2, 9, 14, "steht", "VERB", map[string]string{"Person": "3", "Number": "Sing"}),
		studyWord(3, 14, 15, ".", "PUNCT", nil),
	)
	line := alignSentenceStudy(study)
	require.False(t, line.WholeLineFallback)
	require.False(t, line.Degraded)
	assert.Equal(t, "Das Haus steht.", studySource(line))
	words := studyWords(line)
	require.Len(t, words, 3)
	assert.Equal(t, "Das", words[0].Text)
	assert.Equal(t, "Haus", words[1].Text)
	assert.Equal(t, "steht", words[2].Text)
	// Punctuation is source text, not a stacked unit.
	assert.NotContains(t, studySource(line), "\ufffd")
	assert.Equal(t, "SG.NOM", sentenceStudyWordLabels(words[0]))
}

func TestSentenceStudyLineLocalFallbackKeepsNeighbours(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		unsafe   []domain.SentenceStudyToken
		safeText string
	}{
		{
			name: "german zum contraction",
			text: "zum Haus",
			unsafe: []domain.SentenceStudyToken{
				studyWord(0, 0, 3, "zu", "ADP", nil),
				studyWord(1, 0, 3, "dem", "DET", nil),
			},
			safeText: "Haus",
		},
		{
			name: "german im contraction",
			text: "im Haus",
			unsafe: []domain.SentenceStudyToken{
				studyWord(0, 0, 2, "in", "ADP", nil),
				studyWord(1, 0, 2, "dem", "DET", nil),
			},
			safeText: "Haus",
		},
		{
			name: "italian del contraction",
			text: "del libro",
			unsafe: []domain.SentenceStudyToken{
				studyWord(0, 0, 3, "di", "ADP", nil),
				studyWord(1, 0, 3, "il", "DET", nil),
			},
			safeText: "libro",
		},
		{
			name: "italian dell apostrophe contraction",
			text: "dell’acqua",
			unsafe: []domain.SentenceStudyToken{
				studyWord(0, 0, 5, "di", "ADP", nil),
				studyWord(1, 0, 5, "la", "DET", nil),
			},
			safeText: "acqua",
		},
		{
			// A legacy shared-span shape: the two analyzed words keep one span
			// and one surface, so neither is uniquely attributable. This is the
			// contract every language's shared spans must degrade under; the
			// configured Greek `mwt` model happens to split στο/στην instead (see
			// sentence_study_captured_test.go).
			name: "shared-span article contraction",
			text: "στην πόλη",
			unsafe: []domain.SentenceStudyToken{
				studyWord(0, 0, 4, "σε", "ADP", nil),
				studyWord(1, 0, 4, "την", "DET", nil),
			},
			safeText: "πόλη",
		},
		{
			name: "shared-span article contraction with mismatched surface",
			text: "στο σπίτι",
			unsafe: []domain.SentenceStudyToken{
				studyWord(0, 0, 3, "σε", "ADP", nil),
				studyWord(1, 0, 3, "το", "DET", nil),
			},
			safeText: "σπίτι",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prefix, _, found := strings.Cut(test.text, test.safeText)
			require.True(t, found, "%q must contain %q", test.text, test.safeText)
			start := int64(utf8.RuneCountInString(prefix))
			tokens := append([]domain.SentenceStudyToken{}, test.unsafe...)
			tokens = append(tokens, studyWord(int64(len(tokens)), start, start+int64(len([]rune(test.safeText))), test.safeText, "NOUN", map[string]string{"Case": "Nom"}))
			study := studySentence(test.text, int64(len([]rune(test.text))), tokens...)
			line := alignSentenceStudy(study)

			require.True(t, line.Degraded, "shared or mismatched spans degrade locally")
			assert.Equal(t, test.text, studySource(line), "the original source text is shown unannotated")
			words := studyWords(line)
			require.Len(t, words, 1)
			assert.Equal(t, test.safeText, words[0].Text, "safe neighbouring annotations are kept")
		})
	}
}

func TestSentenceStudyLineWholeLineFallback(t *testing.T) {
	tests := []struct {
		name  string
		study domain.SentenceStudy
	}{
		{
			name: "stored text disagrees with the source slice",
			study: domain.SentenceStudy{SentenceText: "Der Hund.", UnitText: "Das Haus.", SentenceStartOffset: 0, SentenceEndOffset: 9,
				Tokens: []domain.SentenceStudyToken{studyWord(0, 0, 3, "Der", "DET", nil)}},
		},
		{
			name: "sentence bounds exceed the immutable unit",
			study: domain.SentenceStudy{SentenceText: "Das Haus.", UnitText: "Das Haus.", SentenceStartOffset: 0, SentenceEndOffset: 40,
				Tokens: []domain.SentenceStudyToken{studyWord(0, 0, 3, "Das", "DET", nil)}},
		},
		{
			name: "sentence start precedes the unit",
			study: domain.SentenceStudy{SentenceText: "Das", UnitText: "Das", SentenceStartOffset: -1, SentenceEndOffset: 3,
				Tokens: []domain.SentenceStudyToken{studyWord(0, 0, 3, "Das", "DET", nil)}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			line := alignSentenceStudy(test.study)
			assert.True(t, line.WholeLineFallback)
			assert.Empty(t, line.Segments, "whole-line fallback withholds the line")
		})
	}
}

func TestSentenceStudyLineKeepsRepeatedFormsDistinct(t *testing.T) {
	study := studySentence("Haus und Haus", 13,
		studyWord(0, 0, 4, "Haus", "NOUN", map[string]string{"Number": "Sing"}),
		studyWord(1, 5, 8, "und", "CCONJ", nil),
		studyWord(2, 9, 13, "Haus", "NOUN", map[string]string{"Number": "Plur"}),
	)
	line := alignSentenceStudy(study)
	require.False(t, line.Degraded)
	words := studyWords(line)
	require.Len(t, words, 3)
	assert.Equal(t, "Haus", words[0].Text)
	assert.Equal(t, "Haus", words[2].Text)
	assert.Equal(t, "SG", sentenceStudyWordLabels(words[0]))
	assert.Equal(t, "PL", sentenceStudyWordLabels(words[2]))
}

func TestSentenceStudyLineLeavesEmptyMorphologyUnannotated(t *testing.T) {
	study := studySentence("gut", 3, studyWord(0, 0, 3, "gut", "ADV", map[string]string{}))
	line := alignSentenceStudy(study)
	require.False(t, line.WholeLineFallback)
	require.True(t, line.HasUnannotated)
	require.Len(t, line.Segments, 1)
	require.NotNil(t, line.Segments[0].Word)
	assert.Empty(t, line.Segments[0].Word.Labels)
	assert.Empty(t, sentenceStudyWordLabels(*line.Segments[0].Word))
}

func TestSentenceStudyLineJoinsSuppliedAlternativesWithSlash(t *testing.T) {
	study := studySentence("Haus", 4, studyWord(0, 0, 4, "Haus", "NOUN", map[string]string{"Case": "Nom,Acc", "Number": "Sing"}))
	line := alignSentenceStudy(study)
	require.True(t, line.HasAlternatives)
	words := studyWords(line)
	require.Len(t, words, 1)
	assert.Equal(t, "SG.NOM/ACC", sentenceStudyWordLabels(words[0]))
}

func TestSentenceStudyLineUsesCodePointOffsetsForGreek(t *testing.T) {
	study := studySentence("ΟΔΌΣ σπίτι", 10,
		studyWord(0, 0, 4, "ΟΔΌΣ", "NOUN", map[string]string{"Case": "Nom"}),
		studyWord(1, 5, 10, "σπίτι", "NOUN", map[string]string{"Case": "Acc"}),
	)
	line := alignSentenceStudy(study)
	require.False(t, line.Degraded)
	assert.Equal(t, "ΟΔΌΣ σπίτι", studySource(line))
	words := studyWords(line)
	require.Len(t, words, 2)
	assert.Equal(t, "ΟΔΌΣ", words[0].Text)
	assert.Equal(t, "σπίτι", words[1].Text)
}

func TestSentenceStudyLineSurvivesMissingOrInvalidSyntax(t *testing.T) {
	missing := studyWord(0, 0, 4, "Haus", "NOUN", map[string]string{"Case": "Nom"})
	missing.Dependency = ""
	invalid := studyWord(1, 5, 10, "steht", "VERB", map[string]string{"Person": "3", "Number": "Sing"})
	invalid.Dependency = "root"
	invalid.HeadOrdinal = 99
	study := studySentence("Haus steht", 10, missing, invalid)
	line := alignSentenceStudy(study)
	require.False(t, line.WholeLineFallback)
	require.False(t, line.Degraded)
	words := studyWords(line)
	require.Len(t, words, 2)
	assert.Equal(t, "NOM", sentenceStudyWordLabels(words[0]))
	assert.Equal(t, "3SG", sentenceStudyWordLabels(words[1]))
}

func TestSentenceStudyLineWithoutATargetEmphasizesNothing(t *testing.T) {
	study := studySentence("Haus", 4, studyWord(0, 0, 4, "Haus", "NOUN", map[string]string{"Case": "Nom"}))
	study.TargetOrdinal = 42
	line := alignSentenceStudy(study)
	for _, word := range studyWords(line) {
		assert.False(t, word.Target, "a target missing from the evidence is not emphasized")
	}
}

func TestSentenceStudyCoreLabelsOrderAndFusion(t *testing.T) {
	labels := sentenceStudyCoreLabels(map[string]string{
		"Person": "3", "Number": "Sing", "Gender": "Fem", "Case": "Nom",
		"Tense": "Pres", "Aspect": "Imp", "Mood": "Ind", "Voice": "Act", "VerbForm": "Fin",
	})
	rendered := make([]string, 0, len(labels))
	for _, label := range labels {
		rendered = append(rendered, studyLabelText(label))
	}
	assert.Equal(t, []string{"3SG", "F", "NOM", "PRS", "IPFV", "IND", "active", "finite"}, rendered)
}

// studyLabelText renders a single label the same way the view does, so the
// tests do not reimplement the slash join.
func studyLabelText(label sentenceStudyLabel) string {
	return sentenceStudyWordLabels(sentenceStudyWord{Labels: []sentenceStudyLabel{label}})
}

func TestSentenceStudyLabelsApplyTheAbbreviationTable(t *testing.T) {
	tests := map[string]sentenceStudyLabelPart{
		"Case\x00Gen":      {Abbreviation: "GEN", Expansion: "genitive case, the case of possession or belonging"},
		"Number\x00Plur":   {Abbreviation: "PL", Expansion: "plural"},
		"Gender\x00Masc":   {Abbreviation: "M", Expansion: "masculine"},
		"Tense\x00Pqp":     {Abbreviation: "pluperfect", Expansion: "pluperfect tense, a past situation before another past situation"},
		"Mood\x00Sub":      {Abbreviation: "SBJV", Expansion: "subjunctive mood, for possibility, wish or doubt"},
		"Voice\x00Pass":    {Abbreviation: "PASS", Expansion: "passive voice, where the subject is acted on"},
		"VerbForm\x00Conv": {Abbreviation: "CVB", Expansion: "converb, a verb form that links clauses"},
		"VerbForm\x00Ger":  {Abbreviation: "gerund", Expansion: "gerund, a verb form used like a noun"},
		"Case\x00Xyz":      {Abbreviation: "Xyz", Expansion: "no standard abbreviation; shown as the analyzer's raw case value"},
		"Tense\x00Pqp2":    {Abbreviation: "Pqp2", Expansion: "no standard abbreviation; shown as the analyzer's raw tense value"},
	}
	for key, want := range tests {
		t.Run(key, func(t *testing.T) {
			feature, value, _ := strings.Cut(key, "\x00")
			require.Equal(t, want, sentenceStudyMapFeatureValue(feature, value))
		})
	}
}

func TestSentenceStudyItalianImperfectIsNotGreekImperfective(t *testing.T) {
	italian := sentenceStudyCoreLabels(map[string]string{"Tense": "Imp"})
	greek := sentenceStudyCoreLabels(map[string]string{"Aspect": "Imp"})
	require.Len(t, italian, 1)
	require.Len(t, greek, 1)
	require.Equal(t, "imperfect", studyLabelText(italian[0]))
	require.Equal(t, "IPFV", studyLabelText(greek[0]))
	require.NotEqual(t, studyLabelText(italian[0]), studyLabelText(greek[0]))
}

func TestSentenceStudyKeyListsOnlyUsedAbbreviationsSorted(t *testing.T) {
	study := studySentence("Haus steht", 10,
		studyWord(0, 0, 4, "Haus", "NOUN", map[string]string{"Case": "Nom", "Number": "Sing"}),
		studyWord(1, 5, 10, "steht", "VERB", map[string]string{"Person": "3", "Number": "Sing"}),
	)
	line := alignSentenceStudy(study)
	entries := sentenceStudyKey(line)
	abbreviations := make([]string, 0, len(entries))
	for _, entry := range entries {
		abbreviations = append(abbreviations, entry.Abbreviation)
	}
	assert.Equal(t, []string{"3SG", "NOM", "SG"}, abbreviations)
}

// studyNotationKey returns the rendered notation key disclosure, or "" when the
// page does not show one.
func studyNotationKey(body string) string {
	const marker = `<details id="sentence-study-notation-key"`
	start := strings.Index(body, marker)
	if start < 0 {
		return ""
	}
	end := strings.Index(body[start:], "</details>")
	if end < 0 {
		return body[start:]
	}
	return body[start : start+end]
}

func TestSentenceStudyNotationKeyExplainsTermsForGermanItalianAndGreek(t *testing.T) {
	t.Run("german explains each case term it uses", func(t *testing.T) {
		study := studySentence("Ich helfe dem Mann.", 19,
			studyWord(0, 0, 3, "Ich", "PRON", map[string]string{"Person": "1", "Number": "Sing", "Case": "Nom"}),
			studyWord(1, 4, 9, "helfe", "VERB", map[string]string{"Person": "1", "Number": "Sing", "Tense": "Pres", "Mood": "Ind", "VerbForm": "Fin"}),
			studyWord(2, 10, 13, "dem", "DET", map[string]string{"Case": "Dat", "Gender": "Masc", "Number": "Sing"}),
			studyWord(3, 14, 18, "Mann", "NOUN", map[string]string{"Case": "Dat", "Gender": "Masc", "Number": "Sing"}),
			studyWord(4, 18, 19, ".", "PUNCT", nil))
		key := studyNotationKey(renderSentenceStudy(t, study))
		require.NotEmpty(t, key, "a shown morphology line is accompanied by the key")
		for _, want := range []string{
			`<dt>1SG</dt><dd>first person singular</dd>`,
			`<dt>M</dt><dd>masculine</dd>`,
			`<dt>NOM</dt><dd>nominative case, the case of the subject</dd>`,
			`<dt>DAT</dt><dd>dative case, the case of the indirect object or recipient</dd>`,
			`<dt>PRS</dt><dd>present tense, for what happens now or is generally true</dd>`,
			`<dt>IND</dt><dd>indicative mood, stating what is the case</dd>`,
			`<dt>finite</dt><dd>finite verb form, carrying tense and agreeing with the subject</dd>`,
		} {
			require.Contains(t, key, want)
		}
		// Exactly the used labels; unused case values stay out.
		require.NotContains(t, key, `<dt>ACC</dt>`)
		require.NotContains(t, key, `<dt>GEN</dt>`)
	})

	t.Run("italian imperfect tense is explained as a tense", func(t *testing.T) {
		study := studySentence("Parlavamo.", 10,
			studyWord(0, 0, 9, "Parlavamo", "VERB", map[string]string{"Person": "1", "Number": "Plur", "Tense": "Imp", "Mood": "Ind", "VerbForm": "Fin"}),
			studyWord(1, 9, 10, ".", "PUNCT", nil))
		key := studyNotationKey(renderSentenceStudy(t, study))
		require.Contains(t, key, `<dt>imperfect</dt><dd>imperfect tense, an ongoing or repeated past situation</dd>`)
		require.Contains(t, key, `<dt>1PL</dt><dd>first person plural</dd>`)
		require.NotContains(t, key, "IPFV")
		require.NotContains(t, key, "imperfective")
	})

	t.Run("greek imperfective aspect is explained as an aspect", func(t *testing.T) {
		study := studySentence("Έγραφε.", 7,
			studyWord(0, 0, 6, "Έγραφε", "VERB", map[string]string{"Person": "3", "Number": "Sing", "Aspect": "Imp", "Mood": "Ind", "VerbForm": "Fin"}),
			studyWord(1, 6, 7, ".", "PUNCT", nil))
		key := studyNotationKey(renderSentenceStudy(t, study))
		require.Contains(t, key, `<dt>IPFV</dt><dd>imperfective aspect, an ongoing or repeated situation without a stated endpoint</dd>`)
		require.Contains(t, key, `<dt>3SG</dt><dd>third person singular</dd>`)
		require.NotContains(t, key, "imperfect tense")
	})
}

func TestSentenceStudyNotationKeyExplainsTheSlashOnlyWhenAlternativesAppear(t *testing.T) {
	const slashNote = "A slash separates analyzer-supplied alternatives"

	withAlternatives := studySentence("Haus", 4,
		studyWord(0, 0, 4, "Haus", "NOUN", map[string]string{"Case": "Nom,Acc", "Number": "Sing"}))
	key := studyNotationKey(renderSentenceStudy(t, withAlternatives))
	require.Contains(t, key, slashNote)
	require.Contains(t, key, `<dt>NOM</dt>`)
	require.Contains(t, key, `<dt>ACC</dt>`)

	withoutAlternatives := studySentence("Haus", 4,
		studyWord(0, 0, 4, "Haus", "NOUN", map[string]string{"Case": "Nom", "Number": "Sing"}))
	key = studyNotationKey(renderSentenceStudy(t, withoutAlternatives))
	require.NotContains(t, key, slashNote)
}

func TestSentenceStudyNotationKeyAlwaysExplainsUnannotatedWords(t *testing.T) {
	const unannotated = "that does not mean the word lacks those features"

	annotated := studySentence("Haus", 4, studyWord(0, 0, 4, "Haus", "NOUN", map[string]string{"Case": "Nom"}))
	key := studyNotationKey(renderSentenceStudy(t, annotated))
	require.Contains(t, key, unannotated, "the key explains unannotated words even when none appear")

	emptyMorphology := studySentence("gut", 3, studyWord(0, 0, 3, "gut", "ADV", map[string]string{}))
	key = studyNotationKey(renderSentenceStudy(t, emptyMorphology))
	require.Contains(t, key, unannotated)
}

func TestSentenceStudyNotationKeyMakesNoSentenceSpecificClaims(t *testing.T) {
	study := studySentence("Ich helfe dem Mann.", 19,
		studyWord(0, 0, 3, "Ich", "PRON", map[string]string{"Person": "1", "Number": "Sing", "Case": "Nom"}),
		studyWord(1, 4, 9, "helfe", "VERB", map[string]string{"Person": "1", "Number": "Sing", "Tense": "Pres", "Mood": "Ind", "VerbForm": "Fin"}),
		studyWord(2, 10, 13, "dem", "DET", map[string]string{"Case": "Dat", "Gender": "Masc", "Number": "Sing"}),
		studyWord(3, 14, 18, "Mann", "NOUN", map[string]string{"Case": "Dat", "Gender": "Masc", "Number": "Sing"}))
	key := studyNotationKey(renderSentenceStudy(t, study))
	require.NotEmpty(t, key)
	require.NotContains(t, key, "because")
	for _, surface := range []string{"Ich", "helfe", "dem", "Mann"} {
		require.NotContains(t, key, surface, "the key explains terms in general, never this sentence's words")
	}
}

func TestSentenceStudyNotationKeyIsAbsentWithoutAMorphologyLine(t *testing.T) {
	t.Run("no analyzer evidence", func(t *testing.T) {
		study := domain.SentenceStudy{BookTitle: "Buch", SentenceText: "Das Haus.", UnitText: "Das Haus.",
			SentenceStartOffset: 0, SentenceEndOffset: 9, TargetSurface: "Haus"}
		require.Empty(t, studyNotationKey(renderSentenceStudy(t, study)))
	})
	t.Run("whole-line fallback", func(t *testing.T) {
		study := domain.SentenceStudy{BookTitle: "Buch", SentenceText: "Das Haus.", UnitText: "Anderer Text.",
			SentenceStartOffset: 0, SentenceEndOffset: 11, TargetSurface: "Haus",
			Tokens: []domain.SentenceStudyToken{studyWord(0, 0, 3, "Das", "DET", nil)}}
		require.Empty(t, studyNotationKey(renderSentenceStudy(t, study)))
	})
}

func TestSentenceStudyLineDegradesASharedSpanSiblingEvenWhenOneSurfaceMatches(t *testing.T) {
	// A legacy contraction may store one word whose surface happens to equal the
	// shared source text alongside its siblings. It is still not uniquely
	// attributable, so the whole shared region degrades.
	study := studySentence("zum Haus", 8,
		studyWord(0, 0, 3, "zum", "ADP", map[string]string{"Case": "Dat"}),
		studyWord(1, 0, 3, "dem", "DET", nil),
		studyWord(2, 4, 8, "Haus", "NOUN", map[string]string{"Case": "Dat"}),
	)
	line := alignSentenceStudy(study)
	require.True(t, line.Degraded)
	words := studyWords(line)
	require.Len(t, words, 1)
	require.Equal(t, "Haus", words[0].Text, "the safe neighbour is still annotated")
	require.Equal(t, "zum Haus", studySource(line))
}

func TestSentenceStudyLineIgnoresUnsafePunctuationWhenDecidingDegraded(t *testing.T) {
	study := studySentence("Haus .", 6,
		studyWord(0, 0, 4, "Haus", "NOUN", map[string]string{"Case": "Nom"}),
		studyWord(1, 90, 91, ".", "PUNCT", nil),
	)
	line := alignSentenceStudy(study)
	require.False(t, line.Degraded, "a degraded punctuation span is not a word-level local fallback")
	require.Equal(t, "Haus .", studySource(line))
}

func renderSentenceStudy(t *testing.T, study domain.SentenceStudy) string {
	t.Helper()
	var rendered bytes.Buffer
	require.NoError(t, VocabularySentenceStudyPageView(domain.User{}, "", study, "/vocabulary/concordance").Render(t.Context(), &rendered))
	return rendered.String()
}

func TestSentenceStudyPageRendersTheMorphologyLineAndKey(t *testing.T) {
	study := studySentence("Das Haus sieht gut aus.", 23, studyWord(0, 0, 3, "Das", "DET", map[string]string{"Case": "Nom", "Number": "Sing"}),
		studyWord(1, 4, 8, "Haus", "NOUN", map[string]string{"Case": "Nom", "Number": "Sing"}),
		studyWord(2, 9, 14, "sieht", "VERB", map[string]string{"Person": "3", "Number": "Sing", "Tense": "Pres", "Mood": "Ind", "VerbForm": "Fin"}),
		studyWord(3, 15, 18, "gut", "ADV", map[string]string{}),
		studyWord(4, 19, 22, "aus", "PART", map[string]string{"PartType": "Sep"}),
		studyWord(5, 22, 23, ".", "PUNCT", nil))
	study.TargetOrdinal = 1
	body := renderSentenceStudy(t, study)

	t.Run("renders each aligned word with its stacked labels", func(t *testing.T) {
		for _, want := range []string{
			`<p class="sentence-study-line">`,
			`<strong class="sentence-study-word">Haus</strong>`,
			`<span class="sentence-study-word">Das</span>`,
			`<span class="sentence-study-labels">SG.NOM</span>`,
			`<span class="sentence-study-labels">3SG.PRS.IND.finite</span>`,
		} {
			require.Contains(t, body, want)
		}
	})
	t.Run("names the notation key and explains the used labels in plain English", func(t *testing.T) {
		for _, want := range []string{
			`id="sentence-study-notation-key"`,
			`<summary>Notation key</summary>`,
			`<dt>SG</dt><dd>singular</dd>`,
			`<dt>NOM</dt><dd>nominative case, the case of the subject</dd>`,
			`<dt>3SG</dt><dd>third person singular</dd>`,
		} {
			require.Contains(t, body, want)
		}
		// Unused labels stay out of the key.
		require.NotContains(t, body, `<dt>DAT</dt>`)
		require.NotContains(t, body, `<dt>ACC</dt>`)
	})
	t.Run("keeps unannotated words as unlabelled source words with one note", func(t *testing.T) {
		require.Contains(t, body, `<span class="sentence-study-word">gut</span>`)
		require.Contains(t, body, `<span class="sentence-study-word">aus</span>`)
		require.Contains(t, body, "A word shown without labels had no core grammatical features supplied by the analyzer; that does not mean the word lacks those features.")
	})
	t.Run("preserves the source spacing between words", func(t *testing.T) {
		require.Contains(t, body, `<span class="sentence-study-source"> `)
	})
	t.Run("keeps the text alternative closed by default", func(t *testing.T) {
		_, details, ok := strings.Cut(body, `<details id="sentence-study-text-alternative"`)
		require.True(t, ok, "the complete text alternative is not rendered")
		open := strings.Index(details, ">")
		require.NotContains(t, details[:open], "open")
	})
}

func TestSentenceStudyPageWholeLineFallbackOpensTheTextAlternative(t *testing.T) {
	study := domain.SentenceStudy{BookTitle: "Buch", SentenceText: "Das Haus.", UnitText: "Anderer Text.", SentenceStartOffset: 0, SentenceEndOffset: 13,
		Tokens: []domain.SentenceStudyToken{studyWord(0, 0, 3, "Das", "DET", nil)}}
	body := renderSentenceStudy(t, study)
	t.Run("withholds the line and key", func(t *testing.T) {
		require.NotContains(t, body, `class="sentence-study-line"`)
		require.NotContains(t, body, `id="sentence-study-notation-key"`)
	})
	t.Run("explains why the line is withheld", func(t *testing.T) {
		require.Contains(t, body, "Word-level labels are withheld because the sentence could not be verified against its stored source text.")
	})
	t.Run("opens the text alternative by default", func(t *testing.T) {
		_, details, ok := strings.Cut(body, `<details id="sentence-study-text-alternative"`)
		require.True(t, ok, "the complete text alternative is not rendered")
		open := strings.Index(details, ">")
		require.Contains(t, details[:open], "open")
	})
}

func TestSentenceStudyPageLocalFallbackNamesTheTextAlternative(t *testing.T) {
	study := studySentence("zum Haus", 8,
		studyWord(0, 0, 3, "zu", "ADP", nil),
		studyWord(1, 0, 3, "dem", "DET", nil),
		studyWord(2, 4, 8, "Haus", "NOUN", map[string]string{"Case": "Dat"}))
	body := renderSentenceStudy(t, study)
	t.Run("shows a brief note that names the degraded region", func(t *testing.T) {
		require.Contains(t, body, `class="sentence-study-note"`)
		require.Contains(t, body, "Some words could not be aligned safely to their source text (zum)")
	})
	t.Run("names the text alternative", func(t *testing.T) {
		require.Contains(t, body, "their analyzer evidence is listed in the complete text alternative")
	})
	t.Run("keeps the neighbouring safe annotation", func(t *testing.T) {
		require.Contains(t, body, `<span class="sentence-study-word">Haus</span>`)
	})
	t.Run("shows the degraded source region unannotated", func(t *testing.T) {
		require.Contains(t, body, `<span class="sentence-study-source">zum </span>`)
	})
}
