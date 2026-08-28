package webapp

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/a-h/templ"
	"github.com/justin-hayes/mouseion/internal/domain"
)

type StatusTone string

const (
	StatusNeutral StatusTone = "neutral"
	StatusInfo    StatusTone = "info"
	StatusSuccess StatusTone = "success"
	StatusWarning StatusTone = "warning"
	StatusDanger  StatusTone = "danger"
)

type FeedbackKind string

const (
	FeedbackInfo    FeedbackKind = "info"
	FeedbackSuccess FeedbackKind = "success"
	FeedbackWarning FeedbackKind = "warning"
	FeedbackError   FeedbackKind = "error"
)

type StatItem struct {
	Label  string
	Value  string
	Detail string
}

type MetadataItem struct {
	Term        string
	Description string
}

func statusBadgeClass(tone StatusTone) string {
	switch tone {
	case StatusInfo, StatusSuccess, StatusWarning, StatusDanger:
		return "status-badge status-badge--" + string(tone)
	default:
		return "status-badge status-badge--neutral"
	}
}

func confirmationClass(tone StatusTone) string {
	if tone == StatusDanger {
		return "confirmation confirmation--danger"
	}
	return "confirmation"
}

func statusTone(value string) StatusTone {
	switch value {
	case "analyzed", "active", "ready", "complete", "completed", "analysis result ready", "ready to analyze":
		return StatusSuccess
	case "analyzing", "queued", "running", "preparing", "analysis queued", "analysis running":
		return StatusInfo
	case "review_required", "degraded", "scope review required":
		return StatusWarning
	case "abandoned", "cancelled", "discarded", "failed", "analysis failed", "analysis cancelled", "analysis failed — action required":
		return StatusDanger
	default:
		return StatusNeutral
	}
}

type bookLifecycleAction struct {
	Status      string
	Description string
	Label       string
	URL         string
	Tone        StatusTone
	Submit      bool
}

func bookLifecycleActionFor(book domain.SourceMaterialSummary, history []domain.AnalysisJob) bookLifecycleAction {
	runID := book.AnalysisRunID
	if runID == "" {
		for _, job := range history {
			if job.SourceMaterialID == book.Source.ID && job.AnalysisState == "completed" && job.AnalysisRunID != "" {
				runID = job.AnalysisRunID
				break
			}
		}
	}

	state := strings.ToLower(strings.TrimSpace(book.AnalysisState))
	status := strings.ToLower(strings.TrimSpace(book.AnalysisStatus))
	if state == "" {
		switch {
		case status == "analyzed", status == "analysis result ready", status == "completed":
			state = "completed"
		case status == "analysis failed" || status == "analysis failed — action required" || status == "failed":
			state = "failed"
		case status == "analysis cancelled" || status == "cancelled":
			state = "cancelled"
		case status == "analysis queued" || status == "queued":
			state = "queued"
		case status == "analysis running" || status == "analyzing" || status == "running":
			state = "running"
		}
	}

	jobURL := ""
	if book.AnalysisJobID > 0 {
		jobURL = fmt.Sprintf("/jobs/%d", book.AnalysisJobID)
	}
	if jobURL == "" {
		jobURL = "/books/" + book.Source.ID
	}
	switch state {
	case "queued":
		return bookLifecycleAction{"Analysis queued", "The confirmed scope is waiting for analysis to begin.", "View analysis status", jobURL, StatusInfo, false}
	case "running":
		return bookLifecycleAction{"Analysis running", "The confirmed scope is being analyzed.", "View analysis status", jobURL, StatusInfo, false}
	case "failed":
		return bookLifecycleAction{"Analysis failed — action required", "The analysis needs attention before you can inspect a result.", "Review failed analysis", jobURL, StatusDanger, false}
	case "cancelled":
		return bookLifecycleAction{"Analysis cancelled", "The analysis was cancelled before producing a result.", "Review cancelled analysis", jobURL, StatusDanger, false}
	case "completed":
		if runID != "" {
			return bookLifecycleAction{"Analysis result ready", "Inspect the insights for this exact completed analysis.", "View analysis result", fmt.Sprintf("/books/%s/analyses/%s", book.Source.ID, runID), StatusSuccess, false}
		}
		if book.AnalysisJobID > 0 {
			return bookLifecycleAction{"Analysis result ready", "This historical analysis remains available through its operational record.", "View analysis history", jobURL, StatusSuccess, false}
		}
		return bookLifecycleAction{"Analysis result ready", "Review the available historical analysis details for this book.", "View analysis details", "/books/" + book.Source.ID, StatusSuccess, false}
	}

	if book.ConfirmedScopeID != "" || book.ReviewedScopeID != "" || status == "scope confirmed" {
		return bookLifecycleAction{"Ready to analyze", "A confirmed scope is ready. Start analysis when you are ready to spend analysis resources.", "Start analysis", "/books/" + book.Source.ID + "/analyze", StatusSuccess, true}
	}
	if book.Source.MediaType == "application/epub+zip" || status == "not analyzed" || status == "scope review required" || status == "" {
		return bookLifecycleAction{"Scope review required", "Choose the readable EPUB units before starting analysis.", "Review scope", scopeReviewActionURL(book), StatusWarning, false}
	}
	return bookLifecycleAction{"Ready to analyze", "This book is ready for an explicit analysis submission.", "Start analysis", "/books/" + book.Source.ID + "/analyze", StatusSuccess, true}
}

