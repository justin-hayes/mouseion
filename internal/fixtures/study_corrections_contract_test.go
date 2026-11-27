package fixtures

import (
	"context"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/storecontract"
)

func (h *contractHarness) StudyCorrections() storecontract.StudyCorrectionsStore { return h }

func (h *contractHarness) ListStudyOccurrences(owner string, identity domain.StudyItem) ([]domain.StudyOccurrenceRef, error) {
	return h.store.ListStudyOccurrences(context.Background(), owner, identity)
}

func (h *contractHarness) FindStudyOccurrences(owner string, identity domain.StudyItem, source domain.StudyOccurrenceRef) ([]domain.StudyOccurrenceRef, error) {
	return h.store.FindStudyOccurrences(context.Background(), owner, identity, source)
}

func (h *contractHarness) SupersedeStudyMaterial(owner, materialID string) error {
	return h.store.SupersedeStudyMaterial(context.Background(), owner, materialID)
}

func (h *contractHarness) SetStudyWordWithdrawn(owner, progressID string, withdrawn bool, now time.Time) error {
	return h.store.SetStudyWordWithdrawn(context.Background(), owner, progressID, withdrawn, now)
}

func (h *contractHarness) CompleteStudyReintroductions(owner, language string, now time.Time) error {
	return h.store.CompleteStudyReintroductions(context.Background(), owner, language, now)
}

func (h *contractHarness) StudyOccurrenceLocator(owner string, ref domain.StudyOccurrenceRef) (domain.StudyOccurrenceRef, error) {
	return h.store.StudyOccurrenceLocator(context.Background(), owner, ref)
}

func (h *contractHarness) RelocateStudyMaterial(owner, materialID string, ref domain.StudyOccurrenceRef) error {
	return h.store.RelocateStudyMaterial(context.Background(), owner, materialID, ref)
}

func (h *contractHarness) SetStudyMaterialEvidence(owner, materialID, evidence string) error {
	return h.store.SetStudyMaterialEvidence(context.Background(), owner, materialID, evidence)
}

func (h *contractHarness) RecordStudyMaterialNoDictionaryEntry(owner, materialID, pin string) error {
	return h.store.RecordStudyMaterialNoDictionaryEntry(context.Background(), owner, materialID, pin)
}

func (h *contractHarness) ReopenStudyMaterialNoDictionaryEntry(owner, materialID, pin string) error {
	return h.store.ReopenStudyMaterialNoDictionaryEntry(context.Background(), owner, materialID, pin)
}
