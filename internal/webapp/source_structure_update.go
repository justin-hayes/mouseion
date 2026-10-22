package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

const (
	sourceStructureUpdateNotice         = "A source structure update is available."
	sourceStructureUpdateChangedMessage = "The available source structure changed before you confirmed. Nothing was adopted or queued; review the refreshed offer and try again."
)

func readingSourceStructureUpdateURL(bookID string) string {
	return "/reading/books/" + url.PathEscape(bookID) + "/source-structure-update"
}

// annotateSourceStructureUpdates attaches checked Source structure update
// offers to acquired Books. A failed lookup offers nothing: a check is advisory
// and never becomes a page error.
func (h *Handler) annotateSourceStructureUpdates(ctx context.Context, owner string, books []domain.MyBook) {
	if h.services.SourceStructures == nil {
		return
	}
	sourceIDs := sourceIDsForBooks(books)
	if len(sourceIDs) == 0 {
		return
	}
	candidates, err := h.services.SourceStructures.SourceStructureUpdateCandidates(ctx, owner, sourceIDs)
	if err != nil {
		log.Printf("source structure update candidates unavailable: %v", err)
		return
	}
	applyBookCandidates(books, candidates, func(acquired *domain.SourceMaterialSummary, candidate domain.SourceStructureUpdateCandidate) {
		acquired.SourceStructureUpdate = &candidate
	})
}

// myBookSourceStructureUpdate returns the checked offer for a Book that may
// adopt it. A Book outside the To Read workflow is not offered a new analysis;
// the current reading keeps its persisted To Read disposition and is offered
// the update because its own evidence stays pinned.
func myBookSourceStructureUpdate(book domain.MyBook) *domain.SourceStructureUpdateCandidate {
	if book.Disposition != domain.BookDispositionToRead || book.Acquired == nil {
		return nil
	}
	return book.Acquired.SourceStructureUpdate
}

// sourceStructureUpdateConfirmationText states the old Reading/deck boundary
// and the confirmed decisions that do not carry over to the new analysis.
func sourceStructureUpdateConfirmationText(candidate domain.SourceStructureUpdateCandidate, currentReading bool) string {
	text := "Confirming re-extracts this Book's retained file, switches it to the new source structure and queues its re-analysis. The current analysis becomes stale immediately and is replaced when the new analysis publishes. " +
		sourceStructureUpdateDecisionsText(candidate.LemmaCorrections, candidate.OccurrenceExclusions)
	if currentReading {
		text += " Your active Reading keeps its exact current evidence, Reserved vocabulary and prepared deck; new evidence needs End, restart and re-preparation."
	} else {
		text += " Earlier Readings and prepared decks keep the evidence they froze."
	}
	return text + " There is no way back to the old structure."
}

func sourceStructureUpdateDecisionsText(corrections, exclusions int) string {
	var decisions []string
	if corrections > 0 {
		decisions = append(decisions, countNoun(corrections, "confirmed lemma correction", "confirmed lemma corrections"))
	}
	if exclusions > 0 {
		decisions = append(decisions, countNoun(exclusions, "occurrence exclusion", "occurrence exclusions"))
	}
	if len(decisions) == 0 {
		return "You have no confirmed lemma corrections or occurrence exclusions on the current analysis."
	}
	verb := "do"
	if corrections+exclusions == 1 {
		verb = "does"
	}
	return "Your " + strings.Join(decisions, " and ") + " on the current analysis " + verb + " not apply to the new analysis."
}

// updateBookSourceStructure adopts the checked Source structure update. The
// service revalidates the expected revision, snapshot, target and checked result
// under lock; a changed offer adopts nothing and returns the learner to the
// refreshed notice.
func (h *Handler) updateBookSourceStructure(w http.ResponseWriter, r *http.Request) {
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
	if h.services.SourceStructures == nil || detail.Acquired == nil {
		redirect(w, r, "/reading?error="+url.QueryEscape(sourceStructureUpdateChangedMessage))
		return
	}
	expected := domain.SourceStructureUpdateExpectation{
		ContentRevisionID:     strings.TrimSpace(r.FormValue("expected_content_revision_id")),
		SnapshotID:            strings.TrimSpace(r.FormValue("expected_snapshot_id")),
		ConventionSet:         strings.TrimSpace(r.FormValue("expected_convention_set")),
		ResultIdentity:        strings.TrimSpace(r.FormValue("expected_result_identity")),
		TargetContractVersion: formInt(r.FormValue("expected_contract_version")),
	}
	handle, err := h.services.SourceStructures.SubmitSourceStructureUpdate(r.Context(), owner, detail.Book.ID, detail.Acquired.Source.ID, expected)
	switch {
	case errors.Is(err, analysis.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, analysis.ErrSourceStructureUpdateChanged), errors.Is(err, analysis.ErrEPUBRequired), errors.Is(err, domain.ErrExtractedUnitsUnavailable):
		redirect(w, r, "/reading?error="+url.QueryEscape(sourceStructureUpdateChangedMessage))
	case err != nil:
		fail(w, err)
	default:
		redirect(w, r, "/reading?message="+url.QueryEscape(fmt.Sprintf("Source structure update for %s adopted as analysis job #%d. Earlier Readings and decks keep the evidence they froze.", detail.Book.Title, handle.DisplayNumber)))
	}
}

// formInt parses a small non-negative form value, treating anything else as
// zero so the service rejects the offer rather than adopting a guessed target.
func formInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}
