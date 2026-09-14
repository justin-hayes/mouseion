package dictionary

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
)

func TestOrderSensesBoostsPhraseAndFallsBackToPrimary(t *testing.T) {
	senses := []Sense{
		{Gloss: "home", Examples: []string{"The house is quiet."}},
		{Gloss: "at home", Phrase: "zu Hause", Examples: []string{"She stayed at home."}},
	}
	request := LookupRequest{ExampleSentence: "Er bleibt zu Hause und liest."}
	ordered := OrderSenses(request, senses)
	assert.Equal(t, "at home", ordered[0].Gloss)
	assert.Equal(t, "home", ordered[1].Gloss)

	ordered = OrderSenses(LookupRequest{ExampleSentence: "Das Wort steht hier."}, senses)
	assert.Equal(t, "home", ordered[0].Gloss)
	assert.Equal(t, "at home", ordered[1].Gloss)
}

func TestOrderSensesUsesTokenContextDeterministically(t *testing.T) {
	request := LookupRequest{
		TargetWord: "Haus",
		SentenceTokens: []analyzer.Token{
			{Surface: "Das", CanonicalLemma: "der", UPOS: "DET"},
			{Surface: "Haus", CanonicalLemma: "haus", UPOS: "NOUN", Dependency: "nsubj"},
			{Surface: "schwimmt", CanonicalLemma: "schwimmen", UPOS: "VERB", Dependency: "root"},
		},
	}
	senses := []Sense{{Gloss: "building"}, {Gloss: "household", Topics: []string{"schwimmen"}}}
	first := OrderSenses(request, senses)
	second := OrderSenses(request, senses)
	assert.Equal(t, first, second)
	assert.Equal(t, "household", first[0].Gloss)
}

func TestRenderGlossLimitsSensesAndTokens(t *testing.T) {
	got := RenderGloss([]Sense{{Gloss: "one two three four"}, {Gloss: "second meaning"}, {Gloss: "third"}}, 2, 3)
	assert.Equal(t, "one two three · second meaning", got)
}