func scopeReviewActionURL(book domain.SourceMaterialSummary) string {
	base := "/books/" + book.Source.ID + "/scope"
	if book.ReviewedScopeID == "" {
		return base
	}
	return base + "?preset=prior&prior_scope_id=" + url.QueryEscape(book.ReviewedScopeID)
}

func analysisHistoryURL(sourceID string, job domain.AnalysisJob) string {
	if strings.TrimSpace(sourceID) == "" || job.SourceMaterialID != sourceID || job.ID <= 0 {
		return ""
	}
	if job.AnalysisState == "completed" && job.AnalysisRunID != "" && job.CorpusID != "" {
		return fmt.Sprintf("/books/%s/analyses/%s", sourceID, job.AnalysisRunID)
	}
	return fmt.Sprintf("/jobs/%d", job.ID)
}

func analysisHistoryLabel(job domain.AnalysisJob) string {
	if job.AnalysisState == "completed" && job.AnalysisRunID != "" && job.CorpusID != "" {
		return fmt.Sprintf("Completed analysis #%d", job.DisplayNumber)
	}
	return fmt.Sprintf("Analysis job #%d", job.DisplayNumber)
}

func feedbackClass(kind FeedbackKind) string {
	switch kind {
	case FeedbackSuccess, FeedbackWarning, FeedbackError:
		return "feedback feedback--" + string(kind)
	default:
		return "feedback feedback--info"
	}
}

func feedbackRole(kind FeedbackKind) string {
	if kind == FeedbackError {
		return "alert"
	}
	if kind == FeedbackWarning {
		return "note"
	}
	return "status"
}

func feedbackLive(kind FeedbackKind) string {
	if kind == FeedbackError {
		return "assertive"
	}
	return "polite"
}

func feedbackAttributes(kind FeedbackKind) templ.Attributes {
	if kind == FeedbackError {
		return templ.Attributes{"tabindex": "-1"}
	}
	return nil
}

func boolString(value bool) string { return fmt.Sprintf("%t", value) }

func jobStatusAttributes(id int64, running bool) templ.Attributes {
	if !running {
		return nil
	}
	return templ.Attributes{
		"hx-get":     fmt.Sprintf("/jobs/%d/status", id),
		"hx-trigger": "every 2s",
		"hx-swap":    "outerHTML",
	}
}

func textProfileStatItems(profile domain.TextProfile) []StatItem {
	return []StatItem{
		{Label: "sentences", Value: fmt.Sprintf("%d", profile.SentenceCount)},
		{Label: "median tokens per sentence", Value: fmt.Sprintf("%.1f", profile.MedianSentenceTokenCount)},
		{Label: "90th-percentile tokens", Value: fmt.Sprintf("%d", profile.P90SentenceTokenCount)},
		{Label: "long sentences (>35 tokens)", Value: fmt.Sprintf("%.1f%%", longSentencePercent(profile))},
	}
}

func coverageStatItems(coverage domain.AnalysisCoverage) []StatItem {
	return []StatItem{
		{Label: "current-known coverage", Value: fmt.Sprintf("%.1f%%", knownCoveragePercent(coverage))},
		{Label: "active-campaign projected coverage", Value: fmt.Sprintf("%.1f%%", activeCampaignCoveragePercent(coverage))},
		{Label: "analyzable tokens", Value: fmt.Sprintf("%d", coverage.AnalyzableTokenCount)},
		{Label: "distinct lemmas", Value: fmt.Sprintf("%d", coverage.DistinctLemmaCount)},
	}
}

func thresholdStatItems(thresholds []domain.CoverageThreshold) []StatItem {
	items := make([]StatItem, 0, len(thresholds))
	for _, threshold := range thresholds {
		if threshold.Reachable {
			items = append(items, StatItem{Label: fmt.Sprintf("lemmas for %d%%", threshold.TargetPercent), Value: fmt.Sprintf("%d", threshold.LemmaCount)})
			continue
		}
		items = append(items, StatItem{Label: fmt.Sprintf("%d%% cannot be reached with deck-eligible vocabulary", threshold.TargetPercent), Value: "Unavailable"})
	}
	return items
}

func projectionStatItems(projections []domain.CoverageProjection, analyzableTokenCount int64) []StatItem {
	items := make([]StatItem, 0, len(projections))
	for _, projection := range projections {
		items = append(items, StatItem{
			Label: fmt.Sprintf("after top %d lemmas", projection.TopLemmaCount),
			Value: fmt.Sprintf("%.1f%%", projectedCoveragePercent(projection, analyzableTokenCount)),
		})
	}
	return items
}
