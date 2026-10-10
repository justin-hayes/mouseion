//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLemmaReviewScopeFollowsSelectionEligibility checks that listing and
// writing agree with selection.OccurrenceEligible rather than a SQL POS list.
func TestLemmaReviewScopeFollowsSelectionEligibility(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "review-eligibility", false)
	require.NoError(t, err)

	words := []string{"Haus", "ab", "Heim", "Rot"}
	text := "Haus ab Heim Rot"
	end := int64(len([]rune(text)))
	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Review eligibility", "eligibility", false,
		[]domain.ExtractedUnit{concordanceUnit(0, "eligibility", text, 0, uint64(end))})
	specs := []concordanceToken{
		{Upos: "NOUN"},
		{Upos: "ADV", Dependency: "compound:prt"},
		{Upos: " NOUN "},
		{Upos: "ADP"},
	}
	tokens := make([]concordanceToken, 0, len(words))
	offset := int64(0)
	for i, word := range words {
		length := int64(len([]rune(word)))
		spec := specs[i]
		spec.Surface, spec.Lemma, spec.Start, spec.End = word, word, offset, offset+length
		tokens = append(tokens, spec)
		offset += length + 1
	}
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{{
		UnitID: "epub-unit-v1:0:eligibility", Ordinal: 0, Text: text, Start: 0, End: end, Tokens: tokens,
	}})

	list := func(form string) []domain.LemmaReviewOccurrence {
		t.Helper()
		occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, form)
		require.NoError(t, err)
		return occurrences
	}
	assert.Empty(t, list("ab"), "a separable particle is not listed")
	assert.Empty(t, list("Rot"), "a non-content occurrence is not listed")
	require.Len(t, list(""), 2, "only content occurrences are listed")

	padded := list("Heim")
	require.Len(t, padded, 1, "padded part of speech that selection counts is listed")
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{
		Occurrence: padded[0], CanonicalLemma: "haus", NormalizationProfile: "german-post-1996", NormalizationVersion: "6",
	}}))
	assert.Equal(t, "haus", list("Heim")[0].CorrectedLemma)

	forged := func(token concordanceToken) domain.LemmaReviewOccurrence {
		occurrence := padded[0]
		occurrence.Surface, occurrence.RawLemma, occurrence.CanonicalLemma, occurrence.UPOS = token.Surface, token.Lemma, token.Lemma, token.Upos
		occurrence.StartOffset, occurrence.EndOffset = token.Start, token.End
		occurrence.CorrectedLemma, occurrence.Excluded = "", false
		return occurrence
	}
	for name, token := range map[string]concordanceToken{"separable particle": tokens[1], "non-content": tokens[3]} {
		for _, excluded := range []bool{false, true} {
			err := store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{
				Occurrence: forged(token), CanonicalLemma: "x" + token.Surface, Excluded: excluded,
				NormalizationProfile: "german-post-1996", NormalizationVersion: "6",
			}})
			require.ErrorIs(t, err, ErrNotFound, "%s excluded=%v", name, excluded)
		}
	}
	var written int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&written))
	assert.Equal(t, 1, written, "only the eligible correction was written")
}
