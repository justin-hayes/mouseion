package webapp

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
	"github.com/justin-hayes/mouseion/internal/study"
)

// This file is the Lemma decision workflow the Wrong-word remediation screen
// uses: the preview step, the confirm step, how a decision's outcome is
// answered and the learner-facing copy of its rejections.

// lemmaDecisionPreview is what the preview step gathers: the Book, the form
// searched, its review and the previewed consequence.
type lemmaDecisionPreview struct {
	Detail  domain.MyBook
	Form    string
	Review  lemmareview.Review
	Preview lemmareview.Preview
}

// previewLemmaDecisionStep previews the decision the request names for the
// given occurrences. Nothing is saved.
func (h *Handler) previewLemmaDecisionStep(r *http.Request, owner, bookID string, occurrenceIDs []string) (lemmaDecisionPreview, error) {
	ctx := r.Context()
	detail, err := h.services.Store.Reading.GetBookDetail(ctx, owner, bookID)
	if err != nil {
		return lemmaDecisionPreview{}, err
	}
	adoptDetailLanguage(ctx, detail)
	language, _ := activeStudyLanguageForContext(ctx)
	preview, err := h.services.LemmaReview.Preview(ctx, owner, bookID, lemmaReviewProposal(r, language, occurrenceIDs))
	if err != nil {
		return lemmaDecisionPreview{}, err
	}
	form := strings.TrimSpace(r.FormValue("form"))
	review, err := h.services.LemmaReview.Review(ctx, owner, bookID, form)
	if err != nil {
		return lemmaDecisionPreview{}, err
	}
	return lemmaDecisionPreview{Detail: detail, Form: form, Review: review, Preview: preview}, nil
}

// confirmLemmaDecisionStep saves the decision the request names for the given
// occurrences, then ends the open Wrong-word reports it repaired.
func (h *Handler) confirmLemmaDecisionStep(r *http.Request, owner, bookID string, selected []string) error {
	ctx := r.Context()
	detail, err := h.services.Store.Reading.GetBookDetail(ctx, owner, bookID)
	if err != nil {
		return err
	}
	adoptDetailLanguage(ctx, detail)
	language, _ := activeStudyLanguageForContext(ctx)
	proposal := lemmaReviewProposal(r, language, selected)
	if err := h.services.LemmaReview.Confirm(ctx, owner, bookID, proposal, r.FormValue("fingerprint")); err != nil {
		return err
	}
	h.repairStudyReports(r, owner, bookID)
	return nil
}

// writeLemmaDecisionError answers a failed step: a missing Book is not found,
// a workflow rejection has its copy, anything else is a failure.
func writeLemmaDecisionError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	writeLemmaReviewError(w, err)
}

// repairStudyReports ends the open Wrong-word reports in the Book that the
// confirmed decision repaired, so their words return to Study (ADR 0135 §2). The
// decision is already saved; a failure here is logged, and Today's own
// reconciliation ends the report on the next visit.
func (h *Handler) repairStudyReports(r *http.Request, owner, bookID string) {
	if h.services.Store.Study == nil {
		return
	}
	if err := study.ReconcileBookReports(r.Context(), h.services.Store.Study, owner, bookID, h.now()); err != nil {
		log.Printf("mouseion: repair study reports after a lemma decision: %v", err)
	}
}

func lemmaReviewProposal(r *http.Request, language string, occurrenceIDs []string) lemmareview.Proposal {
	return lemmareview.Proposal{
		Language: language, Form: strings.TrimSpace(r.FormValue("form")), Action: r.FormValue("decision"),
		Lemma: r.FormValue("lemma"), OccurrenceIDs: occurrenceIDs,
	}
}

// lemmaReviewRejection is how the webapp answers one workflow rejection: the
// HTTP status and the learner-facing message.
type lemmaReviewRejection struct {
	sentinel error
	status   int
	message  string
}

// lemmaReviewRejections is the one place learner-facing lemma review copy lives.
// Every domain rejection needs an entry; the table test enforces it.
var lemmaReviewRejections = []lemmaReviewRejection{
	{domain.ErrLemmaReviewCurrentReading, http.StatusConflict, "Stop this Book's current reading before changing its vocabulary."},
	{domain.ErrLemmaReviewStale, http.StatusConflict, "Learner vocabulary state or this proposal changed after preview. No decision was saved; review it again."},
	{domain.ErrLemmaReviewCountsRefreshing, http.StatusConflict, "Vocabulary counts are being refreshed. No decision was saved; review it again shortly."},
	{domain.ErrLemmaReviewOccurrenceUnavailable, http.StatusBadRequest, "The selected occurrence changed. Find it again before reviewing."},
	{domain.ErrLemmaReviewOccurrenceMissing, http.StatusConflict, "That occurrence is no longer available in this analysis."},
	{domain.ErrLemmaReviewNoSelection, http.StatusBadRequest, "The preview contains no selected occurrences."},
	{domain.ErrLemmaReviewDecision, http.StatusBadRequest, "Choose a decision to preview."},
	{domain.ErrLemmaReviewLemma, http.StatusBadRequest, "Enter one valid canonical lemma without spaces."},
	{domain.ErrLemmaReviewUnsupportedLanguage, http.StatusBadRequest, "Choose a supported study language before correcting a lemma."},
}

// lemmaReviewRejectionFor returns the table entry for a workflow rejection.
func lemmaReviewRejectionFor(err error) (lemmaReviewRejection, bool) {
	for _, rejection := range lemmaReviewRejections {
		if errors.Is(err, rejection.sentinel) {
			return rejection, true
		}
	}
	return lemmaReviewRejection{}, false
}

// writeLemmaReviewError maps the workflow's rejections to statuses and learner
// copy. Anything else is an infrastructure failure.
func writeLemmaReviewError(w http.ResponseWriter, err error) {
	rejection, ok := lemmaReviewRejectionFor(err)
	if !ok {
		fail(w, err)
		return
	}
	http.Error(w, rejection.message, rejection.status)
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

// lemmaReviewReported reports whether a Study report is of the occurrence: the
// same place in the Book's unit when the report is located, else the same token
// of the same run.
func lemmaReviewReported(report domain.StudyReport, occurrence domain.LemmaReviewOccurrence) bool {
	source := report.Source
	if source.BookID != occurrence.BookID {
		return false
	}
	if source.UnitID != "" {
		return source.UnitID == occurrence.SourceDocumentID && source.StartOffset == occurrence.StartOffset && source.EndOffset == occurrence.EndOffset
	}
	return source.AnalysisRunID == occurrence.AnalysisRunID && source.SentenceOrdinal == occurrence.SentenceOrdinal && source.TokenOrdinal == occurrence.TokenOrdinal
}
