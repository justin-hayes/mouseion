package enrichment

import (
	"errors"
	"fmt"
	"slices"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/cardpolicy"
)

// ErrProviderAssessmentNotRecorded marks a provider call whose exact payload
// has not been reviewed and recorded for personal use. Callers fail closed: no
// request is transmitted.
var ErrProviderAssessmentNotRecorded = errors.New("enrichment: provider payload assessment is not recorded")

// ProviderPayloadKind names one exact external-provider payload an assessment
// covers. A recorded assessment answers for its own kind alone: the same
// language and provider never carry over to a different payload.
type ProviderPayloadKind string

const (
	// LatinEvidenceAssistedPassPayload is the Classical Latin evidence-assisted
	// pass: the representative sentence, its passage context, and the pinned
	// Lewis & Short senses as short evidence handles that each name their entry
	// (ADR 0138 §1). It has its own contract and its own record (#2766).
	LatinEvidenceAssistedPassPayload ProviderPayloadKind = "latin-evidence-assisted-pass"
	// GreekEntryEvidenceAssistedPassPayload is the Ancient Greek evidence-assisted
	// pass of Study: the representative sentence, its passage context, and the
	// pinned LSJ senses of every homograph entry as short evidence handles that
	// each name their entry (ADR 0139). It has its own contract and its own record (#2774).
	GreekEntryEvidenceAssistedPassPayload ProviderPayloadKind = "greek-entry-evidence-assisted-pass" //nolint:gosec // a payload kind name, not a credential
	// IdiomCardPayload is the dictionary-anchored idiom card: the headword,
	// matched forms, representative sentence and bounded candidate senses alone
	// (ADR 0114).
	IdiomCardPayload ProviderPayloadKind = "idiom-card"
	// FluencyAgreementPayload is the Study Fluency agreement: the lemma, its
	// part of speech, the word as it appears, one library sentence and the
	// learned meaning alone (ADR 0134 §7).
	FluencyAgreementPayload ProviderPayloadKind = "fluency-agreement"
	// FluencyAgreementSourceEvidencePayload is the separate Fluency agreement
	// variant for a source-evidence language: the modern payload's fields plus
	// the dictionary senses, with their definitions, that the Learned meaning
	// and the occurrence's Gloss cite (ADR 0138 §4). Greek's (#2765) and Latin's
	// (#2766) are recorded; no other language's is, so it sends nothing for them.
	FluencyAgreementSourceEvidencePayload ProviderPayloadKind = "fluency-agreement-source-evidence"
)

// ProviderAssessment is the reviewed personal-use disposition for sending one
// language's exact external-provider payload to the configured provider. It is
// compiled reviewed data, not runtime configuration: changing the provider, the
// prompt contract, or the transmitted field set invalidates the recorded
// assessment and must fail closed until it is re-recorded.
type ProviderAssessment struct {
	// Kind names the payload this assessment covers.
	Kind ProviderPayloadKind
	// Language is the dedicated language the assessment covers.
	Language string
	// Provider and PromptVersion pin the reviewed contract.
	Provider      string
	PromptVersion string
	// Fields lists the exact transmitted fields, in order, for the record.
	Fields []string
	// Disposition is the recorded human disposition for this payload.
	Disposition string
	// Ticket is the issue where the disposition was recorded.
	Ticket string
	// Recorded reports whether the disposition has been recorded.
	Recorded bool
}

// idiomCardAssessment is the reviewed personal-use disposition for the exact
// idiom-card payload: the headword, matched forms, representative sentence and
// bounded candidate senses alone. It is recorded for Italian by the passing
// review in doc/reviews/idiom-review-v3-gate2-2026-11-08.md under the ADR 0115
// gate. Any other language still fails closed and sends nothing (ADR 0114).
var idiomCardAssessment = ProviderAssessment{
	Kind:          IdiomCardPayload,
	Language:      "it",
	Provider:      "openai-compatible",
	PromptVersion: "idiom-card-v3",
	Fields: []string{
		"item_id", "language", "target_language", "headword", "matched_forms",
		"example_sentence", "candidate_senses",
	},
	Disposition: "approved for personal use after the Italian idiom review passed",
	Ticket:      "#2389",
	Recorded:    true,
}

