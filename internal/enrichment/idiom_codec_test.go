package enrichment

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func idiomCardRequest() TranslationRequest {
	return TranslationRequest{
		Language: "it", TargetLanguage: "en", CanonicalLemma: "in gamba", UPOS: "IDIOM",
		TargetWord: "gamba", ExampleSentence: "È davvero un ragazzo in gamba.",
		RequireIdiomCard: true, IdiomHeadword: "in gamba", IdiomMatchedForms: []string{"in", "gamba"},
		CandidateSenses: []LexicalSense{{EvidenceID: "wikt:capable", Gloss: "capable, on the ball"}},
	}
}

func TestIdiomCardCodecEncodesOnlyTheIdiomPayload(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	body, err := codec.EncodeRequest(idiomCardRequest())
	require.NoError(t, err)
	assert.Contains(t, string(body), "Confirm and translate the supplied idiom", "request body carries the idiom prompt")
	assert.Contains(t, string(body), `\"item_id\":\"`+IdiomCardItemHandle+`\"`, "request body=%s", body)
	assert.Contains(t, string(body), `\"headword\":\"in gamba\"`, "request body=%s", body)
	assert.Contains(t, string(body), `\"matched_forms\":[\"in\",\"gamba\"]`, "request body=%s", body)
	for _, forbidden := range []string{"canonical_lemma", "passage_context", "book_title", "owner_id", "learner"} {
		assert.NotContains(t, string(body), forbidden, "idiom payload leaked %q", forbidden)
	}
	assert.Equal(t, idiomCardPromptVersion, codec.IdiomCardPromptContract())
	assert.Equal(t, "model/"+idiomCardPromptVersion, codec.IdiomCardProviderVersion())
}

func TestIdiomCardCodecDecodesAResolvedResponseAndMapsEvidenceHandles(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := idiomCardRequest()
	handle := newContextualGlossHandles(input).toHandle["wikt:capable"]
	content := `{"item_id":"` + IdiomCardItemHandle + `","source_language":"it","target_language":"en","translation":"capable","sentence_translation":"He is really a capable young man.","sentence_translation_targets":["capable young man"],"gloss":"capable","evidence_ids":["` + handle + `"],"context_only":false}`
	response, err := codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(content)+`}}]}`))
	require.NoError(t, err)
	assert.Equal(t, "capable", response.Gloss)
	assert.Equal(t, "capable", response.Translation)
	assert.Equal(t, []string{"wikt:capable"}, response.EvidenceIDs)
	assert.Equal(t, []string{"capable young man"}, response.SentenceTranslationTargets)
	assert.False(t, response.ContextOnly)
}

func TestIdiomCardCodecAcceptsLiteralUseAndRejectsLemmaMismatch(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := idiomCardRequest()
	literal := `{"item_id":"` + IdiomCardItemHandle + `","source_language":"it","target_language":"en","translation":"leg","sentence_translation":"He has a leg.","sentence_translation_targets":[],"gloss":"","evidence_ids":[],"context_only":true,"unresolved_kind":"literal_use","unresolved_reason":"The sentence uses gamba as the literal body part."}`
	response, err := codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(literal)+`}}]}`))
	require.NoError(t, err)
	assert.Empty(t, response.Gloss)
	assert.Equal(t, ContextualGlossUnresolvedLiteralUse, response.UnresolvedKind)

	mismatch := strings.Replace(literal, `"literal_use"`, `"lemma_mismatch"`, 1)
	_, err = codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(mismatch)+`}}]}`))
	assert.ErrorIs(t, err, ErrInvalidTranslationResponse)
}

func TestIdiomCardCodecRequiresTheCompleteOutcome(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := idiomCardRequest()
	content := `{"item_id":"` + IdiomCardItemHandle + `","source_language":"it","target_language":"en","translation":"capable","sentence_translation":"He is really a capable young man.","sentence_translation_targets":[],"gloss":"capable","evidence_ids":[],"context_only":true}`
	body := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`
	_, err = codec.DecodeResponse(input, []byte(body))
	require.NoError(t, err)

	missingGloss := strings.Replace(content, `"gloss":"capable",`, ``, 1)
	_, err = codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(missingGloss)+`}}]}`))
	assert.ErrorIs(t, err, ErrInvalidTranslationResponse)
}
