package study

import (
	"context"
	"errors"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// CorrectionStore is what checking a word against current evidence reads and
// writes (ADR 0134 §6). Both the PostgreSQL store and the fixture store satisfy
// it.
type CorrectionStore interface {
	GetStudyMaterial(ctx context.Context, owner, id string) (domain.StudyMaterial, error)
	ListCurrentStudyMaterial(ctx context.Context, owner, language string) ([]domain.StudyMaterial, error)
	StudyOccurrenceLocator(ctx context.Context, owner string, ref domain.StudyOccurrenceRef) (domain.StudyOccurrenceRef, error)
	FindStudyOccurrences(ctx context.Context, owner string, identity domain.StudyIdentity, source domain.StudyOccurrenceRef) ([]domain.StudyOccurrenceRef, error)
	ListStudyOccurrences(ctx context.Context, owner string, identity domain.StudyIdentity) ([]domain.StudyOccurrenceRef, error)
	SupersedeStudyMaterial(ctx context.Context, owner, materialID string) error
	SetStudyWordWithdrawn(ctx context.Context, owner, progressID string, withdrawn bool, now time.Time) error
	RelocateStudyMaterial(ctx context.Context, owner, materialID string, ref domain.StudyOccurrenceRef) error
}

// ReportStore adds what repairing a Wrong-word report reads and writes.
type ReportStore interface {
	CorrectionStore
	StudyOccurrenceState(ctx context.Context, owner, language string, source domain.StudyOccurrenceRef) (domain.StudyOccurrenceState, error)
	ListOpenStudyReports(ctx context.Context, owner, language string, kind domain.StudyReportKind) ([]domain.StudyReport, error)
	ListOpenWrongWordReportsForBook(ctx context.Context, owner, bookID string) ([]domain.StudyReport, error)
	EndStudyReport(ctx context.Context, owner, id string, reason domain.StudyReportEnd, now time.Time) error
}

// ReviewWord checks one word's introducing occurrence against current evidence
// and applies what that requires. It reports whether the word's progress or
// material changed. An occurrence an Analysis update re-published unchanged
// keeps its material, which then names the occurrence in the new run.
func ReviewWord(ctx context.Context, store CorrectionStore, owner string, word domain.StudyProgress, now time.Time) (bool, error) {
	evidence, current, err := gatherEvidence(ctx, store, owner, word)
	if err != nil {
		return false, err
	}
	return applyEvidence(ctx, store, owner, word, evidence, current, now)
}

func applyEvidence(ctx context.Context, store CorrectionStore, owner string, word domain.StudyProgress, evidence domain.StudyEvidence, current domain.StudyMaterial, now time.Time) (bool, error) {
	relocated, moved := domain.StudyRelocation(evidence)
	switch domain.ReviewStudyEvidence(evidence) {
	case domain.StudyEvidenceWithdraw:
		return true, store.SetStudyWordWithdrawn(ctx, owner, word.ID, true, now)
	case domain.StudyEvidenceReintroduce:
		return true, store.SupersedeStudyMaterial(ctx, owner, word.MaterialID)
	case domain.StudyEvidenceReturn:
		if err := store.SetStudyWordWithdrawn(ctx, owner, word.ID, false, now); err != nil {
			return false, err
		}
	case domain.StudyEvidenceReturnReintroduce:
		if err := store.SetStudyWordWithdrawn(ctx, owner, word.ID, false, now); err != nil {
			return false, err
		}
		// The word's current material is the unpreparable record, or the
		// material it was withdrawn with.
		if current.ID == "" {
			return true, nil
		}
		return true, store.SupersedeStudyMaterial(ctx, owner, current.ID)
	case domain.StudyEvidenceNone:
	}
	if moved {
		return false, store.RelocateStudyMaterial(ctx, owner, word.MaterialID, relocated)
	}
	return false, nil
}

// gatherEvidence collects what ReviewStudyEvidence decides from: the occurrence
// the word was introduced in, with its locator, the occurrences that resolve to
// its identity now, and what a failed attempt to prepare it saw. current is the
// word's current Study material, empty when it has none.
func gatherEvidence(ctx context.Context, store CorrectionStore, owner string, word domain.StudyProgress) (domain.StudyEvidence, domain.StudyMaterial, error) {
	material, err := store.GetStudyMaterial(ctx, owner, word.MaterialID)
	if err != nil {
		return domain.StudyEvidence{}, domain.StudyMaterial{}, err
	}
	source := material.Source()
	switch located, err := store.StudyOccurrenceLocator(ctx, owner, source); {
	case err == nil:
		source = located
	case !errors.Is(err, domain.ErrNotFound):
		return domain.StudyEvidence{}, domain.StudyMaterial{}, err
	}
	evidence := domain.StudyEvidence{Withdrawn: word.Withdrawn, Source: source}
	current := material
	if word.Withdrawn {
		all, err := store.ListCurrentStudyMaterial(ctx, owner, word.Language)
		if err != nil {
			return domain.StudyEvidence{}, domain.StudyMaterial{}, err
		}
		current = domain.StudyMaterial{}
		for _, item := range all {
			if item.StudyIdentity == word.StudyIdentity {
				evidence.CurrentUnpreparable = item.State == domain.StudyMaterialUnpreparable
				evidence.FailedEvidence = item.EvidenceFingerprint
				current = item
				break
			}
		}
	}
	// The decision needs the introducing occurrence and, when it no longer
	// resolves, one other valid occurrence, so the store never lists them all.
	// Only a word withdrawn after unpreparable material compares the whole
	// evidence, against what the failed attempt saw.
	if evidence.CurrentUnpreparable {
		evidence.Valid, err = store.ListStudyOccurrences(ctx, owner, word.StudyIdentity)
	} else {
		evidence.Valid, err = store.FindStudyOccurrences(ctx, owner, word.StudyIdentity, source)
	}
	if err != nil {
		return domain.StudyEvidence{}, domain.StudyMaterial{}, err
	}
	return evidence, current, nil
}

// ReconcileReports repairs the learner's open Wrong-word reports of a language
// that current evidence has settled (ADR 0135 §2). A held word is never due, so
// Today's check of the next due word never reaches it; this checks each held
// word directly, from the report alone. The cost is bounded by the open reports:
// a few token lookups each, never a scan of the candidates or of the learner's
// Study progress. A report whose occurrence still resolves to its identity stays
// open, including after an Analysis update that kept it.
func ReconcileReports(ctx context.Context, store ReportStore, owner, language string, now time.Time) error {
	reports, err := store.ListOpenStudyReports(ctx, owner, language, domain.StudyReportWrongWord)
	if err != nil {
		return err
	}
	return reconcileReports(ctx, store, owner, reports, now)
}

// ReconcileBookReports does the same for the open Wrong-word reports of one
// Book, after a Lemma decision was confirmed in it. Each report ends for what
// the evidence says of its own occurrence, so a decision about another
// occurrence ends nothing.
func ReconcileBookReports(ctx context.Context, store ReportStore, owner, bookID string, now time.Time) error {
	reports, err := store.ListOpenWrongWordReportsForBook(ctx, owner, bookID)
	if err != nil {
		return err
	}
	return reconcileReports(ctx, store, owner, reports, now)
}

func reconcileReports(ctx context.Context, store ReportStore, owner string, reports []domain.StudyReport, now time.Time) error {
	for _, report := range reports {
		if err := reconcileReport(ctx, store, owner, report, now); err != nil {
			return err
		}
	}
	return nil
}

// reconcileReport checks one report's occurrence. The word is the report's: its
// identity and the material reported. A reported new word may have no Study
// progress; its report still ends, and there is nothing to re-introduce.
func reconcileReport(ctx context.Context, store ReportStore, owner string, report domain.StudyReport, now time.Time) error {
	word := domain.StudyProgress{ID: report.ProgressID, StudyIdentity: report.StudyIdentity, MaterialID: report.MaterialID}
	evidence, current, err := gatherEvidence(ctx, store, owner, word)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	state, err := store.StudyOccurrenceState(ctx, owner, report.Language, evidence.Source)
	if err != nil {
		return err
	}
	action := domain.ReviewStudyEvidence(evidence)
	if state.Present && action == domain.StudyEvidenceNone {
		// The occurrence still resolves to the word's identity; an Analysis
		// update that kept it only moves the material to the new run.
		return ignoreMissing(applyOnly(ctx, store, owner, word, evidence, current, now))
	}
	// The report ends first: the word is due again at the start of the next
	// Study day, and is then re-introduced, or withdrawn when no valid
	// occurrence is left, with its progress kept.
	if err := store.EndStudyReport(ctx, owner, report.ID, state.EndReason(), now); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
	if word.ID == "" {
		return nil
	}
	if action == domain.StudyEvidenceNone {
		// The occurrence left the library but its identity resolves elsewhere.
		if len(evidence.Valid) == 0 {
			return ignoreMissing(store.SetStudyWordWithdrawn(ctx, owner, word.ID, true, now))
		}
		return ignoreMissing(store.SupersedeStudyMaterial(ctx, owner, word.MaterialID))
	}
	return ignoreMissing(applyOnly(ctx, store, owner, word, evidence, current, now))
}

// ignoreMissing drops the not-found of a word or material another request
// already changed.
func ignoreMissing(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	return err
}

func applyOnly(ctx context.Context, store CorrectionStore, owner string, word domain.StudyProgress, evidence domain.StudyEvidence, current domain.StudyMaterial, now time.Time) error {
	_, err := applyEvidence(ctx, store, owner, word, evidence, current, now)
	return err
}
