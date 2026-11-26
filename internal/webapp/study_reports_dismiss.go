package webapp

import (
	"errors"
	"net/http"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// ownedStudyReport reads the report named by the request's path for a Study
// form. A report that is missing, of another learner or of another language is
// not found; the response is written and false returned.
func (h *Handler) ownedStudyReport(w http.ResponseWriter, r *http.Request, language string) (domain.StudyReport, bool) {
	report, err := h.services.Store.Study.GetStudyReport(r.Context(), user(r).ID, r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) || (err == nil && report.Language != language) {
		http.NotFound(w, r)
		return domain.StudyReport{}, false
	}
	if err != nil {
		fail(w, err)
		return domain.StudyReport{}, false
	}
	return report, true
}

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
	report, ok := h.ownedStudyReport(w, r, language)
	if !ok {
		return
	}
	// Already ended (a second tap, or repaired meanwhile) leaves the list current.
	if err := h.services.Store.Study.DismissStudyReport(r.Context(), user(r).ID, report.ID, h.now()); err != nil && !errors.Is(err, domain.ErrNotFound) {
		fail(w, err)
		return
	}
	redirect(w, r, studyLanguagePath(pageKindStudyReports, language))
}
