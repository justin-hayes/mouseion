package webapp

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
)

// goalSectionView is the server-truth fragment returned after an HTMX Goal
// mutation.
type goalSectionView struct {
	Goal    *journeyBookView
	Message string
	Error   string
}

const (
	goalStaleMessage            = "This Primary Goal changed since this page was loaded. No changes were made; review Reading Journey before trying again."
	goalConcurrentMessage       = "Another book became your Primary Goal while you were choosing. No changes were made; review Reading Journey before trying again."
	goalUnavailableMessage      = "This book is not available in My Books."
	goalIneligibleMessage       = "This book must be an active Reading Journey member with a successfully completed current analysis before it can become a Primary Goal."
	goalLanguageRequiredMessage = "Choose a study language before setting a Primary Goal."
)

func (h *Handler) goalBookTitle(ctx context.Context, owner, bookID string) string {
	books, err := h.services.Store.Books.ListSourceMaterials(ctx, owner)
	if err == nil {
		for _, book := range books {
			if book.Source.ID == bookID {
				return canonicalBookTitle(book)
			}
		}
	}
	if book, err := h.services.Store.Books.GetBook(ctx, owner, bookID); err == nil && strings.TrimSpace(book.Title) != "" {
		return book.Title
	}
	return bookID
}

func (h *Handler) goalSection(ctx context.Context, owner, message, pageError string) (goalSectionView, error) {
	language, _ := activeStudyLanguageForContext(ctx)
	journey, err := h.buildJourneyView(ctx, owner, language)
	if err != nil {
		return goalSectionView{}, err
	}
	return goalSectionView{Goal: journey.Goal, Message: message, Error: pageError}, nil
}

func (h *Handler) respondGoal(w http.ResponseWriter, r *http.Request, message, pageError, focusBookID string) {
	if isHTMX(r) {
		section, err := h.goalSection(r.Context(), user(r).ID, message, pageError)
		if err != nil {
			fail(w, err)
			return
		}
		render(w, r, GoalSection(section.Goal, h.csrf(w, r), section.Message, section.Error, focusBookID))
		return
	}
	query := url.Values{}
	if message != "" {
		query.Set("message", message)
	}
	if pageError != "" {
		query.Set("error", pageError)
	}
	location := "/journey"
	if encoded := query.Encode(); encoded != "" {
		location += "?" + encoded
	}
	redirect(w, r, location)
}

