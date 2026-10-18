package enrichment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

var llmSystemPrompt = fmt.Sprintf("Translate the supplied lemma into the target language. Return exactly one JSON object using only these eight field names, with no markdown or additional keys: item_id, source_language, target_language, translation (a concise lemma translation), sentence_translation (a natural translation of the complete example sentence), sentence_translation_targets (an ordered array of exact plain-text excerpt(s) from sentence_translation that identify only the smallest meaning-bearing words or phrases corresponding to the supplied target; use one excerpt for a contiguous phrase, multiple excerpts only for a confident discontinuous correspondence, and an empty array when no reliable alignment exists), sense_order (an optional array of distinct 0-based integer indices into the frozen candidate_senses list, in best-fit order; return at most %d indices, capped at the display limit of %d; omit it, or return an empty array, when no candidate sense fits), and fallback_gloss (an optional concise English gloss only when no candidate sense fits or the candidate list is empty). Do not include the whole sentence or unrelated words in an alignment. Echo item_id and both languages exactly. When no example sentence is supplied, sentence_translation and sentence_translation_targets must be empty. Do not return HTML or markup in any field.", DefaultMaxSenses, DefaultMaxSenses)

var contextualGlossSystemPrompt = "Translate the supplied lemma and complete representative sentence. Return exactly one JSON object using only these ten fields: item_id, source_language, target_language, translation, sentence_translation, sentence_translation_targets, gloss, evidence_ids, context_only, unresolved_reason. sentence_translation_targets is an ordered array of exact plain-text excerpt(s) from sentence_translation that identify only the smallest meaning-bearing words or phrases corresponding to the supplied target; use one excerpt for a contiguous phrase, multiple excerpts only for a confident discontinuous correspondence, and an empty array when no reliable alignment exists. Do not include the whole sentence or unrelated words. Gloss must be one brief English cue for the target's meaning in this sentence, not a list of unrelated senses. evidence_ids must contain only the exact short evidence_id handles (for example e1 or e2) shown in candidate_senses that support the gloss. If no defensible contextual meaning can be given, return an empty gloss, empty evidence_ids, context_only=true, and a brief unresolved_reason explaining why. Otherwise return a non-empty gloss, supporting evidence_ids when any frozen evidence applies (or an empty list and context_only=true when none applies), and unresolved_reason as an empty string. Never invent IDs. Echo item_id and languages exactly. Do not return HTML or markup."

// evidenceAssistedPassSystemPrompt is the strict Ancient Greek evidence-assisted
// pass contract. The payload carries the representative sentence, its
// unhighlighted passage context and the pinned LSJ senses with their native IDs
// as short handles. The provider proposes a contextual Gloss with a sentence
// translation and English target alignment, or abstains. It never guarantees a
// card: a failed or abstaining answer leaves the source-evidence card as it
// was (ADR 0101).
var evidenceAssistedPassSystemPrompt = "Translate the supplied lemma in the supplied representative sentence of an Ancient Greek text. The passage_context is the labelled English of nearby sections, not a translation of the sentence; use it only as background. candidate_senses are the pinned LSJ senses of the lemma, each with a short evidence_id handle. Return exactly one JSON object using only these ten fields: item_id, source_language, target_language, translation, sentence_translation, sentence_translation_targets, gloss, evidence_ids, context_only, unresolved_reason. translation is a concise lemma translation. gloss must be one brief English cue for the target's meaning in this sentence, not a list of unrelated senses. sentence_translation is a natural English translation of the complete representative sentence. sentence_translation_targets is an ordered array of exact plain-text excerpt(s) from sentence_translation that identify only the smallest meaning-bearing words or phrases corresponding to the supplied target; use one excerpt for a contiguous phrase, multiple excerpts only for a confident discontinuous correspondence, and an empty array when no reliable alignment exists. Do not include the whole sentence or unrelated words. evidence_ids must contain only the exact short evidence_id handles shown in candidate_senses that support the gloss. If no defensible contextual meaning can be given, abstain: return an empty gloss, empty evidence_ids, context_only=true, and a brief unresolved_reason explaining why; still return a translation and a sentence_translation. Otherwise return a non-empty gloss, supporting evidence_ids when any pinned evidence applies (or an empty list and context_only=true when none applies), and unresolved_reason as an empty string. Never invent IDs. Echo item_id and languages exactly. Do not return HTML or markup."

