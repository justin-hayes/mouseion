package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/readingintent"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type readingChooserBookView struct {
	Book        domain.MyBook
	Coverage    *domain.AnalysisCoverage
	Band        domain.CoverageBand
	State       readingChooserState
	Description string
	// RecoveryLabel and RecoveryAllowed come from the same preparation evidence
	// as State, so a terminal refusal never offers a retry.
	RecoveryLabel   string
	RecoveryAllowed bool
}

type readingChooserState string

// vocabularyCountsUpdatingDescription explains why a Book shows no coverage
// while its effective vocabulary counts are rebuilt.
const vocabularyCountsUpdatingDescription = "This Book's vocabulary counts are being refreshed. Coverage appears when they are ready."

const (
	readingChooserInProgress     readingChooserState = "in_progress"
	readingChooserNeedsAttention readingChooserState = "needs_attention"
	readingChooserUpdating       readingChooserState = "updating"
)

// readingChooserEntryView is one neutral chooser entry: a single To Read Book.
type readingChooserEntryView struct {
	Key    string
	Title  string
	Author string
	Book   readingChooserBookView
}

type readingChooserPageView struct {
	Language, LanguageLabel          string
	CurrentBookID, CurrentSnapshotID string
	Entries                          []readingChooserEntryView
}

func (h *Handler) reading(w http.ResponseWriter, r *http.Request) {
	owner := user(r)
	language, languageLabel := activeStudyLanguageForContext(r.Context())
	currentReading, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner.ID, language)
	if err != nil {
		fail(w, err)
		return
	}
	// Back to book vocabulary names the commitment it left. Only that exact
	// snapshot may restore controls; anything else is explained before any
	// other reading context opens, and nothing is transferred to it.
	if snapshot := strings.TrimSpace(r.URL.Query().Get("snapshot")); snapshot != "" && (!currentReading.IsActive() || currentReading.SnapshotID != snapshot || r.URL.Query().Get("reading") != currentReading.BookID) {
		h.renderReadingBrowseOriginExpired(w, r, owner, language, currentReading.IsActive())
		return
	}
	if currentReading.IsActive() {
		readingView, buildErr := h.buildReadingView(r.Context(), owner.ID, language)
		if buildErr != nil {
			fail(w, buildErr)
			return
		}
		readingView.Language = language
		readingView.LanguageLabel = languageLabel
		status := http.StatusOK
		if readingView.CurrentReading != nil {
			readingView.Browse, status = h.loadReadingBrowse(r.Context(), r, owner.ID, language, currentReading.BookID)
			readingView.Browse.SnapshotID = currentReading.SnapshotID
		}
		renderStatus(w, r, status, ReadingPage(owner, h.csrf(w, r), readingView, r.URL.Query().Get("message"), r.URL.Query().Get("error")))
		return
	}
	view, err := h.buildReadingChooser(r.Context(), owner.ID, language, languageLabel, "")
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, ReadingChooserPage(owner, h.csrf(w, r), view, r.URL.Query().Get("message"), r.URL.Query().Get("error")))
}

// renderReadingBrowseOriginExpired explains a Back to book vocabulary request
// whose commitment ended or was restarted. The My Books recovery is offered
// only for a Book the owner has in the active study language.
func (h *Handler) renderReadingBrowseOriginExpired(w http.ResponseWriter, r *http.Request, owner domain.User, language string, hasCurrent bool) {
	myBooksURL := ""
	if bookID := strings.TrimSpace(r.URL.Query().Get("reading")); bookID != "" {
		detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner.ID, bookID)
		switch {
		case errors.Is(err, domain.ErrNotFound):
		case err != nil:
			fail(w, err)
			return
		case detail.Book.LanguageTag == language:
			myBooksURL = myBookSearchURL(detail)
		}
	}
	renderStatus(w, r, http.StatusConflict, ReadingBrowseOriginExpiredPage(owner, h.csrf(w, r), hasCurrent, myBooksURL))
}

func (h *Handler) switchReadingPage(w http.ResponseWriter, r *http.Request) {
	owner := user(r).ID
	language, languageLabel := activeStudyLanguageForContext(r.Context())
	current, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	if !current.IsActive() {
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("There is no current book to switch from. Choose a book from Reading first."))
		return
	}
	view, err := h.buildReadingChooser(r.Context(), owner, language, languageLabel, current.BookID)
	if err != nil {
		fail(w, err)
		return
	}
	view.CurrentBookID = current.BookID
	view.CurrentSnapshotID = current.SnapshotID
	render(w, r, ReadingChooserPage(user(r), h.csrf(w, r), view, "", ""))
}

