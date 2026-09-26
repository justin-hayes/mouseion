package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/justin-hayes/mouseion/internal/persistence"
)

type primaryGoalFinisher interface {
	RecordReadingFinishedPrimaryGoal(context.Context, string, string, string, string) (persistence.ReadingFinishResult, error)
}

type primaryGoalFinishView struct {
	BookTitle                string
	GraduatedVocabularyCount int
	AlreadyKnownCount        int
}

func (h *Handler) finishPrimaryGoal(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	finisher, ok := h.services.Store.Goals.(primaryGoalFinisher)
	if !ok {
		h.respondGoal(w, r, "", "Reading finish is not available. No changes were made; review Reading and try again.", "")
		return
	}
	owner := user(r).ID
	expectedBookID := strings.TrimSpace(r.FormValue("expected_goal_book_id"))
	expectedSnapshotID := strings.TrimSpace(r.FormValue("expected_goal_snapshot_id"))
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" {
		h.respondGoal(w, r, "", goalLanguageRequiredMessage, "")
		return
	}
	if expectedBookID == "" {
		h.respondGoal(w, r, "", "No current reading is available to finish. Review Reading before trying again.", "")
		return
	}

	result, err := finisher.RecordReadingFinishedPrimaryGoal(r.Context(), owner, language, expectedBookID, expectedSnapshotID)
	if errors.Is(err, persistence.ErrGoalStale) {
		h.respondGoal(w, r, "", goalStaleMessage, "")
		return
	}
	if errors.Is(err, persistence.ErrNotFound) {
		h.respondGoal(w, r, "", "No current reading is available to finish. Review Reading before trying again.", "")
		return
	}
	if err != nil {
		h.respondGoal(w, r, "", "Reading could not be marked finished. No changes were made; review Reading and try again.", "")
		return
	}

	outcome := primaryGoalFinishView{
		BookTitle:                h.finishBookTitle(r.Context(), owner, result.Completion.BookID),
		GraduatedVocabularyCount: result.Completion.GraduatedVocabularyCount,
		AlreadyKnownCount:        result.Completion.AlreadyKnownVocabularyCount,
	}

	if isHTMX(r) {
		render(w, r, PrimaryGoalFinish(outcome))
		return
	}
	render(w, r, PrimaryGoalFinishPage(user(r), h.csrf(w, r), outcome))
}

func (h *Handler) finishBookTitle(ctx context.Context, owner, bookID string) string {
	book, err := h.services.Store.Books.GetBook(ctx, owner, bookID)
	if err == nil && strings.TrimSpace(book.Title) != "" {
		return book.Title
	}
	return bookID
}

func finishGraduationText(outcome primaryGoalFinishView) string {
	return fmt.Sprintf("Vocabulary: %d identities newly Known; %d identities already Known. These are modeled counts, not a mastery measure.", outcome.GraduatedVocabularyCount, outcome.AlreadyKnownCount)
}

func goalCompletionConfirmationText(item journeyBookView) string {
	return fmt.Sprintf("Record the reading achievement and accept %d currently eligible frozen Reserved identities into Known vocabulary. This is a modeled vocabulary consequence, not verified per-card mastery.", item.GoalVocabularyEligible)
}
