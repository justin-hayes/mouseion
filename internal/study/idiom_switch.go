package study

import (
	"fmt"
	"os"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// IdiomLanguagesEnv is the comma-separated Idiom switch. Only a language with a
// committed review report may appear; it is empty by default (ADR 0114).
const IdiomLanguagesEnv = "MOUSEION_IDIOM_CARD_LANGUAGES"

// IdiomLanguagesFromEnv parses the Idiom switch. A malformed or empty entry is
// rejected so an operator typo cannot silently enable the wrong language; an
// unset variable keeps Idioms off.
func IdiomLanguagesFromEnv() (map[string]bool, error) {
	value := strings.TrimSpace(os.Getenv(IdiomLanguagesEnv))
	languages := make(map[string]bool)
	if value == "" {
		return languages, nil
	}
	for part := range strings.SplitSeq(value, ",") {
		language := canonicalization.NormalizeLanguage(strings.TrimSpace(part))
		if language == "" {
			return nil, fmt.Errorf("%s contains an empty language: %q", IdiomLanguagesEnv, value)
		}
		languages[language] = true
	}
	return languages, nil
}

// IdiomSwitch reports whether a language's Idiom switch is on: the operator
// listed it in languages (MOUSEION_IDIOM_CARD_LANGUAGES) and its idiom-card
// payload assessment is recorded for the codec. A nil assessments set means the
// production assessments (ADR 0137); a language that fails the gate sends
// nothing.
func IdiomSwitch(languages map[string]bool, assessments enrichment.ProviderAssessmentSet, codec *enrichment.TranslationCodec) func(language string) bool {
	if assessments == nil {
		assessments = enrichment.ProviderAssessments()
	}
	return func(language string) bool {
		language = canonicalization.NormalizeLanguage(language)
		if codec == nil || !languages[language] {
			return false
		}
		return assessments.MaySend(enrichment.IdiomCardPayload, language, codec) == nil
	}
}
