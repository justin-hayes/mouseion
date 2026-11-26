package webapp

import (
	"errors"
	"net/http"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// undoStudyReport undoes the learner's Unsuitable-context report while its card
// is shown (ADR 0135 §3). The store refuses once another Study action followed
// or the learner moved on; like a stale grade, that and a report already ended
// change nothing and the learner is sent back to Study. An undone report returns
// the word's card, with its answer side shown as it was reported.
func (h *Handler) undoStudyReport(w http.ResponseWriter, r *http.Request) {
	language, back, ok := h.studyForm(w, r)
	if !ok {
		return
	}
	report, ok := h.ownedStudyReport(w, r, language)
	if !ok {
		return
	}
	err := h.services.Store.Study.UndoStudyReport(r.Context(), user(r).ID, report.ID, h.now())
	switch {
	case err == nil:
		redirect(w, r, back+"?answer=1")
	case errors.Is(err, domain.ErrStudyStale) || errors.Is(err, domain.ErrNotFound):
		redirect(w, r, back)
	default:
		fail(w, err)
	}
}

// nextAfterStudyReport moves the learner on from the reported card: the Undo
// offer ends and the report retires its sentence for good (ADR 0135 §3).
func (h *Handler) nextAfterStudyReport(w http.ResponseWriter, r *http.Request) {
	language, back, ok := h.studyForm(w, r)
	if !ok {
		return
	}
	report, ok := h.ownedStudyReport(w, r, language)
	if !ok {
		return
	}
	if err := h.services.Store.Study.EndStudyReport(r.Context(), user(r).ID, report.ID, domain.StudyReportEndRetired, h.now()); err != nil && !errors.Is(err, domain.ErrNotFound) {
		fail(w, err)
		return
	}
	redirect(w, r, back)
}
