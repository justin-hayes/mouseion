package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadingJourneyValidate(t *testing.T) {
	valid := ReadingJourney{OwnerID: "owner", Entries: []ReadingJourneyEntry{{OwnerID: "owner", BookID: "book", Position: 1}}}
	require.NoError(t, valid.Validate(), "valid journey rejected")

	tests := []struct {
		name  string
		entry ReadingJourneyEntry
	}{
		{name: "empty owner", entry: ReadingJourneyEntry{BookID: "book", Position: 1}},
		{name: "empty book", entry: ReadingJourneyEntry{OwnerID: "owner", Position: 1}},
		{name: "zero position", entry: ReadingJourneyEntry{OwnerID: "owner", BookID: "book"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Error(t, tt.entry.Validate(), "invalid entry accepted: %+v", tt.entry)
		})
	}
}
