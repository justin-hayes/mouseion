package webapp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/riverqueue/river/rivertype"
)

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	h.renderSettings(w, r, nil, nil, r.URL.Query().Get("message"))
}

func (h *Handler) addStudyLanguage(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	language := strings.TrimSpace(r.FormValue("language"))
	language = canonicalization.NormalizeLanguage(language)
	supported, degraded, capabilityErr := h.supportedNLP(r.Context())
	if capabilityErr != nil {
		fail(w, capabilityErr)
		return
	}
	if degraded {
		http.Error(w, "NLP language discovery is temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	var err error
	for _, candidate := range supported {
		if candidate.Language == language {
			if _, err = h.services.Store.PutLanguageProfile(r.Context(), user(r).ID, candidate.Language, candidate.DisplayName); err != nil {
				fail(w, err)
				return
			}
			redirect(w, r, "/settings?message=Study+language+added")
			return
		}
	}
	http.Error(w, "unsupported study language", http.StatusBadRequest)
}

func (h *Handler) removeStudyLanguage(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	if err := h.services.Store.DeleteLanguageProfile(r.Context(), user(r).ID, strings.TrimSpace(r.FormValue("language"))); err != nil {
		fail(w, err)
		return
	}
	language := strings.TrimSpace(r.FormValue("language"))
	redirect(w, r, "/settings?language="+url.QueryEscape(language)+"&message="+url.QueryEscape("Study language removed; only the preference was removed. Your vocabulary, books, analyses, prepared decks, and campaigns remain."))
}

func (h *Handler) renderSettings(w http.ResponseWriter, r *http.Request, result *knownvocab.ImportResult, known []domain.KnownVocabulary, message string) {
	u := user(r)
	supported, degraded, capabilityErr := h.supportedNLP(r.Context())
	if capabilityErr != nil {
		fail(w, capabilityErr)
		return
	}
	profiles, err := h.services.Store.ListLanguageProfiles(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	language, _ := knownVocabImportContext(r)
	language = canonicalization.NormalizeLanguage(language)
	studyLanguages, err := h.services.Store.ListStudyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	if known == nil && language != "" {
		known, err = h.services.Store.ListKnownVocabulary(r.Context(), u.ID, language)
		if err != nil {
			fail(w, err)
			return
		}
	}
	render(w, r, SettingsPage(u, h.csrf(w, r), supported, profiles, studyLanguages, degraded, language, result, known, message))
}

func (h *Handler) supportedNLP(ctx context.Context) ([]domain.SupportedLanguage, bool, error) {
	if h.services.Capabilities == nil {
		return nil, true, nil
	}
	capabilities, err := h.services.Capabilities.GetCapabilities(ctx)
	if err != nil {
		return nil, true, nil
	}
	languages := analyzer.ReadySupportedLanguages(capabilities)
	if h.services.Store != nil {
		if err := h.services.Store.SyncSupportedLanguages(ctx, languages); err != nil {
			return nil, false, fmt.Errorf("sync supported languages: %w", err)
		}
	}
	return languages, capabilities.Degraded, nil
}

func (h *Handler) knownVocabPage(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	requestedLanguage := strings.TrimSpace(r.URL.Query().Get("language"))
	requestedLanguage = canonicalization.NormalizeLanguage(requestedLanguage)
	studyLanguages, err := h.services.Store.ListStudyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, knownVocabSettingsTarget(requestedLanguage, studyLanguages))
}

func knownVocabSettingsTarget(requestedLanguage string, languages []domain.StudyLanguage) string {
	for _, language := range languages {
		if requestedLanguage == language.Language {
			return "/settings?language=" + url.QueryEscape(requestedLanguage) + "#known-vocabulary"
		}
	}
	return "/settings#known-vocabulary"
}

func (h *Handler) importKnownVocab(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		language, _ := knownVocabImportContext(r)
		h.renderKnownVocabResult(w, r, language, nil, nil, "The import is too large or could not be read.")
		return
	}
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	language, _ := knownVocabImportContext(r)
	language = canonicalization.NormalizeLanguage(language)
	var input bytes.Buffer
	file, header, err := r.FormFile("vocabulary_file")
	if err == nil {
		defer file.Close()
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
		h.renderKnownVocabResult(w, r, language, nil, nil, "Choose a language present in your library before importing.")
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
	redirect(w, r, fmt.Sprintf("/known-vocab/imports/%d/status", handle.ID))
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
	_, returnTo := knownVocabImportContext(r)
	if returnTo == "settings" {
		h.renderSettings(w, r, result, known, message)
		return
	}
	u := user(r)
	studyLanguages, err := h.services.Store.ListStudyLanguages(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, KnownVocabPageWithResult(u, h.csrf(w, r), studyLanguages, language, result, known, message))
}

func knownVocabImportContext(r *http.Request) (language, returnTo string) {
	language = strings.TrimSpace(r.URL.Query().Get("language"))
	returnTo = strings.TrimSpace(r.URL.Query().Get("return_to"))
	if r.Form == nil {
		return language, returnTo
	}
	if values, ok := r.Form["language"]; ok {
		language = ""
		if len(values) > 0 {
			language = strings.TrimSpace(values[0])
		}
	}
	if values, ok := r.Form["return_to"]; ok {
		returnTo = ""
		if len(values) > 0 {
			returnTo = strings.TrimSpace(values[0])
		}
	}
	return language, returnTo
}

func knownVocabImportAction(language, returnTo string) string {
	query := url.Values{}
	if language = strings.TrimSpace(language); language != "" {
		query.Set("language", language)
	}
	if returnTo = strings.TrimSpace(returnTo); returnTo != "" {
		query.Set("return_to", returnTo)
	}
	if len(query) == 0 {
		return "/known-vocab/import"
	}
	return "/known-vocab/import?" + query.Encode()
}

func knownVocabImportRecoveryTarget(language string) string {
	if language = strings.TrimSpace(language); language == "" {
		return "/settings#known-vocabulary"
	}
	return "/settings?language=" + url.QueryEscape(language) + "#known-vocabulary"
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
