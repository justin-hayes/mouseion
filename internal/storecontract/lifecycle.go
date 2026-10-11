package storecontract

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unknownBookID is well-formed as a uuid but names no Book in any store.
const unknownBookID = "00000000-0000-0000-0000-000000000000"

func german(lemma, upos string) domain.SnapshotIdentity {
	return domain.SnapshotIdentity{Language: Language, CanonicalLemma: lemma, UPOS: upos}
}

// defaultVocabulary is a small analysis whose identities all freeze into a
// snapshot when nothing is Known.
var defaultVocabulary = []domain.SnapshotIdentity{german("haus", "NOUN"), german("gehen", "VERB")}

// startReading seeds an eligible To Read Book with vocabulary and starts it.
func startReading(t *testing.T, h Harness, vocabulary []domain.SnapshotIdentity) (string, domain.CurrentReading) {
	t.Helper()
	bookID := h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{Vocabulary: vocabulary, ToRead: true})
	reading, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
	require.NoError(t, err)
	return bookID, reading
}

func lifecycleScenarios() []Scenario {
	return []Scenario{{
		Name: "start accepts an analyzed To Read Book and freezes its vocabulary",
		Run: func(t *testing.T, h Harness) {
			bookID, reading := startReading(t, h, defaultVocabulary)
			assert.Equal(t, bookID, reading.BookID)
			assert.NotEmpty(t, reading.SnapshotID)
			assert.Equal(t, len(defaultVocabulary), reading.SnapshotSize)
			current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), Language)
			require.NoError(t, err)
			assert.Equal(t, reading.SnapshotID, current.SnapshotID)
			assert.Equal(t, bookID, current.BookID)
		},
	}, {
		Name: "start replays the current Book without a new snapshot",
		Run: func(t *testing.T, h Harness) {
			bookID, reading := startReading(t, h, defaultVocabulary)
			replay, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
			require.NoError(t, err)
			assert.Equal(t, reading.SnapshotID, replay.SnapshotID)
		},
	}, {
		Name: "start refuses a second Book while a reading is current",
		Run: func(t *testing.T, h Harness) {
			startReading(t, h, defaultVocabulary)
			second := h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{Vocabulary: defaultVocabulary, ToRead: true})
			_, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, second)
			require.ErrorIs(t, err, domain.ErrCurrentReadingExists)
		},
	}, {
		Name: "start refuses a Book that is not To Read",
		Run: func(t *testing.T, h Harness) {
			bookID := h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{Vocabulary: defaultVocabulary})
			_, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
			require.ErrorIs(t, err, domain.ErrCurrentReadingIneligible)
		},
	}, {
		Name: "start refuses an unknown Book as not found",
		Run: func(t *testing.T, h Harness) {
			_, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, unknownBookID)
			require.ErrorIs(t, err, domain.ErrNotFound)
		},
	}, {
		Name: "switch moves the current reading to another Book with a new snapshot",
		Run: func(t *testing.T, h Harness) {
			first, reading := startReading(t, h, defaultVocabulary)
			second := h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{Vocabulary: []domain.SnapshotIdentity{german("hund", "NOUN")}, ToRead: true})
			switched, err := h.Store().SwitchCurrentReading(h.Seeds().Owner(), Language, second, first, reading.SnapshotID)
			require.NoError(t, err)
			assert.Equal(t, second, switched.BookID)
			assert.NotEqual(t, reading.SnapshotID, switched.SnapshotID)
			current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), Language)
			require.NoError(t, err)
			assert.Equal(t, second, current.BookID)
		},
	}, {
		Name: "switch refuses a stale expected snapshot",
		Run: func(t *testing.T, h Harness) {
			first, _ := startReading(t, h, defaultVocabulary)
			second := h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{Vocabulary: []domain.SnapshotIdentity{german("hund", "NOUN")}, ToRead: true})
			_, err := h.Store().SwitchCurrentReading(h.Seeds().Owner(), Language, second, first, "stale-snapshot")
			require.ErrorIs(t, err, domain.ErrCurrentReadingStale)
		},
	}, {
		Name: "end clears the exact commitment",
		Run: func(t *testing.T, h Harness) {
			bookID, reading := startReading(t, h, defaultVocabulary)
			require.NoError(t, h.Store().EndCurrentReading(h.Seeds().Owner(), Language, bookID, reading.SnapshotID))
			current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), Language)
			require.NoError(t, err)
			assert.Empty(t, current.BookID)
		},
	}, {
		Name: "end refuses a stale commitment",
		Run: func(t *testing.T, h Harness) {
			bookID, _ := startReading(t, h, defaultVocabulary)
			err := h.Store().EndCurrentReading(h.Seeds().Owner(), Language, bookID, "stale-snapshot")
			require.ErrorIs(t, err, domain.ErrCurrentReadingStale)
		},
	}, {
		Name: "end replays the same commitment after the reading has ended",
		Run: func(t *testing.T, h Harness) {
			bookID, reading := startReading(t, h, defaultVocabulary)
			require.NoError(t, h.Store().EndCurrentReading(h.Seeds().Owner(), Language, bookID, reading.SnapshotID))
			require.NoError(t, h.Store().EndCurrentReading(h.Seeds().Owner(), Language, bookID, reading.SnapshotID))
		},
	}}
}
