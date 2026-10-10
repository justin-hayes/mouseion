package testutil

import (
	"sort"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// FinishCase is one fresh Finish of a German Current reading. The fixture and
// Postgres stores must both produce exactly the expected counts and Known
// vocabulary for the same facts.
type FinishCase struct {
	Name string
	// Snapshot holds the frozen identities; Known the pre-existing Known rows
	// (an empty UPOS is the lemma wildcard).
	Snapshot, Known []domain.SnapshotIdentity
	// Expected Completion counts.
	SnapshotCount, EligibleCount, GraduatedCount, AlreadyKnownCount int
	// KnownAfter lists "lemma|UPOS" for every Known row after Finish.
	KnownAfter []string
}

func germanIdentity(lemma, upos string) domain.SnapshotIdentity {
	return domain.SnapshotIdentity{Language: "de", CanonicalLemma: lemma, UPOS: upos}
}

// FinishCases is the shared table of facts and expected outcomes.
func FinishCases() []FinishCase {
	return []FinishCase{{
		// Every snapshot identity is Reserved by the Current reading; Known
		// decides which of them are already accepted.
		Name: "mixed Known, Reserved, and new identities",
		Snapshot: []domain.SnapshotIdentity{
			germanIdentity("haus", "NOUN"), germanIdentity("gehen", "VERB"),
			germanIdentity("weg", "NOUN"), germanIdentity("sehen", "VERB"),
		},
		Known: []domain.SnapshotIdentity{
			germanIdentity("haus", "NOUN"), germanIdentity("gehen", ""), germanIdentity("weg", "VERB"),
		},
		SnapshotCount: 4, EligibleCount: 2, GraduatedCount: 2, AlreadyKnownCount: 2,
		KnownAfter: []string{"gehen|", "haus|NOUN", "sehen|VERB", "weg|NOUN", "weg|VERB"},
	}, {
		Name:          "every identity already Known",
		Snapshot:      []domain.SnapshotIdentity{germanIdentity("haus", "NOUN"), germanIdentity("gehen", "VERB")},
		Known:         []domain.SnapshotIdentity{germanIdentity("haus", ""), germanIdentity("gehen", "VERB")},
		SnapshotCount: 2, EligibleCount: 0, GraduatedCount: 0, AlreadyKnownCount: 2,
		KnownAfter: []string{"gehen|VERB", "haus|"},
	}, {
		Name:          "nothing Known yet",
		Snapshot:      []domain.SnapshotIdentity{germanIdentity("haus", "NOUN"), germanIdentity("gehen", "VERB")},
		SnapshotCount: 2, EligibleCount: 2, GraduatedCount: 2, AlreadyKnownCount: 0,
		KnownAfter: []string{"gehen|VERB", "haus|NOUN"},
	}, {
		Name:       "empty snapshot",
		Known:      []domain.SnapshotIdentity{germanIdentity("haus", "NOUN")},
		KnownAfter: []string{"haus|NOUN"},
	}}
}

// KnownKeys renders Known rows as the sorted "lemma|UPOS" keys KnownAfter uses.
func KnownKeys(known []domain.KnownVocabulary) []string {
	keys := make([]string, 0, len(known))
	for _, item := range known {
		keys = append(keys, item.CanonicalLemma+"|"+item.UPOS)
	}
	sort.Strings(keys)
	return keys
}
