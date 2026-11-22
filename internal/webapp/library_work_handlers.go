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

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/readingintent"
)

// workBooks loads every owned Book of one reviewed Work in constituent-book
// order, including Books outside the current filter. Work-level actions
// therefore act on the whole Work while disposition stays per Book.
func (h *Handler) workBooks(ctx context.Context, owner, workKey string) ([]domain.MyBook, error) {
	workKey = strings.TrimSpace(workKey)
	if workKey == "" {
		return nil, domain.ErrNotFound
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
		return nil, domain.ErrNotFound
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
	if errors.Is(err, domain.ErrNotFound) {
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

	total := moveWorkToReadOutcome{}
	for _, book := range books {
		total.add(h.moveWorkBookToRead(r.Context(), owner, book))
	}
	myBooksRedirectWithNotice(w, r, returnTo, workToReadNotice(total))
}

// moveWorkToReadOutcome is one Book's independent result in a Work move.
// legacyRows and withdrawn name causes no retry can fix; prepFailed counts only
// every other preparation failure.
type moveWorkToReadOutcome struct {
	moved, unchanged, prepFailed, failed, legacyRows, withdrawn int
}

// add folds one Book's result into the Work total.
func (o *moveWorkToReadOutcome) add(other moveWorkToReadOutcome) {
	o.moved += other.moved
	o.unchanged += other.unchanged
	o.prepFailed += other.prepFailed
	o.failed += other.failed
	o.legacyRows += other.legacyRows
	o.withdrawn += other.withdrawn
}

// workToReadNotice states what happened to the Work and names the preparation
// causes a learner cannot fix by retrying.
func workToReadNotice(outcome moveWorkToReadOutcome) string {
	message := fmt.Sprintf("Moved %s to To Read.", countNoun(outcome.moved, "Book", "Books"))
	if outcome.unchanged > 0 {
		message += fmt.Sprintf(" Left %s unchanged.", countNoun(outcome.unchanged, "Book", "Books"))
	}
	if outcome.failed > 0 {
		message += fmt.Sprintf(" Could not update %s.", countNoun(outcome.failed, "Book", "Books"))
	}
	if outcome.prepFailed > 0 {
		message += fmt.Sprintf(" Preparation could not start for %s.", countNoun(outcome.prepFailed, "Book", "Books"))
	}
	if outcome.legacyRows > 0 {
		message += fmt.Sprintf(" %s cannot be prepared while this installation converts its legacy Ancient Greek rows.",
			countNoun(outcome.legacyRows, "Book", "Books"))
	}
	if outcome.withdrawn > 0 {
		verb := "have"
		if outcome.withdrawn == 1 {
			verb = "has"
		}
		message += fmt.Sprintf(" %s %s a withdrawn definition and cannot be prepared.",
			countNoun(outcome.withdrawn, "Book", "Books"), verb)
	}
	return message
}

// moveWorkBookToRead applies the bulk rule to one Book: no reading history,
// never the current Book, and preparation only after a real move. The
// reading-intent module owns the transition, the route and the log line; this
// loop keeps only the Work eligibility rule and its tally. Every Book in a Work
// carries a Reading unit, so the module's EPUB route is not reached here.
func (h *Handler) moveWorkBookToRead(ctx context.Context, owner string, book domain.MyBook) moveWorkToReadOutcome {
	if !book.WorkToReadEligible() {
		return moveWorkToReadOutcome{unchanged: 1}
	}
	intent, err := h.readingIntentModule()
	if err != nil {
		logWorkActionFailure(owner, book.Book.ID, "To Read", err)
		return moveWorkToReadOutcome{failed: 1}
	}
	outcome, err := intent.Move(ctx, owner, book.Book.ID, book.DispositionRevision)
	if err != nil {
		if workTransitionUnchanged(err) {
			return moveWorkToReadOutcome{unchanged: 1}
		}
		logWorkActionFailure(owner, book.Book.ID, "To Read", err)
		return moveWorkToReadOutcome{failed: 1}
	}
	switch outcome.Kind {
	case readingintent.OutcomeUnchanged, readingintent.OutcomeStaleRevision, readingintent.OutcomeNotFound:
		return moveWorkToReadOutcome{unchanged: 1}
	case readingintent.OutcomeLegacyRows:
		return moveWorkToReadOutcome{moved: 1, legacyRows: 1}
	case readingintent.OutcomeWithdrawnDefinition:
		return moveWorkToReadOutcome{moved: 1, withdrawn: 1}
	case readingintent.OutcomeSourceRefused, readingintent.OutcomeCouldNotStart,
		readingintent.OutcomeNoUsableEPUBUnits, readingintent.OutcomePreparationUnavailable:
		return moveWorkToReadOutcome{moved: 1, prepFailed: 1}
	default:
		return moveWorkToReadOutcome{moved: 1}
	}
}

// removeWorkFromMyBooks ends every Book of one grouped Work's membership, except
// the current reading. It names that Book so the learner knows the Work is not
// entirely gone.
func (h *Handler) removeWorkFromMyBooks(w http.ResponseWriter, r *http.Request) {
	books, owner, returnTo, ok := h.workAction(w, r)
	if !ok {
		return
	}

	removal, err := h.services.Store.MyBooks.RemoveWorkFromMyBooks(r.Context(), owner, r.PathValue("workKey"))
	if errors.Is(err, domain.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	message := fmt.Sprintf("Removed %s from My Books.", countNoun(removal.RemovedBooks, "Book", "Books"))
	if removal.CurrentReading != nil {
		message += fmt.Sprintf(" %s stays because it is your current reading.", myBooksCurrentReadingName(books, removal.CurrentReading))
	}
	myBooksRedirectWithNotice(w, r, returnTo, message)
}

// myBooksCurrentReadingName names the skipped current Book with its constituent
// position when the ordered Work rows carry one, and otherwise uses the title
// the removal result returned.
func myBooksCurrentReadingName(books []domain.MyBook, current *domain.Book) string {
	if current == nil {
		return "The current Book"
	}
	for _, book := range books {
		if book.Book.ID == current.ID {
			return "“" + myBookDisplayTitle(book) + "”"
		}
	}
	return "“" + current.Title + "”"
}

// workTransitionUnchanged reports a transition that needs no retry: a stale
// form or a Book that left the collection did not change anything.
func workTransitionUnchanged(err error) bool {
	return errors.Is(err, domain.ErrStaleBookDisposition) || errors.Is(err, domain.ErrNotFound)
}

func logWorkActionFailure(owner, bookID, action string, err error) {
	log.Printf("mouseion: work %s update failed for owner %s book %s: %s",
		strconv.Quote(action), strconv.Quote(owner), strconv.Quote(bookID), strconv.Quote(err.Error()))
}

func myBooksRedirectWithNotice(w http.ResponseWriter, r *http.Request, returnTo, text string) {
	separator := "?"
	if strings.Contains(returnTo, "?") {
		separator = "&"
	}
	redirect(w, r, returnTo+separator+"message="+url.QueryEscape(text))
}
