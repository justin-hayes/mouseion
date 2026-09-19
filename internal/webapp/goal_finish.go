package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/justin-hayes/mouseion/internal/persistence"
)

type primaryGoalFinisher interface {
	RecordReadingFinishedPrimaryGoal(context.Context, string, string, string) (persistence.ReadingFinishResult, error)
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
	BookTitle                string
	SnapshotVocabularyCount  int
	EligibleVocabularyCount  int
	GraduatedVocabularyCount int
	AlreadyKnownCount        int
	Evidence                 []finishEvidenceView
	Journey                  journeyPageView
	Error                    string
}

func (h *Handler) finishPrimaryGoal(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	finisher, ok := h.services.Store.Goals.(primaryGoalFinisher)
	if !ok {
		h.respondGoal(w, r, "", "Reading finish is not available. No changes were made; review Reading Journey and try again.", "")
		return
	}
	owner := user(r).ID
	expectedBookID := strings.TrimSpace(r.FormValue("expected_goal_book_id"))
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" {
		h.respondGoal(w, r, "", goalLanguageRequiredMessage, "")
		return
	}
	if expectedBookID == "" {
		h.respondGoal(w, r, "", "No Primary Goal is available to finish. Review Reading Journey before trying again.", "")
		return
	}

	before, err := h.buildJourneyView(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	result, err := finisher.RecordReadingFinishedPrimaryGoal(r.Context(), owner, language, expectedBookID)
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

	after, afterErr := h.buildJourneyView(r.Context(), owner, language)
	outcome := primaryGoalFinishView{
		BookTitle:                h.finishBookTitle(r.Context(), owner, before, result.Completion.BookID),
		SnapshotVocabularyCount:  result.Completion.SnapshotVocabularyCount,
		EligibleVocabularyCount:  result.Completion.EligibleVocabularyCount,
		GraduatedVocabularyCount: result.Completion.GraduatedVocabularyCount,
		AlreadyKnownCount:        result.Completion.AlreadyKnownVocabularyCount,
		Journey:                  after,
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

func (h *Handler) finishBookTitle(ctx context.Context, owner string, before journeyPageView, bookID string) string {
	if before.Goal != nil && journeyBookID(*before.Goal) == bookID {
		return canonicalBookTitle(before.Goal.Book)
	}
	return h.goalBookTitle(ctx, owner, bookID)
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
	return fmt.Sprintf("%d frozen Reserved identities were currently eligible to become Known vocabulary; %d newly accepted identities were added. %d identities were already Known. This is a modeled vocabulary consequence, not verified per-card mastery.", outcome.EligibleVocabularyCount, outcome.GraduatedVocabularyCount, outcome.AlreadyKnownCount)
}

func goalCompletionConfirmationText(item journeyBookView) string {
	return fmt.Sprintf("Record the reading achievement and accept %d currently eligible frozen Reserved identities into Known vocabulary. This is a modeled vocabulary consequence, not verified per-card mastery.", item.GoalVocabularyEligible)
}

func finishOutcomeWhereNextURL(item journeyBookView) string {
	return "/goal/books/" + url.PathEscape(journeyBookID(item))
}
