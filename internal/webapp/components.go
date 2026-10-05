package webapp

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichmentjob"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
)

// loginStylesheet keeps Tailwind/daisyUI scoped to the sign-in route.
func loginStylesheet(enabled []bool) templ.Component {
	if len(enabled) == 0 || !enabled[0] {
		return templ.NopComponent
	}
	return templ.Raw(`<link rel="stylesheet" href="/static/login.css">`)
}

type shellStyle uint8

const (
	shellStyleDefault shellStyle = iota
	shellStyleLogin
	shellStyleMyBooks
	shellStyleReading
	shellStyleVocabulary
	shellStyleCatalogs
	shellStyleJobs
)

func selectedShellStyle(styles []shellStyle) shellStyle {
	if len(styles) == 0 {
		return shellStyleDefault
	}
	return styles[0]
}

func loginStylesRequested(styles []shellStyle) []bool {
	return []bool{selectedShellStyle(styles) == shellStyleLogin}
}

func shellBodyClass(styles []shellStyle) string {
	switch selectedShellStyle(styles) {
	case shellStyleMyBooks:
		return "my-books-shell"
	case shellStyleReading:
		return "reading-shell"
	case shellStyleVocabulary:
		return "vocabulary-shell"
	case shellStyleCatalogs:
		return "catalogs-shell"
	case shellStyleJobs:
		return "jobs-shell"
	case shellStyleDefault, shellStyleLogin:
		return ""
	}
	return ""
}

func studyLanguagePresent(languages []domain.StudyLanguage, language string) bool {
	for _, candidate := range languages {
		if candidate.Language == language {
			return true
		}
	}
	return false
}

func learnerLanguagePresent(studyLanguages, knownLanguages []domain.StudyLanguage, language string) bool {
	return studyLanguagePresent(studyLanguages, language) || studyLanguagePresent(knownLanguages, language)
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

func myBookRowID(bookID string) string { return "book-row-" + url.PathEscape(bookID) }

func bookCoverURL(bookID string) string { return "/books/" + url.PathEscape(bookID) + "/cover" }

func myBookInJourney(book domain.MyBook) bool {
	return book.IsToRead || book.IsCurrentReading
}

func bookCoverLabel(cover domain.BookCover) string {
	switch cover.State {
	case domain.BookCoverPending:
		return "Cover pending"
	case domain.BookCoverUnavailable:
		return "Cover unavailable"
	default:
		return "No cover available"
	}
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
		view.Message = connection.Name + " has never synced. Sync now reconciles ready non-English catalog languages; it does not download EPUB content."
		return view
	}
	switch status.State {
	case domain.CatalogueSyncSyncing:
		view.State = "Syncing"
		view.Syncing = true
		view.Message = connection.Name + " is syncing metadata. Existing Books remain available while Mouseion reconciles bibliographic entries."
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
			view.UpsertSummary = fmt.Sprintf("%d books added or updated. Catalog sync changes metadata only; it does not download EPUB content.", status.LastUpsertedCount)
		}
	case domain.CatalogueSyncFailed:
		view.State = "Sync failed"
		view.Failed = true
		view.Message = connection.Name + " sync failed. Existing Books remain unchanged and available."
		view.Error = status.LastError
	default:
		view.State = "Never synced"
		view.Message = connection.Name + " has never synced. Sync now reconciles ready non-English catalog languages; it does not download EPUB content."
	}
	return view
}

// NavigationContext identifies the authenticated shell context.
type NavigationContext string

const (
	NavigationNone           NavigationContext = ""
	NavigationLibrary        NavigationContext = "library"
	NavigationReadingJourney NavigationContext = "reading"
	NavigationLearning       NavigationContext = NavigationReadingJourney
	NavigationVocabulary     NavigationContext = "vocabulary"
	NavigationCatalogs       NavigationContext = "catalogs"
)

