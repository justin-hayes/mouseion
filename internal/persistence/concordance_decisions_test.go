package persistence

import (
	"testing"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
)

// TestAttributeSentenceStudyDecisionsAttributesOnlyUniqueWords holds the gate
// itself, without a database: a decision reaches a word only when it addresses
// exactly one persisted analyzed word and carries intact evidence. A shared
// span, a missing word, and a correction with no proposed lemma all stay
// unresolved and change no word.
func TestAttributeSentenceStudyDecisionsAttributesOnlyUniqueWords(t *testing.T) {
	study := &domain.SentenceStudy{Tokens: []domain.SentenceStudyToken{
		{Surface: "Haus", RawLemma: "haus", EffectiveLemma: "haus", StartOffset: 4, EndOffset: 8, Morphology: map[string]string{"Case": "Dat"}},
		{Surface: "zu", RawLemma: "zu", EffectiveLemma: "zu", StartOffset: 0, EndOffset: 3},
		{Surface: "dem", RawLemma: "der", EffectiveLemma: "der", StartOffset: 0, EndOffset: 3},
		{Surface: "Weg", RawLemma: "weg", EffectiveLemma: "weg", StartOffset: 9, EndOffset: 12},
		{Surface: "steht", RawLemma: "stehen", EffectiveLemma: "stehen", StartOffset: 13, EndOffset: 16, Morphology: map[string]string{"Mood": "Ind"}},
	}}
	attributeSentenceStudyDecisions(study, []sqlcgen.GetSentenceStudyDecisionsRow{
		{StartOffset: 4, EndOffset: 8, CanonicalLemma: "heim", AddressedWords: 1},
		{StartOffset: 0, EndOffset: 3, CanonicalLemma: "zu", AddressedWords: 2},
		{StartOffset: 9, EndOffset: 12, CanonicalLemma: "", AddressedWords: 1},
		{StartOffset: 13, EndOffset: 16, Excluded: true, AddressedWords: 1},
	})

	t.Run("a unique correction is attributed", func(t *testing.T) {
		require.True(t, study.Tokens[0].Corrected)
		require.Equal(t, "heim", study.Tokens[0].EffectiveLemma)
		require.Equal(t, map[string]string{"Case": "Dat"}, study.Tokens[0].Morphology,
			"attribution leaves grammar evidence untouched")
	})

	t.Run("a shared span is unresolved and applied to no sibling", func(t *testing.T) {
		require.False(t, study.Tokens[1].Corrected)
		require.False(t, study.Tokens[2].Corrected)
		require.Equal(t, "zu", study.Tokens[1].EffectiveLemma)
		require.Equal(t, "der", study.Tokens[2].EffectiveLemma)
	})

	t.Run("a correction with no proposed lemma is unresolved", func(t *testing.T) {
		require.False(t, study.Tokens[3].Corrected, "missing evidence never attributes a correction")
		require.Equal(t, "weg", study.Tokens[3].EffectiveLemma)
	})

	t.Run("a unique exclusion is attributed and keeps grammar evidence", func(t *testing.T) {
		require.True(t, study.Tokens[4].Excluded)
		require.False(t, study.Tokens[4].Corrected)
		require.Equal(t, "stehen", study.Tokens[4].EffectiveLemma)
		require.Equal(t, map[string]string{"Mood": "Ind"}, study.Tokens[4].Morphology)
	})

	t.Run("unresolved decisions are reported with their addressed count", func(t *testing.T) {
		require.Len(t, study.UnresolvedDecisions, 2)
		require.Equal(t, int64(0), study.UnresolvedDecisions[0].StartOffset)
		require.Equal(t, int64(2), study.UnresolvedDecisions[0].AddressedWords)
		require.Equal(t, int64(9), study.UnresolvedDecisions[1].StartOffset)
		require.Equal(t, int64(1), study.UnresolvedDecisions[1].AddressedWords)
	})
}
