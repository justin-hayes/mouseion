package enrichment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

const llmSystemPrompt = "Translate the supplied lemma into English. Return exactly one JSON object with exactly these four string fields and no markdown or additional keys: translation (a concise lemma translation), gloss (a brief sense explanation), sentence_translation (a natural translation of the complete example sentence), and sentence_translation_target (the plain-text English word or phrase corresponding to the supplied target in sentence_translation, or an empty string when there is no reliable literal correspondence). When no example sentence is supplied, sentence_translation and sentence_translation_target must be empty strings. Do not return HTML or markup in any field."

const maxTranslationResponseBytes = 1 << 20

// TranslationCodec owns the provider request and response semantics shared by
// synchronous and Batch transports. It deliberately contains no credentials,
// endpoint, HTTP client, or durable preparation identity.
type TranslationCodec struct {
	model               string
	reasoningEffort     string
	usesReasoningEffort bool
}

// Model returns the model frozen into this codec. Callers use it to verify
// that durable work is not silently rebound to current configuration.
func (c *TranslationCodec) Model() string {
	if c == nil {
		return ""
	}
	return c.model
}

// ProviderName returns the stable provider name used by the existing
// translation-cache identity. Batch and synchronous callers must share this
// value so changing transport does not invalidate or mix cache entries.
func (c *TranslationCodec) ProviderName() string { return "openai-compatible" }

// ProviderVersion returns the stable version used by the existing
// translation-cache identity.
func (c *TranslationCodec) ProviderVersion() string {
	if c == nil {
		return ""
	}
	version := c.model + "/" + llmPromptVersion
	if c.usesReasoningEffort {
		version += "-reasoning-" + c.reasoningEffort
	}
	return version
}

// PromptVersion identifies the request/validation contract frozen into this
// codec. It is recorded by the operator validation report.
func (c *TranslationCodec) PromptVersion() string { return llmPromptVersion }

// NewTranslationCodec builds the shared Chat Completions codec. BaseURL is
// consulted only to preserve the existing reasoning-model capability policy;
// it is not retained by the codec.
func NewTranslationCodec(cfg LLMConfig) (*TranslationCodec, error) {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, errors.New("enrichment: LLM model is required")
	}
	_, parsed, err := parseLLMBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	reasoningEffort := cfg.ReasoningEffort
	if reasoningEffort == "" {
		reasoningEffort = defaultReasoningEffort
	}
	reasoningEffort, err = parseReasoningEffort(reasoningEffort)
	if err != nil {
		return nil, fmt.Errorf("enrichment: %s %w", llmReasoningEffortEnv, err)
	}
	return &TranslationCodec{
		model:               model,
		reasoningEffort:     reasoningEffort,
		usesReasoningEffort: cfg.SupportsReasoningEffort || knownReasoningModel(parsed, model),
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

// EncodeRequest returns the canonical Chat Completions body for one approved
// external-provider input.
func (c *TranslationCodec) EncodeRequest(input TranslationRequest) ([]byte, error) {
	if c == nil {
		return nil, errors.New("encode LLM request: nil translation codec")
	}
	privateInput, err := json.Marshal(struct {
		Language        string `json:"language"`
		CanonicalLemma  string `json:"canonical_lemma"`
		UPOS            string `json:"upos"`
		TargetWord      string `json:"target_word,omitempty"`
		ExampleSentence string `json:"example_sentence,omitempty"`
	}{input.Language, input.CanonicalLemma, input.UPOS, input.TargetWord, input.ExampleSentence})
	if err != nil {
		return nil, fmt.Errorf("encode LLM translation input: %w", err)
	}
	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: llmSystemPrompt},
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
		return nil, fmt.Errorf("encode LLM request: %w", err)
	}
	return body, nil
}

// DecodeResponse applies the shared Chat Completions and translation quality
// validation to a bounded provider response body.
func (c *TranslationCodec) DecodeResponse(input TranslationRequest, body []byte) (TranslationResponse, error) {
	result, _, err := c.DecodeResponseWithUsage(input, body)
	return result, err
}

// DecodeResponseWithUsage applies the same decoder and validator as
// DecodeResponse while returning provider-reported usage for evaluation.
func (c *TranslationCodec) DecodeResponseWithUsage(input TranslationRequest, body []byte) (TranslationResponse, TranslationUsage, error) {
	if c == nil {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM response: nil translation codec")
	}
	if len(body) > maxTranslationResponseBytes {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM response: response exceeds 1 MiB")
	}
	var decoded struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
		Usage TranslationUsage `json:"usage"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&decoded); err != nil {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("decode LLM response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM response: trailing JSON content")
	}
	if len(decoded.Choices) == 0 {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM response: no choices")
	}
	var result TranslationResponse
	resultDecoder := json.NewDecoder(strings.NewReader(decoded.Choices[0].Message.Content))
	resultDecoder.DisallowUnknownFields()
	if err := resultDecoder.Decode(&result); err != nil {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("decode LLM translation: %w", err)
	}
	if err := resultDecoder.Decode(&struct{}{}); err != io.EOF {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM translation: trailing JSON content")
	}
	result.Translation = strings.TrimSpace(result.Translation)
	result.Gloss = strings.TrimSpace(result.Gloss)
	result.SentenceTranslation = strings.TrimSpace(result.SentenceTranslation)
	result.SentenceTranslationTarget = strings.TrimSpace(result.SentenceTranslationTarget)
	if result.Translation == "" {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM translation: translation is empty")
	}
	if input.ExampleSentence != "" && result.SentenceTranslation == "" {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM translation: sentence_translation is empty")
	}
	if decoded.Usage.PromptTokens < 0 || decoded.Usage.CompletionTokens < 0 || decoded.Usage.TotalTokens < 0 {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM response: invalid usage")
	}
	return result, decoded.Usage, nil
}

func parseLLMBaseURL(value string) (string, *url.URL, error) {
	base := strings.TrimRight(strings.TrimSpace(value), "/")
	if base == "" {
		base = defaultLLMBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", nil, errors.New("enrichment: invalid LLM base URL")
	}
	return base, parsed, nil
}
