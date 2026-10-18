package enrichment

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLSJSenseSummaryRequestCarriesOnlyLSJEntryFields proves the context-free
// sense summary payload transmits only the pinned LSJ entry's fields: no
// representative sentence, no passage context and no learner data.
func TestLSJSenseSummaryRequestCarriesOnlyLSJEntryFields(t *testing.T) {
	codec := passCodec(t)
	body, err := codec.EncodeRequest(TranslationRequest{
		RequireLSJSenseSummary: true,
		Headword:               "λόγος",
		LSJSenses: []LSJSenseSummarySense{
			{ID: "n1.0", Glosses: []string{"word", "speech"}},
			{ID: "n1.1", Glosses: []string{"account"}},
		},
	})
	require.NoError(t, err)

	var payload struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Len(t, payload.Messages, 2)
	assert.Equal(t, lsjSenseSummarySystemPrompt, payload.Messages[0].Content)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(payload.Messages[1].Content), &fields))
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	assert.Equal(t, []string{"headword", "item_id", "senses"}, keys)
	expectedFields := LSJSenseSummaryPayloadFields()
	sort.Strings(expectedFields)
	assert.Equal(t, expectedFields, keys)
	assert.Equal(t, LSJSenseSummaryItemHandle, jsonString(t, fields["item_id"]))
	assert.Equal(t, "λόγος", jsonString(t, fields["headword"]))

	var senses []struct {
		ID      string   `json:"id"`
		Glosses []string `json:"glosses"`
	}
	require.NoError(t, json.Unmarshal(fields["senses"], &senses))
	require.Len(t, senses, 2)
	assert.Equal(t, "n1.0", senses[0].ID)
	assert.Equal(t, []string{"word", "speech"}, senses[0].Glosses)
}

// TestLSJSenseSummaryRequestRequiresTheEntry proves a summary request with no
// headword or no sense is refused before anything is sent.
func TestLSJSenseSummaryRequestRequiresTheEntry(t *testing.T) {
	codec := passCodec(t)
	_, err := codec.EncodeRequest(TranslationRequest{RequireLSJSenseSummary: true, LSJSenses: []LSJSenseSummarySense{{ID: "n1.0"}}})
	require.Error(t, err)
	_, err = codec.EncodeRequest(TranslationRequest{RequireLSJSenseSummary: true, Headword: "λόγος"})
	require.Error(t, err)
}

