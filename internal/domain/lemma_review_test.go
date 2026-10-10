package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLemmaReviewDecisionResolutionAndStoredCorrection(t *testing.T) {
	occurrence := LemmaReviewOccurrence{CanonicalLemma: "drache"}
	tests := []struct {
		name       string
		decision   LemmaReviewDecision
		resolution string
		stored     string
	}{
		{name: "a correction stores the new lemma", decision: LemmaReviewDecision{Occurrence: occurrence, CanonicalLemma: "drachenwesen"}, resolution: "correct", stored: "drachenwesen"},
		{name: "a keep of the analyzer lemma stores nothing", decision: LemmaReviewDecision{Occurrence: occurrence, CanonicalLemma: "drache"}, resolution: "keep", stored: ""},
		{name: "an exclusion stores nothing even with a lemma", decision: LemmaReviewDecision{Occurrence: occurrence, CanonicalLemma: "drachenwesen", Excluded: true}, resolution: "exclude", stored: ""},
		{name: "an exclusion without a lemma", decision: LemmaReviewDecision{Occurrence: occurrence, Excluded: true}, resolution: "exclude", stored: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.resolution, tt.decision.Resolution())
			assert.Equal(t, tt.stored, tt.decision.StoredCorrection())
		})
	}
}

func TestLemmaReviewProposalDecisionsPerAction(t *testing.T) {
	occurrences := []LemmaReviewOccurrence{{CanonicalLemma: "drache"}, {CanonicalLemma: "drache", CorrectedLemma: "drachen"}}
	proposal := func(action, lemma string) LemmaReviewProposal {
		return LemmaReviewProposal{Action: action, Lemma: lemma, Occurrences: occurrences, NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}
	}

	t.Run("correct sets the lemma on every selected occurrence", func(t *testing.T) {
		decisions := proposal("correct", "drachenwesen").Decisions()
		assert.Len(t, decisions, 2)
		for _, decision := range decisions {
			assert.Equal(t, "drachenwesen", decision.CanonicalLemma)
			assert.False(t, decision.Excluded)
			assert.Equal(t, "german-post-1996", decision.NormalizationProfile)
			assert.Equal(t, "6", decision.NormalizationVersion)
		}
	})

	t.Run("keep restores each occurrence's analyzer lemma", func(t *testing.T) {
		for _, decision := range proposal("keep", "").Decisions() {
			assert.Equal(t, "drache", decision.CanonicalLemma)
			assert.False(t, decision.Excluded)
		}
	})

	t.Run("exclude excludes each occurrence without a lemma", func(t *testing.T) {
		for _, decision := range proposal("exclude", "").Decisions() {
			assert.True(t, decision.Excluded)
			assert.Empty(t, decision.CanonicalLemma)
		}
	})
}
