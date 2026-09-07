package webapp

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichmentjob"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
)

func studyLanguagePresent(languages []domain.StudyLanguage, language string) bool {
	for _, candidate := range languages {
		if candidate.Language == language {
			return true
		}
	}
	return false
}

func studyLanguageName(languages []domain.StudyLanguage, language string) string {
	for _, candidate := range languages {
		if candidate.Language == language {
			return candidate.DisplayName
		}
	}
	return language
}

func vocabularyLanguageName(studyLanguages, knownLanguages []domain.StudyLanguage, language string) string {
	if name := studyLanguageName(studyLanguages, language); name != language {
		return name
	}
	return studyLanguageName(knownLanguages, language)
}

func knownVocabProvenance(entry domain.KnownVocabulary) string {
	if entry.Provenance != "" {
		return entry.Provenance
	}
	return "Explicitly recorded"
}

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

type catalogueSyncConnectionView struct {
	HasStatus       bool
	State           string
	Message         string
	Error           string
	LastSyncedAt    string
	LastSyncedAtISO string
	UpsertSummary   string
	Syncing         bool
	Failed          bool
	HasLastSyncedAt bool
}

func catalogueSyncConnectionViewFor(connection domain.OpdsConnection, statuses map[string]domain.CatalogueSyncStatus) catalogueSyncConnectionView {
	status, found := statuses[connection.ID]
	view := catalogueSyncConnectionView{HasStatus: found}
	if !found {
		view.State = "Never synced"
		view.Message = fmt.Sprintf("%s has never synced. Sync now reconciles ready non-English catalogue languages; it does not download EPUB content.", connection.Name)
		return view
	}
	switch status.State {
	case domain.CatalogueSyncSyncing:
		view.State = "Syncing"
		view.Syncing = true
		view.Message = fmt.Sprintf("%s is syncing metadata. Existing Books remain available while Mouseion reconciles bibliographic entries.", connection.Name)
	case domain.CatalogueSyncSynced:
		view.State = "Last synced"
		if status.LastSyncedAt != nil && !status.LastSyncedAt.IsZero() {
			view.HasLastSyncedAt = true
			view.LastSyncedAt = status.LastSyncedAt.Format("2006-01-02 15:04 UTC")
			view.LastSyncedAtISO = status.LastSyncedAt.Format(time.RFC3339)
		}
		if status.LastUpsertedCount == 0 {
			view.Message = "Sync completed, but no eligible EPUB entries were found; the library was unchanged."
		} else {
			view.UpsertSummary = fmt.Sprintf("%d books added or updated. Catalogue sync changes metadata only; it does not download EPUB content.", status.LastUpsertedCount)
		}
	case domain.CatalogueSyncFailed:
		view.State = "Sync failed"
		view.Failed = true
		view.Message = fmt.Sprintf("%s sync failed. Existing Books remain unchanged and available.", connection.Name)
		view.Error = status.LastError
	default:
		view.State = "Never synced"
		view.Message = fmt.Sprintf("%s has never synced. Sync now reconciles ready non-English catalogue languages; it does not download EPUB content.", connection.Name)
	}
	return view
}

// NavigationContext identifies the authenticated shell context. Acquisition
// remains a page context for catalog and connections workflows, but is not a
// primary shell destination.
type NavigationContext string

const (
	NavigationNone           NavigationContext = ""
	NavigationLibrary        NavigationContext = "library"
	NavigationReadingJourney NavigationContext = "reading-journey"
	NavigationLearning       NavigationContext = NavigationReadingJourney
	NavigationAcquisition    NavigationContext = "acquisition"
	NavigationVocabulary     NavigationContext = "vocabulary"
)

func navigationContextForTitle(title string) NavigationContext {
	switch {
	case title == "My Books", title == "My Library":
		return NavigationLibrary
	case title == "Reading Journey", title == "Learning":
		return NavigationReadingJourney
	case title == "Vocabulary", title == "Known vocabulary":
		return NavigationVocabulary
	case title == "Add books":
		return NavigationAcquisition
	default:
		return NavigationNone
	}
}

func destinationAttributes(context, destination NavigationContext) templ.Attributes {
	attributes := templ.Attributes{"class": "site-nav__link"}
	if context == destination {
		attributes["aria-current"] = "page"
		attributes["class"] = "site-nav__link site-nav__link--current"
	}
	return attributes
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
	case "review_required", "degraded":
		return StatusWarning
	case "unavailable":
		return StatusDanger
	case "not acquired":
		return StatusNeutral
	case "stale analysis":
		return StatusWarning
	case "abandoned", "cancelled", "discarded", "failed", "analysis failed", "analysis cancelled", "analysis failed — action required":
		return StatusDanger
	default:
		return StatusNeutral
	}
}

