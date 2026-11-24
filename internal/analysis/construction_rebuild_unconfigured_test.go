package analysis

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConstructionRebuildReportsUnconfiguredService(t *testing.T) {
	var service Service
	require.EqualError(t, service.EnqueueMissingConstructionRebuilds(context.Background()), "analysis: Construction rebuild service is not configured")
}
