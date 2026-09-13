package gdex

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScoreSentenceQualityKnockout(t *testing.T) {
	tests := []struct {
		name   string
		change func([]analyzer.Token)
	}{
		{name: "root is not finite", change: func(tokens []analyzer.Token) {
			tokens[1].Morphology = map[string]string{"VerbForm": "Inf"}
		}},
		{name: "root is not a verb", change: func(tokens []analyzer.Token) {
			tokens[1].UPOS = "NOUN"
		}},
		{name: "subject is absent", change: func(tokens []analyzer.Token) {
			tokens[2].Dependency = "obj"
		}},
		{name: "subject relation is not nominal subject", change: func(tokens []analyzer.Token) {
			tokens[2].Dependency = "obl"
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sentence := fixtureSentence(12)
			test.change(sentence.Tokens)
			quality := ScoreSentenceQuality(sentence, []int{2})
			assert.False(t, quality.Accepted)
			assert.Zero(t, quality.Score)
			assert.Contains(t, quality.Reasons, reasonNoFiniteVerbAndSubject)
		})
	}
}

func TestScoreSentenceQualityGradualCriteria(t *testing.T) {
	base := fixtureSentence(12)

	t.Run("main clause target beats subordinate target", func(t *testing.T) {
		sentence := fixtureSentence(12)
		sentence.Tokens[5] = analyzer.Token{Surface: "kommt", UPOS: "VERB", Dependency: "advcl", Head: 1, Morphology: map[string]string{"VerbForm": "Fin"}}
		sentence.Tokens[6] = analyzer.Token{Surface: "Haus", UPOS: "NOUN", Dependency: "obj", Head: 5}
		main := ScoreSentenceQuality(sentence, []int{2})
		subordinate := ScoreSentenceQuality(sentence, []int{6})
		assert.True(t, main.Accepted)
		assert.True(t, subordinate.Accepted)
		assert.Greater(t, main.Score, subordinate.Score)
		assert.Contains(t, subordinate.Reasons, reasonSubordinateTarget)
	})

	t.Run("deictic context scores below neutral context", func(t *testing.T) {
		deictic := fixtureSentence(12)
		deictic.Tokens[0] = analyzer.Token{Surface: "Heute", UPOS: "ADV", Dependency: "advmod", Head: 1}
		neutral := fixtureSentence(12)
		gotDeictic := ScoreSentenceQuality(deictic, []int{2})
		gotNeutral := ScoreSentenceQuality(neutral, []int{2})
		assert.Greater(t, gotNeutral.Score, gotDeictic.Score)
		assert.Contains(t, gotDeictic.Reasons, reasonDeicticContext)
	})

	t.Run("entity dense context scores below entity light context", func(t *testing.T) {
		entityDense := fixtureSentence(12)
		for _, index := range []int{0, 2, 3} {
			entityDense.Tokens[index].UPOS = "PROPN"
		}
		entityLight := fixtureSentence(12)
		gotDense := ScoreSentenceQuality(entityDense, []int{2})
		gotLight := ScoreSentenceQuality(entityLight, []int{2})
		assert.Greater(t, gotLight.Score, gotDense.Score)
		assert.Contains(t, gotDense.Reasons, reasonNamedEntityDensity)
	})

	t.Run("optimal length beats short and long contexts", func(t *testing.T) {
		optimal := ScoreSentenceQuality(base, []int{2})
		short := ScoreSentenceQuality(fixtureSentence(4), []int{2})
		long := ScoreSentenceQuality(fixtureSentence(41), []int{2})
		assert.Greater(t, optimal.Score, short.Score)
		assert.Greater(t, optimal.Score, long.Score)
		assert.Contains(t, optimal.Reasons, reasonOptimalLength)
		assert.Contains(t, short.Reasons, reasonOutsideOptimalLength)
	})
}

func TestScoreSentenceQualityAcceptsAllSubjectRelations(t *testing.T) {
	for _, relation := range []string{"nsubj", "nsubj:pass", "csubj"} {
		t.Run(relation, func(t *testing.T) {
			sentence := fixtureSentence(12)
			sentence.Tokens[2].Dependency = relation
			quality := ScoreSentenceQuality(sentence, []int{2})
			assert.True(t, quality.Accepted)
		})
	}
}

func TestScoreSentenceQualityGermanDeixis(t *testing.T) {
	sentence := fixtureSentence(12)
	sentence.Tokens[0] = analyzer.Token{Surface: "ich", UPOS: "PRON", Dependency: "nsubj", Head: 1, Morphology: map[string]string{"PronType": "Prs"}}
	sentence.Tokens[3] = analyzer.Token{Surface: "dort", UPOS: "ADV", Dependency: "advmod", Head: 1}
	pronoun := ScoreSentenceQuality(sentence, []int{2})
	assert.Contains(t, pronoun.Reasons, reasonDeicticContext)

	sentence.Tokens[3].UPOS = "SCONJ"
	conjunction := ScoreSentenceQuality(sentence, []int{2})
	assert.Greater(t, conjunction.Score, pronoun.Score)
}

func TestScoreSentenceQualityIsDeterministic(t *testing.T) {
	sentence := fixtureSentence(12)
	want := ScoreSentenceQuality(sentence, []int{2})
	for range 10 {
		assert.Equal(t, want, ScoreSentenceQuality(sentence, []int{2}))
	}
}

func fixtureSentence(length int) analyzer.Sentence {
	tokens := make([]analyzer.Token, length)
	for index := range tokens {
		tokens[index] = analyzer.Token{Surface: "Wort", UPOS: "NOUN", Dependency: "obl", Head: 1}
	}
	tokens[0] = analyzer.Token{Surface: "Die", UPOS: "DET", Dependency: "det", Head: 2}
	tokens[1] = analyzer.Token{Surface: "steht", UPOS: "VERB", Dependency: "root", Head: 1, Morphology: map[string]string{"VerbForm": "Fin"}}
	tokens[2] = analyzer.Token{Surface: "Sache", UPOS: "NOUN", Dependency: "nsubj", Head: 1}
	return analyzer.Sentence{Text: "Eine beispielhafte Satzstruktur.", Tokens: tokens}
}

func TestFiniteRootRequiresDirectSubject(t *testing.T) {
	sentence := fixtureSentence(12)
	sentence.Tokens[2].Head = 5
	quality := ScoreSentenceQuality(sentence, []int{2})
	require.False(t, quality.Accepted)
}
