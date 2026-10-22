package enrichment_test

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/require"
)

type faustDirectorProviderFixture struct {
	Name                  string                    `json:"name"`
	Language              string                    `json:"language"`
	TargetLanguage        string                    `json:"target_language"`
	CanonicalLemma        string                    `json:"canonical_lemma"`
	UPOS                  string                    `json:"upos"`
	TargetWord            string                    `json:"target_word"`
	SourceSentence        string                    `json:"source_sentence"`
	ForbiddenContext      []string                  `json:"forbidden_context_markers"`
	CandidateSenses       []enrichment.LexicalSense `json:"candidate_senses"`
	Translation           string                    `json:"translation"`
	FallbackGloss         string                    `json:"fallback_gloss"`
	SentenceTranslation   string                    `json:"sentence_translation"`
	ContiguousTargets     []string                  `json:"contiguous_targets"`
	ContiguousExpected    string                    `json:"contiguous_expected"`
	DiscontinuousTargets  []string                  `json:"discontinuous_targets"`
	DiscontinuousExpected string                    `json:"discontinuous_expected"`
	UnhighlightedTargets  []string                  `json:"unhighlighted_targets"`
	UnhighlightedExpected string                    `json:"unhighlighted_expected"`
}

func loadFaustDirectorFixture(t *testing.T) faustDirectorProviderFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/faust_director_provider.json")
	require.NoError(t, err)
	var fixture faustDirectorProviderFixture
	require.NoError(t, json.Unmarshal(raw, &fixture))
	return fixture
}

// faustDirectorFrozenRequest builds the provider request through the real
// prepared-deck freeze path, so the fixture also catches attribution, title or
// role text added while the study context is assembled.
func faustDirectorFrozenRequest(t *testing.T, fixture faustDirectorProviderFixture) enrichment.TranslationRequest {
	t.Helper()
	refs, err := json.Marshal([]struct {
		SentenceIndex int              `json:"sentence_index"`
		Text          string           `json:"text"`
		Location      map[string]int64 `json:"location"`
	}{{SentenceIndex: 0, Text: fixture.SourceSentence, Location: map[string]int64{"start_offset": 42}}})
	require.NoError(t, err)
	deck, _, err := cardexport.NewPresentation(nil).Freeze(t.Context(), "owner-1", "Faust. Eine Tragödie", []cardexport.CandidateProjection{{
		OwnerID: "owner-1", DeckName: "Faust. Eine Tragödie",
		Candidate: domain.SelectionCandidate{
			OwnerID: "owner-1", CorpusID: "corpus-faust", Language: fixture.Language,
			CanonicalLemma: fixture.CanonicalLemma, UPOS: fixture.UPOS,
			ObservedForms: []byte(`["` + fixture.TargetWord + `"]`), SentenceReferences: refs,
		},
		Entry: cardexport.Entry{
			OwnerID: "owner-1", Language: fixture.Language, CanonicalLemma: fixture.CanonicalLemma,
			UPOS: fixture.UPOS, SourceDocument: "Faust. Eine Tragödie", CandidateSenses: fixture.CandidateSenses,
		},
		Provider: "llm", ProviderVersion: "prompt-v2", TargetLanguage: fixture.TargetLanguage,
	}})
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 1)
	return work[0].Request
}

