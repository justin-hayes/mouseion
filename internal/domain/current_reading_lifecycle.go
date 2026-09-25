package domain

import "time"

// CurrentReadingCompletion is the current-reading vocabulary for the durable
// completion fact. It intentionally does not expose the retired Goal name.
type CurrentReadingCompletion struct {
	OwnerID                     string
	Language                    string
	BookID                      string
	CompletedAt                 time.Time
	SnapshotID                  string
	SnapshotVocabularyCount     int
	EligibleVocabularyCount     int
	GraduatedVocabularyCount    int
	AlreadyKnownVocabularyCount int
}

// CurrentReadingFinishResult is the durable outcome returned by accepting a
// current reading. Replaying the same finish request returns the same result.
type CurrentReadingFinishResult struct {
	Completion CurrentReadingCompletion
}
