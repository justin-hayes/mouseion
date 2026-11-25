package cardexport

import (
	"fmt"
	"sort"
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
	// Every segment whose Citation equals or contains a touched one is shown
	// (ADR 0101): a 1.2 translation covers a sentence touching 1.2.3.
	shown := map[string]struct{}{}
	for _, section := range citations {
		if !collectSegments(section, translations, shown) {
			return "", "", ""
		}
	}
	keys := make([]string, 0, len(shown))
	for key := range shown {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		order, err := classical.CompareCitations(keys[i], keys[j])
		if err != nil {
			return keys[i] < keys[j]
		}
		return order < 0
	})
	sections := make([]string, 0, len(keys))
	for _, key := range keys {
		sections = append(sections, strings.TrimSpace(translations[key]))
	}
	return strings.Join(sections, " "), classical.FormatCitationRange(keys), pin
}

// collectSegments adds every segment key that equals or contains section and
// reports whether there is at least one.
func collectSegments(section string, translations map[string]string, shown map[string]struct{}) bool {
	found := false
	for key, translation := range translations {
		if strings.TrimSpace(translation) == "" || !classical.CitationContains(key, section) {
			continue
		}
		shown[key] = struct{}{}
		found = true
	}
	return found
}
