//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPutLemmaDecisionsValidatesAgainstTheCurrentAnalysis covers decision writes
// that leave stored correction state unchanged: an occurrence outside the
// Book's current analysis is rejected, and reapplying the analyzed identity is a
// silent no-op.
func TestPutLemmaDecisionsValidatesAgainstTheCurrentAnalysis(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "review-decision-validation", false)
	require.NoError(t, err)

	const (
		text   = "Haus Heim"
		unitID = "epub-unit-v1:0:decision-validation"
	)
	heimEnd := int64(len([]rune(text)))
	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Review decision validation", "decision-validation", false,
		[]domain.ExtractedUnit{concordanceUnit(0, "decision-validation", text, 0, uint64(heimEnd))})
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

	t.Run("an occurrence outside the current analysis is not found", func(t *testing.T) {
		forged := occurrences[0]
		forged.StartOffset, forged.EndOffset = 9000, 9004
		err := store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: forged, CanonicalLemma: "haus"}})
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("reapplying the analyzed identity is a no-op", func(t *testing.T) {
		err := store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{
			Occurrence: occurrences[0], CanonicalLemma: occurrences[0].CanonicalLemma,
		}})
		require.NoError(t, err)
		unchanged, listErr := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Heim")
		require.NoError(t, listErr)
		require.Len(t, unchanged, 1)
		assert.Empty(t, unchanged[0].CorrectedLemma, "a no-op decision persists nothing")
	})
}
