package selection

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewImpactUsesReadingEligibility(t *testing.T) {
	haus := Identity{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"}
	eligibility := NewEligibility(nil, nil)

	t.Run("a correction crosses the three-occurrence threshold", func(t *testing.T) {
		impacts := eligibility.ReviewImpact([]ImpactCounts{{Identity: haus, Before: 2, After: 3}})
		require.Len(t, impacts, 1)
		assert.False(t, impacts[0].BeforeEligible)
		assert.True(t, impacts[0].AfterEligible)
	})

	t.Run("two occurrences qualify only with ten across analyzed Books", func(t *testing.T) {
		short := eligibility.ReviewImpact([]ImpactCounts{{Identity: haus, Before: 1, After: 2, OtherBooks: 7}})
		assert.False(t, short[0].AfterEligible, "two here plus seven elsewhere is nine")
		route := eligibility.ReviewImpact([]ImpactCounts{{Identity: haus, Before: 1, After: 2, OtherBooks: 8}})
		assert.False(t, route[0].BeforeEligible)
		assert.True(t, route[0].AfterEligible, "two here plus eight elsewhere is ten")
	})

	t.Run("an exclusion drops an identity below threshold", func(t *testing.T) {
		impacts := eligibility.ReviewImpact([]ImpactCounts{{Identity: haus, Before: 3, After: 2, OtherBooks: 0}})
		assert.True(t, impacts[0].BeforeEligible)
		assert.False(t, impacts[0].AfterEligible)
	})

	t.Run("losing an occurrence also loses the across-Book route", func(t *testing.T) {
		impacts := eligibility.ReviewImpact([]ImpactCounts{{Identity: haus, Before: 2, After: 1, OtherBooks: 8}})
		assert.True(t, impacts[0].BeforeEligible)
		assert.False(t, impacts[0].AfterEligible)
	})

	t.Run("Known and Reserved state keep an identity ineligible", func(t *testing.T) {
		known := NewEligibility([]domain.KnownVocabulary{{Language: "de", CanonicalLemma: "haus"}}, nil)
		impacts := known.ReviewImpact([]ImpactCounts{{Identity: haus, Before: 2, After: 3}})
		assert.True(t, impacts[0].Known)
		assert.False(t, impacts[0].AfterEligible)
	})

	t.Run("impacts are ordered by lemma then UPOS", func(t *testing.T) {
		impacts := eligibility.ReviewImpact([]ImpactCounts{
			{Identity: Identity{Language: "de", CanonicalLemma: "zug", UPOS: "NOUN"}},
			{Identity: Identity{Language: "de", CanonicalLemma: "haus", UPOS: "VERB"}},
			{Identity: haus},
		})
		require.Len(t, impacts, 3)
		assert.Equal(t, "NOUN", impacts[0].Identity.UPOS)
		assert.Equal(t, "VERB", impacts[1].Identity.UPOS)
		assert.Equal(t, "zug", impacts[2].Identity.CanonicalLemma)
	})
}
