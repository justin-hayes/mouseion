package storecontract

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reviewSurface is the observed form every lemma review scenario reviews. Its
// two occurrences share the analyzer lemma "weg".
const reviewSurface = "Weg"

var reviewOccurrences = []OccurrenceSeed{
	{Surface: reviewSurface, Lemma: "weg", UPOS: "NOUN"},
	{Surface: reviewSurface, Lemma: "weg", UPOS: "NOUN"},
}

// seedReviewBook seeds a To Read Book whose analysis contains reviewOccurrences.
func seedReviewBook(t *testing.T, h Harness) string {
	t.Helper()
	return h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{Vocabulary: defaultVocabulary, Occurrences: reviewOccurrences, ToRead: true})
}

// reviewOccurrence returns the occurrence at index among the Book's review
// occurrences of reviewSurface, in sentence order.
func reviewOccurrence(t *testing.T, h Harness, bookID string, index int) domain.LemmaReviewOccurrence {
	t.Helper()
	occurrences, err := h.LemmaReview().ListLemmaReviewOccurrences(h.Seeds().Owner(), bookID, reviewSurface)
	require.NoError(t, err)
	require.Len(t, occurrences, len(reviewOccurrences))
	return occurrences[index]
}

// reviewProposal builds a proposal on reviewSurface for the learner under test.
func reviewProposal(h Harness, bookID, action, lemma string, occurrences ...domain.LemmaReviewOccurrence) domain.LemmaReviewProposal {
	return domain.LemmaReviewProposal{
		OwnerID: h.Seeds().Owner(), BookID: bookID, Language: Language, Surface: reviewSurface,
		Action: action, Lemma: lemma, Occurrences: occurrences,
		NormalizationProfile: "german-post-1996", NormalizationVersion: "6",
	}
}

// previewProposal previews a proposal and returns it with the fingerprint its
// confirm must present.
func previewProposal(t *testing.T, h Harness, proposal domain.LemmaReviewProposal) string {
	t.Helper()
	preview, err := h.LemmaReview().ReadLemmaReviewProposal(proposal)
	require.NoError(t, err)
	return preview.Fingerprint
}

// seedReviewFlag records a detector flag on occurrence.
func seedReviewFlag(t *testing.T, h Harness, occurrence domain.LemmaReviewOccurrence, reason string) {
	t.Helper()
	require.NoError(t, h.LemmaReview().SaveLemmaReviewFlags([]domain.LemmaReviewFlag{{
		Occurrence: occurrence, Reason: reason, Provenance: map[string]any{"detector": "contract"},
	}}))
}

// providerReviewProvenance is the advisory evidence a contextual-Gloss provider
// records: the shared source marker plus its attribution.
func providerReviewProvenance(reason string) map[string]any {
	return map[string]any{
		"source": domain.LemmaReviewFlagProviderSource, "provider": "fixture-provider",
		"model": "fixture-model", "prompt_version": "v16", "unresolved_reason": reason,
	}
}

// seedProviderReviewFlag records a contextual-Gloss provider's advisory flag.
func seedProviderReviewFlag(t *testing.T, h Harness, occurrence domain.LemmaReviewOccurrence, reason string) {
	t.Helper()
	require.NoError(t, h.LemmaReview().SaveLemmaReviewFlags([]domain.LemmaReviewFlag{{
		Occurrence: occurrence, Reason: reason, Provenance: providerReviewProvenance(reason),
	}}))
}

// requireLocalHighRiskFlag asserts the Book's first review occurrence holds
// unresolved local evidence, and that the evidence still gates Start.
func requireLocalHighRiskFlag(t *testing.T, h Harness, bookID string) {
	t.Helper()
	after := reviewOccurrence(t, h, bookID, 0)
	assert.Equal(t, "local high-risk evidence", after.ReviewFlagReason)
	assert.False(t, domain.LemmaReviewFlagProviderSourced(after.ReviewFlagProvenance))
	_, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
	require.ErrorIs(t, err, domain.ErrUnresolvedLemmaReviewFlags)
}

