package enrichment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/justin-hayes/mouseion/internal/textmatch"
)

// fluencyAgreementPromptVersion pins the Fluency agreement contract: one
// occurrence of a mature word, the meaning the learner learned for the word,
// and an answer giving the reason this occurrence does or does not use that meaning (ADR 0134 §7,
// spec #2691).
const fluencyAgreementPromptVersion = "fluency-agreement-v2"

// The reasons a Fluency agreement answer gives. Only FluencyReasonSameMeaning
// agrees with the learned meaning; the others are evidence of the word's other
// uses and are never shown.
const (
	FluencyReasonSameMeaning      = "same-meaning"
	FluencyReasonDifferentMeaning = "different-meaning"
	FluencyReasonIdiomatic        = "idiomatic"
	FluencyReasonNotThisLemma     = "not-this-lemma"
	FluencyReasonUndefendable     = "undefendable"
)

// fluencyReasons is the closed set of reasons the codec accepts.
var fluencyReasons = []string{
	FluencyReasonSameMeaning, FluencyReasonDifferentMeaning, FluencyReasonIdiomatic,
	FluencyReasonNotThisLemma, FluencyReasonUndefendable,
}

// FluencyAgreementItemHandle is the short per-request item handle a Fluency
// agreement payload transmits as item_id.
const FluencyAgreementItemHandle = "f1"

// fluencyAgreementSystemPrompt is the strict Fluency agreement contract. The
// payload carries the lemma, its part of speech, the word as it appears, the
// sentence and the learned meaning: a short English Gloss the same provider gave
// the word when it was introduced. It carries no Book, learner or passage data.
var fluencyAgreementSystemPrompt = "You are checking whether one occurrence of a word uses the meaning a learner already knows for it. The payload carries a lemma with its part of speech, the word as it appears in a sentence, the sentence, and learned_meaning: a short English gloss for the lemma. Decide whether, in this sentence, the word has that meaning. Return exactly one JSON object using only these five fields: item_id, reason, gloss, sentence_translation, sentence_translation_targets. reason is exactly one of: same-meaning, when the word in this sentence has the same meaning as learned_meaning; different-meaning, when it has a different sense; idiomatic, when it is a figurative or idiomatic use that changes the meaning; not-this-lemma, when the occurrence is not the lemma; undefendable, when you cannot defend any of the others. gloss is one brief English cue for the word's meaning in this sentence, required for every reason. sentence_translation is a natural English translation of the complete sentence, required for every reason. sentence_translation_targets is an ordered array of exact plain-text excerpt(s) from sentence_translation that identify only the target word; use an empty array when no reliable alignment exists. Echo item_id exactly. Do not return HTML or markup."

// fluencyAgreementPayloadFields is the exact ordered field set a Fluency
// agreement request transmits in its user message. It is the reviewed contract
// the provider payload assessment pins before any request is sent.
var fluencyAgreementPayloadFields = []string{
	"item_id", "language", "target_language", "canonical_lemma", "upos",
	"target_word", "example_sentence", "learned_meaning",
}

// FluencyAgreementPayloadFields returns a copy of the ordered field set a
// Fluency agreement request transmits.
func FluencyAgreementPayloadFields() []string {
	return slices.Clone(fluencyAgreementPayloadFields)
}

// FluencyAgreementPromptContract returns the versioned Fluency agreement prompt
// contract alone, without the configured model, so the payload assessment is
// stable across model changes.
func (c *TranslationCodec) FluencyAgreementPromptContract() string {
	return fluencyAgreementPromptVersion
}