func navigationContextForTitle(title string) NavigationContext {
	switch {
	case title == "My Books", title == "My Library":
		return NavigationLibrary
	case title == "Reading", title == "Learning":
		return NavigationReadingJourney
	case title == "Vocabulary", title == "Known vocabulary":
		return NavigationVocabulary
	case title == "Catalogs":
		return NavigationCatalogs
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
	case StatusNeutral:
		return "status-badge status-badge--neutral"
	default:
		// Unknown tones use the neutral presentation.
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

func legacyMyBooks(books []domain.SourceMaterialSummary) []domain.MyBook {
	out := make([]domain.MyBook, 0, len(books))
	for _, source := range books {
		bookID := source.BookID
		if bookID == "" {
			bookID = source.Source.ID
		}
		out = append(out, domain.MyBook{Book: domain.Book{ID: bookID, OwnerID: source.Source.OwnerID, Title: canonicalBookTitle(source), Author: source.BookAuthor, LanguageState: domain.LanguageChosen, LanguageTag: source.Source.Language}, Acquired: &source})
	}
	return out
}

type bookLifecycleAction struct {
	Status      string
	Description string
	Label       string
	URL         string
	Tone        StatusTone
	Submit      bool
}

func bookLifecycleActionFor(book domain.SourceMaterialSummary) bookLifecycleAction {
	runID := book.AnalysisRunID

	state := strings.ToLower(strings.TrimSpace(book.AnalysisState))
	status := strings.ToLower(strings.TrimSpace(book.AnalysisStatus))
	if status == "stale" || status == "stale analysis" {
		state = "stale"
	} else if state == "" {
		switch {
		case (status == "analyzed" || status == "analysis result ready" || status == "completed") && runID != "" && book.CorpusID != "":
			state = "completed"
		case status == "analysis failed" || status == "analysis failed — action required" || status == "failed":
			state = "failed"
		case status == "analysis cancelled" || status == "cancelled":
			state = "cancelled"
		case status == "stale" || status == "stale analysis":
			state = "stale"
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
	bookID := book.BookID
	if bookID == "" {
		bookID = book.Source.ID
	}
	switch state {
	case "stale":
		return bookLifecycleAction{"Stale analysis", "The current acquired content differs from the analyzed revision. Re-analyze it to refresh the evidence in Reading.", "Re-analyze", readingReanalyzeURL(bookID), StatusWarning, true}
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
			return bookLifecycleAction{"Analysis result ready", "Open this book in Reading.", "View in Reading", readingBookURL(bookID), StatusSuccess, false}
		}
	}

	return bookLifecycleAction{"Analysis not started", "Analysis evidence is not available for this book yet.", "", "", StatusInfo, false}
}

func feedbackClass(kind FeedbackKind) string {
	switch kind {
	case FeedbackSuccess, FeedbackWarning, FeedbackError:
		return "feedback feedback--" + string(kind)
	case FeedbackInfo:
		return "feedback feedback--info"
	default:
		// Unknown feedback kinds use the informational presentation.
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

func boolString(value bool) string { return strconv.FormatBool(value) }

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
	if preparation.Error == domain.DeckPreparationRequiresRepreparationError {
		return "Re-preparation required"
	}
	if deckPreparationEmpty(preparation) {
		return "No recurring vocabulary"
	}
	return deckPreparationStatusLabel(preparation.State)
}

func deckPreparationStatusTone(preparation domain.DeckPreparation) StatusTone {
	if preparation.Error == domain.DeckPreparationRequiresRepreparationError {
		return StatusWarning
	}
	if deckPreparationEmpty(preparation) {
		return StatusNeutral
	}
	return statusTone(string(preparation.State))
}

func deckPreparationSummary(preparation domain.DeckPreparation) string {
	if preparation.Error == domain.DeckPreparationRequiresRepreparationError {
		return "The previous artifact is retained, but a new preparation is needed from the current analysis."
	}
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

func deckPreparationHasNewerRevision(preparation domain.DeckPreparation) bool {
	return preparation.State == domain.DeckPreparationReady && preparation.DeckRevision > 1 && !deckPreparationEmpty(preparation) && preparation.Error != domain.DeckPreparationRequiresRepreparationError
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
	attributes := templ.Attributes{"data-workflow": "catalog sync"}
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
	//nolint:exhaustive // River's JobState is an open upstream enumeration; unknown states use the generic summary.
	switch status.State {
	case "completed":
		return "The import is complete. New rows are now counted as known vocabulary."
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
