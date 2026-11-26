package fixtures

import (
	"context"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/storecontract"
)

func (h *contractHarness) StudyReports() storecontract.StudyReportStore { return h }

func (h *contractHarness) CreateStudyReport(owner string, record domain.StudyReportRecord) (domain.StudyReport, error) {
	return h.store.CreateStudyReport(context.Background(), owner, record)
}

func (h *contractHarness) GetStudyReport(owner, id string) (domain.StudyReport, error) {
	return h.store.GetStudyReport(context.Background(), owner, id)
}

func (h *contractHarness) GetOpenStudyReport(owner, language, progressID string, kind domain.StudyReportKind) (domain.StudyReport, error) {
	return h.store.GetOpenStudyReport(context.Background(), owner, language, progressID, kind)
}

func (h *contractHarness) ListOpenStudyReports(owner, language string, kind domain.StudyReportKind) ([]domain.StudyReport, error) {
	return h.store.ListOpenStudyReports(context.Background(), owner, language, kind)
}

func (h *contractHarness) ListOpenWrongWordReportsForBook(owner, bookID string) ([]domain.StudyReport, error) {
	return h.store.ListOpenWrongWordReportsForBook(context.Background(), owner, bookID)
}

func (h *contractHarness) ListRetiredStudyOccurrences(owner, progressID string) ([]domain.StudyOccurrenceRef, error) {
	return h.store.ListRetiredStudyOccurrences(context.Background(), owner, progressID)
}

func (h *contractHarness) ListRetiredStudyOccurrencesOfWord(owner string, identity domain.StudyIdentity) ([]domain.StudyOccurrenceRef, error) {
	return h.store.ListRetiredStudyOccurrencesOfWord(context.Background(), owner, identity)
}

func (h *contractHarness) EndStudyReport(owner, id string, reason domain.StudyReportEnd, now time.Time) error {
	return h.store.EndStudyReport(context.Background(), owner, id, reason, now)
}

func (h *contractHarness) DismissStudyReport(owner, id string, now time.Time) error {
	return h.store.DismissStudyReport(context.Background(), owner, id, now)
}

func (h *contractHarness) UndoStudyReport(owner, id string, now time.Time) error {
	return h.store.UndoStudyReport(context.Background(), owner, id, now)
}
