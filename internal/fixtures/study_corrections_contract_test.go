package fixtures

import (
	"context"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/storecontract"
)

func (h *contractHarness) StudyCorrections() storecontract.StudyCorrectionsStore { return h }

func (h *contractHarness) ListStudyOccurrences(owner string, identity domain.StudyIdentity) ([]domain.StudyOccurrenceRef, error) {
	return h.store.ListStudyOccurrences(context.Background(), owner, identity)
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
