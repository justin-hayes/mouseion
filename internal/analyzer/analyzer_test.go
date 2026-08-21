package analyzer_test

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
)

func TestFakeSatisfiesAnalyzerContract(t *testing.T) {
	analyzertest.RunContract(t, func(
		t testing.TB,
		wantRequest analyzer.AnalyzeRequest,
		wantResult analyzer.Result,
	) analyzer.Analyzer {
		t.Helper()
		fake := &analyzertest.Fake{
			AnalyzeFunc: func(_ context.Context, request analyzer.AnalyzeRequest) (analyzer.Result, error) {
				if request != wantRequest {
					t.Fatalf("Analyze() request = %#v, want %#v", request, wantRequest)
				}
				return wantResult, nil
			},
		}
		t.Cleanup(func() {
			if got := len(fake.Requests()); got != 1 {
				t.Errorf("Analyze() call count = %d, want 1", got)
			}
		})
		return fake
	})
}
