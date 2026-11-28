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

// TestGreekEntryEvidenceAssistedPassAssessmentIsRecordedForGreekOnly proves the
// #2774 disposition, re-recorded on #2781 for the citation-form contract, is
// recorded for Greek alone.
func TestGreekEntryEvidenceAssistedPassAssessmentIsRecordedForGreekOnly(t *testing.T) {
	assertRecordedAssessment(t, "#2781", GreekEntryEvidenceAssistedPassPayload, "grc",
		(*TranslationCodec).GreekEntryEvidenceAssistedPassPromptContract, GreekEntryEvidenceAssistedPassPayloadFields)

	t.Run("the disposition serves the Study surface", func(t *testing.T) {
		require.Equal(t, "approved for personal use of the locally produced Study surface", greekEntryEvidenceAssistedPassAssessment.Disposition)
		require.Equal(t, "translation-v20-evidence-assisted-greek-entries-citation-form", greekEntryEvidenceAssistedPassAssessment.PromptVersion)
	})
	t.Run("the superseded v19 contract is refused", func(t *testing.T) {
		superseded := greekEntryEvidenceAssistedPassAssessment
		superseded.PromptVersion = "translation-v19-evidence-assisted-greek-entries"
		require.ErrorIs(t, ProviderAssessmentSet{superseded}.MaySend(GreekEntryEvidenceAssistedPassPayload, "grc", passCodec(t)), ErrProviderAssessmentNotRecorded)
	})
	for _, language := range []string{"la", "de", "it"} {
		t.Run(language+" is refused", func(t *testing.T) {
			require.ErrorIs(t, ProviderAssessments().MaySend(GreekEntryEvidenceAssistedPassPayload, language, passCodec(t)), ErrProviderAssessmentNotRecorded)
		})
	}
	t.Run("the Latin record never answers for it", func(t *testing.T) {
		set := ProviderAssessmentSet{latinEvidenceAssistedPassAssessment}
		require.ErrorIs(t, set.MaySend(GreekEntryEvidenceAssistedPassPayload, "grc", passCodec(t)), ErrProviderAssessmentNotRecorded)
	})
	t.Run("the new record never answers for the Latin payload", func(t *testing.T) {
		set := ProviderAssessmentSet{greekEntryEvidenceAssistedPassAssessment}
		require.ErrorIs(t, set.MaySend(LatinEvidenceAssistedPassPayload, "la", passCodec(t)), ErrProviderAssessmentNotRecorded)
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
}

func TestGreekEntryPassProviderVersionNamesTheModelAndContract(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "gpt-test", ReasoningEffort: "low", SupportsReasoningEffort: true})
	require.NoError(t, err)
	assert.Equal(t, "gpt-test/"+greekCitationFormVersion+"-reasoning-low", codec.GreekEntryEvidenceAssistedPassProviderVersion())
	assert.Equal(t, "gpt-test/"+greekCitationFormVersion, passCodecVersion(t))
	var none *TranslationCodec
	assert.Empty(t, none.GreekEntryEvidenceAssistedPassProviderVersion())
}

func passCodecVersion(t *testing.T) string {
	t.Helper()
	return passCodec(t).GreekEntryEvidenceAssistedPassProviderVersion()
}

func TestStudyEvidenceAssistedPassKindOfAModernLanguageIsRefused(t *testing.T) {
	kind := StudyEvidenceAssistedPassKind("de")
	assert.Empty(t, kind)
	require.ErrorIs(t, ProviderAssessments().MaySend(kind, "de", passCodec(t)), ErrProviderAssessmentNotRecorded)
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

	t.Run("no sense is capped", func(t *testing.T) {
		require.Len(t, payload.Senses, 140)
	})
	t.Run("the request stays inside a few thousand tokens", func(t *testing.T) {
		require.Greater(t, len(body), 8*1024)
		require.Less(t, len(body), 32*1024)
	})
	t.Run("no native ID goes out for a large entry either", func(t *testing.T) {
		require.NotContains(t, string(body), "n2000")
	})
}
