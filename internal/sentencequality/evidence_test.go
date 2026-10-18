package sentencequality

import (
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBestSentenceEvidencePreservesCompleteSourceText(t *testing.T) {
	const source = "  Vor dem alten Haus spielen heute mehrere fröhliche Kinder.  "
	candidate := domain.SelectionCandidate{
		CorpusID:           "corpus-1",
		CanonicalLemma:     "haus",
		ObservedForms:      []byte(`["Haus"]`),
		SentenceReferences: []byte(`[{"text":"  Vor dem alten Haus spielen heute mehrere fröhliche Kinder.  ","location":{"start_offset":10}}]`),
	}
	evidence, ok := BestSentenceEvidence(candidate)
	assert.True(t, ok)
	assert.Equal(t, source, evidence.Sentence)
}

func TestBestSentenceEvidenceRanksAllReferences(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CorpusID:       "corpus-1",
		CanonicalLemma: "haus",
		ObservedForms:  []byte(`["Haus"]`),
		SentenceReferences: []byte(`[
			{"sentence_index":1,"text":"Inhaltsverzeichnis: Das Haus und seine Geschichte ..... 12.","location":{"start_offset":10}},
			{"sentence_index":2,"text":"Vor dem alten Haus spielen heute mehrere fröhliche Kinder im Garten.","location":{"start_offset":80}}
		]`),
	}
	got, ok := BestSentenceEvidence(candidate)
	assert.True(t, ok)
	assert.True(t, got.Quality.Accepted)
	assert.Equal(t, "corpus-1", got.CorpusID)
	assert.Equal(t, int64(80), got.FirstEncounter)
	assert.Equal(t, int64(2), got.SentenceOrdinal)
	assert.Equal(t, "Haus", got.Target)
	assert.True(t, strings.HasPrefix(got.Sentence, "Vor dem"))
}

func TestBestSentenceEvidenceUsesWordTargetsAndStableSourceOrder(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CanonicalLemma: "Haus",
		ObservedForms:  []byte(`["Haus"]`),
		SentenceReferences: []byte(`[
			{"sentence_index":9,"text":"Dieses Haus steht seit vielen Jahren ruhig am See.","location":{"start_offset":90}},
			{"sentence_index":2,"text":"Unser Haus steht seit vielen Jahren ruhig am See.","location":{"start_offset":20}},
			{"sentence_index":1,"text":"Das Hausboot liegt seit vielen Jahren ruhig am See.","location":{"start_offset":10}}
		]`),
	}
	first, ok := BestSentenceEvidence(candidate)
	second, okAgain := BestSentenceEvidence(candidate)
	assert.True(t, ok)
	assert.True(t, okAgain)
	assert.Equal(t, "Unser Haus steht seit vielen Jahren ruhig am See.", first.Sentence)
	assert.Equal(t, second.Sentence, first.Sentence)
	assert.Equal(t, second.Target, first.Target)
	assert.Equal(t, second.FirstEncounter, first.FirstEncounter)
	assert.Equal(t, second.Quality.Accepted, first.Quality.Accepted)
	assert.Equal(t, second.Quality.Score, first.Quality.Score)
	assert.Equal(t, strings.Join(second.Quality.Reasons, "\x00"), strings.Join(first.Quality.Reasons, "\x00"))
}

