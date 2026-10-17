package enrichment

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrProviderAssessmentNotRecorded marks a provider call for a language whose
// exact payload has not been reviewed and recorded for personal use. Callers
// fail closed: no request is transmitted.
var ErrProviderAssessmentNotRecorded = errors.New("enrichment: provider payload assessment is not recorded")

// ProviderAssessment is the reviewed personal-use disposition for sending one
// language's contextual-gloss and translation payload to the configured
// external provider. It is compiled reviewed data, not runtime configuration:
// changing the provider, the prompt contract, or the transmitted field set
// invalidates the recorded assessment and must fail closed until it is
// re-recorded.
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

// AncientGreekProviderAssessment is the recorded personal-use assessment of the
// exact Ancient Greek contextual-gloss and translation payload, recorded by
// Justin on #2110. The transport is the configured OpenAI-compatible Chat
// Completions endpoint; the transmitted fields are exactly the frozen
// TranslationRequest (language, target language, canonical lemma, part of
// speech, tested form, one representative sentence, and the bounded dictionary
// candidate senses). No learner identity, Book title, raw annotation, or
// lexical discriminator is transmitted.
var AncientGreekProviderAssessment = ProviderAssessment{
	Language:      "grc",
	Provider:      "openai-compatible",
	PromptVersion: contextualGlossPromptVersion,
	Fields: []string{
		"item_id", "language", "target_language", "canonical_lemma", "upos",
		"target_word", "example_sentence", "candidate_senses",
	},
	Disposition: "approved for personal use of the locally produced deck",
	Ticket:      "#2110",
	Recorded:    true,
}

// Gate reports whether a language's provider call may proceed. A language the
// installation records no assessment for is unaffected. A covered language
// fails closed when the assessment is unrecorded or the configured provider,
// prompt contract, or transmitted field set does not match the recorded one.
func (a ProviderAssessment) Gate(language, provider, promptVersion string, fields []string) error {
	if !sameLanguage(language, a.Language) {
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

// AllowExternalProviderCall applies the recorded assessments to one provider
// call. It is the single policy gate every Ancient Greek provider call crosses.
func AllowExternalProviderCall(language, provider, promptVersion string, fields []string) error {
	return AncientGreekProviderAssessment.Gate(language, provider, promptVersion, fields)
}

// sameLanguage compares two language tags by their lowercase base language.
func sameLanguage(left, right string) bool {
	normalize := func(tag string) string {
		tag = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"))
		base, _, _ := strings.Cut(tag, "-")
		return base
	}
	return normalize(left) == normalize(right)
}
