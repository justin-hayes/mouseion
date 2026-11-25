package domain

import (
	"math"
	"sort"
	"time"
)

// The Fluency rotation is weighted, not a schedule (spec #2691, ADR 0134 §7):
// every agreeing context of a mature word has a weight, and the heaviest is
// shown next. Nothing is fixed in time. The rotation is a pure function of the
// contexts, their answers, the words' Study progress, each word's recall and
// the current time, so equal inputs always pick the same card.

const (
	// FluencySittingGap is the longest pause between two answers that still
	// belong to one sitting. A sitting is a run of answers, each within this
	// gap of the one before; it is derived from the answers, so no session
	// state is kept.
	FluencySittingGap = 30 * time.Minute
	// FluencyReturnAfter is how many answers later in the sitting a context
	// marked "Didn't recognise it" is brought back.
	FluencyReturnAfter = 3
	// FluencyCooldown is how many of the latest answers of the sitting are
	// kept out of the rotation, so a card never repeats at once while others
	// are left. A "Didn't recognise it" context waits FluencyReturnAfter.
	FluencyCooldown = 2

	// fluencyHalfLife is how fast an answer's effect on a context's weight
	// fades.
	fluencyHalfLife = 3 * 24 * time.Hour
	// fluencyStaleCap is the age, in days, beyond which a form counts as
	// equally long unseen.
	fluencyStaleCap = 14.0
	// The weights of the rotation's terms.
	fluencyBaseWeight         = 1.0
	fluencyNotRecognisedBy    = 1.0
	fluencyRecognisedBy       = -0.5
	fluencyHardnessWeight     = 2.0
	fluencyFormStaleWeight    = 1.0
	fluencyNoveltyWeight      = 0.5
	fluencyContextStaleWeight = 0.5
	fluencyRecallWeight       = 2.0
	fluencyDifficultyWeight   = 0.5
	fluencyMinimumWeight      = 0.05
)

// FluencyRotation is the input of the rotation.
type FluencyRotation struct {
	Contexts []FluencyContext
	Progress []StudyProgress
	// Recall is each word's probability of recall now, by Study progress ID.
	// A word without an entry has no recall term.
	Recall map[string]float64
	Now    time.Time
}

// NextFluencyCard picks the card to show. Only contexts that agree with the
// learned meaning, of words that are mature, are candidates; ok is false when
// there are none.
//
// A context marked "Didn't recognise it" comes back FluencyReturnAfter answers
// later in the same sitting, the one waiting longest first. Otherwise the
// heaviest context wins, among those not just answered. A context's weight
// rises with the "Didn't recognise it" answers it has had and falls with
// "Recognised" ones, each fading over days; with how long its form has gone
// unseen for the word; with the word's low recall and high difficulty in Study
// progress; and slightly for never having been answered. Ties go to the
// context answered longest ago, then to the word and the context ID.
func NextFluencyCard(in FluencyRotation) (FluencyCard, bool) {
	cards := matureFluencyCards(in.Contexts, in.Progress)
	if len(cards) == 0 {
		return FluencyCard{}, false
	}
	sitting := fluencySitting(in.Contexts, in.Now)
	age := map[string]int{}
	lastInSitting := map[string]FluencyResponse{}
	for i, event := range sitting {
		age[event.contextID] = len(sitting) - 1 - i
		lastInSitting[event.contextID] = event.response
	}
	if card, ok := fluencyReturn(cards, age, lastInSitting); ok {
		return card, true
	}
	pool := make([]FluencyCard, 0, len(cards))
	for _, card := range cards {
		seen, answered := age[card.Context.ID]
		wait := FluencyCooldown
		if lastInSitting[card.Context.ID] == FluencyNotRecognised {
			wait = FluencyReturnAfter
		}
		if !answered || seen >= wait {
			pool = append(pool, card)
		}
	}
	if len(pool) == 0 {
		pool = cards
	}
	formSeen := fluencyFormLastSeen(cards)
	weights := make(map[string]float64, len(pool))
	for _, card := range pool {
		weights[card.Context.ID] = fluencyWeight(card, in, formSeen)
	}
	sort.SliceStable(pool, func(i, j int) bool {
		a, b := pool[i], pool[j]
		if wa, wb := weights[a.Context.ID], weights[b.Context.ID]; wa != wb {
			return wa > wb
		}
		return fluencyTieBefore(a, b)
	})
	return pool[0], true
}

type fluencyEvent struct {
	contextID string
	response  FluencyResponse
	at        time.Time
}

