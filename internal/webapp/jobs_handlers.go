package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysis"
)

func (h *Handler) jobs(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	jobs, e := h.services.Store.ListAnalysisJobs(r.Context(), u.ID)
	if e != nil {
		fail(w, e)
		return
	}
	render(w, r, JobsPage(u, h.csrf(w, r), jobs, r.URL.Query().Get("message")))
}
func (h *Handler) job(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	status, ok := h.loadJob(w, r, u.ID)
	if !ok {
		return
	}
	render(w, r, JobPage(u, h.csrf(w, r), status))
}
func (h *Handler) jobStatus(w http.ResponseWriter, r *http.Request) {
	status, ok := h.loadJob(w, r, user(r).ID)
	if !ok {
		return
	}
	render(w, r, JobStatus(h.csrf(w, r), status))
}
func (h *Handler) loadJob(w http.ResponseWriter, r *http.Request, owner string) (analysis.Status, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return analysis.Status{}, false
	}
	if lifecycle, ok := h.services.Analysis.(interface {
		Reconcile(context.Context, string, int64) (analysis.Status, error)
	}); ok {
		if _, reconcileErr := lifecycle.Reconcile(r.Context(), owner, id); reconcileErr != nil && !errors.Is(reconcileErr, analysis.ErrNotFound) {
			// Status reads remain useful even if a best-effort reconciliation is unavailable.
		}
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
	if !h.checkCSRF(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	lifecycle, ok := h.services.Analysis.(interface {
		Retry(context.Context, string, int64) (analysis.Handle, error)
	})
	if !ok {
		http.NotFound(w, r)
		return
	}
	if _, err = lifecycle.Retry(r.Context(), user(r).ID, id); err != nil {
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
	if !h.checkCSRF(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	lifecycle, ok := h.services.Analysis.(interface {
		Cancel(context.Context, string, int64) (analysis.Status, error)
	})
	if !ok {
		http.NotFound(w, r)
		return
	}
	if _, err = lifecycle.Cancel(r.Context(), user(r).ID, id); err != nil {
		if errors.Is(err, analysis.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		redirect(w, r, "/jobs/"+r.PathValue("id")+"?error="+url.QueryEscape("This analysis could not be cancelled."))
		return
	}
	redirect(w, r, "/jobs/"+r.PathValue("id")+"?message="+url.QueryEscape("Analysis cancelled."))
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
	switch status.State {
	case "completed":
		return "Succeeded"
	case "discarded", "cancelled":
		return "Failed"
	case "running":
		return "Running"
	default:
		return strings.Title(string(status.State))
	}
}
func analysisStatusSummary(status analysis.Status) string {
	switch status.LogicalState {
	case "completed":
		return "Analysis complete. Open the exact result to review its insights."
	case "failed":
		return "Analysis failed. Review the message and retry the confirmed scope when it is still valid."
	case "cancelled":
		return "Analysis cancelled. Retry the confirmed scope when you are ready."
	}
	attempt := maxOne(status.Attempt)
	return fmt.Sprintf("%d%% complete · attempt %d", status.Progress, attempt)
}
func jobRetryable(status analysis.Status) bool {
	return status.LogicalState == "failed" || status.LogicalState == "cancelled"
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
func statusClass(status string) string { return strings.ReplaceAll(status, " ", "-") }
