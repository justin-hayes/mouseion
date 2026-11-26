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
	// EvidenceAssistedPassPayload is the Ancient Greek evidence-assisted pass:
	// the representative sentence, its passage context, and the pinned LSJ
	// definitions as short evidence handles (ADR 0101, ADR 0105).
	EvidenceAssistedPassPayload ProviderPayloadKind = "evidence-assisted-pass"
	// LatinEvidenceAssistedPassPayload is the Classical Latin evidence-assisted
	// pass: the representative sentence, its passage context, and the pinned
	// Lewis & Short senses as short evidence handles that each name their entry
	// (ADR 0138 §1). It has its own contract and its own record (#2766).
	LatinEvidenceAssistedPassPayload ProviderPayloadKind = "latin-evidence-assisted-pass"
	// GreekEntryEvidenceAssistedPassPayload is the Ancient Greek evidence-assisted
	// pass of Study: the representative sentence, its passage context, and the
	// pinned LSJ senses of every homograph entry as short evidence handles that
	// each name their entry (ADR 0139). It has its own contract and, until its
	// assessment is recorded (#2774), sends nothing.
	GreekEntryEvidenceAssistedPassPayload ProviderPayloadKind = "greek-entry-evidence-assisted-pass" //nolint:gosec // a payload kind name, not a credential
	// LSJSenseSummaryPayload is the context-free summary of a pinned LSJ entry:
	// its headword, sense IDs and English glosses alone (ADR 0101).
	LSJSenseSummaryPayload ProviderPayloadKind = "lsj-sense-summary"
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

// evidenceAssistedPassAssessment is the reviewed personal-use disposition for
// the exact Ancient Greek evidence-assisted pass payload: the representative
// sentence, its passage context, and the pinned LSJ definitions as short
// evidence handles. Justin reviewed that payload and recorded the disposition on
// #2171, so the pass runs for every Greek deck with a configured provider
// (ADR 0101, ADR 0105).
//
// The transport is the configured OpenAI-compatible Chat Completions endpoint.
// No learner identity, Book title, whole Book, raw annotation, lexical
// discriminator or native LSJ ID is transmitted.
//
// PromptVersion and Fields are written out as the recorded literals, not taken
// from the codec, so the record is a real pin: changing the prompt contract or
// the payload without re-recording the disposition fails the gate and sends
// nothing.
var evidenceAssistedPassAssessment = ProviderAssessment{
	Kind:          EvidenceAssistedPassPayload,
	Language:      "grc",
	Provider:      "openai-compatible",
	PromptVersion: "translation-v16-evidence-assisted-greek",
	Fields: []string{
		"item_id", "language", "target_language", "canonical_lemma", "upos",
		"target_word", "example_sentence", "passage_context", "candidate_senses",
	},
	Disposition: "approved for personal use of the locally produced deck",
	Ticket:      "#2171",
	Recorded:    true,
}

// lsjSenseSummaryAssessment is the reviewed personal-use disposition for the
// exact Ancient Greek LSJ sense summary payload: the pinned LSJ entry's
// headword, its sense IDs and its English glosses alone. Justin reviewed that
// payload and recorded the disposition on #2172, so the pass also asks for a
// context-free summary of each pinned entry (ADR 0101).
//
// The transport is the configured OpenAI-compatible Chat Completions endpoint.
// No learner identity, Book title, representative sentence, whole Book, raw
// annotation, lexical discriminator or Brownson passage is transmitted.
//
// PromptVersion and Fields are written out as the recorded literals, not taken
// from the codec, so the record is a real pin: changing the prompt contract or
// the payload without re-recording the disposition fails the gate and sends
// nothing.
var lsjSenseSummaryAssessment = ProviderAssessment{
	Kind:          LSJSenseSummaryPayload,
	Language:      "grc",
	Provider:      "openai-compatible",
	PromptVersion: "translation-v17-lsj-sense-summary-greek",
	Fields: []string{
		"item_id", "headword", "senses",
	},
	Disposition: "approved for personal use of the locally produced deck",
	Ticket:      "#2172",
	Recorded:    true,
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
// disposition on #2766, so the pass runs for Latin Prepared decks and Study
// material with a configured provider (ADR 0138 §1).
//
// The transport is the configured OpenAI-compatible Chat Completions endpoint.
// No learner identity, Book title, whole Book, raw annotation, lexical
// discriminator or native Lewis & Short entry or sense ID is transmitted.
//
// PromptVersion and Fields are written out as the recorded literals, not taken
// from the codec, so the record is a real pin: changing the prompt contract or
// the payload without re-recording the disposition fails the gate and sends
// nothing.
var latinEvidenceAssistedPassAssessment = ProviderAssessment{
	Kind:          LatinEvidenceAssistedPassPayload,
	Language:      "la",
	Provider:      "openai-compatible",
	PromptVersion: "translation-v18-evidence-assisted-latin",
	Fields: []string{
		"item_id", "language", "target_language", "canonical_lemma", "upos",
		"target_word", "example_sentence", "passage_context", "candidate_senses",
	},
	Disposition: "approved for personal use of the locally produced deck and Study surface",
	Ticket:      "#2766",
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
	return ProviderAssessmentSet{evidenceAssistedPassAssessment, lsjSenseSummaryAssessment, idiomCardAssessment, fluencyAgreementAssessment, fluencyAgreementSourceEvidenceGreekAssessment, latinEvidenceAssistedPassAssessment, fluencyAgreementSourceEvidenceLatinAssessment}
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
	case EvidenceAssistedPassPayload:
		return payloadContract{codec.ProviderName(), codec.EvidenceAssistedPassPromptContract(), EvidenceAssistedPassPayloadFields()}, true
	case LatinEvidenceAssistedPassPayload:
		return payloadContract{codec.ProviderName(), codec.LatinEvidenceAssistedPassPromptContract(), LatinEvidenceAssistedPassPayloadFields()}, true
	case GreekEntryEvidenceAssistedPassPayload:
		return payloadContract{codec.ProviderName(), codec.GreekEntryEvidenceAssistedPassPromptContract(), GreekEntryEvidenceAssistedPassPayloadFields()}, true
	case LSJSenseSummaryPayload:
		return payloadContract{codec.ProviderName(), codec.LSJSenseSummaryPromptContract(), LSJSenseSummaryPayloadFields()}, true
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

// EvidenceAssistedPassKind is the payload kind a source-evidence language's
// evidence-assisted pass transmits: Latin has its own contract, every other
// language the Greek one.
func EvidenceAssistedPassKind(language string) ProviderPayloadKind {
	if isLatinPass(language) {
		return LatinEvidenceAssistedPassPayload
	}
	return EvidenceAssistedPassPayload
}

// StudyEvidenceAssistedPassKind is the payload kind Study's evidence-assisted
// pass transmits for a source-evidence language, selected by its card policy.
func StudyEvidenceAssistedPassKind(language string) ProviderPayloadKind {
	switch cardpolicy.For(language).StudyPassContract {
	case cardpolicy.PassContractLatin:
		return LatinEvidenceAssistedPassPayload
	case cardpolicy.PassContractGreekEntries:
		return GreekEntryEvidenceAssistedPassPayload
	case cardpolicy.PassContractGreek:
	}
	return EvidenceAssistedPassPayload
}
