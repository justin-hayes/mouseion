package prepareddeck

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	BatchMaxRequestsEnv      = "MOUSEION_PREPARED_DECK_BATCH_MAX_REQUESTS"
	BatchPollIntervalEnv     = "MOUSEION_PREPARED_DECK_BATCH_POLL_INTERVAL"
	DefaultBatchMaxRequests  = 5000
	MaximumBatchMaxRequests  = 50000
	DefaultBatchPollInterval = 30 * time.Second
	MaximumBatchPollInterval = 24 * time.Hour
)

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
