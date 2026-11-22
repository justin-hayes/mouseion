package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMyBookWorkflowBucketUsesVisibleBucketPrecedence(t *testing.T) {
	tests := []struct {
		name string
		book MyBook
		want MyBookBucket
	}{
		{name: "current reading takes precedence over every other role", book: MyBook{Disposition: BookDispositionInbox, IsCurrentReading: true, CompletionCount: 1}, want: MyBookBucketCurrentReading},
		{name: "current reading takes precedence over to read", book: MyBook{Disposition: BookDispositionToRead, IsCurrentReading: true, CompletionCount: 1}, want: MyBookBucketCurrentReading},
		{name: "to read takes precedence over history", book: MyBook{Disposition: BookDispositionToRead, CompletionCount: 1}, want: MyBookBucketToRead},
		{name: "previously read inbox book belongs in read", book: MyBook{Disposition: BookDispositionInbox, CompletionCount: 1}, want: MyBookBucketRead},
		{name: "inbox without history", book: MyBook{Disposition: BookDispositionInbox}, want: MyBookBucketInbox},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.book.WorkflowBucket())
		})
	}
}

func TestMyBookCanMarkPreviouslyReadOnlyForUnstartedToRead(t *testing.T) {
	chosen := func(book MyBook) MyBook {
		book.Book.LanguageState = LanguageChosen
		return book
	}
	tests := []struct {
		name string
		book MyBook
		want bool
	}{
		{name: "unstarted To Read", book: chosen(MyBook{Disposition: BookDispositionToRead}), want: true},
		{name: "To Read with a completion", book: chosen(MyBook{Disposition: BookDispositionToRead, CompletionCount: 1}), want: false},
		{name: "To Read that held the current reading", book: chosen(MyBook{Disposition: BookDispositionToRead, EverCurrentReading: true}), want: false},
		{name: "Inbox book", book: chosen(MyBook{Disposition: BookDispositionInbox}), want: false},
		{name: "Read book", book: chosen(MyBook{Disposition: BookDispositionInbox, CompletionCount: 1}), want: false},
		{name: "current reading", book: chosen(MyBook{Disposition: BookDispositionToRead, IsCurrentReading: true}), want: false},
		{name: "unchosen language", book: MyBook{Disposition: BookDispositionToRead}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.book.CanMarkPreviouslyRead())
		})
	}
}

func TestMyBookBucketBrowseFiltersAreMutuallyExclusive(t *testing.T) {
	book := MyBook{Disposition: BookDispositionInbox, CompletionCount: 1}
	bucket := book.WorkflowBucket()
	assert.True(t, bucket.MatchesBrowseFilter("", false))
	assert.True(t, bucket.MatchesBrowseFilter("", true))
	assert.False(t, bucket.MatchesBrowseFilter(BookDispositionInbox, false))
	assert.False(t, bucket.MatchesBrowseFilter(BookDispositionToRead, false))
	assert.Equal(t, BookDispositionInbox, book.Disposition, "deriving Read must not erase the persisted disposition")
}

func TestCurrentReadingAppearsInToReadBrowse(t *testing.T) {
	book := MyBook{Disposition: BookDispositionToRead, IsCurrentReading: true}
	bucket := book.WorkflowBucket()
	assert.Equal(t, MyBookBucketCurrentReading, bucket)
	assert.True(t, bucket.MatchesBrowseFilter(BookDispositionToRead, false))
	assert.False(t, bucket.MatchesBrowseFilter(BookDispositionInbox, false))
	assert.False(t, bucket.MatchesBrowseFilter("", true))
}

func TestBookDispositionValidateAcceptsOnlyInboxAndToRead(t *testing.T) {
	assert.NoError(t, BookDispositionInbox.Validate())
	assert.NoError(t, BookDispositionToRead.Validate())
	assert.Error(t, BookDisposition("set_aside").Validate(), "retired Set Aside disposition is unknown")
}
