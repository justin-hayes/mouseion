package selection

import (
	"sort"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// ReviewIdentityNow is the vocabulary identity a reviewed occurrence counts
// under before a proposal: its learner correction if any, otherwise the analyzer
// lemma. An excluded occurrence, or one that does not count, has the zero
// Identity.
func ReviewIdentityNow(language string, occurrence domain.LemmaReviewOccurrence) Identity {
	var decision *OccurrenceDecision
	switch {
	case occurrence.Excluded:
		decision = &OccurrenceDecision{Excluded: true}
	case occurrence.CorrectedLemma != "":
		decision = &OccurrenceDecision{Lemma: occurrence.CorrectedLemma}
	}
	return reviewIdentity(language, occurrence, decision)
}

// ReviewIdentityAfter is the vocabulary identity a reviewed occurrence counts
// under once the proposed decision applies. The zero Identity means it counts as
// nothing.
func ReviewIdentityAfter(language string, occurrence domain.LemmaReviewOccurrence, decision domain.LemmaReviewDecision) Identity {
	return reviewIdentity(language, occurrence, &OccurrenceDecision{Lemma: decision.CanonicalLemma, Excluded: decision.Excluded})
}

func reviewIdentity(language string, occurrence domain.LemmaReviewOccurrence, decision *OccurrenceDecision) Identity {
	id, counts := Effective(DefaultConfig(""), language, OccurrenceFact{Lemma: occurrence.CanonicalLemma, UPOS: occurrence.UPOS, Dependency: occurrence.Dependency}, decision)
	if !counts {
		return Identity{}
	}
	return id
}

// ProposalIdentities lists the distinct identities a proposal moves, before or
// after, in lemma then UPOS order. Only these identities can change a preview's
// impact or its state fingerprint.
func ProposalIdentities(proposal domain.LemmaReviewProposal) []domain.LemmaReviewIdentity {
	seen := make(map[domain.LemmaReviewIdentity]bool)
	for _, decision := range proposal.Decisions() {
		for _, identity := range []Identity{
			ReviewIdentityNow(proposal.Language, decision.Occurrence),
			ReviewIdentityAfter(proposal.Language, decision.Occurrence, decision),
		} {
			if identity != (Identity{}) {
				seen[domain.LemmaReviewIdentity(identity)] = true
			}
		}
	}
	result := make([]domain.LemmaReviewIdentity, 0, len(seen))
	for identity := range seen {
		result = append(result, identity)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CanonicalLemma != result[j].CanonicalLemma {
			return result[i].CanonicalLemma < result[j].CanonicalLemma
		}
		return result[i].UPOS < result[j].UPOS
	})
	return result
}
