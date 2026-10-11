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

func lemmaReviewScenarios() []Scenario {
	scenarios := lemmaDecisionScenarios()
	return append(scenarios, lemmaFlagScenarios()...)
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
			require.EqualError(t, err, "Stop this Book's current reading before changing its vocabulary.")
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
