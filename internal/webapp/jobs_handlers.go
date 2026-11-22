package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func (h *Handler) jobs(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	preparations, err := h.services.Store.Jobs.ListBookPreparationJobs(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	syncJobs, syncErr := h.services.CatalogueSync.List(r.Context(), u.ID)
	if syncErr != nil {
		fail(w, syncErr)
		return
	}
	catalogCode := ""
	legacy, legacyErr := h.services.Store.Catalogs.ListLegacyLanguageRows(r.Context(), classical.Language)
	if legacyErr != nil {
		fail(w, legacyErr)
		return
	}
	if len(legacy) > 0 {
		catalogCode = classical.LegacyRowsCode
	}
	render(w, r, JobsPage(u, h.csrf(w, r), preparations, syncJobs, catalogCode, r.URL.Query().Get("message")))
}
func (h *Handler) job(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	if status, found, ok := h.loadCatalogueJob(r.Context(), u.ID, r.PathValue("id")); ok {
		if found {
			render(w, r, CatalogueSyncJobPage(u, h.csrf(w, r), status))
		}
		return
	}
	status, ok := h.loadJob(w, r, u.ID)
	if !ok {
		return
	}
	book, err := h.jobBookContextForSource(r.Context(), u.ID, status.SourceMaterialID)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, JobPage(u, h.csrf(w, r), status, book.ReadingURL, book.Title))
}
func (h *Handler) jobStatus(w http.ResponseWriter, r *http.Request) {
	if status, found, ok := h.loadCatalogueJob(r.Context(), user(r).ID, r.PathValue("id")); ok {
		if found {
			render(w, r, CatalogueSyncJobStatus(h.csrf(w, r), status))
		}
		return
	}
	status, ok := h.loadJob(w, r, user(r).ID)
	if !ok {
		return
	}
	book, err := h.jobBookContextForSource(r.Context(), user(r).ID, status.SourceMaterialID)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, JobStatus(h.csrf(w, r), status, book.ReadingURL, book.Title))
}
func (h *Handler) loadJob(w http.ResponseWriter, r *http.Request, owner string) (analysis.Status, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return analysis.Status{}, false
	}
	if _, reconcileErr := h.services.Analysis.Reconcile(r.Context(), owner, id); reconcileErr != nil && !errors.Is(reconcileErr, analysis.ErrNotFound) {
		log.Printf("mouseion: reconcile analysis job status: %v", reconcileErr)
	}
	status, err := h.services.Analysis.Get(r.Context(), owner, id)
	if errors.Is(err, analysis.ErrNotFound) {
		http.NotFound(w, r)
		return analysis.Status{}, false
	}
	if err != nil {
		fail(w, err)
		return analysis.Status{}, false
	}
	return status, true
}

