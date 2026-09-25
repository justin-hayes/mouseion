package domain

// CurrentReadingFinishResult is the durable outcome returned by accepting a
// current reading. Replaying the same finish request returns the same result.
type CurrentReadingFinishResult struct {
	Completion ReadingCompletion
}
