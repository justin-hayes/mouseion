package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

// workBooks loads every owned Book of one reviewed Work in constituent-book
// order, including Hidden Books and Books outside the current filter. Work-level
// actions therefore act on the whole Work while visibility and disposition stay
// per Book.
func (h *Handler) workBooks(ctx context.Context, owner, workKey string) ([]domain.MyBook, error) {
	workKey = strings.TrimSpace(workKey)
	if workKey == "" {
		return nil, persistence.ErrNotFound
	}
	books, err := h.services.Store.MyBooks.ListMyBooksWithEvidence(ctx, owner)
	if err != nil {
		return nil, err
	}
	work := make([]domain.MyBook, 0, len(books))
	for _, book := range books {
		if book.ReadingUnit != nil && book.ReadingUnit.WorkKey == workKey {
			work = append(work, book)
		}
	}
	if len(work) == 0 {
		return nil, persistence.ErrNotFound
	}
	sort.SliceStable(work, func(i, j int) bool {
		return domain.MyBookWorkOrderLess(work[i], work[j])
	})
	return work, nil
}

// workAction authorizes and loads one grouped Work action. The whole Work is
// the target, not only the cells the current filter shows.
func (h *Handler) workAction(w http.ResponseWriter, r *http.Request) (books []domain.MyBook, owner, returnTo string, ok bool) {
	if !h.checkCSRF(w, r) {
		return nil, "", "", false
	}
	owner = user(r).ID
	returnTo = myBooksReturnURL(r.FormValue("return_to"))
	books, err := h.workBooks(r.Context(), owner, r.PathValue("workKey"))
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return nil, "", "", false
	} else if err != nil {
		fail(w, err)
		return nil, "", "", false
	}
	return books, owner, returnTo, true
}

// moveWorkToRead is the grouped Work row's primary action. It moves every Book
// without reading history, leaves the current Book alone, and enqueues each
// Book's preparation in constituent-book order. One Book's failure never stops
// the rest.
func (h *Handler) moveWorkToRead(w http.ResponseWriter, r *http.Request) {
	books, owner, returnTo, ok := h.workAction(w, r)
	if !ok {
		return
	}

	moved, unchanged, prepFailed, failed := 0, 0, 0, 0
	for _, book := range books {
		outcome := h.moveWorkBookToRead(r.Context(), owner, book)
		moved += outcome.moved
		unchanged += outcome.unchanged
		prepFailed += outcome.prepFailed
		failed += outcome.failed
	}

	message := fmt.Sprintf("Moved %s to To Read.", countNoun(moved, "Book", "Books"))
	if unchanged > 0 {
		message += fmt.Sprintf(" Left %s unchanged.", countNoun(unchanged, "Book", "Books"))
	}
	if failed > 0 {
		message += fmt.Sprintf(" Could not update %s.", countNoun(failed, "Book", "Books"))
	}
	if prepFailed > 0 {
		message += fmt.Sprintf(" Preparation could not start for %s.", countNoun(prepFailed, "Book", "Books"))
	}
	myBooksRedirectWithNotice(w, r, returnTo, message)
}

// moveWorkToReadOutcome is one Book's independent result in a Work move.
type moveWorkToReadOutcome struct {
	moved, unchanged, prepFailed, failed int
}

// moveWorkBookToRead applies the bulk rule to one Book: no reading history,
// never the current Book, and preparation only after a real move.
func (h *Handler) moveWorkBookToRead(ctx context.Context, owner string, book domain.MyBook) moveWorkToReadOutcome {
	if !book.WorkToReadEligible() {
		return moveWorkToReadOutcome{unchanged: 1}
	}
	applied, err := h.services.Store.MyBooks.TransitionBookDisposition(ctx, owner, book.Book.ID, book.DispositionRevision, domain.BookDispositionToRead)
	if workTransitionUnchanged(err) {
		return moveWorkToReadOutcome{unchanged: 1}
	} else if err != nil {
		logWorkActionFailure(owner, book.Book.ID, "To Read", err)
		return moveWorkToReadOutcome{failed: 1}
	}
	if !applied {
		return moveWorkToReadOutcome{unchanged: 1}
	}
	outcome := moveWorkToReadOutcome{moved: 1}
	if book.ReadingUnit != nil && !book.Classification().AnalysisInEffect() && !h.queueWorkClassicalPreparation(ctx, owner, book.Book.ID) {
		outcome.prepFailed = 1
	}
	return outcome
}

