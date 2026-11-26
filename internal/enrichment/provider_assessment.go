package enrichment

import (
	"errors"
	"fmt"
	"slices"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
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
// library sentence and the learned meaning. It is NOT recorded: the payload is
// new in #2702 and sends the learned meaning, which no recorded payload does, so
// Justin must review doc/reviews/fluency-agreement-payload-assessment.md and
// set Recorded before Fluency contexts are prepared. Until then the gate fails
// closed and nothing is sent (ADR 0133).
var fluencyAgreementAssessment = ProviderAssessment{
	Kind:          FluencyAgreementPayload,
	Language:      "de",
	Provider:      "openai-compatible",
	PromptVersion: "fluency-agreement-v2",
	Fields: []string{
		"item_id", "language", "target_language", "canonical_lemma", "upos",
		"target_word", "example_sentence", "learned_meaning",
	},
	Disposition: "pending review: doc/reviews/fluency-agreement-payload-assessment.md",
	Ticket:      "#2702",
	Recorded:    false,
}

// ProviderAssessmentSet is the recorded personal-use payload assessments. Its
// one question decides whether an assessed payload may be sent.
type ProviderAssessmentSet []ProviderAssessment

// ProviderAssessments returns the compiled reviewed personal-use payload
// assessments. A caller that can install a recorded assessment for a test
// passes its own set to MaySend instead.
func ProviderAssessments() ProviderAssessmentSet {
	return ProviderAssessmentSet{evidenceAssistedPassAssessment, lsjSenseSummaryAssessment, idiomCardAssessment, fluencyAgreementAssessment}
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
	case LSJSenseSummaryPayload:
		return payloadContract{codec.ProviderName(), codec.LSJSenseSummaryPromptContract(), LSJSenseSummaryPayloadFields()}, true
	case IdiomCardPayload:
		return payloadContract{codec.ProviderName(), codec.IdiomCardPromptContract(), IdiomCardPayloadFields()}, true
	case FluencyAgreementPayload:
		return payloadContract{codec.ProviderName(), codec.FluencyAgreementPromptContract(), FluencyAgreementPayloadFields()}, true
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
