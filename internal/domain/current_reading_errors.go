package domain

import "errors"

// Sentinels every Current reading store returns for the same refusal. They live
// in domain so store contracts can assert on them without importing an adapter;
// the persistence package re-exports them under its own names.
var (
	ErrNotFound                   = errors.New("persistence: not found")
	ErrCurrentReadingExists       = errors.New("persistence: current reading already exists")
	ErrCurrentReadingStale        = errors.New("persistence: current reading state is stale")
	ErrCurrentReadingIneligible   = errors.New("persistence: current reading requires an analyzed To Read book")
	ErrUnresolvedLemmaReviewFlags = errors.New("persistence: unresolved high-risk lemma review flags")
)

// CurrentReadingIneligibleError rejects a Book that the classifier does not allow
// to become the current reading. It matches ErrCurrentReadingIneligible and
// carries the classifier's reason for callers that report it.
type CurrentReadingIneligibleError struct {
	Reason CurrentReadingEligibilityReason
}

func (e CurrentReadingIneligibleError) Error() string {
	return ErrCurrentReadingIneligible.Error() + " (" + string(e.Reason) + ")"
}

func (e CurrentReadingIneligibleError) Is(target error) bool {
	return target == ErrCurrentReadingIneligible
}
