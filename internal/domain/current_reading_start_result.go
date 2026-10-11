package domain

// StartResult is the outcome of a Start. Replayed reports that the requested
// Book was already the current reading, so the Start wrote nothing and Reading
// is that existing reading.
type StartResult struct {
	Reading  CurrentReading
	Replayed bool
}

// StartDecisionError maps a Start decision to the error every store returns for
// it, so adapters reject a Start the same way. Accepted and replayed decisions
// map to nil.
func StartDecisionError(decision StartDecision) error {
	switch decision.Outcome {
	case StartAccepted, StartReplayed:
		return nil
	case StartRejectedAlreadyCurrent:
		return ErrCurrentReadingExists
	case StartRejectedUnresolvedFlags:
		return ErrUnresolvedLemmaReviewFlags
	case StartRejectedIneligible:
		return CurrentReadingIneligibleError{Reason: decision.Reason}
	}
	return CurrentReadingIneligibleError{Reason: decision.Reason}
}
