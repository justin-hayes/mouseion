package domain

import (
	"time"
)

// ReadingCompletion is the durable fact that an owner finished a Book in a
// study language.
type ReadingCompletion struct {
	OwnerID                     string
	Language                    string
	BookID                      string
	CompletedAt                 time.Time
	GoalSnapshotID              string
	SnapshotVocabularyCount     int
	EligibleVocabularyCount     int
	GraduatedVocabularyCount    int
	AlreadyKnownVocabularyCount int
	Source                      ReadingCompletionSource
}
