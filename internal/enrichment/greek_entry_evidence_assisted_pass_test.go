package enrichment

import (
	"encoding/json"
	"fmt"
	"strings"
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

// TestGreekEntryEvidenceAssistedPassAssessmentIsRecordedForGreekOnly proves the
// #2774 disposition is recorded for Greek alone, under the Greek entry contract,
// and that the other records are unchanged.
func TestGreekEntryEvidenceAssistedPassAssessmentIsRecordedForGreekOnly(t *testing.T) {
	assertRecordedAssessment(t, "#2774", GreekEntryEvidenceAssistedPassPayload, "grc",
		(*TranslationCodec).GreekEntryEvidenceAssistedPassPromptContract, GreekEntryEvidenceAssistedPassPayloadFields)

	t.Run("the disposition serves the Study surface", func(t *testing.T) {
		require.Equal(t, "approved for personal use of the locally produced Study surface", greekEntryEvidenceAssistedPassAssessment.Disposition)
		require.Equal(t, "translation-v19-evidence-assisted-greek-entries", greekEntryEvidenceAssistedPassAssessment.PromptVersion)
	})
	for _, language := range []string{"la", "de", "it"} {
		t.Run(language+" is refused", func(t *testing.T) {
			require.ErrorIs(t, ProviderAssessments().MaySend(GreekEntryEvidenceAssistedPassPayload, language, passCodec(t)), ErrProviderAssessmentNotRecorded)
		})
	}
	t.Run("the Greek deck and Latin records never answer for it", func(t *testing.T) {
		set := ProviderAssessmentSet{evidenceAssistedPassAssessment, latinEvidenceAssistedPassAssessment}
		require.ErrorIs(t, set.MaySend(GreekEntryEvidenceAssistedPassPayload, "grc", passCodec(t)), ErrProviderAssessmentNotRecorded)
	})
	t.Run("the new record never answers for the deck or Latin payload", func(t *testing.T) {
		set := ProviderAssessmentSet{greekEntryEvidenceAssistedPassAssessment}
		require.ErrorIs(t, set.MaySend(EvidenceAssistedPassPayload, "grc", passCodec(t)), ErrProviderAssessmentNotRecorded)
		require.ErrorIs(t, set.MaySend(LatinEvidenceAssistedPassPayload, "la", passCodec(t)), ErrProviderAssessmentNotRecorded)
	})
	t.Run("the #2171 record is unchanged", func(t *testing.T) {
		require.Equal(t, ProviderAssessment{
			Kind: EvidenceAssistedPassPayload, Language: "grc", Provider: "openai-compatible",
			PromptVersion: "translation-v16-evidence-assisted-greek",
			Fields: []string{
				"item_id", "language", "target_language", "canonical_lemma", "upos",
				"target_word", "example_sentence", "passage_context", "candidate_senses",
			},
			Disposition: "approved for personal use of the locally produced deck", Ticket: "#2171", Recorded: true,
		}, evidenceAssistedPassAssessment)
	})
	t.Run("a record that drifts from the contract refuses", func(t *testing.T) {
		drifted := greekEntryEvidenceAssistedPassAssessment
		drifted.PromptVersion = "translation-v18-evidence-assisted-greek-entries"
		require.ErrorIs(t, ProviderAssessmentSet{drifted}.MaySend(GreekEntryEvidenceAssistedPassPayload, "grc", passCodec(t)), ErrProviderAssessmentNotRecorded)
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

// TestGreekStudyPassRequestPayloadIsTheRecordedFieldSetInOrder proves from a
// real encoded request that the user message carries exactly the recorded fields
// in the recorded order.
func TestGreekStudyPassRequestPayloadIsTheRecordedFieldSetInOrder(t *testing.T) {
	body, err := passCodec(t).EncodeRequest(greekEntryPassRequest())
	require.NoError(t, err)
	var request struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	decoder := json.NewDecoder(strings.NewReader(request.Messages[1].Content))
	_, err = decoder.Token()
	require.NoError(t, err)
	var fields []string
	for decoder.More() {
		key, err := decoder.Token()
		require.NoError(t, err)
		name, isString := key.(string)
		require.True(t, isString)
		fields = append(fields, name)
		var skip json.RawMessage
		require.NoError(t, decoder.Decode(&skip))
	}
	assert.Equal(t, greekEntryEvidenceAssistedPassAssessment.Fields, fields)
}

// syntheticLargeGreekEntryRequest is a Study pass request over three synthetic
// homograph entries holding 140 senses between them, shaped like a frequent verb
// such as ἔχω (the test data holds no real LSJ text). Definitions run 30 to 130
// characters, as compact LSJ definitions do.
func syntheticLargeGreekEntryRequest() (TranslationRequest, int) {
	request := greekEntryPassRequest()
	request.CanonicalLemma, request.TargetWord = "ἔχω", "ἔχει"
	request.CandidateSenses = nil
	perEntry := []int{90, 36, 14}
	for e, count := range perEntry {
		entry := fmt.Sprintf("n%d", 20000+e)
		for s := range count {
			definition := strings.Repeat("hold, possess, carry, keep; ", 6)[:30+(s*7)%100]
			request.CandidateSenses = append(request.CandidateSenses, LexicalSense{
				EvidenceID: EntryEvidenceID(entry, fmt.Sprintf("%s.%d", entry, s)), Source: "lsj", Gloss: definition,
			})
		}
	}
	return request, len(request.CandidateSenses)
}

// TestGreekStudyPassRequestOfALargeEntryCarriesEverySenseWithinBudget measures
// the request a frequent word's uncapped senses produce. The sizes are recorded
// in the payload assessment; this test keeps them from drifting unnoticed.
func TestGreekStudyPassRequestOfALargeEntryCarriesEverySenseWithinBudget(t *testing.T) {
	request, senses := syntheticLargeGreekEntryRequest()
	body, err := passCodec(t).EncodeRequest(request)
	require.NoError(t, err)
	var decoded struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &decoded))
	var payload struct {
		Senses []map[string]any `json:"candidate_senses"`
	}
	require.NoError(t, json.Unmarshal([]byte(decoded.Messages[1].Content), &payload))
	t.Logf("senses=%d request_bytes=%d user_message_bytes=%d approx_tokens=%d", senses, len(body), len(decoded.Messages[1].Content), len(body)/4)

	assert.Len(t, payload.Senses, 140, "no sense is capped")
	assert.Greater(t, len(body), 8*1024)
	assert.Less(t, len(body), 32*1024, "a very large entry stays inside a few thousand tokens")
	assert.NotContains(t, string(body), "n2000", "no native ID goes out for a large entry either")
}
