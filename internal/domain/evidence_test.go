package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSourceMaterialSummaryEvidenceState(t *testing.T) {
	tests := []struct {
		name   string
		source SourceMaterialSummary
		want   BookEvidenceState
	}{
		{name: "not acquired", want: BookNotAcquired},
		{
			name:   "unavailable without current content",
			source: SourceMaterialSummary{Source: SourceMaterial{ID: "source"}},
			want:   BookUnavailable,
		},
		{
			name:   "stale analysis",
			source: SourceMaterialSummary{Source: SourceMaterial{ID: "source", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"}, AnalysisStatus: "stale"},
			want:   BookStale,
		},
		{
			name:   "analyzed",
			source: SourceMaterialSummary{Source: SourceMaterial{ID: "source", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"}, AnalysisStatus: "analyzed"},
			want:   BookAnalyzed,
		},
		{
			name:   "acquired but unassessed",
			source: SourceMaterialSummary{Source: SourceMaterial{ID: "source", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"}, AnalysisStatus: "not analyzed"},
			want:   BookAcquiredUnassessed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.source.EvidenceState())
		})
	}
}

func TestMyBookEvidenceDerivationDelegatesToAcquiredSummary(t *testing.T) {
	book := MyBook{Acquired: &SourceMaterialSummary{
		Source:         SourceMaterial{ID: "source", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"},
		AnalysisStatus: "analyzed",
	}}

	assert.Equal(t, BookAnalyzed, book.EvidenceState())
	assert.Equal(t, BookNotAcquired, (MyBook{}).EvidenceState())
}

func TestPrimaryGoalIsActive(t *testing.T) {
	for name, goal := range map[string]PrimaryGoal{
		"active": {BookID: "book"},
		"empty":  {},
	} {
		want := name == "active"
		assert.Equal(t, want, goal.IsActive(), "%s IsActive() = %t, want %t", name, goal.IsActive(), want)
	}
}

func TestSourceMaterialSummaryGoalEligibility(t *testing.T) {
	base := SourceMaterialSummary{Source: SourceMaterial{ID: "source", MediaType: "application/epub+zip", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"}}
	tests := []struct {
		name   string
		source SourceMaterialSummary
		want   GoalEligibilityReason
	}{
		{name: "needs current content", source: SourceMaterialSummary{Source: SourceMaterial{ID: "source"}}, want: GoalNeedsCurrentContent},
		{name: "analysis in progress", source: withAnalysis(base, "analyzing", "running", "", ""), want: GoalAnalysisInProgress},
		{name: "failed", source: withAnalysis(base, "analysis failed", "failed", "", ""), want: GoalFailed},
		{name: "cancelled", source: withAnalysis(base, "analysis cancelled", "cancelled", "", ""), want: GoalCancelled},
		{name: "stale", source: withAnalysis(base, "stale", "", "run", "corpus"), want: GoalStale},
		{name: "no completed analysis", source: withAnalysis(base, "not analyzed", "", "", ""), want: GoalNoCompletedAnalysis},
		{name: "eligible", source: withAnalysis(base, "analyzed", "completed", "run", "corpus"), want: GoalEligible},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.source.GoalEligibility())
		})
	}
}

func withAnalysis(source SourceMaterialSummary, status, state, run, corpus string) SourceMaterialSummary {
	source.AnalysisStatus = status
	source.AnalysisState = state
	source.AnalysisRunID = run
	source.CorpusID = corpus
	return source
}
