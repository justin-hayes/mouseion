package fixtures

// Contract status: contractual. Study reports (ADR 0135) must match the
// internal/storecontract scenarios shared with PostgreSQL (ADR 0088).

import (
	"context"
	"fmt"
	"time"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// studyAction is one Study action of a learner in a study language.
type studyAction struct{ owner, language string }

// noteStudyActionLocked records a Study action, so Undo can refuse once one
// followed a report. The caller holds s.mu.
func (s *Store) noteStudyActionLocked(owner, language string) {
	s.studyActions = append(s.studyActions, studyAction{owner: owner, language: canonicalization.NormalizeLanguage(language)})
}

// studyActionSince reports whether the learner took a Study action in the
// language after the first n actions.
func (s *Store) studyActionSince(owner, language string, n int) bool {
	for _, action := range s.studyActions[min(n, len(s.studyActions)):] {
		if action.owner == owner && action.language == language {
			return true
		}
	}
	return false
}

// studyHeldLocked reports whether the word has an open Wrong-word report. The
// caller holds s.mu.
func (s *Store) studyHeldLocked(owner string, identity domain.StudyIdentity) bool {
	for _, report := range s.studyReports {
		if report.OwnerID == owner && report.StudyIdentity == identity && report.Held() {
			return true
		}
	}
	return false
}

// CreateStudyReport records a Study report of the word's current Study
// material. A card answered since it was shown is domain.ErrStudyStale, and
// reporting the same material again while its report is open returns that
// report.
func (s *Store) CreateStudyReport(_ context.Context, owner string, record domain.StudyReportRecord) (domain.StudyReport, error) {
	if _, ok := domain.ParseStudyReportKind(string(record.Kind)); !ok {
		return domain.StudyReport{}, fmt.Errorf("create study report: unknown kind %q", record.Kind)
	}
	note, ok := domain.CleanStudyReportNote(record.Note)
	if !ok {
		return domain.StudyReport{}, fmt.Errorf("create study report: note is longer than %d characters", domain.StudyReportNoteMax)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var progress *domain.StudyProgress
	for i := range s.studyProgress {
		if s.studyProgress[i].OwnerID == owner && s.studyProgress[i].ID == record.ProgressID {
			progress = &s.studyProgress[i]
		}
	}
	if progress == nil {
		return domain.StudyReport{}, errNotFound
	}
	if existing, ok := s.repeatedStudyReportLocked(owner, *progress, record.Kind); ok {
		return existing, nil
	}
	if progress.Withdrawn || progress.Memory.Reps != record.ExpectedReps {
		return domain.StudyReport{}, domain.ErrStudyStale
	}
	material, ok := s.currentReadyMaterialLocked(owner, progress.Language, progress.MaterialID)
	if !ok {
		return domain.StudyReport{}, domain.ErrStudyStale
	}
	report := domain.StudyReport{
		ID: fmt.Sprintf("fixture-study-report-%d", len(s.studyReports)+1), OwnerID: owner, StudyIdentity: progress.StudyIdentity,
		ProgressID: progress.ID, MaterialID: material.ID, Kind: record.Kind, Note: note, Source: material.Source(),
		Sentence: material.Sentence, TargetWord: material.TargetWord, Gloss: material.Gloss, SentenceTranslation: material.SentenceTranslation,
		CreatedAt: record.CreatedAt.UTC(),
	}
	if record.Kind == domain.StudyReportUnsuitableContext {
		report.PreviousMaterialID, report.PreviousCorrectedAt, report.PreviousLearnedMeaning = progress.MaterialID, progress.CorrectedAt, progress.LearnedMeaning
	}
	s.supersedeReportedLocked(record.Kind, material.ID)
	s.noteStudyActionLocked(owner, progress.Language)
	if s.studyReportSeq == nil {
		s.studyReportSeq = map[string]int{}
	}
	s.studyReportSeq[report.ID] = len(s.studyActions)
	s.studyReports = append(s.studyReports, report)
	return report, nil
}

// supersedeReportedLocked re-introduces the word of an Unsuitable-context
// report: its reported material is replaced, so new material is prepared from
// another context (ADR 0135 §3). The caller holds s.mu.
func (s *Store) supersedeReportedLocked(kind domain.StudyReportKind, materialID string) {
	if kind != domain.StudyReportUnsuitableContext {
		return
	}
	if s.studySuperseded == nil {
		s.studySuperseded = map[string]bool{}
	}
	s.studySuperseded[materialID] = true
}

// repeatedStudyReportLocked finds the open report a new report of the kind
// would repeat: a Wrong-word report of the word, or an Unsuitable-context
// report of its current material. The caller holds s.mu.
func (s *Store) repeatedStudyReportLocked(owner string, progress domain.StudyProgress, kind domain.StudyReportKind) (domain.StudyReport, bool) {
	for _, report := range s.studyReports {
		if report.OwnerID != owner || !report.Open() || report.Kind != kind {
			continue
		}
		if (kind == domain.StudyReportWrongWord && report.ProgressID == progress.ID) || report.MaterialID == progress.MaterialID {
			return report, true
		}
	}
	return domain.StudyReport{}, false
}

// GetStudyReport returns one report, open or ended.
func (s *Store) GetStudyReport(_ context.Context, owner, id string) (domain.StudyReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, report := range s.studyReports {
		if report.OwnerID == owner && report.ID == id {
			return report, nil
		}
	}
	return domain.StudyReport{}, errNotFound
}

// GetOpenStudyReport returns the word's open report of the kind.
func (s *Store) GetOpenStudyReport(_ context.Context, owner, language, progressID string, kind domain.StudyReportKind) (domain.StudyReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, report := range s.studyReports {
		if report.OwnerID == owner && report.Language == canonicalization.NormalizeLanguage(language) && report.ProgressID == progressID && report.Kind == kind && report.Open() {
			return report, nil
		}
	}
	return domain.StudyReport{}, errNotFound
}

func (s *Store) openStudyReportsLocked(match func(domain.StudyReport) bool) []domain.StudyReport {
	var reports []domain.StudyReport
	for _, report := range s.studyReports {
		if report.Open() && match(report) {
			reports = append(reports, report)
		}
	}
	return reports
}

// ListOpenStudyReports returns the learner's open reports of the kind in the
// study language, oldest first.
func (s *Store) ListOpenStudyReports(_ context.Context, owner, language string, kind domain.StudyReportKind) ([]domain.StudyReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.openStudyReportsLocked(func(r domain.StudyReport) bool {
		return r.OwnerID == owner && r.Language == canonicalization.NormalizeLanguage(language) && r.Kind == kind
	}), nil
}

// ListOpenWrongWordReportsForBook returns the learner's open Wrong-word reports
// whose reported occurrence is in the Book.
func (s *Store) ListOpenWrongWordReportsForBook(_ context.Context, owner, bookID string) ([]domain.StudyReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.openStudyReportsLocked(func(r domain.StudyReport) bool {
		return r.OwnerID == owner && r.Source.BookID == bookID && r.Kind == domain.StudyReportWrongWord
	}), nil
}

// ListRetiredStudyOccurrences returns the occurrences retired as the word's
// context by its open Unsuitable-context reports, each with its locator.
func (s *Store) ListRetiredStudyOccurrences(_ context.Context, owner, progressID string) ([]domain.StudyOccurrenceRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retiredLocked(owner, func(r domain.StudyReport) bool { return r.ProgressID == progressID }), nil
}

// ListRetiredStudyOccurrencesOfWord is ListRetiredStudyOccurrences by the word's
// vocabulary identity.
func (s *Store) ListRetiredStudyOccurrencesOfWord(_ context.Context, owner string, identity domain.StudyIdentity) ([]domain.StudyOccurrenceRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retiredLocked(owner, func(r domain.StudyReport) bool { return r.StudyIdentity == identity }), nil
}

func (s *Store) retiredLocked(owner string, match func(domain.StudyReport) bool) []domain.StudyOccurrenceRef {
	refs := []domain.StudyOccurrenceRef{}
	for _, report := range s.openStudyReportsLocked(func(r domain.StudyReport) bool {
		return r.OwnerID == owner && r.Kind == domain.StudyReportUnsuitableContext && match(r)
	}) {
		refs = append(refs, s.locatedLocked(owner, report.Source))
	}
	return refs
}

// locatedLocked is the occurrence with its locator, or as it was when it has
// none. The caller holds s.mu.
func (s *Store) locatedLocked(owner string, ref domain.StudyOccurrenceRef) domain.StudyOccurrenceRef {
	if located, err := s.studyLocatorLocked(owner, ref); err == nil {
		return located
	}
	return ref
}

// hasOpenUnsuitableReportLocked reports whether the word has an open
// Unsuitable-context report. The caller holds s.mu.
func (s *Store) hasOpenUnsuitableReportLocked(owner, progressID string) bool {
	return len(s.openStudyReportsLocked(func(r domain.StudyReport) bool {
		return r.OwnerID == owner && r.ProgressID == progressID && r.Kind == domain.StudyReportUnsuitableContext
	})) > 0
}

// contextReportedAtLocked is when the word's latest open Unsuitable-context
// report was made, zero when it has none. The caller holds s.mu.
func (s *Store) contextReportedAtLocked(owner, progressID string) time.Time {
	var latest time.Time
	for _, report := range s.openStudyReportsLocked(func(r domain.StudyReport) bool {
		return r.OwnerID == owner && r.ProgressID == progressID && r.Kind == domain.StudyReportUnsuitableContext
	}) {
		if report.CreatedAt.After(latest) {
			latest = report.CreatedAt
		}
	}
	return latest
}

// withoutRetiredFluencyContextsLocked leaves out the contexts on an occurrence
// the learner retired as the word's context. The caller holds s.mu.
func (s *Store) withoutRetiredFluencyContextsLocked(owner string, contexts []domain.FluencyContext) []domain.FluencyContext {
	kept := contexts[:0:0]
	for _, item := range contexts {
		retired := s.retiredLocked(owner, func(r domain.StudyReport) bool { return r.ProgressID == item.ProgressID })
		ref := domain.StudyOccurrenceRef{BookID: item.BookID, AnalysisRunID: item.AnalysisRunID, CorpusID: item.CorpusID, SentenceOrdinal: item.SentenceOrdinal, TokenOrdinal: item.TokenOrdinal}
		if len(retired) > 0 && domain.RetiredStudyOccurrence(retired, s.locatedLocked(owner, ref)) {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

func (s *Store) openReportIndexLocked(owner, id string) int {
	for i, report := range s.studyReports {
		if report.OwnerID == owner && report.ID == id && report.Open() {
			return i
		}
	}
	return -1
}

func (s *Store) progressByIDLocked(owner, id string) *domain.StudyProgress {
	for i := range s.studyProgress {
		if s.studyProgress[i].OwnerID == owner && s.studyProgress[i].ID == id {
			return &s.studyProgress[i]
		}
	}
	return nil
}

// EndStudyReport ends an open report because its occurrence was repaired or is
// gone. A report that is missing, foreign or already ended is ErrNotFound.
func (s *Store) EndStudyReport(_ context.Context, owner, id string, reason domain.StudyReportEnd, now time.Time) error {
	if !reason.Repair() {
		return fmt.Errorf("end study report: %q is not a repair", reason)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.openReportIndexLocked(owner, id)
	if index < 0 {
		return errNotFound
	}
	s.studyReports[index].EndedAt, s.studyReports[index].EndReason = now.UTC(), reason
	return nil
}

// DismissStudyReport ends an open Wrong-word report without a repair and moves
// the word's due time later by the time it was held.
func (s *Store) DismissStudyReport(_ context.Context, owner, id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.openReportIndexLocked(owner, id)
	if index < 0 || s.studyReports[index].Kind != domain.StudyReportWrongWord {
		return errNotFound
	}
	report := &s.studyReports[index]
	report.EndedAt, report.EndReason = now.UTC(), domain.StudyReportEndDismissed
	if progress := s.progressByIDLocked(owner, report.ProgressID); progress != nil {
		progress.Memory.Due = domain.StudyHeldDue(progress.Memory.Due, report.CreatedAt, now).UTC()
		progress.UpdatedAt = now.UTC()
	}
	return nil
}

// UndoStudyReport ends an open Unsuitable-context report as undone and returns
// the word to the material it had. It is domain.ErrStudyStale once another
// Study action of the learner followed the report.
func (s *Store) UndoStudyReport(_ context.Context, owner, id string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.openReportIndexLocked(owner, id)
	if index < 0 || s.studyReports[index].Kind != domain.StudyReportUnsuitableContext {
		return errNotFound
	}
	report := &s.studyReports[index]
	if s.studyActionSince(owner, report.Language, s.studyReportSeq[report.ID]) {
		return domain.ErrStudyStale
	}
	report.EndedAt, report.EndReason = now.UTC(), domain.StudyReportEndUndone
	progress := s.progressByIDLocked(owner, report.ProgressID)
	if progress == nil || report.PreviousMaterialID == "" {
		return nil
	}
	for _, material := range s.studyMaterial {
		if material.OwnerID == owner && material.StudyIdentity == progress.StudyIdentity && material.ID != report.PreviousMaterialID && !s.studySuperseded[material.ID] {
			s.studySuperseded[material.ID] = true
		}
	}
	delete(s.studySuperseded, report.PreviousMaterialID)
	progress.MaterialID, progress.LearnedMeaning, progress.CorrectedAt = report.PreviousMaterialID, report.PreviousLearnedMeaning, report.PreviousCorrectedAt
	progress.UpdatedAt = now.UTC()
	return nil
}
