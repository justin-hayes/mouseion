package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcordanceTermsUsesDedicatedProfileForGreek proves the Concordance lemma
// key never uses the language-neutral casefold profile for grc: iota subscript
// and its adscript spelling keep distinct keys, and final sigma folds to medial
// sigma, exactly as ancient-greek/1 defines.
func TestConcordanceTermsUsesDedicatedProfileForGreek(t *testing.T) {
	subscript, _, err := concordanceTerms("grc", "\u1FC4")
	require.NoError(t, err)
	adscript, _, err := concordanceTerms("grc", "\u03AE\u03B9")
	require.NoError(t, err)
	assert.NotEqual(t, adscript, subscript, "iota subscript and adscript give different keys")
	assert.NotEqual(t, canonicalization.LanguageNeutral().Canonical("\u1FC4"), subscript, "grc never casefolds")

	finalSigma, _, err := concordanceTerms("grc", "\u03BB\u03AD\u03BE\u03B9\u03C2")
	require.NoError(t, err)
	assert.Equal(t, "\u03BB\u03AD\u03BE\u03B9\u03C3", finalSigma)

	// The form key is the same profile key that is stored on the token, so form
	// search never depends on SQL case folding for grc.
	lemma, form, err := concordanceTerms("grc", "\u03BB\u03CC\u03B3\u03BF\u03C2")
	require.NoError(t, err)
	assert.Equal(t, "\u03BB\u03CC\u03B3\u03BF\u03C3", lemma)
	assert.Equal(t, lemma, form, "the grc form key is the profile key")
}

func TestConcordanceTermsFallsBackForUnregisteredLanguage(t *testing.T) {
	lemma, form, err := concordanceTerms("it", "CITTÀ")
	require.NoError(t, err)
	assert.Equal(t, "città", lemma)
	assert.Equal(t, "città", form)
}

// TestSpeakerLabelOverlapIgnoresInvalidOffsets keeps a malformed stored offset
// from failing or restricting a whole Concordance page or Study read.
func TestSpeakerLabelOverlapIgnoresInvalidOffsets(t *testing.T) {
	structure := domain.SnapshotStructure{Recorded: true, ConventionSet: domain.StructureConventionSet, RoleSpans: []domain.UnitRoleSpan{
		{UnitID: "unit", Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 8},
	}}
	assert.False(t, speakerLabelOverlap(structure, "unit", -1, 4))
	assert.False(t, speakerLabelOverlap(structure, "unit", 0, -1))
	assert.True(t, speakerLabelOverlap(structure, "unit", 2, 4))
}
