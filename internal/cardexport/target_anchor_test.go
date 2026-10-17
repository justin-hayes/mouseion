package cardexport

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tokenAt builds a persisted token whose document-absolute offsets are the
// given Unicode code-point rune indices.
func tokenAt(surface string, startRune, endRune uint64) analyzer.Token {
	return analyzer.Token{Surface: surface, Location: analyzer.SourceLocation{StartOffset: startRune, EndOffset: endRune}}
}

// anchoredSpan reports the sentence text targetSpan selects, or ok=false.
func anchoredSpan(t *testing.T, sentence, target string, tokens []analyzer.Token, sentenceStartOffset uint64) (string, int, int, bool) {
	t.Helper()
	start, end, ok := targetSpan(sentence, target, tokens, sentenceStartOffset)
	if !ok {
		return "", 0, 0, false
	}
	return sentence[start:end], start, end, true
}

// TestTargetSpanAnchorsElidedAndInversionHyphenTargets pins the ADR 0102
// fallback: a target a word-bounded match cannot reach is anchored on its own
// persisted token span, in every apostrophe form and before an inversion
// hyphen.
func TestTargetSpanAnchorsElidedAndInversionHyphenTargets(t *testing.T) {
	for _, test := range []struct {
		name                string
		sentence, target    string
		tokens              []analyzer.Token
		sentenceStartOffset uint64
		want                string
		wantStart, wantEnd  int
	}{
		{
			name:     "french ascii elision",
			sentence: "Je vois l'homme qui marche vers la gare.",
			target:   "homme",
			tokens: []analyzer.Token{
				tokenAt("Je", 0, 2), tokenAt("vois", 3, 7), tokenAt("l'", 8, 10),
				tokenAt("homme", 10, 15), tokenAt("qui", 16, 19), tokenAt("marche", 20, 26),
				tokenAt("vers", 27, 31), tokenAt("la", 32, 34), tokenAt("gare", 35, 39), tokenAt(".", 39, 40),
			},
			want: "homme", wantStart: 10, wantEnd: 15,
		},
		{
			name:     "french curly elision",
			sentence: "Je vois l’homme qui marche vers la gare.",
			target:   "homme",
			tokens: []analyzer.Token{
				tokenAt("Je", 0, 2), tokenAt("vois", 3, 7), tokenAt("l’", 8, 10),
				tokenAt("homme", 10, 15), tokenAt("qui", 16, 19), tokenAt("marche", 20, 26),
				tokenAt("vers", 27, 31), tokenAt("la", 32, 34), tokenAt("gare", 35, 39), tokenAt(".", 39, 40),
			},
			want: "homme", wantStart: 12, wantEnd: 17,
		},
		{
			name:     "french d'accord",
			sentence: "Ils sont d'accord sur tout cela.",
			target:   "accord",
			tokens: []analyzer.Token{
				tokenAt("Ils", 0, 3), tokenAt("sont", 4, 8), tokenAt("d'", 9, 11),
				tokenAt("accord", 11, 17), tokenAt("sur", 18, 21), tokenAt("tout", 22, 26), tokenAt("cela", 27, 31), tokenAt(".", 31, 32),
			},
			want: "accord", wantStart: 11, wantEnd: 17,
		},
		{
			name:     "french qu'il clitic",
			sentence: "Je pense qu'il viendra ce soir.",
			target:   "il",
			tokens: []analyzer.Token{
				tokenAt("Je", 0, 2), tokenAt("pense", 3, 8), tokenAt("qu'", 9, 12),
				tokenAt("il", 12, 14), tokenAt("viendra", 15, 22), tokenAt("ce", 23, 25), tokenAt("soir", 26, 30), tokenAt(".", 30, 31),
			},
			want: "il", wantStart: 12, wantEnd: 14,
		},
		{
			name:     "italian elided article",
			sentence: "L'uomo cammina per la strada.",
			target:   "uomo",
			tokens: []analyzer.Token{
				tokenAt("L'", 0, 2), tokenAt("uomo", 2, 6), tokenAt("cammina", 7, 14),
				tokenAt("per", 15, 18), tokenAt("la", 19, 21), tokenAt("strada", 22, 28), tokenAt(".", 28, 29),
			},
			want: "uomo", wantStart: 2, wantEnd: 6,
		},
		{
			name:     "inversion hyphen after va",
			sentence: "Va-t-il au marché ce matin ?",
			target:   "va",
			tokens: []analyzer.Token{
				tokenAt("Va", 0, 2), tokenAt("-", 2, 3), tokenAt("t", 3, 4), tokenAt("-", 4, 5),
				tokenAt("il", 5, 7), tokenAt("au", 8, 10), tokenAt("marché", 11, 17),
				tokenAt("ce", 18, 20), tokenAt("matin", 21, 26), tokenAt("?", 27, 28),
			},
			want: "Va", wantStart: 0, wantEnd: 2,
		},
		{
			name:     "inversion hyphen before elle",
			sentence: "Dit-elle la vérité ce matin ?",
			target:   "elle",
			tokens: []analyzer.Token{
				tokenAt("Dit", 0, 3), tokenAt("-", 3, 4), tokenAt("elle", 4, 8),
				tokenAt("la", 9, 11), tokenAt("vérité", 12, 18), tokenAt("ce", 19, 21), tokenAt("matin", 22, 27), tokenAt("?", 28, 29),
			},
			want: "elle", wantStart: 4, wantEnd: 8,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			span, start, end, ok := anchoredSpan(t, test.sentence, test.target, test.tokens, test.sentenceStartOffset)
			require.True(t, ok, "target %q was not anchored", test.target)
			assert.Equal(t, test.want, span)
			assert.Equal(t, test.wantStart, start)
			assert.Equal(t, test.wantEnd, end)
		})
	}
}

