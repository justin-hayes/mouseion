package enrichment

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// evidenceAssistedPassRequest is a fully populated pass request, so every
// declared payload field is transmitted and can be checked against the recorded
// assessment.
func evidenceAssistedPassRequest() TranslationRequest {
	return TranslationRequest{
		Language: "grc", TargetLanguage: "en", CanonicalLemma: "λόγοσ", UPOS: "NOUN",
		TargetWord:                  "λόγον",
		ExampleSentence:             "ἄνθρωπος ὁρᾷ τὸν λόγον καὶ δέω βραδέως;",
		PassageContext:              "The man sees the word and runs slowly.",
		RequireEvidenceAssistedPass: true,
		CandidateSenses: []LexicalSense{
			{EvidenceID: "lsj-logos.0", Source: "lsj", Gloss: "word"},
			{EvidenceID: "lsj-logos.0.1", Source: "lsj", Gloss: "account"},
		},
	}
}

// TestEvidenceAssistedPassPayloadMatchesRecordedAssessment asserts the exact
// transmitted field set equals the recorded provider-payload assessment's
// field set.
func TestEvidenceAssistedPassPayloadMatchesRecordedAssessment(t *testing.T) {
	codec := passCodec(t)
	body, err := codec.EncodeRequest(evidenceAssistedPassRequest())
	require.NoError(t, err)
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	require.Len(t, request.Messages, 2)
	assert.Contains(t, request.Messages[0].Content, "candidate_senses are the pinned LSJ senses")

	var transmitted map[string]any
	require.NoError(t, json.Unmarshal([]byte(request.Messages[1].Content), &transmitted))
	keys := make([]string, 0, len(transmitted))
	for key := range transmitted {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	recorded := ProviderAssessments()[0]
	expected := append([]string(nil), recorded.Fields...)
	sort.Strings(expected)
	assert.Equal(t, expected, keys, "the transmitted field set must equal the recorded assessment's field set")
	assert.Equal(t, EvidenceAssistedPassPayloadFields(), recorded.Fields, "the codec's declared payload fields must match the recorded assessment")

	assert.Equal(t, ContextualGlossItemHandle, transmitted["item_id"], "the pass transmits the short per-request handle, never the stable item ID")
	assert.Equal(t, "The man sees the word and runs slowly.", transmitted["passage_context"])
	senses, ok := transmitted["candidate_senses"].([]any)
	require.True(t, ok)
	require.Len(t, senses, 2)
	first, ok := senses[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "e1", first["evidence_id"], "native LSJ sense IDs are transmitted as short handles")
}

// TestEvidenceAssistedPassDecodesResolvedAndAbstained proves the pass accepts a
// resolved contextual Gloss with its sentence translation and alignment, and an
// explicit abstention.
func TestEvidenceAssistedPassDecodesResolvedAndAbstained(t *testing.T) {
	codec := passCodec(t)
	input := evidenceAssistedPassRequest()

	resolved, err := codec.DecodeResponse(input, passChatBody(t, `{"item_id":"`+ContextualGlossItemHandle+`","source_language":"grc","target_language":"en","translation":"word","sentence_translation":"The man sees the word and runs slowly.","sentence_translation_targets":["word"],"gloss":"word","evidence_ids":["e1"],"context_only":false}`))
	require.NoError(t, err)
	assert.Equal(t, "word", resolved.Gloss)
	assert.Equal(t, []string{"lsj-logos.0"}, resolved.EvidenceIDs, "an echoed handle maps back to the native LSJ sense ID")
	assert.Equal(t, []string{"word"}, resolved.SentenceTranslationTargets)

	abstained, err := codec.DecodeResponse(input, passChatBody(t, `{"item_id":"`+ContextualGlossItemHandle+`","source_language":"grc","target_language":"en","translation":"word","sentence_translation":"The man sees the word and runs slowly.","sentence_translation_targets":[],"gloss":"","evidence_ids":[],"context_only":true,"unresolved_reason":"the passage does not decide between the senses"}`))
	require.NoError(t, err)
	assert.Empty(t, abstained.Gloss)
	assert.True(t, abstained.ContextOnly)
	assert.NotEmpty(t, abstained.UnresolvedReason)
}

// TestEvidenceAssistedPassRejectsUnknownEvidence proves the pass cannot cite a
// sense it was never given.
func TestEvidenceAssistedPassRejectsUnknownEvidence(t *testing.T) {
	codec := passCodec(t)
	_, err := codec.DecodeResponse(evidenceAssistedPassRequest(), passChatBody(t, `{"item_id":"`+ContextualGlossItemHandle+`","source_language":"grc","target_language":"en","translation":"word","sentence_translation":"The man sees the word.","sentence_translation_targets":["word"],"gloss":"word","evidence_ids":["e9"],"context_only":false}`))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidTranslationResponse)
	assert.Contains(t, err.Error(), "unknown evidence ID")
}

func passChatBody(t *testing.T, content string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"content": content}}}})
	require.NoError(t, err)
	return body
}