// assessReadingActionFlags loads the Book a start or switch names and records
// its lemma-review flags before the store transition. It writes the response
// and reports false when the request cannot continue.
func (h *Handler) assessReadingActionFlags(w http.ResponseWriter, r *http.Request, owner, bookID string) bool {
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, domain.ErrNotFound) {
		http.NotFound(w, r)
		return false
	}
	if err != nil {
		fail(w, err)
		return false
	}
	// Flag detection is a write the freeze depends on, so it runs before the
	// transition. Persistence decides everything else.
	if _, err := h.services.LemmaReview.Assess(r.Context(), owner, detail.Book.ID); err != nil {
		fail(w, err)
		return false
	}
	return true
}

func (h *Handler) switchReading(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := strings.TrimSpace(r.PathValue("id"))
	if !h.adoptBookLanguageOrFail(w, r, owner, bookID) {
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	expectedBookID := strings.TrimSpace(r.FormValue("expected_current_book_id"))
	expectedSnapshotID := strings.TrimSpace(r.FormValue("expected_current_snapshot_id"))
	if language == "" {
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("Choose a study language and refresh Reading before switching books."))
		return
	}
	if !h.assessReadingActionFlags(w, r, owner, bookID) {
		return
	}
	_, err := h.services.Store.Reading.SwitchCurrentReading(r.Context(), owner, language, bookID, expectedBookID, expectedSnapshotID)
	var ineligible domain.CurrentReadingIneligibleError
	switch {
	case errors.Is(err, domain.ErrCurrentReadingStale), errors.Is(err, domain.ErrNotFound):
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("The current book changed while you were choosing. No changes were made; review Reading before trying again."))
	case errors.As(err, &ineligible) && ineligible.Reason.IsBookChoiceRejection():
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("This book is no longer an eligible To Read choice. No changes were made; refresh Reading and try again."))
	case errors.Is(err, domain.ErrCurrentReadingIneligible):
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("This book no longer has current analyzed content or is no longer To Read. No changes were made; refresh Reading and try again."))
	case errors.Is(err, domain.ErrUnresolvedLemmaReviewFlags):
		redirect(w, r, "/reading/books/"+url.PathEscape(bookID)+"/lemma-review")
	case errors.Is(err, domain.ErrVocabularyBrowseCountsPending):
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("Across-book vocabulary counts are updating. No reading was changed; try switching again when the counts are ready."))
	case errors.Is(err, domain.ErrVocabularyBrowseCountsUnavailable):
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("Across-book vocabulary counts are unavailable after repeated rebuild failures. No reading was changed; ask the server operator to restart Mouseion, then retry."))
	case err != nil:
		fail(w, err)
	default:
		redirect(w, r, readingPagePath(r.Context())+"?message="+url.QueryEscape(h.currentReadingBookTitle(r.Context(), owner, bookID)+" is now your current reading."))
	}
}

// endCurrentReading ends the exact expected commitment. It needs no live
// analysis, so End stays available when the Book's analysis is missing or
// stale. Replays are verified by the store against durable facts.
func (h *Handler) endCurrentReading(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	expectedBookID := strings.TrimSpace(r.FormValue("expected_current_book_id"))
	expectedSnapshotID := strings.TrimSpace(r.FormValue("expected_current_snapshot_id"))
	if !h.adoptBookLanguageOrFail(w, r, owner, expectedBookID) {
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" {
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("Choose a study language and refresh Reading before ending the current reading."))
		return
	}
	if expectedBookID == "" || expectedSnapshotID == "" {
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape(endCurrentReadingStaleMessage))
		return
	}
	err := h.services.Store.Reading.EndCurrentReading(r.Context(), owner, language, expectedBookID, expectedSnapshotID)
	if errors.Is(err, domain.ErrCurrentReadingStale) || errors.Is(err, domain.ErrNotFound) {
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape(endCurrentReadingStaleMessage))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, readingPagePath(r.Context())+"?message="+url.QueryEscape("Reading ended. "+h.currentReadingBookTitle(r.Context(), owner, expectedBookID)+" stays in To Read; its analysis, decks, and history are preserved. Choose a Book when you are ready."))
}

