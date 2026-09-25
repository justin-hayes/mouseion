package domain

// GoalEligibility is retained for the existing Goal handlers and delegates to
// the current-reading projection.
func (s SourceMaterialSummary) GoalEligibility() GoalEligibilityReason {
	return s.CurrentReadingEligibility()
}
