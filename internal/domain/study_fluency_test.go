package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func matureWord(id string, state StudyCardState, days int64) StudyProgress {
	return StudyProgress{ID: id, Language: "de", CanonicalLemma: id, UPOS: "VERB", Memory: StudyMemory{State: state, ScheduledDays: days}}
}

func TestIsStudyMature(t *testing.T) {
	cases := []struct {
		name string
		word StudyProgress
		want bool
	}{
		{"20 days", matureWord("a", StudyStateReview, 20), false},
		{"21 days", matureWord("a", StudyStateReview, 21), true},
		{"long interval", matureWord("a", StudyStateReview, 90), true},
		{"lapse puts it into relearning", matureWord("a", StudyStateRelearning, 0), false},
		{"relearning keeps no maturity whatever the old interval", matureWord("a", StudyStateRelearning, 40), false},
		{"learning", matureWord("a", StudyStateLearning, 30), false},
		{"re-graduated below the threshold", matureWord("a", StudyStateReview, 3), false},
	}
	for _, tc := range cases {
		if got := IsStudyMature(tc.word); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	withdrawn := matureWord("a", StudyStateReview, 40)
	withdrawn.Withdrawn = true
	if IsStudyMature(withdrawn) {
		t.Error("a withdrawn word is never mature")
	}
}

func TestFluencyUnlocked(t *testing.T) {
	finished, open := StudyToday{Finished: true}, StudyToday{Remaining: 2}
	cases := []struct {
		name   string
		today  StudyToday
		ledger StudyBatchLedger
		offer  bool
		want   bool
	}{
		{"reviews left", open, StudyBatchLedger{}, false, false},
		{"finished, nothing on offer", finished, StudyBatchLedger{}, false, true},
		{"finished, offered batch unanswered", finished, StudyBatchLedger{}, true, false},
		{"finished, first batch taken, the next still on offer", finished, StudyBatchLedger{Taken: 1}, true, false},
		{"finished, batch declined, the rest stays takeable", finished, StudyBatchLedger{Declined: 1}, true, true},
		{"finished, batch declined, nothing on offer", finished, StudyBatchLedger{Declined: 1}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, FluencyUnlocked(tc.today, tc.ledger, tc.offer))
		})
	}
}

func TestRankFluencyCandidatesPrefersDifferentForms(t *testing.T) {
	introduced := FluencyForm{Surface: "gehe", Features: "Number=Sing|Person=1|Tense=Pres"}
	same := FluencyCandidate{Key: "a-same", Form: introduced, Quality: 9}
	sameSurface := FluencyCandidate{Key: "b-surface", Form: FluencyForm{Surface: "Gehe", Features: "Mood=Imp"}, Quality: 1}
	past := FluencyCandidate{Key: "c-past", Form: FluencyForm{Surface: "ging", Features: "Number=Sing|Person=1|Tense=Past"}, Quality: 1}
	split := FluencyCandidate{Key: "d-split", Form: FluencyForm{Surface: "gehe", Features: introduced.Features, Split: true}, Quality: 1}
	pastAgain := FluencyCandidate{Key: "e-past-again", Form: past.Form, Quality: 8}

	got := RankFluencyCandidates(introduced, []FluencyCandidate{same, pastAgain, sameSurface, split, past})
	var keys []string
	for _, c := range got {
		keys = append(keys, c.Key)
	}
	// Each pick adds a new set of features while one is left: the past form
	// (its better sentence first), then the imperative and the split form. What
	// is left repeats a form already chosen, so a second past sentence comes
	// after a sentence of the introduced form.
	want := []string{"e-past-again", "b-surface", "d-split", "a-same", "c-past"}
	if len(keys) != len(want) {
		t.Fatalf("got %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("got %v, want %v", keys, want)
		}
	}
}

func TestRankFluencyCandidatesTieBreaks(t *testing.T) {
	form := FluencyForm{Surface: "ging", Features: "Tense=Past"}
	library := FluencyCandidate{Key: "a", Form: form, Quality: 5}
	reading := FluencyCandidate{Key: "b", Form: form, InCurrentReading: true, Quality: 1}
	got := RankFluencyCandidates(FluencyForm{Surface: "gehe"}, []FluencyCandidate{library, reading})
	if got[0].Key != "b" {
		t.Errorf("Current reading must come first, got %v", got)
	}
}

