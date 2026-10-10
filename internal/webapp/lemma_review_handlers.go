package webapp

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

// lemmaReviewSuggestion is the optional LLM suggestion shown beside the
// occurrence it was requested for. It is advisory and never applied.
type lemmaReviewSuggestion struct {
	Target                            string
	Lemma, Provider, Version, Message string
}

func (h *Handler) lemmaReview(w http.ResponseWriter, r *http.Request) {
	owner := user(r)
	bookID := strings.TrimSpace(r.PathValue("bookID"))
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	assessment, err := h.services.LemmaReview.Assess(r.Context(), owner.ID, bookID)
	if err != nil {
		fail(w, err)
		return
	}
	if !lemmaReviewLanguageMatches(w, r, detail, "reviewing occurrences") {
		return
	}
	form := strings.TrimSpace(r.URL.Query().Get("form"))
	review, err := h.services.LemmaReview.Review(r.Context(), owner.ID, bookID, form)
	if err != nil {
		fail(w, err)
		return
	}
	review.Recovery.ReferenceAssessed = assessment.Assessed
	render(w, r, LemmaReviewPage(owner, h.csrf(w, r), bookID, detail.Book.Title, form, lemmaReviewPageError(review.CanCorrect), review.CanCorrect, review.Occurrences, nil, review.Recovery, nil))
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
	if !lemmaReviewLanguageMatches(w, r, detail, "requesting a suggestion") {
		return
	}
	suggestion, err := h.services.LemmaReview.Suggest(r.Context(), owner.ID, bookID, occurrenceID)
	if errors.Is(err, domain.ErrLemmaReviewOccurrenceMissing) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	review, err := h.services.LemmaReview.Review(r.Context(), owner.ID, bookID, form)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, LemmaReviewPage(owner, h.csrf(w, r), bookID, detail.Book.Title, form, lemmaReviewPageError(review.CanCorrect), review.CanCorrect, review.Occurrences, nil, review.Recovery, lemmaSuggestionView(occurrenceID, suggestion)))
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

// lemmaReviewLanguageMatches reports whether the request's study language is the
// Book's, and writes the conflict response when it is not.
func lemmaReviewLanguageMatches(w http.ResponseWriter, r *http.Request, detail domain.MyBook, purpose string) bool {
	language, _ := activeStudyLanguageForContext(r.Context())
	if language != "" && language == detail.Book.LanguageTag {
		return true
	}
	http.Error(w, "Choose this Book's study language in Reading before "+purpose+".", http.StatusConflict)
	return false
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
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	assessment, err := h.services.LemmaReview.Assess(r.Context(), owner.ID, bookID)
	if err != nil {
		fail(w, err)
		return
	}
	if !lemmaReviewLanguageMatches(w, r, detail, "reviewing occurrences") {
		return
	}
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
	render(w, r, LemmaReviewPage(owner, h.csrf(w, r), bookID, detail.Book.Title, form, "", true, review.Occurrences, &preview, review.Recovery, nil))
}

func (h *Handler) confirmLemmaDecision(w http.ResponseWriter, r *http.Request, owner domain.User, bookID string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid review form.", http.StatusBadRequest)
		return
	}
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if !lemmaReviewLanguageMatches(w, r, detail, "reviewing occurrences") {
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	form := strings.TrimSpace(r.FormValue("form"))
	proposal := lemmaReviewProposal(r, language, r.Form["selected"])
	outcome, err := h.services.LemmaReview.Confirm(r.Context(), owner.ID, bookID, proposal, r.FormValue("fingerprint"), r.FormValue("reprepare_ready_deck") == "yes")
	if err != nil {
		writeLemmaReviewError(w, err)
		return
	}
	switch outcome.Kind {
	case lemmareview.OutcomePreparing:
		redirect(w, r, "/deck-preparations/"+url.PathEscape(outcome.PreparationID)+"/status")
	case lemmareview.OutcomeRestartFailed:
		http.Error(w, "The identity decision was saved, but a new Reading snapshot could not be started. Start this Book in Reading, then prepare its deck; the historical deck remains available.", http.StatusServiceUnavailable)
	case lemmareview.OutcomeQueueingFailed:
		http.Error(w, "The identity decision was saved, but re-preparation could not be queued. The historical deck remains available; retry preparation from the deck task.", http.StatusServiceUnavailable)
	case lemmareview.OutcomeSaved, lemmareview.OutcomeFlagsUnresolved:
		// Both return to the review page; the latter shows the flags that block the restart.
		redirect(w, r, "/reading/books/"+url.PathEscape(bookID)+"/lemma-review?form="+url.QueryEscape(form))
	}
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

// writeLemmaReviewError maps the workflow's rejections to statuses. Anything
// else is an infrastructure failure.
func writeLemmaReviewError(w http.ResponseWriter, err error) {
	for _, sentinel := range []error{
		domain.ErrLemmaReviewCurrentReading, domain.ErrLemmaReviewStale, domain.ErrLemmaReviewCountsRefreshing,
		domain.ErrLemmaReviewBlockedByReading, domain.ErrLemmaReviewNotToRead,
	} {
		if errors.Is(err, sentinel) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	for _, sentinel := range []error{
		domain.ErrLemmaReviewRepreparationRequired, domain.ErrLemmaReviewOccurrenceUnavailable, domain.ErrLemmaReviewOccurrenceMissing,
		domain.ErrLemmaReviewNoSelection, domain.ErrLemmaReviewDecision, domain.ErrLemmaReviewLemma, domain.ErrLemmaReviewUnsupportedLanguage,
	} {
		if errors.Is(err, sentinel) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	fail(w, err)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