func (h *Handler) retryJob(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, getErr := h.services.CatalogueSync.Get(r.Context(), user(r).ID, id); getErr == nil {
		handle, retryErr := h.services.CatalogueSync.Retry(r.Context(), user(r).ID, id)
		if retryErr != nil {
			if errors.Is(retryErr, cataloguesync.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			redirect(w, r, "/jobs/"+r.PathValue("id")+"?error="+url.QueryEscape("This catalog sync is not available for retry."))
			return
		}
		redirect(w, r, fmt.Sprintf("/jobs/%d?message=%s", handle.ID, url.QueryEscape("Catalog sync retry submitted.")))
		return
	}
	if _, err = h.services.Analysis.Retry(r.Context(), user(r).ID, id); err != nil {
		if errors.Is(err, analysis.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		redirect(w, r, "/jobs/"+r.PathValue("id")+"?error="+url.QueryEscape("This analysis is not available for retry."))
		return
	}
	redirect(w, r, "/jobs/"+r.PathValue("id")+"?message="+url.QueryEscape("Analysis retry submitted."))
}

func (h *Handler) cancelJob(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRFAnyLanguage(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, getErr := h.services.CatalogueSync.Get(r.Context(), user(r).ID, id); getErr == nil {
		if _, cancelErr := h.services.CatalogueSync.Cancel(r.Context(), user(r).ID, id); cancelErr != nil {
			if errors.Is(cancelErr, cataloguesync.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			redirect(w, r, "/jobs/"+r.PathValue("id")+"?error="+url.QueryEscape("This catalog sync could not be cancelled."))
			return
		}
		redirect(w, r, "/jobs/"+r.PathValue("id")+"?message="+url.QueryEscape("Catalog sync cancelled."))
		return
	}
	if _, err = h.services.Analysis.Cancel(r.Context(), user(r).ID, id); err != nil {
		if errors.Is(err, analysis.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		redirect(w, r, "/jobs/"+r.PathValue("id")+"?error="+url.QueryEscape("This analysis could not be cancelled."))
		return
	}
	redirect(w, r, "/jobs/"+r.PathValue("id")+"?message="+url.QueryEscape("Analysis cancelled."))
}

func (h *Handler) loadCatalogueJob(ctx context.Context, owner, rawID string) (cataloguesync.Status, bool, bool) {
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		return cataloguesync.Status{}, false, false
	}
	status, err := h.services.CatalogueSync.Get(ctx, owner, id)
	if err != nil {
		return cataloguesync.Status{}, false, false
	}
	return status, true, true
}

// jobLifecycleView is the one learner-facing label and tone mapping from the
// shared lifecycle state. Every job kind on /jobs renders through it.
type jobLifecycleView struct {
	Label string
	Tone  StatusTone
}

// bookPreparationKindLabel is the learner-facing name of one Book preparation
// kind. An analysis row shows its number beside the label where it has one.
func bookPreparationKindLabel(kind domain.BookPreparationKind) string {
	switch kind {
	case domain.BookPreparationKindClassical:
		return "Classical preparation"
	case domain.BookPreparationKindEPUBAcquisition:
		return "EPUB acquisition"
	case domain.BookPreparationKindAnalysis:
		return "Analysis"
	default:
		return "Book preparation"
	}
}

func jobLifecycleViewFor(state domain.JobLifecycleState) jobLifecycleView {
	switch state {
	case domain.JobLifecycleQueued:
		return jobLifecycleView{Label: "Queued", Tone: StatusInfo}
	case domain.JobLifecycleRunning:
		return jobLifecycleView{Label: "Running", Tone: StatusInfo}
	case domain.JobLifecycleCompleted:
		return jobLifecycleView{Label: "Completed", Tone: StatusSuccess}
	case domain.JobLifecycleFailed:
		return jobLifecycleView{Label: "Failed", Tone: StatusDanger}
	case domain.JobLifecycleCancelled:
		return jobLifecycleView{Label: "Cancelled", Tone: StatusDanger}
	default:
		return jobLifecycleView{Label: "Queued", Tone: StatusInfo}
	}
}

// jobRunning reports whether the analysis detail page should poll and offer
// Cancel. Finalization keeps today's action set: it is shown as Running but is
// left to settle on its own.
func jobRunning(status analysis.Status) bool {
	return status.Lifecycle.Active() && status.Stage != domain.BookPreparationStageFinalize
}

const analysisCompleteSummary = "Analysis complete. Open the exact result to review its insights."

func analysisStatusSummary(status analysis.Status) string {
	if status.Stage == domain.BookPreparationStageFinalize {
		switch status.Lifecycle {
		case domain.JobLifecycleCompleted:
			return analysisCompleteSummary
		case domain.JobLifecycleFailed:
			return "The analysis corpus is complete, but finalization failed. Its corpus was retained; retry to finish selection and publication without rerunning NLP."
		case domain.JobLifecycleCancelled:
			return "Analysis finalization was cancelled. The completed corpus was retained; retry when you are ready."
		case domain.JobLifecycleQueued, domain.JobLifecycleRunning:
			return "The analysis corpus is complete; selection and publication are still in progress."
		}
	}
	switch status.Lifecycle {
	case domain.JobLifecycleFailed:
		return "Analysis failed. Review the message and retry the EPUB snapshot when you are ready."
	case domain.JobLifecycleCancelled:
		return "Analysis cancelled. Retry the EPUB snapshot when you are ready."
	case domain.JobLifecycleCompleted:
		return analysisCompleteSummary
	case domain.JobLifecycleQueued, domain.JobLifecycleRunning:
		// In-progress analyses report their progress below.
	}
	attempt := maxOne(status.Attempt)
	return fmt.Sprintf("%d%% complete · attempt %d", status.Progress, attempt)
}

// analysisFailureSentence derives the learner-facing sentence from the stable
// failure code. An empty code is a run that failed before codes were recorded;
// it still reads as a completed attempt with a sensible generic sentence.
func analysisFailureSentence(code domain.BookPreparationCode) string {
	switch code {
	case domain.BookPreparationCodeDependencyParsingUnavailable:
		return "Analysis failed because dependency parsing is unavailable."
	case domain.BookPreparationCodeAnalysisInterrupted:
		return "The analysis was interrupted."
	default:
		return "Analysis failed."
	}
}

// analysisFailureReason is the one-sentence reason a code adds after a status
// label. It is empty for a generic failure.
func analysisFailureReason(code domain.BookPreparationCode) string {
	switch code {
	case domain.BookPreparationCodeDependencyParsingUnavailable:
		return "Dependency parsing is unavailable."
	case domain.BookPreparationCodeAnalysisInterrupted:
		return "The analysis was interrupted."
	default:
		return ""
	}
}

// bookPreparationFailureReason is the learner-readable reason a failed row
// adds under its stable code. It is the reason My Books already explains for
// the same failure: the analysis sentence for an analysis run, and the shared
// classical or EPUB acquisition label for the preparation kinds, reusing the
// domain's own stage-to-state classification so the wording cannot drift. An
// unrecognised stage or kind adds no reason rather than a new one.
func bookPreparationFailureReason(job domain.BookPreparationJob) string {
	if job.Lifecycle != domain.JobLifecycleFailed {
		return ""
	}
	signals := domain.AnalysisSignals{}
	switch job.Kind {
	case domain.BookPreparationKindClassical:
		signals.ClassicalPreparation = &domain.ClassicalPreparationStatus{Stage: job.Stage, Code: job.Code}
	case domain.BookPreparationKindEPUBAcquisition:
		signals.EPUBAcquisition = &domain.EPUBAcquisitionStatus{Stage: job.Stage, Code: job.Code}
	case domain.BookPreparationKindAnalysis:
		return analysisFailureSentence(job.Code)
	default:
		return ""
	}
	classification := domain.ClassifyBookEvidence(signals, "", "")
	if view, ok := classicalPreparationForClassification(classification); ok {
		return view.Label
	}
	if classification.Acquisition.Applies() {
		return epubAcquisitionViewFor(classification).Label
	}
	return ""
}

// analysisNeedsAttention reports a failed analysis detail, including a
// completed run whose finalization failed and a run that failed before codes
// were recorded.
func analysisNeedsAttention(status analysis.Status) bool {
	return status.FailureCode != domain.BookPreparationCodeNone || status.Error != "" ||
		status.Lifecycle == domain.JobLifecycleFailed
}

func jobRetryable(status analysis.Status) bool {
	return status.Lifecycle.Retryable()
}

func catalogueSyncJobSummary(status cataloguesync.Status) string {
	if status.Lifecycle == domain.JobLifecycleCompleted {
		return "Catalog metadata sync complete. No EPUB content was downloaded."
	}
	if status.Error != "" {
		return status.Error
	}
	return fmt.Sprintf("%d%% complete · attempt %d", status.Progress, maxOne(int(status.Attempt)))
}