const endCurrentReadingStaleMessage = "The current reading changed before this action. No changes were made; review Reading and try again."

func (h *Handler) reanalyzeToReadBook(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := r.PathValue("id")
	if !h.adoptBookLanguageOrFail(w, r, owner, bookID) {
		return
	}
	intent, err := h.readingIntentModule()
	if err != nil {
		fail(w, err)
		return
	}
	outcome, err := intent.Retry(r.Context(), owner, bookID)
	if err != nil {
		fail(w, err)
		return
	}
	switch outcome.Kind {
	case readingintent.OutcomeNotFound, readingintent.OutcomeNotToRead:
		http.NotFound(w, r)
		return
	default:
	}
	message, isError := retryMessage(outcome)
	queryParam := "message"
	if isError {
		queryParam = "error"
	}
	redirect(w, r, readingPagePath(r.Context())+"?"+queryParam+"="+url.QueryEscape(message))
}

// analysisCouldNotStartMessage is the retry sentence for a submission that
// failed without a stable cause.
const analysisCouldNotStartMessage = "Analysis could not be started. The To Read choice is retained. Review current book content in My Books and try again."

// retryMessage maps a Retry outcome to the learner-facing sentence and whether
// it is an error. An EPUB Book reports the same plain confirmation whether
// acquisition was queued now or is already running.
func retryMessage(outcome readingintent.Outcome) (message string, isError bool) {
	switch outcome.Kind {
	case readingintent.OutcomeCurrentBookRefused:
		return "The current book cannot be refreshed. Its frozen evidence remains unchanged.", true
	case readingintent.OutcomePreparationUnavailable:
		return "Classical preparation is unavailable.", true
	case readingintent.OutcomeAlreadyCurrent:
		return "The analysis is already current.", false
	case readingintent.OutcomeJobQueued:
		switch {
		case outcome.Route == readingintent.RouteClassical:
			return fmt.Sprintf("Preparation job #%d submitted.", outcome.JobNumber), false
		case outcome.Acquisition:
			return epubPreparationConfirmation, false
		default:
			return fmt.Sprintf("Analysis job #%d submitted.", outcome.JobNumber), false
		}
	case readingintent.OutcomeLegacyRows:
		return classical.LegacyRowsReason, true
	case readingintent.OutcomeWithdrawnDefinition:
		return classicalWithdrawnRefusal, true
	case readingintent.OutcomeSourceRefused:
		return classicalSourceMismatchRefusal, true
	case readingintent.OutcomeCouldNotStart:
		if outcome.Route == readingintent.RouteClassical {
			return "Preparation could not be started. Retry when ready.", true
		}
		return analysisCouldNotStartMessage, true
	case readingintent.OutcomeNoUsableEPUBUnits:
		return "The current source has no usable EPUB units. The To Read choice is retained; analysis is unavailable.", true
	default:
		// Retry returns only the kinds above; a new kind must be mapped here.
		return analysisCouldNotStartMessage, true
	}
}

func (h *Handler) startReading(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := strings.TrimSpace(r.PathValue("id"))
	if !h.adoptBookLanguageOrFail(w, r, owner, bookID) {
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" {
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("Choose a study language before starting a book."))
		return
	}
	if !h.assessReadingActionFlags(w, r, owner, bookID) {
		return
	}
	start, err := h.services.Store.Reading.StartCurrentReadingResult(r.Context(), owner, language, bookID)
	var ineligible domain.CurrentReadingIneligibleError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		http.NotFound(w, r)
		return
	case err == nil && start.Replayed:
		// Starting the Book that is already current is a repeat, not a conflict.
		redirect(w, r, readingPagePath(r.Context())+"?message="+url.QueryEscape(h.currentReadingBookTitle(r.Context(), owner, bookID)+" is already your current reading."))
		return
	case errors.Is(err, domain.ErrCurrentReadingExists):
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("A current book is already set for this language. Review it in Reading before starting another."))
		return
	case errors.As(err, &ineligible) && ineligible.Reason.IsBookChoiceRejection():
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("This book is no longer an eligible To Read candidate. Review Reading before trying again."))
		return
	case errors.Is(err, domain.ErrCurrentReadingIneligible):
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("This book no longer has trustworthy current analysis or is no longer To Read. No changes were made; refresh Reading and try again."))
		return
	case errors.Is(err, domain.ErrUnresolvedLemmaReviewFlags):
		redirect(w, r, "/reading/books/"+url.PathEscape(bookID)+"/lemma-review")
		return
	case errors.Is(err, domain.ErrVocabularyBrowseCountsPending):
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("Across-book vocabulary counts are updating. No reading was changed; try starting again when the counts are ready."))
		return
	case errors.Is(err, domain.ErrVocabularyBrowseCountsUnavailable):
		redirect(w, r, readingPagePath(r.Context())+"?error="+url.QueryEscape("Across-book vocabulary counts are unavailable after repeated rebuild failures. No reading was changed; ask the server operator to restart Mouseion, then retry."))
		return
	case err != nil:
		fail(w, err)
		return
	}
	redirect(w, r, readingPagePath(r.Context())+"?message="+url.QueryEscape(h.currentReadingBookTitle(r.Context(), owner, bookID)+" is now your current reading."))
}

