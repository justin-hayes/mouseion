package domain

import "time"

// CurrentReadingCompletion is the durable fact that Finish appended to Reading
// history. Finishing has no vocabulary effect (ADR 0136 §4), so it carries no
// counts.
type CurrentReadingCompletion struct {
	OwnerID     string
	Language    string
	BookID      string
	CompletedAt time.Time
}

// CurrentReadingFinishResult is the outcome of Finish.
type CurrentReadingFinishResult struct {
	Completion CurrentReadingCompletion
}
