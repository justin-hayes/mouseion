package domain

import (
	"testing"
	"time"
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
			if got := tt.source.EvidenceState(); got != tt.want {
				t.Fatalf("EvidenceState() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMyBookEvidenceDerivationDelegatesToAcquiredSummary(t *testing.T) {
	book := MyBook{Acquired: &SourceMaterialSummary{
		Source:         SourceMaterial{ID: "source", ContentRevisionID: "revision", ContentSnapshotID: "snapshot"},
		AnalysisStatus: "analyzed",
	}}

	if got := book.EvidenceState(); got != BookAnalyzed {
		t.Fatalf("EvidenceState() = %q, want %q", got, BookAnalyzed)
	}
	if got := (MyBook{}).EvidenceState(); got != BookNotAcquired {
		t.Fatalf("missing acquired EvidenceState() = %q, want %q", got, BookNotAcquired)
	}
}

func TestLanguageCorpusBookEvidenceState(t *testing.T) {
	tests := []struct {
		name string
		book LanguageCorpusBookEvidence
		want BookEvidenceState
	}{
		{name: "not acquired", want: BookNotAcquired},
		{
			name: "unavailable current content",
			book: LanguageCorpusBookEvidence{SourceMaterialID: "source"},
			want: BookUnavailable,
		},
		{
			name: "stale source",
			book: LanguageCorpusBookEvidence{
				SourceMaterialID:         "source",
				CurrentContentRevisionID: "revision",
				CurrentSnapshotID:        "snapshot",
				AnalysisSourceMaterialID: "older-source",
				AnalysisRunID:            "run",
			},
			want: BookStale,
		},
		{
			name: "stale current analysis",
			book: LanguageCorpusBookEvidence{
				SourceMaterialID:         "source",
				CurrentContentRevisionID: "revision",
				CurrentSnapshotID:        "snapshot",
				CurrentSourceMaterialID:  "older-source",
				CurrentAnalysisRunID:     "run",
				CorpusID:                 "corpus",
			},
			want: BookStale,
		},
		{
			name: "stale missing current analysis",
			book: LanguageCorpusBookEvidence{
				SourceMaterialID:         "source",
				CurrentContentRevisionID: "revision",
				CurrentSnapshotID:        "snapshot",
				AnalysisRunID:            "run",
				CurrentAnalysisRunID:     "",
			},
			want: BookStale,
		},
		{
			name: "analyzed",
			book: LanguageCorpusBookEvidence{
				SourceMaterialID:         "source",
				CurrentContentRevisionID: "revision",
				CurrentSnapshotID:        "snapshot",
				CurrentSourceMaterialID:  "source",
				CurrentAnalysisRunID:     "run",
				CorpusID:                 "corpus",
			},
			want: BookAnalyzed,
		},
		{
			name: "acquired but unassessed",
			book: LanguageCorpusBookEvidence{SourceMaterialID: "source", CurrentContentRevisionID: "revision", CurrentSnapshotID: "snapshot"},
			want: BookAcquiredUnassessed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.book.EvidenceState(); got != tt.want {
				t.Fatalf("EvidenceState() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrimaryGoalIsActive(t *testing.T) {
	finished := nowForEvidenceTest()
	for name, goal := range map[string]PrimaryGoal{
		"active":   {BookID: "book"},
		"empty":    {},
		"finished": {BookID: "book", ReadingFinishedAt: &finished},
	} {
		want := name == "active"
		if got := goal.IsActive(); got != want {
			t.Errorf("%s IsActive() = %t, want %t", name, got, want)
		}
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
			if got := tt.source.GoalEligibility(); got != tt.want {
				t.Fatalf("GoalEligibility() = %q, want %q", got, tt.want)
			}
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

func nowForEvidenceTest() (now time.Time) {
	return time.Unix(1, 0)
}
