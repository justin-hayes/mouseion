package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

type readingChooserBookView struct {
	Book        domain.MyBook
	Coverage    *domain.AnalysisCoverage
	Band        domain.CoverageBand
	State       readingChooserState
	Description string
	// Work facts come from the reviewed Reading-unit definition, never from the
	// Books the learner happens to have added.
	WorkKey        string
	WorkTitle      string
	WorkAuthor     string
	WorkGreekTitle string
	WorkBooks      int
	Position       int
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

// readingChooserEntryView is one neutral chooser entry: a single candidate Book,
// or one Work whose To Read Books are grouped into a band table.
type readingChooserEntryView struct {
	Key        string
	Grouped    bool
	Title      string
	Author     string
	GreekTitle string
	WorkBooks  int
	Books      []readingChooserBookView
	Bands      []readingChooserBandView
}

// readingChooserBandView is one non-empty band row inside a grouped Work entry.
type readingChooserBandView struct {
	Label string
	Books []readingChooserBookView
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
		case errors.Is(err, persistence.ErrNotFound):
		case err != nil:
			fail(w, err)
			return
		case detail.Book.LanguageTag == language:
			myBooksURL = "/library?" + url.Values{"q": {detail.Book.Title}}.Encode()
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
		redirect(w, r, "/reading?error="+url.QueryEscape("There is no current book to switch from. Choose a book from Reading first."))
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

func (h *Handler) switchReading(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := strings.TrimSpace(r.PathValue("id"))
	language, _ := activeStudyLanguageForContext(r.Context())
	expectedBookID := strings.TrimSpace(r.FormValue("expected_current_book_id"))
	expectedSnapshotID := strings.TrimSpace(r.FormValue("expected_current_snapshot_id"))
	if language == "" {
		redirect(w, r, "/reading?error="+url.QueryEscape("Choose a study language and refresh Reading before switching books."))
		return
	}
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	// Flag detection is a write the freeze depends on, so it runs before the
	// switch. Persistence decides everything else.
	if _, err := h.services.LemmaReview.Assess(r.Context(), owner, detail.Book.ID); err != nil {
		fail(w, err)
		return
	}
	_, err = h.services.Store.Reading.SwitchCurrentReading(r.Context(), owner, language, bookID, expectedBookID, expectedSnapshotID)
	var ineligible persistence.CurrentReadingIneligibleError
	switch {
	case errors.Is(err, persistence.ErrCurrentReadingStale), errors.Is(err, persistence.ErrNotFound):
		redirect(w, r, "/reading?error="+url.QueryEscape("The current book changed while you were choosing. No changes were made; review Reading before trying again."))
	case errors.As(err, &ineligible) && ineligible.Reason.IsBookChoiceRejection():
		redirect(w, r, "/reading?error="+url.QueryEscape("This book is no longer an eligible To Read choice. No changes were made; refresh Reading and try again."))
	case errors.Is(err, persistence.ErrCurrentReadingIneligible):
		redirect(w, r, "/reading?error="+url.QueryEscape("This book no longer has current analyzed content or is no longer To Read. No changes were made; refresh Reading and try again."))
	case errors.Is(err, persistence.ErrUnresolvedLemmaReviewFlags):
		redirect(w, r, "/reading/books/"+url.PathEscape(bookID)+"/lemma-review")
	case errors.Is(err, persistence.ErrVocabularyBrowseCountsPending):
		redirect(w, r, "/reading?error="+url.QueryEscape("Across-book vocabulary counts are updating. No reading was changed; try switching again when the counts are ready."))
	case errors.Is(err, persistence.ErrVocabularyBrowseCountsUnavailable):
		redirect(w, r, "/reading?error="+url.QueryEscape("Across-book vocabulary counts are unavailable after repeated rebuild failures. No reading was changed; ask the server operator to restart Mouseion, then retry."))
	case err != nil:
		fail(w, err)
	default:
		redirect(w, r, "/reading?message="+url.QueryEscape(h.currentReadingBookTitle(r.Context(), owner, bookID)+" is now your current reading."))
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
	language, _ := activeStudyLanguageForContext(r.Context())
	expectedBookID := strings.TrimSpace(r.FormValue("expected_current_book_id"))
	expectedSnapshotID := strings.TrimSpace(r.FormValue("expected_current_snapshot_id"))
	if language == "" {
		redirect(w, r, "/reading?error="+url.QueryEscape("Choose a study language and refresh Reading before ending the current reading."))
		return
	}
	if expectedBookID == "" || expectedSnapshotID == "" {
		redirect(w, r, "/reading?error="+url.QueryEscape(endCurrentReadingStaleMessage))
		return
	}
	err := h.services.Store.Reading.EndCurrentReading(r.Context(), owner, language, expectedBookID, expectedSnapshotID)
	if errors.Is(err, persistence.ErrCurrentReadingStale) || errors.Is(err, persistence.ErrNotFound) {
		redirect(w, r, "/reading?error="+url.QueryEscape(endCurrentReadingStaleMessage))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, "/reading?message="+url.QueryEscape("Reading ended. "+h.currentReadingBookTitle(r.Context(), owner, expectedBookID)+" stays in To Read; its analysis, decks, and history are preserved. Choose a Book when you are ready."))
}

const endCurrentReadingStaleMessage = "The current reading changed before this action. No changes were made; review Reading and try again."

func (h *Handler) reanalyzeToReadBook(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := r.PathValue("id")
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	current, err := h.services.Store.Reading.GetCurrentReading(r.Context(), owner, detail.Book.LanguageTag)
	if err != nil {
		fail(w, err)
		return
	}
	if current.IsActive() && current.BookID == detail.Book.ID {
		rejectCurrentBookRefresh(w, r)
		return
	}
	if detail.Disposition != domain.BookDispositionToRead {
		http.NotFound(w, r)
		return
	}
	if detail.ReadingUnit != nil {
		h.retryClassicalPreparation(w, r, owner, detail.Book.ID)
		return
	}
	handle, target, title, acquisitionFailed, err := h.ensureToReadAnalysis(r.Context(), owner, bookID)
	if err != nil {
		if errors.Is(err, analysis.ErrCurrentBook) {
			rejectCurrentBookRefresh(w, r)
			return
		}
		logToReadAnalysisFailure(owner, bookID, acquisitionFailed, err)
		redirect(w, r, "/reading?error="+url.QueryEscape(reanalysisError(r.Context(), h.services.Store.Catalogs, owner, bookID, title, target, acquisitionFailed, err)))
		return
	}
	redirect(w, r, "/reading?message="+url.QueryEscape(fmt.Sprintf("Analysis job #%d submitted.", handle.DisplayNumber)))
}

func rejectCurrentBookRefresh(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, "/reading?error="+url.QueryEscape("The current book cannot be refreshed. Its frozen evidence remains unchanged."))
}

// retryClassicalPreparation submits a classical Book's preparation again. A
// completed run for the current definition is reported as current rather than
// as a second job.
func (h *Handler) retryClassicalPreparation(w http.ResponseWriter, r *http.Request, owner, bookID string) {
	if h.services.ClassicalPreparation == nil {
		redirect(w, r, "/reading?error="+url.QueryEscape("Classical preparation is unavailable."))
		return
	}
	handle, err := h.services.ClassicalPreparation.SubmitClassicalPreparation(r.Context(), owner, bookID)
	if err != nil {
		stage, code := analysis.ClassicalPreparationFailure(err)
		// Log the stable code and identifiers only. Raw error text could carry
		// source detail, and Greek never belongs in operator logs.
		log.Printf("mouseion: classical preparation retry failed for owner %s book %s stage=%s code=%s", owner, strconv.Quote(bookID), stage, code)
		switch {
		case errors.Is(err, classical.ErrLegacyRows):
			redirect(w, r, "/reading?error="+url.QueryEscape(classical.LegacyRowsReason))
		case errors.Is(err, analysis.ErrWithdrawnClassicalDefinition):
			redirect(w, r, "/reading?error="+url.QueryEscape(classicalWithdrawnRefusal))
		default:
			redirect(w, r, "/reading?error="+url.QueryEscape("Preparation could not be started. Retry when ready."))
		}
		return
	}
	if handle.Complete {
		redirect(w, r, "/reading?message="+url.QueryEscape("The analysis is already current."))
		return
	}
	if handle.Terminal {
		redirect(w, r, "/reading?error="+url.QueryEscape(classicalSourceMismatchRefusal))
		return
	}
	redirect(w, r, "/reading?message="+url.QueryEscape(fmt.Sprintf("Preparation job #%d submitted.", handle.JobID)))
}

func (h *Handler) startReading(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	bookID := strings.TrimSpace(r.PathValue("id"))
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" {
		redirect(w, r, "/reading?error="+url.QueryEscape("Choose a study language before starting a book."))
		return
	}
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	// Flag detection is a write the freeze depends on, so it runs before the
	// Start. Persistence decides everything else.
	if _, err := h.services.LemmaReview.Assess(r.Context(), owner, detail.Book.ID); err != nil {
		fail(w, err)
		return
	}
	start, err := h.services.Store.Reading.StartCurrentReadingResult(r.Context(), owner, language, bookID)
	var ineligible persistence.CurrentReadingIneligibleError
	switch {
	case errors.Is(err, persistence.ErrNotFound):
		http.NotFound(w, r)
		return
	case err == nil && start.Replayed:
		// Starting the Book that is already current is a repeat, not a conflict.
		redirect(w, r, "/reading?message="+url.QueryEscape(h.currentReadingBookTitle(r.Context(), owner, bookID)+" is already your current reading."))
		return
	case errors.Is(err, persistence.ErrCurrentReadingExists):
		redirect(w, r, "/reading?error="+url.QueryEscape("A current book is already set for this language. Review it in Reading before starting another."))
		return
	case errors.As(err, &ineligible) && ineligible.Reason.IsBookChoiceRejection():
		redirect(w, r, "/reading?error="+url.QueryEscape("This book is no longer an eligible To Read candidate. Review Reading before trying again."))
		return
	case errors.Is(err, persistence.ErrCurrentReadingIneligible):
		redirect(w, r, "/reading?error="+url.QueryEscape("This book no longer has trustworthy current analysis or is no longer To Read. No changes were made; refresh Reading and try again."))
		return
	case errors.Is(err, persistence.ErrUnresolvedLemmaReviewFlags):
		redirect(w, r, "/reading/books/"+url.PathEscape(bookID)+"/lemma-review")
		return
	case errors.Is(err, persistence.ErrVocabularyBrowseCountsPending):
		redirect(w, r, "/reading?error="+url.QueryEscape("Across-book vocabulary counts are updating. No reading was changed; try starting again when the counts are ready."))
		return
	case errors.Is(err, persistence.ErrVocabularyBrowseCountsUnavailable):
		redirect(w, r, "/reading?error="+url.QueryEscape("Across-book vocabulary counts are unavailable after repeated rebuild failures. No reading was changed; ask the server operator to restart Mouseion, then retry."))
		return
	case err != nil:
		fail(w, err)
		return
	}
	redirect(w, r, "/reading?message="+url.QueryEscape(h.currentReadingBookTitle(r.Context(), owner, bookID)+" is now your current reading."))
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
		h.annotateReadingChooserWorkFacts(&candidate)
		views = append(views, candidate)
	}
	view.Entries = readingChooserEntries(views)
	return view, nil
}

