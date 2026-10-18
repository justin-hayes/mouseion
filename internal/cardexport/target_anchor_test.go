package cardexport

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tokenAt builds a persisted token whose document-absolute offsets are the
// given Unicode code-point rune indices.
func tokenAt(surface string, startRune, endRune uint64) analyzer.Token {
	return analyzer.Token{Surface: surface, Location: analyzer.SourceLocation{StartOffset: startRune, EndOffset: endRune}}
}

// TestApplySentenceDecisionResetsAnchorWithoutPersistedSentence pins that an
// unresolvable persisted reference drops any previously set anchor.
func TestApplySentenceDecisionResetsAnchorWithoutPersistedSentence(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CanonicalLemma: "homme", Language: "fr",
		ObservedForms:      []byte(`["homme"]`),
		SentenceReferences: []byte(`[{"sentence_index":9,"text":"Je vois l'homme qui marche.","location":{"start_offset":0}}]`),
	}
	entry := Entry{
		Language: "fr", CanonicalLemma: "homme", UPOS: "NOUN",
		Sentence: "Je vois l'homme qui marche.", TargetWord: "homme", SentenceStartOffset: 12,
	}

	applySentenceDecision(&entry, candidate, map[int64]analyzer.Sentence{})

	assert.Empty(t, entry.SentenceTokens)
	assert.Equal(t, uint64(0), entry.SentenceStartOffset)
}

// TestMakeNoteAnchorsElidedTargetsThroughTheRenderSeam pins the rendered front:
// an elided or inversion-hyphen target is bolded at its own span and the source
// apostrophe is preserved byte-for-byte.
func TestMakeNoteAnchorsElidedTargetsThroughTheRenderSeam(t *testing.T) {
	for _, test := range []struct {
		name                   string
		sentence, target, want string
		tokens                 []analyzer.Token
	}{
		{
			name: "french ascii elision", sentence: "Je vois l'homme qui marche vers la gare.", target: "homme",
			want: "Je vois l&#39;<b>homme</b> qui marche vers la gare.",
			tokens: []analyzer.Token{
				tokenAt("Je", 0, 2), tokenAt("vois", 3, 7), tokenAt("l'", 8, 10), tokenAt("homme", 10, 15),
				tokenAt("qui", 16, 19), tokenAt("marche", 20, 26), tokenAt("vers", 27, 31), tokenAt("la", 32, 34), tokenAt("gare", 35, 39), tokenAt(".", 39, 40),
			},
		},
		{
			name: "french curly elision keeps the source apostrophe", sentence: "Je vois l’homme qui marche vers la gare.", target: "homme",
			want: "Je vois l’<b>homme</b> qui marche vers la gare.",
			tokens: []analyzer.Token{
				tokenAt("Je", 0, 2), tokenAt("vois", 3, 7), tokenAt("l’", 8, 10), tokenAt("homme", 10, 15),
				tokenAt("qui", 16, 19), tokenAt("marche", 20, 26), tokenAt("vers", 27, 31), tokenAt("la", 32, 34), tokenAt("gare", 35, 39), tokenAt(".", 39, 40),
			},
		},
		{
			name: "italian elided article", sentence: "L'uomo cammina per la strada.", target: "uomo",
			want: "L&#39;<b>uomo</b> cammina per la strada.",
			tokens: []analyzer.Token{
				tokenAt("L'", 0, 2), tokenAt("uomo", 2, 6), tokenAt("cammina", 7, 14), tokenAt("per", 15, 18),
				tokenAt("la", 19, 21), tokenAt("strada", 22, 28), tokenAt(".", 28, 29),
			},
		},
		{
			name: "inversion hyphen", sentence: "Va-t-il au marché ce matin ?", target: "va",
			want: "<b>Va</b>-t-il au marché ce matin ?",
			tokens: []analyzer.Token{
				tokenAt("Va", 0, 2), tokenAt("-", 2, 3), tokenAt("t", 3, 4), tokenAt("-", 4, 5), tokenAt("il", 5, 7),
				tokenAt("au", 8, 10), tokenAt("marché", 11, 17), tokenAt("ce", 18, 20), tokenAt("matin", 21, 26), tokenAt("?", 27, 28),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := Entry{
				Language: "fr", CanonicalLemma: test.target, UPOS: "NOUN",
				Sentence: test.sentence, TargetWord: test.target, SentenceTokens: test.tokens,
			}
			note, err := makeNote("owner", renderInputFromEntry(entry))
			require.NoError(t, err)
			// The expected front reproduces the source sentence byte-for-byte,
			// so the equality also proves the curly apostrophe was not
			// normalized and no source text was rewritten.
			assert.Equal(t, test.want, note.Text)
		})
	}
}

// TestBestSentenceEvidenceCarriesTokenAnchor pins the corpus seam: the shared
// module's evidence carries the persisted sentence start offset and tokens so
// the render can anchor a target the word-bounded matcher cannot reach.
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

	const source = "Je vois l'homme qui marche vers la gare."
	evidence, ok := sentencequality.BestSentenceEvidenceFromCorpus(candidate, sentences)
	require.True(t, ok)
	assert.Equal(t, "homme", evidence.Target)
	assert.Equal(t, uint64(0), evidence.SentenceStartOffset)
	assert.Len(t, evidence.Tokens, 10)
	assert.Equal(t, source, evidence.Sentence)
	assert.Equal(t, source, sentences[3].Text, "the persisted source sentence is byte-identical after export")

	note, err := makeNote("owner", renderInputFromEntry(Entry{
		Language: "fr", CanonicalLemma: "homme", UPOS: "NOUN",
		Sentence: evidence.Sentence, TargetWord: evidence.Target,
		SentenceTokens: evidence.Tokens, SentenceStartOffset: evidence.SentenceStartOffset,
		FirstEncounter: evidence.FirstEncounter,
	}))
	require.NoError(t, err)
	assert.Equal(t, "Je vois l&#39;<b>homme</b> qui marche vers la gare.", note.Text)
}
