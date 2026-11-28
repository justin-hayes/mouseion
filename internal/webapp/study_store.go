package webapp

import (
	"context"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// StudyStore is the Study page's view of the learner's stream, Study material
// and Study progress, and the writes that take a New-word batch and grade a
// card. Both the PostgreSQL store and the fixtures store satisfy it
// (ADR 0088).
type StudyStore interface { //nolint:interfacebloat // the Study page's store is one seam over progress, batches, grades, corrections, Fluency and reports
	CountStudyMaterial(ctx context.Context, owner, language string) (domain.StudyMaterialCounts, error)
	// GetStudyStream is what orders the stream preparation and batches are chosen
	// from: the candidates and the Current reading's coverage; the domain orders
	// them. ListCurrentStudyMaterial, with it, is in StudyCorrectionsStore.
	GetStudyStream(ctx context.Context, owner, language string) (domain.StudyStream, error)
	// GetStudyStreamAmong is the same fact for the given identities only,
	// without reading the library's other candidates.
	GetStudyStreamAmong(ctx context.Context, owner, language string, identities []domain.StudyItem) (domain.StudyStream, error)
	ListStudyProgress(ctx context.Context, owner, language string) ([]domain.StudyProgress, error)
	// CountStudyReviewsDue is how many reviews are still due on the Study day
	// containing now: the count Today's finish line holds, read as one count.
	CountStudyReviewsDue(ctx context.Context, owner, language string, now time.Time) (int, error)
	// StudyBatchLedger reports the batches offered, taken and declined on a
	// Study day. RecordStudyBatchOffer raises the offered count, never lowers it.
	StudyBatchLedger(ctx context.Context, owner, language, studyDay string) (domain.StudyBatchLedger, error)
	RecordStudyBatchOffer(ctx context.Context, owner, language, studyDay string, offered int) error
	// TakeStudyBatch introduces the words of one batch, and DeclineStudyBatch
	// introduces none. Either returns domain.ErrStudyBatchLimit when the day's
	// batches are all answered.
	TakeStudyBatch(ctx context.Context, owner, language, studyDay string, now time.Time, batchLimit int, words []domain.StudyBatchItem) ([]domain.StudyProgress, error)
	DeclineStudyBatch(ctx context.Context, owner, language, studyDay string, batchLimit int) error
	// ListCorpusSentences reads sentences with their tokens, which an Idiom
	// card's matched words are taken from.
	ListCorpusSentences(ctx context.Context, owner, corpusID string, ordinals []int64) (map[int64]analyzer.Sentence, error)
	StudyGradeStore
	StudyFluencyStore
	StudyReportStore
}

// StudyGradeStore is the Today card: the word that is due, and grading it.
type StudyGradeStore interface {
	// NextStudyCard is the earliest due word, or domain.ErrNotFound.
	NextStudyCard(ctx context.Context, owner, language string, now time.Time) (domain.StudyCard, error)
	// RecordStudyGrade appends the review-log row and updates the word's
	// state; a card already graded is domain.ErrStudyStale.
	RecordStudyGrade(ctx context.Context, owner string, record domain.StudyGradeRecord) error
	// RecordStudyAlreadyKnown stores the Already-known answer on an
	// introduction as an event, never a grade, with the word's new state; an
	// introduction already answered is domain.ErrStudyStale.
	RecordStudyAlreadyKnown(ctx context.Context, owner string, record domain.StudyAlreadyKnownRecord) error
	StudyCorrectionsStore
}

// StudyCorrectionsStore is the surface that lets Study react to a Lemma
// correction, an Occurrence exclusion or an Analysis update (ADR 0134 §6).
type StudyCorrectionsStore interface {
	// ListStudyOccurrences reads the occurrences that resolve to a word's
	// identity under current evidence; GetStudyMaterial reads the material a
	// word was introduced in.
	ListStudyOccurrences(ctx context.Context, owner string, identity domain.StudyItem) ([]domain.StudyOccurrenceRef, error)
	// FindStudyOccurrences is the correction check's cheaper read: the source
	// when it still resolves to the identity, else one valid occurrence, so its
	// cost does not grow with how often the word occurs.
	FindStudyOccurrences(ctx context.Context, owner string, identity domain.StudyItem, source domain.StudyOccurrenceRef) ([]domain.StudyOccurrenceRef, error)
	GetStudyMaterial(ctx context.Context, owner, id string) (domain.StudyMaterial, error)
	ListCurrentStudyMaterial(ctx context.Context, owner, language string) ([]domain.StudyMaterial, error)
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

// StudyReportStore is the Study report part of the Study store (ADR 0135). A
// report is of the word's current Study material; the store copies the reported
// occurrence from it. An open Wrong-word report holds the word, which
// ListStudyProgress reports as Held. CreateStudyReport returns the open report
// when the same material is reported twice, and domain.ErrStudyStale for a card
// answered since it was shown. EndStudyReport ends a report whose occurrence was
// repaired or is gone, releasing a Wrong-word report's word at the start of the
// next Study day, DismissStudyReport ends a Wrong-word report without a
// repair and moves the word's due time by the time it was held, and
// UndoStudyReport restores an Unsuitable-context report's word; it is
// domain.ErrStudyStale once another Study action followed.
type StudyReportStore interface {
	CreateStudyReport(ctx context.Context, owner string, record domain.StudyReportRecord) (domain.StudyReport, error)
	GetStudyReport(ctx context.Context, owner, id string) (domain.StudyReport, error)
	GetOpenStudyReport(ctx context.Context, owner, language, progressID string, kind domain.StudyReportKind) (domain.StudyReport, error)
	// GetOpenStudyReportOfMaterial finds the open report of the kind on a Study
	// material, including one whose word has no Study progress left.
	GetOpenStudyReportOfMaterial(ctx context.Context, owner, materialID string, kind domain.StudyReportKind) (domain.StudyReport, error)
	ListOpenStudyReports(ctx context.Context, owner, language string, kind domain.StudyReportKind) ([]domain.StudyReport, error)
	// ListOpenWrongWordReportsForBook is what the Book's Lemma review lists next
	// to its Lemma review flags.
	ListOpenWrongWordReportsForBook(ctx context.Context, owner, bookID string) ([]domain.StudyReport, error)
	StudyReportEndingStore
	// StudyReportUndoable is whether Undo is still offered: the report is open
	// and no other Study action followed.
	StudyReportUndoable(ctx context.Context, owner, id string) (bool, error)
	StudyReportEvidenceStore
}

// StudyReportEndingStore is how a report ends. RetireClosedStudyReports ends, as
// retired, the learner's open Unsuitable-context reports in the language whose
// Undo window has closed because another Study action followed; their sentences
// stay retired.
type StudyReportEndingStore interface {
	EndStudyReport(ctx context.Context, owner, id string, reason domain.StudyReportEnd, now time.Time) error
	DismissStudyReport(ctx context.Context, owner, id string, now time.Time) error
	UndoStudyReport(ctx context.Context, owner, id string, now time.Time) error
	RetireClosedStudyReports(ctx context.Context, owner, language string, now time.Time) error
}

// StudyReportEvidenceStore is what Study reports leave on the word's evidence
// and what current evidence says of a reported occurrence: the occurrences open
// Unsuitable-context reports retired, read when material and Fluency contexts
// are chosen, and the state reconciliation ends a report by.
type StudyReportEvidenceStore interface {
	// ListRetiredStudyOccurrences names the occurrences the word's
	// Unsuitable-context reports not ended as undone retired as its context.
	ListRetiredStudyOccurrences(ctx context.Context, owner, progressID string) ([]domain.StudyOccurrenceRef, error)
	// ListRetiredStudyOccurrencesOfWord is the same by the word's vocabulary
	// identity, for material preparation, which knows no progress row.
	ListRetiredStudyOccurrencesOfWord(ctx context.Context, owner string, identity domain.StudyItem) ([]domain.StudyOccurrenceRef, error)
	// StudyOccurrenceState says whether a reported occurrence is still in the
	// current analysis of a Book in the library, and whether the learner
	// excluded or corrected it.
	StudyOccurrenceState(ctx context.Context, owner, language string, source domain.StudyOccurrenceRef) (domain.StudyOccurrenceState, error)
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
	// IdiomCandidates are the language's Idioms in the stream, with their
	// Corpus candidate counts; none while the language's idiom switch is off.
	IdiomsActive(language string) bool
	IdiomCandidates(ctx context.Context, owner, language string) ([]domain.StudyCandidate, error)
	// ReviewIdiom checks an Idiom in study against current evidence and applies
	// what that requires; it reports whether the Idiom's progress or material
	// changed (ADR 0137 §5).
	ReviewIdiom(ctx context.Context, owner string, word domain.StudyProgress, now time.Time) (bool, error)
	// KeepIdiomReviewable returns an Idiom waiting for new material to its last
	// material while the language's idiom switch is off, so it is still reviewed.
	KeepIdiomReviewable(ctx context.Context, owner string, word domain.StudyProgress) (bool, error)
}
