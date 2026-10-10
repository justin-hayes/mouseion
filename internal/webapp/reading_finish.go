package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/justin-hayes/mouseion/internal/persistence"
)

type currentReadingFinishView struct {
	BookTitle                string
	GraduatedVocabularyCount int
	AlreadyKnownCount        int
}

func (h *Handler) finishCurrentReading(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	expectedBookID := strings.TrimSpace(r.FormValue("expected_current_book_id"))
	expectedSnapshotID := strings.TrimSpace(r.FormValue("expected_current_snapshot_id"))
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" {
		h.respondCurrentReading(w, r, "", currentReadingLanguageRequiredMessage, "")
		return
	}
	if expectedBookID == "" {
		h.respondCurrentReading(w, r, "", "No current reading is available to finish. Review Reading before trying again.", "")
		return
	}

	result, err := h.services.Store.Reading.FinishCurrentReading(r.Context(), owner, language, expectedBookID, expectedSnapshotID)
	if errors.Is(err, persistence.ErrCurrentReadingStale) {
		h.respondCurrentReading(w, r, "", currentReadingStaleMessage, "")
		return
	}
	if errors.Is(err, persistence.ErrNotFound) {
		h.respondCurrentReading(w, r, "", "No current reading is available to finish. Review Reading before trying again.", "")
		return
	}
	if err != nil {
		h.respondCurrentReading(w, r, "", "Reading could not be marked finished. No changes were made; review Reading and try again.", "")
		return
	}

	outcome := currentReadingFinishView{
		BookTitle:                h.finishBookTitle(r.Context(), owner, result.Completion.BookID),
		GraduatedVocabularyCount: result.Completion.GraduatedVocabularyCount,
		AlreadyKnownCount:        result.Completion.AlreadyKnownVocabularyCount,
	}

	if isPartialHTMXRequest(r) {
		render(w, r, CurrentReadingFinish(outcome))
		return
	}
	render(w, r, CurrentReadingFinishPage(user(r), h.csrf(w, r), outcome))
}

func (h *Handler) finishBookTitle(ctx context.Context, owner, bookID string) string {
	book, err := h.services.Store.Reading.GetBook(ctx, owner, bookID)
	if err == nil && strings.TrimSpace(book.Title) != "" {
		return book.Title
	}
	return bookID
}

func finishGraduationText(outcome currentReadingFinishView) string {
	return fmt.Sprintf("Vocabulary: %d identities newly Known; %d identities already Known. These are modeled counts, not a mastery measure.", outcome.GraduatedVocabularyCount, outcome.AlreadyKnownCount)
}

func currentReadingCompletionConfirmationText(item readingBookView) string {
	return fmt.Sprintf("Record the reading achievement and accept %d currently eligible frozen Reserved identities into Known vocabulary. This is a modeled vocabulary consequence, not verified per-card mastery.", item.CurrentReadingVocabularyEligible)
}