// fluencyAgreementAssessment is the disposition for the exact German Fluency
// agreement payload: the lemma, part of speech, the word as it appears, one
// library sentence and the learned meaning. Justin reviewed that payload in
// doc/reviews/fluency-agreement-payload-assessment.md and recorded the
// disposition on #2702, so Fluency contexts are prepared for German (ADR 0133,
// ADR 0134 §7).
//
// PromptVersion and Fields are written out as the recorded literals, not taken
// from the codec, so the record is a real pin: changing the prompt contract or
// the payload without re-recording the disposition fails the gate and sends
// nothing.
var fluencyAgreementAssessment = ProviderAssessment{
	Kind:          FluencyAgreementPayload,
	Language:      "de",
	Provider:      "openai-compatible",
	PromptVersion: "fluency-agreement-v2",
	Fields: []string{
		"item_id", "language", "target_language", "canonical_lemma", "upos",
		"target_word", "example_sentence", "learned_meaning",
	},
	Disposition: "approved for personal use of the locally produced Study surface",
	Ticket:      "#2702",
	Recorded:    true,
}

// fluencyAgreementSourceEvidenceGreekAssessment is the disposition for the exact
// Ancient Greek source-evidence Fluency agreement payload: the modern Fluency
// fields plus the definitions of the LSJ senses that the Learned meaning
// and the occurrence's Gloss cite. Justin reviewed that payload in
// doc/reviews/fluency-agreement-source-evidence-greek-payload-assessment.md and
// recorded the disposition on #2765, so Greek Fluency can send it (ADR 0138 §4).
//
// The transport is the configured OpenAI-compatible Chat Completions endpoint.
// The cited senses are sent as definition strings alone: no dictionary sense ID,
// native or handle, is transmitted, because the answer cites no senses. No
// learner identity, Book title or review history is transmitted.
//
// PromptVersion and Fields are written out as the recorded literals, not taken
// from the codec, so the record is a real pin: changing the prompt contract or
// the payload without re-recording the disposition fails the gate and sends
// nothing.
var fluencyAgreementSourceEvidenceGreekAssessment = ProviderAssessment{
	Kind:          FluencyAgreementSourceEvidencePayload,
	Language:      "grc",
	Provider:      "openai-compatible",
	PromptVersion: "fluency-agreement-source-evidence-v2",
	Fields: []string{
		"item_id", "language", "target_language", "canonical_lemma", "upos",
		"target_word", "example_sentence", "learned_meaning", "learned_senses", "context_senses",
	},
	Disposition: "approved for personal use of the locally produced Study surface",
	Ticket:      "#2765",
	Recorded:    true,
}

// latinEvidenceAssistedPassAssessment is the disposition for the exact Classical
// Latin evidence-assisted pass payload: the representative sentence, its passage
// context (the Perseus reference translation) and the pinned Lewis & Short
// definitions as short evidence handles, each beside the short handle of its
// entry. Justin reviewed that payload in
// doc/reviews/latin-evidence-assisted-pass-payload-assessment.md and recorded the
// disposition on #2766, so the pass runs for Latin Study material with a
// configured provider (ADR 0138 §1).
//
// The transport is the configured OpenAI-compatible Chat Completions endpoint.
// No learner identity, Book title, whole Book, raw annotation, lexical
// discriminator or native Lewis & Short entry or sense ID is transmitted.
//
// PromptVersion and Fields are written out as the recorded literals, not taken
// from the codec, so the record is a real pin: changing the prompt contract or
// the payload without re-recording the disposition fails the gate and sends
// nothing.
//
// The record was re-recorded on #2781 for the citation-form contract of #2779
// (v21): only the prompt changed, the field set and transport are the same, and
// the v18 contract is refused. See the addendum in the review document.
var latinEvidenceAssistedPassAssessment = ProviderAssessment{
	Kind:          LatinEvidenceAssistedPassPayload,
	Language:      "la",
	Provider:      "openai-compatible",
	PromptVersion: "translation-v21-evidence-assisted-latin-citation-form",
	Fields: []string{
		"item_id", "language", "target_language", "canonical_lemma", "upos",
		"target_word", "example_sentence", "passage_context", "candidate_senses",
	},
	Disposition: "approved for personal use of the locally produced deck and Study surface",
	Ticket:      "#2781",
	Recorded:    true,
}

