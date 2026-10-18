package enrichment

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrProviderAssessmentNotRecorded marks a provider call whose exact payload
// has not been reviewed and recorded for personal use. Callers fail closed: no
// request is transmitted.
var ErrProviderAssessmentNotRecorded = errors.New("enrichment: provider payload assessment is not recorded")

// ProviderAssessment is the reviewed personal-use disposition for sending one
// language's exact external-provider payload to the configured provider. It is
// compiled reviewed data, not runtime configuration: changing the provider, the
// prompt contract, or the transmitted field set invalidates the recorded
// assessment and must fail closed until it is re-recorded.
type ProviderAssessment struct {
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
// sentence, its passage context, and the pinned LSJ senses with their native
// IDs. Justin must review that payload and record the disposition on #2171
// before any call is transmitted. Recorded stays false until then, so the pass
// is not offered and Greek decks contain default cards only (ADR 0101).
//
// The transport is the configured OpenAI-compatible Chat Completions endpoint.
// The transmitted fields are exactly the frozen pass payload: identity fields,
// the representative sentence, its passage context, and the LSJ senses with
// their native IDs. No learner identity, Book title, or raw annotation is
// transmitted.
//
// Fields is written out independently of EvidenceAssistedPassPayloadFields so
// the record is a real pin: changing the codec's payload without re-recording
// the disposition fails the gate and sends nothing.
var evidenceAssistedPassAssessment = ProviderAssessment{
	Language:      "grc",
	Provider:      "openai-compatible",
	PromptVersion: evidenceAssistedPassPromptVersion,
	Fields: []string{
		"item_id", "language", "target_language", "canonical_lemma", "upos",
		"target_word", "example_sentence", "passage_context", "candidate_senses",
	},
	Disposition: "awaiting Justin's review of the evidence-assisted pass payload (#2171)",
	Ticket:      "#2171",
	Recorded:    false,
}

// ProviderAssessments returns the compiled reviewed personal-use payload
// assessments. A caller that can install a recorded assessment for a test
// passes its own set to AllowExternalProviderCall instead.
func ProviderAssessments() []ProviderAssessment {
	return []ProviderAssessment{evidenceAssistedPassAssessment}
}

// Gate reports whether a language's provider call may proceed. A language the
// installation records no assessment for is unaffected. A covered language
// fails closed when the assessment is unrecorded or the configured provider,
// prompt contract, or transmitted field set does not match the recorded one.
func (a ProviderAssessment) Gate(language, provider, promptVersion string, fields []string) error {
	if !sameAssessmentLanguage(language, a.Language) {
		return nil
	}
	if !a.Recorded {
		return fmt.Errorf("%w: %s (%s)", ErrProviderAssessmentNotRecorded, a.Ticket, a.Disposition)
	}
	if provider != a.Provider || promptVersion != a.PromptVersion {
		return fmt.Errorf("%w: configured provider does not match the recorded assessment for %s (%s)", ErrProviderAssessmentNotRecorded, a.Ticket, a.Language)
	}
	if !slices.Equal(fields, a.Fields) {
		return fmt.Errorf("%w: transmitted field set does not match the recorded assessment for %s (%s)", ErrProviderAssessmentNotRecorded, a.Ticket, a.Language)
	}
	return nil
}

// AllowExternalProviderCall applies the supplied recorded assessments to one
// provider call. The prepared-deck pass consults it when it freezes a run: a
// frozen run is immutable, so revoking a recorded assessment later never
// rewrites a deck that was already planned. A language with no recorded
// assessment is unaffected.
func AllowExternalProviderCall(assessments []ProviderAssessment, language, provider, promptVersion string, fields []string) error {
	for _, assessment := range assessments {
		if err := assessment.Gate(language, provider, promptVersion, fields); err != nil {
			return err
		}
	}
	return nil
}

// EvidenceAssistedPassOffered reports whether a matching recorded assessment
// makes the evidence-assisted pass available. While it is false the pass is not
// offered and nothing is sent for it (ADR 0101).
func EvidenceAssistedPassOffered(assessments []ProviderAssessment) bool {
	for _, assessment := range assessments {
		if assessment.Recorded && sameAssessmentLanguage(assessment.Language, evidenceAssistedPassAssessment.Language) && assessment.Provider == evidenceAssistedPassAssessment.Provider && assessment.PromptVersion == evidenceAssistedPassPromptVersion && slices.Equal(assessment.Fields, EvidenceAssistedPassPayloadFields()) {
			return true
		}
	}
	return false
}

// sameAssessmentLanguage compares two language tags by their lowercase base
// language.
func sameAssessmentLanguage(left, right string) bool {
	normalize := func(tag string) string {
		tag = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"))
		base, _, _ := strings.Cut(tag, "-")
		return base
	}
	return normalize(left) == normalize(right)
}