// TestTargetSpanKeepsWordBoundedSpanFirst pins that a target the unchanged
// word-bounded matcher already anchors keeps that span, so the German and Greek
// results the ADR must not disturb are unchanged.
func TestTargetSpanKeepsWordBoundedSpanFirst(t *testing.T) {
	sentence := "Die U-Bahn und die Bahn fahren heute."
	tokens := []analyzer.Token{
		tokenAt("Die", 0, 3), tokenAt("U-Bahn", 4, 10), tokenAt("und", 11, 14),
		tokenAt("die", 15, 18), tokenAt("Bahn", 19, 23), tokenAt("fahren", 24, 30), tokenAt("heute", 31, 36), tokenAt(".", 36, 37),
	}
	span, start, end, ok := anchoredSpan(t, sentence, "Bahn", tokens, 0)
	require.True(t, ok)
	assert.Equal(t, "Bahn", span, "must not anchor inside U-Bahn")
	assert.Equal(t, 19, start)
	assert.Equal(t, 23, end)
}

// TestTargetSpanDoesNotAnchorInsideHyphenatedOrApostrophizedWord pins the
// rejected alternative: the token fallback must not see Bahn inside U-Bahn or
// Neill inside O'Neill, because neither is its own persisted token.
func TestTargetSpanDoesNotAnchorInsideHyphenatedOrApostrophizedWord(t *testing.T) {
	for _, test := range []struct {
		name, sentence, target string
		tokens                 []analyzer.Token
	}{
		{
			name: "german hyphenated compound", sentence: "Die U-Bahn fährt heute.",
			target: "Bahn", tokens: []analyzer.Token{tokenAt("Die", 0, 3), tokenAt("U-Bahn", 4, 10), tokenAt("fährt", 11, 16), tokenAt("heute", 17, 22), tokenAt(".", 22, 23)},
		},
		{
			name: "german apostrophized name", sentence: "O'Neill wartet heute dort.",
			target: "Neill", tokens: []analyzer.Token{tokenAt("O'Neill", 0, 7), tokenAt("wartet", 8, 14), tokenAt("heute", 15, 20), tokenAt("dort", 21, 25), tokenAt(".", 25, 26)},
		},
		{
			name: "greek hyphenated word", sentence: "Ο υπουργός-εξωτερικών μίλησε σήμερα.",
			target: "εξωτερικών", tokens: []analyzer.Token{tokenAt("Ο", 0, 1), tokenAt("υπουργός-εξωτερικών", 2, 21), tokenAt("μίλησε", 22, 28), tokenAt("σήμερα", 29, 35), tokenAt(".", 35, 36)},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, ok := anchoredSpan(t, test.sentence, test.target, test.tokens, 0)
			assert.False(t, ok, "%q must stay unmatched", test.target)
		})
	}
}

// TestTargetSpanRejectsUnverifiableTokenSpans pins the acceptance check: a
// multiword-token part carrying a whole contraction's span, a stale offset and
// an out-of-sentence offset are each rejected rather than mis-anchored.
func TestTargetSpanRejectsUnverifiableTokenSpans(t *testing.T) {
	for _, test := range []struct {
		name, sentence, target string
		tokens                 []analyzer.Token
	}{
		{
			name: "contraction part carries whole span", sentence: "Il boit du café le matin.",
			target: "de", tokens: []analyzer.Token{tokenAt("Il", 0, 2), tokenAt("boit", 3, 7), tokenAt("de", 8, 10), tokenAt("café", 11, 15), tokenAt("le", 16, 18), tokenAt("matin", 19, 24), tokenAt(".", 24, 25)},
		},
		{
			name: "stale offset points at other text", sentence: "Je vois l'homme qui marche vers la gare.",
			target: "homme", tokens: []analyzer.Token{tokenAt("homme", 0, 5)},
		},
		{
			name: "offset past end of sentence", sentence: "Je vois l'homme.",
			target: "homme", tokens: []analyzer.Token{tokenAt("homme", 40, 45)},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, ok := anchoredSpan(t, test.sentence, test.target, test.tokens, 0)
			assert.False(t, ok)
		})
	}
}

