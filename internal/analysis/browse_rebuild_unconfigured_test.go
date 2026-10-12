package analysis

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// A Service without its pool or River client cannot schedule rebuilds. The
// guard must say so, not panic, so the startup reconciliation fails clearly.
func TestBrowseRebuildsReportUnconfiguredService(t *testing.T) {
	var service Service
	require.EqualError(t, service.EnqueueMissingBrowseCountRebuilds(context.Background()), "analysis: Browse rebuild service is not configured")
}
