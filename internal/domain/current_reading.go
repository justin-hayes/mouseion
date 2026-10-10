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
	CurrentReadingNotToRead           CurrentReadingEligibilityReason = "not-to-read"
	CurrentReadingNoChosenLanguage    CurrentReadingEligibilityReason = "no-chosen-language"
	CurrentReadingOtherLanguage       CurrentReadingEligibilityReason = "other-language"
	CurrentReadingNeedsCurrentContent CurrentReadingEligibilityReason = "needs-current-content"
	CurrentReadingAnalysisInProgress  CurrentReadingEligibilityReason = "analysis-in-progress"
	CurrentReadingFailed              CurrentReadingEligibilityReason = "failed"
	CurrentReadingCancelled           CurrentReadingEligibilityReason = "cancelled"
	CurrentReadingStale               CurrentReadingEligibilityReason = "stale"
	CurrentReadingNoCompletedAnalysis CurrentReadingEligibilityReason = "no-completed-analysis"
	CurrentReadingEligible            CurrentReadingEligibilityReason = "eligible"
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

// Validate checks the owner-, language-, and Book-scoped identity.
func (r CurrentReading) Validate() error {
	if strings.TrimSpace(r.OwnerID) == "" || strings.TrimSpace(r.Language) == "" || strings.TrimSpace(r.BookID) == "" {
		return errors.New("domain: current reading identity is required")
	}
	return nil
}
