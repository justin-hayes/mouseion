package enrichment

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// greekEntryPassRequest is a Study pass request over two LSJ homograph entries
// of one lemma, whose senses are cited by entry-carrying evidence IDs.
func greekEntryPassRequest() TranslationRequest {
	return TranslationRequest{
		Language: "grc", TargetLanguage: "en", CanonicalLemma: "λέγω", UPOS: "VERB",
		TargetWord:                  "λέγει",
		ExampleSentence:             "ὁ ἀνὴρ λέγει.",
		PassageContext:              "The man speaks.",
		RequireEvidenceAssistedPass: true,
		StudyPass:                   true,
		CandidateSenses: []LexicalSense{
			{EvidenceID: EntryEvidenceID("n61607", "n61607.0"), Source: "lsj", Gloss: "to lay"},
			{EvidenceID: EntryEvidenceID("n61607", "n61607.1"), Source: "lsj", Gloss: "to pick"},
			{EvidenceID: EntryEvidenceID("n61608", "n61608.0"), Source: "lsj", Gloss: "to say"},
		},
	}
}

func TestGreekStudyPassNamesLSJAndCarriesEntries(t *testing.T) {
	body, err := passCodec(t).EncodeRequest(greekEntryPassRequest())
	require.NoError(t, err)
	var request struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	assert.Contains(t, request.Messages[0].Content, "LSJ")
	assert.Contains(t, request.Messages[0].Content, "entry handle")
	assert.NotContains(t, request.Messages[0].Content, "Lewis & Short")

	var transmitted struct {
		Senses []struct {
			EvidenceID string `json:"evidence_id"`
			Entry      string `json:"entry"`
		} `json:"candidate_senses"`
	}
	require.NoError(t, json.Unmarshal([]byte(request.Messages[1].Content), &transmitted))
	require.Len(t, transmitted.Senses, 3)
	assert.Equal(t, transmitted.Senses[0].Entry, transmitted.Senses[1].Entry)
	assert.NotEmpty(t, transmitted.Senses[0].Entry)
	assert.NotEqual(t, transmitted.Senses[0].Entry, transmitted.Senses[2].Entry, "homographs carry distinct entry handles")
}

// TestGreekStudyPassRequestSendsOnlyShortHandles encodes a request over
// realistic native LSJ IDs and proves from the bytes that would be sent that no
// native entry or sense ID goes out.
func TestGreekStudyPassRequestSendsOnlyShortHandles(t *testing.T) {
	body, err := passCodec(t).EncodeRequest(greekEntryPassRequest())
	require.NoError(t, err)
	assert.NotContains(t, string(body), "n6160", "no native LSJ entry or sense ID appears in the request")
	assert.NotContains(t, string(body), "#", "no entry-carrying evidence ID appears in the request")
	var request struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	var payload struct {
		Senses []map[string]any `json:"candidate_senses"`
	}
	require.NoError(t, json.Unmarshal([]byte(request.Messages[1].Content), &payload))
	var handles []any
	for _, sense := range payload.Senses {
		assert.Len(t, sense, 4, "only the handles, the source and the definition")
		handles = append(handles, sense["evidence_id"])
	}
	assert.Equal(t, []any{"e1", "e2", "e3"}, handles)
	assert.Equal(t, "h1", payload.Senses[0]["entry"])
	assert.Equal(t, "h2", payload.Senses[2]["entry"])
}

func TestGreekStudyPassDecodesACitedHandleToOneEntryAndSense(t *testing.T) {
	resolved, err := passCodec(t).DecodeResponse(greekEntryPassRequest(), passChatBody(t, `{"item_id":"`+ContextualGlossItemHandle+`","source_language":"grc","target_language":"en","translation":"say","sentence_translation":"The man speaks.","sentence_translation_targets":[],"gloss":"say","evidence_ids":["e3"],"context_only":false}`))
	require.NoError(t, err)
	require.Len(t, resolved.EvidenceIDs, 1)
	entry, sense, ok := SplitEntryEvidenceID(resolved.EvidenceIDs[0])
	require.True(t, ok)
	assert.Equal(t, "n61608", entry)
	assert.Equal(t, "n61608.0", sense)
}

func TestGreekStudyPassRejectsUnknownAndRepeatedHandles(t *testing.T) {
	for name, ids := range map[string]string{"an unknown handle": `["e9"]`, "a repeated handle": `["e1","e1"]`} {
		t.Run(name, func(t *testing.T) {
			_, err := passCodec(t).DecodeResponse(greekEntryPassRequest(), passChatBody(t, `{"item_id":"`+ContextualGlossItemHandle+`","source_language":"grc","target_language":"en","translation":"say","sentence_translation":"The man speaks.","sentence_translation_targets":[],"gloss":"say","evidence_ids":`+ids+`,"context_only":false}`))
			require.Error(t, err)
		})
	}
}

