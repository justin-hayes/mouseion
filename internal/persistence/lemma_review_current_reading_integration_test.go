//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A preview made while the Book was still To Read can commit after a Start
// made it the current reading. The fingerprint below is read after the Start,
// so the state check passes and only the current-reading gate can reject it.
func TestLemmaDecisionProposalCommittedAfterStartIsRejectedAndSavesNothing(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "review-current-reading-owner", false)
	require.NoError(t, err)
	target := reviewImpactBook(t, ctx, store, owner.ID, "current-gate", true, []string{"Haus", "Heim", "Dach"})
	occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, target.ID, "Heim")
	require.NoError(t, err)
	require.Len(t, occurrences, 1)
	affected := []domain.LemmaReviewIdentity{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"},
		{Language: "de", CanonicalLemma: "heim", UPOS: "NOUN"},
	}
	decisions := []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: "haus", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}

	_, err = store.StartCurrentReading(ctx, owner.ID, "de", target.ID)
	require.NoError(t, err)
	fingerprint, err := store.LemmaReviewStateFingerprint(ctx, owner.ID, target.ID, "de", "Heim", affected)
	require.NoError(t, err)

	err = store.PutLemmaDecisionProposal(ctx, decisions, "Heim", "de", affected, fingerprint)
	require.ErrorIs(t, err, ErrLemmaDecisionCurrentReading)
	require.EqualError(t, err, "Stop this Book's current reading before changing its vocabulary.")
	unchanged, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, target.ID, "Heim")
	require.NoError(t, err)
	assert.Empty(t, unchanged[0].CorrectedLemma, "a rejected decision persists nothing")
}