// TestFaustDirectorProviderRequestCarriesExactNewlineContext pins the exact
// provider payload for the four-source-line Director sentence: the LF-bearing
// sentence and bounded lexical evidence only, with no speaker label, title or
// role/orientation field.
func TestFaustDirectorProviderRequestCarriesExactNewlineContext(t *testing.T) {
	fixture := loadFaustDirectorFixture(t)
	require.Contains(t, fixture.SourceSentence, "\n", "fixture must carry an authorial line break")
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "fixture-model"})
	require.NoError(t, err)

	input := faustDirectorFrozenRequest(t, fixture)
	body, err := codec.EncodeRequest(input)
	require.NoError(t, err)

	var request struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	require.Len(t, request.Messages, 2)
	require.Equal(t, "system", request.Messages[0].Role)
	require.Equal(t, "user", request.Messages[1].Role)

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(request.Messages[1].Content), &payload))
	require.Equal(t, []string{
		"candidate_senses", "canonical_lemma", "example_sentence", "item_id",
		"language", "target_language", "target_word", "upos",
	}, slices.Sorted(maps.Keys(payload)), "provider payload field set changed")

	t.Run("carries the exact newline-bearing sentence", func(t *testing.T) {
		var sentence string
		require.NoError(t, json.Unmarshal(payload["example_sentence"], &sentence))
		require.Equal(t, fixture.SourceSentence, sentence)
		require.Equal(t, strings.Count(fixture.SourceSentence, "\n"), strings.Count(sentence, "\n"))
	})
	t.Run("carries the tested target and bounded evidence", func(t *testing.T) {
		require.Equal(t, fixture.TargetWord, input.TargetWord)
		require.Equal(t, fixture.CanonicalLemma, input.CanonicalLemma)
		require.Len(t, input.CandidateSenses, len(fixture.CandidateSenses))
	})
	for _, forbidden := range fixture.ForbiddenContext {
		t.Run("does not leak "+forbidden, func(t *testing.T) {
			require.NotContains(t, request.Messages[1].Content, forbidden, "provider context leaked attribution/title/orientation")
			require.NotContains(t, request.Messages[0].Content, forbidden)
		})
	}
}

// TestFaustDirectorProviderResponseKeepsValidatedAlignment pins one complete
// English prose rendering: contiguous and discontinuous valid alignment is
// emphasised, invalid alignment falls back to the full unhighlighted
// translation, and provider-supplied markup is rejected.
func TestFaustDirectorProviderResponseKeepsValidatedAlignment(t *testing.T) {
	fixture := loadFaustDirectorFixture(t)
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "fixture-model"})
	require.NoError(t, err)
	input := faustDirectorFrozenRequest(t, fixture)

	for _, test := range []struct {
		name, expected string
		targets        []string
	}{
		{"contiguous alignment", fixture.ContiguousExpected, fixture.ContiguousTargets},
		{"discontinuous alignment", fixture.DiscontinuousExpected, fixture.DiscontinuousTargets},
		{"unhighlighted fallback", fixture.UnhighlightedExpected, fixture.UnhighlightedTargets},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := codec.DecodeResponse(input, faustDirectorResponseBody(t, input, fixture, fixture.SentenceTranslation, test.targets))
			require.NoError(t, err)
			require.Equal(t, fixture.SentenceTranslation, response.SentenceTranslation)
			require.Equal(t, test.expected, cardexport.HighlightEnglishTargets(response.SentenceTranslation, response.SentenceTranslationTargets, ""))
		})
	}

	t.Run("provider HTML is rejected", func(t *testing.T) {
		body := faustDirectorResponseBody(t, input, fixture, "<b>"+fixture.SentenceTranslation+"</b>", fixture.ContiguousTargets)
		_, err := codec.DecodeResponse(input, body)
		require.ErrorIs(t, err, enrichment.ErrInvalidTranslationResponse)
	})
}

func faustDirectorResponseBody(t *testing.T, input enrichment.TranslationRequest, fixture faustDirectorProviderFixture, sentenceTranslation string, targets []string) []byte {
	t.Helper()
	content, err := json.Marshal(struct {
		ItemID                     string   `json:"item_id"`
		SourceLanguage             string   `json:"source_language"`
		TargetLanguage             string   `json:"target_language"`
		Translation                string   `json:"translation"`
		FallbackGloss              string   `json:"fallback_gloss"`
		SentenceTranslation        string   `json:"sentence_translation"`
		SentenceTranslationTargets []string `json:"sentence_translation_targets"`
		SenseOrder                 []int    `json:"sense_order"`
	}{
		ItemID: enrichment.TranslationItemID(input), SourceLanguage: fixture.Language, TargetLanguage: fixture.TargetLanguage,
		Translation: fixture.Translation, FallbackGloss: fixture.FallbackGloss,
		SentenceTranslation: sentenceTranslation, SentenceTranslationTargets: targets,
	})
	require.NoError(t, err)
	body, err := json.Marshal(struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}{Choices: []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}{{Message: struct {
		Content string `json:"content"`
	}{Content: string(content)}}}})
	require.NoError(t, err)
	return body
}
