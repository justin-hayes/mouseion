package persistence

import (
	"testing"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalysisJobFromRowMapsTheSharedLifecycleAndStage(t *testing.T) {
	job := analysisJobFromRow(sqlcgen.ListAnalysisJobsRow{
		RiverJobID:    7,
		DisplayNumber: 3,
		JOwnerID:      "owner",
		AnalysisRunID: "run",
		AnalysisState: "failed",
		AnalysisStage: "finalize",
		FailureCode:   "analysis_failed",
		Progress:      42,
	})
	assert.Equal(t, domain.JobLifecycleFailed, job.Lifecycle)
	assert.Equal(t, domain.BookPreparationStageFinalize, job.Stage)
	assert.Equal(t, domain.BookPreparationCodeAnalysisFailed, job.FailureCode)
	assert.Equal(t, int64(7), job.ID)
	assert.Equal(t, 42, job.Progress)
}

// These names remain exported for callers outside the web app, but they are
// aliases of the domain sentinels so errors.Is classifies them identically.
func TestDomainSentinelAliases(t *testing.T) {
	cases := []struct {
		name        string
		persistence error
		domain      error
	}{
		{"invalid transition", ErrInvalidTransition, domain.ErrInvalidTransition},
		{"browse counts pending", ErrVocabularyBrowseCountsPending, domain.ErrVocabularyBrowseCountsPending},
		{"browse counts unavailable", ErrVocabularyBrowseCountsUnavailable, domain.ErrVocabularyBrowseCountsUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, tc.persistence, tc.domain)
			require.ErrorIs(t, tc.domain, tc.persistence)
		})
	}
}