// buildReadingChooser lists the active language's To Read candidates in neutral
// title order. excludeBookID is the current Book in switch mode: it has no cell
// and does not count toward its Work's To Read total.
func (h *Handler) buildReadingChooser(ctx context.Context, owner, language, languageLabel, excludeBookID string) (readingChooserPageView, error) {
	books, err := h.services.Store.Reading.ListMyBooksWithEvidence(ctx, owner)
	if err != nil {
		return readingChooserPageView{}, err
	}
	view := readingChooserPageView{Language: language, LanguageLabel: languageLabel}
	candidates := make([]domain.MyBook, 0, len(books))
	for _, book := range books {
		if !readingChooserCandidate(book, language) || book.Book.ID == excludeBookID {
			continue
		}
		candidates = append(candidates, book)
	}
	h.annotateAnalysisUpdates(ctx, owner, candidates)
	h.annotateSourceStructureUpdates(ctx, owner, candidates)
	views := make([]readingChooserBookView, 0, len(candidates))
	for _, book := range candidates {
		candidate := readingChooserBookView{Book: book}
		if err := h.classifyReadingChooserCandidate(ctx, owner, &candidate); err != nil {
			return readingChooserPageView{}, err
		}
		views = append(views, candidate)
	}
	view.Entries = readingChooserEntries(views)
	return view, nil
}

// readingChooserCandidate reports whether a Book belongs in the chooser at all:
// a To Read Book in the active language.
func readingChooserCandidate(book domain.MyBook, language string) bool {
	return book.Disposition == domain.BookDispositionToRead && language != "" && book.Book.LanguageTag == language
}

// readingChooserPreparationInProgress reports a classical candidate whose
// preparation is queued or running, as the domain classification decided.
func readingChooserPreparationInProgress(item readingChooserBookView) bool {
	return item.Book.Classification().Preparation.InProgress()
}

// classifyReadingChooserCandidate resolves one eligible Book's coverage and
// evidence state. A missing or untrustworthy reading leaves Coverage nil.
func (h *Handler) classifyReadingChooserCandidate(ctx context.Context, owner string, candidate *readingChooserBookView) error {
	book := candidate.Book
	if preparation, ok := classicalPreparationFor(book); ok {
		candidate.Description = preparation.Description
		candidate.RecoveryLabel = preparation.Recovery
		candidate.RecoveryAllowed = preparation.Actionable
		if preparation.InProgress {
			candidate.State = readingChooserInProgress
		} else {
			candidate.State = readingChooserNeedsAttention
		}
		return nil
	}
	if book.Acquired == nil || !analysisReadyForReading(*book.Acquired) {
		candidate.State, candidate.Description = readingChooserStatusFor(book.Classification())
		candidate.RecoveryLabel = "Retry acquisition or analysis"
		candidate.RecoveryAllowed = candidate.State == readingChooserNeedsAttention
		return nil
	}
	if h.services.AnalysisInsights == nil || strings.TrimSpace(book.Acquired.CorpusID) == "" {
		candidate.State = readingChooserNeedsAttention
		candidate.Description = "Completed analysis statistics are unavailable. Refresh the page or retry analysis."
		candidate.RecoveryLabel = "Retry analysis"
		candidate.RecoveryAllowed = true
		return nil
	}
	coverage, coverageErr := h.services.AnalysisInsights.Coverage(ctx, owner, book.Acquired.CorpusID)
	switch {
	case errors.Is(coverageErr, analysisinsights.ErrStatisticsUnavailable):
		candidate.State = readingChooserNeedsAttention
		candidate.Description = "Completed analysis statistics are unavailable. Retry analysis to refresh the evidence."
		candidate.RecoveryLabel = "Retry analysis"
		candidate.RecoveryAllowed = true
		return nil
	case errors.Is(coverageErr, analysisinsights.ErrCountsUpdating):
		candidate.State = readingChooserUpdating
		candidate.Description = vocabularyCountsUpdatingDescription
		return nil
	case coverageErr != nil:
		return coverageErr
	}
	candidate.Coverage = &coverage
	candidate.Band = domain.CoverageBandFor(coverage.KnownTokenCount, coverage.AnalyzableTokenCount)
	return nil
}