func (c *TranslationCodec) encodeFluencyAgreementRequest(input TranslationRequest) ([]byte, error) {
	if strings.TrimSpace(input.Language) == "" || strings.TrimSpace(input.CanonicalLemma) == "" || strings.TrimSpace(input.UPOS) == "" ||
		strings.TrimSpace(input.TargetWord) == "" || strings.TrimSpace(input.ExampleSentence) == "" || strings.TrimSpace(input.LearnedMeaning) == "" {
		return nil, errors.New("encode fluency agreement input: lemma, part of speech, word, sentence and learned meaning are required")
	}
	privateInput, err := json.Marshal(struct {
		ItemID          string `json:"item_id"`
		Language        string `json:"language"`
		TargetLanguage  string `json:"target_language"`
		CanonicalLemma  string `json:"canonical_lemma"`
		UPOS            string `json:"upos"`
		TargetWord      string `json:"target_word"`
		ExampleSentence string `json:"example_sentence"`
		LearnedMeaning  string `json:"learned_meaning"`
	}{FluencyAgreementItemHandle, input.Language, translationTargetLanguage(input), input.CanonicalLemma, input.UPOS, input.TargetWord, input.ExampleSentence, input.LearnedMeaning})
	if err != nil {
		return nil, fmt.Errorf("encode fluency agreement input: %w", err)
	}
	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: fluencyAgreementSystemPrompt},
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
		return nil, fmt.Errorf("encode fluency agreement request: %w", err)
	}
	return body, nil
}

