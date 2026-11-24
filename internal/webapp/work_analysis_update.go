package webapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/webauth"
)

const (
	workAnalysisUpdatePublished = "Each Book's published analysis stays in effect until its own update publishes."
	workAnalysisUpdateFrozen    = "Prepared decks and earlier Readings keep the evidence they froze."
)

// workAnalysisUpdateBook is one eligible Book and the offer its confirmation
// carries back to the server.
type workAnalysisUpdateBook struct {
	Book      domain.MyBook
	Candidate domain.AnalysisUpdateCandidate
}

// workAnalysisUpdate is one grouped Work row's Analysis update offer. The
// current Reading's Book is kept apart because it is named but never queued.
type workAnalysisUpdate struct {
	WorkKey, WorkTitle string
	Books              []workAnalysisUpdateBook
	Current            *domain.MyBook
	Corrections        int
	Exclusions         int
	// ChangeNotes are the distinct change notes of the Books' target
	// interpretation versions (ADR 0126), in Book order.
	ChangeNotes []string
}

// workAnalysisUpdateFor groups the eligible Books of one Work that the issuing
// surface shows. My Books passes the grouped row's visible cells; the chooser
// passes its visible To Read candidates. current may be supplied when the
// caller's candidate list deliberately excludes the current Reading's Book, as
// the chooser does in switch mode.
func workAnalysisUpdateFor(workKey, workTitle string, books []domain.MyBook, current *domain.MyBook) workAnalysisUpdate {
	update := workAnalysisUpdate{WorkKey: workKey, WorkTitle: workTitle}
	seen := make(map[string]bool, len(books)+1)
	consider := func(book domain.MyBook) {
		if seen[book.Book.ID] {
			return
		}
		seen[book.Book.ID] = true
		offer, candidate := myBookAnalysisUpdate(book)
		switch offer {
		case domain.AnalysisUpdateAvailable:
			update.Books = append(update.Books, workAnalysisUpdateBook{Book: book, Candidate: *candidate})
			update.Corrections += candidate.LemmaCorrections
			update.Exclusions += candidate.OccurrenceExclusions
			if note, ok := analysisUpdateChangeNote(candidate.Target); ok && !slices.Contains(update.ChangeNotes, note) {
				update.ChangeNotes = append(update.ChangeNotes, note)
			}
		case domain.AnalysisUpdateEndReadingFirst:
			value := book
			update.Current = &value
		case domain.AnalysisUpdateNone:
		}
	}
	for _, book := range books {
		consider(book)
	}
	if current != nil {
		consider(*current)
	}
	if update.WorkTitle == "" {
		update.WorkTitle = workAnalysisUpdateTitle(books, current)
	}
	return update
}

func workAnalysisUpdateTitle(books []domain.MyBook, current *domain.MyBook) string {
	fallback := ""
	for _, book := range books {
		if unit := book.ReadingUnit; unit != nil {
			if strings.TrimSpace(unit.WorkTitle) != "" {
				return unit.WorkTitle
			}
		}
		if fallback == "" {
			fallback = myBookDisplayTitle(book)
		}
	}
	if fallback != "" {
		return fallback
	}
	if current != nil {
		return myBookDisplayTitle(*current)
	}
	return ""
}

// Offered reports whether the Work has anything to name.
func (u workAnalysisUpdate) Offered() bool {
	return len(u.Books) > 0 || u.Current != nil
}

// Notice names the grouped offer. A Work whose only outdated Book is the
// current Reading keeps the ordinary notice and adds the End wording.
func (u workAnalysisUpdate) Notice() string {
	if len(u.Books) == 0 {
		return analysisUpdateNotice
	}
	return fmt.Sprintf("A newer analysis is available for %s shown here.", countNoun(len(u.Books), "book", "books"))
}

// ActionLabel names the one bulk action. A single-Book Work never uses this
// component; a grouped row can still have one eligible Book.
func (u workAnalysisUpdate) ActionLabel() string {
	return fmt.Sprintf("Update %s of %s", countNoun(len(u.Books), "book", "books"), u.WorkTitle)
}

// CurrentText names the skipped current Book, using its reviewed constituent
// number when it has one.
func (u workAnalysisUpdate) CurrentText() string {
	if u.Current == nil {
		return ""
	}
	if unit := u.Current.ReadingUnit; unit != nil && unit.ConstituentBook > 0 {
		return fmt.Sprintf("Book %d: End the Reading first", unit.ConstituentBook)
	}
	return "This Book: End the Reading first"
}

// ConfirmationText states the Work-wide totals and the two frozen boundaries,
// led by the change note of each distinct target version that has one. The
// per-Book breakdown is rendered separately, only for Books with decisions.
func (u workAnalysisUpdate) ConfirmationText() string {
	parts := append(slices.Clone(u.ChangeNotes), u.decisionsText(), workAnalysisUpdatePublished, workAnalysisUpdateFrozen)
	if u.Current != nil {
		parts = append(parts, u.CurrentText()+"; nothing is queued for it.")
	}
	return strings.Join(parts, " ")
}

