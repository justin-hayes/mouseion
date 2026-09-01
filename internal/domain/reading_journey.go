package domain

import (
	"errors"
	"strings"
	"time"
)

type ReadingJourneyEntry struct {
	OwnerID   string
	BookID    string
	Position  int
	CreatedAt time.Time
}

type ReadingJourney struct {
	OwnerID   string
	Revision  int64
	UpdatedAt time.Time
	Entries   []ReadingJourneyEntry // ordered by (position, created_at, book_id)
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
