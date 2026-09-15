package enrichment_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmptyCandidateFallbackRunsFromProviderToRenderedCard(t *testing.T) {
	type requestInput struct {
		Language        string          `json:"language"`
		TargetLanguage  string          `json:"target_language"`
		CanonicalLemma  string          `json:"canonical_lemma"`
		UPOS            string          `json:"upos"`
		TargetWord      string          `json:"target_word"`
		ExampleSentence string          `json:"example_sentence"`
		CandidateSenses json.RawMessage `json:"candidate_senses"`
	}
	type responseMessage struct {
		Content string `json:"content"`
	}
	type responseChoice struct {
		Message responseMessage `json:"message"`
	}
	var receivedUser requestInput
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 2 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &receivedUser); err != nil {
			http.Error(w, "invalid user content", http.StatusBadRequest)
			return
		}

		input := enrichment.TranslationRequest{
			Language:        receivedUser.Language,
			TargetLanguage:  receivedUser.TargetLanguage,
			CanonicalLemma:  receivedUser.CanonicalLemma,
			UPOS:            receivedUser.UPOS,
			TargetWord:      receivedUser.TargetWord,
			ExampleSentence: receivedUser.ExampleSentence,
		}
		content, err := json.Marshal(struct {
			ItemID                    string `json:"item_id"`
			SourceLanguage            string `json:"source_language"`
			TargetLanguage            string `json:"target_language"`
			Translation               string `json:"translation"`
			SentenceTranslation       string `json:"sentence_translation"`
			SentenceTranslationTarget string `json:"sentence_translation_target"`
			FallbackGloss             string `json:"fallback_gloss"`
		}{
			ItemID:                    enrichment.TranslationItemID(input),
			SourceLanguage:            input.Language,
			TargetLanguage:            input.TargetLanguage,
			Translation:               "rare word",
			SentenceTranslation:       "The rare thing is important today.",
			SentenceTranslationTarget: "rare",
			FallbackGloss:             "something uncommon",
		})
		if err != nil {
			http.Error(w, "invalid response", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Choices []responseChoice `json:"choices"`
		}{Choices: []responseChoice{{Message: responseMessage{Content: string(content)}}}})
	}))
	defer server.Close()

	config := enrichment.LLMConfig{Enabled: true, APIKey: "secret", Model: "model", BaseURL: server.URL + "/v1"}
	provider, err := enrichment.NewConfiguredLLMProvider(config, server.Client())
	require.NoError(t, err)
	service := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true, ContextMode: enrichment.SentenceContext}, nil, nil, nil, provider, nil)
	entry := cardexport.Entry{
		Language: "de", CanonicalLemma: "seltenes-wort", UPOS: "NOUN", Sentence: "Das seltene Wort ist heute wirklich wichtig.", TargetWord: "seltene",
		DictionaryProviderVersion: "dictionary-v4", FirstEncounter: 1,
	}
	manifest := cardexport.NewManifest("owner-1", "Book", []cardexport.Entry{entry})
	require.Len(t, manifest.EnrichmentCandidates(), 1)
	candidate := manifest.EnrichmentCandidates()[0]
	result := service.Enrich(context.Background(), []enrichment.Candidate{candidate})[0]
	assert.Nil(t, receivedUser.CandidateSenses)
	require.True(t, result.FallbackGloss.Available)
	assert.Equal(t, "something uncommon", result.FallbackGloss.Value)
	assert.Empty(t, result.Candidate.CandidateSenses)

	key, ok := service.ExternalCacheKey(candidate)
	require.True(t, ok)
	bound, err := manifest.BindCacheKeys([]enrichment.CacheKey{key})
	require.NoError(t, err)
	artifact, err := cardexport.NewService(nil).RenderManifest(context.Background(), bound, []cardexport.ExactEnrichment{{CacheKey: key, Result: result}})
	require.NoError(t, err)
	require.Len(t, artifact.Generated, 1)
	assert.Equal(t, "something uncommon", artifact.Generated[0].Note.Gloss)
	assert.Equal(t, 1, artifact.Completeness.CardsWithFallbackGloss)
}
