package domain

import (
	"errors"
	"strings"
	"time"
)

type ReadingJourneyEntry struct {
	OwnerID   string
	Language  string
	BookID    string
	Position  int
	CreatedAt time.Time
}

type ReadingJourney struct {
	OwnerID   string
	Language  string
	Revision  int64
	UpdatedAt time.Time
	Entries   []ReadingJourneyEntry // ordered by (position, created_at, book_id)
}

// JourneyForecastCoverage is the exact, presentation-independent coverage
// result for one Journey stage.
type JourneyForecastCoverage struct {
	KnownTokenCount      int64
	AnalyzableTokenCount int64
}

type JourneyForecastEntry struct {
	BookID            string
	SourceMaterialID  string
	Language          string
	CorpusID          string
	Position          int
	Current           *JourneyForecastCoverage
	AfterGoal         *JourneyForecastCoverage
	OnArrival         *JourneyForecastCoverage
	LowerBound        bool
	UnavailableReason string
}

// JourneyForecast is an on-demand read model. It is never persisted.
type JourneyForecast struct {
	OwnerID  string
	Language string
	Goal     *PrimaryGoal
	Entries  []JourneyForecastEntry
}

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
}

// Validate checks each entry for non-empty identity and position >= 1.
func (r ReadingJourney) Validate() error {
	if strings.TrimSpace(r.OwnerID) == "" {
		return errors.New("domain: reading journey owner is required")
	}
	for _, entry := range r.Entries {
		if err := entry.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (e ReadingJourneyEntry) Validate() error {
	if strings.TrimSpace(e.OwnerID) == "" || strings.TrimSpace(e.BookID) == "" {
		return errors.New("domain: reading journey entry identity is required")
	}
	if e.Position < 1 {
		return errors.New("domain: reading journey entry position must be at least 1")
	}
	return nil
}
