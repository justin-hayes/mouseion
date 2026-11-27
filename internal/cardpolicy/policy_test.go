package cardpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGreekPolicyUsesSourceEvidence(t *testing.T) {
	for _, tag := range []string{"grc", "GRC", " grc "} {
		t.Run(tag, func(t *testing.T) {
			policy := For(tag)
			require.True(t, policy.SourceEvidence, "source-evidence policy")
			assert.Equal(t, GreekSourceNotice, policy.SourceNotice)
			assert.False(t, policy.NLPAnalysed, "imported analysis")
			assert.Equal(t, BoundaryTerminalMark, policy.SentenceBoundary)
			assert.Equal(t, LookupExactlyOne, policy.DictionaryLookup)
			assert.True(t, policy.EvidenceAssistedPass, "evidence-assisted pass allowed")
		})
	}
}

func TestGreekPolicyNamesItsTranslationsAndEntries(t *testing.T) {
	policy := For("grc")

	assert.Equal(t, TranslationPerseus, policy.Translations)
	assert.Equal(t, EntryLSJ, policy.Entries)
	assert.Equal(t, CitationsNone, policy.Citations)
}

func TestLatinPolicyUsesSourceEvidenceWithItsOwnProviderPass(t *testing.T) {
	for _, tag := range []string{"la", "LA", " la "} {
		t.Run(tag, func(t *testing.T) {
			policy := For(tag)
			require.True(t, policy.SourceEvidence, "source-evidence cards")
			assert.False(t, policy.NLPAnalysed, "no NLP notice")
			assert.True(t, policy.EvidenceAssistedPass, "the evidence-assisted pass is on; it is sent only once the la assessment is recorded")
			assert.Equal(t, LatinSourceNotice, policy.SourceNotice)
			assert.Equal(t, TranslationPerseus, policy.Translations)
			assert.Equal(t, BoundaryTerminalMark, policy.SentenceBoundary)
			assert.Equal(t, CitationsWorkAuthor, policy.Citations)
			assert.Equal(t, EntryLewisShort, policy.Entries, "Lewis & Short, never the Greek LSJ")
			assert.Equal(t, LookupEveryEntry, policy.DictionaryLookup, "Latin never reads the Greek index")
		})
	}
}

func TestModernLanguagesTakeTheProviderRoute(t *testing.T) {
	for _, tag := range []string{"de", "it", "el", "fr", "", "xx"} {
		t.Run(tag, func(t *testing.T) {
			policy := For(tag)
			assert.False(t, policy.SourceEvidence)
			assert.Empty(t, policy.SourceNotice)
			assert.False(t, policy.EvidenceAssistedPass)
			assert.True(t, policy.NLPAnalysed, "provider route")
			assert.Equal(t, BoundaryCapitalisedStart, policy.SentenceBoundary)
			assert.Equal(t, LookupStored, policy.DictionaryLookup)
		})
	}
}

func TestStudySwitchIsOnForModernLanguagesAndGreekAndOffForLatin(t *testing.T) {
	tests := []struct {
		language string
		study    bool
	}{
		{"de", true},
		{"it", true},
		{"grc", true}, // Greek Study is on after its evidence run (#2767)
		{"la", false}, // Latin Study is switched on only after its evidence run
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			require.Equal(t, tc.study, For(tc.language).Study)
		})
	}
}

func TestStudyOverrideForTestSwitchesOneLanguageAndRestores(t *testing.T) {
	restore := OverrideStudyForTest("la", true)
	assert.True(t, For("la").Study)
	assert.True(t, For("LA").Study, "the override follows the language tag's normal form")
	assert.True(t, For("grc").Study, "the override leaves the other languages as they are")
	restore()
	assert.False(t, For("la").Study)
}

func TestPassContractIsChosenByPolicy(t *testing.T) {
	tests := []struct {
		language string
		contract PassContract
	}{
		{"grc", PassContractGreek},
		{"la", PassContractLatin},
		{"LA", PassContractLatin},
		{"de", PassContractGreek}, // no pass runs for a modern language; the zero value is the Greek contract
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			require.Equal(t, tc.contract, For(tc.language).PassContract)
		})
	}
}

// TestGreekStudyLooksUpEveryEntryWhereTheDeckTakesOne proves the policy keeps
// the deck's lookup and pass contract and gives Study its own (ADR 0139).
func TestGreekStudyLooksUpEveryEntryWhereTheDeckTakesOne(t *testing.T) {
	policy := For("grc")
	assert.Equal(t, LookupExactlyOne, policy.DictionaryLookup, "the deck keeps one candidate")
	assert.Equal(t, PassContractGreek, policy.PassContract, "the deck keeps the existing Greek contract")
	assert.Equal(t, LookupEveryEntry, policy.StudyLookup)
	assert.Equal(t, PassContractGreekEntries, policy.StudyPassContract)
	assert.True(t, policy.Study, "Greek's Study switch is on after the evidence run (#2767)")
}

func TestLatinStudyNarrowsTheDeckLookupAndKeepsItsContract(t *testing.T) {
	policy := For("la")
	assert.Equal(t, LookupEveryEntry, policy.DictionaryLookup, "the deck keeps every entry")
	assert.Equal(t, LookupCompetingEntries, policy.StudyLookup)
	assert.Equal(t, policy.PassContract, policy.StudyPassContract)
}
