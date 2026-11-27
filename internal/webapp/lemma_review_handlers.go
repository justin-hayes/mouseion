package webapp

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
	"github.com/justin-hayes/mouseion/internal/study"
)

// lemmaReviewSuggestion is the optional LLM suggestion shown beside the
// occurrence it was requested for. It is advisory and never applied.
type lemmaReviewSuggestion struct {
	Target                            string
	Lemma, Provider, Version, Message string
}

// lemmaReviewBook loads the Book a lemma-review action names. It writes the
// response and reports false when the request cannot continue.
func (h *Handler) lemmaReviewBook(w http.ResponseWriter, r *http.Request, owner domain.User, bookID string) (domain.MyBook, bool) {
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
	if errors.Is(err, domain.ErrNotFound) {
		http.NotFound(w, r)
		return domain.MyBook{}, false
	}
	if err != nil {
		fail(w, err)
		return domain.MyBook{}, false
	}
	return detail, true
}

func (h *Handler) lemmaReview(w http.ResponseWriter, r *http.Request) {
	owner := user(r)
	bookID := strings.TrimSpace(r.PathValue("bookID"))
	detail, ok := h.lemmaReviewBook(w, r, owner, bookID)
	if !ok {
		return
	}
	assessment, err := h.services.LemmaReview.Assess(r.Context(), owner.ID, bookID)
	if err != nil {
		fail(w, err)
		return
	}
	adoptDetailLanguage(r.Context(), detail)
	form := strings.TrimSpace(r.URL.Query().Get("form"))
	review, err := h.services.LemmaReview.Review(r.Context(), owner.ID, bookID, form)
	if err != nil {
		fail(w, err)
		return
	}
	review.Recovery.ReferenceAssessed = assessment.Assessed
	review.Occurrences = h.onlyReportedOccurrences(r, owner.ID, bookID, review.Occurrences)
	render(w, r, LemmaReviewPage(owner, h.csrf(w, r), bookID, detail.Book.Title, form, lemmaReviewPageError(review.CanCorrect), review.CanCorrect, review.Occurrences, nil, review.Recovery, nil, h.lemmaReviewReports(r, owner.ID, bookID)))
}

func (h *Handler) suggestLemma(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r)
	bookID := strings.TrimSpace(r.PathValue("bookID"))
	form := strings.TrimSpace(r.FormValue("form"))
	occurrenceID := strings.TrimSpace(r.FormValue("target"))
	if occurrenceID == "" || form == "" {
		http.Error(w, "Choose an occurrence to request a suggestion.", http.StatusBadRequest)
		return
	}
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
	if err != nil {
		fail(w, err)
		return
	}
	adoptDetailLanguage(r.Context(), detail)
	suggestion, err := h.services.LemmaReview.Suggest(r.Context(), owner.ID, bookID, occurrenceID)
	if err != nil {
		writeLemmaReviewError(w, err)
		return
	}
	review, err := h.services.LemmaReview.Review(r.Context(), owner.ID, bookID, form)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, LemmaReviewPage(owner, h.csrf(w, r), bookID, detail.Book.Title, form, lemmaReviewPageError(review.CanCorrect), review.CanCorrect, review.Occurrences, nil, review.Recovery, lemmaSuggestionView(occurrenceID, suggestion), h.lemmaReviewReports(r, owner.ID, bookID)))
}

func lemmaSuggestionView(occurrenceID string, suggestion lemmareview.Suggestion) *lemmaReviewSuggestion {
	view := &lemmaReviewSuggestion{Target: occurrenceID}
	switch suggestion.Status {
	case lemmareview.SuggestionNotConfigured:
		view.Message = "LLM suggestions are not configured. You can still keep, correct, or exclude this occurrence manually."
	case lemmareview.SuggestionUnavailable:
		view.Message = "A lemma suggestion is unavailable right now. You can still keep, correct, or exclude this occurrence manually."
	case lemmareview.SuggestionOffered:
		view.Lemma, view.Provider, view.Version = suggestion.Lemma, suggestion.Provider, suggestion.Version
	}
	return view
}

func lemmaReviewPageError(canCorrect bool) string {
	if canCorrect {
		return ""
	}
	return "This Book is current reading. End current reading before changing its vocabulary; the frozen reading snapshot remains unchanged."
}

func (h *Handler) correctLemma(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r)
	bookID := strings.TrimSpace(r.PathValue("bookID"))
	switch r.FormValue("stage") {
	case "preview":
		h.previewLemmaDecision(w, r, owner, bookID)
	case "confirm":
		h.confirmLemmaDecision(w, r, owner, bookID)
	default:
		http.Error(w, "Review the proposed consequence before confirming a decision.", http.StatusBadRequest)
	}
}

