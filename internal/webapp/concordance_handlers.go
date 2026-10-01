package webapp

import (
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func (h *Handler) vocabularyConcordancePage(w http.ResponseWriter, r *http.Request) {
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
	lookup := domain.ConcordanceLookup{
		Mode: r.URL.Query().Get("mode"), Term: strings.TrimSpace(r.URL.Query().Get("term")),
		UPOS: strings.TrimSpace(r.URL.Query().Get("upos")), Page: 1,
	}
	if lookup.Mode == "" {
		lookup.Mode = "surface"
	}
	if page, parseErr := strconv.Atoi(r.URL.Query().Get("page")); parseErr == nil && page > 0 {
		lookup.Page = page
	}
	if lookup.Mode != "surface" && lookup.Mode != "effective" && lookup.Mode != "analyzer" {
		renderStatus(w, r, http.StatusBadRequest, VocabularyConcordancePageView(u, h.csrf(w, r), language, lookup, domain.ConcordanceResult{}, false, "Choose observed surface, effective lemma, or analyzer lemma evidence."))
		return
	}
	if language == "" || lookup.Term == "" {
		render(w, r, VocabularyConcordancePageView(u, h.csrf(w, r), language, lookup, domain.ConcordanceResult{}, false, ""))
		return
	}
	if lookup.Mode != "surface" && lookup.UPOS == "" {
		renderStatus(w, r, http.StatusBadRequest, VocabularyConcordancePageView(u, h.csrf(w, r), language, lookup, domain.ConcordanceResult{}, false, "Part of speech is required for lemma lookup."))
		return
	}
	result, err := h.services.Store.VocabularyConcordance.ListVocabularyConcordance(r.Context(), u.ID, language, lookup)
	if err != nil {
		log.Printf("mouseion: load Vocabulary Concordance: %v", err)
		renderStatus(w, r, http.StatusInternalServerError, VocabularyConcordancePageView(u, h.csrf(w, r), language, lookup, domain.ConcordanceResult{}, false, "The lookup was not applied. Retry with the same search or narrow the term."))
		return
	}
	render(w, r, VocabularyConcordancePageView(u, h.csrf(w, r), language, lookup, result, true, ""))
}

func vocabularyConcordancePageURL(page int, lookup domain.ConcordanceLookup) string {
	values := url.Values{}
	values.Set("mode", lookup.Mode)
	values.Set("term", lookup.Term)
	if lookup.UPOS != "" {
		values.Set("upos", lookup.UPOS)
	}
	values.Set("page", strconv.Itoa(page))
	return "/vocabulary/concordance?" + values.Encode()
}
