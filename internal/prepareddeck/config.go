package prepareddeck

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	// TranslationConcurrencyEnv configures the maximum number of enrichment
	// operations that one prepared deck may have in flight.
	TranslationConcurrencyEnv     = "MOUSEION_PREPARED_DECK_TRANSLATION_CONCURRENCY"
	DefaultTranslationConcurrency = 1
)

// TranslationConcurrencyFromEnv returns the per-deck translation limit. The
// compatibility default is serial execution.
func TranslationConcurrencyFromEnv() (int, error) {
	value, ok := os.LookupEnv(TranslationConcurrencyEnv)
	if !ok {
		return DefaultTranslationConcurrency, nil
	}
	concurrency, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || concurrency < 1 {
		return 0, fmt.Errorf("%s must be a positive integer: %q", TranslationConcurrencyEnv, value)
	}
	return concurrency, nil
}
