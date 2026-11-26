package domain

import (
	"errors"
	"time"
)

// StudyBatchSize is how many new words a New-word batch offers (spec #2691).
const StudyBatchSize = 10

// StudyGrade is the learner's answer to a Study card. Study has exactly two
// grades; they map to FSRS Again and Good, never Hard or Easy (ADR 0134 §5).
type StudyGrade string

const (
	// StudyGradeAgain keeps the word in learning and repeats it the same day.
	StudyGradeAgain StudyGrade = "again"
	// StudyGradeGotIt is a recalled answer.
	StudyGradeGotIt StudyGrade = "got-it"
)

// ParseStudyGrade reads a grade from its stored or submitted name.
func ParseStudyGrade(name string) (StudyGrade, bool) {
	switch grade := StudyGrade(name); grade {
	case StudyGradeAgain, StudyGradeGotIt:
		return grade, true
	}
	return "", false
}

// StudyCardState is where a word is in the FSRS lifecycle.
type StudyCardState string

const (
	// StudyStateNew is a word introduced but not yet graded.
	StudyStateNew StudyCardState = "new"
	// StudyStateLearning is a word still repeating on the day it was introduced.
	StudyStateLearning StudyCardState = "learning"
	// StudyStateReview is a word on a day-scale schedule.
	StudyStateReview StudyCardState = "review"
	// StudyStateRelearning is a reviewed word that was forgotten and repeats
	// on the same day.
	StudyStateRelearning StudyCardState = "relearning"
)

// StudyMemory is a word's scheduling state: the FSRS memory state and due time.
// It is plain values, so the domain stays free of the scheduling library. The
// review log stores it before and after every grade, which is enough to replay
// the history under other parameters.
type StudyMemory struct {
	State          StudyCardState `json:"state"`
	Due            time.Time      `json:"due"`
	Stability      float64        `json:"stability"`
	Difficulty     float64        `json:"difficulty"`
	ScheduledDays  int64          `json:"scheduled_days"`
	Reps           int64          `json:"reps"`
	Lapses         int64          `json:"lapses"`
	RemainingSteps int            `json:"remaining_steps"`
	LastReview     time.Time      `json:"last_review"`
}

// NewStudyMemory is the state of a word at introduction: new and due now.
func NewStudyMemory(now time.Time) StudyMemory {
	return StudyMemory{State: StudyStateNew, Due: now.UTC()}
}

// StudyWithdrawnNoSuitableContext is the reason a word is withdrawn when no
// other context passes the sentence-quality gate after an Unsuitable-context
// report (ADR 0135 §3).
const StudyWithdrawnNoSuitableContext = "no_suitable_context"

// StudyProgress is one word in study (ADR 0134 §2): its scheduling state, when
// it was introduced, the meaning it was learned with and the Study material it
// is currently reviewed in. It belongs to the word, never to a Book.
type StudyProgress struct {
	ID, OwnerID string
	StudyIdentity
	MaterialID     string
	IntroducedAt   time.Time
	LearnedMeaning string
	Memory         StudyMemory
	// Withdrawn marks a word a correction left with no valid occurrence. Its
	// progress is kept and it is not due.
	Withdrawn bool
	// WithdrawnReason says why the word was withdrawn: empty for a withdrawal
	// by a correction, StudyWithdrawnNoSuitableContext when every context was
	// reported as unsuitable.
	WithdrawnReason string
	// Held marks a word with an open Wrong-word report (ADR 0135 §2). It is
	// derived from the open reports, never stored on the word: a held word gets
	// no reviews, is not part of Today's finish line and does not fall overdue.
	// Its progress is kept.
	Held bool
	// CorrectedAt is when a correction re-introduced the word in new Study
	// material, zero for a word never corrected. Reintroducing is true while
	// the word's replacement material is still being prepared: the review is
	// due but there is no card to show yet.
	CorrectedAt          time.Time
	Reintroducing        bool
	CreatedAt, UpdatedAt time.Time
}

// Corrected reports that the word's card is its re-introduction after a
// correction: it was moved to new material and has not been answered since.
func (p StudyProgress) Corrected() bool {
	return !p.CorrectedAt.IsZero() && (p.Memory.LastReview.IsZero() || p.Memory.LastReview.Before(p.CorrectedAt))
}

// Graded reports whether the learner has answered the word at least once.
func (p StudyProgress) Graded() bool { return p.Memory.Reps > 0 }

// StudyCard is a due word with the Study material it is shown in.
type StudyCard struct {
	Progress StudyProgress
	Material StudyMaterial
}

// StudyReviewLog is one append-only row per graded review: the grade, time,
// Study day and the scheduling state before and after.
type StudyReviewLog struct {
	ID, OwnerID, ProgressID, MaterialID string
	Grade                               StudyGrade
	ReviewedAt                          time.Time
	StudyDay                            string
	Before, After                       StudyMemory
}

// StudyGradeRecord is a grade ready to be stored: the state the scheduler
// computed for it. ExpectedReps is the number of grades the learner saw
// recorded when the card was shown, so a repeated tap of the same card is
// rejected rather than graded twice.
type StudyGradeRecord struct {
	ProgressID   string
	ExpectedReps int64
	Grade        StudyGrade
	ReviewedAt   time.Time
	StudyDay     string
	After        StudyMemory
}

// StudyScheduler computes a word's next scheduling state from a grade. The
// FSRS adapter in internal/study is its only implementation.
type StudyScheduler interface {
	Schedule(memory StudyMemory, grade StudyGrade, now time.Time) (StudyMemory, error)
}

