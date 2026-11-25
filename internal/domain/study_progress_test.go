package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStudyDayBeginsAtFiveInBerlin(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)
	cases := []struct {
		name       string
		at         time.Time
		wantStart  time.Time
		wantNext   time.Time
		wantLabel  string
		wantSameAs string
	}{
		{"afternoon", time.Date(2026, 11, 25, 15, 0, 0, 0, berlin), time.Date(2026, 11, 25, 5, 0, 0, 0, berlin), time.Date(2026, 11, 26, 5, 0, 0, 0, berlin), "2026-11-25", ""},
		{"after midnight belongs to the day being finished", time.Date(2026, 11, 26, 1, 30, 0, 0, berlin), time.Date(2026, 11, 25, 5, 0, 0, 0, berlin), time.Date(2026, 11, 26, 5, 0, 0, 0, berlin), "2026-11-25", ""},
		{"the boundary itself starts the new day", time.Date(2026, 11, 26, 5, 0, 0, 0, berlin), time.Date(2026, 11, 26, 5, 0, 0, 0, berlin), time.Date(2026, 11, 27, 5, 0, 0, 0, berlin), "2026-11-26", ""},
		{"one second before the boundary", time.Date(2026, 11, 26, 4, 59, 59, 0, berlin), time.Date(2026, 11, 25, 5, 0, 0, 0, berlin), time.Date(2026, 11, 26, 5, 0, 0, 0, berlin), "2026-11-25", ""},
		{"spring forward keeps 05:00 local", time.Date(2027, 3, 28, 12, 0, 0, 0, berlin), time.Date(2027, 3, 28, 5, 0, 0, 0, berlin), time.Date(2027, 3, 29, 5, 0, 0, 0, berlin), "2027-03-28", ""},
		{"fall back keeps 05:00 local", time.Date(2026, 10, 25, 12, 0, 0, 0, berlin), time.Date(2026, 10, 25, 5, 0, 0, 0, berlin), time.Date(2026, 10, 26, 5, 0, 0, 0, berlin), "2026-10-25", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.True(t, c.wantStart.Equal(StudyDayStart(c.at.UTC())), "start")
			assert.True(t, c.wantNext.Equal(NextStudyDayStart(c.at.UTC())), "next")
			assert.Equal(t, c.wantLabel, StudyDay(c.at.UTC()))
		})
	}
	// The Study day that contains the 02:00 or 03:00 changeover is 23 or 25
	// hours long, and the next day still starts at 05:00 local.
	start := StudyDayStart(time.Date(2027, 3, 27, 12, 0, 0, 0, berlin))
	assert.Equal(t, 23*time.Hour, NextStudyDayStart(start).Sub(start))
	start = StudyDayStart(time.Date(2026, 10, 24, 12, 0, 0, 0, berlin))
	assert.Equal(t, 25*time.Hour, NextStudyDayStart(start).Sub(start))
}

func TestParseStudyGradeAcceptsOnlyTwoGrades(t *testing.T) {
	for _, name := range []string{"again", "got-it"} {
		grade, ok := ParseStudyGrade(name)
		assert.True(t, ok, name)
		assert.Equal(t, StudyGrade(name), grade)
	}
	for _, name := range []string{"", "hard", "easy", "good", "Again"} {
		_, ok := ParseStudyGrade(name)
		assert.False(t, ok, name)
	}
}

func TestNextStudyBatchTakesReadyWordsInStreamOrder(t *testing.T) {
	word := func(lemma string) StudyIdentity {
		return StudyIdentity{Language: "de", CanonicalLemma: lemma, UPOS: "NOUN"}
	}
	var ordered []StudyCandidate
	var material []StudyMaterial
	for _, lemma := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"} {
		ordered = append(ordered, StudyCandidate{StudyIdentity: word(lemma)})
		material = append(material, StudyMaterial{ID: "m-" + lemma, StudyIdentity: word(lemma), State: StudyMaterialReady})
	}
	material[1].State = StudyMaterialPending                // b is still being prepared
	material[2].State = StudyMaterialUnpreparable           // c has no usable sentence
	progress := []StudyProgress{{StudyIdentity: word("a")}} // a is already in study

	batch := NextStudyBatch(ordered, material, progress)

	var lemmas []string
	for _, item := range batch {
		lemmas = append(lemmas, item.CanonicalLemma)
	}
	assert.Equal(t, []string{"d", "e", "f", "g", "h", "i", "j", "k", "l"}, lemmas,
		"words in study, still preparing or unpreparable are passed over")
	assert.Equal(t, "m-d", batch[0].MaterialID)

	more := append([]StudyCandidate{}, ordered...)
	for _, lemma := range []string{"m", "n"} {
		more = append(more, StudyCandidate{StudyIdentity: word(lemma)})
		material = append(material, StudyMaterial{ID: "m-" + lemma, StudyIdentity: word(lemma), State: StudyMaterialReady})
	}
	assert.Len(t, NextStudyBatch(more, material, progress), StudyBatchSize, "a batch holds at most ten words")
}

func TestWithoutStudyProgressDropsWordsInStudy(t *testing.T) {
	a := StudyIdentity{Language: "de", CanonicalLemma: "a", UPOS: "NOUN"}
	b := StudyIdentity{Language: "de", CanonicalLemma: "b", UPOS: "NOUN"}
	got := WithoutStudyProgress([]StudyCandidate{{StudyIdentity: a}, {StudyIdentity: b}}, []StudyProgress{{StudyIdentity: a}})
	assert.Equal(t, []StudyCandidate{{StudyIdentity: b}}, got)
}
