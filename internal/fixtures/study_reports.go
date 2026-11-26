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
	progress := s.progressByIDLocked(owner, record.ProgressID)
	if existing, ok, err := s.repeatedOrStaleReportLocked(owner, progress, record); err != nil || ok {
		return existing, err
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
		report.PreviousLearnedCitations = progress.LearnedCitations.Clone()
	}
	s.supersedeReportedLocked(record.Kind, material.ID)
	s.noteStudyActionLocked(owner, progress.Language)
	if s.studyReportSeq == nil {
		s.studyReportSeq = map[string]int{}
	}
	s.studyReportSeq[report.ID] = len(s.studyActions)
	if record.Kind == domain.StudyReportWrongWord && progress.Memory.Reps == 0 {
		// A word never graded stays unintroduced while it is held: it has no
		// Study progress to hold. The stored report keeps naming the word.
		s.removeStudyProgressLocked(owner, progress.ID)
		stored := report
		stored.ProgressID = ""
		s.studyReports = append(s.studyReports, stored)
		return report, nil
	}
	s.studyReports = append(s.studyReports, report)
	return report, nil
}

// repeatedOrStaleReportLocked is the check a report makes under the word's lock:
// a repeat of a report that stands returns it, and a card answered or whose
// material was replaced since it was shown is stale. A word with no Study
// progress is missing unless a report of its material stands. The caller holds
// s.mu.
func (s *Store) repeatedOrStaleReportLocked(owner string, progress *domain.StudyProgress, record domain.StudyReportRecord) (domain.StudyReport, bool, error) {
	if progress == nil {
		// A word reported before its first graded review has no progress left: a
		// repeat of that report is the report, not a missing word.
		if existing, ok := s.openReportOfMaterialLocked(owner, record.MaterialID, record.Kind); ok {
			return existing, true, nil
		}
		return domain.StudyReport{}, false, errNotFound
	}
	if existing, ok := s.repeatedStudyReportLocked(owner, *progress, record); ok {
		return existing, true, nil
	}
	// The material the learner was shown is compared under the lock.
	if progress.Withdrawn || progress.Memory.Reps != record.ExpectedReps || (record.MaterialID != "" && progress.MaterialID != record.MaterialID) {
		return domain.StudyReport{}, false, domain.ErrStudyStale
	}
	return domain.StudyReport{}, false, nil
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

// repeatedStudyReportLocked finds the open report a new report would repeat: a
// Wrong-word report of the word, or an Unsuitable-context report of the
// material the learner was shown (the word's current material when the record
// names none). The caller holds s.mu.
func (s *Store) repeatedStudyReportLocked(owner string, progress domain.StudyProgress, record domain.StudyReportRecord) (domain.StudyReport, bool) {
	if record.Kind == domain.StudyReportWrongWord {
		for _, report := range s.studyReports {
			if report.OwnerID == owner && report.Open() && report.Kind == record.Kind && report.ProgressID == progress.ID {
				return report, true
			}
		}
		return domain.StudyReport{}, false
	}
	material := record.MaterialID
	if material == "" {
		material = progress.MaterialID
	}
	return s.openReportOfMaterialLocked(owner, material, record.Kind)
}

// openReportOfMaterialLocked finds the open report of the kind on the material.
// The caller holds s.mu.
func (s *Store) openReportOfMaterialLocked(owner, material string, kind domain.StudyReportKind) (domain.StudyReport, bool) {
	for _, report := range s.studyReports {
		if material != "" && report.OwnerID == owner && report.Open() && report.Kind == kind && report.MaterialID == material {
			return report, true
		}
	}
	return domain.StudyReport{}, false
}

// GetOpenStudyReportOfMaterial returns the open report of the kind on the Study
// material, including one whose word has no Study progress.
func (s *Store) GetOpenStudyReportOfMaterial(_ context.Context, owner, materialID string, kind domain.StudyReportKind) (domain.StudyReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if report, ok := s.openReportOfMaterialLocked(owner, materialID, kind); ok {
		return report, nil
	}
	return domain.StudyReport{}, errNotFound
}

// RetireClosedStudyReports ends, as retired, the learner's open
// Unsuitable-context reports in the language whose Undo window has closed
// because another Study action followed. Their sentences stay retired.
func (s *Store) RetireClosedStudyReports(_ context.Context, owner, language string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.studyReports {
		report := &s.studyReports[i]
		if report.OwnerID == owner && report.Open() && report.Kind == domain.StudyReportUnsuitableContext && report.Language == canonicalization.NormalizeLanguage(language) &&
			s.studyActionSince(owner, report.Language, s.studyReportSeq[report.ID]) {
			report.EndedAt, report.EndReason = now.UTC(), domain.StudyReportEndRetired
		}
	}
	return nil
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

// retiredLocked names the sentences the word's retiring reports retired: its
// Unsuitable-context reports not ended as undone.
func (s *Store) retiredLocked(owner string, match func(domain.StudyReport) bool) []domain.StudyOccurrenceRef {
	refs := []domain.StudyOccurrenceRef{}
	for _, report := range s.studyReports {
		if report.OwnerID == owner && report.Retires() && match(report) {
			refs = append(refs, s.sentencesRetiredLocked(owner, report.Source)...)
		}
	}
	return refs
}

// sentencesRetiredLocked names the sentence of a report in every run it is
// known in: the run it was reported in and the runs that carry the reported
// token with the same source span, surface and source lemma. The caller holds
// s.mu.
func (s *Store) sentencesRetiredLocked(owner string, source domain.StudyOccurrenceRef) []domain.StudyOccurrenceRef {
	refs := []domain.StudyOccurrenceRef{source}
	var token *domain.LemmaReviewOccurrence
	for _, occurrences := range s.lemmaOccurrences {
		for i := range occurrences {
			item := &occurrences[i]
			if item.OwnerID == owner && item.AnalysisRunID == source.AnalysisRunID && item.CorpusID == source.CorpusID &&
				item.SentenceOrdinal == source.SentenceOrdinal && item.TokenOrdinal == source.TokenOrdinal {
				token = item
			}
		}
	}
	if token == nil {
		return refs
	}
	for _, occurrences := range s.lemmaOccurrences {
		for _, item := range occurrences {
			if item.OwnerID == owner && item.BookID == token.BookID && item.AnalysisRunID != token.AnalysisRunID &&
				item.SourceDocumentID == token.SourceDocumentID && item.StartOffset == token.StartOffset && item.EndOffset == token.EndOffset &&
				item.Surface == token.Surface && item.RawLemma == token.RawLemma {
				refs = append(refs, domain.StudyOccurrenceRef{
					BookID: item.BookID, AnalysisRunID: item.AnalysisRunID, CorpusID: item.CorpusID,
					SentenceOrdinal: item.SentenceOrdinal, TokenOrdinal: item.TokenOrdinal,
				})
			}
		}
	}
	return refs
}

// hasRetiringReportLocked reports whether the word has an Unsuitable-context
// report that retires a context. The caller holds s.mu.
func (s *Store) hasRetiringReportLocked(owner, progressID string) bool {
	for _, report := range s.studyReports {
		if report.OwnerID == owner && report.ProgressID == progressID && report.Retires() {
			return true
		}
	}
	return false
}

// contextReportedAtLocked is when the word's latest retiring report was made,
// zero when it has none. The caller holds s.mu.
func (s *Store) contextReportedAtLocked(owner, progressID string) time.Time {
	var latest time.Time
	for _, report := range s.studyReports {
		if report.OwnerID == owner && report.ProgressID == progressID && report.Retires() && report.CreatedAt.After(latest) {
			latest = report.CreatedAt
		}
	}
	return latest
}

// withoutRetiredFluencyContextsLocked leaves out the contexts in a sentence the
// learner retired as the word's context. The caller holds s.mu.
func (s *Store) withoutRetiredFluencyContextsLocked(owner string, contexts []domain.FluencyContext) []domain.FluencyContext {
	kept := contexts[:0:0]
	for _, item := range contexts {
		retired := s.retiredLocked(owner, func(r domain.StudyReport) bool { return r.ProgressID == item.ProgressID })
		ref := domain.StudyOccurrenceRef{BookID: item.BookID, AnalysisRunID: item.AnalysisRunID, CorpusID: item.CorpusID, SentenceOrdinal: item.SentenceOrdinal, TokenOrdinal: item.TokenOrdinal}
		if !domain.RetiredStudyOccurrence(retired, ref) {
			kept = append(kept, item)
		}
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

// nextStudyProgressIDLocked returns an ID no word has had, even one since
// removed. The caller holds s.mu.
func (s *Store) nextStudyProgressIDLocked() string {
	s.studyProgressSeq++
	return fmt.Sprintf("fixture-study-progress-%d", s.studyProgressSeq)
}

// removeStudyProgressLocked drops a word's Study progress. The caller holds
// s.mu.
func (s *Store) removeStudyProgressLocked(owner, id string) {
	for i := range s.studyProgress {
		if s.studyProgress[i].OwnerID == owner && s.studyProgress[i].ID == id {
			s.studyProgress = append(s.studyProgress[:i], s.studyProgress[i+1:]...)
			return
		}
	}
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
// gone. A repaired Wrong-word report releases its word, due at the start of the
// next Study day. A report that is missing, foreign or already ended is ErrNotFound.
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
	report := &s.studyReports[index]
	report.EndedAt, report.EndReason = now.UTC(), reason
	if report.Kind != domain.StudyReportWrongWord {
		return nil
	}
	if progress := s.progressByIDLocked(owner, report.ProgressID); progress != nil {
		progress.Memory.Due = domain.NextStudyDayStart(now).UTC()
		progress.UpdatedAt = now.UTC()
	}
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
	progress.LearnedCitations = report.PreviousLearnedCitations.Clone()
	if progress.WithdrawnReason == domain.StudyWithdrawnNoSuitableContext {
		progress.Withdrawn = false
	}
	progress.WithdrawnReason = ""
	progress.UpdatedAt = now.UTC()
	return nil
}

// StudyOccurrenceState reads the occurrence a report names: present while an
// analyzed Book of the language holds it, with the learner's exclusion or
// correction of it.
func (s *Store) StudyOccurrenceState(_ context.Context, owner, language string, source domain.StudyOccurrenceRef) (domain.StudyOccurrenceState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, book := range s.books {
		if book.Source.OwnerID != owner || book.Source.Language != language || book.BookID != source.BookID {
			continue
		}
		for _, occurrence := range s.lemmaOccurrences[lemmaOccurrenceKey(owner, book.BookID)] {
			if occurrence.AnalysisRunID != source.AnalysisRunID || occurrence.SentenceOrdinal != source.SentenceOrdinal || occurrence.TokenOrdinal != source.TokenOrdinal {
				continue
			}
			key := fixtureLemmaCorrectionKey(owner, book.BookID, occurrence.AnalysisRunID, occurrence.StartOffset, occurrence.EndOffset)
			return domain.StudyOccurrenceState{Present: true, Excluded: s.lemmaExclusions[key], Corrected: s.lemmaCorrections[key] != ""}, nil
		}
	}
	return domain.StudyOccurrenceState{}, nil
}

// StudyReportUndoable reports whether the report is an open Unsuitable-context
// report that no other Study action has followed.
func (s *Store) StudyReportUndoable(_ context.Context, owner, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := s.openReportIndexLocked(owner, id)
	if index < 0 || s.studyReports[index].Kind != domain.StudyReportUnsuitableContext {
		return false, nil
	}
	report := s.studyReports[index]
	return !s.studyActionSince(owner, report.Language, s.studyReportSeq[report.ID]), nil
}
