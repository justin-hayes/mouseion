package fixtures

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// otherLearnerID owns Known rows the finishing learner must never see.
const otherLearnerID = "other-learner"

// TestFixtureFinishMatchesSharedFinishCases pins the fixture store to the same
// outcomes the Postgres store is held to for the same facts.
func TestFixtureFinishMatchesSharedFinishCases(t *testing.T) {
	for _, tc := range testutil.FinishCases() {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.Background()
			store := NewStore()
			const snapshotID = "fixture-de-goal-snapshot"
			var snapshot []domain.DeckPreparationVocabulary
			for _, identity := range tc.Snapshot {
				snapshot = append(snapshot, domain.DeckPreparationVocabulary{OwnerID: OwnerID, Language: identity.Language, CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS})
			}
			store.goalSnapshotVocabulary[snapshotID] = snapshot
			store.known = nil
			for _, identity := range tc.Known {
				store.known = append(store.known, domain.KnownVocabulary{OwnerID: OwnerID, Language: identity.Language, CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS})
			}
			for _, identity := range tc.OtherLearnerKnown {
				store.known = append(store.known, domain.KnownVocabulary{OwnerID: otherLearnerID, Language: identity.Language, CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS})
			}
			expectedSnapshot := snapshotID
			if tc.Legacy {
				goalKey := fixtureGoalKey(OwnerID, "de")
				goal := store.currentReadings[goalKey]
				goal.SnapshotID = ""
				store.currentReadings[goalKey] = goal
				expectedSnapshot = ""
			}

			_, err := store.FinishCurrentReading(ctx, OwnerID, "de", "other-book", expectedSnapshot)
			require.ErrorIs(t, err, persistence.ErrCurrentReadingStale)

			result, err := store.FinishCurrentReading(ctx, OwnerID, "de", BookID, expectedSnapshot)
			require.NoError(t, err)
			assert.Equal(t, tc.SnapshotCount, result.Completion.SnapshotVocabularyCount)
			assert.Equal(t, tc.EligibleCount, result.Completion.EligibleVocabularyCount)
			assert.Equal(t, tc.GraduatedCount, result.Completion.GraduatedVocabularyCount)
			assert.Equal(t, tc.AlreadyKnownCount, result.Completion.AlreadyKnownVocabularyCount)
			known, err := store.ListKnownVocabulary(ctx, OwnerID, "de")
			require.NoError(t, err)
			assert.Equal(t, tc.KnownAfter, testutil.KnownKeys(known))

			replay, err := store.FinishCurrentReading(ctx, OwnerID, "de", BookID, expectedSnapshot)
			require.NoError(t, err)
			assert.Equal(t, result, replay)
		})
	}
}