// StudyRecaller reports how likely a word is to be recalled at a time, from
// its scheduling state. It only reads: Fluency uses it to weight contexts and
// never writes Study progress. The FSRS adapter in internal/study implements
// it alongside StudyScheduler.
type StudyRecaller interface {
	Recall(memory StudyMemory, now time.Time) (float64, error)
}

// StudyBatchWord is one word of a New-word batch with the Study material it
// is introduced in.
type StudyBatchWord struct {
	StudyIdentity
	MaterialID string
	// ProgressID is set for a word carried over from an earlier Study day: it
	// is already in study and is taken again rather than introduced.
	ProgressID string
}

// StudyBatchLedger is what Today has offered on one Study day: the batches
// offered, taken and declined.
type StudyBatchLedger struct {
	StudyDay                 string
	Offered, Taken, Declined int
}

// Decided is how many distinct batches the learner has answered on the day: a
// batch that was declined and then taken counts once, as taken.
func (l StudyBatchLedger) Decided() int { return l.Taken + l.Declined }

// OfferNumber is the ordinal among the day's batches of the batch on offer. A
// declined batch is still the one on offer, so it keeps its number.
func (l StudyBatchLedger) OfferNumber() int {
	if l.Declined > 0 {
		return l.Decided()
	}
	return l.Decided() + 1
}

// Take records one batch taken under the day's limit on distinct batches
// offered. A batch the learner declined earlier is taken in place of the
// declined one and consumes no other batch. ErrStudyBatchLimit means none is
// left to take.
func (l StudyBatchLedger) Take(limit int) (StudyBatchLedger, error) {
	if !StudyBatchTakeable(l, limit) {
		return l, ErrStudyBatchLimit
	}
	if l.Declined > 0 {
		l.Declined--
	}
	l.Taken++
	l.Offered = max(l.Offered, l.Decided())
	return l, nil
}

// Decline records one batch declined under the day's limit. ErrStudyBatchLimit
// means none is left to answer.
func (l StudyBatchLedger) Decline(limit int) (StudyBatchLedger, error) {
	if l.Decided() >= limit {
		return l, ErrStudyBatchLimit
	}
	l.Declined++
	l.Offered = max(l.Offered, l.Decided())
	return l, nil
}

// Errors of the Study progress store.
var (
	// ErrStudyBatchLimit rejects a batch answer beyond the day's batches.
	ErrStudyBatchLimit = errors.New("study: no batch left to answer today")
	// ErrStudyStale rejects a grade for a card that was already graded or is
	// no longer the learner's.
	ErrStudyStale = errors.New("study: the card was already graded")
)

// NextStudyBatch returns the words a New-word batch introduces. The words
// carried over from earlier Study days lead it, oldest first, and count
// towards its size; the first candidates in stream order that are not in study
// and whose current Study material is ready fill the rest, up to
// StudyBatchSize. A word whose material is still being prepared or is
// unpreparable is passed over, so a batch never holds a card with missing
// material. A held word (an open Wrong-word report, ADR 0135 §2) is passed
// over too: held names the identities held by a report that left no Study
// progress, and a held word in progress is never carried over. ordered must
// already be in stream order.
func NextStudyBatch(ordered []StudyCandidate, material []StudyMaterial, progress []StudyProgress, held []StudyIdentity, now time.Time) []StudyBatchWord {
	ready := make(map[StudyIdentity]string, len(material))
	for _, item := range material {
		if item.State == StudyMaterialReady {
			ready[item.StudyIdentity] = item.ID
		}
	}
	inStudy := studyIdentitySet(progress, held)
	batch := make([]StudyBatchWord, 0, StudyBatchSize)
	for _, item := range StudyCarryOver(progress, now) {
		if len(batch) == StudyBatchSize {
			return batch
		}
		batch = append(batch, StudyBatchWord{StudyIdentity: item.StudyIdentity, MaterialID: item.MaterialID, ProgressID: item.ID})
	}
	for _, candidate := range ordered {
		if len(batch) == StudyBatchSize {
			break
		}
		id, ok := ready[candidate.StudyIdentity]
		if !ok || inStudy[candidate.StudyIdentity] {
			continue
		}
		batch = append(batch, StudyBatchWord{StudyIdentity: candidate.StudyIdentity, MaterialID: id})
	}
	return batch
}

// StudyBatchLookup returns the identities NextStudyBatch can fill a batch
// with from the stream: words whose material is ready and that are not in
// study or held. Only these need their place in the stream, so a store need not
// read the other candidates.
func StudyBatchLookup(material []StudyMaterial, progress []StudyProgress, held []StudyIdentity) []StudyIdentity {
	inStudy := studyIdentitySet(progress, held)
	var identities []StudyIdentity
	for _, item := range material {
		if item.State == StudyMaterialReady && !inStudy[item.StudyIdentity] {
			identities = append(identities, item.StudyIdentity)
		}
	}
	return identities
}

// WithoutStudyProgress drops the candidates already in study, so preparation
// and batches only look ahead at new words.
func WithoutStudyProgress(candidates []StudyCandidate, progress []StudyProgress) []StudyCandidate {
	inStudy := make(map[StudyIdentity]bool, len(progress))
	for _, item := range progress {
		inStudy[item.StudyIdentity] = true
	}
	remaining := make([]StudyCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !inStudy[candidate.StudyIdentity] {
			remaining = append(remaining, candidate)
		}
	}
	return remaining
}

// studyIdentitySet is the words a batch cannot introduce: those in study and
// those held by a report.
func studyIdentitySet(progress []StudyProgress, held []StudyIdentity) map[StudyIdentity]bool {
	set := make(map[StudyIdentity]bool, len(progress)+len(held))
	for _, item := range progress {
		set[item.StudyIdentity] = true
	}
	for _, identity := range held {
		set[identity] = true
	}
	return set
}
