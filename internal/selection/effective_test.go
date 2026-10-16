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
		assert.Equal(t, Identity{"de", "Haus", "NOUN", ""}, id)
		assert.True(t, counts)
	})

	t.Run("a correction replaces the analyzer lemma", func(t *testing.T) {
		id, counts := Effective(cfg, "de", fact, &OccurrenceDecision{Lemma: "haus"})
		assert.Equal(t, Identity{"de", "haus", "NOUN", ""}, id)
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
		assert.Equal(t, Identity{"de", "haus", "NOUN", ""}, id)
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
		assert.Equal(t, Identity{"de", "drachenwesen", "NOUN", ""}, ReviewIdentityNow("de", corrected))
		assert.Equal(t, Identity{"de", "drache", "NOUN", ""}, ReviewIdentityNow("de", occurrence))
	})

	t.Run("an excluded occurrence has no current identity", func(t *testing.T) {
		excluded := occurrence
		excluded.Excluded = true
		assert.Equal(t, Identity{}, ReviewIdentityNow("de", excluded))
	})

	t.Run("a proposed exclusion, keep or correction gives the after identity", func(t *testing.T) {
		assert.Equal(t, Identity{}, ReviewIdentityAfter("de", occurrence, domain.LemmaReviewDecision{Occurrence: occurrence, Excluded: true}))
		assert.Equal(t, Identity{"de", "drache", "NOUN", ""}, ReviewIdentityAfter("de", occurrence, domain.LemmaReviewDecision{Occurrence: occurrence, CanonicalLemma: "drache"}))
		assert.Equal(t, Identity{"de", "drachenwesen", "NOUN", ""}, ReviewIdentityAfter("de", occurrence, domain.LemmaReviewDecision{Occurrence: occurrence, CanonicalLemma: "drachenwesen"}))
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
	require.Empty(t, ProposalIdentities(domain.LemmaReviewProposal{Language: "de", Action: "keep"}))
}

// Two identities that differ only in the discriminator are unequal, and an
// empty discriminator equals an empty discriminator.
func TestIdentityComparisonIncludesTheLexicalDiscriminator(t *testing.T) {
	bare := Identity{Language: "grc", CanonicalLemma: "lexis", UPOS: "NOUN"}
	discriminated := Identity{Language: "grc", CanonicalLemma: "lexis", UPOS: "NOUN", LexicalDiscriminator: "morpheus:2"}

	assert.NotEqual(t, bare, discriminated)
	assert.Equal(t, Identity{Language: "grc", CanonicalLemma: "lexis", UPOS: "NOUN"}, bare)
}

// Effective carries the analyzer's discriminator, and a learner correction
// produces an identity with an empty discriminator.
func TestEffectiveCarriesTheLexicalDiscriminator(t *testing.T) {
	cfg := DefaultConfig("corpus")
	discriminated := OccurrenceFact{Lemma: "lexis", UPOS: "NOUN", LexicalDiscriminator: "morpheus:2"}

	id, counts := Effective(cfg, "grc", discriminated, nil)
	require.True(t, counts)
	assert.Equal(t, Identity{"grc", "lexis", "NOUN", "morpheus:2"}, id)

	bare, counts := Effective(cfg, "grc", OccurrenceFact{Lemma: "lexis", UPOS: "NOUN"}, nil)
	require.True(t, counts)
	assert.Equal(t, Identity{"grc", "lexis", "NOUN", ""}, bare)

	keep, counts := Effective(cfg, "grc", discriminated, &OccurrenceDecision{Lemma: "lexis"})
	require.True(t, counts)
	assert.Equal(t, Identity{"grc", "lexis", "NOUN", "morpheus:2"}, keep, "a keep preserves the analyzer identity")

	corrected, counts := Effective(cfg, "grc", discriminated, &OccurrenceDecision{Lemma: "leksis"})
	require.True(t, counts)
	assert.Equal(t, Identity{"grc", "leksis", "NOUN", ""}, corrected, "a correction has an empty discriminator")
}

// Learner state keeps discriminated identities distinct, and a blank Known UPOS
// remains the lemma wildcard within the same discriminator.
func TestEligibilityKeepsDiscriminatedIdentitiesDistinct(t *testing.T) {
	discriminated := domain.SelectionCandidate{Language: "grc", CanonicalLemma: "lexis", UPOS: "NOUN", LexicalDiscriminator: "morpheus:2", OccurrenceCount: 3}
	bare := domain.SelectionCandidate{Language: "grc", CanonicalLemma: "lexis", UPOS: "NOUN", OccurrenceCount: 3}

	reserved := NewEligibility(nil, []domain.DeckPreparationVocabulary{{Language: "grc", CanonicalLemma: "lexis", UPOS: "NOUN", LexicalDiscriminator: "morpheus:2"}})
	assert.False(t, reserved.Allows(discriminated, 1), "the matching discriminator is Reserved")
	assert.True(t, reserved.Allows(bare, 1), "a different discriminator is not Reserved")

	known := NewEligibility([]domain.KnownVocabulary{{Language: "grc", CanonicalLemma: "lexis", UPOS: "NOUN"}}, nil)
	assert.False(t, known.Allows(bare, 1), "empty equals empty")
	assert.True(t, known.Allows(discriminated, 1), "an empty Known discriminator does not exclude a discriminated identity")
}

// Identities that differ only in the discriminator sort bare before
// discriminated, in every deterministic ordering the identity feeds.
func TestDiscriminatedIdentitiesSortAfterBareOnes(t *testing.T) {
	bare := Identity{Language: "grc", CanonicalLemma: "lexis", UPOS: "NOUN"}
	discriminated := Identity{Language: "grc", CanonicalLemma: "lexis", UPOS: "NOUN", LexicalDiscriminator: "morpheus:2"}

	ids := sortedIdentities(map[Identity]*aggregate{discriminated: {}, bare: {}})
	assert.Equal(t, []Identity{bare, discriminated}, ids)

	occurrences := []domain.LemmaReviewOccurrence{
		{CanonicalLemma: "lexis", UPOS: "VERB"},
		{CanonicalLemma: "lexis", UPOS: "NOUN", LexicalDiscriminator: "morpheus:2"},
		{CanonicalLemma: "lexis", UPOS: "NOUN"},
	}
	proposal := domain.LemmaReviewProposal{Language: "grc", Action: "keep", Occurrences: occurrences}
	assert.Equal(t, []domain.LemmaReviewIdentity{
		{Language: "grc", CanonicalLemma: "lexis", UPOS: "NOUN"},
		{Language: "grc", CanonicalLemma: "lexis", UPOS: "NOUN", LexicalDiscriminator: "morpheus:2"},
		{Language: "grc", CanonicalLemma: "lexis", UPOS: "VERB"},
	}, ProposalIdentities(proposal))

	states := []domain.LemmaReviewIdentityState{
		{Identity: domain.LemmaReviewIdentity{Language: "grc", CanonicalLemma: "lexis", UPOS: "NOUN", LexicalDiscriminator: "morpheus:2"}},
		{Identity: domain.LemmaReviewIdentity{Language: "grc", CanonicalLemma: "lexis", UPOS: "NOUN"}},
	}
	impacts := ReviewImpact(states)
	assert.Equal(t, []Identity{bare, discriminated}, []Identity{impacts[0].Identity, impacts[1].Identity})
}
