package selection

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffectiveAppliesTheLearnerDecisionBeforeEligibility(t *testing.T) {
	cfg := DefaultConfig("corpus")
	fact := OccurrenceFact{Lemma: "Haus", UPOS: "NOUN"}

	t.Run("no decision keeps the analyzer lemma", func(t *testing.T) {
		id, counts := Effective(cfg, "de", fact, nil)
		assert.Equal(t, Identity{"de", "Haus", "NOUN"}, id)
		assert.True(t, counts)
	})

	t.Run("a correction replaces the analyzer lemma", func(t *testing.T) {
		id, counts := Effective(cfg, "de", fact, &OccurrenceDecision{Lemma: "haus"})
		assert.Equal(t, Identity{"de", "haus", "NOUN"}, id)
		assert.True(t, counts)
	})

	t.Run("an exclusion has no identity and counts as nothing", func(t *testing.T) {
		id, counts := Effective(cfg, "de", fact, &OccurrenceDecision{Excluded: true})
		assert.Equal(t, Identity{}, id)
		assert.False(t, counts)
	})

	t.Run("a separable particle keeps its identity but does not count", func(t *testing.T) {
		id, counts := Effective(cfg, "de", OccurrenceFact{Lemma: "auf", UPOS: "ADP", Dependency: "compound:prt"}, nil)
		assert.Equal(t, "auf", id.CanonicalLemma)
		assert.False(t, counts)
	})

	t.Run("a non-letter lemma does not count, even as a correction", func(t *testing.T) {
		_, counts := Effective(cfg, "de", OccurrenceFact{Lemma: "1234", UPOS: "NOUN"}, nil)
		assert.False(t, counts)
		_, counts = Effective(cfg, "de", fact, &OccurrenceDecision{Lemma: " 42 "})
		assert.False(t, counts)
	})

	t.Run("padded UPOS and lemma normalize to the same identity", func(t *testing.T) {
		id, counts := Effective(cfg, "de", OccurrenceFact{Lemma: " haus ", UPOS: " noun "}, nil)
		assert.Equal(t, Identity{"de", "haus", "NOUN"}, id)
		assert.True(t, counts)
	})

	t.Run("a disallowed part of speech does not count", func(t *testing.T) {
		_, counts := Effective(cfg, "de", OccurrenceFact{Lemma: "und", UPOS: "CCONJ"}, nil)
		assert.False(t, counts)
	})
}

func TestReviewIdentityFollowsTheCurrentAndProposedDecision(t *testing.T) {
	occurrence := domain.LemmaReviewOccurrence{CanonicalLemma: "drache", UPOS: "NOUN"}

	t.Run("the current identity is the correction when there is one", func(t *testing.T) {
		corrected := occurrence
		corrected.CorrectedLemma = "drachenwesen"
		assert.Equal(t, Identity{"de", "drachenwesen", "NOUN"}, ReviewIdentityNow("de", corrected))
		assert.Equal(t, Identity{"de", "drache", "NOUN"}, ReviewIdentityNow("de", occurrence))
	})

	t.Run("an excluded occurrence has no current identity", func(t *testing.T) {
		excluded := occurrence
		excluded.Excluded = true
		assert.Equal(t, Identity{}, ReviewIdentityNow("de", excluded))
	})

	t.Run("a proposed exclusion, keep or correction gives the after identity", func(t *testing.T) {
		assert.Equal(t, Identity{}, ReviewIdentityAfter("de", occurrence, domain.LemmaReviewDecision{Occurrence: occurrence, Excluded: true}))
		assert.Equal(t, Identity{"de", "drache", "NOUN"}, ReviewIdentityAfter("de", occurrence, domain.LemmaReviewDecision{Occurrence: occurrence, CanonicalLemma: "drache"}))
		assert.Equal(t, Identity{"de", "drachenwesen", "NOUN"}, ReviewIdentityAfter("de", occurrence, domain.LemmaReviewDecision{Occurrence: occurrence, CanonicalLemma: "drachenwesen"}))
	})
}

func TestProposalIdentitiesCoverBeforeAndAfterInOrder(t *testing.T) {
	occurrence := domain.LemmaReviewOccurrence{CanonicalLemma: "heim", UPOS: "NOUN", Dependency: "nk"}
	proposal := domain.LemmaReviewProposal{Language: "de", Action: "correct", Lemma: "haus", Occurrences: []domain.LemmaReviewOccurrence{occurrence}}
	assert.Equal(t, []domain.LemmaReviewIdentity{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"},
		{Language: "de", CanonicalLemma: "heim", UPOS: "NOUN"},
	}, ProposalIdentities(proposal))

	exclude := domain.LemmaReviewProposal{Language: "de", Action: "exclude", Occurrences: []domain.LemmaReviewOccurrence{occurrence}}
	assert.Equal(t, []domain.LemmaReviewIdentity{{Language: "de", CanonicalLemma: "heim", UPOS: "NOUN"}}, ProposalIdentities(exclude))
	require.Len(t, ProposalIdentities(domain.LemmaReviewProposal{Language: "de", Action: "keep"}), 0)
}
