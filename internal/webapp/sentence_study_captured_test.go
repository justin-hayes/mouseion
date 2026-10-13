package webapp

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file carries deterministic regression fixtures captured from the
// configured deployment models for #2001: Stanza 1.15.0 with the `de` and `it`
// default packages and the Greek `gdt`/`gdt_nocharlm`/`gdt_greek-bert`
// selection from model_config.py, run through mouseion_nlp.server.
// AnalyzerServicer (the same code path the NLP gRPC service serves). Every
// sentence below is public or invented; no private-book text is present.
//
// The fixtures record the persisted shape the reader actually receives: the
// immutable unit text, the sentence bounds inside it, and each analyzed word's
// code-point span, surface, UPOS and unfiltered morphology. The comments name
// what the real model produced where it differs from the synthetic fixtures.

// capturedWord is one aligned word unit as the reader renders it.
type capturedWord struct {
	Text   string
	Labels string
}

// capturedWords lists the aligned word units in source order.
func capturedWords(line sentenceStudyLine) []capturedWord {
	words := make([]capturedWord, 0)
	for _, segment := range line.Segments {
		if segment.Word == nil {
			continue
		}
		words = append(words, capturedWord{Text: segment.Word.Text, Labels: sentenceStudyWordLabels(*segment.Word)})
	}
	return words
}

// capturedLabelText renders one morphology map exactly as the morphology line
// does, without rebuilding a whole word unit by hand.
func capturedLabelText(morphology map[string]string) string {
	return sentenceStudyWordLabels(sentenceStudyWord{Labels: sentenceStudyCoreLabels(morphology)})
}

// capturedGreekStoStudy is the configured gdt model's split of στο/στην into
// distinct `σ` + article sub-spans, captured from the gdt pipeline. It is shared
// by the alignment and presentation tests.
func capturedGreekStoStudy() domain.SentenceStudy {
	return studySentence("Πηγαίνω στο σπίτι και στην πόλη.", 32,
		studyWord(0, 0, 7, "Πηγαίνω", "VERB", map[string]string{"Aspect": "Imp", "Mood": "Ind", "Number": "Sing", "Person": "1", "Tense": "Pres", "VerbForm": "Fin", "Voice": "Act"}),
		studyWord(1, 8, 9, "σ", "ADP", nil),
		studyWord(2, 9, 11, "το", "DET", map[string]string{"Case": "Acc", "Definite": "Def", "Gender": "Neut", "Number": "Sing", "PronType": "Art"}),
		studyWord(3, 12, 17, "σπίτι", "NOUN", map[string]string{"Case": "Acc", "Gender": "Neut", "Number": "Sing"}),
		studyWord(4, 18, 21, "και", "CCONJ", nil),
		studyWord(5, 22, 23, "σ", "ADP", nil),
		studyWord(6, 23, 26, "την", "DET", map[string]string{"Case": "Acc", "Definite": "Def", "Gender": "Fem", "Number": "Sing", "PronType": "Art"}),
		studyWord(7, 27, 31, "πόλη", "NOUN", map[string]string{"Case": "Acc", "Gender": "Fem", "Number": "Sing"}),
		studyWord(8, 31, 32, ".", "PUNCT", nil),
	)
}

// capturedGreekUppercaseStudy is the configured model's poorly tokenized
// uppercase-accented Greek, captured from the gdt pipeline: the final noun's
// span includes the sentence-final period (which the producer strips from the
// surface) and ΣΤΗΝ is tagged NOUN. Shared by the alignment and presentation
// tests.
func capturedGreekUppercaseStudy() domain.SentenceStudy {
	return studySentence("ΤΑ ΠΑΙΔΙΆ ΠΑΊΖΟΥΝ ΣΤΗΝ ΑΥΛΉ.", 28,
		studyWord(0, 0, 2, "ΤΑ", "DET", map[string]string{"Case": "Nom", "Definite": "Def", "Gender": "Neut", "Number": "Plur", "PronType": "Art"}),
		studyWord(1, 3, 9, "ΠΑΙΔΙΆ", "NOUN", map[string]string{"Case": "Nom", "Gender": "Neut", "Number": "Plur"}),
		studyWord(2, 10, 17, "ΠΑΊΖΟΥΝ", "VERB", map[string]string{"Aspect": "Imp", "Mood": "Ind", "Number": "Plur", "Person": "3", "Tense": "Pres", "VerbForm": "Fin", "Voice": "Act"}),
		studyWord(3, 18, 22, "ΣΤΗΝ", "NOUN", map[string]string{"Abbr": "Yes"}),
		studyWord(4, 23, 28, "ΑΥΛΉ", "NOUN", map[string]string{"Abbr": "Yes"}),
	)
}

