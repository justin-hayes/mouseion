package prepareddeck

import (
	"sort"
	"sync"
	"time"
)

const (
	MetricBatchSubmissions          = "batch_submissions_total"
	MetricBatchQueueAge             = "batch_queue_age_seconds"
	MetricBatchProviderTransitions  = "batch_provider_transitions_total"
	MetricBatchRequests             = "batch_requests_total"
	MetricBatchUsageInputTokens     = "batch_usage_input_tokens_total"  //nolint:gosec // metric names are not credentials.
	MetricBatchUsageOutputTokens    = "batch_usage_output_tokens_total" //nolint:gosec // metric names are not credentials.
	MetricBatchValidationFailures   = "batch_validation_failures_total"
	MetricBatchRetries              = "batch_retries_total"
	MetricBatchReconciliationErrors = "batch_reconciliation_errors_total"
	MetricBatchFileCleanup          = "batch_file_cleanup_total"
	MetricBatchPhaseLatency         = "batch_phase_latency_seconds"
	MetricBatchTotalLatency         = "batch_total_latency_seconds"
	MetricBatchStuckBatches         = "batch_stuck_batches"
	MetricTranslationUnits          = "batch_translation_units_total"
	MetricProviderRequestLatency    = "batch_provider_request_latency_seconds"
	MetricProviderErrors            = "batch_provider_errors_total"
	MetricCacheHits                 = "batch_cache_hits_total"
	MetricCacheMisses               = "batch_cache_misses_total"
	MetricAPKGOutcome               = "batch_apkg_outcome_total"
)

// BatchMetric contains only low-cardinality labels explicitly permitted by
// the prepared-deck observability contract. IDs and source-derived data have
// no fields here and therefore cannot become metric labels accidentally.
type BatchMetric struct {
	Mode                                     string
	Name, Phase, State, ErrorClass, Provider string
	Value                                    float64
}

type batchMetricKey struct {
	Mode                                     string
	Name, Phase, State, ErrorClass, Provider string
}

// MetricsCollector is a small aggregate backend used by the worker process
// and tests. Deployments can export Snapshot to their metrics system without
// changing the Batch workers or widening their privacy boundary.
type MetricsCollector struct {
	mu     sync.Mutex
	values map[batchMetricKey]float64
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{values: make(map[batchMetricKey]float64)}
}

func (c *MetricsCollector) ObserveBatch(metric BatchMetric) {
	if c == nil || metric.Name == "" {
		return
	}
	metric = boundedBatchMetric(metric)
	if metric.Name == "" {
		return
	}
	key := batchMetricKey{Mode: metric.Mode, Name: metric.Name, Phase: metric.Phase, State: metric.State, ErrorClass: metric.ErrorClass, Provider: metric.Provider}
	c.mu.Lock()
	if c.values == nil {
		c.values = make(map[batchMetricKey]float64)
	}
	c.values[key] += metric.Value
	c.mu.Unlock()
}

type BatchMetricSample struct {
	Mode                                     string
	Name, Phase, State, ErrorClass, Provider string
	Value                                    float64
}

func (c *MetricsCollector) Snapshot() []BatchMetricSample {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]BatchMetricSample, 0, len(c.values))
	for key, value := range c.values {
		result = append(result, BatchMetricSample{Mode: key.Mode, Name: key.Name, Phase: key.Phase, State: key.State, ErrorClass: key.ErrorClass, Provider: key.Provider, Value: value})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		if result[i].Mode != result[j].Mode {
			return result[i].Mode < result[j].Mode
		}
		if result[i].Phase != result[j].Phase {
			return result[i].Phase < result[j].Phase
		}
		if result[i].State != result[j].State {
			return result[i].State < result[j].State
		}
		if result[i].ErrorClass != result[j].ErrorClass {
			return result[i].ErrorClass < result[j].ErrorClass
		}
		return result[i].Provider < result[j].Provider
	})
	return result
}

func boundedBatchMetric(metric BatchMetric) BatchMetric {
	if metric.Mode != "" && metric.Mode != "standard" && metric.Mode != "batch" {
		metric.Mode = "unknown"
	}
	if !knownBatchMetricName(metric.Name) {
		metric.Name = "batch_metric_unknown"
	}
	if !knownBatchMetricPhase(metric.Phase) {
		metric.Phase = "unknown"
	}
	if !knownBatchMetricState(metric.State) {
		metric.State = "unknown"
	}
	if !knownBatchMetricErrorClass(metric.ErrorClass) {
		metric.ErrorClass = "unknown"
	}
	if metric.Provider != "" && metric.Provider != "openai" {
		metric.Provider = "unknown"
	}
	return metric
}

func knownBatchMetricName(name string) bool {
	switch name {
	case MetricBatchSubmissions, MetricBatchQueueAge, MetricBatchProviderTransitions, MetricBatchRequests, MetricBatchUsageInputTokens, MetricBatchUsageOutputTokens, MetricBatchValidationFailures, MetricBatchRetries, MetricBatchReconciliationErrors, MetricBatchFileCleanup, MetricBatchPhaseLatency, MetricBatchTotalLatency, MetricBatchStuckBatches, MetricTranslationUnits, MetricProviderRequestLatency, MetricProviderErrors, MetricCacheHits, MetricCacheMisses, MetricAPKGOutcome:
		return true
	default:
		return false
	}
}

func knownBatchMetricPhase(phase string) bool {
	switch phase {
	case "", "freezing", "submitting", "waiting", "reconciling", "cleanup", "finalizing", "translating", "provider", "cache":
		return true
	default:
		return false
	}
}

func knownBatchMetricState(state string) bool {
	switch state {
	case "", "pending", "submitted", "validating", "in_progress", "finalizing", "completed", "failed", "expired", "cancelled", "cancelling", "deleted", "stuck":
		return true
	default:
		return false
	}
}

func knownBatchMetricErrorClass(class string) bool {
	switch class {
	case "", "configuration", "upload", "submission", "ambiguous_submission", "provider", "unsupported_model", "expired", "cancelled", "poll", "reconciliation", "malformed_result", "missing_result", "duplicate_result", "unknown_result", "validation", "retry_exhausted", "provider_error", "rate_limit", "timeout", "provider_5xx", "terminal":
		return true
	default:
		return false
	}
}

type BatchMetrics interface {
	ObserveBatch(BatchMetric)
}

func observeBatchMetric(metrics BatchMetrics, metric BatchMetric) {
	if metrics == nil {
		return
	}
	defer func() {
		if recover() != nil {
			return
		}
	}()
	metrics.ObserveBatch(metric)
}

func seconds(duration time.Duration) float64 {
	if duration <= 0 {
		return 0
	}
	return duration.Seconds()
}
