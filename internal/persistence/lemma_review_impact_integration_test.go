//go:build integration

package persistence

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reviewImpactBook creates a Book whose analysis repeats one word per entry in
// words, builds its count projection, and returns it.
func reviewImpactBook(t *testing.T, ctx context.Context, store *PostgresStore, ownerID, suffix string, toRead bool, words []string) domain.Book {
	t.Helper()
	text := strings.Join(words, " ")
	end := int64(len([]rune(text)))
	book, source := createConcordanceBook(t, ctx, store, ownerID, "Review impact "+suffix, suffix, toRead,
		[]domain.ExtractedUnit{concordanceUnit(0, suffix, text, 0, uint64(end))})
	tokens := make([]concordanceToken, 0, len(words))
	offset := int64(0)
	for _, word := range words {
		length := int64(len([]rune(word)))
		tokens = append(tokens, concordanceToken{Surface: word, Lemma: strings.ToLower(word), Upos: "NOUN", Start: offset, End: offset + length})
		offset += length + 1
	}
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{{
		UnitID: "epub-unit-v1:0:" + suffix, Ordinal: 0, Text: text, Start: 0, End: end, Tokens: tokens,
	}})
	var runID, corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, ownerID, book.ID).Scan(&runID, &corpusID))
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, BuildVocabularyBrowseCountsTx(ctx, tx, ownerID, book.ID, source.ID, runID, corpusID, "de"))
	require.NoError(t, tx.Commit(ctx))
	return book
}

// TestLemmaReviewImpactAgreesWithSnapshotFreeze previews a decision, confirms
// it, freezes the Current reading, and requires the previewed "after"
// eligibility to match whether the identity is in the frozen snapshot.
func TestLemmaReviewImpactAgreesWithSnapshotFreeze(t *testing.T) {
	repeat := func(word string, n int) []string { return strings.Fields(strings.Repeat(word+" ", n)) }
	tests := []struct {
		name          string
		target        []string
		otherBooks    [][]string
		decision      func(occurrences []domain.LemmaReviewOccurrence) domain.LemmaReviewDecision
		form          string
		wantBefore    bool
		wantAfter     bool
		wantAfterSize int
	}{
		{
			name:       "a correction crosses the three-occurrence threshold",
			target:     append(repeat("Haus", 2), "Heim"),
			form:       "Heim",
			wantBefore: false, wantAfter: true, wantAfterSize: 3,
			decision: func(o []domain.LemmaReviewOccurrence) domain.LemmaReviewDecision {
				return domain.LemmaReviewDecision{Occurrence: o[0], CanonicalLemma: "haus", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}
			},
		},
		{
			name:       "a correction qualifies only through two occurrences with ten across Books",
			target:     append(repeat("Haus", 1), "Heim"),
			otherBooks: [][]string{repeat("Haus", 8)},
			form:       "Heim",
			wantBefore: false, wantAfter: true, wantAfterSize: 2,
			decision: func(o []domain.LemmaReviewOccurrence) domain.LemmaReviewDecision {
				return domain.LemmaReviewDecision{Occurrence: o[0], CanonicalLemma: "haus", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}
			},
		},
		{
			name:       "two occurrences with nine across Books stay ineligible",
			target:     append(repeat("Haus", 1), "Heim"),
			otherBooks: [][]string{repeat("Haus", 7)},
			form:       "Heim",
			wantBefore: false, wantAfter: false, wantAfterSize: 0,
			decision: func(o []domain.LemmaReviewOccurrence) domain.LemmaReviewDecision {
				return domain.LemmaReviewDecision{Occurrence: o[0], CanonicalLemma: "haus", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}
			},
		},
		{
			name:       "an exclusion drops an identity below the threshold",
			target:     repeat("Haus", 3),
			form:       "Haus",
			wantBefore: true, wantAfter: false, wantAfterSize: 0,
			decision: func(o []domain.LemmaReviewOccurrence) domain.LemmaReviewDecision {
				return domain.LemmaReviewDecision{Occurrence: o[0], Excluded: true}
			},
		},
	}
	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
			owner, err := store.CreateUser(ctx, fmt.Sprintf("review-impact-%d", index), false)
			require.NoError(t, err)
			for i, words := range tt.otherBooks {
				reviewImpactBook(t, ctx, store, owner.ID, fmt.Sprintf("other%d", i), false, words)
			}
			book := reviewImpactBook(t, ctx, store, owner.ID, "target", true, tt.target)

			occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, tt.form)
			require.NoError(t, err)
			decisions := []domain.LemmaReviewDecision{tt.decision(occurrences)}
			counts, err := store.PreviewLemmaDecisionCounts(ctx, owner.ID, book.ID, decisions)
			require.NoError(t, err)
			var haus selection.Impact
			for _, impact := range selection.NewEligibility(nil, nil).ReviewImpact(toImpactCounts(counts)) {
				if impact.Identity.CanonicalLemma == "haus" {
					haus = impact
				}
			}
			assert.Equal(t, tt.wantBefore, haus.BeforeEligible)
			assert.Equal(t, tt.wantAfter, haus.AfterEligible)

			unchanged, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, tt.form)
			require.NoError(t, err)
			assert.Equal(t, occurrences, unchanged, "previewing writes nothing")

			require.NoError(t, store.PutLemmaDecisions(ctx, decisions))
			reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
			require.NoError(t, err)
			snapshot, err := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
			require.NoError(t, err)
			frozen := 0
			for _, candidate := range snapshot {
				if candidate.CanonicalLemma == "haus" {
					frozen = candidate.OccurrenceCount
				}
			}
			assert.Equal(t, haus.AfterEligible, frozen > 0, "the previewed eligibility is what the freeze selects")
			assert.Equal(t, tt.wantAfterSize, frozen)
		})
	}
}

func toImpactCounts(counts []domain.LemmaDecisionCounts) []selection.ImpactCounts {
	result := make([]selection.ImpactCounts, 0, len(counts))
	for _, c := range counts {
		result = append(result, selection.ImpactCounts{
			Identity: selection.Identity{Language: c.Language, CanonicalLemma: c.CanonicalLemma, UPOS: c.UPOS},
			Before:   c.Before, After: c.After, OtherBooks: c.OtherBooks,
		})
	}
	return result
}

func TestPreviewLemmaDecisionCountsWithholdsUntilProjectionsAreReady(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "review-impact-pending", false)
	require.NoError(t, err)
	book := reviewImpactBook(t, ctx, store, owner.ID, "pending", false, []string{"Haus", "Heim"})
	occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Heim")
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID)
	require.NoError(t, err)
	_, err = store.PreviewLemmaDecisionCounts(ctx, owner.ID, book.ID, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: "haus"}})
	require.ErrorIs(t, err, ErrVocabularyBrowseCountsPending)
}
