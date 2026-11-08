package prepareddeck

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
)

const (
	TranslationModeEnv            = "MOUSEION_PREPARED_DECK_TRANSLATION_MODE"
	StandardMaxConcurrencyEnv     = "MOUSEION_PREPARED_DECK_STANDARD_MAX_CONCURRENCY"
	StandardMaxAttemptsEnv        = "MOUSEION_PREPARED_DECK_STANDARD_MAX_ATTEMPTS"
	StandardRetryBaseDelayEnv     = "MOUSEION_PREPARED_DECK_STANDARD_RETRY_BASE_DELAY"
	StandardRetryMaxDelayEnv      = "MOUSEION_PREPARED_DECK_STANDARD_RETRY_MAX_DELAY"
	DefaultTranslationMode        = "standard"
	DefaultStandardMaxConcurrency = 4
	MaximumStandardMaxConcurrency = 64
	DefaultStandardMaxAttempts    = 3
	MaximumStandardMaxAttempts    = 20
	DefaultStandardRetryBaseDelay = time.Second
	DefaultStandardRetryMaxDelay  = 30 * time.Second
	MaximumStandardRetryDelay     = 24 * time.Hour
	BatchMaxRequestsEnv           = "MOUSEION_PREPARED_DECK_BATCH_MAX_REQUESTS"
	BatchPollIntervalEnv          = "MOUSEION_PREPARED_DECK_BATCH_POLL_INTERVAL"
	// IdiomCardLanguagesEnv is the comma-separated idiom-card switch. Only a
	// language with a committed review report may appear; it is empty by
	// default (ADR 0114).
	IdiomCardLanguagesEnv    = "MOUSEION_IDIOM_CARD_LANGUAGES"
	DefaultBatchMaxRequests  = 5000
	MaximumBatchMaxRequests  = 50000
	DefaultBatchPollInterval = 30 * time.Second
	MaximumBatchPollInterval = 24 * time.Hour
)

type PreparedDeckConfig struct {
	TranslationMode        string
	StandardMaxConcurrency int
	StandardMaxAttempts    int
	StandardRetryBaseDelay time.Duration
	StandardRetryMaxDelay  time.Duration
	// IdiomCardLanguages is the operator-enabled idiom-card switch, keyed by
	// normalized language. Empty means idiom cards are off everywhere.
	IdiomCardLanguages map[string]bool
}

// PreparedDeckConfigFromEnv reads the translation mode, concurrency, attempt
// budget, and retry delays. Each environment variable is parsed by its own
// named step so a malformed value reports the variable it came from.
func PreparedDeckConfigFromEnv() (PreparedDeckConfig, error) {
	mode, err := translationModeFromEnv()
	if err != nil {
		return PreparedDeckConfig{}, err
	}
	concurrency, err := standardMaxConcurrencyFromEnv()
	if err != nil {
		return PreparedDeckConfig{}, err
	}
	attempts, err := standardMaxAttemptsFromEnv()
	if err != nil {
		return PreparedDeckConfig{}, err
	}
	baseDelay, err := standardRetryDelayFromEnv(StandardRetryBaseDelayEnv, DefaultStandardRetryBaseDelay)
	if err != nil {
		return PreparedDeckConfig{}, err
	}
	maxDelay, err := standardRetryDelayFromEnv(StandardRetryMaxDelayEnv, DefaultStandardRetryMaxDelay)
	if err != nil {
		return PreparedDeckConfig{}, err
	}
	if maxDelay < baseDelay {
		return PreparedDeckConfig{}, fmt.Errorf("%s must not be less than %s", StandardRetryMaxDelayEnv, StandardRetryBaseDelayEnv)
	}
	languages, err := idiomCardLanguagesFromEnv()
	if err != nil {
		return PreparedDeckConfig{}, err
	}
	return PreparedDeckConfig{TranslationMode: mode, StandardMaxConcurrency: concurrency, StandardMaxAttempts: attempts, StandardRetryBaseDelay: baseDelay, StandardRetryMaxDelay: maxDelay, IdiomCardLanguages: languages}, nil
}

