package prepareddeck

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
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
	DefaultBatchMaxRequests       = 5000
	MaximumBatchMaxRequests       = 50000
	DefaultBatchPollInterval      = 30 * time.Second
	MaximumBatchPollInterval      = 24 * time.Hour
)

type PreparedDeckConfig struct {
	TranslationMode        string
	StandardMaxConcurrency int
	StandardMaxAttempts    int
	StandardRetryBaseDelay time.Duration
	StandardRetryMaxDelay  time.Duration
}

func PreparedDeckConfigFromEnv() (PreparedDeckConfig, error) {
	config := PreparedDeckConfig{TranslationMode: DefaultTranslationMode, StandardMaxConcurrency: DefaultStandardMaxConcurrency, StandardMaxAttempts: DefaultStandardMaxAttempts, StandardRetryBaseDelay: DefaultStandardRetryBaseDelay, StandardRetryMaxDelay: DefaultStandardRetryMaxDelay}
	if value, ok := os.LookupEnv(TranslationModeEnv); ok {
		config.TranslationMode = strings.ToLower(strings.TrimSpace(value))
		if config.TranslationMode != "standard" && config.TranslationMode != "batch" {
			return PreparedDeckConfig{}, fmt.Errorf("%s must be standard or batch: %q", TranslationModeEnv, value)
		}
	}
	if value, ok := os.LookupEnv(StandardMaxConcurrencyEnv); ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < 1 || parsed > MaximumStandardMaxConcurrency {
			return PreparedDeckConfig{}, fmt.Errorf("%s must be an integer between 1 and %d: %q", StandardMaxConcurrencyEnv, MaximumStandardMaxConcurrency, value)
		}
		config.StandardMaxConcurrency = parsed
	}
	if value, ok := os.LookupEnv(StandardMaxAttemptsEnv); ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < 1 || parsed > MaximumStandardMaxAttempts {
			return PreparedDeckConfig{}, fmt.Errorf("%s must be an integer between 1 and %d: %q", StandardMaxAttemptsEnv, MaximumStandardMaxAttempts, value)
		}
		config.StandardMaxAttempts = parsed
	}
	parseDelay := func(env string, current *time.Duration) error {
		value, ok := os.LookupEnv(env)
		if !ok {
			return nil
		}
		parsed, err := time.ParseDuration(strings.TrimSpace(value))
		if err != nil || parsed <= 0 || parsed > MaximumStandardRetryDelay {
			return fmt.Errorf("%s must be a positive Go duration no greater than %s: %q", env, MaximumStandardRetryDelay, value)
		}
		*current = parsed
		return nil
	}
	if err := parseDelay(StandardRetryBaseDelayEnv, &config.StandardRetryBaseDelay); err != nil {
		return PreparedDeckConfig{}, err
	}
	if err := parseDelay(StandardRetryMaxDelayEnv, &config.StandardRetryMaxDelay); err != nil {
		return PreparedDeckConfig{}, err
	}
	if config.StandardRetryMaxDelay < config.StandardRetryBaseDelay {
		return PreparedDeckConfig{}, fmt.Errorf("%s must not be less than %s", StandardRetryMaxDelayEnv, StandardRetryBaseDelayEnv)
	}
	return config, nil
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