// lsjSenseSummarySystemPrompt is the strict Ancient Greek LSJ sense summary
// contract. The payload carries only the pinned LSJ entry (its headword and its
// senses as short handles with English glosses): no representative sentence and
// no learner data. The response condenses the entry into a few short lines, each
// citing sense handles that exist in that entry. A summary that cites an unknown
// sense or none is discarded by the caller (ADR 0101).
var lsjSenseSummarySystemPrompt = "Condense the supplied Ancient Greek LSJ dictionary entry into a few short English summary lines. The payload carries only the entry's headword and its senses, each with a short evidence_id handle and its English glosses; there is no sentence and no other context. Return exactly one JSON object using only these two field names: item_id and summary_lines. summary_lines is an ordered array of objects, each with sense_ids (an array of the exact evidence_id handles shown in senses that the line condenses) and text (one short English line). Every line must cite at least one sense_ids handle that appears in senses, and every cited handle must exist. Never invent IDs. Do not add a sentence translation, a gloss, or any other key. Echo item_id exactly. Do not return HTML or markup."

const (
	maxTranslationResponseBytes = 1 << 20
	maxUnresolvedReasonRunes    = 200
)

// ErrInvalidTranslationResponse marks a provider response that failed the
// strict translation schema, identity, or language validation. Callers may
// retry it up to their bounded provider-attempt limit because a malformed
// response is transient provider behavior, unlike request encoding or
// configuration failures.
var ErrInvalidTranslationResponse = errors.New("enrichment: invalid translation response")

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

// ContextualGlossProviderVersion identifies the strict contextual-gloss
// response contract used by standard prepared-deck runs.
func (c *TranslationCodec) ContextualGlossProviderVersion() string {
	if c == nil {
		return ""
	}
	version := c.model + "/" + contextualGlossPromptVersion
	if c.usesReasoningEffort {
		version += "-reasoning-" + c.reasoningEffort
	}
	return version
}

// ContextualGlossPromptContract returns the versioned contextual-gloss prompt
// contract alone, without the configured model. The provider-payload
// assessment pins this contract, so it is stable across model changes.
func (c *TranslationCodec) ContextualGlossPromptContract() string {
	return contextualGlossPromptVersion
}

// EvidenceAssistedPassProviderVersion identifies the strict Ancient Greek
// evidence-assisted pass response contract used by a pass run.
func (c *TranslationCodec) EvidenceAssistedPassProviderVersion() string {
	if c == nil {
		return ""
	}
	version := c.model + "/" + evidenceAssistedPassPromptVersion
	if c.usesReasoningEffort {
		version += "-reasoning-" + c.reasoningEffort
	}
	return version
}

// EvidenceAssistedPassPromptContract returns the versioned evidence-assisted
// pass prompt contract alone, without the configured model. The
// provider-payload assessment pins this contract, so it is stable across model
// changes.
func (c *TranslationCodec) EvidenceAssistedPassPromptContract() string {
	return evidenceAssistedPassPromptVersion
}

// LSJSenseSummaryProviderVersion identifies the strict Ancient Greek LSJ sense
// summary response contract used by a summary run.
func (c *TranslationCodec) LSJSenseSummaryProviderVersion() string {
	if c == nil {
		return ""
	}
	version := c.model + "/" + lsjSenseSummaryPromptVersion
	if c.usesReasoningEffort {
		version += "-reasoning-" + c.reasoningEffort
	}
	return version
}

