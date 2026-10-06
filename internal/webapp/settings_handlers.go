package webapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/lemmadisplay"
	"github.com/riverqueue/river/rivertype"
)

const vocabularyBrowseRequestTimeout = 8 * time.Second

func vocabularyBrowseDisplayLemma(language, lemma, upos string) string {
	return lemmadisplay.Format(language, lemma, upos)
}

func vocabularyBrowseOccurrenceCounts(row domain.VocabularyBrowseRow) string {
	return fmt.Sprintf("In this Book: %d; Across analyzed books: %d", row.OccurrenceCount, row.AcrossBooksOccurrenceCount)
}

func (h *Handler) knownVocabPage(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, "/vocabulary")
}

func (h *Handler) vocabularyPage(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	languages, err := h.services.Store.StudyLanguages.ListStudyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	knownLanguages, err := h.services.Store.StudyLanguages.ListKnownVocabularyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if !learnerLanguagePresent(languages, knownLanguages, language) {
		language = ""
	}
	if language == "" {
		render(w, r, VocabularyBrowsePageView(u, h.csrf(w, r), "", domain.VocabularyBrowsePage{}, ""))
		return
	}
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	prefix := r.URL.Query().Get("q")
	values := r.URL.Query()
	browseQuery := domain.VocabularyBrowseQuery{
		// Legacy Book/POS/state/sort parameters are intentionally ignored. Browse
		// is always the complete current-Book identity set in frequency order.
		Prefix: prefix, ReadingBookID: values.Get("reading"), Sort: "occurrences", Page: page,
		Revision: values.Get("rev"), IncludeAll: values.Get("all") == "1",
	}
	// Browse is an interactive request, not a durable background job. Bound the
	// complete read (including the optional selection lookup) so an unusually
	// broad corpus can never leave the learner waiting indefinitely or turn a
	// partially-read result into a successful page.
	browseCtx, cancel := context.WithTimeout(r.Context(), vocabularyBrowseRequestTimeout)
	defer cancel()
	current := domain.CurrentReading{}
	if h.services.Store.CurrentReading != nil {
		current, err = h.services.Store.CurrentReading.GetCurrentReading(browseCtx, u.ID, language)
		if err != nil {
			log.Printf("mouseion: load Current reading for vocabulary Browse: %v", err)
			renderStatus(w, r, vocabularyBrowseErrorStatus(err, browseCtx), VocabularyBrowseErrorPageView(u, h.csrf(w, r), language, browseQuery))
			return
		}
	}
	if browseQuery.ReadingBookID != "" && (!current.IsActive() || browseQuery.ReadingBookID != current.BookID) {
		browseQuery.CurrentBookID = current.BookID
		renderStatus(w, r, http.StatusConflict, VocabularyBrowseChangedPageView(u, h.csrf(w, r), language, browseQuery))
		return
	}
	browse := domain.VocabularyBrowsePage{Page: page}
	if current.IsActive() {
		browseQuery.CurrentBookID = current.BookID
		browseQuery.ReadingBookID = current.BookID
		browse.CurrentBookID = current.BookID
		browse, err = h.services.Store.VocabularyBrowse.ListVocabularyBrowsePage(browseCtx, u.ID, language, browseQuery)
		browse.CurrentBookID = current.BookID
		if err != nil {
			log.Printf("mouseion: load vocabulary Browse: %v", err)
			renderStatus(w, r, vocabularyBrowseErrorStatus(err, browseCtx), VocabularyBrowseErrorPageView(u, h.csrf(w, r), language, browseQuery))
			return
		}
		browse.ReadingBookID = current.BookID
	}
	if current.IsActive() && browseQuery.Revision != "" && browseQuery.Revision != browse.CorpusRevision {
		renderStatus(w, r, http.StatusConflict, VocabularyBrowseChangedPageView(u, h.csrf(w, r), language, browseQuery))
		return
	}
	render(w, r, VocabularyBrowsePageView(u, h.csrf(w, r), language, browse, prefix))
}

func vocabularyBrowseErrorStatus(err error, ctx context.Context) int {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	return http.StatusInternalServerError
}

func vocabularyBrowsePageURL(page int, browse domain.VocabularyBrowsePage, query string) string {
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	if browse.IncludeAll {
		values.Set("all", "1")
	}
	readingBookID := browse.ReadingBookID
	if readingBookID == "" {
		readingBookID = browse.CurrentBookID
	}
	if readingBookID != "" {
		values.Set("reading", readingBookID)
	}
	if browse.CorpusRevision != "" {
		values.Set("rev", browse.CorpusRevision)
	}
	values.Set("page", strconv.Itoa(page))
	return "/vocabulary?" + values.Encode()
}

