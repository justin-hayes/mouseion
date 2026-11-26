package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lemmadisplay"
	"github.com/justin-hayes/mouseion/internal/study"
)

// studyReportsView is the Reported words page of one study language.
type studyReportsView struct {
	Language string
	Reports  []studyReportEntry
}

// studyReportEntry is one open Wrong-word report: the word, the reported
// sentence with its target word marked, the Book, the date and the note.
type studyReportEntry struct {
	ID, Word              string
	Before, Target, After string
	Marked                bool
	BookTitle, Date, Note string
	// ReviewPath opens the Book's Lemma review at the reported occurrence; it
	// is empty when the Book has left the library.
	ReviewPath string
}

// studyReports serves /{language}/study/reports: the learner's open Wrong-word
// reports in the study language, oldest first (ADR 0135 §2).
func (h *Handler) studyReports(w http.ResponseWriter, r *http.Request) {
	owner := user(r).ID
	language, _ := activeStudyLanguageForContext(r.Context())
	view := studyReportsView{Language: language}
	if study.Available(language) {
		reports, err := h.services.Store.Study.ListOpenStudyReports(r.Context(), owner, language, domain.StudyReportWrongWord)
		if err != nil {
			fail(w, err)
			return
		}
		titles := h.studyReportBookTitles(r.Context(), owner, reports)
		for _, report := range reports {
			entry := studyReportEntry{
				ID: report.ID, Word: lemmadisplay.Format(language, report.CanonicalLemma, report.UPOS),
				BookTitle: titles[report.Source.BookID], Date: domain.StudyDay(report.CreatedAt), Note: report.Note,
			}
			if titles[report.Source.BookID] != studyReportMissingBook {
				entry.ReviewPath = "/reading/books/" + url.PathEscape(report.Source.BookID) + "/lemma-review?form=" + url.QueryEscape(report.TargetWord)
			}
			entry.Before, entry.Target, entry.After, entry.Marked = markTarget(report.Sentence, report.TargetWord)
			view.Reports = append(view.Reports, entry)
		}
	}
	render(w, r, StudyReportsPage(user(r), h.csrf(w, r), view))
}

// studyReportMissingBook names a Book that has left the library.
const studyReportMissingBook = "A Book no longer in your library"

// studyReportBookTitles names the Books the reports' occurrences are in. A Book
// that has left the library is named as such.
func (h *Handler) studyReportBookTitles(ctx context.Context, owner string, reports []domain.StudyReport) map[string]string {
	titles := map[string]string{}
	for _, report := range reports {
		bookID := report.Source.BookID
		if _, done := titles[bookID]; done {
			continue
		}
		titles[bookID] = studyReportMissingBook
		if book, err := h.services.Store.Reading.GetBook(ctx, owner, bookID); err == nil && strings.TrimSpace(book.Title) != "" {
			titles[bookID] = book.Title
		}
	}
	return titles
}

// reportStudyCard records the learner's report of the card they were shown. A
// reported review is not graded: no grade, review-log row or schedule change is
// written. Only a Wrong-word report is accepted until Unsuitable context
// exists. Like a grade, a report of a card that is no longer the one due is
// ignored.
func (h *Handler) reportStudyCard(w http.ResponseWriter, r *http.Request) {
	language, back, ok := h.studyForm(w, r)
	if !ok {
		return
	}
	kind, kindOK := domain.ParseStudyReportKind(r.FormValue("kind"))
	reps, repsErr := strconv.ParseInt(r.FormValue("reps"), 10, 64)
	progressID := strings.TrimSpace(r.FormValue("progress"))
	note, noteOK := domain.CleanStudyReportNote(r.FormValue("note"))
	if !kindOK || kind != domain.StudyReportWrongWord || repsErr != nil || reps < 0 || progressID == "" || !noteOK {
		http.Error(w, "invalid report", http.StatusBadRequest)
		return
	}
	owner, store, now := user(r).ID, h.services.Store.Study, h.now()
	progress, err := store.ListStudyProgress(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	var current *domain.StudyProgress
	for i := range progress {
		if progress[i].ID == progressID {
			current = &progress[i]
		}
	}
	if current == nil {
		http.NotFound(w, r)
		return
	}
	if current.Memory.Reps != reps || !domain.StudyCardIsDue(*current, now) {
		redirect(w, r, back)
		return
	}
	_, err = store.CreateStudyReport(r.Context(), owner, domain.StudyReportRecord{ProgressID: progressID, ExpectedReps: reps, Kind: kind, Note: note, CreatedAt: now})
	if err != nil && !errors.Is(err, domain.ErrStudyStale) && !errors.Is(err, domain.ErrNotFound) {
		fail(w, err)
		return
	}
	redirect(w, r, back)
}