// LSJSenseSummaryPromptContract returns the versioned LSJ sense summary prompt
// contract alone, without the configured model. The provider-payload assessment
// pins this contract, so it is stable across model changes.
func (c *TranslationCodec) LSJSenseSummaryPromptContract() string {
	return lsjSenseSummaryPromptVersion
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

// TranslationItemID returns the stable opaque identity of one translation
// input. It contains no owner identity or source text. A contextual Gloss
// request transmits the short ContextualGlossItemHandle in the payload and
// resolves back to this stable ID when decoding.
func TranslationItemID(input TranslationRequest) string {
	// The digest is deliberately opaque and contains no owner or source text.
	// Hashing the sentence keeps distinct contextual items distinct without
	// placing the sentence itself in provider-visible identity.
	sentenceDigest := sha256.Sum256([]byte(input.ExampleSentence))
	canonical := strings.Join([]string{input.Language, translationTargetLanguage(input), input.CanonicalLemma, input.UPOS, input.TargetWord, hex.EncodeToString(sentenceDigest[:])}, "\x00")
	digest := sha256.Sum256([]byte(canonical))
	return "translation-item-" + hex.EncodeToString(digest[:16])
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
// the exact request body for one Batch item. A contextual Gloss request carries
// the short per-request item handle rather than itemID.
func (c *TranslationCodec) EncodeBatchRequestForValidation(input TranslationRequest, itemID string) ([]byte, error) {
	return c.encodeRequest(input, itemID)
}

func (c *TranslationCodec) encodeRequest(input TranslationRequest, itemID string) ([]byte, error) {
	if c == nil {
		return nil, errors.New("encode LLM request: nil translation codec")
	}
	if input.RequireLSJSenseSummary {
		return c.encodeLSJSenseSummaryRequest(input)
	}
	if strings.TrimSpace(input.Language) == "" || strings.TrimSpace(translationTargetLanguage(input)) == "" || strings.TrimSpace(input.CanonicalLemma) == "" || strings.TrimSpace(input.UPOS) == "" || strings.TrimSpace(itemID) == "" {
		return nil, errors.New("encode LLM translation input: required identity or language field is empty")
	}
	candidateSenses := CloneLexicalSenses(input.CandidateSenses)
	if input.RequireContextualGloss || input.RequireEvidenceAssistedPass {
		handles := newContextualGlossHandles(input)
		itemID = ContextualGlossItemHandle
		for i := range candidateSenses {
			if handle, ok := handles.toHandle[candidateSenses[i].EvidenceID]; ok {
				candidateSenses[i].EvidenceID = handle
			}
		}
	}
	privateInput, err := json.Marshal(struct {
		ItemID          string         `json:"item_id"`
		Language        string         `json:"language"`
		TargetLanguage  string         `json:"target_language"`
		CanonicalLemma  string         `json:"canonical_lemma"`
		UPOS            string         `json:"upos"`
		TargetWord      string         `json:"target_word,omitempty"`
		ExampleSentence string         `json:"example_sentence,omitempty"`
		PassageContext  string         `json:"passage_context,omitempty"`
		CandidateSenses []LexicalSense `json:"candidate_senses,omitempty"`
	}{itemID, input.Language, translationTargetLanguage(input), input.CanonicalLemma, input.UPOS, input.TargetWord, input.ExampleSentence, input.PassageContext, candidateSenses})
	if err != nil {
		return nil, fmt.Errorf("encode LLM translation input: %w", err)
	}
	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: promptForRequest(input)},
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

func promptForRequest(input TranslationRequest) string {
	if input.RequireLSJSenseSummary {
		return lsjSenseSummarySystemPrompt
	}
	if input.RequireEvidenceAssistedPass {
		return evidenceAssistedPassSystemPrompt
	}
	if input.RequireContextualGloss {
		return contextualGlossSystemPrompt
	}
	return llmSystemPrompt
}

// LSJSenseSummaryItemHandle is the short per-request item handle an LSJ sense
// summary payload transmits as item_id. Every summary request carries exactly
// one entry, so the handle is the same short token for each request; the decoder
// still rejects an echoed value that does not match the frozen request.
const LSJSenseSummaryItemHandle = "s1"

// lsjSenseSummaryPayloadFields is the exact ordered field set an Ancient Greek
// LSJ sense summary request transmits in its user message: only the pinned LSJ
// entry (its headword and its senses as short handles with English glosses). It
// is the reviewed contract the #2172 provider-payload assessment pins before any
// request is sent.
var lsjSenseSummaryPayloadFields = []string{"item_id", "headword", "senses"}

// LSJSenseSummaryPayloadFields returns a copy of the ordered field set an LSJ
// sense summary request transmits.
func LSJSenseSummaryPayloadFields() []string {
	return slices.Clone(lsjSenseSummaryPayloadFields)
}

// lsjSenseSummarySense is the wire form of one citable sense: its short handle
// and the English glosses the summary condenses.
type lsjSenseSummarySense struct {
	ID      string   `json:"id"`
	Glosses []string `json:"glosses"`
}

// encodeLSJSenseSummaryRequest encodes the context-free LSJ sense summary
// payload. It carries only the pinned entry's headword and its senses, so it
// transmits no representative sentence and no learner data.
func (c *TranslationCodec) encodeLSJSenseSummaryRequest(input TranslationRequest) ([]byte, error) {
	if strings.TrimSpace(input.Headword) == "" || len(input.LSJSenses) == 0 {
		return nil, errors.New("encode LSJ sense summary input: headword and senses are required")
	}
	senses := make([]lsjSenseSummarySense, 0, len(input.LSJSenses))
	for _, sense := range input.LSJSenses {
		if strings.TrimSpace(sense.ID) == "" {
			return nil, errors.New("encode LSJ sense summary input: sense id is required")
		}
		senses = append(senses, lsjSenseSummarySense{ID: sense.ID, Glosses: append([]string(nil), sense.Glosses...)})
	}
	privateInput, err := json.Marshal(struct {
		ItemID   string                 `json:"item_id"`
		Headword string                 `json:"headword"`
		Senses   []lsjSenseSummarySense `json:"senses"`
	}{LSJSenseSummaryItemHandle, input.Headword, senses})
	if err != nil {
		return nil, fmt.Errorf("encode LSJ sense summary input: %w", err)
	}
	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: lsjSenseSummarySystemPrompt},
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
		return nil, fmt.Errorf("encode LSJ sense summary request: %w", err)
	}
	return body, nil
}

