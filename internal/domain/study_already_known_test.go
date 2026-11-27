package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStudyAlreadyKnownOnlyOnAnIntroduction(t *testing.T) {
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	introduction := StudyProgress{ID: "a", StudyItem: WordItem("de", "haus", "NOUN", ""), Memory: NewStudyMemory(now)}
	assert.True(t, StudyAlreadyKnownOffered(introduction))

	again := introduction
	again.Memory.State, again.Memory.Reps = StudyStateLearning, 1
	assert.False(t, StudyAlreadyKnownOffered(again), "never on a later review")

	reviewed := introduction
	reviewed.Memory.State, reviewed.Memory.Reps = StudyStateReview, 2
	assert.False(t, StudyAlreadyKnownOffered(reviewed))

	withdrawn := introduction
	withdrawn.Withdrawn = true
	assert.False(t, StudyAlreadyKnownOffered(withdrawn))
}

func TestStudyAlreadyKnownMemoryIsMatureAtOnce(t *testing.T) {
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	memory := StudyAlreadyKnownMemory(NewStudyMemory(now), now)

	assert.Equal(t, StudyStateReview, memory.State)
	assert.GreaterOrEqual(t, memory.ScheduledDays, int64(FluencyMaturityDays))
	assert.Equal(t, now.AddDate(0, 0, int(memory.ScheduledDays)), memory.Due)
	assert.Equal(t, now, memory.LastReview)
	assert.Positive(t, memory.Reps, "it is an answered card, not an introduction still waiting")
	assert.True(t, IsStudyMature(StudyProgress{Memory: memory}))
	assert.False(t, StudyCardIsDue(StudyProgress{Memory: memory}, now.Add(time.Hour)), "it comes back later")
	assert.True(t, StudyCardIsDue(StudyProgress{Memory: memory}, memory.Due))

	require.Zero(t, memory.Lapses)
}

func TestStudyAlreadyKnownMemoryKeepsALongerInterval(t *testing.T) {
	now := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	before := NewStudyMemory(now)
	before.ScheduledDays = 40
	assert.EqualValues(t, 40, StudyAlreadyKnownMemory(before, now).ScheduledDays)
}