func myBookEvidenceStateFor(book domain.MyBook) domain.MyBookEvidenceState {
	if book.EvidenceState != "" {
		return book.EvidenceState
	}
	if book.Acquired == nil {
		return domain.MyBookNotAcquired
	}
	if book.Acquired.Source.ContentRevisionID == "" {
		return domain.MyBookUnavailable
	}
	if strings.EqualFold(book.Acquired.AnalysisStatus, "analyzed") {
		return domain.MyBookAnalyzed
	}
	return domain.MyBookAcquiredUnassessed
}

func legacyMyBooks(books []domain.SourceMaterialSummary) []domain.MyBook {
	out := make([]domain.MyBook, 0, len(books))
	for _, source := range books {
		state := domain.MyBookAcquiredUnassessed
		if strings.EqualFold(source.AnalysisStatus, "analyzed") {
			state = domain.MyBookAnalyzed
		}
		bookID := source.BookID
		if bookID == "" {
			bookID = source.Source.ID
		}
		out = append(out, domain.MyBook{Book: domain.Book{ID: bookID, OwnerID: source.Source.OwnerID, Title: source.Source.Title, LanguageState: domain.LanguageChosen, LanguageTag: source.Source.Language}, Acquired: &source, EvidenceState: state})
	}
	return out
}

func myBookEvidenceLabel(state domain.MyBookEvidenceState) string {
	switch state {
	case domain.MyBookUnavailable:
		return "Unavailable"
	case domain.MyBookNotAcquired:
		return "Not acquired"
	case domain.MyBookAcquiredUnassessed:
		return "Ready to analyze"
	case domain.MyBookAnalyzed:
		return "Analyzed"
	case domain.MyBookStale:
		return "Stale analysis"
	default:
		return "Evidence unavailable"
	}
}

func myBookAnalysisLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "not analyzed":
		return "No analysis run"
	case "scope confirmed":
		return "Ready to analyze"
	case "analyzed", "analysis result ready":
		return "Analysis complete"
	case "analysis failed":
		return "Analysis failed"
	case "analysis cancelled":
		return "Analysis cancelled"
	case "analyzing":
		return "Analysis in progress"
	default:
		return status
	}
}