func TestBestSentenceEvidenceRanksAcceptedReferencesByGradualGDEXScore(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CorpusID:       "corpus-1",
		Language:       "de",
		CanonicalLemma: "see",
		ObservedForms:  []byte(`["See"]`),
		SentenceReferences: []byte(`[
			{"sentence_index":0,"text":"marginal","location":{"start_offset":10}},
			{"sentence_index":1,"text":"good","location":{"start_offset":100}}
		]`),
	}
	sentences := map[int64]analyzer.Sentence{
		0: {
			Text: "Anna steht heute am alten See.",
			Tokens: []analyzer.Token{
				{Surface: "Anna", UPOS: "PROPN", Dependency: "nsubj", Head: 1},
				{Surface: "steht", UPOS: "VERB", Dependency: "root", Head: 1, Morphology: map[string]string{"VerbForm": "Fin"}},
				{Surface: "heute", UPOS: "ADV", Dependency: "advmod", Head: 1},
				{Surface: "am", UPOS: "ADP", Dependency: "case", Head: 5},
				{Surface: "alten", UPOS: "ADJ", Dependency: "amod", Head: 5},
				{Surface: "See", UPOS: "NOUN", Dependency: "obl", Head: 1},
			},
		},
		1: {
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
		},
	}

	got, ok := BestSentenceEvidenceFromCorpus(candidate, sentences)
	require.True(t, ok)
	assert.Equal(t, "corpus-1", got.CorpusID)
	assert.Equal(t, sentences[1].Text, got.Sentence)
	assert.Equal(t, int64(100), got.FirstEncounter)
	assert.Equal(t, int64(1), got.SentenceOrdinal)
	assert.Greater(t, got.Quality.GDEXScore, float64(0))
}

func TestBestSentenceEvidenceUsesGenericGreekQualityPath(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CorpusID: "greek-corpus", Language: "el", CanonicalLemma: "σπίτι", ObservedForms: []byte(`["σπίτι"]`),
		SentenceReferences: []byte(`[{"sentence_index":0,"text":"Στο μεγάλο σπίτι ο Νίκος διαβάζει κάθε βράδυ.","location":{"start_offset":10}}]`),
	}
	sentences := map[int64]analyzer.Sentence{0: {
		Text: "Στο μεγάλο σπίτι ο Νίκος διαβάζει κάθε βράδυ.",
		Tokens: []analyzer.Token{
			{Surface: "Στο", UPOS: "ADP", Dependency: "case", Head: 2},
			{Surface: "μεγάλο", UPOS: "ADJ", Dependency: "amod", Head: 2},
			{Surface: "σπίτι", UPOS: "NOUN", Dependency: "nsubj", Head: 5},
			{Surface: "ο", UPOS: "DET", Dependency: "det", Head: 4},
			{Surface: "Νίκος", UPOS: "PROPN", Dependency: "nsubj", Head: 5},
			{Surface: "διαβάζει", UPOS: "VERB", Dependency: "root", Head: 5, Morphology: map[string]string{"VerbForm": "Fin"}},
			{Surface: "κάθε", UPOS: "DET", Dependency: "det", Head: 7},
			{Surface: "βράδυ", UPOS: "NOUN", Dependency: "obl", Head: 5},
		},
	}}
	got, ok := BestSentenceEvidenceFromCorpus(candidate, sentences)
	require.True(t, ok)
	assert.True(t, got.Quality.Accepted)
	assert.Zero(t, got.Quality.GDEXScore)
	assert.NotContains(t, got.Quality.Reasons, "deictic context")
	assert.NotContains(t, got.Quality.Reasons, "named-entity density")
}

func TestBestSentenceEvidenceUsesSourceOrderWhenGradualScoresTie(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CanonicalLemma: "haus",
		ObservedForms:  []byte(`["Haus"]`),
		SentenceReferences: []byte(`[
			{"sentence_index":1,"text":"Das Haus steht heute noch am alten See.","location":{"start_offset":20}},
			{"sentence_index":0,"text":"Das Haus steht heute am alten See.","location":{"start_offset":10}}
		]`),
	}

	got, ok := BestSentenceEvidence(candidate)
	require.True(t, ok)
	assert.Equal(t, "Das Haus steht heute am alten See.", got.Sentence)
	assert.Equal(t, int64(10), got.FirstEncounter)
}

