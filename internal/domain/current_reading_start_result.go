package domain

// StartResult is the outcome of Read next. Replayed reports that the requested
// Book was already the current reading, so nothing was written and Reading is
// that existing reading.
type StartResult struct {
	Reading  CurrentReading
	Replayed bool
}

// StartDecisionError maps a Read next decision to the error every store returns
// for it, so adapters reject it the same way. Accepted and replayed decisions
// map to nil.
func StartDecisionError(decision StartDecision) error {
	switch decision.Outcome {
	case StartAccepted, StartReplayed:
		return nil
	case StartRejectedIneligible:
		return CurrentReadingIneligibleError{Reason: decision.Reason}
	}
	return CurrentReadingIneligibleError{Reason: decision.Reason}
}