// contextualGlossPayloadFields is the exact ordered field set a contextual
// Gloss request transmits in its user message. It is the reviewed contract a
// provider-payload assessment pins before any request is sent.
var contextualGlossPayloadFields = []string{
	"item_id", "language", "target_language", "canonical_lemma", "upos",
	"target_word", "example_sentence", "candidate_senses",
}

// ContextualGlossPayloadFields returns a copy of the ordered field set a
// contextual Gloss request transmits.
func ContextualGlossPayloadFields() []string {
	return slices.Clone(contextualGlossPayloadFields)
}

// evidenceAssistedPassPayloadFields is the exact ordered field set an Ancient
// Greek evidence-assisted pass request transmits in its user message:
// the representative sentence, its passage context and the pinned LSJ senses
// with their native IDs (as short handles). It is the reviewed contract the
// #2171 provider-payload assessment pins before any request is sent.
var evidenceAssistedPassPayloadFields = []string{
	"item_id", "language", "target_language", "canonical_lemma", "upos",
	"target_word", "example_sentence", "passage_context", "candidate_senses",
}

// EvidenceAssistedPassPayloadFields returns a copy of the ordered field set an
// evidence-assisted pass request transmits.
func EvidenceAssistedPassPayloadFields() []string {
	return slices.Clone(evidenceAssistedPassPayloadFields)
}

// ContextualGlossItemHandle is the short handle a contextual Gloss payload
// transmits as item_id in place of the long stable translation item ID. Every
// contextual Gloss request carries exactly one item, so the handle is the same
// short token for each request; the decoder still rejects an echoed value that
// does not match the frozen request.
const ContextualGlossItemHandle = "i1"

// evidenceHandlePrefix prefixes each citable candidate sense's short handle.
const evidenceHandlePrefix = "e"

// contextualGlossHandles is the per-request mapping between the stable evidence
// identifiers a contextual Gloss request freezes and the short handles the
// provider sees. The transmitted payload carries only the handles; decoding
// maps an echoed handle back to its stable identifier and rejects a handle the
// request never issued.
type contextualGlossHandles struct {
	toHandle   map[string]string
	toEvidence map[string]string
}

// newContextualGlossHandles assigns each citable candidate sense a short
// evidence handle in candidate order (e1, e2, …). A sense without a stable
// evidence ID cannot be cited and receives no handle.
func newContextualGlossHandles(input TranslationRequest) contextualGlossHandles {
	handles := contextualGlossHandles{
		toHandle:   make(map[string]string, len(input.CandidateSenses)),
		toEvidence: make(map[string]string, len(input.CandidateSenses)),
	}
	for _, sense := range input.CandidateSenses {
		if sense.EvidenceID == "" {
			continue
		}
		if _, assigned := handles.toHandle[sense.EvidenceID]; assigned {
			continue
		}
		handle := evidenceHandlePrefix + strconv.Itoa(len(handles.toHandle)+1)
		handles.toHandle[sense.EvidenceID] = handle
		handles.toEvidence[handle] = sense.EvidenceID
	}
	return handles
}

