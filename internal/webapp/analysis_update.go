package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

const (
	analysisUpdateNotice          = "A newer analysis is available."
	analysisUpdateEndReadingFirst = "End the Reading first to update this Book's analysis; the Reading keeps its frozen evidence."
	analysisUpdateChangedMessage  = "The available analysis changed before you confirmed. Nothing was queued; review the refreshed offer and try again."
)

func readingAnalysisUpdateURL(bookID string) string {
	return "/reading/books/" + url.PathEscape(bookID) + "/analysis-update"
}

// annotateAnalysisUpdates attaches Analysis update offers to acquired Books. A
// failed lookup offers nothing: an unreachable analysis service treats every
// Book as current and never becomes an error on the page (ADR 0107).
func (h *Handler) annotateAnalysisUpdates(ctx context.Context, owner string, books []domain.MyBook) {
	if h.services.AnalysisUpdates == nil {
		return
	}
	sourceIDs := make([]string, 0, len(books))
	for _, book := range books {
		if book.Acquired != nil && book.Acquired.Source.ID != "" {
			sourceIDs = append(sourceIDs, book.Acquired.Source.ID)
		}
	}
	if len(sourceIDs) == 0 {
		return
	}
	offers, err := h.services.AnalysisUpdates.AnalysisUpdateOffers(ctx, owner, sourceIDs)
	if err != nil {
		log.Printf("analysis update offers unavailable: %v", err)
		return
	}
	for i := range books {
		if books[i].Acquired == nil {
			continue
		}
		if candidate, ok := offers[books[i].Acquired.Source.ID]; ok {
			books[i].Acquired.AnalysisUpdate = &candidate
		}
	}
}

// analysisUpdateConfirmationText states what an Analysis update keeps and what
// it does not carry over.
func analysisUpdateConfirmationText(candidate domain.AnalysisUpdateCandidate) string {
	return "The published analysis stays in effect until the update publishes. " +
		analysisUpdateDecisionsText(candidate.LemmaCorrections, candidate.OccurrenceExclusions) +
		" Prepared decks and earlier Readings keep the evidence they froze."
}

func analysisUpdateDecisionsText(corrections, exclusions int) string {
	if corrections == 0 && exclusions == 0 {
		return "You have no confirmed lemma corrections or occurrence exclusions on it."
	}
	return fmt.Sprintf("Your %s and %s on it do not carry over to the new analysis.",
		countNoun(corrections, "confirmed lemma correction", "confirmed lemma corrections"),
		countNoun(exclusions, "occurrence exclusion", "occurrence exclusions"))
}

func countNoun(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", count, plural)
}

// myBookAnalysisUpdate returns the Book's offer and its candidate, which is
// nil whenever no update is offered.
func myBookAnalysisUpdate(book domain.MyBook) (domain.AnalysisUpdateOffer, *domain.AnalysisUpdateCandidate) {
	offer := book.Classification().Update
	if offer == domain.AnalysisUpdateNone || book.Acquired == nil || book.Acquired.AnalysisUpdate == nil {
		return domain.AnalysisUpdateNone, nil
	}
	return offer, book.Acquired.AnalysisUpdate
}

// updateToReadBookAnalysis queues the confirmed Analysis update. The service
// revalidates the confirmed published run and target under lock; a changed
// offer submits nothing and returns the learner to the refreshed offer.
func (h *Handler) updateToReadBookAnalysis(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := r.PathValue("id")
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if h.services.AnalysisUpdates == nil || detail.Acquired == nil {
		redirect(w, r, "/reading?error="+url.QueryEscape(analysisUpdateChangedMessage))
		return
	}
	expectedRunID := strings.TrimSpace(r.FormValue("expected_run_id"))
	expectedTarget := strings.TrimSpace(r.FormValue("expected_target"))
	handle, err := h.services.AnalysisUpdates.SubmitAnalysisUpdate(r.Context(), owner, detail.Book.ID, detail.Acquired.Source.ID, expectedRunID, expectedTarget)
	switch {
	case errors.Is(err, analysis.ErrCurrentBook):
		redirect(w, r, "/reading?error="+url.QueryEscape("End the Reading first. The current book keeps its frozen evidence; no analysis was queued."))
	case errors.Is(err, analysis.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, analysis.ErrAnalysisUpdateChanged), errors.Is(err, analysis.ErrEPUBRequired), errors.Is(err, domain.ErrExtractedUnitsUnavailable):
		redirect(w, r, "/reading?error="+url.QueryEscape(analysisUpdateChangedMessage))
	case err != nil:
		fail(w, err)
	default:
		redirect(w, r, "/reading?message="+url.QueryEscape(fmt.Sprintf("Analysis update #%d queued for %s. The published analysis stays in effect until it publishes.", handle.DisplayNumber, detail.Book.Title)))
	}
}