// TestSentenceStudyCapturedConfiguredModelAlignment feeds the captured
// configured-model shapes through the reader and pins which words align, which
// degrade locally, and how the original source text is preserved.
func TestSentenceStudyCapturedConfiguredModelAlignment(t *testing.T) {
	tests := []struct {
		name         string
		study        domain.SentenceStudy
		wantDegraded bool
		wantRegions  []string
		wantWords    []capturedWord
	}{
		{
			// German separable-prefix/preposition contractions are multi-word
			// tokens: both words keep the token span, so neither is uniquely
			// attributable and the region degrades locally.
			name: "german zum and im share a span and degrade",
			study: studySentence("Ich gehe zum Markt im Dorf.", 27,
				studyWord(0, 0, 3, "Ich", "PRON", map[string]string{"Case": "Nom", "Number": "Sing", "Person": "1", "PronType": "Prs"}),
				studyWord(1, 4, 8, "gehe", "VERB", map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "1", "Tense": "Pres", "VerbForm": "Fin"}),
				studyWord(2, 9, 12, "zu", "ADP", nil),
				studyWord(3, 9, 12, "dem", "DET", map[string]string{"Case": "Dat", "Definite": "Def", "Gender": "Masc", "Number": "Sing", "PronType": "Art"}),
				studyWord(4, 13, 18, "Markt", "NOUN", map[string]string{"Case": "Dat", "Gender": "Masc", "Number": "Sing"}),
				studyWord(5, 19, 21, "in", "ADP", nil),
				studyWord(6, 19, 21, "dem", "DET", map[string]string{"Case": "Dat", "Definite": "Def", "Gender": "Neut", "Number": "Sing", "PronType": "Art"}),
				studyWord(7, 22, 26, "Dorf", "NOUN", map[string]string{"Case": "Dat", "Gender": "Neut", "Number": "Sing"}),
				studyWord(8, 26, 27, ".", "PUNCT", nil),
			),
			wantDegraded: true,
			wantRegions:  []string{"zum", "im"},
			wantWords: []capturedWord{
				{"Ich", "1SG.NOM"},
				{"gehe", "1SG.PRS.IND.finite"},
				{"Markt", "SG.M.DAT"},
				{"Dorf", "SG.N.DAT"},
			},
		},
		{
			// Sentence-initial `Am` expands to a capitalised `An`, which does
			// not match the source slice; it still shares the span with `dem`.
			name: "german am expands to capitalised an and degrades",
			study: studySentence("Am Abend geht sie zum Bahnhof.", 30,
				studyWord(0, 0, 2, "An", "ADP", nil),
				studyWord(1, 0, 2, "dem", "DET", map[string]string{"Case": "Dat", "Definite": "Def", "Gender": "Masc", "Number": "Sing", "PronType": "Art"}),
				studyWord(2, 3, 8, "Abend", "NOUN", map[string]string{"Case": "Dat", "Gender": "Masc", "Number": "Sing"}),
				studyWord(3, 9, 13, "geht", "VERB", map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "3", "Tense": "Pres", "VerbForm": "Fin"}),
				studyWord(4, 14, 17, "sie", "PRON", map[string]string{"Case": "Nom", "Gender": "Fem", "Number": "Sing", "Person": "3", "PronType": "Prs"}),
				studyWord(5, 18, 21, "zu", "ADP", nil),
				studyWord(6, 18, 21, "dem", "DET", map[string]string{"Case": "Dat", "Definite": "Def", "Gender": "Masc", "Number": "Sing", "PronType": "Art"}),
				studyWord(7, 22, 29, "Bahnhof", "NOUN", map[string]string{"Case": "Dat", "Gender": "Masc", "Number": "Sing"}),
				studyWord(8, 29, 30, ".", "PUNCT", nil),
			),
			wantDegraded: true,
			wantRegions:  []string{"Am", "zum"},
			wantWords: []capturedWord{
				{"Abend", "SG.M.DAT"},
				{"geht", "3SG.PRS.IND.finite"},
				{"sie", "3SG.F.NOM"},
				{"Bahnhof", "SG.M.DAT"},
			},
		},
		{
			// Italian articulated prepositions are multi-word tokens with a
			// shared span; the ASCII apostrophe form behaves the same.
			name: "italian del and dell' share a span and degrade",
			study: studySentence("Il colore del cielo e dell'acqua mi piace.", 42,
				studyWord(0, 0, 2, "Il", "DET", map[string]string{"Definite": "Def", "Gender": "Masc", "Number": "Sing", "PronType": "Art"}),
				studyWord(1, 3, 9, "colore", "NOUN", map[string]string{"Gender": "Masc", "Number": "Sing"}),
				studyWord(2, 10, 13, "di", "ADP", nil),
				studyWord(3, 10, 13, "il", "DET", map[string]string{"Definite": "Def", "Gender": "Masc", "Number": "Sing", "PronType": "Art"}),
				studyWord(4, 14, 19, "cielo", "NOUN", map[string]string{"Gender": "Masc", "Number": "Sing"}),
				studyWord(5, 20, 21, "e", "CCONJ", nil),
				studyWord(6, 22, 27, "di", "ADP", nil),
				studyWord(7, 22, 27, "l'", "DET", map[string]string{"Definite": "Def", "Number": "Sing", "PronType": "Art"}),
				studyWord(8, 27, 32, "acqua", "NOUN", map[string]string{"Gender": "Fem", "Number": "Sing"}),
				studyWord(9, 33, 35, "mi", "PRON", map[string]string{"Clitic": "Yes", "Number": "Sing", "Person": "1", "PronType": "Prs"}),
				studyWord(10, 36, 41, "piace", "VERB", map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "3", "Tense": "Pres", "VerbForm": "Fin"}),
				studyWord(11, 41, 42, ".", "PUNCT", nil),
			),
			wantDegraded: true,
			wantRegions:  []string{"del", "dell'"},
			wantWords: []capturedWord{
				{"Il", "SG.M"},
				{"colore", "SG.M"},
				{"cielo", "SG.M"},
				{"e", ""},
				{"acqua", "SG.F"},
				{"mi", "1SG"},
				{"piace", "3SG.PRS.IND.finite"},
			},
		},
		{
			// The typographic apostrophe (U+2019) is kept lexically by the
			// producer and still shares the span with its sibling.
			name: "italian dell’ with typographic apostrophe degrades",
			study: studySentence("Il colore dell’acqua è blu.", 27,
				studyWord(0, 0, 2, "Il", "DET", map[string]string{"Definite": "Def", "Gender": "Masc", "Number": "Sing", "PronType": "Art"}),
				studyWord(1, 3, 9, "colore", "NOUN", map[string]string{"Gender": "Masc", "Number": "Sing"}),
				studyWord(2, 10, 15, "di", "ADP", nil),
				studyWord(3, 10, 15, "l’", "DET", map[string]string{"Definite": "Def", "Number": "Sing", "PronType": "Art"}),
				studyWord(4, 15, 20, "acqua", "NOUN", map[string]string{"Gender": "Fem", "Number": "Sing"}),
				studyWord(5, 21, 22, "è", "AUX", map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "3", "Tense": "Pres", "VerbForm": "Fin"}),
				studyWord(6, 23, 26, "blu", "ADJ", nil),
				studyWord(7, 26, 27, ".", "PUNCT", nil),
			),
			wantDegraded: true,
			wantRegions:  []string{"dell’"},
			wantWords: []capturedWord{
				{"Il", "SG.M"},
				{"colore", "SG.M"},
				{"acqua", "SG.F"},
				{"è", "3SG.PRS.IND.finite"},
				{"blu", ""},
			},
		},
		{
			// Greek differs: the configured `mwt` processor splits στο/στην into
			// distinct sub-spans that each match the source, so both align rather
			// than degrading. The synthetic fixtures model these as shared spans.
			name:         "greek sto and stin split and align",
			study:        capturedGreekStoStudy(),
			wantDegraded: false,
			wantWords: []capturedWord{
				{"Πηγαίνω", "1SG.PRS.IPFV.IND.active.finite"},
				{"σ", ""},
				{"το", "SG.N.ACC"},
				{"σπίτι", "SG.N.ACC"},
				{"και", ""},
				{"σ", ""},
				{"την", "SG.F.ACC"},
				{"πόλη", "SG.F.ACC"},
			},
		},
		{
			// Italian imperfect tense from the configured model: `ero` and
			// `giocavo` both carry Tense=Imp and render the spelled-out label.
			name: "italian imperfect tense sentence",
			study: studySentence("Quando ero bambino, giocavo ogni giorno.", 40,
				studyWord(0, 0, 6, "Quando", "SCONJ", nil),
				studyWord(1, 7, 10, "ero", "AUX", map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "1", "Tense": "Imp", "VerbForm": "Fin"}),
				studyWord(2, 11, 18, "bambino", "NOUN", map[string]string{"Gender": "Masc", "Number": "Sing"}),
				studyWord(3, 18, 19, ",", "PUNCT", nil),
				studyWord(4, 20, 27, "giocavo", "VERB", map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "1", "Tense": "Imp", "VerbForm": "Fin"}),
				studyWord(5, 28, 32, "ogni", "DET", map[string]string{"Number": "Sing", "PronType": "Ind"}),
				studyWord(6, 33, 39, "giorno", "NOUN", map[string]string{"Gender": "Masc", "Number": "Sing"}),
				studyWord(7, 39, 40, ".", "PUNCT", nil),
			),
			wantWords: []capturedWord{
				{"Quando", ""},
				{"ero", "1SG.imperfect.IND.finite"},
				{"bambino", "SG.M"},
				{"giocavo", "1SG.imperfect.IND.finite"},
				{"ogni", "SG"},
				{"giorno", "SG.M"},
			},
		},
		{
			// Italian past participle: the configured model supplies
			// Tense=Past + VerbForm=Part with gender and number.
			name: "italian past participle sentence",
			study: studySentence("Ho mangiato la mela.", 20,
				studyWord(0, 0, 2, "Ho", "AUX", map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "1", "Tense": "Pres", "VerbForm": "Fin"}),
				studyWord(1, 3, 11, "mangiato", "VERB", map[string]string{"Gender": "Masc", "Number": "Sing", "Tense": "Past", "VerbForm": "Part"}),
				studyWord(2, 12, 14, "la", "DET", map[string]string{"Definite": "Def", "Gender": "Fem", "Number": "Sing", "PronType": "Art"}),
				studyWord(3, 15, 19, "mela", "NOUN", map[string]string{"Gender": "Fem", "Number": "Sing"}),
				studyWord(4, 19, 20, ".", "PUNCT", nil),
			),
			wantWords: []capturedWord{
				{"Ho", "1SG.PRS.IND.finite"},
				{"mangiato", "SG.M.PST.PTCP"},
				{"la", "SG.F"},
				{"mela", "SG.F"},
			},
		},
		{
			// German infinitive and a preposition with empty morphology.
			name: "german infinitive and empty morphology sentence",
			study: studySentence("Er will nach Hause gehen.", 25,
				studyWord(0, 0, 2, "Er", "PRON", map[string]string{"Case": "Nom", "Gender": "Masc", "Number": "Sing", "Person": "3", "PronType": "Prs"}),
				studyWord(1, 3, 7, "will", "AUX", map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "3", "Tense": "Pres", "VerbForm": "Fin"}),
				studyWord(2, 8, 12, "nach", "ADP", nil),
				studyWord(3, 13, 18, "Hause", "NOUN", map[string]string{"Case": "Dat", "Gender": "Neut", "Number": "Sing"}),
				studyWord(4, 19, 24, "gehen", "VERB", map[string]string{"VerbForm": "Inf"}),
				studyWord(5, 24, 25, ".", "PUNCT", nil),
			),
			wantWords: []capturedWord{
				{"Er", "3SG.M.NOM"},
				{"will", "3SG.PRS.IND.finite"},
				{"nach", ""},
				{"Hause", "SG.N.DAT"},
				{"gehen", "INF"},
			},
		},
		{
			// Greek perfective aspect plus an infinitive form in one sentence.
			name: "greek perfective infinitive sentence",
			study: studySentence("Έχω διαβάσει το βιβλίο.", 23,
				studyWord(0, 0, 3, "Έχω", "AUX", map[string]string{"Aspect": "Imp", "Mood": "Ind", "Number": "Sing", "Person": "1", "Tense": "Pres", "VerbForm": "Fin", "Voice": "Act"}),
				studyWord(1, 4, 12, "διαβάσει", "VERB", map[string]string{"Aspect": "Perf", "VerbForm": "Inf", "Voice": "Act"}),
				studyWord(2, 13, 15, "το", "DET", map[string]string{"Case": "Acc", "Definite": "Def", "Gender": "Neut", "Number": "Sing", "PronType": "Art"}),
				studyWord(3, 16, 22, "βιβλίο", "NOUN", map[string]string{"Case": "Acc", "Gender": "Neut", "Number": "Sing"}),
				studyWord(4, 22, 23, ".", "PUNCT", nil),
			),
			wantWords: []capturedWord{
				{"Έχω", "1SG.PRS.IPFV.IND.active.finite"},
				{"διαβάσει", "PFV.active.INF"},
				{"το", "SG.N.ACC"},
				{"βιβλίο", "SG.N.ACC"},
			},
		},
		{
			// Uppercase accented Greek is tokenized poorly by the configured
			// model: the final noun's span includes the sentence-final period,
			// which the producer strips from the surface. The stored surface then
			// disagrees with the source slice and the region degrades, while the
			// original `ΑΥΛΉ.` text is preserved. Filed as a follow-up issue.
			name:         "greek uppercase sentence degrades the noun spanning its period",
			study:        capturedGreekUppercaseStudy(),
			wantDegraded: true,
			wantRegions:  []string{"ΑΥΛΉ."},
			wantWords: []capturedWord{
				{"ΤΑ", "PL.N.NOM"},
				{"ΠΑΙΔΙΆ", "PL.N.NOM"},
				{"ΠΑΊΖΟΥΝ", "3PL.PRS.IPFV.IND.active.finite"},
				{"ΣΤΗΝ", ""},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			line := alignSentenceStudy(test.study)
			require.False(t, line.WholeLineFallback, "real sentence bounds always validate against the unit text")
			assert.Equal(t, test.wantDegraded, line.Degraded)
			assert.Equal(t, test.wantRegions, line.DegradedRegions)
			assert.Equal(t, test.wantWords, capturedWords(line))
			assert.Equal(t, test.study.SentenceText, studySource(line), "the original source text is preserved exactly")
		})
	}
}

