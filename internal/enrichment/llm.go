package enrichment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLLMBaseURL      = "https://api.openai.com/v1"
	defaultLLMTimeout      = 30 * time.Second
	defaultReasoningEffort = "low"
	llmPromptVersion       = "translation-v6-target-alignment-json"
	llmReasoningEffortEnv  = "MOUSEION_LLM_REASONING_EFFORT"
	llmReasoningSupportEnv = "MOUSEION_LLM_SUPPORTS_REASONING_EFFORT"
)

// LLMConfig is the administrator-controlled configuration for the external
// translation provider. User consent remains a separate EnrichmentConfig flag.
type LLMConfig struct {
	Enabled                 bool
	APIKey                  string
	Model                   string
	BaseURL                 string
	Timeout                 time.Duration
	ReasoningEffort         string
	SupportsReasoningEffort bool
}

// LLMConfigFromEnv reads MOUSEION_LLM_ENABLED, MOUSEION_LLM_API_KEY,
// MOUSEION_LLM_MODEL, MOUSEION_LLM_BASE_URL, MOUSEION_LLM_TIMEOUT,
// MOUSEION_LLM_REASONING_EFFORT, and MOUSEION_LLM_SUPPORTS_REASONING_EFFORT.
func LLMConfigFromEnv() (LLMConfig, error) {
	cfg := LLMConfig{
		APIKey:          strings.TrimSpace(os.Getenv("MOUSEION_LLM_API_KEY")),
		Model:           strings.TrimSpace(os.Getenv("MOUSEION_LLM_MODEL")),
		BaseURL:         strings.TrimSpace(os.Getenv("MOUSEION_LLM_BASE_URL")),
		Timeout:         defaultLLMTimeout,
		ReasoningEffort: defaultReasoningEffort,
	}
	if value := strings.TrimSpace(os.Getenv("MOUSEION_LLM_ENABLED")); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return LLMConfig{}, fmt.Errorf("MOUSEION_LLM_ENABLED must be a boolean: %w", err)
		}
		cfg.Enabled = enabled
	}
	if value := strings.TrimSpace(os.Getenv("MOUSEION_LLM_TIMEOUT")); value != "" {
		timeout, err := time.ParseDuration(value)
		if err != nil || timeout <= 0 {
			return LLMConfig{}, fmt.Errorf("MOUSEION_LLM_TIMEOUT must be a positive Go duration: %q", value)
		}
		cfg.Timeout = timeout
	}
	if value := strings.TrimSpace(os.Getenv(llmReasoningEffortEnv)); value != "" {
		reasoningEffort, err := parseReasoningEffort(value)
		if err != nil {
			return LLMConfig{}, fmt.Errorf("%s %w", llmReasoningEffortEnv, err)
		}
		cfg.ReasoningEffort = reasoningEffort
	}
	if value := strings.TrimSpace(os.Getenv(llmReasoningSupportEnv)); value != "" {
		supports, err := strconv.ParseBool(value)
		if err != nil {
			return LLMConfig{}, fmt.Errorf("%s must be a boolean: %w", llmReasoningSupportEnv, err)
		}
		cfg.SupportsReasoningEffort = supports
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultLLMBaseURL
	}
	if cfg.Enabled {
		if cfg.APIKey == "" {
			return LLMConfig{}, errors.New("MOUSEION_LLM_API_KEY is required when MOUSEION_LLM_ENABLED is true")
		}
		if cfg.Model == "" {
			return LLMConfig{}, errors.New("MOUSEION_LLM_MODEL is required when MOUSEION_LLM_ENABLED is true")
		}
	}
	return cfg, nil
}

// NewConfiguredLLMProvider returns nil when the administrator has disabled
// external LLM translation. The caller must still pass the user's opt-in to
// EnrichmentConfig; Service is the final policy boundary for every request.
func NewConfiguredLLMProvider(cfg LLMConfig, client *http.Client) (TranslationProvider, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	llm, err := NewOpenAITranslationClient(cfg, client)
	if err != nil {
		return nil, err
	}
	version := cfg.Model + "/" + llmPromptVersion
	if llm.usesReasoningEffort {
		version += "-reasoning-" + llm.reasoningEffort
	}
	return NewLLMProvider("openai-compatible", version, llm)
}

// OpenAITranslationClient implements the OpenAI-compatible Chat Completions
// protocol. Its request type accepts only TranslationRequest, making enrichment
// identity and reading metadata impossible to pass through this boundary.
type OpenAITranslationClient struct {
	apiKey, model, endpoint string
	reasoningEffort         string
	usesReasoningEffort     bool
	httpClient              *http.Client
}