// fluencySitting returns the current sitting's answers, oldest first: the run
// of answers ending at the latest one, provided it is within the gap of now.
func fluencySitting(contexts []FluencyContext, now time.Time) []fluencyEvent {
	var events []fluencyEvent
	for _, context := range contexts {
		for _, response := range context.Responses {
			events = append(events, fluencyEvent{context.ID, response.Response, response.At})
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].at.Equal(events[j].at) {
			return events[i].at.Before(events[j].at)
		}
		return events[i].contextID < events[j].contextID
	})
	if len(events) == 0 || now.Sub(events[len(events)-1].at) > FluencySittingGap {
		return nil
	}
	start := len(events) - 1
	for start > 0 && events[start].at.Sub(events[start-1].at) <= FluencySittingGap {
		start--
	}
	return events[start:]
}

// fluencyReturn is the card to bring back in this sitting: a context whose
// latest answer, in the sitting, was "Didn't recognise it" and is at least
// FluencyReturnAfter answers old; the one waiting longest first.
func fluencyReturn(cards []FluencyCard, age map[string]int, last map[string]FluencyResponse) (FluencyCard, bool) {
	var best FluencyCard
	found := false
	for _, card := range cards {
		id := card.Context.ID
		waited, answered := age[id]
		if !answered || last[id] != FluencyNotRecognised || waited < FluencyReturnAfter {
			continue
		}
		if !found || waited > age[best.Context.ID] || (waited == age[best.Context.ID] && fluencyTieBefore(card, best)) {
			best, found = card, true
		}
	}
	return best, found
}

// fluencyFormLastSeen is when each form of each word was last answered.
func fluencyFormLastSeen(cards []FluencyCard) map[[2]string]time.Time {
	seen := map[[2]string]time.Time{}
	for _, card := range cards {
		key := [2]string{card.Progress.ID, card.Context.Form.Key()}
		if card.Context.LastResponseAt.After(seen[key]) {
			seen[key] = card.Context.LastResponseAt
		}
	}
	return seen
}

func fluencyWeight(card FluencyCard, in FluencyRotation, formSeen map[[2]string]time.Time) float64 {
	weight := fluencyBaseWeight
	hardness := 0.0
	for _, event := range card.Context.Responses {
		fade := math.Pow(0.5, math.Max(0, float64(in.Now.Sub(event.At)))/float64(fluencyHalfLife))
		if event.Response == FluencyNotRecognised {
			hardness += fluencyNotRecognisedBy * fade
		} else {
			hardness += fluencyRecognisedBy * fade
		}
	}
	weight += fluencyHardnessWeight * hardness
	weight += fluencyFormStaleWeight * fluencyStaleness(formSeen[[2]string{card.Progress.ID, card.Context.Form.Key()}], in.Now)
	if card.Context.LastResponseAt.IsZero() {
		weight += fluencyNoveltyWeight
	} else {
		weight += fluencyContextStaleWeight * fluencyStaleness(card.Context.LastResponseAt, in.Now)
	}
	if recall, ok := in.Recall[card.Progress.ID]; ok {
		weight += fluencyRecallWeight * (1 - math.Min(1, math.Max(0, recall)))
	}
	if difficulty := card.Progress.Memory.Difficulty; difficulty > 0 {
		weight += fluencyDifficultyWeight * (math.Min(10, math.Max(1, difficulty)) - 1) / 9
	}
	return math.Max(weight, fluencyMinimumWeight)
}

// fluencyStaleness is 1 for something never seen and rises from 0 to 1 over
// fluencyStaleCap days since it was last seen.
func fluencyStaleness(seen, now time.Time) float64 {
	if seen.IsZero() {
		return 1
	}
	days := math.Max(0, now.Sub(seen).Hours()/24)
	return math.Min(days, fluencyStaleCap) / fluencyStaleCap
}

// fluencyTieBefore orders equal weights: the context answered longest ago
// (never answered first), then the word, then the context ID.
func fluencyTieBefore(a, b FluencyCard) bool {
	if !a.Context.LastResponseAt.Equal(b.Context.LastResponseAt) {
		return a.Context.LastResponseAt.Before(b.Context.LastResponseAt)
	}
	if a.Progress.ID != b.Progress.ID {
		return studyIdentityBefore(a.Progress.StudyIdentity, b.Progress.StudyIdentity)
	}
	return a.Context.ID < b.Context.ID
}

// matureFluencyCards pairs each agreeing context of a mature word with its
// word.
func matureFluencyCards(contexts []FluencyContext, progress []StudyProgress) []FluencyCard {
	mature := make(map[string]StudyProgress, len(progress))
	for _, item := range progress {
		if IsStudyMature(item) {
			mature[item.ID] = item
		}
	}
	var cards []FluencyCard
	for _, context := range contexts {
		if word, ok := mature[context.ProgressID]; ok && context.Agreement == FluencyAgrees {
			cards = append(cards, FluencyCard{Context: context, Progress: word})
		}
	}
	return cards
}