// TestBestSentenceEvidenceRejectsMalformedCandidates pins the decode failures
// that make a candidate unrankable through the exported seam.
func TestBestSentenceEvidenceRejectsMalformedCandidates(t *testing.T) {
	for _, test := range []struct {
		name      string
		candidate domain.SelectionCandidate
	}{
		{
			name:      "malformed sentence references",
			candidate: domain.SelectionCandidate{ObservedForms: []byte(`["Haus"]`), SentenceReferences: []byte(`{`)},
		},
		{
			name:      "malformed observed forms",
			candidate: domain.SelectionCandidate{ObservedForms: []byte(`{`), SentenceReferences: []byte(`[{"text":"Das Haus steht dort.","location":{"start_offset":1}}]`)},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			evidence, ok := BestSentenceEvidence(test.candidate)
			assert.False(t, ok)
			assert.Equal(t, SentenceEvidence{}, evidence)
		})
	}
}

// TestBestSentenceEvidenceFromCorpusSkipsUnresolvedReferences pins that a
// source reference absent from the persisted corpus is skipped, and that
// skipping every reference reports no evidence.
func TestBestSentenceEvidenceFromCorpusSkipsUnresolvedReferences(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CorpusID:       "corpus-1",
		CanonicalLemma: "haus",
		ObservedForms:  []byte(`["Haus"]`),
		SentenceReferences: []byte(`[
			{"sentence_index":7,"text":"stale","location":{"start_offset":70}},
			{"sentence_index":2,"text":"stale","location":{"start_offset":20}}
		]`),
	}
	sentences := map[int64]analyzer.Sentence{2: {Text: "Das alte Haus steht heute ganz ruhig dort."}}

	evidence, ok := BestSentenceEvidenceFromCorpus(candidate, sentences)
	require.True(t, ok)
	assert.Equal(t, int64(2), evidence.SentenceOrdinal)
	assert.Equal(t, "Das alte Haus steht heute ganz ruhig dort.", evidence.Sentence)

	none, ok := BestSentenceEvidenceFromCorpus(candidate, map[int64]analyzer.Sentence{})
	assert.False(t, ok)
	assert.Equal(t, SentenceEvidence{}, none)
}

// TestBestSentenceEvidenceDefaultsMissingLocation pins that an unreadable
// source location is captured as the invalid offset the quality gate rejects.
func TestBestSentenceEvidenceDefaultsMissingLocation(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CanonicalLemma:     "haus",
		ObservedForms:      []byte(`["Haus"]`),
		SentenceReferences: []byte(`[{"sentence_index":0,"text":"Das Haus steht dort.","location":{}}]`),
	}

	evidence, ok := BestSentenceEvidence(candidate)
	require.True(t, ok)
	assert.Equal(t, int64(-1), evidence.FirstEncounter)
	assert.Contains(t, evidence.Quality.Reasons, "invalid source location")
}

// TestBestSentenceEvidenceBreaksTiesBySentenceOrdinal pins the ordinal
// tie-break that wins before sentence text.
func TestBestSentenceEvidenceBreaksTiesBySentenceOrdinal(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CanonicalLemma: "haus",
		ObservedForms:  []byte(`["Haus"]`),
		SentenceReferences: []byte(`[
			{"sentence_index":2,"text":"Das Haus steht heute am alten See.","location":{"start_offset":20}},
			{"sentence_index":1,"text":"Zum Haus gehen heute viele Kinder.","location":{"start_offset":20}}
		]`),
	}

	evidence, ok := BestSentenceEvidence(candidate)
	require.True(t, ok)
	assert.Equal(t, int64(1), evidence.SentenceOrdinal)
	assert.Equal(t, "Zum Haus gehen heute viele Kinder.", evidence.Sentence)
}

// TestBestSentenceEvidenceBreaksTiesBySentenceText pins the final tie-break
// when two references share an ordinal and a first encounter.
func TestBestSentenceEvidenceBreaksTiesBySentenceText(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CanonicalLemma: "haus",
		ObservedForms:  []byte(`["Haus"]`),
		SentenceReferences: []byte(`[
			{"sentence_index":1,"text":"Zum Haus gehen heute viele Kinder.","location":{"start_offset":20}},
			{"sentence_index":1,"text":"Das Haus steht heute am alten See.","location":{"start_offset":20}}
		]`),
	}

	evidence, ok := BestSentenceEvidence(candidate)
	require.True(t, ok)
	assert.Equal(t, "Das Haus steht heute am alten See.", evidence.Sentence)
}

