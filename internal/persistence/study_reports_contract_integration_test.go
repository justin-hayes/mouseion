//go:build integration

package persistence_test

import (
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/storecontract"
)

func (h *postgresHarness) StudyReports() storecontract.StudyReportStore { return h }

func (h *postgresHarness) CreateStudyReport(owner string, record domain.StudyReportRecord) (domain.StudyReport, error) {
	return h.store.CreateStudyReport(h.ctx, owner, record)
}

func (h *postgresHarness) GetStudyReport(owner, id string) (domain.StudyReport, error) {
	return h.store.GetStudyReport(h.ctx, owner, id)
}

func (h *postgresHarness) GetOpenStudyReport(owner, language, progressID string, kind domain.StudyReportKind) (domain.StudyReport, error) {
	return h.store.GetOpenStudyReport(h.ctx, owner, language, progressID, kind)
}

func (h *postgresHarness) ListOpenStudyReports(owner, language string, kind domain.StudyReportKind) ([]domain.StudyReport, error) {
	return h.store.ListOpenStudyReports(h.ctx, owner, language, kind)
}

func (h *postgresHarness) ListOpenWrongWordReportsForBook(owner, bookID string) ([]domain.StudyReport, error) {
	return h.store.ListOpenWrongWordReportsForBook(h.ctx, owner, bookID)
}

func (h *postgresHarness) ListRetiredStudyOccurrences(owner, progressID string) ([]domain.StudyOccurrenceRef, error) {
	return h.store.ListRetiredStudyOccurrences(h.ctx, owner, progressID)
}

func (h *postgresHarness) ListRetiredStudyOccurrencesOfWord(owner string, identity domain.StudyItem) ([]domain.StudyOccurrenceRef, error) {
	return h.store.ListRetiredStudyOccurrencesOfWord(h.ctx, owner, identity)
}

func (h *postgresHarness) StudyOccurrenceState(owner, language string, source domain.StudyOccurrenceRef) (domain.StudyOccurrenceState, error) {
	return h.store.StudyOccurrenceState(h.ctx, owner, language, source)
}

func (h *postgresHarness) EndStudyReport(owner, id string, reason domain.StudyReportEnd, now time.Time) error {
	return h.store.EndStudyReport(h.ctx, owner, id, reason, now)
}

func (h *postgresHarness) DismissStudyReport(owner, id string, now time.Time) error {
	return h.store.DismissStudyReport(h.ctx, owner, id, now)
}

func (h *postgresHarness) UndoStudyReport(owner, id string, now time.Time) error {
	return h.store.UndoStudyReport(h.ctx, owner, id, now)
}

func (h *postgresHarness) GetOpenStudyReportOfMaterial(owner, materialID string, kind domain.StudyReportKind) (domain.StudyReport, error) {
	return h.store.GetOpenStudyReportOfMaterial(h.ctx, owner, materialID, kind)
}

func (h *postgresHarness) RetireClosedStudyReports(owner, language string, now time.Time) error {
	return h.store.RetireClosedStudyReports(h.ctx, owner, language, now)
}
