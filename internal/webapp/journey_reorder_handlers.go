package webapp

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

const journeyStaleMessage = "This Journey changed since this page was loaded. No changes were made; review Reading Journey before trying again."

func (h *Handler) moveJourneyEntryEarlier(w http.ResponseWriter, r *http.Request) {
	h.moveJourneyEntry(w, r, true)
}

func (h *Handler) moveJourneyEntryLater(w http.ResponseWriter, r *http.Request) {
	h.moveJourneyEntry(w, r, false)
}

func visibleJourneyEntries(entries []domain.ReadingJourneyEntry, goalBookID string) []domain.ReadingJourneyEntry {
	visible := make([]domain.ReadingJourneyEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.BookID != goalBookID {
			visible = append(visible, entry)
		}
	}
	return visible
}

func (h *Handler) moveJourneyEntry(w http.ResponseWriter, r *http.Request, earlier bool) {
	if !h.checkCSRF(w, r) {
		return
	}

	expectedRevision, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("expected_revision")), 10, 64)
	if err != nil {
		redirect(w, r, "/journey?error="+url.QueryEscape(journeyStaleMessage))
		return
	}
	owner := user(r).ID
	bookID := r.PathValue("id")
	journey, err := h.services.Store.GetReadingJourney(r.Context(), owner)
	if err != nil {
		fail(w, err)
		return
	}
	goal, err := h.services.Store.GetPrimaryGoal(r.Context(), owner)
	if err != nil {
		fail(w, err)
		return
	}

	// The Goal is not a reorder target, even if a caller bypasses the rendered
	// controls and posts directly to this endpoint.
	if primaryGoalIsActive(goal) && bookID == goal.BookID {
		h.redirectJourneyMove(w, r, bookID, "did not move")
		return
	}

	visible := visibleJourneyEntries(journey.Entries, goal.BookID)
	memberIndex := -1
	for i, entry := range visible {
		if entry.BookID == bookID {
			memberIndex = i
			break
		}
	}
	if memberIndex == -1 {
		// Distinguish an owned book removed from the Journey from a foreign or
		// unknown book. Both checks remain scoped to the authenticated owner.
		if _, bookErr := h.services.Store.GetBook(r.Context(), owner, bookID); errors.Is(bookErr, persistence.ErrNotFound) {
			http.NotFound(w, r)
			return
		} else if bookErr != nil {
			fail(w, bookErr)
			return
		}
		_, err = h.services.Store.MoveReadingJourneyEntry(r.Context(), owner, bookID, 1, expectedRevision)
		if errors.Is(err, persistence.ErrJourneyStale) {
			redirect(w, r, "/journey?error="+url.QueryEscape(journeyStaleMessage))
			return
		}
		if errors.Is(err, persistence.ErrNotFound) {
			redirect(w, r, "/journey?error="+url.QueryEscape("This book is no longer in your Reading Journey."))
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
		redirect(w, r, "/journey?message="+url.QueryEscape(bookID+" did not move."))
		return
	}

	newPosition := memberIndex + 1
	if earlier {
		if memberIndex > 0 {
			newPosition = memberIndex
		}
	} else if memberIndex+1 < len(visible) {
		newPosition = memberIndex + 2
	}

	// A boundary request passes the current position through the store so the
	// revision is still checked. The store returns the same revision for this
	// deterministic no-op, which is announced without changing the order.
	newRevision, err := h.services.Store.MoveReadingJourneyEntry(r.Context(), owner, bookID, newPosition, expectedRevision)
	if errors.Is(err, persistence.ErrJourneyStale) {
		redirect(w, r, "/journey?error="+url.QueryEscape(journeyStaleMessage))
		return
	}
	if errors.Is(err, persistence.ErrNotFound) {
		redirect(w, r, "/journey?error="+url.QueryEscape("This book is no longer in your Reading Journey."))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}

	view, err := h.buildJourneyView(r.Context(), owner)
	if err != nil {
		fail(w, err)
		return
	}
	position := newPosition
	title := bookID
	for i, item := range view.Provisional {
		if journeyBookID(item) == bookID {
			title = journeyBookTitle(item.Book)
			position = i + 1
			break
		}
	}
	if !isHTMX(r) {
		message := title + " moved."
		if newRevision == journey.Revision {
			message = title + " did not move."
		}
		redirect(w, r, "/journey?message="+url.QueryEscape(message))
		return
	}
	status := "Moved " + title + " to provisional position " + strconv.Itoa(position) + ". Current evidence is shown for the updated order."
	if newRevision == journey.Revision {
		status = title + " did not move and remains at provisional position " + strconv.Itoa(position) + ". Current evidence is unchanged."
	}
	render(w, r, JourneyProvisionalList(view, h.csrf(w, r), bookID, status))
}

func (h *Handler) redirectJourneyMove(w http.ResponseWriter, r *http.Request, bookID, outcome string) {
	redirect(w, r, "/journey?message="+url.QueryEscape(bookID+" "+outcome+"."))
}