// decodeFluencyAgreement decodes the strict Fluency agreement answer. Duplicate
// keys, unknown fields, trailing content, a mismatched item_id and markup are
// rejected.
func decodeFluencyAgreement(envelope chatCompletionEnvelope) (TranslationResponse, TranslationUsage, error) {
	content := envelope.Choices[0].Message.Content
	if duplicate, err := hasDuplicateObjectKey(content); err != nil {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("decode fluency agreement: %w", err)
	} else if duplicate {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode fluency agreement: duplicate field")
	}
	var result struct {
		ItemID                     string   `json:"item_id"`
		Reason                     string   `json:"reason"`
		Gloss                      string   `json:"gloss"`
		SentenceTranslation        string   `json:"sentence_translation"`
		SentenceTranslationTargets []string `json:"sentence_translation_targets"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("decode fluency agreement: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode fluency agreement: trailing JSON content")
	}
	if strings.TrimSpace(result.ItemID) != FluencyAgreementItemHandle {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode fluency agreement: item_id mismatch")
	}
	if !slices.Contains(fluencyReasons, result.Reason) {
		return TranslationResponse{}, TranslationUsage{}, fmt.Errorf("decode fluency agreement: reason %q is not one of %s", result.Reason, strings.Join(fluencyReasons, ", "))
	}
	if hasMarkup(result.Gloss) || hasMarkup(result.SentenceTranslation) {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode fluency agreement: HTML or markup is not allowed")
	}
	response := TranslationResponse{
		Gloss:                      strings.TrimSpace(result.Gloss),
		SentenceTranslation:        strings.TrimSpace(result.SentenceTranslation),
		FluencyReason:              result.Reason,
		SentenceTranslationTargets: ValidateSentenceTranslationTargets(strings.TrimSpace(result.SentenceTranslation), result.SentenceTranslationTargets),
	}
	if response.Gloss == "" || response.SentenceTranslation == "" {
		return TranslationResponse{}, TranslationUsage{}, errors.New("decode fluency agreement: every answer needs a gloss and a sentence translation")
	}
	return response, envelope.Usage, nil
}

// FluencyAgreement is the outcome of one Fluency agreement request: the reason
// the occurrence does or does not use the learned meaning, with its own
// contextual Gloss and sentence translation whichever it is. A disagreeing
// answer's Gloss and translation are evidence of the word's other uses.
type FluencyAgreement struct {
	Reason                     string
	Gloss                      string
	SentenceTranslation        string
	SentenceTranslationTargets []string
}

// Agrees reports whether the occurrence uses the learned meaning.
func (a FluencyAgreement) Agrees() bool { return a.Reason == FluencyReasonSameMeaning }

// EnrichFluencyAgreement asks the provider whether one occurrence of a mature
// word, given as the candidate's word and example sentence, uses the learned
// meaning. It returns ErrExternalNotConfigured, not an agreement, when external
// enrichment is off. A provider failure is returned as an error and is
// retryable; a disagreement is an answer, with a reason, and is not an error. The caller checks
// the payload assessment before calling, so nothing is transmitted without it.
func (s *Service) EnrichFluencyAgreement(ctx context.Context, c Candidate, learnedMeaning string) (FluencyAgreement, error) {
	if !s.ExternalConfigured() {
		return FluencyAgreement{}, ErrExternalNotConfigured
	}
	request := TranslationRequest{
		Language: c.Language, TargetLanguage: "en", CanonicalLemma: c.CanonicalLemma, UPOS: strings.ToUpper(c.UPOS),
		TargetWord: textmatch.CleanLexicalSurface(c.TargetWord), ExampleSentence: c.ExampleSentence,
		LearnedMeaning: learnedMeaning, RequireFluencyAgreement: true,
	}
	return s.fluencyAgreementOf(ctx, request)
}

// fluencyAgreementOf sends one Fluency agreement request, either contract, and
// reads its answer.
func (s *Service) fluencyAgreementOf(ctx context.Context, request TranslationRequest) (FluencyAgreement, error) {
	var metrics ExternalMetrics
	response, err := s.callTranslationWithRetry(ctx, request, &metrics)
	if err != nil {
		return FluencyAgreement{}, err
	}
	gloss, translation := strings.TrimSpace(response.Gloss), strings.TrimSpace(response.SentenceTranslation)
	if gloss == "" || translation == "" {
		return FluencyAgreement{}, fmt.Errorf("%w: every answer needs a gloss and a sentence translation", ErrInvalidTranslationResponse)
	}
	return FluencyAgreement{
		Reason: response.FluencyReason, Gloss: gloss, SentenceTranslation: translation,
		SentenceTranslationTargets: ValidateSentenceTranslationTargets(translation, response.SentenceTranslationTargets),
	}, nil
}

// fluencyAgreementSourceEvidencePromptVersion pins the source-evidence variant
// of the Fluency agreement contract (ADR 0138 §4). It is a contract of its own:
// the modern contract, its fields and its recorded assessment are unchanged.
const fluencyAgreementSourceEvidencePromptVersion = "fluency-agreement-source-evidence-v1"

// requiresFluencyAgreement reports whether the request is a Fluency agreement of
// either contract, whose answer has one shape.
func (r TranslationRequest) requiresFluencyAgreement() bool {
	return r.RequireFluencyAgreement || r.RequireSourceEvidenceFluencyAgreement
}

// FluencySense is one dictionary sense a Fluency agreement variant request
// sends: its native sense ID and its definition.
type FluencySense struct {
	ID         string `json:"sense_id"`
	Definition string `json:"definition"`
}

var fluencyAgreementSourceEvidenceSystemPrompt = "You are checking whether one occurrence of a word uses the meaning a learner already knows for it. The payload carries a lemma with its part of speech, the word as it appears in a sentence, the sentence, learned_meaning: a short English gloss for the lemma, learned_senses: the dictionary senses, each with its definition, that learned_meaning rests on, and context_senses: the dictionary senses, each with its definition, that the word's gloss in this sentence rests on. Decide whether, in this sentence, the word has the learned meaning, judging by the definitions of the cited senses. Return exactly one JSON object using only these five fields: item_id, reason, gloss, sentence_translation, sentence_translation_targets. reason is exactly one of: same-meaning, when the word in this sentence has the same meaning as learned_meaning; different-meaning, when it has a different sense; idiomatic, when it is a figurative or idiomatic use that changes the meaning; not-this-lemma, when the occurrence is not the lemma; undefendable, when you cannot defend any of the others. gloss is one brief English cue for the word's meaning in this sentence, required for every reason. sentence_translation is a natural English translation of the complete sentence, required for every reason. sentence_translation_targets is an ordered array of exact plain-text excerpt(s) from sentence_translation that identify only the target word; use an empty array when no reliable alignment exists. Echo item_id exactly. Do not return HTML or markup."

// fluencyAgreementSourceEvidencePayloadFields is the exact ordered field set the
// variant transmits: the modern fields plus both sides' cited senses.
var fluencyAgreementSourceEvidencePayloadFields = []string{
	"item_id", "language", "target_language", "canonical_lemma", "upos",
	"target_word", "example_sentence", "learned_meaning", "learned_senses", "context_senses",
}

// FluencyAgreementSourceEvidencePayloadFields returns a copy of the ordered
// field set the source-evidence variant transmits.
func FluencyAgreementSourceEvidencePayloadFields() []string {
	return slices.Clone(fluencyAgreementSourceEvidencePayloadFields)
}

// FluencyAgreementSourceEvidencePromptContract returns the versioned variant
// prompt contract alone, without the configured model.
func (c *TranslationCodec) FluencyAgreementSourceEvidencePromptContract() string {
	return fluencyAgreementSourceEvidencePromptVersion
}

func (c *TranslationCodec) encodeSourceEvidenceFluencyAgreementRequest(input TranslationRequest) ([]byte, error) {
	if strings.TrimSpace(input.Language) == "" || strings.TrimSpace(input.CanonicalLemma) == "" || strings.TrimSpace(input.UPOS) == "" ||
		strings.TrimSpace(input.TargetWord) == "" || strings.TrimSpace(input.ExampleSentence) == "" || strings.TrimSpace(input.LearnedMeaning) == "" ||
		len(input.LearnedSenses) == 0 || len(input.ContextSenses) == 0 {
		return nil, errors.New("encode fluency agreement input: lemma, part of speech, word, sentence, learned meaning and both sides' cited senses are required")
	}
	privateInput, err := json.Marshal(struct {
		ItemID          string         `json:"item_id"`
		Language        string         `json:"language"`
		TargetLanguage  string         `json:"target_language"`
		CanonicalLemma  string         `json:"canonical_lemma"`
		UPOS            string         `json:"upos"`
		TargetWord      string         `json:"target_word"`
		ExampleSentence string         `json:"example_sentence"`
		LearnedMeaning  string         `json:"learned_meaning"`
		LearnedSenses   []FluencySense `json:"learned_senses"`
		ContextSenses   []FluencySense `json:"context_senses"`
	}{FluencyAgreementItemHandle, input.Language, translationTargetLanguage(input), input.CanonicalLemma, input.UPOS, input.TargetWord, input.ExampleSentence, input.LearnedMeaning, input.LearnedSenses, input.ContextSenses})
	if err != nil {
		return nil, fmt.Errorf("encode fluency agreement input: %w", err)
	}
	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: fluencyAgreementSourceEvidenceSystemPrompt},
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
		return nil, fmt.Errorf("encode fluency agreement request: %w", err)
	}
	return body, nil
}

// EnrichSourceEvidenceFluencyAgreement is EnrichFluencyAgreement for a
// source-evidence language: it also sends the dictionary senses, with their
// definitions, that the learned meaning and the occurrence's Gloss cite. Both
// sides must cite something. The caller checks the variant's payload assessment
// before calling, so nothing is transmitted without it.
func (s *Service) EnrichSourceEvidenceFluencyAgreement(ctx context.Context, c Candidate, learnedMeaning string, learned, occurrence []FluencySense) (FluencyAgreement, error) {
	if !s.ExternalConfigured() {
		return FluencyAgreement{}, ErrExternalNotConfigured
	}
	request := TranslationRequest{
		Language: c.Language, TargetLanguage: "en", CanonicalLemma: c.CanonicalLemma, UPOS: strings.ToUpper(c.UPOS),
		TargetWord: textmatch.CleanLexicalSurface(c.TargetWord), ExampleSentence: c.ExampleSentence,
		LearnedMeaning: learnedMeaning, RequireSourceEvidenceFluencyAgreement: true,
		LearnedSenses: slices.Clone(learned), ContextSenses: slices.Clone(occurrence),
	}
	return s.fluencyAgreementOf(ctx, request)
}
