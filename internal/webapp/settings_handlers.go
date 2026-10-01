package webapp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/knownvocab"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/riverqueue/river/rivertype"
)

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
	bookIDs := values["book"]
	upos := values["pos"]
	known := vocabularyBrowseRequestState(values.Get("known"), "known", "not-known")
	reserved := vocabularyBrowseRequestState(values.Get("reserved"), "reserved", "not-reserved")
	sortBy := values.Get("sort")
	if sortBy != "occurrences" && sortBy != "books" {
		sortBy = "lemma"
	}
	browseQuery := domain.VocabularyBrowseQuery{
		Prefix: prefix, BookIDs: bookIDs, UPOS: upos, KnownFilter: known, ReservedFilter: reserved, Sort: sortBy, Page: page,
	}
	browse, err := h.services.Store.VocabularyBrowse.ListVocabularyBrowsePage(r.Context(), u.ID, language, browseQuery)
	if err != nil {
		log.Printf("mouseion: load vocabulary Browse: %v", err)
		renderStatus(w, r, http.StatusInternalServerError, VocabularyBrowseErrorPageView(u, h.csrf(w, r), language, browseQuery))
		return
	}
	if h.services.Store.VocabularySelection != nil {
		selection, selectionErr := h.services.Store.VocabularySelection.ListVocabularyBrowseSelection(r.Context(), u.ID, language)
		if selectionErr != nil {
			fail(w, selectionErr)
			return
		}
		selected := make(map[string]bool, len(selection))
		for _, identity := range selection {
			selected[identity.CanonicalLemma+"\x00"+identity.UPOS] = true
		}
		browse.SelectionCount = len(selection)
		for i := range browse.Rows {
			browse.Rows[i].Selected = selected[browse.Rows[i].CanonicalLemma+"\x00"+browse.Rows[i].UPOS]
		}
	}
	render(w, r, VocabularyBrowsePageView(u, h.csrf(w, r), language, browse, prefix))
}

func (h *Handler) setVocabularySelection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	u := user(r)
	language, _ := activeStudyLanguageForContext(r.Context())
	if !h.requireEditableStudyLanguage(w, r, language) {
		return
	}
	selected := r.PathValue("action") == "add"
	err := h.services.Store.VocabularySelection.SetVocabularyBrowseSelection(r.Context(), u.ID, language, r.FormValue("lemma"), r.FormValue("upos"), selected)
	if err != nil {
		if errors.Is(err, persistence.ErrVocabularyIdentityNotCurrent) {
			http.Error(w, "This identity no longer has current evidence. Refresh Browse before selecting it.", http.StatusConflict)
			return
		}
		fail(w, err)
		return
	}
	redirect(w, r, "/vocabulary")
}

func (h *Handler) vocabularySelectionPage(w http.ResponseWriter, r *http.Request) {
	language, _ := activeStudyLanguageForContext(r.Context())
	languages, err := h.services.Store.StudyLanguages.ListStudyLanguages(r.Context(), user(r).ID)
	if err != nil {
		fail(w, err)
		return
	}
	editable := studyLanguagePresent(languages, language)
	selection, err := h.services.Store.VocabularySelection.ListVocabularyBrowseSelection(r.Context(), user(r).ID, language)
	if err != nil {
		fail(w, err)
		return
	}
	decks, err := h.services.Store.VocabularySelection.ListCustomVocabularyDecks(r.Context(), user(r).ID)
	if err != nil {
		fail(w, err)
		return
	}
	page := 1
	if parsed, parseErr := strconv.Atoi(r.URL.Query().Get("page")); parseErr == nil && parsed > 1 {
		page = parsed
	}
	missingOnly := r.URL.Query().Get("missing") == "true"
	filtered := selection[:0]
	var missing int
	for _, identity := range selection {
		if identity.MissingEvidence {
			missing++
		}
		if !missingOnly || identity.MissingEvidence {
			filtered = append(filtered, identity)
		}
	}
	visibleTotal := len(filtered)
	lastPage := visibleTotal / 25
	if visibleTotal%25 != 0 {
		lastPage++
	}
	lastPage = max(1, lastPage)
	page = min(page, lastPage)
	start := (page - 1) * 25
	end := min(start+25, visibleTotal)
	render(w, r, VocabularySelectionPageView(user(r), h.csrf(w, r), language, filtered[start:end], len(selection), missing, visibleTotal, page, missingOnly, uuid.NewString(), "", "", decks, editable))
}