func lemmaReviewScenarios() []Scenario {
	scenarios := lemmaDecisionScenarios()
	scenarios = append(scenarios, lemmaFlagScenarios()...)
	return append(scenarios, lemmaProviderFlagScenarios()...)
}

func lemmaDecisionScenarios() []Scenario {
	return []Scenario{{
		Name: "a confirmed correction stores the lemma for the selected occurrence only",
		Run: func(t *testing.T, h Harness) {
			bookID := seedReviewBook(t, h)
			first := reviewOccurrence(t, h, bookID, 0)
			proposal := reviewProposal(h, bookID, "correct", "pfad", first)
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(proposal, previewProposal(t, h, proposal)))
			assert.Equal(t, "pfad", reviewOccurrence(t, h, bookID, 0).CorrectedLemma)
			assert.Empty(t, reviewOccurrence(t, h, bookID, 1).CorrectedLemma)
		},
	}, {
		Name: "a decision for an occurrence the Book's analysis does not contain is not found",
		Run: func(t *testing.T, h Harness) {
			bookID := seedReviewBook(t, h)
			missing := reviewOccurrence(t, h, bookID, 0)
			missing.StartOffset, missing.EndOffset = 900, 903
			proposal := reviewProposal(h, bookID, "correct", "pfad", missing)
			err := h.LemmaReview().PutLemmaDecisionProposal(proposal, previewProposal(t, h, proposal))
			require.ErrorIs(t, err, domain.ErrNotFound)
			assert.Empty(t, reviewOccurrence(t, h, bookID, 0).CorrectedLemma)
		},
	}, {
		Name: "a stale fingerprint after a Known change to the corrected identity is rejected and saves nothing",
		Run: func(t *testing.T, h Harness) {
			bookID := seedReviewBook(t, h)
			proposal := reviewProposal(h, bookID, "correct", "pfad", reviewOccurrence(t, h, bookID, 0))
			fingerprint := previewProposal(t, h, proposal)
			h.Seeds().SeedKnownVocabulary(t, h.Seeds().Owner(), []domain.SnapshotIdentity{{Language: Language, CanonicalLemma: "pfad", UPOS: "NOUN"}})
			err := h.LemmaReview().PutLemmaDecisionProposal(proposal, fingerprint)
			require.ErrorIs(t, err, domain.ErrLemmaReviewPreviewStale)
			assert.Empty(t, reviewOccurrence(t, h, bookID, 0).CorrectedLemma)
		},
	}, {
		Name: "a stale fingerprint after a decision on another occurrence of the form is rejected and saves nothing",
		Run: func(t *testing.T, h Harness) {
			bookID := seedReviewBook(t, h)
			first, second := reviewOccurrence(t, h, bookID, 0), reviewOccurrence(t, h, bookID, 1)
			proposal := reviewProposal(h, bookID, "correct", "pfad", first)
			fingerprint := previewProposal(t, h, proposal)
			other := reviewProposal(h, bookID, "correct", "hof", second)
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(other, previewProposal(t, h, other)))
			err := h.LemmaReview().PutLemmaDecisionProposal(proposal, fingerprint)
			require.ErrorIs(t, err, domain.ErrLemmaReviewPreviewStale)
			assert.Empty(t, reviewOccurrence(t, h, bookID, 0).CorrectedLemma)
		},
	}, {
		Name: "the current reading refuses a confirmed decision on its Book and saves nothing",
		Run: func(t *testing.T, h Harness) {
			bookID := seedReviewBook(t, h)
			_, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
			require.NoError(t, err)
			proposal := reviewProposal(h, bookID, "correct", "pfad", reviewOccurrence(t, h, bookID, 0))
			err = h.LemmaReview().PutLemmaDecisionProposal(proposal, previewProposal(t, h, proposal))
			require.ErrorIs(t, err, domain.ErrLemmaDecisionCurrentReading)
			assert.Empty(t, reviewOccurrence(t, h, bookID, 0).CorrectedLemma)
		},
	}}
}