// TestLSJSenseSummaryResponseDecodesLines proves a strict two-field summary
// response decodes its lines and rejects a stray item_id.
func TestLSJSenseSummaryResponseDecodesLines(t *testing.T) {
	codec := passCodec(t)
	body := []byte(`{"choices":[{"message":{"content":"{\"item_id\":\"s1\",\"summary_lines\":[{\"sense_ids\":[\"n1.0\"],\"text\":\"word or speech\"},{\"sense_ids\":[\"n1.1\"],\"text\":\"account\"}]}"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	response, err := codec.DecodeResponse(TranslationRequest{RequireLSJSenseSummary: true}, body)
	require.NoError(t, err)
	require.Len(t, response.SenseSummary, 2)
	assert.Equal(t, []string{"n1.0"}, response.SenseSummary[0].SenseIDs)
	assert.Equal(t, "word or speech", response.SenseSummary[0].Text)

	mismatch := []byte(`{"choices":[{"message":{"content":"{\"item_id\":\"other\",\"summary_lines\":[{\"sense_ids\":[\"n1.0\"],\"text\":\"word\"}]}"}}],"usage":{}}`)
	_, err = codec.DecodeResponse(TranslationRequest{RequireLSJSenseSummary: true}, mismatch)
	require.ErrorIs(t, err, ErrInvalidTranslationResponse)
}

// TestValidateLSJSenseSummaryDiscardsUnknownOrMissingSenseIDs proves a summary
// that cites an unknown sense ID, or cites none, is discarded so the card keeps
// its full LSJ entry.
func TestValidateLSJSenseSummaryDiscardsUnknownOrMissingSenseIDs(t *testing.T) {
	known := map[string]struct{}{"n1.0": {}, "n1.1": {}}
	summary := func(lines ...LSJSenseSummaryLine) LSJSenseSummary {
		return LSJSenseSummary{EntryID: "n1", Pin: "pin", Lines: lines}
	}
	valid := summary(LSJSenseSummaryLine{SenseIDs: []string{"n1.0"}, Text: "word"})
	_, ok := ValidateLSJSenseSummary(valid, known)
	assert.True(t, ok)

	if _, ok := ValidateLSJSenseSummary(summary(LSJSenseSummaryLine{SenseIDs: []string{"unknown"}, Text: "word"}), known); ok {
		t.Fatal("a summary citing an unknown sense ID must be discarded")
	}
	if _, ok := ValidateLSJSenseSummary(summary(LSJSenseSummaryLine{SenseIDs: nil, Text: "word"}), known); ok {
		t.Fatal("a summary line citing no sense ID must be discarded")
	}
	if _, ok := ValidateLSJSenseSummary(summary(), known); ok {
		t.Fatal("a summary with no lines must be discarded")
	}
	if _, ok := ValidateLSJSenseSummary(summary(LSJSenseSummaryLine{SenseIDs: []string{"n1.0"}, Text: "  "}), known); ok {
		t.Fatal("a summary line with no text must be discarded")
	}

	long := make([]LSJSenseSummaryLine, 0, MaxLSJSenseSummaryLines+1)
	for range MaxLSJSenseSummaryLines + 1 {
		long = append(long, LSJSenseSummaryLine{SenseIDs: []string{"n1.0"}, Text: "word"})
	}
	if _, ok := ValidateLSJSenseSummary(summary(long...), known); ok {
		t.Fatal("a summary running to more than a few lines must be discarded")
	}
	tooLong := LSJSenseSummaryLine{SenseIDs: []string{"n1.0"}, Text: strings.Repeat("a", MaxLSJSenseSummaryLineRunes+1)}
	if _, ok := ValidateLSJSenseSummary(summary(tooLong), known); ok {
		t.Fatal("a summary line longer than a short line must be discarded")
	}
}

func jsonString(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var value string
	require.NoError(t, json.Unmarshal(raw, &value))
	return value
}

// TestLSJSenseSummaryResponseRejectsMalformedContent proves the strict summary
// contract rejects unknown fields, duplicate keys, missing lines and trailing
// content rather than accepting a partial summary.
func TestLSJSenseSummaryResponseRejectsMalformedContent(t *testing.T) {
	codec := passCodec(t)
	for name, content := range map[string]string{
		"unknown field": `{"item_id":"s1","summary_lines":[{"sense_ids":["n1.0"],"text":"word"}],"extra":1}`,
		"duplicate key": `{"item_id":"s1","item_id":"s1","summary_lines":[{"sense_ids":["n1.0"],"text":"word"}]}`,
		"missing lines": `{"item_id":"s1"}`,
		"empty lines":   `{"item_id":"s1","summary_lines":[]}`,
	} {
		body := []byte(`{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}],"usage":{}}`)
		_, err := codec.DecodeResponse(TranslationRequest{RequireLSJSenseSummary: true}, body)
		require.ErrorIs(t, err, ErrInvalidTranslationResponse, name)
	}
}

// TestLSJSenseSummaryNormalizesLines proves a decoded line drops blank cited
// handles and trims its text, so a stray blank cannot pass the citation check.
func TestLSJSenseSummaryNormalizesLines(t *testing.T) {
	codec := passCodec(t)
	body := []byte(`{"choices":[{"message":{"content":"{\"item_id\":\"s1\",\"summary_lines\":[{\"sense_ids\":[\" n1.0 \",\"\"],\"text\":\"  word  \"}]}"}}],"usage":{}}`)
	response, err := codec.DecodeResponse(TranslationRequest{RequireLSJSenseSummary: true}, body)
	require.NoError(t, err)
	require.Len(t, response.SenseSummary, 1)
	assert.Equal(t, []string{"n1.0"}, response.SenseSummary[0].SenseIDs)
	assert.Equal(t, "word", response.SenseSummary[0].Text)
}

// TestLSJSenseSummaryRequestRejectsSenseWithoutID proves an uncitable sense is
// refused before anything is sent.
func TestLSJSenseSummaryRequestRejectsSenseWithoutID(t *testing.T) {
	codec := passCodec(t)
	_, err := codec.EncodeRequest(TranslationRequest{RequireLSJSenseSummary: true, Headword: "λόγος", LSJSenses: []LSJSenseSummarySense{{Glosses: []string{"word"}}}})
	require.Error(t, err)
}

// TestLSJSenseSummaryProviderVersionPinsTheContract proves the summary provider
// version carries the model, the pinned prompt contract and the reasoning
// effort, so a changed contract or model never reuses a stale summary.
func TestLSJSenseSummaryProviderVersionPinsTheContract(t *testing.T) {
	plain := passCodec(t)
	assert.Equal(t, "gpt-test/"+lsjSenseSummaryPromptVersion, plain.LSJSenseSummaryProviderVersion())
	assert.Equal(t, lsjSenseSummaryPromptVersion, plain.LSJSenseSummaryPromptContract())

	reasoning, err := NewTranslationCodec(LLMConfig{Model: "gpt-test", SupportsReasoningEffort: true, ReasoningEffort: "high"})
	require.NoError(t, err)
	assert.Equal(t, "gpt-test/"+lsjSenseSummaryPromptVersion+"-reasoning-high", reasoning.LSJSenseSummaryProviderVersion())
}
