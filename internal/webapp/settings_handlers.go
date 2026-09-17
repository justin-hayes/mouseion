package webapp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/riverqueue/river/rivertype"
)

func (h *Handler) knownVocabPage(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, "/vocabulary")
}

func (h *Handler) vocabularyPage(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	languages, err := h.services.Store.ListStudyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	knownLanguages, err := h.services.Store.ListKnownVocabularyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if !learnerLanguagePresent(languages, knownLanguages, language) {
		language = ""
	}
	var known []domain.KnownVocabulary
	if language != "" {
		known, err = h.services.Store.ListKnownVocabulary(r.Context(), u.ID, language)
		if err != nil {
			fail(w, err)
			return
		}
	}
	render(w, r, VocabularyPageWithResult(u, h.csrf(w, r), languages, knownLanguages, language, nil, known, ""))
}

func (h *Handler) importKnownVocab(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		language := knownVocabImportLanguage(r)
		h.renderKnownVocabResult(w, r, language, nil, nil, "The import is too large or could not be read.")
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
				h.renderKnownVocabResult(w, r, language, nil, nil, "Choose a UTF-8 plain text file to import.")
				return
			}
		}
		if _, err = io.Copy(&input, file); err != nil {
			h.renderKnownVocabResult(w, r, language, nil, nil, "The uploaded file could not be read.")
			return
		}
	} else if !errors.Is(err, http.ErrMissingFile) {
		h.renderKnownVocabResult(w, r, language, nil, nil, "The uploaded file could not be read.")
		return
	}
	if language == "" {
		h.renderKnownVocabResult(w, r, language, nil, nil, "Choose a language before importing.")
		return
	}
	studyLanguages, err := h.services.Store.ListStudyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	selected := false
	for _, studyLanguage := range studyLanguages {
		selected = selected || studyLanguage.Language == language
	}
	if !selected {
		h.renderKnownVocabResult(w, r, language, nil, nil, "Choose a study language present in your library before importing.")
		return
	}
	if input.Len() == 0 {
		h.renderKnownVocabResult(w, r, language, nil, nil, "Choose a non-empty UTF-8 text file to import.")
		return
	}
	if h.services.KnownVocab == nil {
		fail(w, errors.New("known vocabulary service is unavailable"))
		return
	}
	handle, err := h.services.KnownVocab.Submit(r.Context(), u.ID, language, input.String())
	if err != nil {
		h.renderKnownVocabResult(w, r, language, nil, nil, "Import failed: "+err.Error())
		return
	}
	if r.Header.Get("HX-Request") == "true" {
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
	render(w, r, KnownVocabImportStatus(status))
}

func (h *Handler) renderKnownVocabResult(w http.ResponseWriter, r *http.Request, language string, result *knownvocab.ImportResult, known []domain.KnownVocabulary, message string) {
	if known == nil && language != "" {
		var err error
		known, err = h.services.Store.ListKnownVocabulary(r.Context(), user(r).ID, language)
		if err != nil {
			fail(w, err)
			return
		}
	}
	if r.Header.Get("HX-Request") == "true" {
		render(w, r, KnownVocabResult(language, result, known, message))
		return
	}
	u := user(r)
	studyLanguages, err := h.services.Store.ListStudyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	knownLanguages, err := h.services.Store.ListKnownVocabularyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if !learnerLanguagePresent(studyLanguages, knownLanguages, language) {
		language = ""
		known = nil
	}
	render(w, r, VocabularyPageWithResult(u, h.csrf(w, r), studyLanguages, knownLanguages, language, result, known, message))
}

func knownVocabImportLanguage(r *http.Request) string {
	language, _ := activeStudyLanguageForContext(r.Context())
	return language
}

func knownVocabImportAction() string {
	return "/vocabulary/import"
}

func knownVocabImportRecoveryTarget() string {
	return "/vocabulary"
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

func knownVocabUPOS(upos string) string {
	if upos == "" {
		return "Any"
	}
	return upos
}
