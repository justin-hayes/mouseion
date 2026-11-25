package domain

import (
	"testing"
	"time"
)

var rotationNow = time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)

func rotationWords() []StudyProgress {
	return []StudyProgress{matureWord("a", StudyStateReview, 30), matureWord("b", StudyStateReview, 30)}
}

// answered gives a context its answers; the last decides LastResponseAt.
func answered(c FluencyContext, events ...FluencyResponseEvent) FluencyContext {
	c.Responses = events
	if len(events) > 0 {
		c.LastResponseAt = events[len(events)-1].At
	}
	return c
}

func at(response FluencyResponse, when time.Time) FluencyResponseEvent {
	return FluencyResponseEvent{Response: response, At: when}
}

func pick(t *testing.T, in FluencyRotation) string {
	t.Helper()
	card, ok := NextFluencyCard(in)
	if !ok {
		t.Fatal("expected a card")
	}
	return card.Context.ID
}

func TestNextFluencyCardFiltersAndTieBreaks(t *testing.T) {
	words := append(rotationWords(), matureWord("c", StudyStateReview, 10))
	contexts := []FluencyContext{
		fluencyContext("b1", "b"), fluencyContext("a2", "a"), fluencyContext("a1", "a"), fluencyContext("c1", "c"),
		{ID: "a-no", ProgressID: "a", Agreement: FluencyDisagrees},
	}
	in := FluencyRotation{Contexts: contexts, Progress: words, Now: rotationNow}
	// All equal: the word's identity, then the context ID.
	if got := pick(t, in); got != "a1" {
		t.Errorf("got %s, want a1", got)
	}
	if _, ok := NextFluencyCard(FluencyRotation{Progress: words, Now: rotationNow}); ok {
		t.Error("no contexts means no card")
	}
	// A word that is not mature, or a disagreeing context, is never shown.
	only := FluencyRotation{Contexts: []FluencyContext{fluencyContext("c1", "c"), {ID: "a-no", ProgressID: "a", Agreement: FluencyDisagrees}}, Progress: words, Now: rotationNow}
	if _, ok := NextFluencyCard(only); ok {
		t.Error("an immature word and a disagreeing context are not candidates")
	}
}

func TestNextFluencyCardIsDeterministic(t *testing.T) {
	contexts := []FluencyContext{fluencyContext("b1", "b"), fluencyContext("a1", "a"), fluencyContext("a2", "a")}
	reversed := []FluencyContext{contexts[2], contexts[1], contexts[0]}
	first := pick(t, FluencyRotation{Contexts: contexts, Progress: rotationWords(), Now: rotationNow})
	for range 5 {
		if got := pick(t, FluencyRotation{Contexts: reversed, Progress: rotationWords(), Now: rotationNow}); got != first {
			t.Fatalf("input order changed the pick: %s vs %s", got, first)
		}
	}
}

func TestNotRecognisedComesBackWithinTheSittingAfterOthers(t *testing.T) {
	start := rotationNow.Add(-10 * time.Minute)
	minute := func(n int) time.Time { return start.Add(time.Duration(n) * time.Minute) }
	words := append(rotationWords(), matureWord("c", StudyStateReview, 30), matureWord("d", StudyStateReview, 30), matureWord("e", StudyStateReview, 30))
	hard := answered(fluencyContext("a1", "a"), at(FluencyNotRecognised, minute(0)))
	contexts := func(n int) []FluencyContext {
		out := []FluencyContext{hard}
		for i, id := range []string{"b", "c", "d"} {
			c := fluencyContext(id+"1", id)
			if i < n {
				c = answered(c, at(FluencyRecognised, minute(i+1)))
			}
			out = append(out, c)
		}
		return append(out, fluencyContext("e1", "e"))
	}
	// Two answers later the card is still cooling down; three later it returns
	// although unanswered cards remain.
	if got := pick(t, FluencyRotation{Contexts: contexts(2), Progress: words, Now: rotationNow}); got == "a1" {
		t.Error("a context must not return before FluencyReturnAfter answers")
	}
	if got := pick(t, FluencyRotation{Contexts: contexts(3), Progress: words, Now: rotationNow}); got != "a1" {
		t.Errorf("got %s, want the not-recognised context back in the same sitting", got)
	}
	// Once it is recognised it leaves the sitting's return list.
	back := answered(hard, at(FluencyNotRecognised, minute(0)), at(FluencyRecognised, minute(5)))
	in := FluencyRotation{Contexts: append([]FluencyContext{back}, contexts(3)[1:]...), Progress: words, Now: rotationNow}
	if got := pick(t, in); got == "a1" {
		t.Error("a recognised context is not returned")
	}
}

