package enrichment

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAITranslationClientPrivacyAndResponse(t *testing.T) {
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN", TargetWord: "Haus", ExampleSentence: "Das Haus ist groß."}
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, testChatResponse(input, TranslationResponse{Translation: "house", FallbackGloss: "a dwelling", SentenceTranslation: "The house is large.", SentenceTranslationTarget: "house"}))
	}))
	defer server.Close()

	client, err := NewOpenAITranslationClient(LLMConfig{APIKey: "secret", Model: "test-model", BaseURL: server.URL + "/v1"}, server.Client())
	require.NoError(t, err)
	got, err := client.Translate(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, TranslationResponse{Translation: "house", FallbackGloss: "a dwelling", SentenceTranslation: "The house is large.", SentenceTranslationTarget: "house"}, got)
	body, _ := json.Marshal(received)
	for _, forbidden := range []string{"user_id", "owner", "document", "reading", "corpus", "metadata"} {
		assert.NotContains(t, strings.ToLower(string(body)), forbidden, "request leaked %q: %s", forbidden, body)
	}
	messages := received["messages"].([]any)
	system := messages[0].(map[string]any)["content"].(string)
	assert.True(t, strings.Contains(system, "exactly one JSON object"), "prompt does not enforce concise strict JSON output: %q", system)
	assert.True(t, strings.Contains(system, "these fields"), "prompt does not enumerate the response fields: %q", system)
	assert.False(t, strings.Contains(strings.ToLower(system), "verbosity"), "prompt does not enforce concise strict JSON output: %q", system)
	user := messages[1].(map[string]any)["content"].(string)
	var externalInput map[string]any
	require.NoError(t, json.Unmarshal([]byte(user), &externalInput))
	assert.Len(t, externalInput, 7)
	assert.Equal(t, "de", externalInput["language"])
	assert.Equal(t, "en", externalInput["target_language"])
	assert.Equal(t, "Haus", externalInput["canonical_lemma"])
	assert.Equal(t, "NOUN", externalInput["upos"])
	assert.Equal(t, "Haus", externalInput["target_word"])
	assert.Equal(t, "Das Haus ist groß.", externalInput["example_sentence"])
	assert.Equal(t, float64(0), received["temperature"])
	assert.Nil(t, received["reasoning_effort"])
}

func TestOpenAITranslationClientSendsConfiguredReasoningEffortWithoutTemperature(t *testing.T) {
	requestInput := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN"}
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		_, _ = io.WriteString(w, testChatResponse(requestInput, TranslationResponse{Translation: "house", FallbackGloss: "dwelling"}))
	}))
	defer server.Close()

	client, err := NewOpenAITranslationClient(LLMConfig{
		APIKey:                  "key",
		Model:                   "self-hosted-reasoning-model",
		BaseURL:                 server.URL,
		ReasoningEffort:         "low",
		SupportsReasoningEffort: true,
	}, server.Client())
	require.NoError(t, err)
	_, err = client.Translate(context.Background(), requestInput)
	require.NoError(t, err)
	assert.Equal(t, "low", received["reasoning_effort"], "request=%v", received)
	_, present := received["temperature"]
	assert.False(t, present, "reasoning request included temperature: %v", received)
}

func TestOpenAITranslationClientRequiresContextualOutputForSentence(t *testing.T) {
	requestInput := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN", ExampleSentence: "Das Haus."}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, testChatResponse(requestInput, TranslationResponse{Translation: "house", FallbackGloss: "dwelling"}))
	}))
	defer server.Close()
	client, _ := NewOpenAITranslationClient(LLMConfig{APIKey: "key", Model: "model", BaseURL: server.URL}, server.Client())
	_, err := client.Translate(context.Background(), requestInput)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sentence_translation is empty")
}

func TestOpenAITranslationClientRedactsProviderErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"private source sentence and sk-secret"}}`)
	}))
	defer server.Close()
	client, err := NewOpenAITranslationClient(LLMConfig{APIKey: "key", Model: "model", BaseURL: server.URL}, server.Client())
	require.NoError(t, err)
	_, err = client.Translate(context.Background(), TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"})
	var httpErr *LLMHTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusBadRequest, httpErr.StatusCode)
	assert.NotContains(t, err.Error(), "private")
	assert.NotContains(t, err.Error(), "secret")
}

func TestOpenAITranslationClientLemmaOnlyOmitsSentence(t *testing.T) {
	requestInput := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN"}
	var userContent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []chatMessage `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		userContent = body.Messages[1].Content
		_, _ = io.WriteString(w, testChatResponse(requestInput, TranslationResponse{Translation: "house", FallbackGloss: "dwelling"}))
	}))
	defer server.Close()
	client, _ := NewOpenAITranslationClient(LLMConfig{APIKey: "key", Model: "model", BaseURL: server.URL}, server.Client())
	_, err := client.Translate(context.Background(), requestInput)
	require.NoError(t, err)
	assert.NotContains(t, userContent, "example_sentence", "lemma-only request included sentence field: %s", userContent)
}

