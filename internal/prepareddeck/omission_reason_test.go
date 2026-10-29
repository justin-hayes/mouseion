package prepareddeck

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/require"
)

func TestContextualOmissionReasonUsesTheReservedLemmaMismatchPrefix(t *testing.T) {
	reason := contextualOmissionReason(enrichment.TranslationResponse{
		UnresolvedKind:   enrichment.ContextualGlossUnresolvedLemmaMismatch,
		UnresolvedReason: "The sentence uses fernsehen, not sehen.",
	})

	t.Run("stores the prefix and provider reason", func(t *testing.T) {
		require.Equal(t, domain.DeckPreparationLemmaMismatchPrefix+"The sentence uses fernsehen, not sehen.", reason)
	})
	t.Run("is recognised as a lemma mismatch", func(t *testing.T) {
		require.True(t, domain.IsDeckPreparationLemmaMismatchOmission(reason))
	})
}

func TestContextualOmissionReasonDoesNotLetAmbiguousReasonsMasqueradeAsLemmaMismatch(t *testing.T) {
	reason := contextualOmissionReason(enrichment.TranslationResponse{
		UnresolvedKind:   enrichment.ContextualGlossUnresolvedAmbiguous,
		UnresolvedReason: domain.DeckPreparationLemmaMismatchPrefix + "not really a mismatch",
	})

	t.Run("strips the reserved prefix", func(t *testing.T) {
		require.Equal(t, "not really a mismatch", reason)
	})
	t.Run("is not classified as a lemma mismatch", func(t *testing.T) {
		require.False(t, domain.IsDeckPreparationLemmaMismatchOmission(reason))
	})
}

func TestContextualOmissionReasonKeepsAmbiguousOmissionsUnchanged(t *testing.T) {
	const detail = "The sentence does not distinguish the meanings."

	reason := contextualOmissionReason(enrichment.TranslationResponse{
		UnresolvedKind:   enrichment.ContextualGlossUnresolvedAmbiguous,
		UnresolvedReason: detail,
	})

	t.Run("keeps the provider reason", func(t *testing.T) {
		require.Equal(t, detail, reason)
	})
	t.Run("is not recognised as a lemma mismatch", func(t *testing.T) {
		require.False(t, domain.IsDeckPreparationLemmaMismatchOmission(reason))
	})
}