// mapContextualEvidenceHandles replaces the short handles a contextual Gloss
// or evidence-assisted pass response echoed with the stable evidence IDs the
// request froze. Every other response already carries stable identifiers, so it
// is returned unchanged.
func mapContextualEvidenceHandles(input TranslationRequest, response *TranslationResponse) error {
	if !input.RequireContextualGloss && !input.RequireEvidenceAssistedPass {
		return nil
	}
	handles := newContextualGlossHandles(input)
	for i, handle := range response.EvidenceIDs {
		if handle == "" {
			return errors.New("decode LLM translation: empty evidence ID")
		}
		stable, ok := handles.toEvidence[handle]
		if !ok {
			return errors.New("decode LLM translation: unknown evidence ID")
		}
		response.EvidenceIDs[i] = stable
	}
	return nil
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
	response, usage, err := c.decodeResponseWithItemID(input, body, TranslationItemID(input))
	if err != nil {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("%w: %w", ErrInvalidTranslationResponse, err)
	}
	return response, usage, nil
}

// chatCompletionEnvelope is the bounded Chat Completions view the codec reads:
// the first choice's message and the provider-reported usage.
type chatCompletionEnvelope struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage TranslationUsage `json:"usage"`
}

// decodedTranslationFields is the strict provider content object before
// validation and normalization. Optional meaning fields stay raw so each
// response shape or malformed value is handled as its own step.
type decodedTranslationFields struct {
	ItemID                     string          `json:"item_id"`
	SourceLanguage             string          `json:"source_language"`
	TargetLanguage             string          `json:"target_language"`
	Translation                string          `json:"translation"`
	SentenceTranslation        string          `json:"sentence_translation"`
	SentenceTranslationTargets json.RawMessage `json:"sentence_translation_targets"`
	SenseOrder                 json.RawMessage `json:"sense_order"`
	FallbackGloss              json.RawMessage `json:"fallback_gloss"`
	Gloss                      json.RawMessage `json:"gloss"`
	EvidenceIDs                json.RawMessage `json:"evidence_ids"`
	ContextOnly                json.RawMessage `json:"context_only"`
	UnresolvedReason           json.RawMessage `json:"unresolved_reason"`
}

func (c *TranslationCodec) decodeResponseWithItemID(input TranslationRequest, body []byte, expectedItemID string) (TranslationResponse, TranslationUsage, error) {
	if c == nil {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM response: nil translation codec")
	}
	if len(body) > maxTranslationResponseBytes {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM response: response exceeds 1 MiB")
	}
	decoded, err := decodeChatCompletion(body)
	if err != nil {
		return TranslationResponse{}, TranslationUsage{}, err
	}
	if input.RequireLSJSenseSummary {
		return decodeLSJSenseSummary(decoded)
	}
	result, err := decodeTranslationFields(decoded.Choices[0].Message.Content)
	if err != nil {
		return TranslationResponse{}, TranslationUsage{}, err
	}
	if input.RequireContextualGloss || input.RequireEvidenceAssistedPass {
		// A contextual Gloss or evidence-assisted pass request transmits the
		// short per-request item handle, not the stable item ID, so the echo is
		// validated against it.
		expectedItemID = ContextualGlossItemHandle
	}
	if err := validateTranslationIdentity(input, expectedItemID, result); err != nil {
		return TranslationResponse{}, TranslationUsage{}, err
	}
	if decoded.Usage.PromptTokens < 0 || decoded.Usage.CompletionTokens < 0 || decoded.Usage.TotalTokens < 0 {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM response: invalid usage")
	}
	if hasMarkup(result.Translation) || hasMarkup(result.SentenceTranslation) {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LLM translation: HTML or markup is not allowed")
	}
	response, err := buildTranslationResponse(input, result)
	if err != nil {
		return TranslationResponse{}, TranslationUsage{}, err
	}
	return response, decoded.Usage, nil
}

// decodeChatCompletion decodes the bounded envelope and rejects trailing
// content or an empty choice list.
func decodeChatCompletion(body []byte) (chatCompletionEnvelope, error) {
	var decoded chatCompletionEnvelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&decoded); err != nil {
		return chatCompletionEnvelope{}, fmt.Errorf("decode LLM response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return chatCompletionEnvelope{}, errors.New("decode LLM response: trailing JSON content")
	}
	if len(decoded.Choices) == 0 {
		return chatCompletionEnvelope{}, errors.New("decode LLM response: no choices")
	}
	return decoded, nil
}

