package studycard

import (
	"errors"
	"fmt"

	"github.com/justin-hayes/mouseion/internal/classical"
)

// The Ancient Greek card shows the pinned reference-translation passage context
// of the cited sections a representative sentence touches (ADR 0101). It is
// never a translation of that sentence, so it is labelled and highlights
// nothing.
const (
	passageContextPrefix = "Passage context: "
	passageContextSuffix = ". Not a translation of this sentence" //nolint:gosec // G101 false positive: a user-facing card label, not a credential.
)

// ErrInvalidInput marks a passage pin the translation register does not list.
var ErrInvalidInput = errors.New("studycard: invalid input")

// embeddedPassageAttribution looks a frozen translation pin up in the release's
// translation register. It is a variable so a test can exercise a register that
// fails to parse.
var embeddedPassageAttribution = classical.EmbeddedPassageAttribution

// passageAttribution returns the credit for a frozen translation pin. A pin the
// register does not list, or a register that cannot be parsed, is an error:
// silently crediting another translation would ship a misattribution (ADR 0111).
func passageAttribution(pin string) (classical.PassageAttribution, error) {
	attribution, ok, err := embeddedPassageAttribution(pin)
	if err != nil {
		return classical.PassageAttribution{}, fmt.Errorf("studycard: read translation register: %w", err)
	}
	if !ok {
		return classical.PassageAttribution{}, fmt.Errorf("%w: passage pin %q is not listed in the translation register", ErrInvalidInput, pin)
	}
	return attribution, nil
}

// PassageContextLabel renders the frozen passage citation as the labelled,
// unhighlighted card heading, crediting the translation the pin names: the
// translation, the citation range and the text that it is not a translation of
// the sentence (ADR 0138 §3). A pin the translation register does not list is
// an error, never a guess.
func PassageContextLabel(pin, citation string) (string, error) {
	attribution, err := passageAttribution(pin)
	if err != nil {
		return "", err
	}
	return passageContextPrefix + attribution.Label + " " + citation + passageContextSuffix, nil
}

// PassageNotice is the attribution and licence notice of the translation a
// stored passage pin names; see PassageContextLabel for the error case.
func PassageNotice(pin string) (string, error) {
	attribution, err := passageAttribution(pin)
	if err != nil {
		return "", err
	}
	return attribution.Notice, nil
}