func (h *Handler) previewLemmaDecision(w http.ResponseWriter, r *http.Request, owner domain.User, bookID string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid review form.", http.StatusBadRequest)
		return
	}
	detail, ok := h.lemmaReviewBook(w, r, owner, bookID)
	if !ok {
		return
	}
	assessment, err := h.services.LemmaReview.Assess(r.Context(), owner.ID, bookID)
	if err != nil {
		fail(w, err)
		return
	}
	adoptDetailLanguage(r.Context(), detail)
	language, _ := activeStudyLanguageForContext(r.Context())
	preview, err := h.services.LemmaReview.Preview(r.Context(), owner.ID, bookID, lemmaReviewProposal(r, language, previewOccurrenceIDs(r)))
	if err != nil {
		writeLemmaReviewError(w, err)
		return
	}
	form := strings.TrimSpace(r.FormValue("form"))
	review, err := h.services.LemmaReview.Review(r.Context(), owner.ID, bookID, form)
	if err != nil {
		fail(w, err)
		return
	}
	review.Recovery.ReferenceAssessed = assessment.Assessed
	render(w, r, LemmaReviewPage(owner, h.csrf(w, r), bookID, detail.Book.Title, form, "", true, review.Occurrences, &preview, review.Recovery, nil, h.lemmaReviewReports(r, owner.ID, bookID)))
}

func (h *Handler) confirmLemmaDecision(w http.ResponseWriter, r *http.Request, owner domain.User, bookID string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid review form.", http.StatusBadRequest)
		return
	}
	detail, ok := h.lemmaReviewBook(w, r, owner, bookID)
	if !ok {
		return
	}
	adoptDetailLanguage(r.Context(), detail)
	language, _ := activeStudyLanguageForContext(r.Context())
	form := strings.TrimSpace(r.FormValue("form"))
	proposal := lemmaReviewProposal(r, language, r.Form["selected"])
	if err := h.services.LemmaReview.Confirm(r.Context(), owner.ID, bookID, proposal, r.FormValue("fingerprint")); err != nil {
		writeLemmaReviewError(w, err)
		return
	}
	h.repairStudyReports(r, owner.ID, bookID)
	redirect(w, r, "/reading/books/"+url.PathEscape(bookID)+"/lemma-review?form="+url.QueryEscape(form))
}

// repairStudyReports ends the open Wrong-word reports in the Book that the
// confirmed decision repaired, so their words return to Study (ADR 0135 §2). The
// decision is already saved; a failure here is logged, and Today's own
// reconciliation ends the report on the next visit.
func (h *Handler) repairStudyReports(r *http.Request, owner, bookID string) {
	if h.services.Store.Study == nil {
		return
	}
	if err := study.ReconcileBookReports(r.Context(), h.services.Store.Study, owner, bookID, h.now()); err != nil {
		log.Printf("mouseion: repair study reports after a lemma decision: %v", err)
	}
}

// lemmaReviewReports lists the Book's open Wrong-word reports for its Lemma
// review page. A failure shows none rather than failing the review.
func (h *Handler) lemmaReviewReports(r *http.Request, owner, bookID string) []domain.StudyReport {
	if h.services.Store.Study == nil {
		return nil
	}
	reports, err := h.services.Store.Study.ListOpenWrongWordReportsForBook(r.Context(), owner, bookID)
	if err != nil {
		log.Printf("mouseion: list study reports for lemma review: %v", err)
		return nil
	}
	// An Analysis update re-publishes an occurrence in a new run, so each
	// report is located by its place in the Book's unit, as corrections are.
	for i, report := range reports {
		if located, err := h.services.Store.Study.StudyOccurrenceLocator(r.Context(), owner, report.Source); err == nil {
			reports[i].Source = located
		}
	}
	return reports
}

// previewOccurrenceIDs is the occurrence a preview starts from plus any others
// the learner explicitly included.
func previewOccurrenceIDs(r *http.Request) []string {
	return append([]string{strings.TrimSpace(r.FormValue("target"))}, r.Form["also"]...)
}

func lemmaReviewProposal(r *http.Request, language string, occurrenceIDs []string) lemmareview.Proposal {
	return lemmareview.Proposal{
		Language: language, Form: strings.TrimSpace(r.FormValue("form")), Action: r.FormValue("decision"),
		Lemma: r.FormValue("lemma"), OccurrenceIDs: occurrenceIDs,
	}
}

