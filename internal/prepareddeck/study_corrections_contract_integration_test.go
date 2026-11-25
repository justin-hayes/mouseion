//go:build integration

package prepareddeck

import (
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/storecontract"
)

func (h *postgresHarness) StudyCorrections() storecontract.StudyCorrectionsStore { return h }

func (h *postgresHarness) ListStudyOccurrences(owner string, identity domain.StudyIdentity) ([]domain.StudyOccurrenceRef, error) {
	return h.store.ListStudyOccurrences(h.ctx, owner, identity)
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