// TestBestSentenceEvidenceCarriesTokenAnchor pins the corpus seam: the evidence
// carries the persisted sentence start offset and tokens so a render can anchor
// a target the word-bounded matcher cannot reach.
func TestBestSentenceEvidenceCarriesTokenAnchor(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CorpusID: "corpus-1", CanonicalLemma: "homme", Language: "fr",
		ObservedForms:      []byte(`["homme"]`),
		SentenceReferences: []byte(`[{"sentence_index":3,"text":"stale","location":{"start_offset":0}}]`),
	}
	sentences := map[int64]analyzer.Sentence{3: {
		Text: "Je vois l'homme qui marche vers la gare.",
		Tokens: []analyzer.Token{
			tokenAt("Je", 0, 2), tokenAt("vois", 3, 7), tokenAt("l'", 8, 10), tokenAt("homme", 10, 15),
			tokenAt("qui", 16, 19), tokenAt("marche", 20, 26), tokenAt("vers", 27, 31), tokenAt("la", 32, 34), tokenAt("gare", 35, 39), tokenAt(".", 39, 40),
		},
		Location: analyzer.SourceLocation{StartOffset: 0, EndOffset: 40},
	}}

	evidence, ok := BestSentenceEvidenceFromCorpus(candidate, sentences)
	require.True(t, ok)
	assert.Equal(t, "homme", evidence.Target)
	assert.Equal(t, uint64(0), evidence.SentenceStartOffset)
	assert.Len(t, evidence.Tokens, 10)
	assert.Equal(t, "Je vois l'homme qui marche vers la gare.", evidence.Sentence)
	assert.Equal(t, "Je vois l'homme qui marche vers la gare.", sentences[3].Text, "the persisted source sentence is byte-identical after export")
}

// passableCandidate is a candidate whose sentence references are given as JSON.
func passableCandidate(refs string) domain.SelectionCandidate {
	return domain.SelectionCandidate{
		Language: "de", CanonicalLemma: "haus", ObservedForms: []byte(`["Haus"]`),
		SentenceReferences: []byte(refs),
	}
}

func goodSentence(text string) analyzer.Sentence {
	return analyzer.Sentence{Text: text, Tokens: []analyzer.Token{
		{Surface: "Das", UPOS: "DET", Dependency: "det", Head: 1},
		{Surface: "Haus", UPOS: "NOUN", Dependency: "nsubj", Head: 2},
		{Surface: "steht", UPOS: "VERB", Dependency: "root", Head: 2, Morphology: map[string]string{"VerbForm": "Fin"}},
		{Surface: "heute", UPOS: "ADV", Dependency: "advmod", Head: 2},
		{Surface: "am", UPOS: "ADP", Dependency: "case", Head: 6},
		{Surface: "alten", UPOS: "ADJ", Dependency: "amod", Head: 6},
		{Surface: "See", UPOS: "NOUN", Dependency: "obl", Head: 2},
	}}
}

