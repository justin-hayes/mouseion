package storecontract

import (
	"sort"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// finishCase is one fresh Finish of a German Current reading. Every adapter must
// produce exactly the expected counts and Known vocabulary for the same facts.
type finishCase struct {
	Name string
	// Snapshot holds the identities the reading freezes; Known the Known rows
	// recorded after the Start.
	Snapshot, Known []domain.SnapshotIdentity
	// OtherLearnerKnown holds Known rows of another learner. They never count for
	// the finishing learner.
	OtherLearnerKnown []domain.SnapshotIdentity
	// Legacy names a reading that never received a snapshot, so Finish is named
	// by an empty snapshot ID.
	Legacy bool
	// Expected Completion counts.
	SnapshotCount, EligibleCount, GraduatedCount, AlreadyKnownCount int
	// KnownAfter lists "lemma|UPOS" for every Known row after Finish.
	KnownAfter []string
}

// finishCases is the shared table of facts and expected outcomes.
func finishCases() []finishCase {
	return []finishCase{{
		// Every snapshot identity is Reserved by the Current reading; Known
		// decides which of them are already accepted.
		Name: "mixed Known, Reserved, and new identities",
		Snapshot: []domain.SnapshotIdentity{
			german("haus", "NOUN"), german("gehen", "VERB"),
			german("weg", "NOUN"), german("sehen", "VERB"),
		},
		Known: []domain.SnapshotIdentity{
			german("haus", "NOUN"), german("gehen", ""), german("weg", "VERB"),
		},
		SnapshotCount: 4, EligibleCount: 2, GraduatedCount: 2, AlreadyKnownCount: 2,
		KnownAfter: []string{"gehen|", "haus|NOUN", "sehen|VERB", "weg|NOUN", "weg|VERB"},
	}, {
		Name:          "every identity already Known",
		Snapshot:      []domain.SnapshotIdentity{german("haus", "NOUN"), german("gehen", "VERB")},
		Known:         []domain.SnapshotIdentity{german("haus", ""), german("gehen", "VERB")},
		SnapshotCount: 2, EligibleCount: 0, GraduatedCount: 0, AlreadyKnownCount: 2,
		KnownAfter: []string{"gehen|VERB", "haus|"},
	}, {
		Name:          "nothing Known yet",
		Snapshot:      []domain.SnapshotIdentity{german("haus", "NOUN"), german("gehen", "VERB")},
		SnapshotCount: 2, EligibleCount: 2, GraduatedCount: 2, AlreadyKnownCount: 0,
		KnownAfter: []string{"gehen|VERB", "haus|NOUN"},
	}, {
		Name:       "empty snapshot",
		Known:      []domain.SnapshotIdentity{german("haus", "NOUN")},
		KnownAfter: []string{"haus|NOUN"},
	}, {
		Name:       "empty legacy reading without a snapshot",
		Legacy:     true,
		Known:      []domain.SnapshotIdentity{german("haus", "NOUN")},
		KnownAfter: []string{"haus|NOUN"},
	}, {
		Name:              "identities Known only to another learner",
		Snapshot:          []domain.SnapshotIdentity{german("haus", "NOUN"), german("gehen", "VERB")},
		OtherLearnerKnown: []domain.SnapshotIdentity{german("haus", ""), german("gehen", "VERB")},
		SnapshotCount:     2, EligibleCount: 2, GraduatedCount: 2, AlreadyKnownCount: 0,
		KnownAfter: []string{"gehen|VERB", "haus|NOUN"},
	}}
}

// finishScenario checks one finish case: a wrong Book is refused as stale, the
// right Book finishes with the expected counts and Known rows, and a replay
// returns the recorded completion.
func finishScenario(tc finishCase) Scenario {
	return Scenario{
		Name: "finish: " + tc.Name,
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID, reading := startReading(t, h, tc.Snapshot)
			h.Seeds().SeedKnownVocabulary(t, owner, tc.Known)
			h.Seeds().SeedKnownVocabulary(t, h.Seeds().OtherOwner(), tc.OtherLearnerKnown)
			expectedSnapshot := reading.SnapshotID
			if tc.Legacy {
				h.Seeds().ClearSnapshotID(t, owner, Language)
				expectedSnapshot = ""
			}

			_, err := h.Store().FinishCurrentReading(owner, Language, unknownBookID, expectedSnapshot)
			require.ErrorIs(t, err, domain.ErrCurrentReadingStale)

			result, err := h.Store().FinishCurrentReading(owner, Language, bookID, expectedSnapshot)
			require.NoError(t, err)
			assert.Equal(t, tc.SnapshotCount, result.Completion.SnapshotVocabularyCount)
			assert.Equal(t, tc.EligibleCount, result.Completion.EligibleVocabularyCount)
			assert.Equal(t, tc.GraduatedCount, result.Completion.GraduatedVocabularyCount)
			assert.Equal(t, tc.AlreadyKnownCount, result.Completion.AlreadyKnownVocabularyCount)
			known, err := h.Store().ListKnownVocabulary(owner, Language)
			require.NoError(t, err)
			assert.Equal(t, tc.KnownAfter, knownKeys(known))

			replay, err := h.Store().FinishCurrentReading(owner, Language, bookID, expectedSnapshot)
			require.NoError(t, err)
			assert.Equal(t, result, replay)
		},
	}
}

// knownKeys renders Known rows as the sorted "lemma|UPOS" keys finishCase uses.
func knownKeys(known []domain.KnownVocabulary) []string {
	keys := make([]string, 0, len(known))
	for _, item := range known {
		keys = append(keys, item.CanonicalLemma+"|"+item.UPOS)
	}
	sort.Strings(keys)
	return keys
}
