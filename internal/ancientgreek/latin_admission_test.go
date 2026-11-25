package ancientgreek

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// latinAdmissionSentences is one sentence of synthetic tokens whose lemmas
// exercise admission: an artefact digit, a praenomen abbreviation, a
// Greek-script lemma, an enclitic, and cum as both a preposition and a
// conjunction.
func latinAdmissionSentences() [][]olaToken {
	return [][]olaToken{{
		{"M.", "M.", "n", "Case=n|Gender=m|Number=s", "SBJ", "1.1", 6},
		{"Videt", "video", "v", "Mood=i|Number=s|Person=3|Tense=p|Voice=a", "PRED", "1.1", 0},
		{"et", "et1", "c", "_", "COORD", "1.1", 2},
		{"λόγον", "λόγος", "n", "Case=a|Gender=m|Number=s", "OBJ", "1.1", 2},
		{"rex", "rex", "n", "Case=n|Gender=m|Number=s", "SBJ", "1.1", 2},
		{"cum", "cum", "r", "_", "AuxP", "1.1", 2},
		{"duce", "dux", "n", "Case=b|Gender=m|Number=s", "ADV", "1.1", 6},
		{"cum", "cum", "c", "_", "AuxC", "1.1", 2},
		{"gladio", "gladius", "n", "Case=b|Gender=m|Number=s", "ADV", "1.1", 2},
		{"que", "que", "c", "_", "COORD", "1.1", 2},
		{"uideo", "uideo", "v", "Mood=i|Number=s|Person=1|Tense=p|Voice=a", "PRED", "1.1", 0},
		{"Video", "video", "v", "Mood=i|Number=s|Person=1|Tense=p|Voice=a", "PRED", "1.1", 0},
	}}
}

func TestLatinAdmissionKeepsTokensButAdmitsOnlyLatinWords(t *testing.T) {
	conllu, paula := olaFixture(latinAdmissionSentences())
	parsed, err := ParseOlaSource(conllu, paula)
	require.NoError(t, err)
	definition := latinDefinition("1.1")
	definition.CitationEnd = "1.1"
	unit, err := parsed.CutUnit(definition)
	require.NoError(t, err)

	assert.Equal(t, "latin", unit.Result.NormalizationProfile.Name)
	tokens := unit.Result.Sentences[0].Tokens
	require.Len(t, tokens, 12, "every token stays in the text")
	assert.Equal(t, "et", tokens[2].CanonicalLemma, "the artefact digit is stripped")
	assert.Equal(t, "et", tokens[2].Headword, "the citation headword is the source lemma without the digit")
	assert.Empty(t, tokens[2].LexicalDiscriminator)
	assert.Equal(t, "uideo", tokens[11].CanonicalLemma)
	assert.Equal(t, "video", tokens[11].Headword, "the headword stays as the source gave it")

	everything := selection.DefaultConfig("corpus")
	everything.AllowedPOS = map[string]bool{"NOUN": true, "VERB": true, "ADP": true, "CCONJ": true, "SCONJ": true, "PROPN": true, "ADV": true}
	candidates, err := selection.Project(unit.Result, everything, nil)
	require.NoError(t, err)
	identities := map[string]int{}
	for _, candidate := range candidates {
		identity := candidate.Identity
		assert.Empty(t, identity.LexicalDiscriminator)
		identities[identity.CanonicalLemma+"|"+identity.UPOS] += candidate.OccurrenceCount
	}
	assert.NotContains(t, identities, "m.|NOUN", "an abbreviation is never an identity")
	assert.NotContains(t, identities, "m.|PROPN")
	for identity := range identities {
		assert.NotContains(t, identity, "λόγος", "a Greek-script lemma is never an identity")
	}
	assert.Equal(t, 3, identities["uideo|VERB"], "video, uideo and Video share one identity")
	assert.Equal(t, 1, identities["et|CCONJ"])
	assert.Equal(t, 1, identities["cum|ADP"], "cum as a preposition")
	assert.Equal(t, 1, identities["cum|SCONJ"]+identities["cum|CCONJ"], "cum as a conjunction is a separate identity")
	assert.Contains(t, identities, "rex|NOUN")

	defaults, err := selection.Project(unit.Result, selection.DefaultConfig("corpus"), nil)
	require.NoError(t, err)
	for _, candidate := range defaults {
		assert.NotEqual(t, "que", candidate.Identity.CanonicalLemma, "an enclitic follows the part-of-speech rule")
	}
}
