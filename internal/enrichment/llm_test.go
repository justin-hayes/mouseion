package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/testwrite"
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
		testwrite.String(t, w, testChatResponse(input, TranslationResponse{Translation: "house", FallbackGloss: "a dwelling", SentenceTranslation: "The house is large.", SentenceTranslationTargets: []string{"house"}}))
	}))
	defer server.Close()

	client, err := NewOpenAITranslationClient(LLMConfig{APIKey: "secret", Model: "test-model", BaseURL: server.URL + "/v1"}, server.Client())
	require.NoError(t, err)
	got, err := client.Translate(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, TranslationResponse{Translation: "house", FallbackGloss: "a dwelling", SentenceTranslation: "The house is large.", SentenceTranslationTargets: []string{"house"}}, got)
	body, err := json.Marshal(received)
	require.NoError(t, err)
	for _, forbidden := range []string{"user_id", "owner", "document", "reading", "corpus", "metadata"} {
		assert.NotContains(t, strings.ToLower(string(body)), forbidden, "request leaked %q: %s", forbidden, body)
	}
	messages, ok := received["messages"].([]any)
	require.True(t, ok)
	systemMessage, ok := messages[0].(map[string]any)
	require.True(t, ok)
	system, ok := systemMessage["content"].(string)
	require.True(t, ok)
	assert.Contains(t, system, "exactly one JSON object", "prompt does not enforce concise strict JSON output: %q", system)
	assert.Contains(t, system, "using only these eight field names", "prompt does not enumerate the response fields: %q", system)
	assert.NotContains(t, strings.ToLower(system), "verbosity", "prompt does not enforce concise strict JSON output: %q", system)
	userMessage, ok := messages[1].(map[string]any)
	require.True(t, ok)
	user, ok := userMessage["content"].(string)
	require.True(t, ok)
	var externalInput map[string]any
	require.NoError(t, json.Unmarshal([]byte(user), &externalInput))
	assert.Len(t, externalInput, 7)
	assert.Equal(t, "de", externalInput["language"])
	assert.Equal(t, "en", externalInput["target_language"])
	assert.Equal(t, "Haus", externalInput["canonical_lemma"])
	assert.Equal(t, "NOUN", externalInput["upos"])
	assert.Equal(t, "Haus", externalInput["target_word"])
	assert.Equal(t, "Das Haus ist groß.", externalInput["example_sentence"])
	// Temperature is configured as exactly 0; 1e-9 only absorbs float64 round-trip noise on a [0, 2] scale.
	assert.InDelta(t, float64(0), received["temperature"], 1e-9)
	assert.Nil(t, received["reasoning_effort"])
}

func TestOpenAITranslationClientSendsConfiguredReasoningEffortWithoutTemperature(t *testing.T) {
	requestInput := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN"}
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		testwrite.String(t, w, testChatResponse(requestInput, TranslationResponse{Translation: "house", FallbackGloss: "dwelling"}))
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
		testwrite.String(t, w, testChatResponse(requestInput, TranslationResponse{Translation: "house", FallbackGloss: "dwelling"}))
	}))
	defer server.Close()
	client, err := NewOpenAITranslationClient(LLMConfig{APIKey: "key", Model: "model", BaseURL: server.URL}, server.Client())
	require.NoError(t, err)
	_, err = client.Translate(context.Background(), requestInput)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sentence_translation is empty")
}

func TestOpenAITranslationClientRedactsProviderErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		testwrite.String(t, w, `{"error":{"message":"private source sentence and sk-secret"}}`)
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
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if assert.Len(t, body.Messages, 2) {
			userContent = body.Messages[1].Content
		}
		testwrite.String(t, w, testChatResponse(requestInput, TranslationResponse{Translation: "house", FallbackGloss: "dwelling"}))
	}))
	defer server.Close()
	client, err := NewOpenAITranslationClient(LLMConfig{APIKey: "key", Model: "model", BaseURL: server.URL}, server.Client())
	require.NoError(t, err)
	_, err = client.Translate(context.Background(), requestInput)
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
	content, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("test chat payload is not JSON encodable: %v", err))
	}
	body, err := json.Marshal(struct {
		Choices []choice `json:"choices"`
	}{Choices: []choice{{Message: message{Content: string(content)}}}})
	if err != nil {
		panic(fmt.Sprintf("test chat response is not JSON encodable: %v", err))
	}
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
				testwrite.String(t, w, testChatResponse(requestInput, TranslationResponse{Translation: "house", FallbackGloss: "dwelling"}))
			}))
			defer server.Close()
			client, err := NewOpenAITranslationClient(LLMConfig{APIKey: "key", Model: "model", BaseURL: server.URL}, server.Client())
			require.NoError(t, err)
			provider, err := NewLLMProvider("openai-compatible", "model/translation-v1", client)
			require.NoError(t, err)
			result := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 3, RetryBaseDelay: time.Nanosecond}, nil, nil, nil, provider, nil).
				Enrich(context.Background(), []Candidate{{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}})[0]
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
	assert.Equal(t, "gpt-test/translation-v12-structured-target-alignment-reasoning-medium", provider.Version())
	assert.Equal(t, 4*time.Second, cfg.Timeout)
	assert.Equal(t, "medium", cfg.ReasoningEffort)
	assert.True(t, cfg.SupportsReasoningEffort)
}

