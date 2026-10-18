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
// headword, its sense IDs and its English glosses alone. Justin must review that
// payload and record the disposition on #2172 before any call is transmitted.
// Recorded stays false until then, so no summary is requested and every Greek
// card keeps its full LSJ entry (ADR 0101).
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
	Language:      "grc",
	Provider:      "openai-compatible",
	PromptVersion: "translation-v17-lsj-sense-summary-greek",
	Fields: []string{
		"item_id", "headword", "senses",
	},
	Disposition: "awaiting Justin's review of the LSJ sense summary payload (#2172)",
	Ticket:      "#2172",
	Recorded:    false,
}

// ProviderAssessments returns the compiled reviewed personal-use payload
// assessments. A caller that can install a recorded assessment for a test
// passes its own set to AllowExternalProviderCall instead.
func ProviderAssessments() []ProviderAssessment {
	return []ProviderAssessment{evidenceAssistedPassAssessment, lsjSenseSummaryAssessment}
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
// assessment is unaffected. A language may carry several assessed payloads (the
// evidence-assisted pass and the LSJ sense summary), so a call is allowed when
// any assessment for its language matches it, and fails closed only when the
// language is assessed but none of its assessments matches.
func AllowExternalProviderCall(assessments []ProviderAssessment, language, provider, promptVersion string, fields []string) error {
	covered := false
	var lastErr error
	for i := range assessments {
		assessment := assessments[i]
		if !sameAssessmentLanguage(language, assessment.Language) {
			continue
		}
		covered = true
		if err := assessment.Gate(language, provider, promptVersion, fields); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if covered {
		return lastErr
	}
	return nil
}

// EvidenceAssistedPassOffered reports whether a matching recorded assessment
// makes the evidence-assisted pass available. While it is false the pass does
// not run and nothing is sent for it (ADR 0101, ADR 0105).
func EvidenceAssistedPassOffered(assessments []ProviderAssessment) bool {
	for _, assessment := range assessments {
		if assessment.Recorded && sameAssessmentLanguage(assessment.Language, evidenceAssistedPassAssessment.Language) && assessment.Provider == evidenceAssistedPassAssessment.Provider && assessment.PromptVersion == evidenceAssistedPassPromptVersion && slices.Equal(assessment.Fields, EvidenceAssistedPassPayloadFields()) {
			return true
		}
	}
	return false
}

// LSJSenseSummaryOffered reports whether a matching recorded assessment makes
// the LSJ sense summary available. While it is false the summary does not run
// and nothing is sent for it (ADR 0101).
func LSJSenseSummaryOffered(assessments []ProviderAssessment) bool {
	for _, assessment := range assessments {
		if assessment.Recorded && sameAssessmentLanguage(assessment.Language, lsjSenseSummaryAssessment.Language) && assessment.Provider == lsjSenseSummaryAssessment.Provider && assessment.PromptVersion == lsjSenseSummaryPromptVersion && slices.Equal(assessment.Fields, LSJSenseSummaryPayloadFields()) {
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