// lemmaFlagScenarios pins how a confirmed decision resolves a detector flag and
// how later flag saves treat a flag the learner has resolved.
func lemmaFlagScenarios() []Scenario {
	resolutions := []struct {
		name, action, lemma, resolution, corrected string
		excluded                                   bool
	}{
		{name: "correct", action: "correct", lemma: "pfad", resolution: "correct", corrected: "pfad"},
		{name: "keep", action: "keep", lemma: "weg", resolution: "keep"},
		{name: "exclude", action: "exclude", resolution: "exclude", excluded: true},
	}
	scenarios := make([]Scenario, 0, len(resolutions)+1)
	for _, tc := range resolutions {
		scenarios = append(scenarios, Scenario{
			Name: "a " + tc.name + " decision resolves the flag to " + tc.resolution,
			Run: func(t *testing.T, h Harness) {
				bookID := seedReviewBook(t, h)
				flagged := reviewOccurrence(t, h, bookID, 0)
				seedReviewFlag(t, h, flagged, "analyzer lemma needs review")
				proposal := reviewProposal(h, bookID, tc.action, tc.lemma, flagged)
				require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(proposal, previewProposal(t, h, proposal)))
				after := reviewOccurrence(t, h, bookID, 0)
				assert.Equal(t, tc.resolution, after.ReviewFlagResolution)
				assert.Equal(t, tc.corrected, after.CorrectedLemma)
				assert.Equal(t, tc.excluded, after.Excluded)
			},
		})
	}
	return append(scenarios, Scenario{
		Name: "a flag saved after the learner resolved it does not reopen it",
		Run: func(t *testing.T, h Harness) {
			bookID := seedReviewBook(t, h)
			flagged := reviewOccurrence(t, h, bookID, 0)
			seedReviewFlag(t, h, flagged, "analyzer lemma needs review")
			proposal := reviewProposal(h, bookID, "correct", "pfad", flagged)
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(proposal, previewProposal(t, h, proposal)))
			seedReviewFlag(t, h, flagged, "a later detector pass")
			after := reviewOccurrence(t, h, bookID, 0)
			assert.Equal(t, "correct", after.ReviewFlagResolution)
			assert.Equal(t, "pfad", after.CorrectedLemma)
		},
	})
}

