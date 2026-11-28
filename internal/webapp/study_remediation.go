package webapp

import (
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
)

// studyRemediationView is the remediation screen of one open Wrong-word report:
// the reported occurrence, the decisions open on it and, once one is previewed,
// its consequence (ADR 0136 §6).
type studyRemediationView struct {
	Report     domain.StudyReport
	Language   string
	BookTitle  string
	Path       string
	Occurrence domain.LemmaReviewOccurrence
	// Found is false when the report's occurrence is no longer in the Book's
	// analysis; the learner can only dismiss the report then.
	Found   bool
	Preview *lemmareview.Preview
	// Books are the other occurrences in the Corpus of the reported word's
	// form and lemma, by Book, none ticked. MatchPreviews are the previews of
	// the ticked ones, one per Book, each decided in its own Book's analysis.
	Books         []studyRemediationBook
	MatchPreviews []studyRemediationMatchPreview
}

// studyRemediationBook is one Book's matching occurrences.
type studyRemediationBook struct {
	ID, Title string
	Matches   []studyRemediationMatch
}

// studyRemediationMatch is one matching occurrence a learner may tick.
type studyRemediationMatch struct {
	Value, Sentence string
}

// studyRemediationMatchPreview is the preview of the ticked occurrences of one
// Book.
type studyRemediationMatchPreview struct {
	BookID, Title string
	Preview       lemmareview.Preview
}

// matchValue names a ticked occurrence by its Book, which the occurrence's own
// identity does not carry.
func matchValue(bookID, occurrenceID string) string { return bookID + "/" + occurrenceID }

// tickedMatches reads the matches the learner ticked, by Book. Only an
// occurrence the Corpus offers for this word counts, and only in a Book whose
// vocabulary can change. A tick the Corpus no longer offers is rejected, never
// dropped, so a decision is never saved without one the learner asked for.
func tickedMatches(r *http.Request, books []studyRemediationBook) (map[string][]string, error) {
	offered := map[string]bool{}
	for _, book := range books {
		for _, match := range book.Matches {
			offered[match.Value] = true
		}
	}
	ticked := map[string][]string{}
	for _, value := range r.Form["match"] {
		bookID, occurrenceID, ok := strings.Cut(value, "/")
		if !ok || !offered[value] {
			return nil, domain.ErrLemmaReviewOccurrenceUnavailable
		}
		if !slices.Contains(ticked[bookID], occurrenceID) {
			ticked[bookID] = append(ticked[bookID], occurrenceID)
		}
	}
	return ticked, nil
}

// studyRemediationPath is the remediation screen of a report.
func studyRemediationPath(reportID string) string {
	return "/study/reports/" + url.PathEscape(reportID) + "/remediation"
}

// openWrongWordReport reads the learner's open Wrong-word report the path
// names. Any other report is not found; the response is written and false
// returned.
func (h *Handler) openWrongWordReport(w http.ResponseWriter, r *http.Request) (domain.StudyReport, bool) {
	report, err := h.services.Store.Study.GetStudyReport(r.Context(), user(r).ID, r.PathValue("id"))
	if errors.Is(err, domain.ErrNotFound) || (err == nil && (!report.Open() || report.Kind != domain.StudyReportWrongWord)) {
		http.NotFound(w, r)
		return domain.StudyReport{}, false
	}
	if err != nil {
		fail(w, err)
		return domain.StudyReport{}, false
	}
	return report, true
}

// reportedOccurrence finds the occurrence a report names among the Book's
// occurrences of its word, by its place in the Book, as Lemma review does.
func (h *Handler) reportedOccurrence(r *http.Request, owner string, report domain.StudyReport) (lemmareview.Review, domain.LemmaReviewOccurrence, bool, error) {
	owned := report
	if located, err := h.services.Store.Study.StudyOccurrenceLocator(r.Context(), owner, report.Source); err == nil {
		owned.Source = located
	}
	review, err := h.services.LemmaReview.Review(r.Context(), owner, report.Source.BookID, report.TargetWord)
	if err != nil {
		return review, domain.LemmaReviewOccurrence{}, false, err
	}
	for _, occurrence := range review.Occurrences {
		if lemmaReviewReported(owned, occurrence) {
			return review, occurrence, true, nil
		}
	}
	return review, domain.LemmaReviewOccurrence{}, false, nil
}

