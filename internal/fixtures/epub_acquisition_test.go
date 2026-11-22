package fixtures

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEPUBAcquisitionReportsAQueuedFixtureJob(t *testing.T) {
	handle, err := (EPUBAcquisition{}).SubmitEPUBAcquisition(context.Background(), "owner", "book")
	require.NoError(t, err)
	assert.Equal(t, int64(1), handle.JobID)
}
