package domain

import (
	"errors"
	"strings"
	"time"
)

// PrimaryGoal is the owner's current commitment to finish one book.
// Analysis, deck preparation, reading progress, and vocabulary work are
// independent of the Goal and may not exist yet.
type PrimaryGoal struct {
	OwnerID   string
	BookID    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate checks the owner- and book-scoped identity of a Goal.
func (g PrimaryGoal) Validate() error {
	if strings.TrimSpace(g.OwnerID) == "" || strings.TrimSpace(g.BookID) == "" {
		return errors.New("domain: primary goal identity is required")
	}
	return nil
}