// greekEntryEvidenceAssistedPassAssessment is the disposition for the exact
// Ancient Greek Study evidence-assisted pass payload: the representative
// sentence, its passage context (the Perseus reference translation) and the
// pinned LSJ definitions of every homograph entry as short evidence handles,
// each beside the short handle of its entry. Justin reviewed that payload in
// doc/reviews/greek-entry-evidence-assisted-pass-payload-assessment.md and
// recorded the disposition on #2774, so Greek Study material can send it
// (ADR 0139).
//
// The transport is the configured OpenAI-compatible Chat Completions endpoint.
// No learner identity, Book title, whole Book, raw annotation, lexical
// discriminator or native LSJ entry or sense ID is transmitted.
//
// PromptVersion and Fields are written out as the recorded literals, not taken
// from the codec, so the record is a real pin: changing the prompt contract or
// the payload without re-recording the disposition fails the gate and sends
// nothing.
//
// The record was re-recorded on #2781 for the citation-form contract of #2779
// (v20): only the prompt changed, the field set and transport are the same, and
// the v19 contract is refused. See the addendum in the review document.
var greekEntryEvidenceAssistedPassAssessment = ProviderAssessment{
	Kind:          GreekEntryEvidenceAssistedPassPayload,
	Language:      "grc",
	Provider:      "openai-compatible",
	PromptVersion: "translation-v20-evidence-assisted-greek-entries-citation-form",
	Fields: []string{
		"item_id", "language", "target_language", "canonical_lemma", "upos",
		"target_word", "example_sentence", "passage_context", "candidate_senses",
	},
	Disposition: "approved for personal use of the locally produced Study surface",
	Ticket:      "#2781",
	Recorded:    true,
}

// fluencyAgreementSourceEvidenceLatinAssessment is the disposition for the exact
// Classical Latin source-evidence Fluency agreement payload: the modern Fluency
// fields plus the Lewis & Short definitions of the senses that the Learned
// meaning and the occurrence's Gloss cite. Justin reviewed that payload in
// doc/reviews/fluency-agreement-source-evidence-latin-payload-assessment.md and
// recorded the disposition on #2766, so Latin Fluency can send it (ADR 0138 §4).
//
// The transport is the configured OpenAI-compatible Chat Completions endpoint.
// The cited senses are sent as definition strings alone: no dictionary entry or
// sense ID, native or handle, is transmitted. No learner identity, Book title or
// review history is transmitted.
//
// PromptVersion and Fields are written out as the recorded literals, not taken
// from the codec, so the record is a real pin: changing the prompt contract or
// the payload without re-recording the disposition fails the gate and sends
// nothing.
var fluencyAgreementSourceEvidenceLatinAssessment = ProviderAssessment{
	Kind:          FluencyAgreementSourceEvidencePayload,
	Language:      "la",
	Provider:      "openai-compatible",
	PromptVersion: "fluency-agreement-source-evidence-v2",
	Fields: []string{
		"item_id", "language", "target_language", "canonical_lemma", "upos",
		"target_word", "example_sentence", "learned_meaning", "learned_senses", "context_senses",
	},
	Disposition: "approved for personal use of the locally produced Study surface",
	Ticket:      "#2766",
	Recorded:    true,
}

// ProviderAssessmentSet is the recorded personal-use payload assessments. Its
// one question decides whether an assessed payload may be sent.
type ProviderAssessmentSet []ProviderAssessment

// ProviderAssessments returns the compiled reviewed personal-use payload
// assessments. A caller that can install a recorded assessment for a test
// passes its own set to MaySend instead.
func ProviderAssessments() ProviderAssessmentSet {
	return ProviderAssessmentSet{idiomCardAssessment, fluencyAgreementAssessment, fluencyAgreementSourceEvidenceGreekAssessment, latinEvidenceAssistedPassAssessment, fluencyAgreementSourceEvidenceLatinAssessment, greekEntryEvidenceAssistedPassAssessment}
}