// moveWorkToInbox returns every Book outside the current Reading to Inbox. The
// current Book keeps its To Read disposition, so moving the Work back never ends
// the Reading.
func (h *Handler) moveWorkToInbox(w http.ResponseWriter, r *http.Request) {
	books, owner, returnTo, ok := h.workAction(w, r)
	if !ok {
		return
	}

	moved, unchanged, currentKept, failed := 0, 0, 0, 0
	for _, book := range books {
		if book.IsCurrentReading {
			currentKept++
			continue
		}
		if myBookDisposition(book) == domain.BookDispositionInbox {
			unchanged++
			continue
		}
		applied, err := h.services.Store.MyBooks.TransitionBookDisposition(r.Context(), owner, book.Book.ID, book.DispositionRevision, domain.BookDispositionInbox)
		switch {
		case workTransitionUnchanged(err):
			unchanged++
		case err != nil:
			logWorkActionFailure(owner, book.Book.ID, "Inbox", err)
			failed++
		case applied:
			moved++
		default:
			unchanged++
		}
	}

	message := fmt.Sprintf("Moved %s back to Inbox.", countNoun(moved, "Book", "Books"))
	if currentKept > 0 {
		message += " The current Book stays To Read."
	}
	if failed > 0 {
		message += fmt.Sprintf(" Could not update %s.", countNoun(failed, "Book", "Books"))
	}
	myBooksRedirectWithNotice(w, r, returnTo, message)
}

func (h *Handler) hideWork(w http.ResponseWriter, r *http.Request) {
	h.setWorkVisibility(w, r, true)
}

func (h *Handler) unhideWork(w http.ResponseWriter, r *http.Request) {
	h.setWorkVisibility(w, r, false)
}

// setWorkVisibility changes every owned Book's visibility. Visibility stays per
// Book underneath: disposition, current Reading and history are untouched.
func (h *Handler) setWorkVisibility(w http.ResponseWriter, r *http.Request, hidden bool) {
	books, owner, returnTo, ok := h.workAction(w, r)
	if !ok {
		return
	}

	changed, failed := 0, 0
	for _, book := range books {
		if book.Hidden == hidden {
			continue
		}
		applied, err := h.services.Store.MyBooks.SetBookHidden(r.Context(), owner, book.Book.ID, book.VisibilityRevision, hidden)
		switch {
		case errors.Is(err, persistence.ErrStaleBookVisibility), errors.Is(err, persistence.ErrNotFound):
			continue
		case err != nil:
			logWorkActionFailure(owner, book.Book.ID, "visibility", err)
			failed++
		case applied:
			changed++
		}
	}

	if hidden {
		message := fmt.Sprintf("Hid %s. Use Show hidden books to find them again. Reading intent and history are unchanged.", countNoun(changed, "Book", "Books"))
		if failed > 0 {
			message += fmt.Sprintf(" Could not update %s.", countNoun(failed, "Book", "Books"))
		}
		myBooksRedirectWithNotice(w, r, returnTo, message)
		return
	}
	message := fmt.Sprintf("Unhid %s. Reading intent and history are unchanged.", countNoun(changed, "Book", "Books"))
	if failed > 0 {
		message += fmt.Sprintf(" Could not update %s.", countNoun(failed, "Book", "Books"))
	}
	myBooksRedirectWithNotice(w, r, returnTo, message)
}

// workTransitionUnchanged reports a transition that needs no retry: a stale
// form or a Book that left the collection did not change anything.
func workTransitionUnchanged(err error) bool {
	return errors.Is(err, persistence.ErrStaleBookDisposition) || errors.Is(err, persistence.ErrNotFound)
}

func logWorkActionFailure(owner, bookID, action string, err error) {
	log.Printf("mouseion: work %s update failed for owner %s book %s: %s",
		strconv.Quote(action), strconv.Quote(owner), strconv.Quote(bookID), strconv.Quote(err.Error()))
}

// queueWorkClassicalPreparation submits one Book's ensure-once preparation and
// reports whether the submission started. A failure is logged with stable codes
// and identifiers only, then the rest of the Work continues.
func (h *Handler) queueWorkClassicalPreparation(ctx context.Context, owner, bookID string) bool {
	if h.services.ClassicalPreparation == nil {
		return false
	}
	handle, err := h.services.ClassicalPreparation.SubmitClassicalPreparation(ctx, owner, bookID)
	if err != nil {
		stage, code := analysis.ClassicalPreparationFailure(err)
		log.Printf("mouseion: classical preparation submission failed for owner %s book %s stage=%s code=%s",
			strconv.Quote(owner), strconv.Quote(strings.TrimSpace(bookID)), strconv.Quote(string(stage)), strconv.Quote(code))
		return false
	}
	return !handle.Terminal
}

func myBooksRedirectWithNotice(w http.ResponseWriter, r *http.Request, returnTo, text string) {
	separator := "?"
	if strings.Contains(returnTo, "?") {
		separator = "&"
	}
	redirect(w, r, returnTo+separator+"message="+url.QueryEscape(text))
}
