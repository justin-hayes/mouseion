package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
)

// TestLatestRunSignalFromViewMapsFailedStateToRunFailed pins the failed view
// state to the run's own failure whatever the accompanying status says. The
// evidence view reports "failed" only for a failed analysis run, whose status
// is always "analysis failed"; the mapping must not fall back to a job-level
// signal if it ever sees anything else.
func TestLatestRunSignalFromViewMapsFailedStateToRunFailed(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status string
	}{
		{name: "analysis failed status", status: "analysis failed"},
		{name: "in-progress status", status: "analyzing"},
		{name: "unanalyzed status", status: "not analyzed"},
		{name: "empty status", status: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, domain.RunFailed, latestRunSignalFromView(tt.status, "failed"))
		})
	}
}
