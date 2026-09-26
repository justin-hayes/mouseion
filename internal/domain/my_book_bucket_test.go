package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMyBookWorkflowBucketUsesCurrentDispositionHistoryPrecedence(t *testing.T) {
	tests := []struct {
		name string
		book MyBook
		want MyBookBucket
	}{
		{name: "current reading takes precedence over to read and history", book: MyBook{Disposition: BookDispositionToRead, IsCurrentReading: true, CompletionCount: 1}, want: MyBookBucketCurrentReading},
		{name: "to read takes precedence over history", book: MyBook{Disposition: BookDispositionToRead, CompletionCount: 1}, want: MyBookBucketToRead},
		{name: "inbox takes precedence over history", book: MyBook{Disposition: BookDispositionInbox, CompletionCount: 1}, want: MyBookBucketInbox},
		{name: "history takes precedence over set aside", book: MyBook{Disposition: BookDispositionSetAside, CompletionCount: 1}, want: MyBookBucketRead},
		{name: "set aside without history", book: MyBook{Disposition: BookDispositionSetAside}, want: MyBookBucketSetAside},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.book.WorkflowBucket())
		})
	}
}
