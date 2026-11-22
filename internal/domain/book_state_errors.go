package domain

import "errors"

// Sentinels for the expected-revision protocol that Book disposition writes
// use. They live in domain so store contracts can assert on them; the
// persistence package re-exports them under its own names.
var (
	ErrStaleBookDisposition = errors.New("book disposition changed since this page was loaded")
	// ErrBookIsCurrentReading refuses ending membership for the Book a reading
	// is on. Remove is the opposite of Add, and the current Book cannot leave
	// My Books while it is being read. The same refusal covers recording the
	// Book as Previously read.
	ErrBookIsCurrentReading = errors.New("book is the current reading")
	// ErrPreviouslyReadIneligible refuses the My Books assertion for a Book that
	// is not an unstarted To Read Book.
	ErrPreviouslyReadIneligible = errors.New("only an unstarted To Read Book can be marked previously read")
	// ErrBookNotChosen refuses a state that would leave an active My Books
	// member neither To Read nor Read. My Books holds only chosen Books.
	ErrBookNotChosen = errors.New("book is neither To Read nor read")
	// ErrMyBooksBackfillBlocked refuses the Inbox retirement backfill until
	// every connection's Catalog index has completed a sync.
	ErrMyBooksBackfillBlocked = errors.New("a catalog connection has never completed a sync")
)