func (h *Handler) removeVocabularySelection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if !h.requireEditableStudyLanguage(w, r, language) {
		return
	}
	if err := h.services.Store.VocabularySelection.SetVocabularyBrowseSelection(r.Context(), user(r).ID, language, r.FormValue("lemma"), r.FormValue("upos"), false); err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, "/vocabulary/selection")
}

func (h *Handler) clearVocabularySelection(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if !h.requireEditableStudyLanguage(w, r, language) {
		return
	}
	if err := h.services.Store.VocabularySelection.ClearVocabularyBrowseSelection(r.Context(), user(r).ID, language); err != nil {
		fail(w, err)
		return
	}
	redirect(w, r, "/vocabulary/selection")
}

func (h *Handler) confirmClearVocabularySelection(w http.ResponseWriter, r *http.Request) {
	language, _ := activeStudyLanguageForContext(r.Context())
	selection, err := h.services.Store.VocabularySelection.ListVocabularyBrowseSelection(r.Context(), user(r).ID, language)
	if err != nil {
		fail(w, err)
		return
	}
	languages, err := h.services.Store.StudyLanguages.ListStudyLanguages(r.Context(), user(r).ID)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, VocabularySelectionClearConfirmView(user(r), h.csrf(w, r), language, len(selection), studyLanguagePresent(languages, language)))
}

func (h *Handler) createCustomVocabularyDeck(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if !h.requireEditableStudyLanguage(w, r, language) {
		return
	}
	deck, err := h.services.Store.VocabularySelection.CreateCustomVocabularyDeck(r.Context(), user(r).ID, language, r.FormValue("name"), r.FormValue("creation_key"))
	if err != nil {
		selection, listErr := h.services.Store.VocabularySelection.ListVocabularyBrowseSelection(r.Context(), user(r).ID, language)
		if listErr != nil {
			fail(w, err)
			return
		}
		var missing int
		for _, identity := range selection {
			if identity.MissingEvidence {
				missing++
			}
		}
		log.Printf("mouseion: create Custom deck: %v", err)
		decks, decksErr := h.services.Store.VocabularySelection.ListCustomVocabularyDecks(r.Context(), user(r).ID)
		if decksErr != nil {
			fail(w, decksErr)
			return
		}
		renderStatus(w, r, http.StatusInternalServerError, VocabularySelectionPageView(user(r), h.csrf(w, r), language, selection, len(selection), missing, len(selection), 1, false, r.FormValue("creation_key"), r.FormValue("name"), "Deck creation did not complete. Your selection is unchanged; retry this same action.", decks, true))
		return
	}
	redirect(w, r, "/vocabulary/decks/"+deck.ID)
}

func (h *Handler) customVocabularyDeckPage(w http.ResponseWriter, r *http.Request) {
	deck, err := h.services.Store.VocabularySelection.GetCustomVocabularyDeck(r.Context(), user(r).ID, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, persistence.ErrCustomVocabularyDeckNotFound) {
			http.NotFound(w, r)
		} else {
			fail(w, err)
		}
		return
	}
	languages, err := h.services.Store.StudyLanguages.ListStudyLanguages(r.Context(), user(r).ID)
	if err != nil {
		fail(w, err)
		return
	}
	editable := studyLanguagePresent(languages, deck.Language)
	page := 1
	if parsed, parseErr := strconv.Atoi(r.URL.Query().Get("page")); parseErr == nil && parsed > 1 {
		page = parsed
	}
	missingOnly := r.URL.Query().Get("missing") == "true"
	filtered := make([]domain.VocabularyIdentity, 0, len(deck.Identities))
	for _, identity := range deck.Identities {
		if !missingOnly || identity.MissingEvidence {
			filtered = append(filtered, identity)
		}
	}
	lastPage := max(1, (len(filtered)+24)/25)
	page = min(page, lastPage)
	start, end := (page-1)*25, min(page*25, len(filtered))
	decks, err := h.services.Store.VocabularySelection.ListCustomVocabularyDecks(r.Context(), user(r).ID)
	if err != nil {
		fail(w, err)
		return
	}
	var preparation *domain.CustomDeckPreparation
	if h.services.CustomDeckPreparation != nil {
		latest, latestErr := h.services.CustomDeckPreparation.Latest(r.Context(), user(r).ID, deck.ID)
		if latestErr == nil {
			preparation = &latest
		} else if !errors.Is(latestErr, persistence.ErrNotFound) {
			fail(w, latestErr)
			return
		}
	}
	render(w, r, CustomVocabularyDeckPageView(user(r), h.csrf(w, r), deck, decks, editable, page, lastPage, filtered[start:end], missingOnly, preparation, uuid.NewString()))
}

