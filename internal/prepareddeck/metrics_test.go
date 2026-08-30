package prepareddeck

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBatchMetricsAggregateOnlyAllowedLabels(t *testing.T) {
	collector := NewMetricsCollector()
	collector.ObserveBatch(BatchMetric{Name: MetricBatchRequests, Phase: "reconciling", State: "completed", ErrorClass: "", Provider: "openai", Value: 3})
	collector.ObserveBatch(BatchMetric{Name: MetricBatchRequests, Phase: "reconciling", State: "completed", ErrorClass: "", Provider: "openai", Value: 2})
	samples, err := json.Marshal(collector.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(samples)
	for _, prohibited := range []string{"owner", "preparation", "run", "batch_id", "lemma", "title", "sentence", "prompt", "response", "error_body", "credential"} {
		if strings.Contains(strings.ToLower(encoded), prohibited) {
			t.Fatalf("metric contains prohibited label %q: %s", prohibited, encoded)
		}
	}
	if !strings.Contains(encoded, `"Value":5`) {
		t.Fatalf("metric did not aggregate values: %s", encoded)
	}
}

func TestBatchMetricsBoundUnknownLabels(t *testing.T) {
	collector := NewMetricsCollector()
	collector.ObserveBatch(BatchMetric{Name: "owner-123", Phase: "run-456", State: "batch-789", ErrorClass: "raw provider error", Provider: "secret-provider", Value: 1})
	encoded, err := json.Marshal(collector.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(encoded))
	for _, prohibited := range []string{"owner-123", "run-456", "batch-789", "raw provider error", "secret-provider"} {
		if strings.Contains(text, prohibited) {
			t.Fatalf("metric retained unbounded label %q: %s", prohibited, encoded)
		}
	}
}

func TestMetricsAggregateByModeAndBoundUnknownMode(t *testing.T) {
	collector := NewMetricsCollector()
	collector.ObserveBatch(BatchMetric{Mode: "standard", Name: MetricBatchRequests, Phase: "provider", State: "completed", Provider: "openai", Value: 1})
	collector.ObserveBatch(BatchMetric{Mode: "batch", Name: MetricBatchRequests, Phase: "reconciling", State: "completed", Provider: "openai", Value: 2})
	collector.ObserveBatch(BatchMetric{Mode: "run-id", Name: MetricBatchRequests, Value: 3})
	samples := collector.Snapshot()
	if len(samples) != 3 {
		t.Fatalf("samples=%+v", samples)
	}
	seen := map[string]bool{}
	for _, sample := range samples {
		seen[sample.Mode] = true
	}
	for _, mode := range []string{"standard", "batch", "unknown"} {
		if !seen[mode] {
			t.Fatalf("mode %q missing from samples: %+v", mode, samples)
		}
	}
}

func TestBoundedProviderFileErrorDoesNotExposeObjectID(t *testing.T) {
	if got := boundedProviderCode("file_batch_123/provider-secret"); got != "provider_error" {
		t.Fatalf("bounded provider code=%q", got)
	}
}