func (u workAnalysisUpdate) decisionsText() string {
	decisions := analysisUpdateDecisionParts(u.Corrections, u.Exclusions)
	if len(decisions) == 0 {
		return "No Book in this work has confirmed lemma corrections or occurrence exclusions."
	}
	if u.Corrections+u.Exclusions == 1 {
		return "Together, the Books to be updated have " + strings.Join(decisions, " and ") + ", which carries over wherever the word is unchanged in its new analysis."
	}
	return "Together, the Books to be updated have " + strings.Join(decisions, " and ") + ", which carry over wherever the word is unchanged in their new analyses."
}

// workAnalysisUpdateDecisions is one Book's decisions in the confirmation.
type workAnalysisUpdateDecisions struct {
	BookLabel string
	Text      string
}

// Decisions breaks the totals down only for Books that have corrections or
// exclusions.
func (u workAnalysisUpdate) Decisions() []workAnalysisUpdateDecisions {
	breakdown := make([]workAnalysisUpdateDecisions, 0, len(u.Books))
	for _, item := range u.Books {
		decisions := analysisUpdateDecisionParts(item.Candidate.LemmaCorrections, item.Candidate.OccurrenceExclusions)
		if len(decisions) == 0 {
			continue
		}
		breakdown = append(breakdown, workAnalysisUpdateDecisions{
			BookLabel: myBookDisplayTitle(item.Book),
			Text:      strings.Join(decisions, " and "),
		})
	}
	return breakdown
}

func workAnalysisUpdateURL(workKey string) string {
	return "/library/works/" + url.PathEscape(workKey) + "/analysis-update"
}

// workAnalysisUpdateExpectation is one confirmed Book's offer. The source is
// carried too, so a Book whose acquisition changed is skipped rather than
// queued against different content.
type workAnalysisUpdateExpectation struct {
	SourceID       string
	PublishedRunID string
	Target         string
}

// updateWorkAnalysis queues one ensure-once classical preparation job per
// confirmed Book in constituent-book order. Each Book is revalidated
// separately: a changed offer, source, current Reading or disposition is
// skipped while the rest go ahead.
func (h *Handler) updateWorkAnalysis(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r).ID
	workKey := r.PathValue("workKey")
	returnTo := analysisUpdateReturnURL(r.FormValue("return_to"))
	books, err := h.workBooks(r.Context(), owner, workKey)
	if errors.Is(err, domain.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	expectations, ok := workAnalysisUpdateExpectations(r)
	if !ok || h.services.AnalysisUpdates == nil {
		redirectAnalysisUpdateNotice(w, r, returnTo, "error", analysisUpdateChangedMessage)
		return
	}
	h.annotateAnalysisUpdates(r.Context(), owner, books)
	outcome := h.submitWorkAnalysisUpdates(r.Context(), owner, books, expectations)
	message := workAnalysisUpdateMessage(outcome.Queued, outcome.Skipped, outcome.CurrentSkipped, outcome.Failed)
	notice := "message"
	if outcome.Queued == 0 {
		notice = "error"
	}
	redirectAnalysisUpdateNotice(w, r, returnTo, notice, message)
}

// workAnalysisUpdateOutcome counts the independent per-Book results of one
// Work confirmation.
type workAnalysisUpdateOutcome struct {
	Queued, Skipped, CurrentSkipped, Failed int
}

// submitWorkAnalysisUpdates visits Books in constituent-book order. Every Book
// is revalidated on its own, so one stale tab does not block the rest.
func (h *Handler) submitWorkAnalysisUpdates(ctx context.Context, owner string, books []domain.MyBook, expectations map[string]workAnalysisUpdateExpectation) workAnalysisUpdateOutcome {
	var outcome workAnalysisUpdateOutcome
	for _, book := range books {
		expected, submitted := expectations[book.Book.ID]
		if !submitted {
			continue
		}
		switch h.submitWorkAnalysisBook(ctx, owner, book, expected) {
		case workAnalysisUpdateQueued:
			outcome.Queued++
		case workAnalysisUpdateSkipped:
			outcome.Skipped++
		case workAnalysisUpdateCurrentSkipped:
			outcome.CurrentSkipped++
		case workAnalysisUpdateFailed:
			outcome.Failed++
		}
	}
	return outcome
}

type workAnalysisUpdateResult int

const (
	workAnalysisUpdateQueued workAnalysisUpdateResult = iota
	workAnalysisUpdateSkipped
	workAnalysisUpdateCurrentSkipped
	workAnalysisUpdateFailed
)