func (h *Handler) prepareCustomVocabularyDeck(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	if h.services.CustomDeckPreparation == nil {
		http.NotFound(w, r)
		return
	}
	owner, deckID := user(r).ID, r.PathValue("id")
	deck, err := h.services.Store.VocabularySelection.GetCustomVocabularyDeck(r.Context(), owner, deckID)
	if errors.Is(err, persistence.ErrCustomVocabularyDeckNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if !h.requireEditableStudyLanguage(w, r, deck.Language) {
		return
	}
	preparation, err := h.services.CustomDeckPreparation.Submit(r.Context(), owner, deckID, r.FormValue("action_key"))
	if errors.Is(err, persistence.ErrCustomDeckPreparationUnavailable) {
		http.Error(w, "No selected identity currently has eligible evidence for preparation.", http.StatusConflict)
		return
	}
	if errors.Is(err, persistence.ErrCustomVocabularyDeckNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/vocabulary/deck-preparations/"+url.PathEscape(preparation.ID), http.StatusSeeOther)
}

func (h *Handler) customDeckPreparationStatus(w http.ResponseWriter, r *http.Request) {
	if h.services.CustomDeckPreparation == nil {
		http.NotFound(w, r)
		return
	}
	p, err := h.services.CustomDeckPreparation.Get(r.Context(), user(r).ID, r.PathValue("id"))
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	deck, err := h.services.Store.VocabularySelection.GetCustomVocabularyDeck(r.Context(), user(r).ID, p.DeckID)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, CustomDeckPreparationStatusPage(user(r), h.csrf(w, r), p, deck))
}

func (h *Handler) downloadCustomDeckPreparation(w http.ResponseWriter, r *http.Request) {
	if h.services.CustomDeckPreparation == nil {
		http.NotFound(w, r)
		return
	}
	p, err := h.services.CustomDeckPreparation.Download(r.Context(), user(r).ID, r.PathValue("id"))
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.anki")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": p.Filename}))
	w.Header().Set("X-Mouseion-Deck-Name", p.DeckName)
	w.Header().Set("X-Mouseion-Cards-Total", strconv.Itoa(p.TotalCards))
	if _, err := w.Write(p.Artifact); err != nil {
		log.Printf("write Custom deck APKG: %v", err)
	}
}

