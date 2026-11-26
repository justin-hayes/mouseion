//go:build integration

package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// studyCorrectionCheckBudget is what the correction check that runs before
// every Study card may cost for a word that is still valid. It measured about
// 9 ms for a rare word, and 8 minutes for "der" when it listed every
// occurrence (#2705).
const studyCorrectionCheckBudget = 100 * time.Millisecond

// TestStudyCorrectionCheckStaysFastHoweverCommonTheWordIs seeds the 552,000
// token library of the concordance scale test, where "der" occurs 55,200
// times, and holds that the check of a due word's introducing occurrence costs
// the same for it as for a rare word, and that the search for another valid
// occurrence stops at the first it finds.
func TestStudyCorrectionCheckStaysFastHoweverCommonTheWordIs(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds a large corpus")
	}
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "study-correction-scale", false)
	require.NoError(t, err)
	library := scaleLibrary{books: 12, units: 20, sentencesPerUnit: 230}
	books := seedScaleLibrary(t, ctx, store, owner.ID, library)
	bookIndex := 5
	corpus, run := currentSentenceStudyAnalysis(t, ctx, store, owner.ID, books[bookIndex])
	der := domain.StudyIdentity{Language: "de", CanonicalLemma: "der", UPOS: "DET"}
	aufstehen := domain.StudyIdentity{Language: "de", CanonicalLemma: "aufstehen", UPOS: "VERB"}

	// occurrenceAt names the token a sentence's token holds, the way a Study
	// word's introducing occurrence is located.
	occurrenceAt := func(sentence, token int64) domain.StudyOccurrenceRef {
		unitIndex, within := sentence/int64(library.sentencesPerUnit), sentence%int64(library.sentencesPerUnit)
		start := within*60 + token*6
		return domain.StudyOccurrenceRef{
			BookID: books[bookIndex], AnalysisRunID: run, CorpusID: corpus, SentenceOrdinal: sentence, TokenOrdinal: token,
			UnitID: fmt.Sprintf("epub-unit-v1:%d:scale-%d-%d", unitIndex, bookIndex, unitIndex), StartOffset: start, EndOffset: start + 5,
		}
	}
	timed := func(identity domain.StudyIdentity, source domain.StudyOccurrenceRef) ([]domain.StudyOccurrenceRef, time.Duration) {
		started := time.Now()
		found, err := store.FindStudyOccurrences(ctx, owner.ID, identity, source)
		elapsed := time.Since(started)
		require.NoError(t, err)
		return found, elapsed
	}
	// Warm the connection and the statement cache with another word.
	_, _ = timed(aufstehen, occurrenceAt(3, 4))

	t.Run("a frequent word whose introducing occurrence still resolves", func(t *testing.T) {
		source := occurrenceAt(1000, 0)
		found, elapsed := timed(der, source)
		assert.Equal(t, []domain.StudyOccurrenceRef{source}, found)
		assert.Less(t, elapsed, studyCorrectionCheckBudget, "the check of a word with 55,200 occurrences took %s", elapsed)
	})
	t.Run("a rare word costs the same", func(t *testing.T) {
		source := occurrenceAt(53, 4)
		found, elapsed := timed(aufstehen, source)
		assert.Equal(t, []domain.StudyOccurrenceRef{source}, found)
		assert.Less(t, elapsed, studyCorrectionCheckBudget)
	})
	t.Run("an Analysis update re-published the occurrence in a new run", func(t *testing.T) {
		current := occurrenceAt(2000, 0)
		old := current
		old.AnalysisRunID, old.CorpusID = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
		old.SentenceOrdinal, old.TokenOrdinal = 7, 3
		found, elapsed := timed(der, old)
		assert.Equal(t, []domain.StudyOccurrenceRef{current}, found, "the same token is found at its locator in the current run")
		assert.Less(t, elapsed, studyCorrectionCheckBudget, "relocation took %s", elapsed)
	})
	t.Run("a source with no locator that left the current analysis finds another occurrence", func(t *testing.T) {
		old := occurrenceAt(2000, 0)
		old.AnalysisRunID, old.CorpusID, old.UnitID, old.StartOffset, old.EndOffset = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "", 0, 0
		found, elapsed := timed(der, old)
		require.Len(t, found, 1)
		assert.NotEqual(t, old.AnalysisRunID, found[0].AnalysisRunID)
		assert.Less(t, elapsed, concordanceScaleBudget, "stopping at the first occurrence took %s", elapsed)
	})

	t.Run("an excluded source finds another valid occurrence without listing them all", func(t *testing.T) {
		source := occurrenceAt(1000, 0)
		insertOccurrenceDecision(t, ctx, store, owner.ID, source.BookID, corpus, run, source.UnitID, source.StartOffset, source.EndOffset, "", true)
		found, elapsed := timed(der, source)
		require.Len(t, found, 1)
		assert.NotEqual(t, source, found[0])
		assert.Less(t, elapsed, concordanceScaleBudget, "stopping at the first occurrence took %s", elapsed)
	})
}