// lemmaProviderFlagScenarios pin the advisory provider flag's place across the
// freeze gates. Provider flags never pause a freeze; unresolved local high-risk
// evidence still does, and takes precedence over an unresolved provider flag.
func lemmaProviderFlagScenarios() []Scenario {
	return []Scenario{{
		Name: "an unresolved provider flag does not block start or direct deck preparation",
		Run: func(t *testing.T, h Harness) {
			bookID := seedReviewBook(t, h)
			seedProviderReviewFlag(t, h, reviewOccurrence(t, h, bookID, 0), "provider lemma mismatch")
			reading, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
			require.NoError(t, err)
			_, err = h.Store().PrepareCurrentReadingDeck(h.Seeds().Owner(), bookID, reading.SnapshotID)
			require.NoError(t, err, "a provider flag never pauses a freeze or its deck")
		},
	}, {
		Name: "an unresolved local high-risk flag still blocks start",
		Run: func(t *testing.T, h Harness) {
			bookID := seedReviewBook(t, h)
			seedReviewFlag(t, h, reviewOccurrence(t, h, bookID, 0), "local high-risk evidence")
			_, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
			require.ErrorIs(t, err, domain.ErrUnresolvedLemmaReviewFlags)
		},
	}, {
		Name: "an unresolved provider flag does not block a switch",
		Run: func(t *testing.T, h Harness) {
			first := seedReviewBook(t, h)
			reading, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, first)
			require.NoError(t, err)
			target := seedReviewBook(t, h)
			seedProviderReviewFlag(t, h, reviewOccurrence(t, h, target, 0), "provider lemma mismatch")
			_, err = h.Store().SwitchCurrentReading(h.Seeds().Owner(), Language, target, first, reading.SnapshotID)
			require.NoError(t, err)
		},
	}, {
		Name: "an unresolved local high-risk flag still blocks a switch",
		Run: func(t *testing.T, h Harness) {
			first := seedReviewBook(t, h)
			reading, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, first)
			require.NoError(t, err)
			target := seedReviewBook(t, h)
			seedReviewFlag(t, h, reviewOccurrence(t, h, target, 0), "local high-risk evidence")
			_, err = h.Store().SwitchCurrentReading(h.Seeds().Owner(), Language, target, first, reading.SnapshotID)
			require.ErrorIs(t, err, domain.ErrUnresolvedLemmaReviewFlags)
		},
	}, {
		Name: "a local high-risk flag replaces an unresolved provider flag and blocks start",
		Run: func(t *testing.T, h Harness) {
			bookID := seedReviewBook(t, h)
			flagged := reviewOccurrence(t, h, bookID, 0)
			seedProviderReviewFlag(t, h, flagged, "provider lemma mismatch")
			seedReviewFlag(t, h, flagged, "local high-risk evidence")
			requireLocalHighRiskFlag(t, h, bookID)
		},
	}, {
		Name: "a provider flag leaves an unresolved local high-risk flag unchanged",
		Run: func(t *testing.T, h Harness) {
			bookID := seedReviewBook(t, h)
			flagged := reviewOccurrence(t, h, bookID, 0)
			seedReviewFlag(t, h, flagged, "local high-risk evidence")
			seedProviderReviewFlag(t, h, flagged, "provider lemma mismatch")
			requireLocalHighRiskFlag(t, h, bookID)
		},
	}, {
		Name: "later evidence from the same source leaves the stored flag unchanged",
		Run: func(t *testing.T, h Harness) {
			bookID := seedReviewBook(t, h)
			providerFlagged := reviewOccurrence(t, h, bookID, 0)
			seedProviderReviewFlag(t, h, providerFlagged, "first provider evidence")
			seedProviderReviewFlag(t, h, providerFlagged, "second provider evidence")
			assert.Equal(t, "first provider evidence", reviewOccurrence(t, h, bookID, 0).ReviewFlagReason)

			localFlagged := reviewOccurrence(t, h, bookID, 1)
			seedReviewFlag(t, h, localFlagged, "first local evidence")
			seedReviewFlag(t, h, localFlagged, "second local evidence")
			assert.Equal(t, "first local evidence", reviewOccurrence(t, h, bookID, 1).ReviewFlagReason)
		},
	}, {
		Name: "a provider flag saved after the freeze leaves the reading and ready deck unchanged",
		Run: func(t *testing.T, h Harness) {
			bookID := seedReviewBook(t, h)
			reading, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
			require.NoError(t, err)
			h.Seeds().SeedReadyPreparation(t, h.Seeds().Owner(), bookID, reading.SnapshotID)
			seedProviderReviewFlag(t, h, reviewOccurrence(t, h, bookID, 1), "provider evidence after freeze")

			current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), Language)
			require.NoError(t, err)
			assert.Equal(t, reading.BookID, current.BookID)
			assert.Equal(t, reading.SnapshotID, current.SnapshotID)
			assert.Equal(t, reading.SnapshotSize, current.SnapshotSize)
			_, err = h.Store().PrepareCurrentReadingDeck(h.Seeds().Owner(), bookID, reading.SnapshotID)
			require.ErrorIs(t, err, domain.ErrDeckPreparationInvalidTransition, "the ready deck remains ready")
		},
	}, {
		Name: "a resolved provider flag is not reopened by later provider or local evidence",
		Run: func(t *testing.T, h Harness) {
			bookID := seedReviewBook(t, h)
			flagged := reviewOccurrence(t, h, bookID, 0)
			seedProviderReviewFlag(t, h, flagged, "provider lemma mismatch")
			proposal := reviewProposal(h, bookID, "keep", "weg", flagged)
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(proposal, previewProposal(t, h, proposal)))
			seedProviderReviewFlag(t, h, flagged, "later provider evidence")
			seedReviewFlag(t, h, flagged, "later local evidence")
			after := reviewOccurrence(t, h, bookID, 0)
			assert.Equal(t, "keep", after.ReviewFlagResolution)
			assert.Equal(t, "provider lemma mismatch", after.ReviewFlagReason)
			_, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
			require.NoError(t, err, "a resolved flag never gates a freeze")
		},
	}}
}
