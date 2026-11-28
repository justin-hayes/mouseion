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
	return h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{Vocabulary: defaultVocabulary, Occurrences: reviewOccurrences})
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

func lemmaReviewScenarios() []Scenario {
	scenarios := lemmaDecisionScenarios()
	return append(scenarios, lemmaCorpusScenarios()...)
}

// lemmaCorpusScenarios pin the Corpus-wide lookup the Wrong-word remediation
// screen offers matching occurrences from, and that a decision on one Book's
// occurrence leaves every other occurrence unchanged (ADR 0136 §6).
func lemmaCorpusScenarios() []Scenario {
	return []Scenario{{
		Name: "the Corpus lists the occurrences of a form and lemma in every Book of My Books, and a decision on one leaves the rest unchanged",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			first := seedReviewBook(t, h)
			second := seedReviewBook(t, h)
			// Another lemma for the same form, and another form of the lemma,
			// are not matches.
			h.Seeds().SeedBook(t, owner, BookSeed{Vocabulary: defaultVocabulary, Occurrences: []OccurrenceSeed{
				{Surface: reviewSurface, Lemma: "wegen", UPOS: "ADP"}, {Surface: "Wege", Lemma: "weg", UPOS: "NOUN"},
			}})
			removed := seedReviewBook(t, h)
			require.NoError(t, h.BookState().RemoveBookFromMyBooks(owner, removed))

			found, err := h.LemmaReview().ListCorpusLemmaReviewOccurrences(owner, Language, reviewSurface, "weg")
			require.NoError(t, err)
			books := map[string]int{}
			for _, occurrence := range found {
				books[occurrence.BookID]++
				assert.Equal(t, owner, occurrence.OwnerID)
			}
			assert.Equal(t, map[string]int{first: 2, second: 2}, books, "both Books in My Books, none removed")

			corrected := reviewProposal(h, first, "correct", "pfad", reviewOccurrence(t, h, first, 0))
			require.NoError(t, h.LemmaReview().PutLemmaDecisionProposal(corrected, previewProposal(t, h, corrected)))
			found, err = h.LemmaReview().ListCorpusLemmaReviewOccurrences(owner, Language, reviewSurface, "weg")
			require.NoError(t, err)
			decided := 0
			for _, occurrence := range found {
				if occurrence.CorrectedLemma != "" {
					decided++
					assert.Equal(t, first, occurrence.BookID)
				}
			}
			assert.Equal(t, 1, decided, "only the decided occurrence carries the correction")
			assert.Empty(t, reviewOccurrence(t, h, second, 0).CorrectedLemma)
		},
	}}
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
