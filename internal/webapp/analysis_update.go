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
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
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
	sourceIDs := sourceIDsForBooks(books)
	if len(sourceIDs) == 0 {
		return
	}
	candidates, err := h.services.AnalysisUpdates.AnalysisUpdateCandidates(ctx, owner, sourceIDs)
	if err != nil {
		log.Printf("analysis update candidates unavailable: %v", err)
		return
	}
	applyBookCandidates(books, candidates, func(acquired *domain.SourceMaterialSummary, candidate domain.AnalysisUpdateCandidate) {
		acquired.AnalysisUpdate = &candidate
	})
}

// sourceIDsForBooks collects the acquired source identities an offer lookup may
// ask about, preserving the rows' order.
func sourceIDsForBooks(books []domain.MyBook) []string {
	sourceIDs := make([]string, 0, len(books))
	for _, book := range books {
		if book.Acquired != nil && book.Acquired.Source.ID != "" {
			sourceIDs = append(sourceIDs, book.Acquired.Source.ID)
		}
	}
	return sourceIDs
}

// applyBookCandidates attaches offered candidates to the acquired rows that have
// one. It is shared by the independent Analysis and Source structure offers.
func applyBookCandidates[T any](books []domain.MyBook, candidates map[string]T, apply func(*domain.SourceMaterialSummary, T)) {
	for i := range books {
		if books[i].Acquired == nil {
			continue
		}
		if candidate, ok := candidates[books[i].Acquired.Source.ID]; ok {
			apply(books[i].Acquired, candidate)
		}
	}
}

// analysisUpdateConfirmationText states what an Analysis update keeps and
// carries over (ADR 0126). A Classical target whose interpretation version has a
// change note leads with it.
func analysisUpdateConfirmationText(candidate domain.AnalysisUpdateCandidate) string {
	note, _ := analysisUpdateChangeNote(candidate.Target)
	return analysisUpdateConfirmationLines(note, candidate.LemmaCorrections, candidate.OccurrenceExclusions)
}

// analysisUpdateConfirmationLines joins the optional change note and the three
// lines every per-Book confirmation carries.
func analysisUpdateConfirmationLines(note string, corrections, exclusions int) string {
	lines := make([]string, 0, 4)
	if note != "" {
		lines = append(lines, note)
	}
	lines = append(lines,
		"The published analysis stays in effect until the update publishes.",
		analysisUpdateDecisionsText(corrections, exclusions),
		"Prepared decks and earlier Readings keep the evidence they froze.")
	return strings.Join(lines, " ")
}

// analysisUpdateChangeNote looks up the change note of a Classical target's
// interpretation version. An NLP target never has one.
func analysisUpdateChangeNote(target domain.AnalysisUpdateTarget) (string, bool) {
	if target.AnalyzerName != analysis.ClassicalUpdateAnalyzerName {
		return "", false
	}
	return classical.ChangeNote(target.AnalyzerVersion)
}

func analysisUpdateDecisionsText(corrections, exclusions int) string {
	decisions := analysisUpdateDecisionParts(corrections, exclusions)
	if len(decisions) == 0 {
		return "You have no confirmed lemma corrections or occurrence exclusions on it."
	}
	verb := "carry"
	if corrections+exclusions == 1 {
		verb = "carries"
	}
	return "Your " + strings.Join(decisions, " and ") + " " + verb + " over wherever the word is unchanged in the new analysis."
}

// analysisUpdateDecisionParts names the confirmed decisions one analysis
// holds, which carry over to a replacement. It is shared by the per-Book and
// per-Work confirmations, which differ only in their surrounding sentence.
func analysisUpdateDecisionParts(corrections, exclusions int) []string {
	var decisions []string
	if corrections > 0 {
		decisions = append(decisions, countNoun(corrections, "confirmed lemma correction", "confirmed lemma corrections"))
	}
	if exclusions > 0 {
		decisions = append(decisions, countNoun(exclusions, "occurrence exclusion", "occurrence exclusions"))
	}
	return decisions
}

func countNoun(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", count, plural)
}

// analysisUpdateOfferChanged reports submission failures that mean the offer
// the learner confirmed is no longer current. Nothing is queued and the learner
// reviews the refreshed offer.
func analysisUpdateOfferChanged(err error) bool {
	return errors.Is(err, analysis.ErrAnalysisUpdateChanged) ||
		errors.Is(err, analysis.ErrEPUBRequired) ||
		errors.Is(err, domain.ErrExtractedUnitsUnavailable)
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
	if errors.Is(err, domain.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	adoptDetailLanguage(r.Context(), detail)
	if h.services.AnalysisUpdates == nil || detail.Acquired == nil {
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape(analysisUpdateChangedMessage))
		return
	}
	expectedRunID := strings.TrimSpace(r.FormValue("expected_run_id"))
	expectedTarget := strings.TrimSpace(r.FormValue("expected_target"))
	handle, err := h.services.AnalysisUpdates.SubmitAnalysisUpdate(r.Context(), owner, detail.Book.ID, detail.Acquired.Source.ID, expectedRunID, expectedTarget)
	switch {
	case errors.Is(err, analysis.ErrCurrentBook):
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("End the Reading first. The current book keeps its frozen evidence; no analysis was queued."))
	case errors.Is(err, analysis.ErrNotFound):
		http.NotFound(w, r)
	case analysisUpdateOfferChanged(err):
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape(analysisUpdateChangedMessage))
	case err != nil:
		fail(w, err)
	default:
		redirect(w, r, readingPagePath(r.Context())+"?message="+url.QueryEscape(fmt.Sprintf("Analysis update for %s submitted as analysis job #%d. The published analysis stays in effect until the update publishes.", detail.Book.Title, handle.DisplayNumber)))
	}
}
