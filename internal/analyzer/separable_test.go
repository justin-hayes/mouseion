package analyzer_test

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
)

func TestIsSeparableParticle(t *testing.T) {
	tests := map[string]struct {
		tokens []analyzer.Token
		index  int
		want   bool
	}{
		"compound particle": {
			tokens: []analyzer.Token{
				{Surface: "steht", CanonicalLemma: "aufstehen", UPOS: "VERB", Dependency: "root", Head: 0},
				{Surface: "auf", CanonicalLemma: "auf", UPOS: "ADV", Dependency: "compound:prt", Head: 0},
			},
			index: 1,
			want:  true,
		},
		"recovered advmod particle": {
			tokens: []analyzer.Token{
				{Surface: "sieht", CanonicalLemma: "fernsehen", UPOS: "VERB", Dependency: "root", Head: 0},
				{Surface: "fern", CanonicalLemma: "fern", UPOS: "ADV", Dependency: "advmod", Head: 0},
			},
			index: 1,
			want:  true,
		},
		"true adverb keeps the base lemma": {
			tokens: []analyzer.Token{
				{Surface: "liegt", CanonicalLemma: "liegen", UPOS: "VERB", Dependency: "root", Head: 0},
				{Surface: "fern", CanonicalLemma: "fern", UPOS: "ADV", Dependency: "advmod", Head: 0},
			},
			index: 1,
			want:  false,
		},
		"adverb is not a prefix of the verb lemma": {
			tokens: []analyzer.Token{
				{Surface: "steht", CanonicalLemma: "aufstehen", UPOS: "VERB", Dependency: "root", Head: 0},
				{Surface: "gestern", CanonicalLemma: "gestern", UPOS: "ADV", Dependency: "advmod", Head: 0},
			},
			index: 1,
			want:  false,
		},
		"advmod of a non-verb is not a particle": {
			tokens: []analyzer.Token{
				{Surface: "ist", CanonicalLemma: "sein", UPOS: "AUX", Dependency: "root", Head: 0},
				{Surface: "fern", CanonicalLemma: "fern", UPOS: "ADV", Dependency: "advmod", Head: 0},
			},
			index: 1,
			want:  false,
		},
		"self-headed advmod is not a particle": {
			tokens: []analyzer.Token{
				{Surface: "fern", CanonicalLemma: "fernsehen", UPOS: "ADV", Dependency: "advmod", Head: 0},
			},
			index: 0,
			want:  false,
		},
		"non-advmod dependent is not a particle": {
			tokens: []analyzer.Token{
				{Surface: "sieht", CanonicalLemma: "fernsehen", UPOS: "VERB", Dependency: "root", Head: 0},
				{Surface: "fern", CanonicalLemma: "fern", UPOS: "ADV", Dependency: "nsubj", Head: 0},
			},
			index: 1,
			want:  false,
		},
		"non-adverb advmod is not a particle": {
			tokens: []analyzer.Token{
				{Surface: "sieht", CanonicalLemma: "fernsehen", UPOS: "VERB", Dependency: "root", Head: 0},
				{Surface: "fern", CanonicalLemma: "fern", UPOS: "ADJ", Dependency: "advmod", Head: 0},
			},
			index: 1,
			want:  false,
		},
		"index outside the sentence is not a particle": {
			tokens: []analyzer.Token{},
			index:  0,
			want:   false,
		},
		"pre-verbal advmod is not a particle": {
			tokens: []analyzer.Token{
				{Surface: "fern", CanonicalLemma: "fern", UPOS: "ADV", Dependency: "advmod", Head: 1},
				{Surface: "sieht", CanonicalLemma: "fernsehen", UPOS: "VERB", Dependency: "root", Head: 1},
			},
			index: 0,
			want:  false,
		},
		"advmod with a dependent is not a particle": {
			tokens: []analyzer.Token{
				{Surface: "sieht", CanonicalLemma: "fernsehen", UPOS: "VERB", Dependency: "root", Head: 0},
				{Surface: "fern", CanonicalLemma: "fern", UPOS: "ADV", Dependency: "advmod", Head: 0},
				{Surface: "Stadt", CanonicalLemma: "stadt", UPOS: "NOUN", Dependency: "nmod", Head: 1},
			},
			index: 1,
			want:  false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, analyzer.IsSeparableParticle(test.tokens, test.index))
		})
	}
}