// submitWorkAnalysisBook revalidates one confirmed Book and queues its
// ensure-once preparation only when the offer still names exactly this Book.
func (h *Handler) submitWorkAnalysisBook(ctx context.Context, owner string, book domain.MyBook, expected workAnalysisUpdateExpectation) workAnalysisUpdateResult {
	if book.Acquired == nil || book.Acquired.Source.ID != expected.SourceID {
		return workAnalysisUpdateSkipped
	}
	offer, _ := myBookAnalysisUpdate(book)
	switch offer {
	case domain.AnalysisUpdateAvailable:
	case domain.AnalysisUpdateEndReadingFirst:
		return workAnalysisUpdateCurrentSkipped
	case domain.AnalysisUpdateNone:
		return workAnalysisUpdateSkipped
	}
	_, err := h.services.AnalysisUpdates.SubmitAnalysisUpdate(ctx, owner, book.Book.ID, book.Acquired.Source.ID, expected.PublishedRunID, expected.Target)
	switch {
	case err == nil:
		return workAnalysisUpdateQueued
	case errors.Is(err, analysis.ErrCurrentBook):
		return workAnalysisUpdateCurrentSkipped
	case analysisUpdateOfferChanged(err), errors.Is(err, analysis.ErrNotFound):
		return workAnalysisUpdateSkipped
	default:
		log.Printf("mouseion: work analysis update failed for owner %s book %s: %s", owner, book.Book.ID, err)
		return workAnalysisUpdateFailed
	}
}

func workAnalysisUpdateMessage(queued, skipped, currentSkipped, failed int) string {
	var parts []string
	if queued > 0 {
		parts = append(parts, "Queued analysis updates for "+countNoun(queued, "book", "books")+". "+workAnalysisUpdatePublished)
	} else {
		parts = append(parts, "No analysis update was queued.")
	}
	if skipped > 0 {
		parts = append(parts, "Skipped "+countNoun(skipped, "book", "books")+" whose offer changed; the refreshed offer is shown.")
	}
	if currentSkipped > 0 {
		parts = append(parts, "Skipped "+countNoun(currentSkipped, "current Book", "current Books")+"; end the Reading first.")
	}
	if failed > 0 {
		parts = append(parts, "Could not update "+countNoun(failed, "book", "books")+".")
	}
	return strings.Join(parts, " ")
}

// workAnalysisUpdateExpectations reads the parallel hidden fields the one
// Work confirmation carries. Anything incomplete or contradictory is refused
// as a changed offer rather than guessed.
func workAnalysisUpdateExpectations(r *http.Request) (map[string]workAnalysisUpdateExpectation, bool) {
	bookIDs := r.Form["book_id"]
	sourceIDs := r.Form["source_id"]
	runIDs := r.Form["expected_run_id"]
	targets := r.Form["expected_target"]
	if len(bookIDs) == 0 || len(bookIDs) != len(sourceIDs) || len(bookIDs) != len(runIDs) || len(bookIDs) != len(targets) {
		return nil, false
	}
	expectations := make(map[string]workAnalysisUpdateExpectation, len(bookIDs))
	for i, bookID := range bookIDs {
		bookID = strings.TrimSpace(bookID)
		sourceID := strings.TrimSpace(sourceIDs[i])
		runID := strings.TrimSpace(runIDs[i])
		target := strings.TrimSpace(targets[i])
		if bookID == "" || sourceID == "" || runID == "" || target == "" {
			return nil, false
		}
		if _, duplicate := expectations[bookID]; duplicate {
			return nil, false
		}
		expectations[bookID] = workAnalysisUpdateExpectation{SourceID: sourceID, PublishedRunID: runID, Target: target}
	}
	return expectations, true
}

// analysisUpdateReturnURL keeps a Work confirmation on the surface that issued
// it. Only local My Books and Reading locations are honored.
func analysisUpdateReturnURL(raw string) string {
	path := webauth.SafeReturnPath(strings.TrimSpace(raw))
	parsed, err := url.Parse(path)
	if err != nil {
		return "/library"
	}
	if parsed.Path == "/library" {
		// A stale My Books location falls back to My Books like every other
		// My Books return.
		return myBooksReturnURL(path)
	}
	if strings.HasPrefix(parsed.Path, "/library/") ||
		parsed.Path == "/reading" || parsed.Path == "/reading/switch" {
		return path
	}
	return "/library"
}

func redirectAnalysisUpdateNotice(w http.ResponseWriter, r *http.Request, returnTo, key, text string) {
	parsed, err := url.Parse(returnTo)
	if err != nil {
		parsed = &url.URL{Path: "/library"}
	}
	values := parsed.Query()
	values.Set(key, text)
	parsed.RawQuery = values.Encode()
	redirect(w, r, parsed.String())
}

// workBooks loads every owned Book of one reviewed Work in constituent-book
// order, including Books outside the current filter. Work-level actions
// therefore act on the whole Work while disposition stays per Book.
func (h *Handler) workBooks(ctx context.Context, owner, workKey string) ([]domain.MyBook, error) {
	workKey = strings.TrimSpace(workKey)
	if workKey == "" {
		return nil, domain.ErrNotFound
	}
	books, err := h.services.Store.MyBooks.ListMyBooksWithEvidence(ctx, owner)
	if err != nil {
		return nil, err
	}
	work := make([]domain.MyBook, 0, len(books))
	for _, book := range books {
		if book.ReadingUnit != nil && book.ReadingUnit.WorkKey == workKey {
			work = append(work, book)
		}
	}
	if len(work) == 0 {
		return nil, domain.ErrNotFound
	}
	sort.SliceStable(work, func(i, j int) bool {
		return domain.MyBookWorkOrderLess(work[i], work[j])
	})
	return work, nil
}