func (h *Handler) choosePrimaryGoal(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	language, _ := activeStudyLanguageForContext(r.Context())
	bookID := strings.TrimSpace(r.PathValue("id"))
	if bookID == "" {
		h.respondGoal(w, r, "", "Choose a book before setting a Primary Goal.", "")
		return
	}
	if language == "" {
		if _, bookErr := h.services.Store.Books.GetBook(r.Context(), owner, bookID); errors.Is(bookErr, persistence.ErrNotFound) {
			h.respondGoal(w, r, "", goalUnavailableMessage, "")
			return
		} else if bookErr != nil {
			fail(w, bookErr)
			return
		}
		h.respondGoal(w, r, "", goalLanguageRequiredMessage, bookID)
		return
	}
	expectedBookID := strings.TrimSpace(r.FormValue("expected_goal_book_id"))
	var selectedGoal domain.PrimaryGoal
	current, err := h.services.Store.Goals.GetPrimaryGoal(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	title := h.goalBookTitle(r.Context(), owner, bookID)
	currentActive := current.IsActive()
	if currentActive && current.BookID == bookID {
		h.respondGoal(w, r, title+" is already your Primary Goal.", "", bookID)
		return
	}
	if currentActive && current.BookID != expectedBookID {
		h.respondGoal(w, r, "", goalStaleMessage, current.BookID)
		return
	}
	if !currentActive {
		if expectedBookID != "" {
			h.respondGoal(w, r, "", goalStaleMessage, "")
			return
		}
		selectedGoal, err = h.services.Store.Goals.CreatePrimaryGoal(r.Context(), owner, language, bookID)
		if errors.Is(err, persistence.ErrGoalExists) {
			latest, readErr := h.services.Store.Goals.GetPrimaryGoal(r.Context(), owner, language)
			if readErr != nil {
				fail(w, readErr)
				return
			}
			if latest.BookID == bookID {
				h.respondGoal(w, r, title+" is already your Primary Goal.", "", bookID)
				return
			}
			h.respondGoal(w, r, "", goalConcurrentMessage, latest.BookID)
			return
		}
		if errors.Is(err, persistence.ErrNotFound) {
			h.respondGoal(w, r, "", goalUnavailableMessage, "")
			return
		}
		if errors.Is(err, persistence.ErrGoalIneligible) {
			h.respondGoal(w, r, "", goalIneligibleMessage, bookID)
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
	} else {
		selectedGoal, err = h.services.Store.Goals.ChangePrimaryGoal(r.Context(), owner, language, bookID, expectedBookID)
		if errors.Is(err, persistence.ErrGoalStale) {
			h.respondGoal(w, r, "", goalStaleMessage, current.BookID)
			return
		}
		if errors.Is(err, persistence.ErrNotFound) {
			if _, bookErr := h.services.Store.Books.GetBook(r.Context(), owner, bookID); errors.Is(bookErr, persistence.ErrNotFound) {
				h.respondGoal(w, r, "", goalUnavailableMessage, current.BookID)
				return
			} else if bookErr != nil {
				fail(w, bookErr)
				return
			}
			h.respondGoal(w, r, "", goalStaleMessage, current.BookID)
			return
		}
		if errors.Is(err, persistence.ErrGoalIneligible) {
			h.respondGoal(w, r, "", goalIneligibleMessage, bookID)
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
	}
	if selectedGoal.AnalysisRunID != "" && selectedGoal.SnapshotSize > 0 && h.services.PreparedDeck != nil {
		var prepareErr error
		if goalDeck, ok := h.services.PreparedDeck.(interface {
			SubmitForGoal(context.Context, string, string) (prepareddeck.Handle, error)
		}); ok {
			_, prepareErr = goalDeck.SubmitForGoal(r.Context(), owner, selectedGoal.AnalysisRunID)
		} else {
			_, prepareErr = h.services.PreparedDeck.Submit(r.Context(), owner, selectedGoal.AnalysisRunID, false)
		}
		if prepareErr != nil {
			log.Printf("primary goal deck preparation owner=%s language=%s book=%s: %v", owner, language, selectedGoal.BookID, prepareErr)
		}
	}
	message := title + " is your Primary Goal."
	h.respondGoal(w, r, message, "", bookID)
}

func (h *Handler) clearPrimaryGoal(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" {
		h.respondGoal(w, r, "", goalLanguageRequiredMessage, "")
		return
	}
	expectedBookID := strings.TrimSpace(r.FormValue("expected_goal_book_id"))
	current, err := h.services.Store.Goals.GetPrimaryGoal(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	if current.BookID == "" {
		if expectedBookID != "" {
			h.respondGoal(w, r, "", goalStaleMessage, "")
		} else {
			h.respondGoal(w, r, "No Primary Goal was set.", "", "")
		}
		return
	}
	if current.BookID != expectedBookID {
		h.respondGoal(w, r, "", goalStaleMessage, current.BookID)
		return
	}
	err = h.services.Store.Goals.ClearPrimaryGoal(r.Context(), owner, language, expectedBookID)
	if errors.Is(err, persistence.ErrGoalStale) {
		h.respondGoal(w, r, "", goalStaleMessage, "")
		return
	}
	if errors.Is(err, persistence.ErrNotFound) {
		h.respondGoal(w, r, "Primary Goal cleared.", "", "")
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	message := "Primary Goal cleared."
	h.respondGoal(w, r, message, "", "")
}
