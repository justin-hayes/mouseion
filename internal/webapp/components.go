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
)

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

func learnerLanguagePresent(studyLanguages, progressLanguages []domain.StudyLanguage, language string) bool {
	return studyLanguagePresent(studyLanguages, language) || studyLanguagePresent(progressLanguages, language)
}

func myBookRowID(bookID string) string { return "book-row-" + url.PathEscape(bookID) }

func bookCoverURL(bookID string) string { return "/books/" + url.PathEscape(bookID) + "/cover" }

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

// titlePlate is the text a Title plate composes when a text has no cover
// image (ADR 0130). It is presentation only: never stored, never fetched.
type titlePlate struct {
	Title, OriginalTitle, Author string
	// OriginalLang is the language tag of OriginalTitle: the language of the
	// Reading unit's Definition.
	OriginalLang string
}

// newTitlePlate builds a plate. An original title that only repeats the English
// title (ignoring case and spacing) is dropped (ADR 0132), so every surface
// shows it once or not at all.
func newTitlePlate(title, originalTitle, originalLang, author string) titlePlate {
	return titlePlate{
		Title:         strings.TrimSpace(title),
		OriginalTitle: distinctOriginalTitle(title, originalTitle),
		OriginalLang:  strings.TrimSpace(originalLang),
		Author:        strings.TrimSpace(author),
	}
}

