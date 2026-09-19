package selection

import "github.com/justin-hayes/mouseion/internal/domain"

const DefaultRecurringMinOccurrences = 3

// Eligibility contains the learner-state exclusions shared by recurring-
// vocabulary selectors. Generated vocabulary is deliberately absent: it is
// provenance, not learner state.
type Eligibility struct {
	known    map[Identity]struct{}
	reserved map[Identity]struct{}
}

func NewEligibility(known []domain.KnownVocabulary, reserved []domain.DeckPreparationVocabulary) Eligibility {
	result := Eligibility{
		known:    make(map[Identity]struct{}, len(known)),
		reserved: make(map[Identity]struct{}, len(reserved)),
	}
	for _, word := range known {
		result.known[Identity{Language: word.Language, CanonicalLemma: word.CanonicalLemma, UPOS: word.UPOS}] = struct{}{}
	}
	for _, word := range reserved {
		result.reserved[Identity{Language: word.Language, CanonicalLemma: word.CanonicalLemma, UPOS: word.UPOS}] = struct{}{}
	}
	return result
}

// Allows applies the recurring-vocabulary frequency floor and learner-state
// exclusions. An empty Known UPOS is the lemma wildcard; reservations remain
// exact lemma-plus-UPOS identities.
func (e Eligibility) Allows(candidate domain.SelectionCandidate, minOccurrences int) bool {
	identity := Identity{Language: candidate.Language, CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS}
	return allowsIdentity(candidate.OccurrenceCount, minOccurrences, e.knownContains(identity), e.reservedContains(identity))
}

func (e Eligibility) knownContains(identity Identity) bool {
	if _, ok := e.known[identity]; ok {
		return true
	}
	_, ok := e.known[Identity{Language: identity.Language, CanonicalLemma: identity.CanonicalLemma}]
	return ok
}

func (e Eligibility) reservedContains(identity Identity) bool {
	_, ok := e.reserved[identity]
	return ok
}

func allowsIdentity(occurrenceCount, minOccurrences int, known, reserved bool) bool {
	return minOccurrences >= 1 && occurrenceCount >= minOccurrences && !known && !reserved
}
