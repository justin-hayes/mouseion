package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type primaryGoalFinisher interface {
	FinishReadingPrimaryGoal(context.Context, string, string) (persistence.PrimaryGoalFinishResult, error)
}

type finishEvidenceView struct {
	Book            journeyBookView
	BeforeLabel     string
	BeforeCurrent   string
	BeforeProjected string
	AfterLabel      string
	AfterCurrent    string
	AfterProjected  string
	Changed         bool
}

type primaryGoalFinishView struct {
	BookTitle          string
	Campaign           *domain.LearningCampaign
	Graduated          []domain.CampaignVocabulary
	ResidualVocabulary int
	Evidence           []finishEvidenceView
	Journey            journeyPageView
	Error              string
}

func (h *Handler) finishPrimaryGoal(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	finisher, ok := h.services.Store.(primaryGoalFinisher)
	if !ok {
		h.respondGoal(w, r, "", "Reading finish is not available. No changes were made; review Reading Journey and try again.", "")
		return
	}
	owner := user(r).ID
	expectedBookID := strings.TrimSpace(r.FormValue("expected_goal_book_id"))
	if expectedBookID == "" {
		h.respondGoal(w, r, "", "No Primary Goal is available to finish. Review Reading Journey before trying again.", "")
		return
	}

	before, err := h.buildJourneyView(r.Context(), owner)
	if err != nil {
		fail(w, err)
		return
	}
	result, err := finisher.FinishReadingPrimaryGoal(r.Context(), owner, expectedBookID)
	if errors.Is(err, persistence.ErrGoalStale) {
		h.respondGoal(w, r, "", goalStaleMessage, "")
		return
	}
	if errors.Is(err, persistence.ErrNotFound) {
		h.respondGoal(w, r, "", "No Primary Goal is available to finish. Review Reading Journey before trying again.", "")
		return
	}
	if err != nil {
		h.respondGoal(w, r, "", "Reading could not be marked finished. No changes were made; review Reading Journey and try again.", "")
		return
	}

	after, afterErr := h.buildJourneyView(r.Context(), owner)
	outcome := primaryGoalFinishView{
		BookTitle:          finishBookTitle(before, result.Goal.BookID),
		Campaign:           result.Campaign,
		Graduated:          result.Graduated,
		ResidualVocabulary: result.ResidualVocabularyCount,
		Journey:            after,
	}
	if afterErr != nil {
		outcome.Error = "Reading finished was saved, but the recalculated Journey evidence is temporarily unavailable. Return to Reading Journey and try again."
		outcome.Journey = journeyPageView{}
	} else {
		outcome.Evidence = finishEvidence(before, after)
	}

	if isHTMX(r) {
		render(w, r, PrimaryGoalFinish(outcome, h.csrf(w, r)))
		return
	}
	render(w, r, PrimaryGoalFinishPage(user(r), h.csrf(w, r), outcome))
}

func finishBookTitle(before journeyPageView, bookID string) string {
	if before.Goal != nil && journeyBookID(*before.Goal) == bookID {
		return journeyBookTitle(before.Goal.Book)
	}
	return bookID
}

func finishEvidence(before, after journeyPageView) []finishEvidenceView {
	beforeByID := make(map[string]journeyBookView, len(before.Provisional))
	for _, item := range before.Provisional {
		beforeByID[journeyBookID(item)] = item
	}
	evidence := make([]finishEvidenceView, 0, len(after.Provisional))
	for _, item := range after.Provisional {
		beforeItem, hadBefore := beforeByID[journeyBookID(item)]
		view := finishEvidenceView{
			Book:           item,
			AfterLabel:     journeyEvidenceLabel(item),
			AfterCurrent:   journeyCurrentCoverage(item),
			AfterProjected: journeyProjectedCoverage(item),
		}
		if hadBefore {
			view.BeforeLabel = journeyEvidenceLabel(beforeItem)
			view.BeforeCurrent = journeyCurrentCoverage(beforeItem)
			view.BeforeProjected = journeyProjectedCoverage(beforeItem)
			view.Changed = view.BeforeLabel != view.AfterLabel || view.BeforeCurrent != view.AfterCurrent || view.BeforeProjected != view.AfterProjected
		} else {
			view.BeforeLabel = "Not previously available"
			view.BeforeCurrent = "unavailable"
			view.BeforeProjected = "unavailable"
			view.Changed = true
		}
		evidence = append(evidence, view)
	}
	return evidence
}

func finishGraduationText(outcome primaryGoalFinishView) string {
	if outcome.Campaign != nil && outcome.Campaign.VocabularyGraduatedAt != nil {
		return fmt.Sprintf("The associated prepared-deck vocabulary work completed, and exactly %d eligible vocabulary identities were added to known vocabulary.", len(outcome.Graduated))
	}
	if outcome.Campaign != nil && outcome.ResidualVocabulary > 0 {
		return fmt.Sprintf("No vocabulary was added to known vocabulary. Vocabulary work remains: %d ungraduated identities remain reserved, pending confirmed deck review; any future effect is conditional.", outcome.ResidualVocabulary)
	}
	return "No vocabulary was added to known vocabulary. Reading finished is a reading record, not evidence of vocabulary knowledge."
}

func finishGraduatedIdentity(item domain.CampaignVocabulary) string {
	if item.UPOS == "" {
		return item.CanonicalLemma
	}
	return item.CanonicalLemma + " (" + item.UPOS + ")"
}

func finishOutcomeWhereNextURL(item journeyBookView) string {
	return "/goal/books/" + url.PathEscape(journeyBookID(item))
}
