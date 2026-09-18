package persistence

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateReadingJourneyMove(t *testing.T) {
	members := []readingJourneyMembership{{bookID: "first"}, {bookID: "second"}}

	tests := []struct {
		name             string
		exists           bool
		revision         int64
		expectedRevision int64
		bookID           string
		wantIndex        int
		wantErr          error
	}{
		{name: "member", exists: true, revision: 4, expectedRevision: 4, bookID: "second", wantIndex: 1},
		{name: "stale", exists: true, revision: 4, expectedRevision: 3, bookID: "second", wantIndex: -1, wantErr: ErrJourneyStale},
		{name: "missing member", exists: true, revision: 4, expectedRevision: 4, bookID: "missing", wantIndex: -1, wantErr: ErrNotFound},
		{name: "missing journey", exists: false, revision: 0, expectedRevision: 0, bookID: "second", wantIndex: -1, wantErr: ErrNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			index, err := validateReadingJourneyMove(test.exists, test.revision, test.expectedRevision, members, test.bookID)
			assert.Equal(t, test.wantIndex, index)
			if test.wantErr == nil {
				require.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, test.wantErr)
			}
		})
	}
}

func TestMoveReadingJourneyMembersPreservesPrimaryGoalAnchor(t *testing.T) {
	members := []readingJourneyMembership{{bookID: "first"}, {bookID: "goal"}, {bookID: "second"}, {bookID: "third"}}

	moved, changed := moveReadingJourneyMembers(members, 2, "second", "goal", 1)

	require.True(t, changed)
	assert.Equal(t, []string{"second", "goal", "first", "third"}, readingJourneyBookIDs(moved))

	unchanged, changed := moveReadingJourneyMembers(moved, 0, "second", "goal", 1)
	assert.False(t, changed)
	assert.Equal(t, []string{"second", "goal", "first", "third"}, readingJourneyBookIDs(unchanged))
}

func TestMoveReadingJourneyMembersClampsAndMovesWithoutGoal(t *testing.T) {
	members := []readingJourneyMembership{{bookID: "first"}, {bookID: "second"}, {bookID: "third"}}

	moved, changed := moveReadingJourneyMembers(members, 0, "first", "", 99)

	require.True(t, changed)
	assert.Equal(t, []string{"second", "third", "first"}, readingJourneyBookIDs(moved))

	moved, changed = moveReadingJourneyMembers(moved, 2, "first", "", 0)
	require.True(t, changed)
	assert.Equal(t, []string{"first", "second", "third"}, readingJourneyBookIDs(moved))
}

func readingJourneyBookIDs(members []readingJourneyMembership) []string {
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.bookID)
	}
	return ids
}
