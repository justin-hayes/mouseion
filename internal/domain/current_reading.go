package domain

import (
	"errors"
	"strings"
	"time"
)

// CurrentReadingEligibilityReason explains why a Book can or cannot become
// the learner's current reading. It is a read-only projection; persistence
// remains the enforcement boundary.
type CurrentReadingEligibilityReason string

const (
	CurrentReadingNeedsCurrentContent CurrentReadingEligibilityReason = "needs-current-content"
	CurrentReadingAnalysisInProgress  CurrentReadingEligibilityReason = "analysis-in-progress"
	CurrentReadingFailed              CurrentReadingEligibilityReason = "failed"
	CurrentReadingCancelled           CurrentReadingEligibilityReason = "cancelled"
	CurrentReadingStale               CurrentReadingEligibilityReason = "stale"
	CurrentReadingNoCompletedAnalysis CurrentReadingEligibilityReason = "no-completed-analysis"
	CurrentReadingEligible            CurrentReadingEligibilityReason = "eligible"
)

// PrimaryGoal is retained as a source-compatibility alias while the existing
// Goal handlers are migrated to the current-reading vocabulary.
type PrimaryGoal = CurrentReading

// GoalEligibilityReason is the compatibility name for the current-reading
// eligibility projection.
type GoalEligibilityReason = CurrentReadingEligibilityReason

const (
	GoalNeedsCurrentContent = CurrentReadingNeedsCurrentContent
	GoalAnalysisInProgress  = CurrentReadingAnalysisInProgress
	GoalFailed              = CurrentReadingFailed
	GoalCancelled           = CurrentReadingCancelled
	GoalStale               = CurrentReadingStale
	GoalNoCompletedAnalysis = CurrentReadingNoCompletedAnalysis
	GoalEligible            = CurrentReadingEligible
)

// CurrentReading is the owner's commitment to finish one Book in a study
// language. Analysis, deck preparation, reading progress, and vocabulary work
// are independent of the current reading and may not exist yet.
type CurrentReading struct {
	OwnerID, Language, BookID string
	SnapshotID                string
	SourceMaterialID          string
	AnalysisRunID             string
	ContentRevisionID         string
	ContentSnapshotID         string
	CorpusID                  string
	SnapshotSize              int
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

// IsActive reports whether this row represents the owner's current reading.
func (r CurrentReading) IsActive() bool {
	return r.BookID != ""
}

// CurrentReadingEligibility classifies whether a Book can be started.
func (s SourceMaterialSummary) CurrentReadingEligibility() CurrentReadingEligibilityReason {
	if s.Source.ID == "" || s.Source.ContentRevisionID == "" || s.Source.ContentSnapshotID == "" || !strings.EqualFold(strings.TrimSpace(s.Source.MediaType), "application/epub+zip") {
		return CurrentReadingNeedsCurrentContent
	}
	status := strings.ToLower(strings.TrimSpace(s.AnalysisStatus + " " + s.AnalysisState))
	switch {
	case strings.Contains(status, "queued") || strings.Contains(status, "running") || strings.Contains(status, "analyzing"):
		return CurrentReadingAnalysisInProgress
	case strings.Contains(status, "failed"):
		return CurrentReadingFailed
	case strings.Contains(status, "cancelled"):
		return CurrentReadingCancelled
	case strings.Contains(status, "stale"):
		return CurrentReadingStale
	case s.AnalysisStatus != "analyzed" || s.AnalysisState != "completed" || s.AnalysisRunID == "" || s.CorpusID == "":
		return CurrentReadingNoCompletedAnalysis
	default:
		return CurrentReadingEligible
	}
}

// Validate checks the owner-, language-, and Book-scoped identity.
func (r CurrentReading) Validate() error {
	if strings.TrimSpace(r.OwnerID) == "" || strings.TrimSpace(r.Language) == "" || strings.TrimSpace(r.BookID) == "" {
		return errors.New("domain: current reading identity is required")
	}
	return nil
}