// studyRemediation serves the remediation screen: the reported occurrence with
// the Lemma correction and the Occurrence exclusion open on it.
func (h *Handler) studyRemediation(w http.ResponseWriter, r *http.Request) {
	report, ok := h.openWrongWordReport(w, r)
	if !ok {
		return
	}
	view, err := h.studyRemediationView(r, report)
	if err != nil {
		writeLemmaDecisionError(w, r, err)
		return
	}
	render(w, r, StudyRemediationPage(user(r), h.csrf(w, r), view))
}

func (h *Handler) studyRemediationView(r *http.Request, report domain.StudyReport) (studyRemediationView, error) {
	owner := user(r).ID
	detail, err := h.services.Store.Reading.GetBookDetail(r.Context(), owner, report.Source.BookID)
	if err != nil {
		return studyRemediationView{}, err
	}
	adoptDetailLanguage(r.Context(), detail)
	_, occurrence, found, err := h.reportedOccurrence(r, owner, report)
	if err != nil {
		return studyRemediationView{}, err
	}
	view := studyRemediationView{
		Report: report, Language: report.Language, BookTitle: detail.Book.Title, Path: studyRemediationPath(report.ID),
		Occurrence: occurrence, Found: found,
	}
	if found {
		matches, err := h.services.LemmaReview.CorpusMatches(r.Context(), owner, report.Language, occurrence)
		if err != nil {
			return studyRemediationView{}, err
		}
		view.Books = remediationBooks(matches)
	}
	return view, nil
}

// remediationBooks groups matches by Book, in the order the Corpus lists them.
func remediationBooks(matches []lemmareview.CorpusMatch) []studyRemediationBook {
	var books []studyRemediationBook
	for _, match := range matches {
		id := match.Occurrence.BookID
		if len(books) == 0 || books[len(books)-1].ID != id {
			books = append(books, studyRemediationBook{ID: id, Title: match.BookTitle})
		}
		last := &books[len(books)-1]
		last.Matches = append(last.Matches, studyRemediationMatch{Value: matchValue(id, match.Occurrence.ID()), Sentence: match.Occurrence.SentenceText})
	}
	return books
}

// decideStudyRemediation takes the preview and the confirmation of a decision
// on the reported occurrence. Both run Lemma review's own steps, on that one
// occurrence: any other occurrence the form names is ignored, so a report
// repairs only its own occurrence (ADR 0136 §6).
func (h *Handler) decideStudyRemediation(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	report, ok := h.openWrongWordReport(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid review form.", http.StatusBadRequest)
		return
	}
	if decision := r.FormValue("decision"); decision != "correct" && decision != "exclude" {
		http.Error(w, "Choose a correction or an exclusion to preview.", http.StatusBadRequest)
		return
	}
	owner := user(r).ID
	_, occurrence, found, err := h.reportedOccurrence(r, owner, report)
	if err != nil {
		writeLemmaDecisionError(w, r, err)
		return
	}
	if !found {
		http.Error(w, "The reported occurrence is no longer in this Book's analysis.", http.StatusConflict)
		return
	}
	// The decision is of the report's word, whatever form the request names.
	r.Form.Set("form", report.TargetWord)
	only := []string{occurrence.ID()}
	view, err := h.studyRemediationView(r, report)
	if err != nil {
		writeLemmaDecisionError(w, r, err)
		return
	}
	ticked, err := tickedMatches(r, view.Books)
	if err != nil {
		writeLemmaDecisionError(w, r, err)
		return
	}
	switch r.FormValue("stage") {
	case "preview":
		h.previewStudyRemediation(w, r, view, only, ticked)
	case "confirm":
		h.confirmStudyRemediation(w, r, report, only, ticked)
	default:
		http.Error(w, "Review the proposed consequence before confirming a decision.", http.StatusBadRequest)
	}
}