// MaySend reports whether one payload kind may be sent for a language with the
// configured codec. The provider name and the current prompt contract and field
// set come from the codec, never from the record, so a drifted record fails
// closed until it is re-recorded. A missing codec, an unrecorded assessment, a
// language with no record for that kind, or any provider, prompt version or
// field list mismatch returns ErrProviderAssessmentNotRecorded. A payload that
// is not assessed does not consult this set.
func (s ProviderAssessmentSet) MaySend(kind ProviderPayloadKind, language string, codec *TranslationCodec) error {
	contract, ok := currentPayloadContract(kind, codec)
	if !ok {
		return fmt.Errorf("%w: no current contract for %s", ErrProviderAssessmentNotRecorded, kind)
	}
	covered := false
	var lastErr error
	for i := range s {
		assessment := s[i]
		if assessment.Kind != kind || !sameAssessmentLanguage(language, assessment.Language) {
			continue
		}
		covered = true
		if err := assessment.allows(contract); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if covered {
		return lastErr
	}
	return fmt.Errorf("%w: no recorded %s assessment for %s", ErrProviderAssessmentNotRecorded, kind, language)
}

// payloadContract is the provider name and the current prompt contract and
// transmitted field set one payload kind requires.
type payloadContract struct {
	provider      string
	promptVersion string
	fields        []string
}

// currentPayloadContract returns the current contract for one payload kind, read
// from the codec's own constants. It reports false when the codec is missing or
// the kind has no contract.
func currentPayloadContract(kind ProviderPayloadKind, codec *TranslationCodec) (payloadContract, bool) {
	if codec == nil {
		return payloadContract{}, false
	}
	switch kind {
	case LatinEvidenceAssistedPassPayload:
		return payloadContract{codec.ProviderName(), codec.LatinEvidenceAssistedPassPromptContract(), LatinEvidenceAssistedPassPayloadFields()}, true
	case GreekEntryEvidenceAssistedPassPayload:
		return payloadContract{codec.ProviderName(), codec.GreekEntryEvidenceAssistedPassPromptContract(), GreekEntryEvidenceAssistedPassPayloadFields()}, true
	case IdiomCardPayload:
		return payloadContract{codec.ProviderName(), codec.IdiomCardPromptContract(), IdiomCardPayloadFields()}, true
	case FluencyAgreementPayload:
		return payloadContract{codec.ProviderName(), codec.FluencyAgreementPromptContract(), FluencyAgreementPayloadFields()}, true
	case FluencyAgreementSourceEvidencePayload:
		return payloadContract{codec.ProviderName(), codec.FluencyAgreementSourceEvidencePromptContract(), FluencyAgreementSourceEvidencePayloadFields()}, true
	default:
		return payloadContract{}, false
	}
}

// allows reports whether one record pins the given contract exactly.
func (a ProviderAssessment) allows(contract payloadContract) error {
	if !a.Recorded {
		return fmt.Errorf("%w: %s (%s)", ErrProviderAssessmentNotRecorded, a.Ticket, a.Disposition)
	}
	if contract.provider != a.Provider || contract.promptVersion != a.PromptVersion {
		return fmt.Errorf("%w: configured provider or prompt does not match the recorded %s assessment for %s (%s)", ErrProviderAssessmentNotRecorded, a.Kind, a.Language, a.Ticket)
	}
	if !slices.Equal(contract.fields, a.Fields) {
		return fmt.Errorf("%w: transmitted field set does not match the recorded %s assessment for %s (%s)", ErrProviderAssessmentNotRecorded, a.Kind, a.Language, a.Ticket)
	}
	return nil
}

// sameAssessmentLanguage compares two language tags by their lowercase base
// language.
func sameAssessmentLanguage(left, right string) bool {
	return canonicalization.NormalizeLanguage(left) == canonicalization.NormalizeLanguage(right)
}

// StudyEvidenceAssistedPassKind is the payload kind Study's evidence-assisted
// pass transmits for a source-evidence language, selected by its card policy. A
// language with no pass contract has no kind, so MaySend refuses it.
func StudyEvidenceAssistedPassKind(language string) ProviderPayloadKind {
	switch passContractOf(language) {
	case cardpolicy.PassContractLatin:
		return LatinEvidenceAssistedPassPayload
	case cardpolicy.PassContractGreekEntries:
		return GreekEntryEvidenceAssistedPassPayload
	case cardpolicy.PassContractNone:
	}
	return ""
}

// CheckPassAssessment refuses a request for an evidence-assisted pass whose
// payload contract the compiled assessments do not record for the codec's
// current prompt version. It is the last check before a pass request is
// encoded for the provider, so work frozen before a contract changed (a
// retried Study item) sends nothing under the new, unrecorded contract (#2779).
// A request that is not a pass is never refused.
func CheckPassAssessment(codec *TranslationCodec, input TranslationRequest) error {
	if !input.RequireEvidenceAssistedPass {
		return nil
	}
	return ProviderAssessments().MaySend(StudyEvidenceAssistedPassKind(input.Language), input.Language, codec)
}
