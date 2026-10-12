package enrichment

import (
	"bytes"
	"context"
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
	defaultLLMBaseURL            = "https://api.openai.com/v1"
	defaultLLMTimeout            = 30 * time.Second
	defaultReasoningEffort       = "low"
	llmPromptVersion             = "translation-v12-structured-target-alignment"
	contextualGlossPromptVersion = "translation-v14-structured-target-alignment"
	llmReasoningEffortEnv        = "MOUSEION_LLM_REASONING_EFFORT"
	llmReasoningSupportEnv       = "MOUSEION_LLM_SUPPORTS_REASONING_EFFORT"
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
	if err := parseLLMBoolEnv("MOUSEION_LLM_ENABLED", &cfg.Enabled); err != nil {
		return LLMConfig{}, err
	}
	if err := parseLLMTimeoutEnv(&cfg.Timeout); err != nil {
		return LLMConfig{}, err
	}
	if err := parseLLMReasoningEffortEnv(&cfg.ReasoningEffort); err != nil {
		return LLMConfig{}, err
	}
	if err := parseLLMBoolEnv(llmReasoningSupportEnv, &cfg.SupportsReasoningEffort); err != nil {
		return LLMConfig{}, err
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultLLMBaseURL
	}
	if err := cfg.validateEnabled(); err != nil {
		return LLMConfig{}, err
	}
	return cfg, nil
}

// parseLLMBoolEnv parses one optional boolean variable into dst. An unset or
// blank value leaves dst unchanged.
func parseLLMBoolEnv(name string, dst *bool) error {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("%s must be a boolean: %w", name, err)
	}
	*dst = parsed
	return nil
}

// parseLLMTimeoutEnv parses the optional positive provider timeout into dst.
func parseLLMTimeoutEnv(dst *time.Duration) error {
	value := strings.TrimSpace(os.Getenv("MOUSEION_LLM_TIMEOUT"))
	if value == "" {
		return nil
	}
	timeout, err := time.ParseDuration(value)
	if err != nil || timeout <= 0 {
		return fmt.Errorf("MOUSEION_LLM_TIMEOUT must be a positive Go duration: %q", value)
	}
	*dst = timeout
	return nil
}

// parseLLMReasoningEffortEnv parses the optional reasoning-effort selector.
func parseLLMReasoningEffortEnv(dst *string) error {
	value := strings.TrimSpace(os.Getenv(llmReasoningEffortEnv))
	if value == "" {
		return nil
	}
	reasoningEffort, err := parseReasoningEffort(value)
	if err != nil {
		return fmt.Errorf("%s %w", llmReasoningEffortEnv, err)
	}
	*dst = reasoningEffort
	return nil
}

// validateEnabled requires the credentials an enabled provider needs before it
// can reach the network.
func (cfg LLMConfig) validateEnabled() error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.APIKey == "" {
		return errors.New("MOUSEION_LLM_API_KEY is required when MOUSEION_LLM_ENABLED is true")
	}
	if cfg.Model == "" {
		return errors.New("MOUSEION_LLM_MODEL is required when MOUSEION_LLM_ENABLED is true")
	}
	return nil
}

// NewConfiguredLLMProvider returns a DisabledTranslationProvider when the
// administrator has turned external LLM translation off. The caller must still
// pass the user's opt-in to EnrichmentConfig; Service is the final policy
// boundary for every request.
func NewConfiguredLLMProvider(cfg LLMConfig, client *http.Client) (TranslationProvider, error) {
	if !cfg.Enabled {
		return DisabledTranslationProvider{}, nil
	}
	llm, err := NewOpenAITranslationClient(cfg, client)
	if err != nil {
		return nil, err
	}
	return NewLLMProvider(llm.codec.ProviderName(), llm.codec.ProviderVersion(), llm)
}

var errExternalTranslationDisabled = errors.New("enrichment: external translation is disabled")

// DisabledTranslationProvider is the translation provider used while external
// translation is administratively off. It reports Enabled as false and refuses
// to translate, so a caller that skips the enabled query fails instead of
// reaching the network.
type DisabledTranslationProvider struct{}

func (DisabledTranslationProvider) Name() string    { return "disabled" }
func (DisabledTranslationProvider) Version() string { return "disabled" }
func (DisabledTranslationProvider) Enabled() bool   { return false }
func (DisabledTranslationProvider) Translate(context.Context, TranslationRequest) (TranslationResponse, error) {
	return TranslationResponse{}, errExternalTranslationDisabled
}

// OpenAITranslationClient implements the OpenAI-compatible Chat Completions
// protocol. Its request type accepts only TranslationRequest, making enrichment
// identity and reading metadata impossible to pass through this boundary.
type OpenAITranslationClient struct {
	apiKey, model, endpoint string
	reasoningEffort         string
	usesReasoningEffort     bool
	codec                   *TranslationCodec
	httpClient              *http.Client
}

func NewOpenAITranslationClient(cfg LLMConfig, client *http.Client) (*OpenAITranslationClient, error) {
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("enrichment: LLM API key and model are required")
	}
	base, _, err := parseLLMBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	codec, err := NewTranslationCodec(cfg)
	if err != nil {
		return nil, err
	}
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
		reasoningEffort:     codec.reasoningEffort,
		usesReasoningEffort: codec.usesReasoningEffort,
		codec:               codec,
		httpClient:          client,
	}, nil
}

func (c *OpenAITranslationClient) Translate(ctx context.Context, input TranslationRequest) (TranslationResponse, error) {
	result, _, err := c.TranslateWithUsage(ctx, input)
	return result, err
}

// TranslateWithUsage is the observed form used only by the explicit
// validation harness. Normal enrichment deliberately keeps usage out of its
// semantic result and cache identity.
func (c *OpenAITranslationClient) TranslateWithUsage(ctx context.Context, input TranslationRequest) (response TranslationResponse, usage TranslationUsage, err error) {
	body, err := c.codec.EncodeRequest(input)
	if err != nil {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("encode LLM request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("create LLM request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("call LLM: %w", err)
	}
	defer func() { err = errors.Join(err, resp.Body.Close()) }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var responseErr error = &LLMHTTPError{StatusCode: resp.StatusCode}
		if _, copyErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10)); copyErr != nil {
			responseErr = errors.Join(responseErr, fmt.Errorf("drain LLM response: %w", copyErr))
		}
		return TranslationResponse{}, TranslationUsage{}, responseErr
	}
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxTranslationResponseBytes+1))
	if err != nil {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("decode LLM response: %w", err)
	}
	return c.codec.DecodeResponseWithUsage(input, responseBody)
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
	// Message is retained only for compatibility with existing constructors;
	// provider bodies may contain prompts, responses, or other private data and
	// must never be emitted in an error or warning.
	return fmt.Sprintf("LLM API returned HTTP %d", e.StatusCode)
}

func (e *LLMHTTPError) Temporary() bool {
	return e.StatusCode == http.StatusRequestTimeout || e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}
