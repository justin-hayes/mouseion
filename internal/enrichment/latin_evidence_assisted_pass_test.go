package enrichment

import (
	"encoding/json"
	"slices"
	"strings"
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
	tests := []struct{ name, ids string }{
		{"an unknown handle", `["e9"]`},
		{"a repeated handle", `["e1","e1"]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := codec.DecodeResponse(input, passChatBody(t, `{"item_id":"`+ContextualGlossItemHandle+`","source_language":"la","target_language":"en","translation":"say","sentence_translation":"Caesar speaks.","sentence_translation_targets":[],"gloss":"say","evidence_ids":`+tc.ids+`,"context_only":false}`))
			require.Error(t, err)
		})
	}
}

// TestLatinEvidenceAssistedPassAssessmentIsRecordedForLatinOnly proves the #2766
// disposition is recorded for Latin alone, under the Latin contract.
func TestLatinEvidenceAssistedPassAssessmentIsRecordedForLatinOnly(t *testing.T) {
	assertRecordedAssessment(t, "#2766", LatinEvidenceAssistedPassPayload, "la",
		(*TranslationCodec).LatinEvidenceAssistedPassPromptContract, LatinEvidenceAssistedPassPayloadFields)

	t.Run("the disposition serves the deck and the Study surface", func(t *testing.T) {
		require.Equal(t, "approved for personal use of the locally produced deck and Study surface", latinEvidenceAssistedPassAssessment.Disposition)
	})
	for _, language := range []string{"grc", "de"} {
		t.Run(language+" is refused", func(t *testing.T) {
			require.ErrorIs(t, ProviderAssessments().MaySend(LatinEvidenceAssistedPassPayload, language, passCodec(t)), ErrProviderAssessmentNotRecorded)
		})
	}
	t.Run("the Latin record never answers for the Greek payload", func(t *testing.T) {
		require.ErrorIs(t, ProviderAssessmentSet{latinEvidenceAssistedPassAssessment}.MaySend(EvidenceAssistedPassPayload, "la", passCodec(t)), ErrProviderAssessmentNotRecorded)
	})
	t.Run("Greek's pass contract is unchanged", func(t *testing.T) {
		require.Equal(t, "translation-v16-evidence-assisted-greek", evidenceAssistedPassAssessment.PromptVersion)
		require.Equal(t, "grc", evidenceAssistedPassAssessment.Language)
		require.Equal(t, "#2171", evidenceAssistedPassAssessment.Ticket)
	})
}

// TestLatinPassRequestSendsOnlyShortHandles encodes a request over realistic
// native Lewis & Short IDs and proves, from the bytes that would be sent, that
// only the per-request handles go out.
func TestLatinPassRequestSendsOnlyShortHandles(t *testing.T) {
	input := latinPassRequest()
	input.CandidateSenses = []LexicalSense{
		{EvidenceID: LatinEvidenceID("n40213", "n40213.0"), Source: "ls", Gloss: "to dedicate"},
		{EvidenceID: LatinEvidenceID("n40213", "n40213.1"), Source: "ls", Gloss: "to devote"},
		{EvidenceID: LatinEvidenceID("n40214", "n40214.0"), Source: "ls", Gloss: "to say"},
	}
	body, err := passCodec(t).EncodeRequest(input)
	require.NoError(t, err)
	var request struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	require.Len(t, request.Messages, 2)

	t.Run("no native entry or sense ID appears anywhere in the request", func(t *testing.T) {
		require.NotContains(t, string(body), "n402")
	})
	t.Run("the payload is exactly the recorded field set, in order", func(t *testing.T) {
		decoder := json.NewDecoder(strings.NewReader(request.Messages[1].Content))
		_, err := decoder.Token()
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
		require.Equal(t, latinEvidenceAssistedPassAssessment.Fields, fields)
	})
	t.Run("each sense carries only its handles, source and definition", func(t *testing.T) {
		var payload struct {
			Senses []map[string]any `json:"candidate_senses"`
		}
		require.NoError(t, json.Unmarshal([]byte(request.Messages[1].Content), &payload))
		require.Len(t, payload.Senses, 3)
		var shapes [][]string
		var handles []any
		for _, sense := range payload.Senses {
			keys := make([]string, 0, len(sense))
			for key := range sense {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			shapes = append(shapes, keys)
			handles = append(handles, sense["evidence_id"])
		}
		want := []string{"entry", "evidence_id", "gloss", "source"}
		require.Equal(t, [][]string{want, want, want}, shapes)
		require.Equal(t, []any{"e1", "e2", "e3"}, handles)
		require.Equal(t, "h1", payload.Senses[0]["entry"])
		require.Equal(t, "h2", payload.Senses[2]["entry"])
	})
}

func TestEvidenceAssistedPassKindFollowsLanguage(t *testing.T) {
	assert.Equal(t, LatinEvidenceAssistedPassPayload, EvidenceAssistedPassKind("la"))
	assert.Equal(t, EvidenceAssistedPassPayload, EvidenceAssistedPassKind("grc"))
}
