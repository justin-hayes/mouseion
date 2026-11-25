package webapp

import (
	"context"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// StudyStore is the Study page's view of the learner's stream, Study material
// and Study progress, and the writes that take a New-word batch and grade a
// card. Both the PostgreSQL store and the fixtures store satisfy it
// (ADR 0088).
type StudyStore interface {
	CountStudyMaterial(ctx context.Context, owner, language string) (domain.StudyMaterialCounts, error)
	// ListStudyCandidates and ListCurrentStudyMaterial are the facts a batch is
	// chosen from; the domain orders them.
	ListStudyCandidates(ctx context.Context, owner, language string) ([]domain.StudyCandidate, error)
	ListCurrentStudyMaterial(ctx context.Context, owner, language string) ([]domain.StudyMaterial, error)
	ListStudyProgress(ctx context.Context, owner, language string) ([]domain.StudyProgress, error)
	// StudyBatchLedger reports the batches offered, taken and declined on a
	// Study day. RecordStudyBatchOffer raises the offered count, never lowers it.
	StudyBatchLedger(ctx context.Context, owner, language, studyDay string) (domain.StudyBatchLedger, error)
	RecordStudyBatchOffer(ctx context.Context, owner, language, studyDay string, offered int) error
	// TakeStudyBatch introduces the words of one batch, and DeclineStudyBatch
	// introduces none. Either returns domain.ErrStudyBatchLimit when the day's
	// batches are all answered.
	TakeStudyBatch(ctx context.Context, owner, language, studyDay string, now time.Time, batchLimit int, words []domain.StudyBatchWord) ([]domain.StudyProgress, error)
	DeclineStudyBatch(ctx context.Context, owner, language, studyDay string, batchLimit int) error
	StudyGradeStore
	StudyFluencyStore
}

// StudyGradeStore is the Today card: the word that is due, and grading it.
type StudyGradeStore interface {
	// NextStudyCard is the earliest due word, or domain.ErrNotFound.
	NextStudyCard(ctx context.Context, owner, language string, now time.Time) (domain.StudyCard, error)
	// RecordStudyGrade appends the review-log row and updates the word's
	// state; a card already graded is domain.ErrStudyStale.
	RecordStudyGrade(ctx context.Context, owner string, record domain.StudyGradeRecord) error
}

// StudyFluencyStore is the Fluency part of the Study store: the contexts of the
// learner's mature words and the answers to them. Fluency never writes Study
// progress. ListStudyFluencyContexts returns the contexts with when each was
// last answered. RecordStudyFluencyResponse appends an answer and changes
// nothing else; a missing or disagreeing context is domain.ErrNotFound.
type StudyFluencyStore interface {
	ListStudyFluencyContexts(ctx context.Context, owner, language string) ([]domain.FluencyContext, error)
	RecordStudyFluencyResponse(ctx context.Context, owner string, record domain.FluencyResponseRecord) error
}

// Study is the seam through which the page asks for Study material to be
// prepared ahead. It only enqueues durable work: a page request never calls a
// provider (ADR 0134 §8).
type Study interface {
	EnqueueStudyPreparation(ctx context.Context, owner, language string) error
	// EnqueueFluencyPreparation asks for Fluency contexts for the mature words
	// that lack them.
	EnqueueFluencyPreparation(ctx context.Context, owner, language string) error
}
