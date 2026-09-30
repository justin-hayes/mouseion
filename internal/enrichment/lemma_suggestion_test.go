package enrichment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLemmaSuggestionProviderSendsOnlyBoundedOccurrenceEvidence(t *testing.T) {
	var sent map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
		_, writeErr := w.Write([]byte(`{"choices":[{"message":{"content":"{\"lemma\":\"Drache\"}"}}]}`))
		require.NoError(t, writeErr)
	}))
	defer server.Close()
	provider, err := NewConfiguredLemmaSuggestionProvider(LLMConfig{Enabled: true, APIKey: "secret", Model: "test", BaseURL: server.URL + "/v1"}, server.Client())
	require.NoError(t, err)
	require.NotNil(t, provider)
	suggestion, err := provider.SuggestLemma(context.Background(), LemmaSuggestionRequest{
		Language: "de", Surface: "Drachen", AnalyzedLemma: "Drach", UPOS: "NOUN", Sentence: "Ein Drache sieht einen Drachen.",
		LexicalAlternative: "Drache", LexicalSource: "kaikki", LexicalVersion: "fixture-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "Drache", suggestion.Lemma)
	assert.Equal(t, "test", sent["model"])
	messages, ok := sent["messages"].([]any)
	require.True(t, ok)
	var userContent string
	for _, msg := range messages {
		item, ok := msg.(map[string]any)
		require.True(t, ok)
		if item["role"] == "user" {
			userContent, ok = item["content"].(string)
			require.True(t, ok)
		}
	}
	for _, expected := range []string{"Drachen", "Drach", "NOUN", "Ein Drache sieht einen Drachen.", "kaikki"} {
		assert.Contains(t, userContent, expected)
	}
	for _, forbidden := range []string{"owner", "book", "history", "secret"} {
		assert.NotContains(t, strings.ToLower(userContent), forbidden)
	}
}

func TestLemmaSuggestionProviderDisabledAndBounds(t *testing.T) {
	provider, err := NewConfiguredLemmaSuggestionProvider(LLMConfig{}, nil)
	require.NoError(t, err)
	assert.Nil(t, provider)
	_, err = boundLemmaSuggestionRequest(LemmaSuggestionRequest{Language: "de", Surface: "Haus", AnalyzedLemma: "haus", UPOS: "NOUN", Sentence: strings.Repeat("x", 2001)})
	assert.Error(t, err)
}
