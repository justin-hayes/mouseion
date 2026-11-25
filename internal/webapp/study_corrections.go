package webapp

import (
	"context"
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
// progress or material changed.
func (h *Handler) reviewStudyWord(ctx context.Context, owner string, word domain.StudyProgress, now time.Time) (bool, error) {
	store := h.services.Store.Study
	material, err := store.GetStudyMaterial(ctx, owner, word.MaterialID)
	if err != nil {
		return false, err
	}
	valid, err := store.ListStudyOccurrences(ctx, owner, word.StudyIdentity)
	if err != nil {
		return false, err
	}
	evidence := domain.StudyEvidence{Withdrawn: word.Withdrawn, Source: material.Source(), Valid: valid}
	if word.Withdrawn {
		current, err := store.ListCurrentStudyMaterial(ctx, owner, word.Language)
		if err != nil {
			return false, err
		}
		for _, item := range current {
			evidence.CurrentUnpreparable = evidence.CurrentUnpreparable || (item.StudyIdentity == word.StudyIdentity && item.State == domain.StudyMaterialUnpreparable)
		}
	}
	switch domain.ReviewStudyEvidence(evidence) {
	case domain.StudyEvidenceWithdraw:
		return true, store.SetStudyWordWithdrawn(ctx, owner, word.ID, true, now)
	case domain.StudyEvidenceReintroduce:
		return true, store.SupersedeStudyMaterial(ctx, owner, word.MaterialID)
	case domain.StudyEvidenceReturn:
		return true, store.SetStudyWordWithdrawn(ctx, owner, word.ID, false, now)
	case domain.StudyEvidenceReturnReintroduce:
		if err := store.SetStudyWordWithdrawn(ctx, owner, word.ID, false, now); err != nil {
			return false, err
		}
		return true, store.SupersedeStudyMaterial(ctx, owner, word.MaterialID)
	case domain.StudyEvidenceNone:
	}
	return false, nil
}
