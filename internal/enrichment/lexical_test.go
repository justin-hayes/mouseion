package enrichment

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
)

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
