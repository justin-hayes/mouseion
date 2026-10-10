package selection

import (
	"sort"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// Impact is what a proposed lemma decision does to one identity's Reading
// eligibility.
type Impact struct {
	Identity       Identity
	BeforeCount    int64
	AfterCount     int64
	Known          bool
	Reserved       bool
	BeforeEligible bool
	AfterEligible  bool
}

// ReviewImpact judges each identity with the Reading eligibility the snapshot
// freeze uses, so a previewed "becomes eligible" agrees with what confirming
// would freeze. The states come from one preview read. Results are ordered by
// lemma then UPOS.
func ReviewImpact(states []domain.LemmaReviewIdentityState) []Impact {
	impacts := make([]Impact, 0, len(states))
	for _, state := range states {
		eligible := func(inBook int64) bool {
			return readingFrequencyEligible(inBook, state.OtherBooks+inBook) && !state.Known && !state.Reserved
		}
		impacts = append(impacts, Impact{
			Identity: Identity(state.Identity), BeforeCount: state.InBook, AfterCount: state.AfterInBook,
			Known: state.Known, Reserved: state.Reserved,
			BeforeEligible: eligible(state.InBook), AfterEligible: eligible(state.AfterInBook),
		})
	}
	sort.Slice(impacts, func(i, j int) bool {
		if impacts[i].Identity.CanonicalLemma != impacts[j].Identity.CanonicalLemma {
			return impacts[i].Identity.CanonicalLemma < impacts[j].Identity.CanonicalLemma
		}
		return impacts[i].Identity.UPOS < impacts[j].Identity.UPOS
	})
	return impacts
}
