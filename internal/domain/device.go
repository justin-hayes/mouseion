package domain

import "time"

// Device is a named credential a learner issues for a device protocol. The
// credential itself is shown once when the Device is added and never kept.
type Device struct {
	ID         string
	Name       string
	Username   string
	CreatedAt  time.Time
	LastSeenAt *time.Time
	RevokedAt  *time.Time
}
