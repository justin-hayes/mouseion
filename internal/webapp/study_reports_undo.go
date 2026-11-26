package webapp

import (
	"errors"
	"net/http"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// undoStudyReport undoes the learner's Unsuitable-context report while its card
// is shown (ADR 0135 §3). The store refuses once another Study action followed;
// like a stale grade, that and a report already ended change nothing and the
// learner is sent back to Study. An undone report returns the word's card.
func (h *Handler) undoStudyReport(w http.ResponseWriter, r *http.Request) {
	language, back, ok := h.studyForm(w, r)
	if !ok {
		return
	}
	owner, store, id := user(r).ID, h.services.Store.Study, r.PathValue("id")
	report, err := store.GetStudyReport(r.Context(), owner, id)
	if errors.Is(err, domain.ErrNotFound) || (err == nil && report.Language != language) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if err := store.UndoStudyReport(r.Context(), owner, id, h.now()); err != nil && !errors.Is(err, domain.ErrStudyStale) && !errors.Is(err, domain.ErrNotFound) {
		fail(w, err)
		return
	}
	redirect(w, r, back)
}
