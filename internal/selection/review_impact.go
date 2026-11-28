package selection

import (
	"sort"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// DefaultRecurringMinOccurrences and DefaultAcrossBooksMinOccurrences are the
// Reading frequency floors lemma review previews against. The across-Books
// floor is provisional (issue #1803): an absolute library total that needs
// re-measuring as the learner's library grows.
const (
	DefaultRecurringMinOccurrences   = 3
	DefaultAcrossBooksMinOccurrences = 15
)

// readingFrequencyEligible is the Reading frequency rule of the lemma review
// impact preview: three occurrences in the Book, or at least one with fifteen
// across the learner's currently analyzed Books, including this one.
func readingFrequencyEligible(inBook, acrossBooks int64) bool {
	return inBook >= DefaultRecurringMinOccurrences || (inBook >= 1 && acrossBooks >= DefaultAcrossBooksMinOccurrences)
}

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
	sort.Slice(impacts, func(i, j int) bool { return identityLess(impacts[i].Identity, impacts[j].Identity) })
	return impacts
}
