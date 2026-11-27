package enrichment

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderSensesReturnsDegenerateInputsUnchanged(t *testing.T) {
	t.Run("single sense is returned unchanged", func(t *testing.T) {
		single := []LexicalSense{{Gloss: "only"}}
		require.Equal(t, single, OrderSenses(LexicalLookupRequest{RepresentativeSentence: "Das Wort steht hier."}, single))
	})
	t.Run("no senses stays nil", func(t *testing.T) {
		require.Nil(t, OrderSenses(LexicalLookupRequest{}, nil))
	})
}

func TestOrderSensesBoostsPhraseAndFallsBackToPrimary(t *testing.T) {
	senses := []LexicalSense{
		{Gloss: "home", Examples: []string{"The house is quiet."}},
		{Gloss: "at home", Phrase: "zu Hause", Examples: []string{"She stayed at home."}},
	}
	request := LexicalLookupRequest{RepresentativeSentence: "Er bleibt zu Hause und liest."}
	ordered := OrderSenses(request, senses)
	assert.Equal(t, "at home", ordered[0].Gloss)
	assert.Equal(t, "home", ordered[1].Gloss)

	ordered = OrderSenses(LexicalLookupRequest{RepresentativeSentence: "Das Wort steht hier."}, senses)
	assert.Equal(t, "home", ordered[0].Gloss)
	assert.Equal(t, "at home", ordered[1].Gloss)
}

func TestOrderSensesMatchesFixedPhrasesByWord(t *testing.T) {
	senses := []LexicalSense{
		{Gloss: "primary"},
		{Gloss: "phrase", Phrase: "zu Hause"},
	}

	ordered := OrderSenses(LexicalLookupRequest{RepresentativeSentence: "Er bleibt zu Hause."}, senses)
	assert.Equal(t, "phrase", ordered[0].Gloss)

	ordered = OrderSenses(LexicalLookupRequest{RepresentativeSentence: "Er bleibt zu Hausen."}, senses)
	assert.Equal(t, "primary", ordered[0].Gloss)
}

func TestOrderSensesCountsDistinctContextWords(t *testing.T) {
	request := LexicalLookupRequest{RepresentativeSentence: "Das Wort ist schnell, kalt und rot.", TargetWord: "Wort"}
	senses := []LexicalSense{
		{Gloss: "schnell schnell schnell"},
		{Gloss: "kalt rot"},
	}

	ordered := OrderSenses(request, senses)
	assert.Equal(t, "kalt rot", ordered[0].Gloss)
}

func TestOrderSensesKeepsPrimaryForLowSignal(t *testing.T) {
	request := LexicalLookupRequest{RepresentativeSentence: "Das Wort ist schnell.", TargetWord: "Wort"}
	senses := []LexicalSense{
		{Gloss: "primary"},
		{Gloss: "schnell"},
	}

	ordered := OrderSenses(request, senses)
	assert.Equal(t, "primary", ordered[0].Gloss)
}

func TestOrderSensesOnlyReorders(t *testing.T) {
	senses := []LexicalSense{
		{Gloss: "primary"},
		{Gloss: "second", Phrase: "second target"},
		{Gloss: "third"},
	}

	ordered := OrderSenses(LexicalLookupRequest{RepresentativeSentence: "The second target.", TargetWord: "target"}, senses)
	assert.Len(t, ordered, len(senses))
	assert.Equal(t, "second", ordered[0].Gloss)
	assert.ElementsMatch(t, []string{"primary", "second", "third"}, []string{ordered[0].Gloss, ordered[1].Gloss, ordered[2].Gloss})
}

func TestOrderSensesKeepsPrimaryWhenTargetIsAbsent(t *testing.T) {
	request := LexicalLookupRequest{RepresentativeSentence: "Der Fluss ist schnell.", TargetWord: "Haus"}
	senses := []LexicalSense{
		{Gloss: "primary"},
		{Gloss: "fast"},
	}

	ordered := OrderSenses(request, senses)
	assert.Equal(t, "primary", ordered[0].Gloss)
}

