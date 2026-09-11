package analyzer_test

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/stretchr/testify/require"
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
				require.Equal(t, wantRequest, request)
				return wantResult, nil
			},
		}
		t.Cleanup(func() {
			require.Equal(t, 1, len(fake.Requests()), "Analyze() call count = %d, want 1", len(fake.Requests()))
		})
		return fake
	})
}
