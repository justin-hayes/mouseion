package webapp

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/readingintent"
)

// vocabularyCountsUpdatingDescription explains why a Book shows no coverage
// while its effective vocabulary counts are rebuilt.
const vocabularyCountsUpdatingDescription = "This Book's vocabulary counts are being refreshed. Coverage appears when they are ready."

func (h *Handler) reading(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if !readingDeskRequest(query) {
		redirectMyBooksFeedback(w, r, query)
		return
	}
	owner := user(r)
	language, languageLabel := activeStudyLanguageForContext(r.Context())
	currentReading, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner.ID, language)
	if err != nil {
		fail(w, err)
		return
	}
	// Browse state with no Current reading and no Book named has nothing to
	// restore: between Books there is no Browse, so the learner is sent to My Books.
	if reading := strings.TrimSpace(query.Get("reading")); reading == "" && !currentReading.IsActive() {
		redirectMyBooksFeedback(w, r, query)
		return
	}
	// Back to book vocabulary names the Book it left. Only that Book may restore
	// controls; anything else is explained before any other reading context
	// opens, and nothing is transferred to it.
	if reading := strings.TrimSpace(query.Get("reading")); !currentReading.IsActive() || (reading != "" && reading != currentReading.BookID) {
		h.renderReadingBrowseOriginExpired(w, r, owner, language, currentReading.IsActive())
		return
	}
	readingView, err := h.buildReadingView(r.Context(), owner.ID, language)
	if err != nil {
		fail(w, err)
		return
	}
	readingView.Language = language
	readingView.LanguageLabel = languageLabel
	status := http.StatusOK
	if readingView.CurrentReading != nil {
		readingView.Browse, status = h.loadReadingBrowse(r.Context(), r, owner.ID, language, currentReading.BookID)
	}
	renderStatus(w, r, status, ReadingPage(owner, h.csrf(w, r), readingView, query.Get("message"), query.Get("error")))
}

// readingDeskQueryKeys are the vocabulary Browse state a working-desk address
// carries: the Book it belongs to, its prefix, accounting reveal, evidence
// revision, row and page.
var readingDeskQueryKeys = []string{"reading", "q", "all", "rev", "row", "page"}

// readingDeskRequest reports whether the query names vocabulary Browse state,
// as opposed to a bare Reading address that now belongs to My Books.
func readingDeskRequest(query url.Values) bool {
	return slices.ContainsFunc(readingDeskQueryKeys, query.Has)
}

// redirectMyBooksFeedback sends a retired Reading address to My Books in the
// request's study language, keeping the message or error it carried.
func redirectMyBooksFeedback(w http.ResponseWriter, r *http.Request, query url.Values) {
	feedback := url.Values{}
	for _, key := range []string{"message", "error"} {
		if value := query.Get(key); value != "" {
			feedback.Set(key, value)
		}
	}
	location := myBooksPagePath(r.Context())
	if encoded := feedback.Encode(); encoded != "" {
		location += "?" + encoded
	}
	redirect(w, r, location)
}

// renderReadingBrowseOriginExpired explains a Back to book vocabulary request
// whose commitment ended or was restarted. The My Books recovery is offered
// only for a Book the owner has in the active study language.
func (h *Handler) renderReadingBrowseOriginExpired(w http.ResponseWriter, r *http.Request, owner domain.User, language string, hasCurrent bool) {
	myBooksURL := ""
	if bookID := strings.TrimSpace(r.URL.Query().Get("reading")); bookID != "" {
		detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
		switch {
		case errors.Is(err, domain.ErrNotFound):
		case err != nil:
			fail(w, err)
			return
		case detail.Book.LanguageTag == language:
			myBooksURL = myBookSearchURL(detail)
		}
	}
	renderStatus(w, r, http.StatusConflict, ReadingBrowseOriginExpiredPage(owner, h.csrf(w, r), hasCurrent, myBooksURL))
}

// switchReadingPage retires the Switch page: Read next on My Books replaces
// the current reading directly.
func (h *Handler) switchReadingPage(w http.ResponseWriter, r *http.Request) {
	redirectMyBooksFeedback(w, r, r.URL.Query())
}

// endCurrentReading ends the language's current reading. It needs no live
// analysis, so End stays available when the Book's analysis is missing or
// stale, and it takes no confirmation token (ADR 0136 §4).
func (h *Handler) endCurrentReading(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	language := pageLanguage(r)
	if language == "" {
		redirect(w, r, myBooksPagePath(r.Context())+"?error="+url.QueryEscape("Choose a study language and refresh My Books before ending the current reading."))
		return
	}
	current, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	err = h.services.Store.Reading.EndCurrentReading(r.Context(), owner, language)
	if errors.Is(err, domain.ErrNotFound) {
		redirect(w, r, myBooksPagePath(r.Context())+"?error="+url.QueryEscape(noCurrentReadingMessage))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, myBooksPagePath(r.Context())+"?message="+url.QueryEscape("Reading ended. "+h.currentReadingBookTitle(r.Context(), owner, current.BookID)+" stays in To Read; its analysis and history are preserved. Choose a Book when you are ready."))
}

const noCurrentReadingMessage = "There is no current reading. Choose a book from My Books."