// TestGreekDeckPassContractIsUnchanged proves the Prepared deck's pass, which
// does not set StudyPass, keeps the existing Greek contract byte for byte.
func TestGreekDeckPassContractIsUnchanged(t *testing.T) {
	codec := passCodec(t)
	deck := greekEntryPassRequest()
	deck.StudyPass = false
	deck.CandidateSenses = []LexicalSense{{EvidenceID: "n61607.0", Source: "lsj", Gloss: "to lay"}}
	body, err := codec.EncodeRequest(deck)
	require.NoError(t, err)
	assert.NotContains(t, string(body), `"entry"`, "the deck's senses carry no entry handle")
	assert.Contains(t, string(body), "candidate_senses are the pinned LSJ senses of the lemma, each with a short evidence_id handle. Return")
	assert.Equal(t, "translation-v16-evidence-assisted-greek", codec.EvidenceAssistedPassPromptContract())
	assert.NotEqual(t, codec.EvidenceAssistedPassPromptContract(), codec.GreekEntryEvidenceAssistedPassPromptContract())
	assert.Contains(t, codec.GreekEntryEvidenceAssistedPassProviderVersion(), codec.GreekEntryEvidenceAssistedPassPromptContract())
}

// TestGreekEntryPassIsRefusedUntilRecorded proves the new payload kind has no
// recorded assessment, so the gate refuses it for every language, while a record
// pinned to the current contract would allow it.
func TestGreekEntryPassIsRefusedUntilRecorded(t *testing.T) {
	codec := passCodec(t)
	for _, language := range []string{"grc", "la", "de"} {
		t.Run(language, func(t *testing.T) {
			require.ErrorIs(t, ProviderAssessments().MaySend(GreekEntryEvidenceAssistedPassPayload, language, codec), ErrProviderAssessmentNotRecorded)
		})
	}
	t.Run("the Greek deck and Latin records never answer for it", func(t *testing.T) {
		set := ProviderAssessmentSet{evidenceAssistedPassAssessment, latinEvidenceAssistedPassAssessment}
		require.ErrorIs(t, set.MaySend(GreekEntryEvidenceAssistedPassPayload, "grc", codec), ErrProviderAssessmentNotRecorded)
	})
	t.Run("a record pinned to the current contract allows it", func(t *testing.T) {
		record := ProviderAssessment{
			Kind: GreekEntryEvidenceAssistedPassPayload, Language: "grc", Provider: codec.ProviderName(),
			PromptVersion: codec.GreekEntryEvidenceAssistedPassPromptContract(), Fields: GreekEntryEvidenceAssistedPassPayloadFields(), Recorded: true,
		}
		require.NoError(t, ProviderAssessmentSet{record}.MaySend(GreekEntryEvidenceAssistedPassPayload, "grc", codec))
		record.Recorded = false
		require.ErrorIs(t, ProviderAssessmentSet{record}.MaySend(GreekEntryEvidenceAssistedPassPayload, "grc", codec), ErrProviderAssessmentNotRecorded)
	})
}

func TestStudyEvidenceAssistedPassKindFollowsThePolicy(t *testing.T) {
	assert.Equal(t, GreekEntryEvidenceAssistedPassPayload, StudyEvidenceAssistedPassKind("grc"))
	assert.Equal(t, LatinEvidenceAssistedPassPayload, StudyEvidenceAssistedPassKind("la"))
	assert.Equal(t, EvidenceAssistedPassPayload, EvidenceAssistedPassKind("grc"), "the deck's kind is unchanged")
}

func TestGreekEntryPassProviderVersionNamesTheModelAndContract(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "gpt-test", ReasoningEffort: "low", SupportsReasoningEffort: true})
	require.NoError(t, err)
	assert.Equal(t, "gpt-test/translation-v19-evidence-assisted-greek-entries-reasoning-low", codec.GreekEntryEvidenceAssistedPassProviderVersion())
	assert.Equal(t, "gpt-test/translation-v19-evidence-assisted-greek-entries", passCodecVersion(t))
	var none *TranslationCodec
	assert.Empty(t, none.GreekEntryEvidenceAssistedPassProviderVersion())
}

func passCodecVersion(t *testing.T) string {
	t.Helper()
	return passCodec(t).GreekEntryEvidenceAssistedPassProviderVersion()
}

func TestStudyEvidenceAssistedPassKindOfAModernLanguageIsTheDeckPass(t *testing.T) {
	assert.Equal(t, EvidenceAssistedPassPayload, StudyEvidenceAssistedPassKind("de"))
}