func myBookLifecycleActionFor(book domain.MyBook) bookLifecycleAction {
	state := myBookEvidenceStateFor(book)
	if book.Acquired == nil || state == domain.MyBookUnavailable {
		if state == domain.MyBookNotAcquired {
			return bookLifecycleAction{
				Status:      "Not acquired",
				Description: "Acquire and analyze the EPUB in one action.",
				Label:       "Start analysis",
				URL:         "/books/" + url.PathEscape(book.Book.ID) + "/analyze",
				Tone:        StatusNeutral,
				Submit:      true,
			}
		}
		return bookLifecycleAction{
			Status:      myBookEvidenceLabel(state),
			Description: "No usable acquired EPUB evidence is available for assessment. Open the book to acquire content before using later actions.",
			Label:       "Open book",
			URL:         "/books/" + url.PathEscape(book.Book.ID),
			Tone:        statusTone(myBookEvidenceLabel(state)),
		}
	}
	if state == domain.MyBookStale {
		return bookLifecycleAction{Status: "Stale analysis", Description: "The current acquired content differs from the analyzed revision. Re-analyze the whole book to refresh your insights for the current content.", Label: "Start analysis", URL: "/books/" + url.PathEscape(book.Book.ID) + "/analyze", Tone: StatusWarning, Submit: true}
	}
	action := bookLifecycleActionFor(*book.Acquired, nil)
	if strings.HasPrefix(action.URL, "/books/") {
		action.URL = "/books/" + url.PathEscape(book.Book.ID) + strings.TrimPrefix(action.URL, "/books/"+url.PathEscape(book.Acquired.Source.ID))
	}
	return action
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
	_ = history // Operational history is displayed separately and never selects the current result.

	state := strings.ToLower(strings.TrimSpace(book.AnalysisState))
	status := strings.ToLower(strings.TrimSpace(book.AnalysisStatus))
	if state == "" {
		switch {
		case (status == "analyzed" || status == "analysis result ready" || status == "completed") && runID != "" && book.CorpusID != "":
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
		return bookLifecycleAction{"Analysis queued", "The EPUB snapshot is waiting for analysis to begin.", "View analysis status", jobURL, StatusInfo, false}
	case "running":
		return bookLifecycleAction{"Analysis running", "The EPUB snapshot is being analyzed.", "View analysis status", jobURL, StatusInfo, false}
	case "failed":
		return bookLifecycleAction{"Analysis failed — action required", "The analysis needs attention before you can inspect a result.", "Review failed analysis", jobURL, StatusDanger, false}
	case "cancelled":
		return bookLifecycleAction{"Analysis cancelled", "The analysis was cancelled before producing a result.", "Review cancelled analysis", jobURL, StatusDanger, false}
	case "completed":
		if runID != "" && book.CorpusID != "" {
			return bookLifecycleAction{"Analysis result ready", "Inspect the insights for this exact completed analysis.", "View analysis result", "/books/" + url.PathEscape(book.Source.ID), StatusSuccess, false}
		}
		state = ""
	}

	return bookLifecycleAction{"Ready to analyze", "This book is ready for an explicit analysis submission.", "Start analysis", "/books/" + url.PathEscape(book.Source.ID) + "/analyze", StatusSuccess, true}
}

func analysisHistoryURL(sourceID string, job domain.AnalysisJob) string {
	if strings.TrimSpace(sourceID) == "" || job.SourceMaterialID != sourceID || job.ID <= 0 {
		return ""
	}
	if job.AnalysisState == "completed" && job.AnalysisRunID != "" && job.CorpusID != "" {
		return "/books/" + url.PathEscape(sourceID)
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

func deckPreparationAttributes(preparation domain.DeckPreparation) templ.Attributes {
	attributes := templ.Attributes{"data-deck-preparation": "true", "data-workflow": "deck preparation"}
	if deckPreparationActive(preparation) {
		attributes["hx-get"] = "/deck-preparations/" + url.PathEscape(preparation.ID) + "/status"
		attributes["hx-trigger"] = "every 3s"
		attributes["hx-swap"] = "outerHTML"
	}
	return attributes
}

func deckPreparationActive(preparation domain.DeckPreparation) bool {
	return preparation.State == domain.DeckPreparationQueued || preparation.State == domain.DeckPreparationPreparing
}

func preparationProgress(preparation domain.DeckPreparation) int {
	progress := 0
	if preparation.State == domain.DeckPreparationPreparing {
		if preparation.TranslationEligible > 0 {
			progress = (preparation.TranslationDone + preparation.TranslationFailed) * 100 / preparation.TranslationEligible
		} else {
			progress = 50
		}
	} else if preparation.State == domain.DeckPreparationReady || preparation.State == domain.DeckPreparationFailed || preparation.State == domain.DeckPreparationCancelled {
		progress = 100
	}
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
}

func deckPreparationStatusLabel(state domain.DeckPreparationState) string {
	switch state {
	case domain.DeckPreparationQueued:
		return "Deck preparation queued"
	case domain.DeckPreparationPreparing:
		return "Deck preparation running"
	case domain.DeckPreparationReady:
		return "Deck ready"
	case domain.DeckPreparationFailed:
		return "Deck preparation failed"
	case domain.DeckPreparationCancelled:
		return "Deck preparation cancelled"
	default:
		return "Deck preparation"
	}
}

func deckPreparationTitle(preparation domain.DeckPreparation) string {
	if deckPreparationEmpty(preparation) {
		return "No recurring vocabulary"
	}
	return deckPreparationStatusLabel(preparation.State)
}

func deckPreparationStatusTone(preparation domain.DeckPreparation) StatusTone {
	if deckPreparationEmpty(preparation) {
		return StatusNeutral
	}
	return statusTone(string(preparation.State))
}

func deckPreparationSummary(preparation domain.DeckPreparation) string {
	if preparation.State == domain.DeckPreparationFailed {
		return "Preparation stopped and can be retried after reviewing the recovery message below."
	}
	if preparation.State == domain.DeckPreparationCancelled {
		return "Preparation was cancelled before the deck was ready. You can retry this exact analysis when you want to continue."
	}
	if deckPreparationEmpty(preparation) {
		return "This book has no recurring vocabulary to study, so there is no deck to download."
	}
	if preparation.State == domain.DeckPreparationReady {
		return "The immutable Anki artifact is ready to download."
	}
	if preparation.State == domain.DeckPreparationQueued {
		return "The exact analysis is queued for deck preparation."
	}
	phase := map[string]string{
		"freezing":    "Freezing the selected vocabulary.",
		"submitting":  "Submitting translation requests.",
		"waiting":     "Waiting for Batch translation; it can take hours (up to 24h).",
		"reconciling": "Reconciling translation results.",
		"retrying":    "Retrying temporary translation failures.",
		"translating": "Translating selected vocabulary.",
		"finalizing":  "Finalizing the immutable Anki artifact.",
		"assembling":  "Assembling the immutable Anki artifact.",
	}
	if summary, ok := phase[preparation.Phase]; ok {
		return summary
	}
	return "Preparing the immutable Anki artifact. You can leave this page and return later."
}

func deckPreparationEmpty(preparation domain.DeckPreparation) bool {
	return preparation.State == domain.DeckPreparationReady && preparation.TotalCards == 0 && preparation.QualityOmissions == 0
}

func analysisResultURL(result analysis.CompletedAnalysis) string {
	return "/books/" + url.PathEscape(result.Source.ID)
}

func analysisDeckPreparationURL(result analysis.CompletedAnalysis) string {
	return "/books/" + url.PathEscape(result.Source.ID) + "/deck/preparations"
}

func jobStatusAttributes(id int64, running bool) templ.Attributes {
	attributes := templ.Attributes{"data-workflow": "analysis"}
	if !running {
		return attributes
	}
	attributes["hx-get"] = fmt.Sprintf("/jobs/%d/status", id)
	attributes["hx-trigger"] = "every 2s"
	attributes["hx-swap"] = "outerHTML"
	return attributes
}

func catalogueSyncJobStatusAttributes(id int64, running bool) templ.Attributes {
	attributes := templ.Attributes{"data-workflow": "catalogue sync"}
	if running {
		attributes["hx-get"] = fmt.Sprintf("/jobs/%d/status", id)
		attributes["hx-trigger"] = "every 2s"
		attributes["hx-swap"] = "outerHTML"
	}
	return attributes
}

func catalogueSyncStatusRunning(status cataloguesync.Status) bool {
	return status.LogicalState == "queued" || status.LogicalState == "running"
}

func knownVocabImportAttributes(status knownvocab.Status) templ.Attributes {
	attributes := templ.Attributes{"data-workflow": "known-vocabulary import"}
	if knownVocabJobBusy(string(status.State)) {
		attributes["hx-get"] = fmt.Sprintf("/vocabulary/imports/%d/status", status.ID)
		attributes["hx-trigger"] = "every 2s"
		attributes["hx-swap"] = "outerHTML"
	}
	return attributes
}

func knownVocabImportTitle(status knownvocab.Status) string {
	return "Known-vocabulary import: " + knownVocabJobLabel(string(status.State))
}

func knownVocabImportSummary(status knownvocab.Status) string {
	if knownVocabJobBusy(string(status.State)) {
		if status.Total > 0 {
			return fmt.Sprintf("%d of %d rows processed. You can leave this page; the import will continue.", status.Processed, status.Total)
		}
		return "The import is continuing in the background. You can leave this page and return later."
	}
	switch status.State {
	case "completed":
		return "The import is complete. Review the updated known vocabulary in Vocabulary."
	case "cancelled":
		return "The import was cancelled before a complete result was available. Upload the file again when ready."
	case "discarded":
		return "The import failed after repeated attempts. Correct the file or configuration and upload it again."
	default:
		return "Review the import result and choose the next action below."
	}
}

func enrichmentJobAttributes(status enrichmentjob.Status) templ.Attributes {
	attributes := templ.Attributes{"data-workflow": "contextual translation"}
	if status.State != "completed" && status.State != "cancelled" && status.State != "discarded" {
		attributes["hx-get"] = fmt.Sprintf("/enrichment-jobs/%d/status", status.ID)
		attributes["hx-trigger"] = "every 2s"
		attributes["hx-swap"] = "outerHTML"
	}
	return attributes
}

func enrichmentJobSummary(status enrichmentjob.Status) string {
	if status.Total == 0 {
		return "Contextual translation work is being processed."
	}
	return fmt.Sprintf("%d of %d translations complete. Attempt %d.", status.Completed, status.Total, maxOne(status.Attempt))
}

func maxOne(value int) int {
	if value < 1 {
		return 1
	}
	return value
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

func currentCoverageStatItems(coverage domain.AnalysisCoverage) []StatItem {
	return []StatItem{{Label: "Current known coverage", Value: fmt.Sprintf("%.1f%%", knownCoveragePercent(coverage)), Detail: "of the analyzed units"}}
}

func thresholdStatItems(thresholds []domain.CoverageThreshold) []StatItem {
	items := make([]StatItem, 0, len(thresholds))
	for _, threshold := range thresholds {
		if threshold.Reachable {
			items = append(items, StatItem{Label: fmt.Sprintf("lemmas for %d%% of analyzed text", threshold.TargetPercent), Value: fmt.Sprintf("%d", threshold.LemmaCount)})
			continue
		}
		items = append(items, StatItem{Label: fmt.Sprintf("%d%% of analyzed text cannot be reached with deck-eligible vocabulary", threshold.TargetPercent), Value: "Unavailable"})
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
