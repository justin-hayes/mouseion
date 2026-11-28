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

// defaultVocabulary is a small analysis a seeded Book carries.
var defaultVocabulary = []domain.SnapshotIdentity{german("haus", "NOUN"), german("gehen", "VERB")}

// otherVocabulary is the analysis of a second Book.
var otherVocabulary = []domain.SnapshotIdentity{german("hund", "NOUN")}

// seedToRead seeds an analyzed To Read Book for the learner under test.
func seedToRead(t *testing.T, h Harness, vocabulary []domain.SnapshotIdentity) string {
	t.Helper()
	return h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{Vocabulary: vocabulary})
}

// startReading seeds an eligible To Read Book with vocabulary and reads it next.
func startReading(t *testing.T, h Harness, vocabulary []domain.SnapshotIdentity) (string, domain.CurrentReading) {
	t.Helper()
	bookID := seedToRead(t, h, vocabulary)
	reading, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
	require.NoError(t, err)
	return bookID, reading
}

// requireCurrent asserts that the learner's current reading is bookID.
func requireCurrent(t *testing.T, h Harness, bookID string) {
	t.Helper()
	current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), Language)
	require.NoError(t, err)
	assert.Equal(t, bookID, current.BookID)
}

func lifecycleScenarios() []Scenario {
	scenarios := readNextScenarios()
	scenarios = append(scenarios, endScenarios()...)
	return append(scenarios, finishScenarios()...)
}

// readNextScenarios pin Read next (ADR 0136 §4): it replaces Start and Switch,
// needs no commitment from the caller, and is refused only for a Book that is
// not To Read, not analysed, or has no Book language.
func readNextScenarios() []Scenario {
	return []Scenario{{
		Name: "read next makes an analyzed To Read Book the current reading",
		Run: func(t *testing.T, h Harness) {
			bookID, reading := startReading(t, h, defaultVocabulary)
			assert.Equal(t, bookID, reading.BookID)
			requireCurrent(t, h, bookID)
		},
	}, {
		Name: "read next replays the current Book",
		Run: func(t *testing.T, h Harness) {
			bookID, _ := startReading(t, h, defaultVocabulary)
			replay, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
			require.NoError(t, err)
			assert.Equal(t, bookID, replay.BookID)
			requireCurrent(t, h, bookID)
		},
	}, {
		Name: "read next replaces the current reading with another Book",
		Run: func(t *testing.T, h Harness) {
			startReading(t, h, defaultVocabulary)
			second := seedToRead(t, h, otherVocabulary)
			switched, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, second)
			require.NoError(t, err)
			assert.Equal(t, second, switched.BookID)
			requireCurrent(t, h, second)
		},
	}, {
		Name: "read next refuses a Book that is not To Read",
		Run: func(t *testing.T, h Harness) {
			bookID := h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{Inbox: true, Vocabulary: defaultVocabulary})
			_, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
			var ineligible domain.CurrentReadingIneligibleError
			require.ErrorAs(t, err, &ineligible)
			require.ErrorIs(t, err, domain.ErrCurrentReadingIneligible)
			assert.Equal(t, domain.CurrentReadingNotToRead, ineligible.Reason)
		},
	}, {
		Name: "read next refuses a Book of another language",
		Run: func(t *testing.T, h Harness) {
			bookID := seedGreekToRead(t, h, greekVocabulary)
			_, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
			require.ErrorIs(t, err, domain.ErrCurrentReadingIneligible)
			current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), Language)
			require.NoError(t, err)
			assert.Empty(t, current.BookID, "a refused Read next changes nothing")
		},
	}, {
		Name: "a refused read next leaves the current reading unchanged",
		Run: func(t *testing.T, h Harness) {
			bookID, _ := startReading(t, h, defaultVocabulary)
			inbox := h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{Inbox: true, Vocabulary: otherVocabulary})
			_, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, inbox)
			require.ErrorIs(t, err, domain.ErrCurrentReadingIneligible)
			requireCurrent(t, h, bookID)
		},
	}, {
		Name: "read next refuses an unknown Book as not found",
		Run: func(t *testing.T, h Harness) {
			_, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, unknownBookID)
			require.ErrorIs(t, err, domain.ErrNotFound)
		},
	}}
}

func endScenarios() []Scenario {
	return []Scenario{{
		Name: "end clears the current reading and keeps the Book To Read",
		Run: func(t *testing.T, h Harness) {
			bookID, _ := startReading(t, h, defaultVocabulary)
			require.NoError(t, h.Store().EndCurrentReading(h.Seeds().Owner(), Language))
			current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), Language)
			require.NoError(t, err)
			assert.Empty(t, current.BookID)
			detail := requireDetail(t, h, h.Seeds().Owner(), bookID)
			assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
			assert.Zero(t, detail.CompletionCount, "End records no completion")
		},
	}, {
		Name: "end with no current reading is not found",
		Run: func(t *testing.T, h Harness) {
			require.ErrorIs(t, h.Store().EndCurrentReading(h.Seeds().Owner(), Language), domain.ErrNotFound)
		},
	}, {
		Name: "end clears only its own language",
		Run: func(t *testing.T, h Harness) {
			startReading(t, h, defaultVocabulary)
			greekBook, _ := startGreekReading(t, h, greekVocabulary)
			require.NoError(t, h.Store().EndCurrentReading(h.Seeds().Owner(), Language))
			current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), GreekLanguage)
			require.NoError(t, err)
			assert.Equal(t, greekBook, current.BookID)
		},
	}}
}

// finishScenarios pin Finish (ADR 0136 §4): it appends a Reading completion and
// clears the role and To Read, with no vocabulary effect and no commitment.
func finishScenarios() []Scenario {
	return []Scenario{{
		Name: "finish appends a completion and clears the role and To Read",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID, _ := startReading(t, h, defaultVocabulary)
			result, err := h.Store().FinishCurrentReading(owner, Language)
			require.NoError(t, err)
			assert.Equal(t, bookID, result.Completion.BookID)
			assert.Equal(t, Language, result.Completion.Language)
			assert.False(t, result.Completion.CompletedAt.IsZero())

			current, err := h.Store().GetCurrentReading(owner, Language)
			require.NoError(t, err)
			assert.Empty(t, current.BookID)
			detail := requireDetail(t, h, owner, bookID)
			assert.Equal(t, domain.BookDispositionInbox, detail.Disposition, "Finish clears To Read")
			assert.Equal(t, 1, detail.CompletionCount)
		},
	}, {
		Name: "finish with no current reading is not found",
		Run: func(t *testing.T, h Harness) {
			_, err := h.Store().FinishCurrentReading(h.Seeds().Owner(), Language)
			require.ErrorIs(t, err, domain.ErrNotFound)
		},
	}, {
		Name: "finishing a Book again appends a second completion",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID, _ := startReading(t, h, defaultVocabulary)
			_, err := h.Store().FinishCurrentReading(owner, Language)
			require.NoError(t, err)
			require.NoError(t, h.BookState().SetBookDisposition(owner, bookID, domain.BookDispositionToRead))
			_, err = h.Store().StartCurrentReading(owner, Language, bookID)
			require.NoError(t, err)
			_, err = h.Store().FinishCurrentReading(owner, Language)
			require.NoError(t, err)
			assert.Equal(t, 2, requireDetail(t, h, owner, bookID).CompletionCount)
		},
	}}
}