// readingChooserCandidate reports whether a Book belongs in the chooser at all:
// a non-hidden To Read Book in the active language.
func readingChooserCandidate(book domain.MyBook, language string) bool {
	if book.Disposition != domain.BookDispositionToRead || language == "" || book.Book.LanguageTag != language {
		return false
	}
	// Hidden only keeps a Book out of the default chooser; it never makes the
	// Book ineligible. Recovery is My Books → Show hidden books → Unhide →
	// choose it here.
	return !book.Hidden
}

// annotateReadingChooserWorkFacts copies the reviewed definition's Work identity
// and constituent position onto one candidate. A Book with no current reviewed
// unit keeps no Work facts, so its row stays ordinary.
func (h *Handler) annotateReadingChooserWorkFacts(candidate *readingChooserBookView) {
	work, unit, ok := h.readingBookWork(candidate.Book)
	if !ok {
		return
	}
	candidate.WorkKey = work.Key
	candidate.WorkTitle = work.Title
	candidate.WorkAuthor = work.Author
	candidate.WorkGreekTitle = work.GreekTitle
	candidate.WorkBooks = work.ConstituentBooks
	candidate.Position = unit.ConstituentBook
}

// readingBookWork resolves a Book's reviewed Work and Reading unit. It reads the
// immutable Reading-unit association, never the learner's added Books.
func (h *Handler) readingBookWork(book domain.MyBook) (work classical.CatalogWork, unit classical.CatalogUnit, ok bool) {
	if h.services.Classical == nil || book.ReadingUnit == nil || strings.TrimSpace(book.ReadingUnit.ReadingUnitKey) == "" {
		return classical.CatalogWork{}, classical.CatalogUnit{}, false
	}
	return h.services.Classical.Unit(book.ReadingUnit.ReadingUnitKey)
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

// readingChooserEntries groups multiple To Read Books of one Work into a single
// entry and leaves every other candidate ordinary, then applies the same neutral
// title order across both kinds.
func readingChooserEntries(candidates []readingChooserBookView) []readingChooserEntryView {
	workCounts := map[string]int{}
	for _, candidate := range candidates {
		if candidate.WorkKey != "" && candidate.Position > 0 {
			workCounts[candidate.WorkKey]++
		}
	}
	works := map[string][]readingChooserBookView{}
	entries := make([]readingChooserEntryView, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.WorkKey != "" && candidate.Position > 0 && workCounts[candidate.WorkKey] > 1 {
			works[candidate.WorkKey] = append(works[candidate.WorkKey], candidate)
			continue
		}
		entries = append(entries, readingChooserEntryView{
			Key:        "book:" + candidate.Book.Book.ID,
			Title:      readingChooserBookTitle(candidate.Book),
			Author:     strings.TrimSpace(candidate.Book.Book.Author),
			GreekTitle: candidate.WorkGreekTitle,
			WorkBooks:  candidate.WorkBooks,
			Books:      []readingChooserBookView{candidate},
		})
	}
	for key, books := range works {
		entries = append(entries, readingChooserWorkEntry(key, books))
	}
	sort.Slice(entries, func(i, j int) bool { return readingChooserEntryLess(entries[i], entries[j]) })
	return entries
}

// readingChooserWorkEntry builds one grouped Work entry. Its books stay in
// constituent-book order, independent of how many the learner added.
func readingChooserWorkEntry(key string, books []readingChooserBookView) readingChooserEntryView {
	sort.SliceStable(books, func(i, j int) bool {
		if books[i].Position != books[j].Position {
			return books[i].Position < books[j].Position
		}
		return readingChooserLess(books[i], books[j])
	})
	first := books[0]
	entry := readingChooserEntryView{
		Key:        "work:" + key,
		Grouped:    true,
		Title:      first.WorkTitle,
		Author:     first.WorkAuthor,
		GreekTitle: first.WorkGreekTitle,
		WorkBooks:  first.WorkBooks,
		Books:      books,
	}
	if entry.Title == "" {
		entry.Title = readingChooserBookTitle(first.Book)
	}
	if entry.Author == "" {
		entry.Author = strings.TrimSpace(first.Book.Book.Author)
	}
	if entry.WorkBooks <= 0 {
		entry.WorkBooks = len(books)
	}
	entry.Bands = readingChooserWorkBands(books)
	return entry
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

// readingChooserBandOrder is the shipped order of grouped Work band rows.
var readingChooserBandOrder = []string{
	"99% or above",
	"97% to below 99%",
	"95% to below 97%",
	"Below 95%",
	"No vocabulary comparison",
	"Analysis in progress or Updating",
	"Not assessed",
}

// readingChooserWorkBands lists one row per shipped band in shipped order,
// omitting a band with no candidate. The books inside a row keep book order.
func readingChooserWorkBands(books []readingChooserBookView) []readingChooserBandView {
	rows := make([]readingChooserBandView, 0, len(readingChooserBandOrder))
	for _, band := range readingChooserBandOrder {
		matching := make([]readingChooserBookView, 0, len(books))
		for _, item := range books {
			if readingChooserBand(item) == band {
				matching = append(matching, item)
			}
		}
		if len(matching) > 0 {
			rows = append(rows, readingChooserBandView{Label: band, Books: matching})
		}
	}
	return rows
}

// readingChooserBand names the grouped row one candidate belongs to.
func readingChooserBand(item readingChooserBookView) string {
	if item.Coverage != nil {
		return readingChooserBandLabel(item.Band)
	}
	switch item.State {
	case readingChooserInProgress, readingChooserUpdating:
		return "Analysis in progress or Updating"
	case readingChooserNeedsAttention:
		return "Not assessed"
	}
	return ""
}

func readingChooserLess(left, right readingChooserBookView) bool {
	leftTitle, rightTitle := strings.ToLower(readingChooserBookTitle(left.Book)), strings.ToLower(readingChooserBookTitle(right.Book))
	if leftTitle != rightTitle {
		return leftTitle < rightTitle
	}
	leftAuthor, rightAuthor := strings.ToLower(strings.TrimSpace(left.Book.Book.Author)), strings.ToLower(strings.TrimSpace(right.Book.Book.Author))
	if leftAuthor != rightAuthor {
		return leftAuthor < rightAuthor
	}
	return left.Book.Book.ID < right.Book.Book.ID
}

func readingChooserBookTitle(book domain.MyBook) string {
	if title := strings.TrimSpace(book.Book.Title); title != "" {
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
	if c.Content == domain.ContentNotAcquired {
		return readingChooserNeedsAttention, "Book content has not been acquired yet. Return to My Books to review its catalog entry."
	}
	if c.PublicationPending() {
		return readingChooserInProgress, analysisPublicationPendingDescription
	}
	if c.Phase == domain.PhaseAnalyzing {
		return readingChooserInProgress, "Analysis is queued or running. This candidate will show its band when current evidence is ready."
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
	count := 0
	for _, entry := range view.Entries {
		count += len(entry.Books)
	}
	return count
}

func readingChooserHasCoverage(view readingChooserPageView) bool {
	for _, entry := range view.Entries {
		for _, item := range entry.Books {
			if item.Coverage != nil {
				return true
			}
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

// readingChooserWorkCounts states the grouped Work's candidate counts: only To
// Read Books appear, and assessed means a coverage reading is available.
func readingChooserWorkCounts(entry readingChooserEntryView) string {
	assessed := 0
	for _, item := range entry.Books {
		if item.Coverage != nil {
			assessed++
		}
	}
	return fmt.Sprintf("%s, %d assessed", classicalCountLabel(len(entry.Books), "To Read Book", "To Read Books"), assessed)
}

// readingChooserWorkCoverageRange is one Known coverage range across the
// grouped Work's numerically assessed Books, to one decimal.
func readingChooserWorkCoverageRange(books []readingChooserBookView) string {
	low, high := 0.0, 0.0
	found := false
	for _, item := range books {
		if item.Coverage == nil {
			continue
		}
		value, ok := readingChooserCoveragePercent(*item.Coverage)
		if !ok {
			continue
		}
		if !found || value < low {
			low = value
		}
		if !found || value > high {
			high = value
		}
		found = true
	}
	if !found {
		return ""
	}
	if low == high {
		return fmt.Sprintf("Known coverage %.1f%%", low)
	}
	return fmt.Sprintf("Known coverage %.1f%%–%.1f%%", low, high)
}

// readingChooserWorkPosition names a constituent Book's place in the reviewed
// Work. A whole-work unit has no position.
func readingChooserWorkPosition(item readingChooserBookView) string {
	if item.Position <= 0 || item.WorkBooks <= 0 {
		return ""
	}
	return fmt.Sprintf("Book %d of %d", item.Position, item.WorkBooks)
}

// readingChooserCandidateTitle adds the constituent number when one Book is
// opened inline inside its Work's grouped entry.
func readingChooserCandidateTitle(item readingChooserBookView, inWork bool) string {
	title := readingChooserBookTitle(item.Book)
	if inWork && item.Position > 0 {
		return fmt.Sprintf("%s, Book %d", title, item.Position)
	}
	return title
}

// readingChooserCandidateAnchorID is the fragment target a grouped Work's band
// cells open inline.
func readingChooserCandidateAnchorID(item readingChooserBookView) string {
	return "reading-chooser-book-" + item.Book.Book.ID
}

// readingChooserBookAccessibleLabel names one band-cell link completely: Work,
// constituent Book and its evidence.
func readingChooserBookAccessibleLabel(item readingChooserBookView) string {
	identity := readingChooserBookTitle(item.Book)
	if item.Position > 0 {
		identity = fmt.Sprintf("%s, Book %d", identity, item.Position)
	}
	evidence := readingChooserBookBandLabel(item)
	if item.Coverage != nil && item.Coverage.AnalyzableTokenCount > 0 {
		evidence = "Known coverage " + readingChooserCoverageDisplay(*item.Coverage)
	}
	return identity + ", " + evidence
}

// readingChooserBookNumber is the visible band-cell text: the constituent book
// number only, never a coverage figure.
func readingChooserBookNumber(item readingChooserBookView) string {
	if item.Position <= 0 {
		return ""
	}
	return strconv.Itoa(item.Position)
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

// readingChooserCoveragePercent is the one clamped percentage both a Book's
// margin and its Work's range use.
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