// TestStudyCorrectionCheckDecidesAsTheFullListingDoes holds that the check
// reads what the full listing of a word's occurrences would decide from, across
// decisions towards and away from the word, an exclusion, another part of
// speech and another word.
func TestStudyCorrectionCheckDecidesAsTheFullListingDoes(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "study-correction-equivalence", false)
	require.NoError(t, err)
	library := scaleLibrary{books: 3, units: 2, sentencesPerUnit: 100}
	books := seedScaleLibrary(t, ctx, store, owner.ID, library)
	bookIndex := 1
	corpus, run := currentSentenceStudyAnalysis(t, ctx, store, owner.ID, books[bookIndex])
	der := domain.StudyIdentity{Language: "de", CanonicalLemma: "der", UPOS: "DET"}
	aufstehen := domain.StudyIdentity{Language: "de", CanonicalLemma: "aufstehen", UPOS: "VERB"}
	occurrenceAt := func(sentence, token int64) domain.StudyOccurrenceRef {
		unitIndex, within := sentence/int64(library.sentencesPerUnit), sentence%int64(library.sentencesPerUnit)
		start := within*60 + token*6
		return domain.StudyOccurrenceRef{
			BookID: books[bookIndex], AnalysisRunID: run, CorpusID: corpus, SentenceOrdinal: sentence, TokenOrdinal: token,
			UnitID: fmt.Sprintf("epub-unit-v1:%d:scale-%d-%d", unitIndex, bookIndex, unitIndex), StartOffset: start, EndOffset: start + 5,
		}
	}
	insertDecision := func(sentence, token int64, lemma string, excluded bool) {
		ref := occurrenceAt(sentence, token)
		insertOccurrenceDecision(t, ctx, store, owner.ID, ref.BookID, corpus, run, ref.UnitID, ref.StartOffset, ref.EndOffset, lemma, excluded)
	}
	insertDecision(10, 0, "", true)           // a "der" excluded
	insertDecision(11, 0, "aufstehen", false) // a "der" attributed to "aufstehen"
	insertDecision(53, 4, "gehen", false)     // an "aufstehen" attributed to "gehen"
	insertDecision(103, 4, "", true)          // an "aufstehen" excluded
	moved := occurrenceAt(2, 0)
	moved.AnalysisRunID, moved.CorpusID = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	unlocated := occurrenceAt(2, 0)
	unlocated.UnitID, unlocated.StartOffset, unlocated.EndOffset = "", 0, 0

	for _, tc := range []struct {
		name     string
		identity domain.StudyIdentity
		source   domain.StudyOccurrenceRef
	}{
		{"source resolves", der, occurrenceAt(2, 0)},
		{"source excluded", der, occurrenceAt(10, 0)},
		{"source corrected away", aufstehen, occurrenceAt(53, 4)},
		{"source of a word excluded", aufstehen, occurrenceAt(103, 4)},
		{"source corrected in", aufstehen, occurrenceAt(11, 0)},
		{"source in a replaced run, found by its locator", der, moved},
		{"source in a replaced run with no locator", der, unlocated},
		{"source of another part of speech", domain.StudyIdentity{Language: "de", CanonicalLemma: "der", UPOS: "VERB"}, occurrenceAt(12, 0)},
		{"source of another word", aufstehen, occurrenceAt(12, 0)},
		{"unknown identity", domain.StudyIdentity{Language: "de", CanonicalLemma: "nirgendwo", UPOS: "NOUN"}, occurrenceAt(12, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			full, err := store.ListStudyOccurrences(ctx, owner.ID, tc.identity)
			require.NoError(t, err)
			found, err := store.FindStudyOccurrences(ctx, owner.ID, tc.identity, tc.source)
			require.NoError(t, err)
			assert.Equal(t, domain.StudyEvidenceWitnesses(full, tc.source), found)
			evidence := func(valid []domain.StudyOccurrenceRef) domain.StudyEvidence {
				return domain.StudyEvidence{Source: tc.source, Valid: valid}
			}
			assert.Equal(t, domain.ReviewStudyEvidence(evidence(full)), domain.ReviewStudyEvidence(evidence(found)))
		})
	}
}
