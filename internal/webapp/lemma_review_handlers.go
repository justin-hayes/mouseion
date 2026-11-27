package webapp

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
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
	step, err := h.previewLemmaDecisionStep(r, owner.ID, bookID, previewOccurrenceIDs(r))
	if err != nil {
		writeLemmaDecisionError(w, r, err)
		return
	}
	assessment, err := h.services.LemmaReview.Assess(r.Context(), owner.ID, bookID)
	if err != nil {
		fail(w, err)
		return
	}
	step.Review.Recovery.ReferenceAssessed = assessment.Assessed
	render(w, r, LemmaReviewPage(owner, h.csrf(w, r), bookID, step.Detail.Book.Title, step.Form, "", true, step.Review.Occurrences, &step.Preview, step.Review.Recovery, nil, h.lemmaReviewReports(r, owner.ID, bookID)))
}

func (h *Handler) confirmLemmaDecision(w http.ResponseWriter, r *http.Request, owner domain.User, bookID string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid review form.", http.StatusBadRequest)
		return
	}
	form, err := h.confirmLemmaDecisionStep(r, owner.ID, bookID, r.Form["selected"])
	if err != nil {
		writeLemmaDecisionError(w, r, err)
		return
	}
	redirect(w, r, "/reading/books/"+url.PathEscape(bookID)+"/lemma-review?form="+url.QueryEscape(form))
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