// readingChooserEntries lists one ordinary entry per candidate Book in neutral
// title order.
func readingChooserEntries(candidates []readingChooserBookView) []readingChooserEntryView {
	entries := make([]readingChooserEntryView, 0, len(candidates))
	for _, candidate := range candidates {
		entries = append(entries, readingChooserEntryView{
			Key:    "book:" + candidate.Book.Book.ID,
			Title:  readingChooserBookTitle(candidate.Book),
			Author: strings.TrimSpace(candidate.Book.Book.Author),
			Book:   candidate,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return readingChooserEntryLess(entries[i], entries[j]) })
	return entries
}

func readingChooserEntryLess(left, right readingChooserEntryView) bool {
	leftTitle, rightTitle := strings.ToLower(left.Title), strings.ToLower(right.Title)
	if leftTitle != rightTitle {
		return leftTitle < rightTitle
	}
	leftAuthor, rightAuthor := strings.ToLower(strings.TrimSpace(left.Author)), strings.ToLower(strings.TrimSpace(right.Author))
	if leftAuthor != rightAuthor {
		return leftAuthor < rightAuthor
	}
	return left.Key < right.Key
}

func readingChooserBookTitle(book domain.MyBook) string {
	if title := strings.TrimSpace(myBookDisplayTitle(book)); title != "" {
		return title
	}
	if book.Acquired != nil {
		if title := strings.TrimSpace(book.Acquired.BookTitle); title != "" {
			return title
		}
	}
	return book.Book.ID
}

// readingChooserStatusFor places a To Read Book without completed analysis in
// the chooser's in-progress or needs-attention group.
func readingChooserStatusFor(c domain.BookEvidenceClassification) (readingChooserState, string) {
	if c.PublicationPending() {
		return readingChooserInProgress, analysisPublicationPendingDescription
	}
	if c.Phase == domain.PhaseAnalyzing {
		return readingChooserInProgress, "Analysis is queued or running. This candidate will show its band when current evidence is ready."
	}
	if c.Content == domain.ContentNotAcquired {
		return readingChooserNeedsAttention, "Book content has not been acquired yet. Return to My Books to review its catalog entry."
	}
	if c.Evidence == domain.BookStale {
		return readingChooserNeedsAttention, "The analysis no longer matches the current book content. Retry analysis to refresh its evidence."
	}
	if c.Evidence == domain.BookUnavailable || c.Evidence == domain.BookNotAcquired {
		return readingChooserNeedsAttention, "Current book content is unavailable. Retry acquisition or analysis from My Books."
	}
	if c.Phase == domain.PhaseFailed || c.Phase == domain.PhaseCancelled {
		return readingChooserNeedsAttention, "The last analysis did not complete. Retry analysis to refresh its evidence."
	}
	return readingChooserNeedsAttention, "Current analysis is not complete. Retry analysis to produce usable evidence."
}

func readingChooserStateLabel(state readingChooserState) string {
	switch state {
	case readingChooserInProgress:
		return "Analysis in progress"
	case readingChooserUpdating:
		return "Updating"
	case readingChooserNeedsAttention:
	}
	return "Not assessed"
}

func readingChooserTitle(view readingChooserPageView) string {
	if strings.TrimSpace(view.LanguageLabel) == "" {
		return "Choose a To Read book"
	}
	return "Choose a To Read book in " + view.LanguageLabel
}

func readingChooserPageTitle(view readingChooserPageView) string {
	if view.CurrentBookID != "" {
		return "Switch current reading"
	}
	return readingChooserTitle(view)
}

func readingChooserPageDescription(view readingChooserPageView) string {
	if view.CurrentBookID != "" {
		return "Choose another To Read Book. Your current Book will return to To Read, so you can come back to it later. Coverage groups describe your current vocabulary, not what you ought to read next."
	}
	return "Choose a To Read book when you are ready. Coverage groups describe current vocabulary evidence, not difficulty or reading recommendations."
}

func readingChooserCandidateCount(view readingChooserPageView) int {
	return len(view.Entries)
}

func readingChooserHasCoverage(view readingChooserPageView) bool {
	for _, entry := range view.Entries {
		if entry.Book.Coverage != nil {
			return true
		}
	}
	return false
}

// readingChooserBandLabel names a numeric or no-comparison coverage band.
func readingChooserBandLabel(band domain.CoverageBand) string {
	switch band {
	case domain.CoverageBand99Plus:
		return "99% or above"
	case domain.CoverageBand97To99:
		return "97% to below 99%"
	case domain.CoverageBand95To97:
		return "95% to below 97%"
	case domain.CoverageBandBelow95:
		return "Below 95%"
	case domain.CoverageBandNoComparison:
		return "No vocabulary comparison"
	}
	return ""
}

// readingChooserBookBandLabel names the evidence band an ordinary candidate's
// margin shows. An untrustworthy analysis names its state instead.
func readingChooserBookBandLabel(item readingChooserBookView) string {
	if item.Coverage != nil {
		return readingChooserBandLabel(item.Band)
	}
	return readingChooserStateLabel(item.State)
}

// readingChooserBookAnchorID is the fragment target My Books' To Read cells open
// on that Book's chooser entry.
func readingChooserBookAnchorID(bookID string) string {
	return "reading-chooser-book-" + bookID
}

// readingChooserCandidateAnchorID names the anchor for one chooser candidate.
func readingChooserCandidateAnchorID(item readingChooserBookView) string {
	return readingChooserBookAnchorID(item.Book.Book.ID)
}

func readingChooserNextMarkerText(coverage domain.AnalysisCoverage, band domain.CoverageBand) string {
	marker, hasNextMarker := readingChooserNextMarkers[band]
	if !hasNextMarker {
		if band == domain.CoverageBand99Plus {
			return "At least 99% of the words in this Book are already Known."
		}
		if band == domain.CoverageBandNoComparison {
			return "No words to compare for coverage."
		}
		return "The number of words to the next band is unavailable."
	}
	for _, threshold := range coverage.Thresholds {
		if threshold.TargetPercent == marker {
			return readingChooserThresholdText(threshold)
		}
	}
	return fmt.Sprintf("The number of words to reach %d%% is unavailable.", marker)
}

var readingChooserNextMarkers = map[domain.CoverageBand]int{
	domain.CoverageBandBelow95: 95,
	domain.CoverageBand95To97:  97,
	domain.CoverageBand97To99:  99,
}

func readingChooserThresholdText(threshold domain.CoverageThreshold) string {
	if !threshold.Reachable {
		return fmt.Sprintf("To reach %d%%: not possible with the words available for review.", threshold.TargetPercent)
	}
	return fmt.Sprintf("To reach %d%%: %s more distinct words marked Known.", threshold.TargetPercent, readingChooserCount(threshold.LemmaCount))
}

func readingChooserAnalysisJobURL(jobID int64) string {
	if jobID <= 0 {
		return "/library?disposition=to_read"
	}
	return fmt.Sprintf("/jobs/%d", jobID)
}

func readingChooserCoverageDisplay(coverage domain.AnalysisCoverage) string {
	percent, ok := readingChooserCoveragePercent(coverage)
	if !ok {
		return "No words to compare"
	}
	return fmt.Sprintf("%.1f%%", percent)
}

// readingChooserCoveragePercent is the one clamped percentage a Book's margin uses.
func readingChooserCoveragePercent(coverage domain.AnalysisCoverage) (float64, bool) {
	if coverage.AnalyzableTokenCount <= 0 {
		return 0, false
	}
	return min(float64(coverage.KnownTokenCount)*100/float64(coverage.AnalyzableTokenCount), 100), true
}

// The learner-facing interface is English; group evidence counts in that locale.
func readingChooserCount(count int64) string {
	return message.NewPrinter(language.English).Sprintf("%d", count)
}
