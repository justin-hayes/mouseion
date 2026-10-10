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
	"github.com/justin-hayes/mouseion/internal/domain"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func (h *Handler) jobs(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	jobs, e := h.services.Store.Jobs.ListAnalysisJobs(r.Context(), u.ID)
	if e != nil {
		fail(w, e)
		return
	}
	jobBookContexts, err := h.jobBookContexts(r.Context(), u.ID, analysisJobSourceIDs(jobs))
	if err != nil {
		fail(w, err)
		return
	}
	syncJobs, syncErr := h.services.CatalogueSync.List(r.Context(), u.ID)
	if syncErr != nil {
		fail(w, syncErr)
		return
	}
	render(w, r, JobsPageWithCatalogueSync(u, h.csrf(w, r), jobs, syncJobs, r.URL.Query().Get("message"), jobBookContexts))
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

func jobRunning(status analysis.Status) bool {
	if status.LogicalState != "" {
		return status.LogicalState == "queued" || status.LogicalState == "running"
	}
	return status.State == "available" || status.State == "pending" || status.State == "running" || status.State == "retryable" || status.State == "scheduled"
}
func jobState(status analysis.Status) string {
	if status.LogicalState != "" {
		switch status.LogicalState {
		case "completed":
			return "Completed"
		case "failed":
			return "Failed"
		case "cancelled":
			return "Cancelled"
		case "running":
			return "Running"
		default:
			return "Queued"
		}
	}
	//nolint:exhaustive // River's JobState is an open upstream enumeration; unknown states retain their provider label.
	switch status.State {
	case "completed":
		return "Succeeded"
	case "discarded", "cancelled":
		return "Failed"
	case "running":
		return "Running"
	default:
		return cases.Title(language.Und).String(string(status.State))
	}
}

func analysisJobStatusLabel(state string) string {
	switch state {
	case "completed":
		return "Completed"
	case "failed", "analysis failed":
		return "Failed"
	case "cancelled", "analysis cancelled":
		return "Cancelled"
	case "running", "processing":
		return "Processing"
	case "queued", "pending", "available", "scheduled", "retryable":
		return "Queued"
	default:
		return cases.Title(language.Und).String(state)
	}
}

func analysisJobStatusTone(state string) StatusTone {
	switch analysisJobStatusLabel(state) {
	case "Completed":
		return StatusSuccess
	case "Failed", "Cancelled":
		return StatusDanger
	case "Processing", "Queued":
		return StatusInfo
	default:
		return StatusNeutral
	}
}
func analysisStatusSummary(status analysis.Status) string {
	switch status.LogicalState {
	case "completed":
		return "Analysis complete. Open the exact result to review its insights."
	case "finalization-failed":
		return "The analysis corpus is complete, but finalization failed. Its corpus was retained; retry to finish selection and publication without rerunning NLP."
	case "finalization-cancelled":
		return "Analysis finalization was cancelled. The completed corpus was retained; retry when you are ready."
	case "finalizing":
		return "The analysis corpus is complete; selection and publication are still in progress."
	case "failed":
		return "Analysis failed. Review the message and retry the EPUB snapshot when you are ready."
	case "cancelled":
		return "Analysis cancelled. Retry the EPUB snapshot when you are ready."
	}
	attempt := maxOne(status.Attempt)
	return fmt.Sprintf("%d%% complete · attempt %d", status.Progress, attempt)
}
func jobRetryable(status analysis.Status) bool {
	return status.LogicalState == "failed" || status.LogicalState == "cancelled" || status.LogicalState == "finalization-failed" || status.LogicalState == "finalization-cancelled"
}
func analysisJobSourceIDs(jobs []domain.AnalysisJob) []string {
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.SourceMaterialID)
	}
	return ids
}
func catalogueSyncJobState(status cataloguesync.Status) string {
	switch status.LogicalState {
	case "completed":
		return "Completed"
	case "failed":
		return "Failed"
	case "cancelled":
		return "Cancelled"
	case "running":
		return "Running"
	default:
		return "Queued"
	}
}

func catalogueSyncJobSummary(status cataloguesync.Status) string {
	if status.LogicalState == "completed" {
		return "Catalog metadata sync complete. No EPUB content was downloaded."
	}
	if status.Error != "" {
		return status.Error
	}
	return fmt.Sprintf("%d%% complete · attempt %d", status.Progress, maxOne(int(status.Attempt)))
}