// TestSentenceStudyCapturedConfiguredModelLabels pins the label presentation of
// the configured models' real morphology: Italian imperfect tense, Greek aspect,
// participles, infinitives and empty morphology.
func TestSentenceStudyCapturedConfiguredModelLabels(t *testing.T) {
	tests := []struct {
		name       string
		morphology map[string]string
		want       string
	}{
		{name: "italian imperfect tense", morphology: map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "1", "Tense": "Imp", "VerbForm": "Fin"}, want: "1SG.imperfect.IND.finite"},
		{name: "greek imperfective aspect", morphology: map[string]string{"Aspect": "Imp", "Mood": "Ind", "Number": "Sing", "Person": "1", "Tense": "Pres", "VerbForm": "Fin", "Voice": "Act"}, want: "1SG.PRS.IPFV.IND.active.finite"},
		{name: "greek perfective aspect", morphology: map[string]string{"Aspect": "Perf", "VerbForm": "Inf", "Voice": "Act"}, want: "PFV.active.INF"},
		{name: "greek finite perfective past", morphology: map[string]string{"Aspect": "Perf", "Mood": "Ind", "Number": "Sing", "Person": "3", "Tense": "Past", "VerbForm": "Fin", "Voice": "Act"}, want: "3SG.PST.PFV.IND.active.finite"},
		{name: "italian past participle", morphology: map[string]string{"Gender": "Masc", "Number": "Sing", "Tense": "Past", "VerbForm": "Part"}, want: "SG.M.PST.PTCP"},
		{name: "german past participle", morphology: map[string]string{"VerbForm": "Part"}, want: "PTCP"},
		{name: "german infinitive", morphology: map[string]string{"VerbForm": "Inf"}, want: "INF"},
		{name: "dative noun", morphology: map[string]string{"Case": "Dat", "Gender": "Masc", "Number": "Sing"}, want: "SG.M.DAT"},
		{name: "pronoun with clitic", morphology: map[string]string{"Clitic": "Yes", "Number": "Sing", "Person": "1", "PronType": "Prs"}, want: "1SG"},
		{name: "no core features", morphology: map[string]string{"Abbr": "Yes"}, want: ""},
		{name: "empty morphology", morphology: map[string]string{}, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, capturedLabelText(test.morphology))
		})
	}

	t.Run("italian imperfect and greek imperfective stay distinct", func(t *testing.T) {
		italian := sentenceStudyCoreLabels(map[string]string{"Tense": "Imp"})
		greek := sentenceStudyCoreLabels(map[string]string{"Aspect": "Imp"})
		require.Len(t, italian, 1)
		require.Len(t, greek, 1)
		assert.Equal(t, "imperfect", studyLabelText(italian[0]))
		assert.Equal(t, "IPFV", studyLabelText(greek[0]))
	})
}