func NewOpenAITranslationClient(cfg LLMConfig, client *http.Client) (*OpenAITranslationClient, error) {
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("enrichment: LLM API key and model are required")
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = defaultLLMBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("enrichment: invalid LLM base URL %q", base)
	}
	reasoningEffort := cfg.ReasoningEffort
	if reasoningEffort == "" {
		reasoningEffort = defaultReasoningEffort
	}
	normalizedReasoningEffort, err := parseReasoningEffort(reasoningEffort)
	if err != nil {
		return nil, fmt.Errorf("enrichment: %s %w", llmReasoningEffortEnv, err)
	}
	reasoningEffort = normalizedReasoningEffort
	usesReasoningEffort := cfg.SupportsReasoningEffort || knownReasoningModel(parsed, cfg.Model)
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = defaultLLMTimeout
		}
		client = &http.Client{Timeout: timeout}
	}
	return &OpenAITranslationClient{
		apiKey:              cfg.APIKey,
		model:               cfg.Model,
		endpoint:            base + "/chat/completions",
		reasoningEffort:     reasoningEffort,
		usesReasoningEffort: usesReasoningEffort,
		httpClient:          client,
	}, nil
}

type chatRequest struct {
	Model           string        `json:"model"`
	Messages        []chatMessage `json:"messages"`
	Temperature     *int          `json:"temperature,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
	ResponseFormat  struct {
		Type string `json:"type"`
	} `json:"response_format"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (c *OpenAITranslationClient) Translate(ctx context.Context, input TranslationRequest) (TranslationResponse, error) {
	privateInput, err := json.Marshal(struct {
		Language        string `json:"language"`
		CanonicalLemma  string `json:"canonical_lemma"`
		UPOS            string `json:"upos"`
		TargetWord      string `json:"target_word,omitempty"`
		ExampleSentence string `json:"example_sentence,omitempty"`
	}{input.Language, input.CanonicalLemma, input.UPOS, input.TargetWord, input.ExampleSentence})
	if err != nil {
		return TranslationResponse{}, fmt.Errorf("encode LLM translation input: %w", err)
	}
	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: "Translate the supplied lemma into English. Return exactly one JSON object with exactly these four string fields and no markdown or additional keys: translation (a concise lemma translation), gloss (a brief sense explanation), sentence_translation (a natural translation of the complete example sentence), and sentence_translation_target (the plain-text English word or phrase corresponding to the supplied target in sentence_translation, or an empty string when there is no reliable literal correspondence). When no example sentence is supplied, sentence_translation and sentence_translation_target must be empty strings. Do not return HTML or markup in any field."},
			{Role: "user", Content: string(privateInput)},
		},
	}
	if c.usesReasoningEffort {
		payload.ReasoningEffort = c.reasoningEffort
	} else {
		temperature := 0
		payload.Temperature = &temperature
	}
	payload.ResponseFormat.Type = "json_object"
	body, err := json.Marshal(payload)
	if err != nil {
		return TranslationResponse{}, fmt.Errorf("encode LLM request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return TranslationResponse{}, fmt.Errorf("create LLM request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TranslationResponse{}, fmt.Errorf("call LLM: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return TranslationResponse{}, &LLMHTTPError{StatusCode: resp.StatusCode, Message: strings.TrimSpace(string(message))}
	}
	var decoded struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&decoded); err != nil {
		return TranslationResponse{}, fmt.Errorf("decode LLM response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return TranslationResponse{}, errors.New("decode LLM response: no choices")
	}
	var result TranslationResponse
	resultDecoder := json.NewDecoder(strings.NewReader(decoded.Choices[0].Message.Content))
	resultDecoder.DisallowUnknownFields()
	if err := resultDecoder.Decode(&result); err != nil {
		return TranslationResponse{}, fmt.Errorf("decode LLM translation: %w", err)
	}
	if err := resultDecoder.Decode(&struct{}{}); err != io.EOF {
		return TranslationResponse{}, errors.New("decode LLM translation: trailing JSON content")
	}
	result.Translation = strings.TrimSpace(result.Translation)
	result.Gloss = strings.TrimSpace(result.Gloss)
	result.SentenceTranslation = strings.TrimSpace(result.SentenceTranslation)
	result.SentenceTranslationTarget = strings.TrimSpace(result.SentenceTranslationTarget)
	if result.Translation == "" {
		return TranslationResponse{}, errors.New("decode LLM translation: translation is empty")
	}
	if input.ExampleSentence != "" && result.SentenceTranslation == "" {
		return TranslationResponse{}, errors.New("decode LLM translation: sentence_translation is empty")
	}
	return result, nil
}

func parseReasoningEffort(value string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(value)); normalized {
	case "low", "medium", "high":
		return normalized, nil
	default:
		return "", fmt.Errorf("must be one of low, medium, or high; got %q", value)
	}
}

func knownReasoningModel(baseURL *url.URL, model string) bool {
	if strings.ToLower(strings.TrimSuffix(baseURL.Hostname(), ".")) != "api.openai.com" {
		return false
	}
	model = strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range []string{"o1", "o3", "o4", "gpt-5", "gpt-oss"} {
		if model == prefix || strings.HasPrefix(model, prefix+"-") || strings.HasPrefix(model, prefix+".") {
			return !strings.Contains(model, "-pro")
		}
	}
	return false
}

// LLMHTTPError reports whether an API response is safe to retry.
type LLMHTTPError struct {
	StatusCode int
	Message    string
}

func (e *LLMHTTPError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("LLM API returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("LLM API returned HTTP %d: %s", e.StatusCode, e.Message)
}

func (e *LLMHTTPError) Temporary() bool {
	return e.StatusCode == http.StatusRequestTimeout || e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}
