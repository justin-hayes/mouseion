package prepareddeck

import (
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// contextualOmissionReason stores a contextual-gloss abstention through the
// existing omission reason. A lemma mismatch carries the reserved, domain-owned
// prefix so the preparation result can count it separately; every other
// omission keeps the provider's bounded reason unchanged.
func contextualOmissionReason(response enrichment.TranslationResponse) string {
	reason := strings.TrimSpace(response.UnresolvedReason)
	if response.UnresolvedKind == enrichment.ContextualGlossUnresolvedLemmaMismatch {
		return domain.DeckPreparationLemmaMismatchOmissionReason(reason)
	}
	if response.UnresolvedKind == enrichment.ContextualGlossUnresolvedLiteralUse {
		// An idiom omission is labelled separately and never raises a lemma
		// review flag (ADR 0114).
		return domain.DeckPreparationIdiomLiteralUseLabel
	}
	if domain.IsDeckPreparationLemmaMismatchOmission(reason) {
		// An ambiguous provider reason must never masquerade as the reserved
		// lemma-mismatch kind when the preparation result classifies it later.
		reason = strings.TrimSpace(strings.TrimPrefix(reason, domain.DeckPreparationLemmaMismatchPrefix))
	}
	return reason
}