func (h *Handler) vocabularyImportPage(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	languages, err := h.services.Store.StudyLanguages.ListStudyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	knownLanguages, err := h.services.Store.StudyLanguages.ListKnownVocabularyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if !learnerLanguagePresent(languages, knownLanguages, language) {
		language = ""
	}
	render(w, r, VocabularyPageWithResult(u, h.csrf(w, r), languages, knownLanguages, language, nil, ""))
}

func (h *Handler) importKnownVocab(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	// The request body is capped before parsing, and the multipart memory budget
	// prevents the parser from retaining an unbounded number of form values.
	//nolint:gosec // MaxBytesReader above bounds the complete multipart request.
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		language := knownVocabImportLanguage(r)
		h.renderKnownVocabResult(w, r, language, nil, "The import is too large or could not be read.")
		return
	}
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	language := knownVocabImportLanguage(r)
	language = canonicalization.NormalizeLanguage(language)
	var input bytes.Buffer
	file, header, err := r.FormFile("vocabulary_file")
	if err == nil {
		defer func() {
			if closeErr := file.Close(); closeErr != nil {
				log.Printf("close uploaded vocabulary file: %v", closeErr)
			}
		}()
		if contentType := header.Header.Get("Content-Type"); contentType != "" {
			mediaType, _, mediaErr := mime.ParseMediaType(contentType)
			if mediaErr != nil || (mediaType != "text/plain" && mediaType != "application/octet-stream") {
				h.renderKnownVocabResult(w, r, language, nil, "Choose a UTF-8 plain text file to import.")
				return
			}
		}
		if _, err = io.Copy(&input, file); err != nil {
			h.renderKnownVocabResult(w, r, language, nil, "The uploaded file could not be read.")
			return
		}
	} else if !errors.Is(err, http.ErrMissingFile) {
		h.renderKnownVocabResult(w, r, language, nil, "The uploaded file could not be read.")
		return
	}
	if language == "" {
		h.renderKnownVocabResult(w, r, language, nil, "Choose a language before importing.")
		return
	}
	studyLanguages, err := h.services.Store.StudyLanguages.ListStudyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	selected := false
	for _, studyLanguage := range studyLanguages {
		selected = selected || studyLanguage.Language == language
	}
	if !selected {
		h.renderKnownVocabResult(w, r, language, nil, "Choose a study language present in your library before importing.")
		return
	}
	if input.Len() == 0 {
		h.renderKnownVocabResult(w, r, language, nil, "Choose a non-empty UTF-8 text file to import.")
		return
	}
	if h.services.KnownVocab == nil {
		fail(w, errors.New("known vocabulary service is unavailable"))
		return
	}
	handle, err := h.services.KnownVocab.Submit(r.Context(), u.ID, language, input.String())
	if err != nil {
		h.renderKnownVocabResult(w, r, language, nil, "Import failed: "+err.Error())
		return
	}
	if isPartialHTMXRequest(r) {
		render(w, r, KnownVocabImportStatus(knownvocab.Status{ID: handle.ID, Language: language, State: rivertype.JobStateAvailable}))
		return
	}
	redirect(w, r, fmt.Sprintf("/vocabulary/imports/%d/status", handle.ID))
}

func (h *Handler) knownVocabImportStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	status, err := h.services.KnownVocab.Get(r.Context(), user(r).ID, id)
	if errors.Is(err, knownvocab.ErrJobNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if isPartialHTMXRequest(r) {
		render(w, r, KnownVocabImportStatus(status))
		return
	}
	render(w, r, KnownVocabImportStatusPage(user(r), h.csrf(w, r), status))
}

func (h *Handler) renderKnownVocabResult(w http.ResponseWriter, r *http.Request, language string, result *knownvocab.ImportResult, message string) {
	if isPartialHTMXRequest(r) {
		render(w, r, KnownVocabResult(result, message))
		return
	}
	u := user(r)
	studyLanguages, err := h.services.Store.StudyLanguages.ListStudyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	knownLanguages, err := h.services.Store.StudyLanguages.ListKnownVocabularyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if !learnerLanguagePresent(studyLanguages, knownLanguages, language) {
		language = ""
	}
	render(w, r, VocabularyPageWithResult(u, h.csrf(w, r), studyLanguages, knownLanguages, language, result, message))
}

func knownVocabImportLanguage(r *http.Request) string {
	language, _ := activeStudyLanguageForContext(r.Context())
	return language
}

func knownVocabImportAction() string {
	return "/vocabulary/import"
}

func knownVocabImportRecoveryTarget() string {
	return "/vocabulary/import"
}

func knownVocabJobLabel(status string) string {
	switch status {
	case "available", "scheduled", "retryable", "pending":
		return "Processing queued"
	case "running":
		return "Processing"
	case "completed":
		return "Complete"
	case "discarded":
		return "Failed"
	case "cancelled":
		return "Cancelled"
	default:
		return status
	}
}

func knownVocabJobBusy(status string) bool {
	switch status {
	case "completed", "cancelled", "discarded":
		return false
	default:
		return true
	}
}
