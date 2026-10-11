package storecontract

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// admissionScenarios pin the refusal order of Book deck requests against the
// Current reading. Each refusal is the first rule that applies, in the order the
// domain admission decision checks them.
func admissionScenarios() []Scenario {
	return []Scenario{{
		Name: "admission refuses a stale expected snapshot before anything is queued",
		Run: func(t *testing.T, h Harness) {
			bookID, _ := startReading(t, h, defaultVocabulary)
			_, err := h.Store().PrepareCurrentReadingDeck(h.Seeds().Owner(), bookID, "stale-snapshot")
			require.ErrorIs(t, err, domain.ErrDeckPreparationStale)
		},
	}, {
		Name: "admission refuses to re-prepare a queued preparation",
		Run: func(t *testing.T, h Harness) {
			bookID, reading := startReading(t, h, defaultVocabulary)
			queued, err := h.Store().PrepareCurrentReadingDeck(h.Seeds().Owner(), bookID, reading.SnapshotID)
			require.NoError(t, err)
			assert.Equal(t, domain.DeckPreparationQueued, queued.State)
			_, err = h.Store().RepreparePreparation(h.Seeds().Owner(), queued.ID, reading.SnapshotID)
			require.ErrorIs(t, err, domain.ErrDeckPreparationInvalidTransition)
		},
	}, {
		Name: "admission refuses to submit over a healthy ready preparation",
		Run: func(t *testing.T, h Harness) {
			bookID, reading := startReading(t, h, defaultVocabulary)
			h.Seeds().SeedReadyPreparation(t, h.Seeds().Owner(), bookID, reading.SnapshotID)
			_, err := h.Store().PrepareCurrentReadingDeck(h.Seeds().Owner(), bookID, reading.SnapshotID)
			require.ErrorIs(t, err, domain.ErrDeckPreparationInvalidTransition)
		},
	}, {
		Name: "admission refuses a deck once the reading has ended",
		Run: func(t *testing.T, h Harness) {
			bookID, reading := startReading(t, h, defaultVocabulary)
			require.NoError(t, h.Store().EndCurrentReading(h.Seeds().Owner(), Language, bookID, reading.SnapshotID))
			_, err := h.Store().PrepareCurrentReadingDeck(h.Seeds().Owner(), bookID, reading.SnapshotID)
			require.ErrorIs(t, err, domain.ErrDeckPreparationNotCurrentReading)
		},
	}}
}
