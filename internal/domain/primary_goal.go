package domain

import (
	"errors"
	"strings"
	"time"
)

type GoalEligibilityReason string

const (
	GoalNeedsCurrentContent GoalEligibilityReason = "needs-current-content"
	GoalAnalysisInProgress  GoalEligibilityReason = "analysis-in-progress"
	GoalFailed              GoalEligibilityReason = "failed"
	GoalCancelled           GoalEligibilityReason = "cancelled"
	GoalStale               GoalEligibilityReason = "stale"
	GoalNoCompletedAnalysis GoalEligibilityReason = "no-completed-analysis"
	GoalEligible            GoalEligibilityReason = "eligible"
)

// PrimaryGoal is the owner's current commitment to finish one book in a
// particular study language.
// Analysis, deck preparation, reading progress, and vocabulary work are
// independent of the Goal and may not exist yet.
type PrimaryGoal struct {
	OwnerID   string
	Language  string
	BookID    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsActive reports whether this row represents the owner's current commitment.
func (g PrimaryGoal) IsActive() bool {
	return g.BookID != ""
}

// GoalEligibility classifies whether the summary can be promoted to a
// Primary Goal. It is a read-only projection; persistence remains the
// enforcement boundary.
func (s SourceMaterialSummary) GoalEligibility() GoalEligibilityReason {
	if s.Source.ID == "" || s.Source.ContentRevisionID == "" || s.Source.ContentSnapshotID == "" || !strings.EqualFold(strings.TrimSpace(s.Source.MediaType), "application/epub+zip") {
		return GoalNeedsCurrentContent
	}
	status := strings.ToLower(strings.TrimSpace(s.AnalysisStatus + " " + s.AnalysisState))
	switch {
	case strings.Contains(status, "queued") || strings.Contains(status, "running") || strings.Contains(status, "analyzing"):
		return GoalAnalysisInProgress
	case strings.Contains(status, "failed"):
		return GoalFailed
	case strings.Contains(status, "cancelled"):
		return GoalCancelled
	case strings.Contains(status, "stale"):
		return GoalStale
	case s.AnalysisStatus != "analyzed" || s.AnalysisState != "completed" || s.AnalysisRunID == "" || s.CorpusID == "":
		return GoalNoCompletedAnalysis
	default:
		return GoalEligible
	}
}

// Validate checks the owner-, language-, and book-scoped identity of a Goal.
func (g PrimaryGoal) Validate() error {
	if strings.TrimSpace(g.OwnerID) == "" || strings.TrimSpace(g.Language) == "" || strings.TrimSpace(g.BookID) == "" {
		return errors.New("domain: primary goal identity is required")
	}
	return nil
}
