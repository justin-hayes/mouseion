package webapp

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func (h *Handler) vocabularyConcordancePage(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	queryCtx, cancel := context.WithTimeout(r.Context(), h.interactiveReadTimeout())
	defer cancel()
	lookup := domain.ConcordanceLookup{
		Mode: r.URL.Query().Get("mode"), Term: strings.TrimSpace(r.URL.Query().Get("term")),
		UPOS: strings.TrimSpace(r.URL.Query().Get("upos")), BookIDs: r.URL.Query()["book"],
		GrammarDirection: strings.TrimSpace(r.URL.Query().Get("grammar")),
		Relation:         strings.TrimSpace(r.URL.Query().Get("relation")), Page: 1,
		Revision: strings.TrimSpace(r.URL.Query().Get("rev")),
	}
	focusTarget := strings.TrimSpace(r.URL.Query().Get("focus"))
	if lookup.Mode == "" {
		lookup.Mode = "surface"
	}
	if page, parseErr := strconv.Atoi(r.URL.Query().Get("page")); parseErr == nil && page > 0 {
		lookup.Page = page
	}
	requestedLanguage, _ := activeStudyLanguageForContext(r.Context())
	loadError := func(language string, books []domain.SourceMaterialSummary, err error) {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			h.renderConcordanceFailure(w, r, http.StatusGatewayTimeout, u, language, books, lookup, false)
			return
		}
		log.Printf("mouseion: load Concordance context: %v", err)
		h.renderConcordanceFailure(w, r, http.StatusInternalServerError, u, language, books, lookup, false)
	}
	languages, err := h.services.Store.StudyLanguages.ListStudyLanguages(queryCtx, u.ID)
	if err != nil {
		loadError(requestedLanguage, nil, err)
		return
	}
	knownLanguages, err := h.services.Store.StudyLanguages.ListKnownVocabularyLanguages(queryCtx, u.ID)
	if err != nil {
		loadError(requestedLanguage, nil, err)
		return
	}
	language := requestedLanguage
	if !learnerLanguagePresent(languages, knownLanguages, language) {
		language = ""
	}
	books, err := h.services.Store.Books.ListSourceMaterials(queryCtx, u.ID)
	if err != nil {
		loadError(language, nil, err)
		return
	}
	bookOptions := make([]domain.SourceMaterialSummary, 0, len(books))
	for _, book := range books {
		if book.Source.Language == language && book.BookID != "" {
			bookOptions = append(bookOptions, book)
		}
	}
	if lookup.Mode != "surface" && lookup.Mode != "effective" && lookup.Mode != "analyzer" {
		renderStatus(w, r, http.StatusBadRequest, VocabularyConcordancePageView(u, h.csrf(w, r), language, bookOptions, lookup, domain.ConcordanceResult{}, false, "Choose observed surface, effective lemma, or analyzer lemma evidence.", ""))
		return
	}
	if lookup.GrammarDirection != "" && lookup.GrammarDirection != "own" && lookup.GrammarDirection != "governor" {
		renderStatus(w, r, http.StatusBadRequest, VocabularyConcordancePageView(u, h.csrf(w, r), language, bookOptions, lookup, domain.ConcordanceResult{}, false, "Choose queried occurrence relation or governor dependents.", ""))
		return
	}
	if lookup.GrammarDirection != "" && lookup.Relation == "" {
		renderStatus(w, r, http.StatusBadRequest, VocabularyConcordancePageView(u, h.csrf(w, r), language, bookOptions, lookup, domain.ConcordanceResult{}, false, "Choose a dependency relation before applying grammar.", ""))
		return
	}
	if language == "" || lookup.Term == "" {
		render(w, r, VocabularyConcordancePageView(u, h.csrf(w, r), language, bookOptions, lookup, domain.ConcordanceResult{}, false, "", ""))
		return
	}
	if lookup.Mode != "surface" && lookup.UPOS == "" {
		renderStatus(w, r, http.StatusBadRequest, VocabularyConcordancePageView(u, h.csrf(w, r), language, bookOptions, lookup, domain.ConcordanceResult{}, false, "Part of speech is required for lemma lookup.", ""))
		return
	}
	result, err := h.services.Store.VocabularyConcordance.ListVocabularyConcordance(queryCtx, u.ID, language, lookup)
	if err != nil {
		log.Printf("mouseion: load Vocabulary Concordance: %v", err)
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		h.renderConcordanceFailure(w, r, status, u, language, bookOptions, lookup, false)
		return
	}
	if result.Stale {
		h.renderConcordanceFailure(w, r, http.StatusConflict, u, language, bookOptions, lookup, true)
		return
	}
	lookup.Revision = result.Revision
	render(w, r, VocabularyConcordancePageView(u, h.csrf(w, r), language, bookOptions, lookup, result, true, "", focusTarget))
}

func (h *Handler) renderConcordanceFailure(w http.ResponseWriter, r *http.Request, status int, u domain.User, language string, books []domain.SourceMaterialSummary, lookup domain.ConcordanceLookup, changed bool) {
	csrf := h.csrf(w, r)
	if isPartialHTMXRequest(r) {
		if changed {
			renderStatus(w, r, status, VocabularyConcordanceChangedRecovery(books, lookup))
			return
		}
		renderStatus(w, r, status, VocabularyConcordanceErrorRecovery(books, lookup, status == http.StatusGatewayTimeout))
		return
	}
	if changed {
		renderStatus(w, r, status, VocabularyConcordanceChangedPageView(u, csrf, language, books, lookup))
		return
	}
	renderStatus(w, r, status, VocabularyConcordanceErrorPageView(u, csrf, language, books, lookup, status == http.StatusGatewayTimeout))
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
	back := safeConcordanceReturnURL(query.Get("return"))
	if direction := query.Get("grammar"); direction == "own" || direction == "governor" {
		study.GrammarDirection = direction
		study.Relation = query.Get("relation")
	}
	render(w, r, VocabularySentenceStudyPageView(u, h.csrf(w, r), study, back))
}

func safeConcordanceReturnURL(candidate string) string {
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil || parsed.Path != "/vocabulary/concordance" || parsed.RawPath != "" {
		return "/vocabulary/concordance"
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return "/vocabulary/concordance"
	}
	focus := values.Get("focus")
	if focus == "" || parsed.Fragment != focus || values.Get("term") == "" {
		return "/vocabulary/concordance"
	}
	for key := range values {
		switch key {
		case "mode", "term", "upos", "book", "grammar", "relation", "rev", "page", "focus":
		default:
			return "/vocabulary/concordance"
		}
	}
	page, err := strconv.Atoi(values.Get("page"))
	if err != nil || page < 1 {
		return "/vocabulary/concordance"
	}
	return parsed.String()
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
	if lookup.Revision != "" {
		values.Set("rev", lookup.Revision)
	}
	values.Set("page", strconv.Itoa(page))
	return "/vocabulary/concordance?" + values.Encode()
}

func vocabularyConcordanceRestartURL(lookup domain.ConcordanceLookup) string {
	lookup.Page = 1
	lookup.Revision = ""
	return vocabularyConcordancePageURL(1, lookup)
}