// previewStudyRemediation previews the decision on the reported occurrence and,
// in each Book with ticked matches, on those matches. Nothing is saved.
func (h *Handler) previewStudyRemediation(w http.ResponseWriter, r *http.Request, view studyRemediationView, only []string, ticked map[string][]string) {
	owner, report := user(r).ID, view.Report
	step, err := h.previewLemmaDecisionStep(r, owner, report.Source.BookID, only)
	if err != nil {
		writeLemmaDecisionError(w, r, err)
		return
	}
	view.Preview = &step.Preview
	for _, book := range view.Books {
		if len(ticked[book.ID]) == 0 {
			continue
		}
		match, err := h.previewLemmaDecisionStep(r, owner, book.ID, ticked[book.ID])
		if err != nil {
			writeLemmaDecisionError(w, r, err)
			return
		}
		view.MatchPreviews = append(view.MatchPreviews, studyRemediationMatchPreview{BookID: book.ID, Title: book.Title, Preview: match.Preview})
	}
	render(w, r, StudyRemediationPage(user(r), h.csrf(w, r), view))
}

// confirmStudyRemediation saves the decision on the reported occurrence, then
// each Book's ticked matches as that Book's own decision. Every Book's preview
// is checked against what the learner reviewed before anything is saved. A
// decision changes the identities' counts across the Corpus, so each later
// Book's fingerprint is read again once the earlier ones are saved; a Book
// whose state still changed under the request saves nothing and ends the run,
// and the decisions already saved stand.
func (h *Handler) confirmStudyRemediation(w http.ResponseWriter, r *http.Request, report domain.StudyReport, only []string, ticked map[string][]string) {
	owner := user(r).ID
	books := slices.Sorted(maps.Keys(ticked))
	if err := h.checkReviewedMatches(r, owner, books, ticked); err != nil {
		writeLemmaDecisionError(w, r, err)
		return
	}
	err := h.confirmLemmaDecisionStep(r, owner, report.Source.BookID, only)
	if err != nil {
		writeLemmaDecisionError(w, r, err)
		return
	}
	for _, bookID := range books {
		if err := h.confirmMatches(r, owner, bookID, ticked[bookID]); err != nil {
			writeMatchDecisionError(w, r, err)
			return
		}
	}
	redirect(w, r, studyLanguagePath(pageKindStudyReports, report.Language))
}

// checkReviewedMatches previews each Book's ticked matches again and rejects
// the request when one no longer matches what the learner reviewed.
func (h *Handler) checkReviewedMatches(r *http.Request, owner string, books []string, ticked map[string][]string) error {
	reviewed := map[string]string{}
	for _, value := range r.Form["match_fingerprint"] {
		if bookID, fingerprint, ok := strings.Cut(value, "/"); ok {
			reviewed[bookID] = fingerprint
		}
	}
	for _, bookID := range books {
		step, err := h.previewLemmaDecisionStep(r, owner, bookID, ticked[bookID])
		if err != nil {
			return err
		}
		if step.Preview.Fingerprint != reviewed[bookID] {
			return domain.ErrLemmaReviewStale
		}
	}
	return nil
}

// confirmMatches saves one Book's ticked matches against the Book's state now.
func (h *Handler) confirmMatches(r *http.Request, owner, bookID string, occurrenceIDs []string) error {
	step, err := h.previewLemmaDecisionStep(r, owner, bookID, occurrenceIDs)
	if err != nil {
		return err
	}
	r.Form.Set("fingerprint", step.Preview.Fingerprint)
	return h.confirmLemmaDecisionStep(r, owner, bookID, occurrenceIDs)
}

// writeMatchDecisionError answers a match's rejected decision. The reported
// occurrence's decision was already saved, so the workflow's "no decision was
// saved" is not the whole truth.
func writeMatchDecisionError(w http.ResponseWriter, r *http.Request, err error) {
	if rejection, ok := lemmaReviewRejectionFor(err); ok {
		http.Error(w, rejection.message+" The reported occurrence and the matches before this one were saved; repair the rest from another report or review them again.", rejection.status)
		return
	}
	writeLemmaDecisionError(w, r, err)
}