// lemmaReviewRejection is how the webapp answers one workflow rejection: the
// HTTP status and the learner-facing message.
type lemmaReviewRejection struct {
	sentinel error
	status   int
	message  string
}

// lemmaReviewRejections is the one place learner-facing lemma review copy lives.
// Every domain rejection needs an entry; the table test enforces it.
var lemmaReviewRejections = []lemmaReviewRejection{
	{domain.ErrLemmaReviewCurrentReading, http.StatusConflict, "Stop this Book's current reading before changing its vocabulary."},
	{domain.ErrLemmaReviewStale, http.StatusConflict, "Learner vocabulary state or this proposal changed after preview. No decision was saved; review it again."},
	{domain.ErrLemmaReviewCountsRefreshing, http.StatusConflict, "Vocabulary counts are being refreshed. No decision was saved; review it again shortly."},
	{domain.ErrLemmaReviewOccurrenceUnavailable, http.StatusBadRequest, "The selected occurrence changed. Find it again before reviewing."},
	{domain.ErrLemmaReviewOccurrenceMissing, http.StatusConflict, "That occurrence is no longer available in this analysis."},
	{domain.ErrLemmaReviewNoSelection, http.StatusBadRequest, "The preview contains no selected occurrences."},
	{domain.ErrLemmaReviewDecision, http.StatusBadRequest, "Choose a decision to preview."},
	{domain.ErrLemmaReviewLemma, http.StatusBadRequest, "Enter one valid canonical lemma without spaces."},
	{domain.ErrLemmaReviewUnsupportedLanguage, http.StatusBadRequest, "Choose a supported study language before correcting a lemma."},
}

// lemmaReviewRejectionFor returns the table entry for a workflow rejection.
func lemmaReviewRejectionFor(err error) (lemmaReviewRejection, bool) {
	for _, rejection := range lemmaReviewRejections {
		if errors.Is(err, rejection.sentinel) {
			return rejection, true
		}
	}
	return lemmaReviewRejection{}, false
}

// writeLemmaReviewError maps the workflow's rejections to statuses and learner
// copy. Anything else is an infrastructure failure.
func writeLemmaReviewError(w http.ResponseWriter, err error) {
	rejection, ok := lemmaReviewRejectionFor(err)
	if !ok {
		fail(w, err)
		return
	}
	http.Error(w, rejection.message, rejection.status)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

// lemmaReviewReported reports whether a Study report is of the occurrence: the
// same place in the Book's unit when the report is located, else the same token
// of the same run.
func lemmaReviewReported(report domain.StudyReport, occurrence domain.LemmaReviewOccurrence) bool {
	source := report.Source
	if source.BookID != occurrence.BookID {
		return false
	}
	if source.UnitID != "" {
		return source.UnitID == occurrence.SourceDocumentID && source.StartOffset == occurrence.StartOffset && source.EndOffset == occurrence.EndOffset
	}
	return source.AnalysisRunID == occurrence.AnalysisRunID && source.SentenceOrdinal == occurrence.SentenceOrdinal && source.TokenOrdinal == occurrence.TokenOrdinal
}

// lemmaReviewReportPath opens the Book's Lemma review at the occurrence a Study
// report names: the report's own occurrence, not every occurrence of its
// surface form, which a homograph could match (ADR 0135 §2).
func lemmaReviewReportPath(bookID string, report domain.StudyReport) string {
	return "/reading/books/" + url.PathEscape(bookID) + "/lemma-review?" + url.Values{"form": {report.TargetWord}, "report": {report.ID}}.Encode()
}

// lemmaReviewReportParam names the Study report whose occurrence the page opens.
const lemmaReviewReportParam = "report"

// onlyReportedOccurrences narrows the searched occurrences to the one the Study
// report named by the page's report parameter is of. The page keeps the whole
// search when the parameter is absent, is not the learner's open report in this
// Book, or its occurrence is no longer among the results.
func (h *Handler) onlyReportedOccurrences(r *http.Request, owner, bookID string, occurrences []domain.LemmaReviewOccurrence) []domain.LemmaReviewOccurrence {
	id := strings.TrimSpace(r.URL.Query().Get(lemmaReviewReportParam))
	if id == "" {
		return occurrences
	}
	report, err := h.services.Store.Study.GetStudyReport(r.Context(), owner, id)
	if err != nil || !report.Open() || report.Source.BookID != bookID {
		return occurrences
	}
	var reported []domain.LemmaReviewOccurrence
	for _, occurrence := range occurrences {
		if lemmaReviewReported(report, occurrence) {
			reported = append(reported, occurrence)
		}
	}
	if len(reported) == 0 {
		return occurrences
	}
	return reported
}