// decodeLSJSenseSummary decodes the strict LSJ sense summary content object:
// only item_id and summary_lines. Duplicate keys and unknown fields are
// rejected, and an echoed item_id that does not match the frozen handle is
// rejected. Sense-ID citation validity is checked by the caller against the
// pinned entry, because an unknown or missing sense ID discards the summary
// rather than failing the request (ADR 0101).
func decodeLSJSenseSummary(envelope chatCompletionEnvelope) (TranslationResponse, TranslationUsage, error) {
	content := envelope.Choices[0].Message.Content
	if duplicate, err := hasDuplicateObjectKey(content); err != nil {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("decode LSJ sense summary: %w", err)
	} else if duplicate {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LSJ sense summary: duplicate field")
	}
	var result struct {
		ItemID       string                `json:"item_id"`
		SummaryLines []LSJSenseSummaryLine `json:"summary_lines"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("decode LSJ sense summary: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LSJ sense summary: trailing JSON content")
	}
	if strings.TrimSpace(result.ItemID) != LSJSenseSummaryItemHandle {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LSJ sense summary: item_id mismatch")
	}
	if len(result.SummaryLines) == 0 {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode LSJ sense summary: no summary lines")
	}
	return TranslationResponse{SenseSummary: normalizeLSJSenseSummaryLines(result.SummaryLines)}, envelope.Usage, nil
}

// normalizeLSJSenseSummaryLines trims each decoded line's text and drops blank
// cited handles so a stray empty value cannot pass the citation check.
func normalizeLSJSenseSummaryLines(lines []LSJSenseSummaryLine) []LSJSenseSummaryLine {
	result := make([]LSJSenseSummaryLine, 0, len(lines))
	for _, line := range lines {
		ids := make([]string, 0, len(line.SenseIDs))
		for _, id := range line.SenseIDs {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				ids = append(ids, trimmed)
			}
		}
		result = append(result, LSJSenseSummaryLine{SenseIDs: ids, Text: strings.TrimSpace(line.Text)})
	}
	return result
}

// decodeTranslationFields decodes the strict provider content object. Duplicate
// keys and unknown fields are rejected, and echoed identity fields are trimmed.
func decodeTranslationFields(content string) (decodedTranslationFields, error) {
	var result decodedTranslationFields
	resultDecoder := json.NewDecoder(strings.NewReader(content))
	if duplicate, err := hasDuplicateObjectKey(content); err != nil {
		return decodedTranslationFields{}, fmt.Errorf("decode LLM translation: %w", err)
	} else if duplicate {
		return decodedTranslationFields{}, errors.New("decode LLM translation: duplicate field")
	}
	resultDecoder.DisallowUnknownFields()
	if err := resultDecoder.Decode(&result); err != nil {
		return decodedTranslationFields{}, fmt.Errorf("decode LLM translation: %w", err)
	}
	if err := resultDecoder.Decode(&struct{}{}); err != io.EOF {
		return decodedTranslationFields{}, errors.New("decode LLM translation: trailing JSON content")
	}
	result.ItemID = strings.TrimSpace(result.ItemID)
	result.SourceLanguage = strings.TrimSpace(result.SourceLanguage)
	result.TargetLanguage = strings.TrimSpace(result.TargetLanguage)
	result.Translation = strings.TrimSpace(result.Translation)
	result.SentenceTranslation = strings.TrimSpace(result.SentenceTranslation)
	return result, nil
}

// validateTranslationIdentity enforces the echoed identity, languages and
// required translation fields against the frozen request.
func validateTranslationIdentity(input TranslationRequest, expectedItemID string, result decodedTranslationFields) error {
	if result.ItemID == "" || result.ItemID != expectedItemID {
		return errors.New("decode LLM translation: item_id mismatch")
	}
	if result.SourceLanguage == "" || result.SourceLanguage != strings.TrimSpace(input.Language) {
		return errors.New("decode LLM translation: source_language mismatch")
	}
	if result.TargetLanguage == "" || result.TargetLanguage != translationTargetLanguage(input) {
		return errors.New("decode LLM translation: target_language mismatch")
	}
	if result.Translation == "" {
		return errors.New("decode LLM translation: translation is empty")
	}
	if input.ExampleSentence != "" && result.SentenceTranslation == "" {
		return errors.New("decode LLM translation: sentence_translation is empty")
	}
	return nil
}

// buildTranslationResponse assembles the optional meaning fields and applies
// the shared normalization contract.
func buildTranslationResponse(input TranslationRequest, result decodedTranslationFields) (TranslationResponse, error) {
	response := TranslationResponse{Translation: result.Translation, SentenceTranslation: result.SentenceTranslation}
	if len(result.SentenceTranslationTargets) > 0 && string(result.SentenceTranslationTargets) != "null" {
		response.SentenceTranslationTargets = decodeOptionalTargets(result.SentenceTranslationTargets)
	}
	if err := requireContextualGlossFields(input, result); err != nil {
		return TranslationResponse{}, err
	}
	if err := decodeMeaningFields(result, &response); err != nil {
		return TranslationResponse{}, err
	}
	if err := mapContextualEvidenceHandles(input, &response); err != nil {
		return TranslationResponse{}, err
	}
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
	if input.RequireContextualGloss || input.RequireEvidenceAssistedPass {
		// The transmitted payload carried a short per-request handle in place
		// of the stable item ID; the decoded response reports the stable ID.
		response.ItemID = TranslationItemID(input)
	}
	return NormalizeTranslationResponse(input, response)
}

// requireContextualGlossFields rejects a contextual-gloss or evidence-assisted
// pass response that omits one of the four mandatory outcome fields (even when
// it sent JSON null).
func requireContextualGlossFields(input TranslationRequest, result decodedTranslationFields) error {
	if (input.RequireContextualGloss || input.RequireEvidenceAssistedPass) && (len(result.Gloss) == 0 || len(result.EvidenceIDs) == 0 || len(result.ContextOnly) == 0 || string(result.Gloss) == "null" || string(result.EvidenceIDs) == "null" || string(result.ContextOnly) == "null") {
		return errors.New("decode LLM translation: incomplete contextual gloss outcome")
	}
	return nil
}

// decodeMeaningFields decodes the optional meaning fields one shape at a time,
// keeping a malformed optional field's error distinct from the others.
func decodeMeaningFields(result decodedTranslationFields, response *TranslationResponse) error {
	if len(result.Gloss) == 0 && len(result.EvidenceIDs) == 0 && len(result.ContextOnly) == 0 && len(result.UnresolvedReason) == 0 {
		return nil
	}
	if err := json.Unmarshal(result.Gloss, &response.Gloss); err != nil {
		return errors.New("decode LLM translation: invalid gloss")
	}
	if err := json.Unmarshal(result.EvidenceIDs, &response.EvidenceIDs); err != nil {
		return errors.New("decode LLM translation: invalid evidence_ids")
	}
	if err := json.Unmarshal(result.ContextOnly, &response.ContextOnly); err != nil {
		return errors.New("decode LLM translation: invalid context_only")
	}
	if len(result.UnresolvedReason) != 0 {
		if err := json.Unmarshal(result.UnresolvedReason, &response.UnresolvedReason); err != nil {
			return errors.New("decode LLM translation: invalid unresolved_reason")
		}
	}
	return nil
}

func decodeOptionalTargets(raw json.RawMessage) []string {
	var targets []string
	if err := json.Unmarshal(raw, &targets); err != nil {
		return nil
	}
	return targets
}

// NormalizeTranslationResponse keeps malformed optional meaning fields from
// failing a translation run while still enforcing the required translation
// contract against the frozen candidate senses.
func NormalizeTranslationResponse(input TranslationRequest, response TranslationResponse) (TranslationResponse, error) {
	response.Translation = strings.TrimSpace(response.Translation)
	response.SentenceTranslation = strings.TrimSpace(response.SentenceTranslation)
	response.SentenceTranslationTargets = ValidateSentenceTranslationTargets(response.SentenceTranslation, response.SentenceTranslationTargets)
	response.Gloss = strings.TrimSpace(response.Gloss)
	response.UnresolvedReason = strings.TrimSpace(response.UnresolvedReason)
	response.FallbackGloss = strings.TrimSpace(response.FallbackGloss)
	if response.Translation == "" {
		return TranslationResponse{}, errors.New("decode LLM translation: translation is empty")
	}
	if input.RequireContextualGloss || input.RequireEvidenceAssistedPass {
		// A resolved meaning and an unresolved reason are mutually exclusive.
		// Keep the resolved meaning and discard the stray reason without
		// validating it: it is never stored, cached, or rendered, and failing
		// the response would spend an item's retry budget on an irrelevant field.
		if response.Gloss != "" {
			response.UnresolvedReason = ""
		}
		if err := validateContextualMeaning(input, response); err != nil {
			return TranslationResponse{}, err
		}
	}
	if input.ExampleSentence != "" && response.SentenceTranslation == "" {
		return TranslationResponse{}, errors.New("decode LLM translation: sentence_translation is empty")
	}
	if hasMarkup(response.Translation) || hasMarkup(response.SentenceTranslation) {
		return TranslationResponse{}, errors.New("decode LLM translation: HTML or markup is not allowed")
	}
	return reconcileSenseSelection(input, response), nil
}

// validateContextualMeaning enforces the strict contextual-gloss contract:
// bounded gloss and reason, evidence IDs drawn from the frozen candidates, and
// a consistent resolved-or-unresolved outcome.
func validateContextualMeaning(input TranslationRequest, response TranslationResponse) error {
	if err := validateContextualGloss(response.Gloss); err != nil {
		return err
	}
	if err := validateContextualUnresolvedReason(response.UnresolvedReason); err != nil {
		return err
	}
	if err := validateContextualEvidence(input, response); err != nil {
		return err
	}
	if err := validateContextualEvidenceAgreement(response); err != nil {
		return err
	}
	return validateContextualOutcome(response)
}

// validateContextualGloss caps the free-text gloss and rejects markup.
func validateContextualGloss(gloss string) error {
	if len([]rune(gloss)) > MaxFallbackGlossRunes || hasMarkup(gloss) {
		return errors.New("decode LLM translation: invalid contextual gloss")
	}
	return nil
}

// validateContextualUnresolvedReason caps the free-text unresolved reason and
// rejects markup.
func validateContextualUnresolvedReason(reason string) error {
	if len([]rune(reason)) > maxUnresolvedReasonRunes || hasMarkup(reason) {
		return errors.New("decode LLM translation: invalid unresolved reason")
	}
	return nil
}

// validateContextualEvidence requires every evidence ID to be named, known, and
// unique.
func validateContextualEvidence(input TranslationRequest, response TranslationResponse) error {
	knownEvidence := make(map[string]struct{}, len(input.CandidateSenses))
	for _, sense := range input.CandidateSenses {
		if sense.EvidenceID != "" {
			knownEvidence[sense.EvidenceID] = struct{}{}
		}
	}
	seenEvidence := make(map[string]struct{}, len(response.EvidenceIDs))
	for _, id := range response.EvidenceIDs {
		if err := validateEvidenceID(id, knownEvidence, seenEvidence); err != nil {
			return err
		}
	}
	return nil
}

// validateContextualEvidenceAgreement requires context_only to agree with the
// evidence list.
func validateContextualEvidenceAgreement(response TranslationResponse) error {
	if response.ContextOnly != (len(response.EvidenceIDs) == 0) {
		return errors.New("decode LLM translation: context_only contradicts evidence IDs")
	}
	return nil
}

// validateEvidenceID rejects an evidence ID that is blank, unknown, or already
// seen in this response.
func validateEvidenceID(id string, knownEvidence, seenEvidence map[string]struct{}) error {
	if id == "" {
		return errors.New("decode LLM translation: empty evidence ID")
	}
	if _, ok := knownEvidence[id]; !ok {
		return errors.New("decode LLM translation: unknown evidence ID")
	}
	if _, duplicate := seenEvidence[id]; duplicate {
		return errors.New("decode LLM translation: duplicate evidence ID")
	}
	seenEvidence[id] = struct{}{}
	return nil
}

// validateContextualOutcome requires a complete abstention when there is no
// gloss. A resolved gloss's stray unresolved reason is discarded during
// normalization, so only the abstention branch remains here.
func validateContextualOutcome(response TranslationResponse) error {
	if response.Gloss == "" {
		if !response.ContextOnly || len(response.EvidenceIDs) != 0 || response.UnresolvedReason == "" {
			return errors.New("decode LLM translation: unresolved meaning is incomplete")
		}
	}
	return nil
}

// reconcileSenseSelection applies the sense-selection warning and fallback-gloss
// rules: an invalid selection falls back to deterministic order, an over-limit
// selection is truncated, and fallback glosses are bounded and mutually
// exclusive with a sense selection.
func reconcileSenseSelection(input TranslationRequest, response TranslationResponse) TranslationResponse {
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
	return response
}

func hasWarning(warnings []string, want string) bool {
	return slices.Contains(warnings, want)
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
		if err := decoder.Decode(&raw); err != nil {
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
