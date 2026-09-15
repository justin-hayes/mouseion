package enrichment

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

var llmSystemPrompt = fmt.Sprintf("Translate the supplied lemma into the target language. Return exactly one JSON object using only these eight field names, with no markdown or additional keys: item_id, source_language, target_language, translation (a concise lemma translation), sentence_translation (a natural translation of the complete example sentence), sentence_translation_target (the plain-text target-language word or phrase corresponding to the supplied target in sentence_translation, or an empty string when there is no reliable literal correspondence), sense_order (an optional array of distinct 0-based integer indices into the frozen candidate_senses list, in best-fit order; return at most %d indices, capped at the display limit of %d; omit it, or return an empty array, when no candidate sense fits), and fallback_gloss (an optional concise English gloss only when no candidate sense fits or the candidate list is empty). Echo item_id and both languages exactly. When no example sentence is supplied, sentence_translation and sentence_translation_target must be empty strings. Do not return HTML or markup in any field.", DefaultMaxSenses, DefaultMaxSenses)

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

// TranslationItemID returns the stable opaque identity used by synchronous
// requests. It contains no owner identity or source text.
func TranslationItemID(input TranslationRequest) string {
	// The digest is deliberately opaque and contains no owner or source text.
	// Hashing the sentence keeps distinct contextual items distinct without
	// placing the sentence itself in provider-visible identity.
	sentenceDigest := sha256.Sum256([]byte(input.ExampleSentence))
	canonical := strings.Join([]string{input.Language, translationTargetLanguage(input), input.CanonicalLemma, input.UPOS, input.TargetWord, fmt.Sprintf("%x", sentenceDigest)}, "\x00")
	digest := sha256.Sum256([]byte(canonical))
	return "translation-item-" + fmt.Sprintf("%x", digest[:16])
}

func translationTargetLanguage(input TranslationRequest) string {
	if strings.TrimSpace(input.TargetLanguage) == "" {
		return "en"
	}
	return strings.TrimSpace(input.TargetLanguage)
}

// EncodeRequest returns the canonical Chat Completions body for one approved
// external-provider input.
func (c *TranslationCodec) EncodeRequest(input TranslationRequest) ([]byte, error) {
	return c.encodeRequest(input, TranslationItemID(input))
}

// EncodeBatchRequestForValidation is used by the offline harness to reproduce
// the exact request body whose echoed item ID is the Batch custom ID.
func (c *TranslationCodec) EncodeBatchRequestForValidation(input TranslationRequest, itemID string) ([]byte, error) {
	return c.encodeRequest(input, itemID)
}

func (c *TranslationCodec) encodeRequest(input TranslationRequest, itemID string) ([]byte, error) {
	if c == nil {
		return nil, errors.New("encode LLM request: nil translation codec")
	}
	if strings.TrimSpace(input.Language) == "" || strings.TrimSpace(translationTargetLanguage(input)) == "" || strings.TrimSpace(input.CanonicalLemma) == "" || strings.TrimSpace(input.UPOS) == "" || strings.TrimSpace(itemID) == "" {
		return nil, errors.New("encode LLM translation input: required identity or language field is empty")
	}
	privateInput, err := json.Marshal(struct {
		ItemID          string         `json:"item_id"`
		Language        string         `json:"language"`
		TargetLanguage  string         `json:"target_language"`
		CanonicalLemma  string         `json:"canonical_lemma"`
		UPOS            string         `json:"upos"`
		TargetWord      string         `json:"target_word,omitempty"`
		ExampleSentence string         `json:"example_sentence,omitempty"`
		CandidateSenses []LexicalSense `json:"candidate_senses,omitempty"`
	}{itemID, input.Language, translationTargetLanguage(input), input.CanonicalLemma, input.UPOS, input.TargetWord, input.ExampleSentence, CloneLexicalSenses(input.CandidateSenses)})
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
	return c.decodeResponseWithItemID(input, body, TranslationItemID(input))
}