func testChatResponse(input TranslationRequest, response TranslationResponse) string {
	type message struct {
		Content string `json:"content"`
	}
	type choice struct {
		Message message `json:"message"`
	}
	payload := struct {
		ItemID         string `json:"item_id"`
		SourceLanguage string `json:"source_language"`
		TargetLanguage string `json:"target_language"`
		TranslationResponse
	}{TranslationItemID(input), input.Language, "en", response}
	content, _ := json.Marshal(payload)
	body, _ := json.Marshal(struct {
		Choices []choice `json:"choices"`
	}{Choices: []choice{{Message: message{Content: string(content)}}}})
	return string(body)
}

func TestLLMRetryRateLimitAndPermanentDegradation(t *testing.T) {
	tests := []struct {
		name, statuses string
		wantCalls      int
		wantAvailable  bool
	}{
		{name: "rate limit retries", statuses: "429,200", wantCalls: 2, wantAvailable: true},
		{name: "bad request does not retry", statuses: "400", wantCalls: 1, wantAvailable: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestInput := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN"}
			statuses := strings.Split(test.statuses, ",")
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				status := statuses[min(calls, len(statuses)-1)]
				calls++
				if status != "200" {
					if status == "429" {
						w.WriteHeader(http.StatusTooManyRequests)
					} else {
						w.WriteHeader(http.StatusBadRequest)
					}
					return
				}
				_, _ = io.WriteString(w, testChatResponse(requestInput, TranslationResponse{Translation: "house", FallbackGloss: "dwelling"}))
			}))
			defer server.Close()
			client, _ := NewOpenAITranslationClient(LLMConfig{APIKey: "key", Model: "model", BaseURL: server.URL}, server.Client())
			provider, _ := NewLLMProvider("openai-compatible", "model/translation-v1", client)
			result := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 3, RetryBaseDelay: time.Nanosecond}, nil, nil, nil, provider, nil).
				Enrich(context.Background(), []Candidate{{Identity: Identity{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}}})[0]
			assert.Equal(t, test.wantCalls, calls, "result=%+v", result)
			assert.Equal(t, test.wantAvailable, result.Translation.Available, "result=%+v", result)
			if !test.wantAvailable {
				assert.Len(t, result.Warnings, 1, "expected graceful warning: %+v", result)
			}
		})
	}
}

func TestConfiguredLLMProviderAndEnvironment(t *testing.T) {
	t.Setenv("MOUSEION_LLM_ENABLED", "true")
	t.Setenv("MOUSEION_LLM_API_KEY", "secret")
	t.Setenv("MOUSEION_LLM_MODEL", "gpt-test")
	t.Setenv("MOUSEION_LLM_BASE_URL", "https://llm.example/v1/")
	t.Setenv("MOUSEION_LLM_TIMEOUT", "4s")
	t.Setenv("MOUSEION_LLM_REASONING_EFFORT", "medium")
	t.Setenv("MOUSEION_LLM_SUPPORTS_REASONING_EFFORT", "true")
	cfg, err := LLMConfigFromEnv()
	require.NoError(t, err)
	provider, err := NewConfiguredLLMProvider(cfg, &http.Client{})
	require.NoError(t, err)
	assert.Equal(t, "openai-compatible", provider.Name())
	assert.Equal(t, "gpt-test/translation-v10-sense-selection-json-reasoning-medium", provider.Version())
	assert.Equal(t, 4*time.Second, cfg.Timeout)
	assert.Equal(t, "medium", cfg.ReasoningEffort)
	assert.True(t, cfg.SupportsReasoningEffort)
}

func TestConfiguredLLMProviderRetainsSharedCacheIdentity(t *testing.T) {
	cfg := LLMConfig{Enabled: true, APIKey: "secret", Model: "gpt-test", BaseURL: "https://api.openai.com/v1"}
	provider, err := NewConfiguredLLMProvider(cfg, nil)
	require.NoError(t, err)
	codec, err := NewTranslationCodec(cfg)
	require.NoError(t, err)
	assert.Equal(t, codec.ProviderName(), provider.Name())
	assert.Equal(t, codec.ProviderVersion(), provider.Version())
}

func TestLLMConfigFromEnvDefaultsReasoningEffortToLow(t *testing.T) {
	t.Setenv("MOUSEION_LLM_REASONING_EFFORT", "")
	cfg, err := LLMConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "low", cfg.ReasoningEffort)
}

func TestLLMConfigFromEnvRejectsInvalidReasoningEffort(t *testing.T) {
	t.Setenv("MOUSEION_LLM_REASONING_EFFORT", "maximum")
	_, err := LLMConfigFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MOUSEION_LLM_REASONING_EFFORT")
	assert.Contains(t, err.Error(), "one of low, medium, or high")
}

func TestLLMConfigFromEnvRejectsInvalidReasoningSupport(t *testing.T) {
	t.Setenv("MOUSEION_LLM_SUPPORTS_REASONING_EFFORT", "sometimes")
	_, err := LLMConfigFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MOUSEION_LLM_SUPPORTS_REASONING_EFFORT must be a boolean")
}

func TestKnownReasoningModelRequiresOpenAIEndpoint(t *testing.T) {
	openAIURL, _ := url.Parse("https://api.openai.com/v1")
	customURL, _ := url.Parse("https://llm.example/v1")
	assert.True(t, knownReasoningModel(openAIURL, "o3-mini"), "o3-mini should be recognized at the OpenAI endpoint")
	assert.False(t, knownReasoningModel(customURL, "o3-mini"), "o3-mini should not be assumed supported at an unknown endpoint")
}
