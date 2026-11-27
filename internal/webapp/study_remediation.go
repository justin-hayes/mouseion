package webapp

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
)

// studyRemediationView is the remediation screen of one open Wrong-word report:
// the reported occurrence, the decisions open on it and, once one is previewed,
// its consequence (ADR 0136 §6).
type studyRemediationView struct {
	Report     domain.StudyReport
	Language   string
	BookTitle  string
	Path       string
	Occurrence domain.LemmaReviewOccurrence
	// Found is false when the report's occurrence is no longer in the Book's
	// analysis; the learner can only dismiss the report then.
	Found      bool
	CanCorrect bool
	Preview    *lemmareview.Preview
	Recovery   lemmareview.Recovery
}

// studyRemediationPath is the remediation screen of a report.
func studyRemediationPath(reportID string) string {
	return "/study/reports/" + url.PathEscape(reportID) + "/remediation"
}

// openWrongWordReport reads the learner's open Wrong-word report the path
// names. Any other report is not found; the response is written and false
// returned.
func (h *Handler) openWrongWordReport(w http.ResponseWriter, r *http.Request) (domain.StudyReport, bool) {
	report, err := h.services.Store.Study.GetStudyReport(r.Context(), user(r).ID, r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) || (err == nil && (!report.Open() || report.Kind != domain.StudyReportWrongWord)) {
		http.NotFound(w, r)
		return domain.StudyReport{}, false
	}
	if err != nil {
		fail(w, err)
		return domain.StudyReport{}, false
	}
	return report, true
}

// reportedOccurrence finds the occurrence a report names among the Book's
// occurrences of its word, by its place in the Book, as Lemma review does.
func (h *Handler) reportedOccurrence(r *http.Request, owner string, report domain.StudyReport) (lemmareview.Review, domain.LemmaReviewOccurrence, bool, error) {
	owned := report
	if located, err := h.services.Store.Study.StudyOccurrenceLocator(r.Context(), owner, report.Source); err == nil {
		owned.Source = located
	}
	review, err := h.services.LemmaReview.Review(r.Context(), owner, report.Source.BookID, report.TargetWord)
	if err != nil {
		return review, domain.LemmaReviewOccurrence{}, false, err
	}
	for _, occurrence := range review.Occurrences {
		if lemmaReviewReported(owned, occurrence) {
			return review, occurrence, true, nil
		}
	}
	return review, domain.LemmaReviewOccurrence{}, false, nil
}

// studyRemediation serves the remediation screen: the reported occurrence with
// the Lemma correction and the Occurrence exclusion open on it.
func (h *Handler) studyRemediation(w http.ResponseWriter, r *http.Request) {
	report, ok := h.openWrongWordReport(w, r)
	if !ok {
		return
	}
	view, err := h.studyRemediationView(r, report)
	if err != nil {
		writeLemmaDecisionError(w, r, err)
		return
	}
	render(w, r, StudyRemediationPage(user(r), h.csrf(w, r), view))
}

func (h *Handler) studyRemediationView(r *http.Request, report domain.StudyReport) (studyRemediationView, error) {
	owner := user(r).ID
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner, report.Source.BookID)
	if err != nil {
		return studyRemediationView{}, err
	}
	adoptDetailLanguage(r.Context(), detail)
	review, occurrence, found, err := h.reportedOccurrence(r, owner, report)
	if err != nil {
		return studyRemediationView{}, err
	}
	return studyRemediationView{
		Report: report, Language: report.Language, BookTitle: detail.Book.Title, Path: studyRemediationPath(report.ID),
		Occurrence: occurrence, Found: found, CanCorrect: review.CanCorrect, Recovery: review.Recovery,
	}, nil
}

// decideStudyRemediation takes the preview and the confirmation of a decision
// on the reported occurrence. Both run Lemma review's own steps, on that one
// occurrence: any other occurrence the form names is ignored, so a report
// repairs only its own occurrence (ADR 0136 §6).
func (h *Handler) decideStudyRemediation(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	report, ok := h.openWrongWordReport(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid review form.", http.StatusBadRequest)
		return
	}
	owner, bookID := user(r).ID, report.Source.BookID
	_, occurrence, found, err := h.reportedOccurrence(r, owner, report)
	if err != nil {
		writeLemmaDecisionError(w, r, err)
		return
	}
	if !found {
		http.Error(w, "The reported occurrence is no longer in this Book's analysis.", http.StatusConflict)
		return
	}
	// The decision is of the report's word, whatever form the request names.
	r.Form.Set("form", report.TargetWord)
	only := []string{occurrence.ID()}
	switch r.FormValue("stage") {
	case "preview":
		h.previewStudyRemediation(w, r, report, only)
	case "confirm":
		outcome, _, err := h.confirmLemmaDecisionStep(r, owner, bookID, only)
		if err != nil {
			writeLemmaDecisionError(w, r, err)
			return
		}
		respondLemmaOutcome(w, r, outcome, studyLanguagePath(pageKindStudyReports, report.Language))
	default:
		http.Error(w, "Review the proposed consequence before confirming a decision.", http.StatusBadRequest)
	}
}

func (h *Handler) previewStudyRemediation(w http.ResponseWriter, r *http.Request, report domain.StudyReport, only []string) {
	step, err := h.previewLemmaDecisionStep(r, user(r).ID, report.Source.BookID, only)
	if err != nil {
		writeLemmaDecisionError(w, r, err)
		return
	}
	view, err := h.studyRemediationView(r, report)
	if err != nil {
		writeLemmaDecisionError(w, r, err)
		return
	}
	view.Preview, view.Recovery = &step.Preview, step.Review.Recovery
	render(w, r, StudyRemediationPage(user(r), h.csrf(w, r), view))
}
