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

func TestLatinPolicyUsesSourceEvidenceWithNoProviderPass(t *testing.T) {
	for _, tag := range []string{"la", "LA", " la "} {
		t.Run(tag, func(t *testing.T) {
			policy := For(tag)
			require.True(t, policy.SourceEvidence, "source-evidence cards")
			assert.False(t, policy.NLPAnalysed, "no NLP notice")
			assert.False(t, policy.EvidenceAssistedPass, "no evidence-assisted pass")
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