func TestOrderSensesUsesTokenContextDeterministically(t *testing.T) {
	request := LexicalLookupRequest{
		TargetWord: "Haus",
		SentenceTokens: []analyzer.Token{
			{Surface: "Das", CanonicalLemma: "der", UPOS: "DET"},
			{Surface: "Haus", CanonicalLemma: "haus", UPOS: "NOUN", Dependency: "nsubj"},
			{Surface: "schwimmt", CanonicalLemma: "schwimmen", UPOS: "VERB", Dependency: "root"},
		},
	}
	senses := []LexicalSense{{Gloss: "building"}, {Gloss: "household", Topics: []string{"schwimmen", "nsubj"}}}
	first := OrderSenses(request, senses)
	second := OrderSenses(request, senses)
	assert.Equal(t, first, second)
	assert.Equal(t, "household", first[0].Gloss)
}

func TestOrderSensesIgnoresGreekFunctionWords(t *testing.T) {
	request := LexicalLookupRequest{Language: "el", RepresentativeSentence: "Ο άνθρωπος και εδώ.", TargetWord: "άνθρωπος"}
	senses := []LexicalSense{{Gloss: "primary"}, {Gloss: "secondary", Topics: []string{"ο", "και"}}}

	ordered := OrderSenses(request, senses)
	assert.Equal(t, "primary", ordered[0].Gloss)
}

func TestOrderSensesTokenContextRequiresResolvableTarget(t *testing.T) {
	senses := []LexicalSense{{Gloss: "primary"}, {Gloss: "secondary", Topics: []string{"schwimmen", "nsubj"}}}
	tokens := []analyzer.Token{
		{Surface: "Der", CanonicalLemma: "der", UPOS: "DET"},
		{Surface: "Haus", CanonicalLemma: "haus", UPOS: "NOUN", Dependency: "nsubj"},
		{Surface: "schwimmt", CanonicalLemma: "schwimmen", UPOS: "VERB", Dependency: "root"},
	}
	for _, test := range []struct {
		name, target string
	}{
		{"empty target cannot anchor a token window", ""},
		{"absent target cannot anchor a token window", "Fahrrad"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ordered := OrderSenses(LexicalLookupRequest{TargetWord: test.target, SentenceTokens: tokens}, senses)
			assert.Equal(t, "primary", ordered[0].Gloss)
		})
	}
}

func TestRenderGlossLimitsSensesAndTokens(t *testing.T) {
	got := RenderGloss([]LexicalSense{{Gloss: "one two three four"}, {Gloss: "second meaning"}, {Gloss: "third"}}, 2, 3)
	assert.Equal(t, "one two three · second meaning", got)
}

func TestValidateSenseSelectionChecksCandidateRangeAndUniqueness(t *testing.T) {
	valid := []int{1, 0}
	assert.True(t, ValidateSenseSelection(valid, 2))
	assert.False(t, ValidateSenseSelection([]int{0, 0}, 2))
	assert.False(t, ValidateSenseSelection([]int{-1}, 2))
	assert.False(t, ValidateSenseSelection([]int{2}, 2))
	assert.True(t, ValidateSenseSelection([]int{0, 1, 2, 3}, 4))
}

func TestIsContextWordUsesTheFrenchStopwordSet(t *testing.T) {
	for _, word := range []string{"le", "de", "que", "ne", "pas", "n", "l", "d", "où"} {
		assert.False(t, isContextWord("fr", word), word)
	}
	for _, word := range []string{"maison", "homme", "près", "là"} {
		assert.True(t, isContextWord("fr", word), word)
	}
	// The French set is language-scoped: German and Italian are unchanged.
	assert.True(t, isContextWord("de", "ne"), "German ne is not a French stopword")
	assert.True(t, isContextWord("it", "pas"), "Italian pas is not a French stopword")
}

func TestIsContextWordUsesTheClassicalFunctionWordSets(t *testing.T) {
	for _, word := range []string{"δέ", "καί", "οὐ", "οὐκ", "μή", "ἄν", "ἀλλά", "δὲ", "καὶ", "γὰρ"} {
		assert.False(t, isContextWord("grc", word), word)
	}
	for _, word := range []string{"ὡς", "νῦν", "λόγος", "αὐτός"} {
		assert.True(t, isContextWord("grc", word), word)
	}
	for _, word := range []string{"et", "non", "ne", "uero", "vero", "uel", "vel", "tamen", "Vero"} {
		assert.False(t, isContextWord("la", word), word)
	}
	for _, word := range []string{"nunc", "rex", "ita"} {
		assert.True(t, isContextWord("la", word), word)
	}
	assert.True(t, isContextWord("de", "tamen"), "the Latin set is language-scoped")
	assert.True(t, isContextWord("el", "δέ"), "the Greek set is not the Modern Greek set")
}

