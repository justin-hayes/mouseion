package testutil

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/stretchr/testify/require"
)

// TestGatePassingSeedsPassTheSharedGate pins the seeds the store contracts and
// the corpus helper write. If a gate change ever rejects one of them, this test
// fails here instead of silently leaving integration fixtures with empty
// snapshots.
func TestGatePassingSeedsPassTheSharedGate(t *testing.T) {
	cases := []struct {
		name, language string
		word           CorpusWord
	}{
		{name: "german noun", language: "de", word: CorpusWord{Lemma: "haus", UPOS: "NOUN"}},
		{name: "german verb", language: "de", word: CorpusWord{Lemma: "gehen", UPOS: "VERB"}},
		{name: "german adjective", language: "de", word: CorpusWord{Lemma: "schnell", UPOS: "ADJ"}},
		{name: "german adverb", language: "de", word: CorpusWord{Lemma: "gern", UPOS: "ADV"}},
		{name: "modern greek noun", language: "el", word: CorpusWord{Lemma: "σπίτι", UPOS: "NOUN"}},
		{name: "ancient greek noun", language: "grc", word: CorpusWord{Lemma: "λόγος", UPOS: "NOUN"}},
		{name: "ancient greek verb", language: "grc", word: CorpusWord{Lemma: "δέω", UPOS: "VERB"}},
		{name: "other language noun", language: "fr", word: CorpusWord{Lemma: "maison", UPOS: "NOUN"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate, sentences := gateCandidate(t, GatePassingSentence(tc.language, tc.word), tc.language, tc.word.Lemma)
			require.True(t, sentencequality.AnyReferencePasses(candidate, sentences))
		})
	}
}

// TestFragmentSeedsFailTheSharedGate pins the explicit unusable seed. A
// fragment is a one-token sentence the gate must reject, whatever the language.
func TestFragmentSeedsFailTheSharedGate(t *testing.T) {
	for _, language := range []string{"de", "el", "grc"} {
		t.Run(language, func(t *testing.T) {
			word := CorpusWord{Lemma: "haus", UPOS: "NOUN"}
			candidate, sentences := gateCandidate(t, fragmentSentence(word), language, word.Lemma)
			require.False(t, sentencequality.AnyReferencePasses(candidate, sentences))
			_, resolves := sentencequality.BestSentenceEvidenceFromCorpus(candidate, sentences)
			require.True(t, resolves, "a fragment is evidence that resolves and is rejected, not missing evidence")
		})
	}
}

// gateCandidate builds the freeze's two scoring inputs for one seeded sentence:
// a candidate whose single reference addresses sentence zero, and the persisted
// sentences keyed the way the deck keys them.
func gateCandidate(t *testing.T, sentence SeededSentence, language, lemma string) (domain.SelectionCandidate, map[int64]analyzer.Sentence) {
	t.Helper()
	references, err := json.Marshal([]selection.SentenceReference{{
		SentenceIndex: 0, Text: sentence.Text,
		Location: analyzer.SourceLocation{StartOffset: 0, EndOffset: mustUint64(utf8.RuneCountInString(sentence.Text))},
	}})
	require.NoError(t, err)
	forms, err := json.Marshal([]string{lemma})
	require.NoError(t, err)
	candidate := domain.SelectionCandidate{
		Language: language, CanonicalLemma: lemma,
		ObservedForms: forms, SentenceReferences: references,
	}
	sentences := map[int64]analyzer.Sentence{
		0: {Text: sentence.Text, Tokens: sentence.Tokens, Location: analyzer.SourceLocation{StartOffset: 0}},
	}
	return candidate, sentences
}
