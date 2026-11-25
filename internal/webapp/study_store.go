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
	// StudyBatchLedger reports the batches taken and declined on a Study day.
	StudyBatchLedger(ctx context.Context, owner, language, studyDay string) (domain.StudyBatchLedger, error)
	// TakeStudyBatch introduces the words of one batch, and DeclineStudyBatch
	// introduces none. Either returns domain.ErrStudyBatchLimit when the day's
	// batches are all answered.
	TakeStudyBatch(ctx context.Context, owner, language, studyDay string, now time.Time, batchLimit int, words []domain.StudyBatchWord) ([]domain.StudyProgress, error)
	DeclineStudyBatch(ctx context.Context, owner, language, studyDay string, batchLimit int) error
	// NextStudyCard is the earliest due word, or domain.ErrNotFound.
	NextStudyCard(ctx context.Context, owner, language string, now time.Time) (domain.StudyCard, error)
	// RecordStudyGrade appends the review-log row and updates the word's
	// state; a card already graded is domain.ErrStudyStale.
	RecordStudyGrade(ctx context.Context, owner string, record domain.StudyGradeRecord) error
	StudyCorrectionsStore
}

// StudyCorrectionsStore is the surface that lets Study react to a Lemma
// correction, an Occurrence exclusion or an Analysis update (ADR 0134 §6).
type StudyCorrectionsStore interface {
	// ListStudyOccurrences reads the occurrences that resolve to a word's
	// identity under current evidence; GetStudyMaterial reads the material a
	// word was introduced in.
	ListStudyOccurrences(ctx context.Context, owner string, identity domain.StudyIdentity) ([]domain.StudyOccurrenceRef, error)
	GetStudyMaterial(ctx context.Context, owner, id string) (domain.StudyMaterial, error)
	// SupersedeStudyMaterial replaces a word's material so it is prepared
	// again, and SetStudyWordWithdrawn withdraws a word or returns it, keeping
	// its progress.
	SupersedeStudyMaterial(ctx context.Context, owner, materialID string) error
	SetStudyWordWithdrawn(ctx context.Context, owner, progressID string, withdrawn bool, now time.Time) error
	// CompleteStudyReintroductions moves each word whose new material is ready
	// onto it, and withdraws one whose new material cannot be prepared.
	CompleteStudyReintroductions(ctx context.Context, owner, language string, now time.Time) error
	// StudyOccurrenceLocator completes an occurrence with the unit and token
	// span an Analysis update carries decisions across runs by, and
	// RelocateStudyMaterial names where a word's material now comes from when an
	// Analysis update re-published its introducing occurrence unchanged.
	StudyOccurrenceLocator(ctx context.Context, owner string, ref domain.StudyOccurrenceRef) (domain.StudyOccurrenceRef, error)
	RelocateStudyMaterial(ctx context.Context, owner, materialID string, ref domain.StudyOccurrenceRef) error
}

// Study is the seam through which the page asks for Study material to be
// prepared ahead. It only enqueues durable work: a page request never calls a
// provider (ADR 0134 §8).
type Study interface {
	EnqueueStudyPreparation(ctx context.Context, owner, language string) error
}
