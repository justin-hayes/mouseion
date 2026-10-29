package domain

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestLemmaMismatchOmissionReasonUsesTheReservedPrefix(t *testing.T) {
	reason := DeckPreparationLemmaMismatchOmissionReason("The sentence uses fernsehen, not sehen.")

	t.Run("stores the reserved prefix and provider reason", func(t *testing.T) {
		require.Equal(t, DeckPreparationLemmaMismatchPrefix+"The sentence uses fernsehen, not sehen.", reason)
	})
	t.Run("recognises only the reserved prefix", func(t *testing.T) {
		require.True(t, IsDeckPreparationLemmaMismatchOmission(reason))
		require.False(t, IsDeckPreparationLemmaMismatchOmission("The sentence does not distinguish the meanings."))
	})
}

func TestLemmaMismatchOmissionReasonStaysWithinTheStoredBound(t *testing.T) {
	reason := DeckPreparationLemmaMismatchOmissionReason(strings.Repeat("r", 400))

	t.Run("stays within the persisted bound", func(t *testing.T) {
		require.LessOrEqual(t, utf8.RuneCountInString(reason), 200)
	})
	t.Run("keeps the reserved prefix", func(t *testing.T) {
		require.True(t, strings.HasPrefix(reason, DeckPreparationLemmaMismatchPrefix))
	})
}
