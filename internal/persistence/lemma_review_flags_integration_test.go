//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSaveLemmaReviewFlagsRejectsUnsupportedEvidence covers the guard branches
// of the flag writer: an empty batch saves nothing, and a flag for a missing
// Book, for another Book than the batch's first flag, outside the Book's
// current analysis, or without a reason is rejected without saving the batch.
func TestSaveLemmaReviewFlagsRejectsUnsupportedEvidence(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "review-flag-guard", false)
	require.NoError(t, err)

	const (
		text   = "Haus Heim"
		unitID = "epub-unit-v1:0:flag-guard"
	)
	heimEnd := int64(len([]rune(text)))
	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Flag guard", "flag-guard", true,
		[]domain.ExtractedUnit{concordanceUnit(0, "flag-guard", text, 0, uint64(heimEnd))})
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{{
		UnitID: unitID, Ordinal: 0, Text: text, Start: 0, End: heimEnd,
		Tokens: []concordanceToken{
			{Surface: "Haus", Lemma: "Haus", Upos: "NOUN", Start: 0, End: 4},
			{Surface: "Heim", Lemma: "Heim", Upos: "NOUN", Start: 5, End: heimEnd},
		},
	}})
	occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Heim")
	require.NoError(t, err)
	require.Len(t, occurrences, 1)

	t.Run("an empty batch is a no-op", func(t *testing.T) {
		require.NoError(t, store.SaveLemmaReviewFlags(ctx, nil))
	})

	t.Run("a flag for an unknown Book is not found", func(t *testing.T) {
		missing := occurrences[0]
		missing.BookID = "00000000-0000-0000-0000-000000000000"
		err := store.SaveLemmaReviewFlags(ctx, []domain.LemmaReviewFlag{{Occurrence: missing, Reason: "detector"}})
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("a flag that leaves the first flag's Book is not found and saves nothing", func(t *testing.T) {
		other := occurrences[0]
		other.BookID = "00000000-0000-0000-0000-000000000000"
		err := store.SaveLemmaReviewFlags(ctx, []domain.LemmaReviewFlag{
			{Occurrence: occurrences[0], Reason: "detector"},
			{Occurrence: other, Reason: "detector"},
		})
		require.ErrorIs(t, err, ErrNotFound)
		unchanged, listErr := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Heim")
		require.NoError(t, listErr)
		require.Len(t, unchanged, 1)
		assert.Empty(t, unchanged[0].ReviewFlagReason, "the rejected batch rolls back the valid flag")
	})

	t.Run("an occurrence outside the current analysis is not found", func(t *testing.T) {
		forged := occurrences[0]
		forged.StartOffset, forged.EndOffset = 9000, 9004
		err := store.SaveLemmaReviewFlags(ctx, []domain.LemmaReviewFlag{{Occurrence: forged, Reason: "detector"}})
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("a blank reason is not found", func(t *testing.T) {
		err := store.SaveLemmaReviewFlags(ctx, []domain.LemmaReviewFlag{{Occurrence: occurrences[0], Reason: "   "}})
		require.ErrorIs(t, err, ErrNotFound)
	})
}
