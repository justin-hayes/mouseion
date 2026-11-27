//go:build integration

package persistence_test

import (
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/storecontract"
)

func (h *postgresHarness) StudyCorrections() storecontract.StudyCorrectionsStore { return h }

func (h *postgresHarness) ListStudyOccurrences(owner string, identity domain.StudyItem) ([]domain.StudyOccurrenceRef, error) {
	return h.store.ListStudyOccurrences(h.ctx, owner, identity)
}

func (h *postgresHarness) FindStudyOccurrences(owner string, identity domain.StudyItem, source domain.StudyOccurrenceRef) ([]domain.StudyOccurrenceRef, error) {
	return h.store.FindStudyOccurrences(h.ctx, owner, identity, source)
}

func (h *postgresHarness) SupersedeStudyMaterial(owner, materialID string) error {
	return h.store.SupersedeStudyMaterial(h.ctx, owner, materialID)
}

func (h *postgresHarness) SetStudyWordWithdrawn(owner, progressID string, withdrawn bool, now time.Time) error {
	return h.store.SetStudyWordWithdrawn(h.ctx, owner, progressID, withdrawn, now)
}

func (h *postgresHarness) CompleteStudyReintroductions(owner, language string, now time.Time) error {
	return h.store.CompleteStudyReintroductions(h.ctx, owner, language, now)
}

func (h *postgresHarness) StudyOccurrenceLocator(owner string, ref domain.StudyOccurrenceRef) (domain.StudyOccurrenceRef, error) {
	return h.store.StudyOccurrenceLocator(h.ctx, owner, ref)
}

func (h *postgresHarness) RelocateStudyMaterial(owner, materialID string, ref domain.StudyOccurrenceRef) error {
	return h.store.RelocateStudyMaterial(h.ctx, owner, materialID, ref)
}

func (h *postgresHarness) SetStudyMaterialEvidence(owner, materialID, evidence string) error {
	return h.store.SetStudyMaterialEvidence(h.ctx, owner, materialID, evidence)
}

func (h *postgresHarness) RecordStudyMaterialNoDictionaryEntry(owner, materialID, pin string) error {
	return h.store.RecordStudyMaterialNoDictionaryEntry(h.ctx, owner, materialID, pin)
}

func (h *postgresHarness) ReopenStudyMaterialNoDictionaryEntry(owner, materialID, pin string) error {
	return h.store.ReopenStudyMaterialNoDictionaryEntry(h.ctx, owner, materialID, pin)
}
