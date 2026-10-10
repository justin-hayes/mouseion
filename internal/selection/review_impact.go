package selection

import "sort"

// ImpactCounts carries the effective counts of one vocabulary identity around a
// proposed lemma decision. Before and After come from the same subset
// projection the decision recompute uses; OtherBooks is the identity's count in
// the learner's other currently analyzed Books.
type ImpactCounts struct {
	Identity   Identity
	Before     int64
	After      int64
	OtherBooks int64
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
// would freeze. Results are ordered by lemma then UPOS.
func (e Eligibility) ReviewImpact(counts []ImpactCounts) []Impact {
	impacts := make([]Impact, 0, len(counts))
	for _, c := range counts {
		known, reserved := e.knownContains(c.Identity), e.reservedContains(c.Identity)
		eligible := func(inBook int64) bool {
			return readingFrequencyEligible(inBook, c.OtherBooks+inBook) && !known && !reserved
		}
		impacts = append(impacts, Impact{
			Identity: c.Identity, BeforeCount: c.Before, AfterCount: c.After, Known: known, Reserved: reserved,
			BeforeEligible: eligible(c.Before), AfterEligible: eligible(c.After),
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