// readingUnitOriginalTitle is the trimmed original title of a Book's Reading unit, or
// "" when the Book has none.
func readingUnitOriginalTitle(unit *domain.BookReadingUnit) string {
	if unit == nil {
		return ""
	}
	return strings.TrimSpace(unit.OriginalTitle)
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

type catalogueSyncConnectionView struct {
	HasStatus       bool
	State           string
	Message         string
	Error           string
	LastSyncedAt    string
	LastSyncedAtISO string
	// UpsertSummary reports the linked Books whose metadata the last sync updated.
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
			view.Message = "Sync completed. The Catalog index is current; no Book metadata changed."
		} else {
			view.UpsertSummary = fmt.Sprintf("%d Books updated from their Catalog entries. Catalog sync changes metadata only; it does not download EPUB content.", status.LastUpsertedCount)
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

func (view catalogueSyncConnectionView) Tone() StatusTone {
	switch {
	case view.Failed:
		return StatusDanger
	case view.Syncing:
		return StatusInfo
	case view.HasLastSyncedAt:
		return StatusSuccess
	default:
		return StatusNeutral
	}
}

// NavigationContext identifies the authenticated shell context.
type NavigationContext string

const (
	NavigationNone       NavigationContext = ""
	NavigationLibrary    NavigationContext = "library"
	NavigationReading    NavigationContext = "reading"
	NavigationStudy      NavigationContext = "study"
	NavigationVocabulary NavigationContext = "vocabulary"
	NavigationCatalogs   NavigationContext = "catalogs"
)

func navigationContextForTitle(title string) NavigationContext {
	switch title {
	case "My Books", "My Library":
		return NavigationLibrary
	case "Study":
		return NavigationStudy
	case "Reading", "Learning":
		return NavigationReading
	case "Vocabulary":
		return NavigationVocabulary
	case "Catalog", "Catalogs":
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

// studyDestinationAttributes is the Study tab's link: the destination, plus an
// accessible name that carries the due count while reviews are due.
func studyDestinationAttributes(context NavigationContext, view *shellView) templ.Attributes {
	attributes := destinationAttributes(context, NavigationStudy)
	if view != nil && view.StudyDue > 0 {
		noun := " reviews due"
		if view.StudyDue == 1 {
			noun = " review due"
		}
		attributes["aria-label"] = "Study, " + strconv.Itoa(view.StudyDue) + noun
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

func statusBadgeShapeClass(label string, tone StatusTone) string {
	switch {
	case tone == StatusDanger:
		return "status-badge__shape status-badge__shape--diamond"
	case strings.Contains(strings.ToLower(label), "current") || strings.Contains(strings.ToLower(label), "ready"):
		return "status-badge__shape status-badge__shape--dot"
	default:
		return "status-badge__shape status-badge__shape--ring"
	}
}

func confirmationClass(tone StatusTone) string {
	if tone == StatusDanger {
		return "confirmation confirmation--danger"
	}
	return "confirmation"
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
	classification := book.EvidenceClassification()
	if classification.ReAnalysisShadowed() {
		return reanalysisLifecycleAction(book, classification.Run)
	}

	var state string
	switch {
	case classification.PublicationPending():
		state = "publishing"
	case classification.Phase == domain.PhaseStale:
		state = "stale"
	case classification.Run == domain.RunQueued:
		state = "queued"
	case classification.Run == domain.RunRunning:
		state = "running"
	case classification.Run == domain.RunFailed:
		state = "failed"
	case classification.Run == domain.RunCancelled:
		state = "cancelled"
	case classification.RunFinished():
		state = "completed"
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
	case "publishing":
		return bookLifecycleAction{analysisPublicationPendingLabel, analysisPublicationPendingDescription, "", "", StatusInfo, false}
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
			return bookLifecycleAction{"Analysis result ready", "Find this book in My Books.", "View in My Books", readingBookURL(book.Source.Language, bookID), StatusSuccess, false}
		}
	}

	return bookLifecycleAction{"Analysis not started", "Analysis evidence is not available for this book yet.", "", "", StatusInfo, false}
}

// reanalysisLifecycleAction presents a newer run that the published analysis
// outlives. The published analysis remains the evidence, so the copy names the
// newer run as secondary and says what stays in effect. A failed or cancelled
// run offers Retry only when the classification does; the Current reading's
// Book cannot be refreshed, so its run links to the job instead.
func reanalysisLifecycleAction(book domain.SourceMaterialSummary, run domain.LatestRunSignal) bookLifecycleAction {
	jobURL := ""
	if book.AnalysisJobID > 0 {
		jobURL = fmt.Sprintf("/jobs/%d", book.AnalysisJobID)
	}
	bookID := book.BookID
	if bookID == "" {
		bookID = book.Source.ID
	}
	retryable := book.EvidenceClassification().Recovery == domain.RecoveryRetryAnalysis
	switch run {
	case domain.RunQueued:
		return bookLifecycleAction{"Re-analysis queued", "A newer analysis of the current EPUB is waiting to begin. The current analysis stays in effect until it completes.", "View analysis status", jobURL, StatusInfo, false}
	case domain.RunRunning:
		return bookLifecycleAction{"Re-analysis running", "A newer analysis of the current EPUB is being analyzed. The current analysis stays in effect until it completes.", "View analysis status", jobURL, StatusInfo, false}
	case domain.RunFailed:
		if !retryable {
			return reanalysisJobAction("Re-analysis failed", "The current analysis stays in effect.", "Review failed analysis", jobURL)
		}
		return bookLifecycleAction{"Re-analysis failed", "The newer analysis failed. The current analysis stays in effect; retry to refresh the evidence.", "Retry analysis", readingReanalyzeURL(bookID), StatusWarning, true}
	case domain.RunCancelled:
		if !retryable {
			return reanalysisJobAction("Re-analysis cancelled", "The current analysis stays in effect.", "Review cancelled analysis", jobURL)
		}
		return bookLifecycleAction{"Re-analysis cancelled", "The newer analysis was cancelled. The current analysis stays in effect; retry to refresh the evidence.", "Retry analysis", readingReanalyzeURL(bookID), StatusWarning, true}
	case domain.RunNone, domain.RunPublicationPending, domain.RunPublicationFailed, domain.RunCompleted:
	}
	return bookLifecycleAction{"Analysis not started", "Analysis evidence is not available for this book yet.", "", "", StatusInfo, false}
}

// reanalysisJobAction presents a settled newer run without a retry. The link
// is omitted when no job is recorded, so no dead route is offered.
func reanalysisJobAction(status, description, label, jobURL string) bookLifecycleAction {
	if jobURL == "" {
		return bookLifecycleAction{status, description, "", "", StatusWarning, false}
	}
	return bookLifecycleAction{status, description, label, jobURL, StatusWarning, false}
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
	return status.Lifecycle.Active()
}

func maxOne(value int) int {
	if value < 1 {
		return 1
	}
	return value
}

func completedAtLabel(value *time.Time) string {
	if value == nil || value.IsZero() {
		return "Unavailable"
	}
	return value.UTC().Format("2006-01-02 15:04:05 UTC")
}
