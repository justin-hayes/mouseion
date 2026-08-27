package enrichment

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAITranslationClientPrivacyAndResponse(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("request path=%q authorization=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"{\"translation\":\"house\",\"gloss\":\"a dwelling\",\"sentence_translation\":\"The house is large.\",\"context_sentence\":\"Das Haus ist\"}"}}]}`)
	}))
	defer server.Close()

	client, err := NewOpenAITranslationClient(LLMConfig{APIKey: "secret", Model: "test-model", BaseURL: server.URL + "/v1"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Translate(context.Background(), TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", ExampleSentence: "Das Haus ist groß."})
	if err != nil || got != (TranslationResponse{Translation: "house", Gloss: "a dwelling", SentenceTranslation: "The house is large.", ContextSentence: "Das Haus ist"}) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	body, _ := json.Marshal(received)
	for _, forbidden := range []string{"user_id", "owner", "document", "reading", "corpus", "metadata"} {
		if strings.Contains(strings.ToLower(string(body)), forbidden) {
			t.Fatalf("request leaked %q: %s", forbidden, body)
		}
	}
	messages := received["messages"].([]any)
	user := messages[1].(map[string]any)["content"].(string)
	var input map[string]any
	if err := json.Unmarshal([]byte(user), &input); err != nil {
		t.Fatal(err)
	}
	if len(input) != 4 || input["language"] != "de" || input["canonical_lemma"] != "Haus" || input["upos"] != "NOUN" || input["example_sentence"] != "Das Haus ist groß." {
		t.Fatalf("external input=%v", input)
	}
}

func TestOpenAITranslationClientRequiresContextualOutputForSentence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{\"translation\":\"house\",\"gloss\":\"dwelling\"}"}}]}`)
	}))
	defer server.Close()
	client, _ := NewOpenAITranslationClient(LLMConfig{APIKey: "key", Model: "model", BaseURL: server.URL}, server.Client())
	_, err := client.Translate(context.Background(), TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", ExampleSentence: "Das Haus."})
	if err == nil || !strings.Contains(err.Error(), "sentence_translation is empty") {
		t.Fatalf("err=%v", err)
	}
}

func TestOpenAITranslationClientLemmaOnlyOmitsSentence(t *testing.T) {
	var userContent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []chatMessage `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		userContent = body.Messages[1].Content
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{\"translation\":\"house\",\"gloss\":\"\"}"}}]}`)
	}))
	defer server.Close()
	client, _ := NewOpenAITranslationClient(LLMConfig{APIKey: "key", Model: "model", BaseURL: server.URL}, server.Client())
	if _, err := client.Translate(context.Background(), TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(userContent, "example_sentence") {
		t.Fatalf("lemma-only request included sentence field: %s", userContent)
	}
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
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"{\"translation\":\"house\",\"gloss\":\"dwelling\"}"}}]}`)
			}))
			defer server.Close()
			client, _ := NewOpenAITranslationClient(LLMConfig{APIKey: "key", Model: "model", BaseURL: server.URL}, server.Client())
			provider, _ := NewLLMProvider("openai-compatible", "model/translation-v1", client)
			result := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 3, RetryBaseDelay: time.Nanosecond}, nil, nil, nil, provider, nil).
				Enrich(context.Background(), []Candidate{{Identity: Identity{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}}})[0]
			if calls != test.wantCalls || result.Translation.Available != test.wantAvailable {
				t.Fatalf("calls=%d result=%+v", calls, result)
			}
			if !test.wantAvailable && len(result.Warnings) != 1 {
				t.Fatalf("expected graceful warning: %+v", result)
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
	cfg, err := LLMConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewConfiguredLLMProvider(cfg, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Name() != "openai-compatible" || provider.Version() != "gpt-test/translation-v3" || cfg.Timeout != 4*time.Second {
		t.Fatalf("provider=%s/%s config=%+v", provider.Name(), provider.Version(), cfg)
	}
}