func (h *Handler) renameCustomVocabularyDeck(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	deck, err := h.services.Store.VocabularySelection.GetCustomVocabularyDeck(r.Context(), user(r).ID, r.PathValue("id"))
	if errors.Is(err, persistence.ErrCustomVocabularyDeckNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if !h.requireEditableStudyLanguage(w, r, deck.Language) {
		return
	}
	if err := h.services.Store.VocabularySelection.RenameCustomVocabularyDeck(r.Context(), user(r).ID, deck.ID, r.FormValue("name")); err != nil {
		if errors.Is(err, persistence.ErrCustomVocabularyDeckNameInvalid) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fail(w, err)
		return
	}
	redirect(w, r, "/vocabulary/decks/"+deck.ID)
}

func (h *Handler) setCustomVocabularyDeckIdentity(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner, id := user(r).ID, r.PathValue("id")
	deck, err := h.services.Store.VocabularySelection.GetCustomVocabularyDeck(r.Context(), owner, id)
	if errors.Is(err, persistence.ErrCustomVocabularyDeckNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if !h.requireEditableStudyLanguage(w, r, deck.Language) {
		return
	}
	action := r.PathValue("action")
	if action != "add" && action != "remove" {
		http.NotFound(w, r)
		return
	}
	if err := h.services.Store.VocabularySelection.SetCustomVocabularyDeckIdentity(r.Context(), owner, id, r.FormValue("lemma"), r.FormValue("upos"), action == "add"); err != nil {
		if errors.Is(err, persistence.ErrCustomVocabularyDeckNotFound) {
			http.NotFound(w, r)
			return
		}
		fail(w, err)
		return
	}
	redirect(w, r, "/vocabulary/decks/"+id)
}

func (h *Handler) confirmDeleteCustomVocabularyDeck(w http.ResponseWriter, r *http.Request) {
	deck, err := h.services.Store.VocabularySelection.GetCustomVocabularyDeck(r.Context(), user(r).ID, r.PathValue("id"))
	if errors.Is(err, persistence.ErrCustomVocabularyDeckNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	languages, err := h.services.Store.StudyLanguages.ListStudyLanguages(r.Context(), user(r).ID)
	if err != nil {
		fail(w, err)
		return
	}
	render(w, r, CustomVocabularyDeckDeleteConfirmView(user(r), h.csrf(w, r), deck, studyLanguagePresent(languages, deck.Language)))
}

func (h *Handler) deleteCustomVocabularyDeck(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner, id := user(r).ID, r.PathValue("id")
	deck, err := h.services.Store.VocabularySelection.GetCustomVocabularyDeck(r.Context(), owner, id)
	if errors.Is(err, persistence.ErrCustomVocabularyDeckNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	if !h.requireEditableStudyLanguage(w, r, deck.Language) {
		return
	}
	if err := h.services.Store.VocabularySelection.DeleteCustomVocabularyDeck(r.Context(), owner, id); err != nil {
		if errors.Is(err, persistence.ErrCustomVocabularyDeckNotFound) {
			http.NotFound(w, r)
			return
		}
		fail(w, err)
		return
	}
	redirect(w, r, "/vocabulary/selection")
}

func (h *Handler) requireEditableStudyLanguage(w http.ResponseWriter, r *http.Request, language string) bool {
	languages, err := h.services.Store.StudyLanguages.ListStudyLanguages(r.Context(), user(r).ID)
	if err != nil {
		fail(w, err)
		return false
	}
	if language == "" || !studyLanguagePresent(languages, language) {
		http.Error(w, "This language is not currently available for editing.", http.StatusConflict)
		return false
	}
	return true
}

func vocabularyBrowseRequestState(value, positive, negative string) string {
	if value == positive || value == negative || value == "not-known-or-reserved" && positive == "known" {
		return value
	}
	return "any"
}

func vocabularyBrowsePageURL(page int, browse domain.VocabularyBrowsePage, query string) string {
	values := url.Values{}
	if query != "" {
		values.Set("q", query)
	}
	for _, book := range browse.SelectedBooks {
		values.Add("book", book)
	}
	for _, pos := range browse.SelectedUPOS {
		values.Add("pos", pos)
	}
	if browse.KnownFilter != "any" && browse.KnownFilter != "" {
		values.Set("known", browse.KnownFilter)
	}
	if browse.ReservedFilter != "any" && browse.ReservedFilter != "" {
		values.Set("reserved", browse.ReservedFilter)
	}
	if browse.Sort != "" && browse.Sort != "lemma" {
		values.Set("sort", browse.Sort)
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
	if r.Header.Get("Hx-Request") == "true" {
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

func (h *Handler) renderKnownVocabResult(w http.ResponseWriter, r *http.Request, language string, result *knownvocab.ImportResult, message string) {
	if r.Header.Get("Hx-Request") == "true" {
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

func vocabularySelectionAction(selected bool) string {
	if selected {
		return "/vocabulary/selection/remove"
	}
	return "/vocabulary/selection/add"
}

func vocabularySelectionButton(selected bool) string {
	if selected {
		return "Remove from selection"
	}
	return "Select"
}

func selectionEvidenceLabel(identity domain.VocabularyIdentity) string {
	if identity.MissingEvidence {
		return "No current evidence"
	}
	return fmt.Sprintf("%d occurrences · %d Books", identity.OccurrenceCount, identity.BookCount)
}

func vocabularySelectionPageURL(page int, missingOnly bool) string {
	values := url.Values{"page": []string{strconv.Itoa(page)}}
	if missingOnly {
		values.Set("missing", "true")
	}
	return "/vocabulary/selection?" + values.Encode()
}

func customVocabularyDeckPageURL(deckID string, page int, missingOnly bool) string {
	values := url.Values{"page": []string{strconv.Itoa(page)}}
	if missingOnly {
		values.Set("missing", "true")
	}
	return "/vocabulary/decks/" + url.PathEscape(deckID) + "?" + values.Encode()
}
