package domain

import "testing"

func TestReadingJourneyValidate(t *testing.T) {
	valid := ReadingJourney{OwnerID: "owner", Entries: []ReadingJourneyEntry{{OwnerID: "owner", BookID: "book", Position: 1}}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid journey rejected: %v", err)
	}

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
			if err := tt.entry.Validate(); err == nil {
				t.Fatalf("invalid entry accepted: %+v", tt.entry)
			}
		})
	}
}