func (c *TranslationCodec) decodeResponseWithItemID(input TranslationRequest, body []byte, expectedItemID string) (TranslationResponse, TranslationUsage, error) {
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
	var result struct {
		ItemID                    string          `json:"item_id"`
		SourceLanguage            string          `json:"source_language"`
		TargetLanguage            string          `json:"target_language"`
		Translation               string          `json:"translation"`
		SentenceTranslation       string          `json:"sentence_translation"`
		SentenceTranslationTarget string          `json:"sentence_translation_target"`
		SenseOrder                json.RawMessage `json:"sense_order"`
		FallbackGloss             json.RawMessage `json:"fallback_gloss"`
	}
	resultDecoder := json.NewDecoder(strings.NewReader(decoded.Choices[0].Message.Content))
	if duplicate, err := hasDuplicateObjectKey(decoded.Choices[0].Message.Content); err != nil {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("decode LLM translation: %w", err)
	} else if duplicate {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM translation: duplicate field")
	}
	resultDecoder.DisallowUnknownFields()
	if err := resultDecoder.Decode(&result); err != nil {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("decode LLM translation: %w", err)
	}
	if err := resultDecoder.Decode(&struct{}{}); err != io.EOF {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM translation: trailing JSON content")
	}
	result.ItemID = strings.TrimSpace(result.ItemID)
	result.SourceLanguage = strings.TrimSpace(result.SourceLanguage)
	result.TargetLanguage = strings.TrimSpace(result.TargetLanguage)
	result.Translation = strings.TrimSpace(result.Translation)
	result.SentenceTranslation = strings.TrimSpace(result.SentenceTranslation)
	result.SentenceTranslationTarget = strings.TrimSpace(result.SentenceTranslationTarget)
	if result.ItemID == "" || result.ItemID != expectedItemID {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM translation: item_id mismatch")
	}
	if result.SourceLanguage == "" || result.SourceLanguage != strings.TrimSpace(input.Language) {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM translation: source_language mismatch")
	}
	if result.TargetLanguage == "" || result.TargetLanguage != translationTargetLanguage(input) {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM translation: target_language mismatch")
	}
	if result.Translation == "" {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM translation: translation is empty")
	}
	if input.ExampleSentence != "" && result.SentenceTranslation == "" {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM translation: sentence_translation is empty")
	}
	if decoded.Usage.PromptTokens < 0 || decoded.Usage.CompletionTokens < 0 || decoded.Usage.TotalTokens < 0 {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM response: invalid usage")
	}
	if hasMarkup(result.Translation) || hasMarkup(result.SentenceTranslation) || hasMarkup(result.SentenceTranslationTarget) {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM translation: HTML or markup is not allowed")
	}
	response := TranslationResponse{Translation: result.Translation, SentenceTranslation: result.SentenceTranslation, SentenceTranslationTarget: result.SentenceTranslationTarget}
	if order, warning := decodeSenseOrder(result.SenseOrder); warning != "" {
		response.Warnings = append(response.Warnings, warning)
	} else {
		response.SenseOrder = order
	}
	if fallback, warning := decodeFallbackGloss(result.FallbackGloss); warning != "" {
		response.Warnings = append(response.Warnings, warning)
	} else {
		response.FallbackGloss = fallback
	}
	response, normalizeErr := NormalizeTranslationResponse(input, response)
	if normalizeErr != nil {
		return TranslationResponse{}, TranslationUsage{}, normalizeErr
	}
	return response, decoded.Usage, nil
}

// NormalizeTranslationResponse keeps malformed optional meaning fields from
// failing a translation run while still enforcing the required translation
// contract against the frozen candidate senses.
func NormalizeTranslationResponse(input TranslationRequest, response TranslationResponse) (TranslationResponse, error) {
	response.Translation = strings.TrimSpace(response.Translation)
	response.SentenceTranslation = strings.TrimSpace(response.SentenceTranslation)
	response.SentenceTranslationTarget = strings.TrimSpace(response.SentenceTranslationTarget)
	response.FallbackGloss = strings.TrimSpace(response.FallbackGloss)
	if response.Translation == "" {
		return TranslationResponse{}, errors.New("decode LLM translation: translation is empty")
	}
	if input.ExampleSentence != "" && response.SentenceTranslation == "" {
		return TranslationResponse{}, errors.New("decode LLM translation: sentence_translation is empty")
	}
	if hasMarkup(response.Translation) || hasMarkup(response.SentenceTranslation) || hasMarkup(response.SentenceTranslationTarget) {
		return TranslationResponse{}, errors.New("decode LLM translation: HTML or markup is not allowed")
	}
	selectionInvalid := !ValidateSenseSelection(response.SenseOrder, len(input.CandidateSenses))
	for _, warning := range response.Warnings {
		selectionInvalid = selectionInvalid || strings.HasPrefix(warning, "sense selection") || strings.HasPrefix(warning, "invalid sense selection")
	}
	if selectionInvalid {
		response.SenseOrder = nil
		response.FallbackGloss = ""
		if !hasWarning(response.Warnings, "invalid sense selection; using deterministic order") {
			response.Warnings = append(response.Warnings, "invalid sense selection; using deterministic order")
		}
	} else if len(response.SenseOrder) > DefaultMaxSenses {
		response.SenseOrder = response.SenseOrder[:DefaultMaxSenses]
	}
	if response.FallbackGloss != "" && (hasMarkup(response.FallbackGloss) || len([]rune(response.FallbackGloss)) > MaxFallbackGlossRunes) {
		response.FallbackGloss = ""
		response.Warnings = append(response.Warnings, "invalid fallback gloss; ignoring it")
	}
	if len(response.SenseOrder) > 0 && response.FallbackGloss != "" {
		response.FallbackGloss = ""
		response.Warnings = append(response.Warnings, "fallback gloss supplied with a sense selection; ignoring it")
	}
	return response, nil
}

func hasWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}
	return false
}

func decodeSenseOrder(raw json.RawMessage) ([]int, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, ""
	}
	var order []int
	if err := json.Unmarshal(raw, &order); err != nil {
		return nil, "invalid sense selection; using deterministic order"
	}
	return order, ""
}

func decodeFallbackGloss(raw json.RawMessage) (string, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", ""
	}
	var fallback string
	if err := json.Unmarshal(raw, &fallback); err != nil {
		return "", "invalid fallback gloss; ignoring it"
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "", ""
	}
	if hasMarkup(fallback) || len([]rune(fallback)) > MaxFallbackGlossRunes {
		return "", "invalid fallback gloss; ignoring it"
	}
	return fallback, ""
}

func hasMarkup(value string) bool { return strings.ContainsAny(value, "<>") }

func hasDuplicateObjectKey(value string) (bool, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	token, err := decoder.Token()
	if err != nil {
		return false, err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return false, nil
	}
	seen := map[string]struct{}{}
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return false, err
		}
		key, ok := token.(string)
		if !ok {
			return false, errors.New("object field name is not a string")
		}
		if _, exists := seen[key]; exists {
			return true, nil
		}
		seen[key] = struct{}{}
		var raw json.RawMessage
		if err = decoder.Decode(&raw); err != nil {
			return false, err
		}
	}
	return false, nil
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
