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

func TestIdiomCardCodecAcceptsAmbiguousAndRejectsUnknownKinds(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := idiomCardRequest()
	ambiguous := `{"item_id":"` + IdiomCardItemHandle + `","source_language":"it","target_language":"en","translation":"leg","sentence_translation":"He has a leg.","sentence_translation_targets":[],"gloss":"","evidence_ids":[],"context_only":true,"unresolved_kind":"ambiguous","unresolved_reason":"The sentence does not distinguish the meanings."}`
	response, err := codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(ambiguous)+`}}]}`))
	require.NoError(t, err)
	assert.Empty(t, response.Gloss)
	assert.Equal(t, ContextualGlossUnresolvedAmbiguous, response.UnresolvedKind)
	assert.Equal(t, "The sentence does not distinguish the meanings.", response.UnresolvedReason)

	for _, kind := range []string{"lemma_mismatch", "unsupported_kind"} {
		invalid := strings.Replace(ambiguous, `"ambiguous"`, `"`+kind+`"`, 1)
		_, err = codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(invalid)+`}}]}`))
		assert.ErrorIs(t, err, ErrInvalidTranslationResponse, "kind=%s", kind)
	}
}

// The idiom-card prompt defines translation as the idiom in the confirmed
// sense, so an unresolved answer may leave it blank (#2385).
func TestIdiomCardCodecAcceptsAnEmptyTranslationOnAnUnresolvedAnswer(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := idiomCardRequest()
	for _, kind := range []ContextualGlossUnresolvedKind{ContextualGlossUnresolvedLiteralUse, ContextualGlossUnresolvedAmbiguous} {
		t.Run(string(kind), func(t *testing.T) {
			content := `{"item_id":"` + IdiomCardItemHandle + `","source_language":"it","target_language":"en","translation":"","sentence_translation":"He has a leg.","sentence_translation_targets":[],"gloss":"","evidence_ids":[],"context_only":true,"unresolved_kind":"` + string(kind) + `","unresolved_reason":"The idiom is not used here."}`
			response, err := codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(content)+`}}]}`))
			require.NoError(t, err)
			assert.Empty(t, response.Translation)
			assert.Empty(t, response.Gloss)
			assert.Equal(t, kind, response.UnresolvedKind)
		})
	}
}

func TestIdiomCardCodecRejectsAnEmptyTranslationOnAResolvedAnswer(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := idiomCardRequest()
	handle := newContextualGlossHandles(input).toHandle["wikt:capable"]
	content := `{"item_id":"` + IdiomCardItemHandle + `","source_language":"it","target_language":"en","translation":"","sentence_translation":"He is really a capable young man.","sentence_translation_targets":[],"gloss":"capable","evidence_ids":["` + handle + `"],"context_only":false}`
	_, err = codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(content)+`}}]}`))
	assert.ErrorIs(t, err, ErrInvalidTranslationResponse)
}

func TestNormalizeTranslationResponseKeepsTheEmptyTranslationRuleOutsideUnresolvedIdioms(t *testing.T) {
	unresolved := TranslationResponse{SentenceTranslation: "He has a leg.", ContextOnly: true, UnresolvedKind: ContextualGlossUnresolvedLiteralUse, UnresolvedReason: "literal"}

	_, err := NormalizeTranslationResponse(idiomCardRequest(), unresolved)
	require.NoError(t, err, "an unresolved idiom answer may leave translation blank")

	resolved := TranslationResponse{SentenceTranslation: "He is capable.", Gloss: "capable", EvidenceIDs: []string{"wikt:capable"}}
	_, err = NormalizeTranslationResponse(idiomCardRequest(), resolved)
	require.ErrorContains(t, err, "translation is empty")

	lemma := TranslationRequest{Language: "it", TargetLanguage: "en", CanonicalLemma: "gamba", UPOS: "NOUN", ExampleSentence: "Ho una gamba."}
	_, err = NormalizeTranslationResponse(lemma, TranslationResponse{SentenceTranslation: "I have a leg."})
	require.ErrorContains(t, err, "translation is empty")

	contextual := lemma
	contextual.RequireContextualGloss = true
	_, err = NormalizeTranslationResponse(contextual, TranslationResponse{SentenceTranslation: "I have a leg.", ContextOnly: true, UnresolvedKind: ContextualGlossUnresolvedAmbiguous, UnresolvedReason: "unclear"})
	require.ErrorContains(t, err, "translation is empty", "a contextual-gloss abstention keeps the rule")
}

func TestIdiomCardCodecDiscardsAStrayKindAndReasonBesideAResolvedGloss(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := idiomCardRequest()
	content := `{"item_id":"` + IdiomCardItemHandle + `","source_language":"it","target_language":"en","translation":"capable","sentence_translation":"He is really a capable young man.","sentence_translation_targets":["capable"],"gloss":"capable","evidence_ids":[],"context_only":true,"unresolved_kind":"literal_use","unresolved_reason":"The words are literal."}`
	response, err := codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(content)+`}}]}`))
	require.NoError(t, err)
	assert.Equal(t, "capable", response.Gloss)
	assert.Empty(t, response.UnresolvedKind, "a stray kind beside a resolved Gloss is discarded")
	assert.Empty(t, response.UnresolvedReason, "a stray reason beside a resolved Gloss is discarded")
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

// The 2026-11-08 Italian idiom review failed on three cards: a Gloss that
// followed the first listed sense (di fronte, "front"), a sense the dictionary
// does not list (comparative rispetto a), and a literal use confirmed despite
// the literal-use marker sense (fare bene). idiom-card-v2 answers each.
func TestIdiomCardContractV2CarriesTheSentenceSenseLiteralMarkerAndNoFittingSenseRules(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	t.Run("version", func(t *testing.T) {
		require.Equal(t, "idiom-card-v2", codec.IdiomCardPromptContract())
		require.Equal(t, "model/idiom-card-v2", codec.IdiomCardProviderVersion())
	})

	body, err := codec.EncodeRequest(idiomCardRequest())
	require.NoError(t, err)
	for name, rule := range map[string]string{
		"sense the sentence uses":       "the order of candidate_senses is not a preference",
		"gloss agrees with translation": "translation and gloss must render the same sense",
		"literal-use marker":            "Used other than figuratively or idiomatically",
		"no fitting sense abstains":     "If no candidate sense matches the meaning this sentence uses, do not gloss from context",
		"confirmation cites a sense":    "a confirmed gloss always cites at least one candidate sense",
	} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, string(body), rule)
		})
	}
	t.Run("no context-only Gloss path", func(t *testing.T) {
		require.NotContains(t, string(body), "or an empty list and context_only=true when none applies")
	})
}