func TestCloneLexicalSensesDeepCopiesDictionaryMetadata(t *testing.T) {
	original := []LexicalSense{{Examples: []string{"example"}, Topics: []string{"topic"}, Tags: []string{"tag"}}}
	clone := CloneLexicalSenses(original)
	clone[0].Examples[0] = "changed"
	clone[0].Topics[0] = "changed"
	clone[0].Tags[0] = "changed"

	assert.Equal(t, "example", original[0].Examples[0])
	assert.Equal(t, "topic", original[0].Topics[0])
	assert.Equal(t, "tag", original[0].Tags[0])
}

func TestOrderSensesAddsRelationWordsOnlyForUDRuns(t *testing.T) {
	tokens := []analyzer.Token{
		{Surface: "Haus", CanonicalLemma: "haus", UPOS: "NOUN", Dependency: "SBJ"},
		{Surface: "schwimmt", CanonicalLemma: "schwimmen", UPOS: "VERB", Dependency: "PRED"},
	}
	// The second sense matches only the AGDT relation words the tokens
	// would add, so it wins exactly when the relation is read as context.
	senses := []LexicalSense{{Gloss: "primary"}, {Gloss: "relation-only", Topics: []string{"sbj", "pred"}}}

	for _, test := range []struct {
		name, state string
		wantFirst   string
	}{
		{"a ud run reads the relation", "ud", "relation-only"},
		{"an unspecified state keeps the existing behaviour", "", "relation-only"},
		{"a native AGDT run ignores the relation", "native_agdt", "primary"},
		{"a run without syntax ignores the relation", "absent", "primary"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := LexicalLookupRequest{TargetWord: "Haus", SentenceTokens: tokens, SyntaxState: test.state}
			assert.Equal(t, test.wantFirst, OrderSenses(request, senses)[0].Gloss)
		})
	}
}

func TestOrderSensesGreekOrderingIsUnchangedByAGDTLabels(t *testing.T) {
	senses := []LexicalSense{{Gloss: "first"}, {Gloss: "second", Topics: []string{"κῦρος"}}, {Gloss: "third", Topics: []string{"atr", "pred"}}}
	before := []analyzer.Token{
		{Surface: "λέγει", CanonicalLemma: "λέγω", UPOS: "VERB", Dependency: "dep"},
		{Surface: "Κῦρος", CanonicalLemma: "κῦρος", UPOS: "PROPN", Dependency: "dep"},
	}
	after := []analyzer.Token{
		{Surface: "λέγει", CanonicalLemma: "λέγω", UPOS: "VERB", Dependency: "PRED"},
		{Surface: "Κῦρος", CanonicalLemma: "κῦρος", UPOS: "PROPN", Dependency: "ATR"},
	}
	want := OrderSenses(LexicalLookupRequest{Language: "grc", TargetWord: "λέγει", SentenceTokens: before, SyntaxState: "native_agdt"}, senses)
	got := OrderSenses(LexicalLookupRequest{Language: "grc", TargetWord: "λέγει", SentenceTokens: after, SyntaxState: "native_agdt"}, senses)
	assert.Equal(t, want, got)
	assert.Equal(t, "first", got[0].Gloss, "an AGDT label never lifts a sense")
}

func TestOrderSensesGreekUDRunStillUsesTheRelation(t *testing.T) {
	tokens := []analyzer.Token{
		{Surface: "λέγει", CanonicalLemma: "λέγω", UPOS: "VERB", Dependency: "root"},
		{Surface: "Κῦρος", CanonicalLemma: "κῦρος", UPOS: "PROPN", Dependency: "nsubj"},
	}
	senses := []LexicalSense{{Gloss: "first"}, {Gloss: "relation-only", Topics: []string{"root", "nsubj"}}}

	ud := OrderSenses(LexicalLookupRequest{Language: "grc", TargetWord: "λέγει", SentenceTokens: tokens, SyntaxState: "ud"}, senses)
	native := OrderSenses(LexicalLookupRequest{Language: "grc", TargetWord: "λέγει", SentenceTokens: tokens, SyntaxState: "native_agdt"}, senses)

	assert.Equal(t, "relation-only", ud[0].Gloss, "a Stanza grc run is ud, so its relations are context")
	assert.Equal(t, "first", native[0].Gloss)
}
