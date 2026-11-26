package webapp

import (
	"context"
	"errors"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// reconcileStudyCorrections applies the learner's corrections to the words in
// study (ADR 0134 §6). A word is checked when it is next due: if its
// introducing occurrence no longer resolves to its identity under current
// evidence, its Study material is replaced and prepared again, and the word is
// re-introduced there with its FSRS state kept; if no valid occurrence of its
// identity remains, the word is withdrawn with its progress kept. A withdrawn
// word returns when evidence reappears. The identity a correction points to is
// never put into study: this only acts on words already in it.
func (h *Handler) reconcileStudyCorrections(ctx context.Context, owner, language string, now time.Time) error {
	store := h.services.Store.Study
	if err := store.CompleteStudyReintroductions(ctx, owner, language, now); err != nil {
		return err
	}
	progress, err := store.ListStudyProgress(ctx, owner, language)
	if err != nil {
		return err
	}
	// Withdrawn words return as soon as their evidence does.
	checked, changed, err := h.reviewWithdrawnWords(ctx, owner, progress, now)
	if err != nil {
		return err
	}
	// The word about to be shown is verified first. One that changed leaves the
	// next in line to be checked, so every card shown has a valid sentence.
	for {
		if changed {
			if progress, err = store.ListStudyProgress(ctx, owner, language); err != nil {
				return err
			}
		}
		next, ok := domain.NextStudyReview(progress, now)
		if !ok || checked[next.ID] {
			return nil
		}
		checked[next.ID] = true
		if changed, err = h.reviewStudyWord(ctx, owner, next, now); err != nil || !changed {
			return err
		}
	}
}

// reviewWithdrawnWords checks every withdrawn word against current evidence. It
// returns the words checked and whether any of them changed.
func (h *Handler) reviewWithdrawnWords(ctx context.Context, owner string, progress []domain.StudyProgress, now time.Time) (checked map[string]bool, changed bool, err error) {
	checked = map[string]bool{}
	for _, word := range progress {
		if !word.Withdrawn {
			continue
		}
		checked[word.ID] = true
		moved, err := h.reviewStudyWord(ctx, owner, word, now)
		if err != nil {
			return nil, false, err
		}
		changed = changed || moved
	}
	return checked, changed, nil
}

// reviewStudyWord checks one word's introducing occurrence against current
// evidence and applies what that requires. It reports whether the word's
// progress or material changed. An occurrence an Analysis update re-published
// unchanged keeps its material, which then names the occurrence in the new run.
func (h *Handler) reviewStudyWord(ctx context.Context, owner string, word domain.StudyProgress, now time.Time) (bool, error) {
	store := h.services.Store.Study
	evidence, current, err := h.studyEvidence(ctx, owner, word)
	if err != nil {
		return false, err
	}
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

// studyEvidence gathers what ReviewStudyEvidence decides from: the occurrence
// the word was introduced in, with its locator, the occurrences that resolve to
// its identity now, and what a failed attempt to prepare it saw. current is the
// word's current Study material, empty when it has none.
func (h *Handler) studyEvidence(ctx context.Context, owner string, word domain.StudyProgress) (domain.StudyEvidence, domain.StudyMaterial, error) {
	store := h.services.Store.Study
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