// idiomCardLanguagesFromEnv parses the comma-separated idiom-card switch. A
// malformed or empty entry is rejected so an operator typo cannot silently
// enable the wrong language; an unset variable keeps idiom cards off.
func idiomCardLanguagesFromEnv() (map[string]bool, error) {
	value := strings.TrimSpace(os.Getenv(IdiomCardLanguagesEnv))
	languages := make(map[string]bool)
	if value == "" {
		return languages, nil
	}
	for part := range strings.SplitSeq(value, ",") {
		language := canonicalization.NormalizeLanguage(strings.TrimSpace(part))
		if language == "" {
			return nil, fmt.Errorf("%s contains an empty language: %q", IdiomCardLanguagesEnv, value)
		}
		languages[language] = true
	}
	return languages, nil
}

// translationModeFromEnv parses the translation mode, defaulting when unset.
func translationModeFromEnv() (string, error) {
	value, ok := os.LookupEnv(TranslationModeEnv)
	if !ok {
		return DefaultTranslationMode, nil
	}
	mode := strings.ToLower(strings.TrimSpace(value))
	if mode != "standard" && mode != "batch" {
		return "", fmt.Errorf("%s must be standard or batch: %q", TranslationModeEnv, value)
	}
	return mode, nil
}

// standardMaxConcurrencyFromEnv parses the standard-translation concurrency
// ceiling, defaulting when unset.
func standardMaxConcurrencyFromEnv() (int, error) {
	value, ok := os.LookupEnv(StandardMaxConcurrencyEnv)
	if !ok {
		return DefaultStandardMaxConcurrency, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 || parsed > MaximumStandardMaxConcurrency {
		return 0, fmt.Errorf("%s must be an integer between 1 and %d: %q", StandardMaxConcurrencyEnv, MaximumStandardMaxConcurrency, value)
	}
	return parsed, nil
}

// standardMaxAttemptsFromEnv parses the per-item provider attempt budget,
// defaulting when unset.
func standardMaxAttemptsFromEnv() (int, error) {
	value, ok := os.LookupEnv(StandardMaxAttemptsEnv)
	if !ok {
		return DefaultStandardMaxAttempts, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 || parsed > MaximumStandardMaxAttempts {
		return 0, fmt.Errorf("%s must be an integer between 1 and %d: %q", StandardMaxAttemptsEnv, MaximumStandardMaxAttempts, value)
	}
	return parsed, nil
}

// standardRetryDelayFromEnv parses one retry-delay variable, returning fallback
// when it is unset.
func standardRetryDelayFromEnv(env string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(env)
	if !ok {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed <= 0 || parsed > MaximumStandardRetryDelay {
		return 0, fmt.Errorf("%s must be a positive Go duration no greater than %s: %q", env, MaximumStandardRetryDelay, value)
	}
	return parsed, nil
}

type BatchConfig struct {
	MaxRequests  int
	PollInterval time.Duration
}

// BatchConfigFromEnv reads the operational Batch chunk ceiling and polling
// cadence. It does not enable Batch or select between transports.
func BatchConfigFromEnv() (BatchConfig, error) {
	config := BatchConfig{MaxRequests: DefaultBatchMaxRequests, PollInterval: DefaultBatchPollInterval}
	if value, ok := os.LookupEnv(BatchMaxRequestsEnv); ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < 1 || parsed > MaximumBatchMaxRequests {
			return BatchConfig{}, fmt.Errorf("%s must be an integer between 1 and %d: %q", BatchMaxRequestsEnv, MaximumBatchMaxRequests, value)
		}
		config.MaxRequests = parsed
	}
	if value, ok := os.LookupEnv(BatchPollIntervalEnv); ok {
		parsed, err := time.ParseDuration(strings.TrimSpace(value))
		if err != nil || parsed <= 0 || parsed > MaximumBatchPollInterval {
			return BatchConfig{}, fmt.Errorf("%s must be a positive Go duration no greater than %s: %q", BatchPollIntervalEnv, MaximumBatchPollInterval, value)
		}
		config.PollInterval = parsed
	}
	return config, nil
}
