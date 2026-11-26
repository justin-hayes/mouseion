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
			entry.Before, entry.Target, entry.After, entry.Marked = markTarget(report.Sentence, report.TargetWord)
			view.Reports = append(view.Reports, entry)
		}
	}
	render(w, r, StudyReportsPage(user(r), h.csrf(w, r), view))
}

// studyReportBookTitles names the Books the reports' occurrences are in. A Book
// that has left the library is named as such.
func (h *Handler) studyReportBookTitles(ctx context.Context, owner string, reports []domain.StudyReport) map[string]string {
	titles := map[string]string{}
	for _, report := range reports {
		bookID := report.Source.BookID
		if _, done := titles[bookID]; done {
			continue
		}
		titles[bookID] = "A Book no longer in your library"
		if book, err := h.services.Store.Reading.GetBook(ctx, owner, bookID); err == nil && strings.TrimSpace(book.Title) != "" {
			titles[bookID] = book.Title
		}
	}
	return titles
}

// reportStudyCard records the learner's report of the card they were shown. A
// reported review is not graded: no grade, review-log row or schedule change is
// written. A Wrong-word report advances to the next card. An
// Unsuitable-context report replaces the word's context and shows the reported
// card once more, so the learner can see what was reported (decision on Undo,
// ADR 0135 §3). Like a grade, a report of a card that is no longer the one due
// is ignored.
func (h *Handler) reportStudyCard(w http.ResponseWriter, r *http.Request) {
	language, back, ok := h.studyForm(w, r)
	if !ok {
		return
	}
	kind, kindOK := domain.ParseStudyReportKind(r.FormValue("kind"))
	reps, repsErr := strconv.ParseInt(r.FormValue("reps"), 10, 64)
	progressID := strings.TrimSpace(r.FormValue("progress"))
	note, noteOK := domain.CleanStudyReportNote(r.FormValue("note"))
	if !kindOK || repsErr != nil || reps < 0 || progressID == "" || !noteOK {
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
	// A form for a card whose material was replaced since is stale, though the
	// replacement keeps the word's reps.
	if material := strings.TrimSpace(r.FormValue("material")); (material != "" && material != current.MaterialID) || current.Memory.Reps != reps || !domain.StudyCardIsDue(*current, now) {
		redirect(w, r, back)
		return
	}
	report, err := store.CreateStudyReport(r.Context(), owner, domain.StudyReportRecord{ProgressID: progressID, ExpectedReps: reps, Kind: kind, Note: note, CreatedAt: now})
	if err != nil && !errors.Is(err, domain.ErrStudyStale) && !errors.Is(err, domain.ErrNotFound) {
		fail(w, err)
		return
	}
	if err == nil && kind == domain.StudyReportUnsuitableContext {
		redirect(w, r, back+"?"+url.Values{studyReportedParam: {report.ID}}.Encode())
		return
	}
	redirect(w, r, back)
}

// studyReportedParam names the report whose card the Study page shows once more
// after an Unsuitable-context report.
const studyReportedParam = "reported"

// studyReportedView is the card of an Unsuitable-context report, shown after
// the report with its answer side: the sentence, Gloss and translation that were
// rejected. The word has already been replaced; the next card is a Next away.
// Undo sits beside Next.
type studyReportedView struct {
	cardFace
	ReportID string
}

// loadReportedCard reads the report the Study page was asked to show once more.
// It is nothing unless the report is the learner's open Unsuitable-context
// report in this language.
func (h *Handler) loadReportedCard(r *http.Request, owner string, view *studyPageView) error {
	id := strings.TrimSpace(r.URL.Query().Get(studyReportedParam))
	if id == "" {
		return nil
	}
	report, err := h.services.Store.Study.GetStudyReport(r.Context(), owner, id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if report.Kind != domain.StudyReportUnsuitableContext || !report.Open() || report.Language != view.Language {
		return nil
	}
	face := cardFace{
		ShowAnswer: true, Headword: lemmadisplay.Format(view.Language, report.CanonicalLemma, report.UPOS),
		Gloss: report.Gloss, Translation: report.SentenceTranslation,
	}
	face.Before, face.Target, face.After, face.Marked = markTarget(report.Sentence, report.TargetWord)
	view.Reported = &studyReportedView{cardFace: face, ReportID: report.ID}
	return nil
}