func fluencyContext(id, progress string) FluencyContext {
	return FluencyContext{ID: id, ProgressID: progress, Agreement: FluencyAgrees}
}

func TestFluencyContextsNeeded(t *testing.T) {
	words := []StudyProgress{matureWord("a", StudyStateReview, 30), matureWord("b", StudyStateReview, 30), matureWord("c", StudyStateReview, 5)}
	var contexts []FluencyContext
	for range FluencyContextsPerWord {
		contexts = append(contexts, fluencyContext("a", "a"))
	}
	contexts = append(contexts, FluencyContext{ProgressID: "b", Agreement: FluencyDisagrees})
	needed := FluencyContextsNeeded(words, contexts, nil, time.Time{})
	if len(needed) != 1 || needed[0].ID != "b" {
		t.Fatalf("got %v", needed)
	}
}

func TestParseFluencyResponse(t *testing.T) {
	for _, name := range []string{"recognised", "not-recognised"} {
		if _, ok := ParseFluencyResponse(name); !ok {
			t.Errorf("%s should parse", name)
		}
	}
	if _, ok := ParseFluencyResponse("again"); ok {
		t.Error("a grade is not a Fluency response")
	}
}

func TestFluencyReasonAgreesOnlyForTheSameMeaning(t *testing.T) {
	for name, want := range map[string]FluencyAgreement{
		"same-meaning": FluencyAgrees, "different-meaning": FluencyDisagrees, "idiomatic": FluencyDisagrees,
		"not-this-lemma": FluencyDisagrees, "undefendable": FluencyDisagrees,
	} {
		reason, ok := ParseFluencyReason(name)
		require.True(t, ok, name)
		require.Equal(t, want, reason.Agreement(), name)
	}
	_, ok := ParseFluencyReason("maybe")
	require.False(t, ok)
}

func TestRankFluencyCandidatesPutsIdiomCandidatesLast(t *testing.T) {
	plural := FluencyForm{Surface: "haeuser", Features: "Number=Plur"}
	same := FluencyForm{Surface: "haus", Features: "Number=Sing"}
	ranked := RankFluencyCandidates(same, []FluencyCandidate{
		{Key: "a", Form: plural, InIdiom: true, InCurrentReading: true, Quality: 9},
		{Key: "b", Form: same, Quality: 1},
	})
	require.Len(t, ranked, 2)
	require.Equal(t, "b", ranked[0].Key, "outside an Idiom candidate leads, whatever the form, reading or quality")
	require.Equal(t, "a", ranked[1].Key)
}

func TestFluencyContextsNeededSkipsAWordWhoseScanStillExhaustsItsCorpus(t *testing.T) {
	at := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	words := []StudyProgress{matureWord("a", StudyStateReview, 30), matureWord("b", StudyStateReview, 30), matureWord("c", StudyStateReview, 30)}
	ids := func(needed []StudyProgress) []string {
		var got []string
		for _, item := range needed {
			got = append(got, item.ID)
		}
		return got
	}
	scans := []FluencyScan{{ProgressID: "a", ExhaustedAt: at}, {ProgressID: "b", ExhaustedAt: at}}

	assert.Equal(t, []string{"c"}, ids(FluencyContextsNeeded(words, nil, scans, at.Add(-time.Hour))), "scanned words wait while nothing changed")
	assert.Equal(t, []string{"c"}, ids(FluencyContextsNeeded(words, nil, scans, time.Time{})), "a Corpus that never changed does not release them")
	assert.Equal(t, []string{"a", "b", "c"}, ids(FluencyContextsNeeded(words, nil, scans, at.Add(time.Hour))), "a Corpus change after the scan releases every scanned word")
	assert.Equal(t, []string{"a", "b", "c"}, ids(FluencyContextsNeeded(words, nil, scans, at)), "a change at the scan's own instant is not before it")

	scans[0].LastLapseAt = at.Add(time.Minute)
	assert.Equal(t, []string{"a", "c"}, ids(FluencyContextsNeeded(words, nil, scans, at.Add(-time.Hour))), "a lapse after the scan releases that word only")
	scans[0].LastLapseAt = at.Add(-time.Minute)
	assert.Equal(t, []string{"c"}, ids(FluencyContextsNeeded(words, nil, scans, at.Add(-time.Hour))), "a lapse before the scan does not")
}
