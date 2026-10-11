package domain

import "errors"

// Sentinels for the expected-revision protocol that Book visibility and
// disposition writes share. They live in domain so store contracts can assert
// on them; the persistence package re-exports them under its own names.
var (
	ErrStaleBookVisibility  = errors.New("book visibility changed since this page was loaded")
	ErrStaleBookDisposition = errors.New("book disposition changed since this page was loaded")
)