func TestAnyReferencePasses(t *testing.T) {
	fragment := "Inhaltsverzeichnis: Das Haus und seine Geschichte ..... 12."
	good := "Das Haus steht heute am alten See."

	t.Run("none pass", func(t *testing.T) {
		candidate := passableCandidate(`[
			{"sentence_index":0,"text":"x","location":{"start_offset":10}},
			{"sentence_index":1,"text":"y","location":{"start_offset":20}}
		]`)
		sentences := map[int64]analyzer.Sentence{
			0: {Text: fragment, Tokens: []analyzer.Token{{Surface: "Haus", UPOS: "NOUN", Dependency: "root", Head: 0}}},
			1: {Text: fragment, Tokens: []analyzer.Token{{Surface: "Haus", UPOS: "NOUN", Dependency: "root", Head: 0}}},
		}
		assert.False(t, AnyReferencePasses(candidate, sentences))
	})

	t.Run("only a later reference passes", func(t *testing.T) {
		candidate := passableCandidate(`[
			{"sentence_index":0,"text":"x","location":{"start_offset":10}},
			{"sentence_index":1,"text":"y","location":{"start_offset":20}}
		]`)
		sentences := map[int64]analyzer.Sentence{
			0: {Text: fragment, Tokens: []analyzer.Token{{Surface: "Haus", UPOS: "NOUN", Dependency: "root", Head: 0}}},
			1: goodSentence(good),
		}
		assert.True(t, AnyReferencePasses(candidate, sentences))
	})

	t.Run("unresolved references are ignored", func(t *testing.T) {
		candidate := passableCandidate(`[
			{"sentence_index":7,"text":"stale","location":{"start_offset":70}},
			{"sentence_index":2,"text":"y","location":{"start_offset":20}}
		]`)
		sentences := map[int64]analyzer.Sentence{2: goodSentence(good)}
		assert.True(t, AnyReferencePasses(candidate, sentences))

		noneResolve, ok := BestSentenceEvidenceFromCorpus(passableCandidate(`[
			{"sentence_index":7,"text":"stale","location":{"start_offset":70}}
		]`), sentences)
		assert.False(t, ok)
		assert.False(t, AnyReferencePasses(passableCandidate(`[
			{"sentence_index":7,"text":"stale","location":{"start_offset":70}}
		]`), sentences))
		assert.Equal(t, SentenceEvidence{}, noneResolve)
	})

	t.Run("malformed candidate never passes", func(t *testing.T) {
		assert.False(t, AnyReferencePasses(domain.SelectionCandidate{SentenceReferences: []byte(`{`)}, nil))
	})
}

// TestAnyReferencePassesAgreesWithBestSentenceEvidence pins that the freeze's
// operation and the deck's operation can never disagree: a candidate has a
// passing reference exactly when its best evidence is accepted.
func TestAnyReferencePassesAgreesWithBestSentenceEvidence(t *testing.T) {
	fragment := "Inhaltsverzeichnis: Das Haus und seine Geschichte ..... 12."
	good := "Das Haus steht heute am alten See."

	cases := []struct {
		name      string
		candidate domain.SelectionCandidate
		sentences map[int64]analyzer.Sentence
	}{
		{
			name: "none pass",
			candidate: passableCandidate(`[
				{"sentence_index":0,"text":"x","location":{"start_offset":10}},
				{"sentence_index":1,"text":"y","location":{"start_offset":20}}
			]`),
			sentences: map[int64]analyzer.Sentence{
				0: {Text: fragment, Tokens: []analyzer.Token{{Surface: "Haus", UPOS: "NOUN", Dependency: "root", Head: 0}}},
				1: {Text: fragment, Tokens: []analyzer.Token{{Surface: "Haus", UPOS: "NOUN", Dependency: "root", Head: 0}}},
			},
		},
		{
			name: "a later reference passes",
			candidate: passableCandidate(`[
				{"sentence_index":0,"text":"x","location":{"start_offset":10}},
				{"sentence_index":1,"text":"y","location":{"start_offset":20}}
			]`),
			sentences: map[int64]analyzer.Sentence{
				0: {Text: fragment, Tokens: []analyzer.Token{{Surface: "Haus", UPOS: "NOUN", Dependency: "root", Head: 0}}},
				1: goodSentence(good),
			},
		},
		{
			name: "no reference resolves",
			candidate: passableCandidate(`[
				{"sentence_index":7,"text":"stale","location":{"start_offset":70}}
			]`),
			sentences: map[int64]analyzer.Sentence{2: goodSentence(good)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			best, ok := BestSentenceEvidenceFromCorpus(tc.candidate, tc.sentences)
			want := ok && best.Quality.Accepted
			assert.Equal(t, want, AnyReferencePasses(tc.candidate, tc.sentences))
		})
	}
}