func TestNotRecognisedIsHeavierInLaterSittings(t *testing.T) {
	later := rotationNow.Add(48 * time.Hour)
	answeredAt := rotationNow
	contexts := []FluencyContext{
		answered(fluencyContext("a1", "a"), at(FluencyRecognised, answeredAt)),
		answered(fluencyContext("a2", "a"), at(FluencyNotRecognised, answeredAt)),
		answered(fluencyContext("a3", "a"), at(FluencyRecognised, answeredAt)),
	}
	if got := pick(t, FluencyRotation{Contexts: contexts, Progress: rotationWords(), Now: later}); got != "a2" {
		t.Errorf("got %s, want the not-recognised context first in a later sitting", got)
	}
	// The effect fades, so a long time later the answer no longer decides.
	long := later.Add(120 * 24 * time.Hour)
	for _, c := range contexts {
		c.LastResponseAt = answeredAt
	}
	a := weightOf(contexts[1], long)
	b := weightOf(contexts[0], long)
	if a-b > 0.1 {
		t.Errorf("the effect of an old answer should have faded: %v vs %v", a, b)
	}
}

func weightOf(c FluencyContext, now time.Time) float64 {
	word := matureWord("a", StudyStateReview, 30)
	card := FluencyCard{Context: c, Progress: word}
	in := FluencyRotation{Now: now}
	return fluencyWeight(card, in, fluencyFormLastSeen([]FluencyCard{card}))
}

func TestFormsNotSeenRecentlyRankHigher(t *testing.T) {
	past := FluencyForm{Surface: "ging", Features: "Tense=Past"}
	pres := FluencyForm{Surface: "geht", Features: "Tense=Pres"}
	long := rotationNow.Add(-10 * 24 * time.Hour)
	recent := rotationNow.Add(-2 * time.Hour)
	// The past form of the word was seen two hours ago, the present ten days
	// ago; neither context was answered recognisably better than the other.
	one := answered(fluencyContext("a1", "a"), at(FluencyRecognised, recent))
	one.Form = past
	two := answered(fluencyContext("a2", "a"), at(FluencyRecognised, long))
	two.Form = pres
	if got := pick(t, FluencyRotation{Contexts: []FluencyContext{one, two}, Progress: rotationWords(), Now: rotationNow}); got != "a2" {
		t.Errorf("got %s, want the form not seen recently", got)
	}
}

func TestHarderWordsRankHigher(t *testing.T) {
	words := rotationWords()
	contexts := []FluencyContext{fluencyContext("a1", "a"), fluencyContext("b1", "b")}
	// Lower recall wins.
	in := FluencyRotation{Contexts: contexts, Progress: words, Now: rotationNow, Recall: map[string]float64{"a": 0.95, "b": 0.6}}
	if got := pick(t, in); got != "b1" {
		t.Errorf("got %s, want the word with lower recall", got)
	}
	// With equal recall, higher difficulty wins.
	words[0].Memory.Difficulty = 3
	words[1].Memory.Difficulty = 8
	in = FluencyRotation{Contexts: contexts, Progress: words, Now: rotationNow, Recall: map[string]float64{"a": 0.9, "b": 0.9}}
	if got := pick(t, in); got != "b1" {
		t.Errorf("got %s, want the harder word", got)
	}
	// Recall outweighs a small difficulty gap.
	words[0].Memory.Difficulty, words[1].Memory.Difficulty = 9, 8
	in.Recall = map[string]float64{"a": 0.97, "b": 0.7}
	if got := pick(t, in); got != "b1" {
		t.Errorf("got %s, want recall to decide", got)
	}
}

func TestJustAnsweredCardsCoolDownUnlessAloneAndSittingEnds(t *testing.T) {
	last := rotationNow.Add(-time.Minute)
	one := answered(fluencyContext("a1", "a"), at(FluencyNotRecognised, last))
	two := fluencyContext("b1", "b")
	if got := pick(t, FluencyRotation{Contexts: []FluencyContext{one, two}, Progress: rotationWords(), Now: rotationNow}); got != "b1" {
		t.Errorf("got %s, the card just answered must wait", got)
	}
	if got := pick(t, FluencyRotation{Contexts: []FluencyContext{one}, Progress: rotationWords(), Now: rotationNow}); got != "a1" {
		t.Errorf("got %s, a lone card is shown again", got)
	}
	// After the sitting gap there is no sitting and no cooldown.
	if got := pick(t, FluencyRotation{Contexts: []FluencyContext{one, two}, Progress: rotationWords(), Now: rotationNow.Add(FluencySittingGap + time.Hour)}); got != "a1" {
		t.Errorf("got %s, a later sitting favours the not-recognised card", got)
	}
}

func TestFluencySitting(t *testing.T) {
	base := rotationNow.Add(-5 * time.Hour)
	c := answered(fluencyContext("a1", "a"),
		at(FluencyRecognised, base), at(FluencyRecognised, base.Add(10*time.Minute)),
		at(FluencyRecognised, base.Add(2*time.Hour)), at(FluencyRecognised, base.Add(2*time.Hour+5*time.Minute)))
	if got := len(fluencySitting([]FluencyContext{c}, base.Add(2*time.Hour+10*time.Minute))); got != 2 {
		t.Errorf("sitting has %d answers, want the 2 after the pause", got)
	}
	if got := len(fluencySitting([]FluencyContext{c}, rotationNow)); got != 0 {
		t.Errorf("sitting has %d answers, want none once the gap has passed", got)
	}
}
