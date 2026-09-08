package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

// goalResidualView describes reservation state that remains independent of the
// learner's current Primary Goal role.
type goalResidualView struct {
	CampaignID    string
	BookTitle     string
	ReservedCount *int
}

// goalSectionView is the server-truth fragment returned after an HTMX Goal
// mutation.
type goalSectionView struct {
	Goal     *journeyBookView
	Residual *goalResidualView
	Message  string
	Error    string
}

const (
	goalStaleMessage            = "This Primary Goal changed since this page was loaded. No changes were made; review Reading Journey before trying again."
	goalConcurrentMessage       = "Another book became your Primary Goal while you were choosing. No changes were made; review Reading Journey before trying again."
	goalUnavailableMessage      = "This book is not available in My Books."
	goalIneligibleMessage       = "This book must be an active Reading Journey member with a successfully completed current analysis before it can become a Primary Goal."
	goalLanguageRequiredMessage = "Choose a study language before setting a Primary Goal."
)

func goalBookHasDeck(bookID string, campaigns []domain.LearningCampaign, preparations []domain.DeckPreparation) bool {
	for _, campaign := range campaigns {
		if campaign.SourceMaterialID == bookID {
			return true
		}
	}
	for _, preparation := range preparations {
		if preparation.SourceMaterialID == bookID {
			return true
		}
	}
	return false
}

func (h *Handler) goalResidual(ctx context.Context, owner, bookID, title string, campaigns []domain.LearningCampaign) *goalResidualView {
	for _, campaign := range campaigns {
		if campaign.OwnerID != "" && campaign.OwnerID != owner {
			continue
		}
		if campaign.SourceMaterialID != bookID || campaign.Status != domain.CampaignActive {
			continue
		}
		residual := &goalResidualView{CampaignID: campaign.ID, BookTitle: title}
		if counter, ok := h.services.Store.(campaignVocabularyCounter); ok {
			if count, err := counter.CountCampaignVocabularyToGraduate(ctx, owner, campaign.ID); err == nil {
				residual.ReservedCount = &count
			}
		}
		return residual
	}
	return nil
}

func (h *Handler) currentGoalResidual(ctx context.Context, owner, bookID string) *goalResidualView {
	if bookID == "" {
		return nil
	}
	campaigns, err := h.services.Store.ListLearningCampaigns(ctx, owner)
	if err != nil {
		return nil
	}
	title := h.goalBookTitle(ctx, owner, bookID)
	return h.goalResidual(ctx, owner, h.goalSourceMaterialID(ctx, owner, bookID), title, campaigns)
}

func (h *Handler) goalSourceMaterialID(ctx context.Context, owner, bookID string) string {
	if reader, ok := h.services.Store.(interface {
		ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error)
	}); ok {
		if books, err := reader.ListMyBooksWithEvidence(ctx, owner); err == nil {
			for _, book := range books {
				if book.Book.ID == bookID && book.Acquired != nil {
					return book.Acquired.Source.ID
				}
			}
		}
	}
	return bookID
}

func (h *Handler) goalBookTitle(ctx context.Context, owner, bookID string) string {
	books, err := h.services.Store.ListSourceMaterials(ctx, owner)
	if err == nil {
		for _, book := range books {
			if book.Source.ID == bookID {
				return journeyBookTitle(book)
			}
		}
	}
	if book, err := h.services.Store.GetBook(ctx, owner, bookID); err == nil && strings.TrimSpace(book.Title) != "" {
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
	return goalSectionView{Goal: journey.Goal, Residual: journey.Residual, Message: message, Error: pageError}, nil
}

func (h *Handler) respondGoal(w http.ResponseWriter, r *http.Request, message, pageError, focusBookID string) {
	if isHTMX(r) {
		section, err := h.goalSection(r.Context(), user(r).ID, message, pageError)
		if err != nil {
			fail(w, err)
			return
		}
		render(w, r, GoalSection(section.Goal, section.Residual, h.csrf(w, r), section.Message, section.Error, focusBookID))
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
		if _, bookErr := h.services.Store.GetBook(r.Context(), owner, bookID); errors.Is(bookErr, persistence.ErrNotFound) {
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
	current, err := h.services.Store.GetPrimaryGoal(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	title := h.goalBookTitle(r.Context(), owner, bookID)
	currentActive := primaryGoalIsActive(current)
	if currentActive && current.BookID == bookID {
		h.respondGoal(w, r, title+" is already your Primary Goal.", "", bookID)
		return
	}
	if currentActive && current.BookID != expectedBookID {
		h.respondGoal(w, r, "", goalStaleMessage, current.BookID)
		return
	}
	previousResidual := h.currentGoalResidual(r.Context(), owner, current.BookID)
	if !currentActive {
		if current.ReadingFinishedAt != nil && expectedBookID != "" && expectedBookID != current.BookID {
			h.respondGoal(w, r, "", goalStaleMessage, "")
			return
		}
		if expectedBookID != "" {
			h.respondGoal(w, r, "", goalStaleMessage, "")
			return
		}
		_, err = h.services.Store.CreatePrimaryGoal(r.Context(), owner, language, bookID)
		if errors.Is(err, persistence.ErrGoalExists) {
			latest, readErr := h.services.Store.GetPrimaryGoal(r.Context(), owner, language)
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
		_, err = h.services.Store.ChangePrimaryGoal(r.Context(), owner, language, bookID, expectedBookID)
		if errors.Is(err, persistence.ErrGoalStale) {
			h.respondGoal(w, r, "", goalStaleMessage, current.BookID)
			return
		}
		if errors.Is(err, persistence.ErrNotFound) {
			if _, bookErr := h.services.Store.GetBook(r.Context(), owner, bookID); errors.Is(bookErr, persistence.ErrNotFound) {
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
	message := title + " is your Primary Goal."
	if previousResidual != nil && currentActive {
		message += " Its active campaign and reserved vocabulary are unchanged; resolve them from Campaign history & operations."
	}
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
	current, err := h.services.Store.GetPrimaryGoal(r.Context(), owner, language)
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
	residual := h.currentGoalResidual(r.Context(), owner, current.BookID)
	err = h.services.Store.ClearPrimaryGoal(r.Context(), owner, language, expectedBookID)
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
	if residual != nil {
		message += " Its active campaign and reserved vocabulary are unchanged until you graduate or abandon them."
	}
	h.respondGoal(w, r, message, "", "")
}