// TestTargetSpanWithoutTokensKeepsWordBoundedResult pins that a sentence with
// no persisted tokens keeps today's result rather than inventing an anchor.
func TestTargetSpanWithoutTokensKeepsWordBoundedResult(t *testing.T) {
	_, _, ok := targetSpan("Je vois l'homme qui marche.", "homme", nil, 0)
	assert.False(t, ok)
	_, _, ok = targetSpan("Je vois l'homme qui marche.", "", []analyzer.Token{tokenAt("homme", 10, 15)}, 0)
	assert.False(t, ok, "an empty target never anchors")
	start, end, ok := targetSpan("Das Haus steht dort.", "Haus", nil, 0)
	require.True(t, ok)
	assert.Equal(t, "Haus", "Das Haus steht dort."[start:end])
}

// TestTokenSpanBytesRejectsOutOfRangeOffsets pins the offset guards, including
// the code-point-to-byte conversion for a multi-byte sentence.
func TestTokenSpanBytesRejectsOutOfRangeOffsets(t *testing.T) {
	for _, test := range []struct {
		name                   string
		sentence               string
		start, end, sentenceAt uint64
	}{
		{name: "start before sentence start", sentence: "homme", start: 5, end: 10, sentenceAt: 6},
		{name: "end before start", sentence: "homme", start: 4, end: 2, sentenceAt: 0},
		{name: "start past end of sentence", sentence: "homme", start: 6, end: 10, sentenceAt: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, ok := tokenSpanBytes(test.sentence, test.start, test.end, test.sentenceAt)
			assert.False(t, ok)
		})
	}

	start, end, ok := tokenSpanBytes("à homme", 2, 7, 0)
	require.True(t, ok)
	assert.Equal(t, "homme", "à homme"[start:end], "code points convert to byte offsets")
	assert.Equal(t, 3, start)
	assert.Equal(t, 8, end)
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

// TestTargetSpanHonorsSentenceStartOffset pins that code-point offsets are made
// relative to the persisted sentence start rather than used as raw sentence
// indices.
func TestTargetSpanHonorsSentenceStartOffset(t *testing.T) {
	// The sentence begins at document code point 1000; its first token starts
	// there and homme begins 10 code points later.
	sentence := "Je vois l'homme qui marche."
	tokens := []analyzer.Token{
		tokenAt("Je", 1000, 1002), tokenAt("vois", 1003, 1007), tokenAt("l'", 1008, 1010),
		tokenAt("homme", 1010, 1015), tokenAt("qui", 1016, 1019), tokenAt("marche", 1020, 1026), tokenAt(".", 1026, 1027),
	}
	span, start, end, ok := anchoredSpan(t, sentence, "homme", tokens, 1000)
	require.True(t, ok)
	assert.Equal(t, "homme", span)
	assert.Equal(t, 10, start)
	assert.Equal(t, 15, end)
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

// TestSentenceQualityAcceptsElidedTarget pins the sentence-quality "target
// present" check, which otherwise rejects an elided sentence before rendering.
func TestSentenceQualityAcceptsElidedTarget(t *testing.T) {
	sentence := "Je vois l'homme qui marche vers la gare."
	tokens := []analyzer.Token{
		tokenAt("Je", 0, 2), tokenAt("vois", 3, 7), tokenAt("l'", 8, 10), tokenAt("homme", 10, 15),
		tokenAt("qui", 16, 19), tokenAt("marche", 20, 26), tokenAt("vers", 27, 31), tokenAt("la", 32, 34), tokenAt("gare", 35, 39), tokenAt(".", 39, 40),
	}

	quality := scoreSentenceQuality("fr", sentence, "homme", 0, tokens, 0)
	assert.True(t, quality.Accepted, "quality=%+v", quality)
	assert.Contains(t, quality.Reasons, "target present")

	assert.Equal(t, "homme", matchingSentenceTarget(sentence, []string{"homme"}, tokens, 0))
	assert.Equal(t, -1, targetIndex(sentence, "homme", nil, 0))
	assert.NotEqual(t, -1, targetIndex(sentence, "homme", tokens, 0))
}

// TestBestSentenceEvidenceCarriesTokenAnchor pins the corpus seam: the evidence
// carries the persisted sentence start offset and tokens so the render can
// anchor a target the word-bounded matcher cannot reach.
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

	note, err := makeNote("owner", renderInputFromEntry(Entry{
		Language: "fr", CanonicalLemma: "homme", UPOS: "NOUN",
		Sentence: evidence.Sentence, TargetWord: evidence.Target,
		SentenceTokens: evidence.Tokens, SentenceStartOffset: evidence.SentenceStartOffset,
		FirstEncounter: evidence.FirstEncounter,
	}))
	require.NoError(t, err)
	assert.Equal(t, "Je vois l&#39;<b>homme</b> qui marche vers la gare.", note.Text)
}
