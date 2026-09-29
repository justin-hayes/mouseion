package webapp

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lexical"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (h *Handler) lemmaReview(w http.ResponseWriter, r *http.Request) {
	owner := user(r)
	bookID := strings.TrimSpace(r.PathValue("bookID"))
	store := h.services.Store.LemmaReview
	if store == nil {
		http.Error(w, "Occurrence review is unavailable.", http.StatusServiceUnavailable)
		return
	}
	detail, err := h.services.Store.Books.GetBookDetail(r.Context(), owner.ID, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" || detail.Book.LanguageTag != language {
		http.Error(w, "Choose this Book's study language in Reading before reviewing occurrences.", http.StatusConflict)
		return
	}
	current, err := h.services.Store.CurrentReading.GetCurrentReading(r.Context(), owner.ID, language)
	if err != nil {
		fail(w, err)
		return
	}
	canCorrect := !(current.IsActive() && current.BookID == bookID)
	form := strings.TrimSpace(r.URL.Query().Get("form"))
	var occurrences []domain.LemmaReviewOccurrence
	if form != "" {
		occurrences, err = store.ListLemmaReviewOccurrences(r.Context(), owner.ID, bookID, form)
		if err != nil {
			fail(w, err)
			return
		}
	}
	pageError := ""
	if !canCorrect {
		pageError = "This Book is current reading. Stop reading before changing its vocabulary; the frozen reading snapshot remains unchanged."
	}
	render(w, r, LemmaReviewPage(owner, h.csrf(w, r), bookID, detail.Book.Title, form, pageError, canCorrect, occurrences))
}

func (h *Handler) correctLemma(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r)
	bookID := strings.TrimSpace(r.PathValue("bookID"))
	store := h.services.Store.LemmaReview
	if store == nil {
		http.Error(w, "Occurrence review is unavailable.", http.StatusServiceUnavailable)
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	profile, err := canonicalization.For(language)
	if language == "" || err != nil {
		http.Error(w, "Choose a supported study language before correcting a lemma.", http.StatusConflict)
		return
	}
	action := r.FormValue("action")
	excluded := action == "exclude"
	lemma := profile.Canonical(strings.TrimSpace(r.FormValue("lemma")))
	form := strings.TrimSpace(r.FormValue("form"))
	occurrences, err := store.ListLemmaReviewOccurrences(r.Context(), owner.ID, bookID, form)
	if err != nil {
		fail(w, err)
		return
	}
	start, startErr := strconv.ParseInt(r.FormValue("start_offset"), 10, 64)
	end, endErr := strconv.ParseInt(r.FormValue("end_offset"), 10, 64)
	var chosen *domain.LemmaReviewOccurrence
	for i := range occurrences {
		candidate := &occurrences[i]
		if candidate.AnalysisRunID == r.FormValue("analysis_run_id") && candidate.SourceDocumentID == r.FormValue("source_document_id") && candidate.StartOffset == start && candidate.EndOffset == end && candidate.Surface == r.FormValue("surface") && candidate.RawLemma == r.FormValue("raw_lemma") && candidate.CanonicalLemma == r.FormValue("canonical_lemma") && candidate.UPOS == r.FormValue("upos") {
			chosen = candidate
			break
		}
	}
	if startErr != nil || endErr != nil || chosen == nil {
		http.Error(w, "This occurrence changed or belongs to an older analysis. No correction was saved; find it again in Reading.", http.StatusConflict)
		return
	}
	expectedExcluded, parseErr := strconv.ParseBool(r.FormValue("expected_excluded"))
	if parseErr != nil || r.FormValue("expected_corrected_lemma") != chosen.CorrectedLemma || expectedExcluded != chosen.Excluded {
		http.Error(w, "This occurrence's correction changed after you opened it. No correction was saved; review it again in Reading.", http.StatusConflict)
		return
	}
	if action == "keep" || action == "restore" {
		lemma = chosen.CanonicalLemma
	} else if action != "exclude" && !lexical.IsLemma(lemma) {
		http.Error(w, "Enter one valid canonical lemma without spaces.", http.StatusBadRequest)
		return
	}
	if err := store.PutLemmaDecision(r.Context(), *chosen, lemma, excluded, profile.Name(), profile.Version()); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			http.Error(w, "This occurrence changed or belongs to an older analysis. No correction was saved; find it again in Reading.", http.StatusConflict)
			return
		}
		fail(w, err)
		return
	}
	redirect(w, r, "/reading/books/"+url.PathEscape(bookID)+"/lemma-review?form="+url.QueryEscape(form))
}
