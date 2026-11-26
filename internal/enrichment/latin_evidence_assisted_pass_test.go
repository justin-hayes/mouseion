package enrichment

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// latinPassRequest is a Latin pass request over two stacked homograph entries
// (dīco¹, dīco²) whose senses are cited by entry-carrying evidence IDs.
func latinPassRequest() TranslationRequest {
	return TranslationRequest{
		Language: "la", TargetLanguage: "en", CanonicalLemma: "dico", UPOS: "VERB",
		TargetWord:                  "dicit",
		ExampleSentence:             "Caesar dicit.",
		PassageContext:              "Caesar speaks.",
		RequireEvidenceAssistedPass: true,
		CandidateSenses: []LexicalSense{
			{EvidenceID: LatinEvidenceID("n1", "n1.0"), Source: "ls", Gloss: "to dedicate"},
			{EvidenceID: LatinEvidenceID("n1", "n1.1"), Source: "ls", Gloss: "to devote"},
			{EvidenceID: LatinEvidenceID("n2", "n2.0"), Source: "ls", Gloss: "to say"},
		},
	}
}

func TestLatinEvidenceIDRoundTrips(t *testing.T) {
	entry, sense, ok := SplitLatinEvidenceID(LatinEvidenceID("n1", "n1.0"))
	require.True(t, ok)
	assert.Equal(t, "n1", entry)
	assert.Equal(t, "n1.0", sense)
	_, _, ok = SplitLatinEvidenceID("n1.0")
	assert.False(t, ok, "a bare sense ID names no entry")
}

func TestLatinPassNamesLewisShortAndCarriesEntries(t *testing.T) {
	codec := passCodec(t)
	body, err := codec.EncodeRequest(latinPassRequest())
	require.NoError(t, err)
	var request struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	assert.Contains(t, request.Messages[0].Content, "Lewis & Short")
	assert.NotContains(t, request.Messages[0].Content, "LSJ")

	var transmitted struct {
		Language string `json:"language"`
		Senses   []struct {
			EvidenceID string `json:"evidence_id"`
			Entry      string `json:"entry"`
		} `json:"candidate_senses"`
	}
	require.NoError(t, json.Unmarshal([]byte(request.Messages[1].Content), &transmitted))
	require.Len(t, transmitted.Senses, 3)
	assert.Equal(t, "e1", transmitted.Senses[0].EvidenceID)
	assert.Equal(t, "e3", transmitted.Senses[2].EvidenceID)
	assert.Equal(t, transmitted.Senses[0].Entry, transmitted.Senses[1].Entry)
	assert.NotEmpty(t, transmitted.Senses[0].Entry)
	assert.NotEqual(t, transmitted.Senses[0].Entry, transmitted.Senses[2].Entry, "stacked homographs carry distinct entry handles")
	assert.NotContains(t, request.Messages[1].Content, "n1", "no native entry or sense ID is transmitted")
}

func TestLatinPassFieldsMatchItsOwnContract(t *testing.T) {
	codec := passCodec(t)
	assert.NotEqual(t, codec.EvidenceAssistedPassPromptContract(), codec.LatinEvidenceAssistedPassPromptContract())
	assert.Contains(t, codec.LatinEvidenceAssistedPassProviderVersion(), codec.LatinEvidenceAssistedPassPromptContract())
	assert.NotEmpty(t, LatinEvidenceAssistedPassPayloadFields())
	assert.Equal(t, "translation-v16-evidence-assisted-greek", codec.EvidenceAssistedPassPromptContract(), "Greek's contract is unchanged")
}

func TestLatinPassDecodesACitedHandleToOneEntryAndSense(t *testing.T) {
	codec := passCodec(t)
	input := latinPassRequest()
	resolved, err := codec.DecodeResponse(input, passChatBody(t, `{"item_id":"`+ContextualGlossItemHandle+`","source_language":"la","target_language":"en","translation":"say","sentence_translation":"Caesar speaks.","sentence_translation_targets":[],"gloss":"say","evidence_ids":["e3"],"context_only":false}`))
	require.NoError(t, err)
	require.Len(t, resolved.EvidenceIDs, 1)
	entry, sense, ok := SplitLatinEvidenceID(resolved.EvidenceIDs[0])
	require.True(t, ok)
	assert.Equal(t, "n2", entry)
	assert.Equal(t, "n2.0", sense)
}

func TestLatinPassRejectsUnknownAndRepeatedHandles(t *testing.T) {
	codec := passCodec(t)
	input := latinPassRequest()
	for _, ids := range []string{`["e9"]`, `["e1","e1"]`} {
		_, err := codec.DecodeResponse(input, passChatBody(t, `{"item_id":"`+ContextualGlossItemHandle+`","source_language":"la","target_language":"en","translation":"say","sentence_translation":"Caesar speaks.","sentence_translation_targets":[],"gloss":"say","evidence_ids":`+ids+`,"context_only":false}`))
		require.Error(t, err, ids)
	}
}

func TestLatinPassIsRefusedUntilRecorded(t *testing.T) {
	codec := passCodec(t)
	require.ErrorIs(t, ProviderAssessments().MaySend(LatinEvidenceAssistedPassPayload, "la", codec), ErrProviderAssessmentNotRecorded)
	for _, assessment := range ProviderAssessments() {
		assert.NotEqual(t, LatinEvidenceAssistedPassPayload, assessment.Kind, "the la record is a human act")
	}
	recorded := ProviderAssessmentSet{{
		Kind: LatinEvidenceAssistedPassPayload, Language: "la", Provider: codec.ProviderName(),
		PromptVersion: codec.LatinEvidenceAssistedPassPromptContract(), Fields: LatinEvidenceAssistedPassPayloadFields(),
		Ticket: "#test", Recorded: true,
	}}
	require.NoError(t, recorded.MaySend(LatinEvidenceAssistedPassPayload, "la", codec))
	require.Error(t, recorded.MaySend(EvidenceAssistedPassPayload, "la", codec), "a Latin record never answers for the Greek payload")
}

func TestEvidenceAssistedPassKindFollowsLanguage(t *testing.T) {
	assert.Equal(t, LatinEvidenceAssistedPassPayload, EvidenceAssistedPassKind("la"))
	assert.Equal(t, EvidenceAssistedPassPayload, EvidenceAssistedPassKind("grc"))
}
