package webapp

import (
	"errors"
	"net/http"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// dismissStudyReport ends a Wrong-word report the learner no longer wants held.
// The word's due time moves later by the time it was held, so nothing becomes
// overdue; no grade or review-log row is written and the material and Learned
// meaning stay as they are (ADR 0135 §2). A report that is gone, ended, of
// another learner or of another language is not found.
func (h *Handler) dismissStudyReport(w http.ResponseWriter, r *http.Request) {
	language, _, ok := h.studyForm(w, r)
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
	if err := store.DismissStudyReport(r.Context(), owner, id, h.now()); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Already ended (a second tap, or repaired meanwhile): the list is current.
			redirect(w, r, studyLanguagePath(pageKindStudyReports, language))
			return
		}
		fail(w, err)
		return
	}
	redirect(w, r, studyLanguagePath(pageKindStudyReports, language))
}
