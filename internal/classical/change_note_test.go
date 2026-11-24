package classical

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChangeNoteIsAbsentForAVersionWithoutOne(t *testing.T) {
	note, ok := ChangeNote("no-such-version")
	assert.False(t, ok)
	assert.Empty(t, note)
}

func TestChangeNoteReturnsTheNoteOfAVersionThatHasOne(t *testing.T) {
	changeNotes["synthetic-version"] = "Adds a synthetic feature."
	t.Cleanup(func() { delete(changeNotes, "synthetic-version") })
	note, ok := ChangeNote("synthetic-version")
	assert.True(t, ok)
	assert.Equal(t, "Adds a synthetic feature.", note)
}

func TestChangeNoteOfTheNativeSyntaxVersionNamesWordRelations(t *testing.T) {
	note, ok := ChangeNote("oga-agdt-syntax/1")
	assert.True(t, ok)
	assert.Equal(t, "Adds word relations in Sentence study.", note)
}