func TestConfiguredLLMProviderDisabledIsExplicitNoOp(t *testing.T) {
	provider, err := NewConfiguredLLMProvider(LLMConfig{}, nil)
	require.NoError(t, err)
	require.NotNil(t, provider)
	assert.False(t, provider.Enabled())
	assert.Equal(t, "disabled", provider.Name())
	assert.Equal(t, "disabled", provider.Version())
	_, err = provider.Translate(context.Background(), TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN"})
	require.Error(t, err)
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, provider, nil)
	assert.False(t, service.ExternalConfigured())
}

func TestConfiguredLLMProviderRetainsSharedCacheIdentity(t *testing.T) {
	cfg := LLMConfig{Enabled: true, APIKey: "secret", Model: "gpt-test", BaseURL: "https://api.openai.com/v1"}
	provider, err := NewConfiguredLLMProvider(cfg, nil)
	require.NoError(t, err)
	codec, err := NewTranslationCodec(cfg)
	require.NoError(t, err)
	assert.Equal(t, codec.ProviderName(), provider.Name())
	assert.Equal(t, codec.ProviderVersion(), provider.Version())
	assert.Equal(t, "gpt-test/translation-v14-structured-target-alignment", codec.ContextualGlossProviderVersion())
	assert.NotEqual(t, codec.ProviderVersion(), codec.ContextualGlossProviderVersion())
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

func TestLLMConfigFromEnvRejectsInvalidEnabledFlag(t *testing.T) {
	t.Setenv("MOUSEION_LLM_ENABLED", "sometimes")
	_, err := LLMConfigFromEnv()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MOUSEION_LLM_ENABLED must be a boolean")
}

func TestLLMConfigFromEnvRejectsNonPositiveTimeout(t *testing.T) {
	for _, value := range []string{"soon", "0s", "-1s"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("MOUSEION_LLM_TIMEOUT", value)
			_, err := LLMConfigFromEnv()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "MOUSEION_LLM_TIMEOUT must be a positive Go duration")
		})
	}
}

func TestLLMConfigFromEnvRequiresCredentialsWhenEnabled(t *testing.T) {
	t.Setenv("MOUSEION_LLM_ENABLED", "true")
	t.Run("requires an API key", func(t *testing.T) {
		t.Setenv("MOUSEION_LLM_API_KEY", "")
		_, err := LLMConfigFromEnv()
		require.ErrorContains(t, err, "MOUSEION_LLM_API_KEY is required when MOUSEION_LLM_ENABLED is true")
	})
	t.Run("requires a model", func(t *testing.T) {
		t.Setenv("MOUSEION_LLM_API_KEY", "secret")
		t.Setenv("MOUSEION_LLM_MODEL", "")
		_, err := LLMConfigFromEnv()
		require.ErrorContains(t, err, "MOUSEION_LLM_MODEL is required when MOUSEION_LLM_ENABLED is true")
	})
}

func TestKnownReasoningModelRequiresOpenAIEndpoint(t *testing.T) {
	openAIURL, err := url.Parse("https://api.openai.com/v1")
	require.NoError(t, err)
	customURL, err := url.Parse("https://llm.example/v1")
	require.NoError(t, err)
	assert.True(t, knownReasoningModel(openAIURL, "o3-mini"), "o3-mini should be recognized at the OpenAI endpoint")
	assert.False(t, knownReasoningModel(customURL, "o3-mini"), "o3-mini should not be assumed supported at an unknown endpoint")
}