// TestSentenceStudyCapturedGreekSplitContractionRendersAdjacentUnits checks the
// page keeps the split contraction as two adjacent, source-preserving units and
// names the notation key for the labels the real model supplied.
func TestSentenceStudyCapturedGreekSplitContractionRendersAdjacentUnits(t *testing.T) {
	study := capturedGreekStoStudy()
	study.TargetOrdinal = 2
	line := alignSentenceStudy(study)
	require.Equal(t, "Πηγαίνω στο σπίτι και στην πόλη.", studySource(line),
		"the split contraction keeps its original source text with no invented space")
	body := renderSentenceStudy(t, study)

	for _, want := range []string{
		`<span class="sentence-study-word">σ</span>`,
		`<strong class="sentence-study-word">το</strong>`,
		`<span class="sentence-study-labels">SG.N.ACC</span>`,
		`<span class="sentence-study-labels">1SG.PRS.IPFV.IND.active.finite</span>`,
		`<dt>IPFV</dt><dd>imperfective aspect`,
	} {
		require.Contains(t, body, want)
	}
}

// TestSentenceStudyCapturedGreekUppercaseNamesTheDegradedRegion checks the
// configured model's poorly tokenized uppercase sentence still degrades
// honestly: the note names the region and points at the text alternative.
func TestSentenceStudyCapturedGreekUppercaseNamesTheDegradedRegion(t *testing.T) {
	study := capturedGreekUppercaseStudy()
	body := renderSentenceStudy(t, study)
	require.Contains(t, body, `class="sentence-study-note"`)
	require.Contains(t, body, "Some words could not be aligned safely to their source text (ΑΥΛΉ.)")
	require.Contains(t, body, `<span class="sentence-study-source"> ΑΥΛΉ.</span>`)
	require.Contains(t, body, "their analyzer evidence is listed in the complete text alternative")
}
