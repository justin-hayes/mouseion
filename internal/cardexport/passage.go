package cardexport

import (
	"fmt"
	"strings"

	"github.com/justin-hayes/mouseion/internal/classical"
)

// The Ancient Greek default card shows the pinned reference-translation
// passage context of the cited sections a representative sentence touches
// (ADR 0101). It is never a translation of that sentence, so it is labelled and
// highlights nothing.
const (
	passageContextPrefix = "Passage context: "
	passageContextSuffix = ". Not a translation of this sentence" //nolint:gosec // G101 false positive: a user-facing card label, not a credential.
)

// embeddedPassageAttribution looks a frozen translation pin up in the release's
// translation register. It is a variable so a card-export test can exercise a
// register that fails to parse.
var embeddedPassageAttribution = classical.EmbeddedPassageAttribution

// passageAttribution returns the credit for a frozen translation pin. A pin the
// register does not list, or a register that cannot be parsed, is an error:
// silently crediting another translation would ship a misattribution (ADR 0111).
func passageAttribution(pin string) (classical.PassageAttribution, error) {
	attribution, ok, err := embeddedPassageAttribution(pin)
	if err != nil {
		return classical.PassageAttribution{}, fmt.Errorf("cardexport: read translation register: %w", err)
	}
	if !ok {
		return classical.PassageAttribution{}, fmt.Errorf("%w: passage pin %q is not listed in the translation register", ErrInvalidInput, pin)
	}
	return attribution, nil
}

// passageContextLabel renders the frozen passage citation as the labelled,
// unhighlighted card heading, crediting the translation the pin names.
func passageContextLabel(pin, citation string) (string, error) {
	attribution, err := passageAttribution(pin)
	if err != nil {
		return "", err
	}
	return passageContextPrefix + attribution.Label + " " + citation + passageContextSuffix, nil
}

// resolvePassage returns the passage text, its citation label and the pin for a
// representative sentence, or empty values when the card must show no English.
// Every cited section must have a translation; a single missing key withholds
// the whole passage, never a partial one.
func resolvePassage(citations []string, translations map[string]string, pin string) (text, citation, sourcePin string) {
	if strings.TrimSpace(pin) == "" || len(citations) == 0 || len(translations) == 0 {
		return "", "", ""
	}
	sections := make([]string, 0, len(citations))
	for _, section := range citations {
		translation := strings.TrimSpace(translations[section])
		if translation == "" {
			return "", "", ""
		}
		sections = append(sections, translation)
	}
	return strings.Join(sections, " "), classical.FormatCitationRange(citations), pin
}
