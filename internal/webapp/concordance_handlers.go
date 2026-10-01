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
		UPOS: strings.TrimSpace(r.URL.Query().Get("upos")), BookIDs: r.URL.Query()["book"],
		GrammarDirection: strings.TrimSpace(r.URL.Query().Get("grammar")),
		Relation:         strings.TrimSpace(r.URL.Query().Get("relation")), Page: 1,
	}
	books, err := h.services.Store.Books.ListSourceMaterials(r.Context(), u.ID)
	if err != nil {
		fail(w, err)
		return
	}
	bookOptions := make([]domain.SourceMaterialSummary, 0, len(books))
	for _, book := range books {
		if book.Source.Language == language && book.BookID != "" {
			bookOptions = append(bookOptions, book)
		}
	}
	if lookup.Mode == "" {
		lookup.Mode = "surface"
	}
	if page, parseErr := strconv.Atoi(r.URL.Query().Get("page")); parseErr == nil && page > 0 {
		lookup.Page = page
	}
	if lookup.Mode != "surface" && lookup.Mode != "effective" && lookup.Mode != "analyzer" {
		renderStatus(w, r, http.StatusBadRequest, VocabularyConcordancePageView(u, h.csrf(w, r), language, bookOptions, lookup, domain.ConcordanceResult{}, false, "Choose observed surface, effective lemma, or analyzer lemma evidence."))
		return
	}
	if lookup.GrammarDirection != "" && lookup.GrammarDirection != "own" && lookup.GrammarDirection != "governor" {
		renderStatus(w, r, http.StatusBadRequest, VocabularyConcordancePageView(u, h.csrf(w, r), language, bookOptions, lookup, domain.ConcordanceResult{}, false, "Choose queried occurrence relation or governor dependents."))
		return
	}
	if lookup.GrammarDirection != "" && lookup.Relation == "" {
		renderStatus(w, r, http.StatusBadRequest, VocabularyConcordancePageView(u, h.csrf(w, r), language, bookOptions, lookup, domain.ConcordanceResult{}, false, "Choose a dependency relation before applying grammar."))
		return
	}
	if language == "" || lookup.Term == "" {
		render(w, r, VocabularyConcordancePageView(u, h.csrf(w, r), language, bookOptions, lookup, domain.ConcordanceResult{}, false, ""))
		return
	}
	if lookup.Mode != "surface" && lookup.UPOS == "" {
		renderStatus(w, r, http.StatusBadRequest, VocabularyConcordancePageView(u, h.csrf(w, r), language, bookOptions, lookup, domain.ConcordanceResult{}, false, "Part of speech is required for lemma lookup."))
		return
	}
	result, err := h.services.Store.VocabularyConcordance.ListVocabularyConcordance(r.Context(), u.ID, language, lookup)
	if err != nil {
		log.Printf("mouseion: load Vocabulary Concordance: %v", err)
		renderStatus(w, r, http.StatusInternalServerError, VocabularyConcordancePageView(u, h.csrf(w, r), language, bookOptions, lookup, domain.ConcordanceResult{}, false, "The lookup was not applied. Retry with the same search or narrow the term."))
		return
	}
	render(w, r, VocabularyConcordancePageView(u, h.csrf(w, r), language, bookOptions, lookup, result, true, ""))
}

func (h *Handler) vocabularySentenceStudyPage(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	query := r.URL.Query()
	parseInt := func(key string) (int64, error) { return strconv.ParseInt(query.Get(key), 10, 64) }
	sentence, sentenceErr := parseInt("sentence")
	target, targetErr := parseInt("target")
	if sentenceErr != nil || targetErr != nil || sentence < 0 || target < 0 {
		http.NotFound(w, r)
		return
	}
	study, err := h.services.Store.VocabularyConcordance.GetVocabularySentenceStudy(r.Context(), u.ID,
		query.Get("book"), query.Get("run"), query.Get("corpus"), query.Get("unit"), sentence, target, query.Get("target_surface"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	back := query.Get("return")
	if !strings.HasPrefix(back, "/vocabulary/concordance?") {
		back = "/vocabulary/concordance"
	}
	if direction := query.Get("grammar"); direction == "own" || direction == "governor" {
		study.GrammarDirection = direction
		study.Relation = query.Get("relation")
	}
	render(w, r, VocabularySentenceStudyPageView(u, h.csrf(w, r), study, back))
}

func vocabularyConcordancePageURL(page int, lookup domain.ConcordanceLookup) string {
	values := url.Values{}
	values.Set("mode", lookup.Mode)
	values.Set("term", lookup.Term)
	if lookup.UPOS != "" {
		values.Set("upos", lookup.UPOS)
	}
	for _, bookID := range lookup.BookIDs {
		values.Add("book", bookID)
	}
	if lookup.GrammarDirection != "" {
		values.Set("grammar", lookup.GrammarDirection)
		values.Set("relation", lookup.Relation)
	}
	values.Set("page", strconv.Itoa(page))
	return "/vocabulary/concordance?" + values.Encode()
}
