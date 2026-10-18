package storecontract

import (
	"sort"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gatedIdentities is one modern-language Book's analysis: haus appears in a
// sentence the sentence-quality gate accepts, while garten's only sentences are
// fragments the gate rejects.
func gatedIdentities() (usable, unusable domain.SnapshotIdentity) {
	return german("haus", "NOUN"), german("garten", "NOUN")
}

// seedGatedToRead seeds a To Read Book with a usable identity and one the seed
// declares unusable, so a gated Start must freeze only the usable one.
func seedGatedToRead(t *testing.T, h Harness, usable, unusable domain.SnapshotIdentity) string {
	t.Helper()
	return h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{
		Vocabulary: []domain.SnapshotIdentity{usable, unusable},
		Unusable:   []domain.SnapshotIdentity{unusable},
		ToRead:     true,
	})
}

// reservedLemmas renders the owner's active reserved vocabulary in a language as
// sorted "lemma|UPOS" keys.
func reservedLemmas(t *testing.T, h Harness, language string) []string {
	t.Helper()
	reserved, err := h.Store().ListReservedVocabulary(h.Seeds().Owner(), language)
	require.NoError(t, err)
	keys := make([]string, 0, len(reserved))
	for _, item := range reserved {
		keys = append(keys, item.CanonicalLemma+"|"+item.UPOS)
	}
	sort.Strings(keys)
	return keys
}

// knownLemmas renders the owner's Known vocabulary in a language as sorted
// "lemma|UPOS" keys.
func knownLemmas(t *testing.T, h Harness, language string) []string {
	t.Helper()
	known, err := h.Store().ListKnownVocabulary(h.Seeds().Owner(), language)
	require.NoError(t, err)
	keys := make([]string, 0, len(known))
	for _, item := range known {
		keys = append(keys, item.CanonicalLemma+"|"+item.UPOS)
	}
	sort.Strings(keys)
	return keys
}

// gateScenarios pin ADR 0106: a Start, Switch or Finish treats an identity
// without a gate-passing sentence exactly as if the Book had not offered it.
func gateScenarios() []Scenario {
	return []Scenario{{
		Name: "start freezes only identities with a gate-passing sentence",
		Run: func(t *testing.T, h Harness) {
			usable, unusable := gatedIdentities()
			bookID := seedGatedToRead(t, h, usable, unusable)
			reading, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
			require.NoError(t, err)
			assert.Equal(t, 1, reading.SnapshotSize, "only the usable identity is frozen")
			assert.Equal(t, []string{"haus|NOUN"}, reservedLemmas(t, h, Language))
			assert.NotContains(t, reservedLemmas(t, h, Language), "garten|NOUN")
		},
	}, {
		Name: "switch applies the same gate as start",
		Run: func(t *testing.T, h Harness) {
			first, reading := startReading(t, h, defaultVocabulary)
			usable, unusable := gatedIdentities()
			second := seedGatedToRead(t, h, usable, unusable)
			switched, err := h.Store().SwitchCurrentReading(h.Seeds().Owner(), Language, second, first, reading.SnapshotID)
			require.NoError(t, err)
			assert.Equal(t, 1, switched.SnapshotSize, "a Switch reserves no identity a Start would not")
			assert.Equal(t, []string{"haus|NOUN"}, reservedLemmas(t, h, Language))
		},
	}, {
		Name: "finish makes only frozen identities Known and leaves gated-out identities Unknown",
		Run: func(t *testing.T, h Harness) {
			usable, unusable := gatedIdentities()
			bookID := seedGatedToRead(t, h, usable, unusable)
			reading, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
			require.NoError(t, err)
			result, err := h.Store().FinishCurrentReading(h.Seeds().Owner(), Language, bookID, reading.SnapshotID)
			require.NoError(t, err)
			assert.Equal(t, 1, result.Completion.SnapshotVocabularyCount)
			assert.Equal(t, 1, result.Completion.EligibleVocabularyCount)
			assert.Equal(t, 1, result.Completion.GraduatedVocabularyCount)
			assert.Equal(t, []string{"haus|NOUN"}, knownLemmas(t, h, Language), "the gated-out identity stays Unknown for a later Book")

			// The gated-out identity is still Unknown, so a later Book with a
			// usable sentence can freeze it.
			later := h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{
				Vocabulary: []domain.SnapshotIdentity{unusable},
				ToRead:     true,
			})
			laterReading, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, later)
			require.NoError(t, err)
			assert.Equal(t, 1, laterReading.SnapshotSize, "a later Book can select the word the gate kept out")
			assert.Equal(t, []string{"garten|NOUN"}, reservedLemmas(t, h, Language))
		},
	}, {
		Name: "a Book whose only frequent identity is unusable freezes an empty snapshot that finishes cleanly",
		Run: func(t *testing.T, h Harness) {
			unusable := german("garten", "NOUN")
			bookID := h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{
				Vocabulary: []domain.SnapshotIdentity{unusable},
				Unusable:   []domain.SnapshotIdentity{unusable},
				ToRead:     true,
			})
			reading, err := h.Store().StartCurrentReading(h.Seeds().Owner(), Language, bookID)
			require.NoError(t, err)
			assert.Zero(t, reading.SnapshotSize)
			assert.Empty(t, reservedLemmas(t, h, Language))

			result, err := h.Store().FinishCurrentReading(h.Seeds().Owner(), Language, bookID, reading.SnapshotID)
			require.NoError(t, err)
			assert.Zero(t, result.Completion.SnapshotVocabularyCount)
			assert.Zero(t, result.Completion.EligibleVocabularyCount)
			assert.Zero(t, result.Completion.GraduatedVocabularyCount)
			assert.Empty(t, knownLemmas(t, h, Language))
		},
	}, {
		Name: "grc start freezes only identities with a gate-passing sentence",
		Run: func(t *testing.T, h Harness) {
			usable := greekIdentity("λέγω", "VERB", "", "λέγω")
			unusable := greekIdentity("λόγος", "NOUN", "", "λόγος")
			bookID := h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{
				Language: GreekLanguage, Imported: true,
				Vocabulary: []domain.SnapshotIdentity{usable, unusable},
				Unusable:   []domain.SnapshotIdentity{unusable},
				ToRead:     true,
			})
			reading, err := h.Store().StartCurrentReading(h.Seeds().Owner(), GreekLanguage, bookID)
			require.NoError(t, err)
			assert.Equal(t, 1, reading.SnapshotSize, "grc follows the same gate as a modern language")
			assert.Equal(t, []string{"λέγω|VERB"}, reservedLemmas(t, h, GreekLanguage))
		},
	}}
}