func (h *Handler) reanalyzeToReadBook(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := r.PathValue("id")
	if !h.adoptBookLanguageOrFail(w, r, owner, bookID) {
		return
	}
	intent, err := h.readingIntentModule()
	if err != nil {
		fail(w, err)
		return
	}
	outcome, err := intent.Retry(r.Context(), owner, bookID)
	if err != nil {
		fail(w, err)
		return
	}
	switch outcome.Kind {
	case readingintent.OutcomeNotFound, readingintent.OutcomeNotToRead:
		http.NotFound(w, r)
		return
	default:
	}
	message, isError := retryMessage(outcome)
	queryParam := "message"
	if isError {
		queryParam = "error"
	}
	redirect(w, r, myBooksPagePath(r.Context())+"?"+queryParam+"="+url.QueryEscape(message))
}

// analysisCouldNotStartMessage is the retry sentence for a submission that
// failed without a stable cause.
const analysisCouldNotStartMessage = "Analysis could not be started. The To Read choice is retained. Review current book content in My Books and try again."

// retryMessage maps a Retry outcome to the learner-facing sentence and whether
// it is an error. An EPUB Book reports the same plain confirmation whether
// acquisition was queued now or is already running.
func retryMessage(outcome readingintent.Outcome) (message string, isError bool) {
	switch outcome.Kind {
	case readingintent.OutcomeCurrentBookRefused:
		return "The current book cannot be refreshed. Its frozen evidence remains unchanged.", true
	case readingintent.OutcomePreparationUnavailable:
		return "Classical preparation is unavailable.", true
	case readingintent.OutcomeAlreadyCurrent:
		return "The analysis is already current.", false
	case readingintent.OutcomeJobQueued:
		switch {
		case outcome.Route == readingintent.RouteClassical:
			return fmt.Sprintf("Preparation job #%d submitted.", outcome.JobNumber), false
		case outcome.Acquisition:
			return epubPreparationConfirmation, false
		default:
			return fmt.Sprintf("Analysis job #%d submitted.", outcome.JobNumber), false
		}
	case readingintent.OutcomeLegacyRows:
		return classical.LegacyRowsReason, true
	case readingintent.OutcomeWithdrawnDefinition:
		return classicalWithdrawnRefusal, true
	case readingintent.OutcomeSourceRefused:
		return classicalSourceMismatchRefusal, true
	case readingintent.OutcomeCouldNotStart:
		if outcome.Route == readingintent.RouteClassical {
			return "Preparation could not be started. Retry when ready.", true
		}
		return analysisCouldNotStartMessage, true
	case readingintent.OutcomeNoUsableEPUBUnits:
		return "The current source has no usable EPUB units. The To Read choice is retained; analysis is unavailable.", true
	default:
		// Retry returns only the kinds above; a new kind must be mapped here.
		return analysisCouldNotStartMessage, true
	}
}

// readNext makes a To Read Book the current reading, replacing any other. It
// needs no confirmation and no commitment from the page; it is refused only
// when the Book is not To Read, not analysed, or has no Book language
// (ADR 0136 §4). Starting and switching are the same transition.
func (h *Handler) readNext(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := strings.TrimSpace(r.PathValue("id"))
	if !h.adoptBookLanguageOrFail(w, r, owner, bookID) {
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" {
		redirect(w, r, myBooksPagePath(r.Context())+"?error="+url.QueryEscape("Choose a study language before reading a book."))
		return
	}
	start, err := h.services.Store.Reading.StartCurrentReadingResult(r.Context(), owner, language, bookID)
	var ineligible domain.CurrentReadingIneligibleError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		http.NotFound(w, r)
	case err == nil && start.Replayed:
		redirect(w, r, myBooksPagePath(r.Context())+"?message="+url.QueryEscape(h.currentReadingBookTitle(r.Context(), owner, bookID)+" is already your current reading."))
	case errors.As(err, &ineligible) && ineligible.Reason == domain.CurrentReadingNoChosenLanguage:
		redirect(w, r, myBooksPagePath(r.Context())+"?error="+url.QueryEscape("This book has no language yet. Choose its language in My Books before reading it. No changes were made."))
	case errors.As(err, &ineligible) && ineligible.Reason.IsBookChoiceRejection():
		redirect(w, r, myBooksPagePath(r.Context())+"?error="+url.QueryEscape("This book is no longer an eligible To Read candidate. Review My Books before trying again."))
	case errors.Is(err, domain.ErrCurrentReadingIneligible):
		redirect(w, r, myBooksPagePath(r.Context())+"?error="+url.QueryEscape("This book no longer has trustworthy current analysis or is no longer To Read. No changes were made; refresh My Books and try again."))
	case err != nil:
		fail(w, err)
	default:
		redirect(w, r, myBooksPagePath(r.Context())+"?message="+url.QueryEscape(h.currentReadingBookTitle(r.Context(), owner, bookID)+" is now your current reading."))
	}
}

// pageLanguage is the study language the Reading page that posted the form was
// about, falling back to the active study language. It names a language, not a
// commitment, so Finish and End act on the language the learner saw even when
// another tab has moved the Last study language.
func pageLanguage(r *http.Request) string {
	if language := strings.TrimSpace(r.FormValue("language")); language != "" {
		return language
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	return language
}
