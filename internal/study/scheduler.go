package study

import (
	"fmt"
	"math"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"
)

// Scheduler is the FSRS scheduling adapter: the only place in Mouseion that
// imports go-fsrs (ADR 0134 §5, §8). It satisfies domain.StudyScheduler.
//
// It uses the library's default weights, a target recall of 0.9 and interval
// fuzz. FSRS learning and relearning steps provide the same-day repeats, with a
// single short step each: Again keeps a word in (re)learning and repeats it
// within minutes, and Got it graduates it. There is no parameter optimiser.
type Scheduler struct {
	fsrs *fsrs.FSRS
}

var (
	_ domain.StudyScheduler = (*Scheduler)(nil)
	_ domain.StudyRecaller  = (*Scheduler)(nil)
)

// NewScheduler returns the FSRS scheduler with Study's parameters.
func NewScheduler() *Scheduler {
	params := fsrs.DefaultParam()
	params.RequestRetention = 0.9
	params.EnableFuzz = true
	params.EnableShortTerm = true
	// One short step each. With the library's default {1, 10} a Got it on a new
	// word would stay in learning for ten more minutes instead of graduating.
	params.LearningSteps = []float64{1}
	params.RelearningSteps = []float64{10}
	return &Scheduler{fsrs: fsrs.NewFSRS(params)}
}

// Schedule applies one grade to a word's scheduling state at now. Got it maps
// to FSRS Good and Again to FSRS Again; Hard and Easy are never used.
//
// One rule overrides the library. The spec makes a word's introduction due on
// the next Study day when it is answered Got it (spec #2691 "Introduction is
// graded"; ADR 0134 §5), whereas FSRS would graduate it with an interval of a
// couple of days. So a Got it that graduates a word still being introduced
// (new, or learning after an Again) keeps the FSRS memory state but is due at
// the start of the next Study day. Every later review follows FSRS unchanged.
func (s *Scheduler) Schedule(memory domain.StudyMemory, grade domain.StudyGrade, now time.Time) (domain.StudyMemory, error) {
	given, ok := ratingFor(grade)
	if !ok {
		return domain.StudyMemory{}, fmt.Errorf("study: unknown grade %q", grade)
	}
	card, err := toCard(memory)
	if err != nil {
		return domain.StudyMemory{}, err
	}
	info, err := s.fsrs.Next(card, now, given)
	if err != nil {
		return domain.StudyMemory{}, fmt.Errorf("study: schedule: %w", err)
	}
	after := fromCard(info.Card)
	introducing := memory.State == domain.StudyStateNew || memory.State == domain.StudyStateLearning
	if grade == domain.StudyGradeGotIt && introducing && after.State == domain.StudyStateReview {
		after.Due = domain.NextStudyDayStart(now)
		after.ScheduledDays = 1
	}
	return after, nil
}

// Recall is the probability that the word is recalled at now, from FSRS's
// forgetting curve. It reads the state and changes nothing. A word with no
// review yet has no recall: 0 and no error.
func (s *Scheduler) Recall(memory domain.StudyMemory, now time.Time) (float64, error) {
	card, err := toCard(memory)
	if err != nil {
		return 0, err
	}
	recall, err := s.fsrs.Retrievability(card, now)
	if err != nil {
		return 0, fmt.Errorf("study: recall: %w", err)
	}
	return recall, nil
}

func ratingFor(grade domain.StudyGrade) (fsrs.Rating, bool) {
	switch grade {
	case domain.StudyGradeAgain:
		return fsrs.Again, true
	case domain.StudyGradeGotIt:
		return fsrs.Good, true
	}
	return 0, false
}

var cardStates = map[domain.StudyCardState]fsrs.State{
	domain.StudyStateNew:        fsrs.New,
	domain.StudyStateLearning:   fsrs.Learning,
	domain.StudyStateReview:     fsrs.Review,
	domain.StudyStateRelearning: fsrs.Relearning,
}

func toCard(memory domain.StudyMemory) (fsrs.Card, error) {
	state, ok := cardStates[memory.State]
	if !ok {
		return fsrs.Card{}, fmt.Errorf("study: unknown card state %q", memory.State)
	}
	return fsrs.Card{
		Due: memory.Due, Stability: memory.Stability, Difficulty: memory.Difficulty,
		ScheduledDays: nonNegative(memory.ScheduledDays), Reps: nonNegative(memory.Reps), Lapses: nonNegative(memory.Lapses),
		State: state, LastReview: memory.LastReview, RemainingSteps: memory.RemainingSteps,
	}, nil
}

func fromCard(card fsrs.Card) domain.StudyMemory {
	memory := domain.StudyMemory{
		Due: card.Due.UTC(), Stability: card.Stability, Difficulty: card.Difficulty,
		ScheduledDays: clampInt64(card.ScheduledDays), Reps: clampInt64(card.Reps), Lapses: clampInt64(card.Lapses),
		RemainingSteps: card.RemainingSteps, LastReview: card.LastReview.UTC(),
	}
	for name, state := range cardStates {
		if state == card.State {
			memory.State = name
		}
	}
	return memory
}

func nonNegative(value int64) uint64 {
	if value < 0 {
		return 0
	}
	return uint64(value)
}

func clampInt64(value uint64) int64 {
	if value > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(value)
}
