package webapp

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// concordanceUPOS matches the Universal POS tags the analyzer emits.
var concordanceUPOS = regexp.MustCompile(`^[A-Z]{1,8}$`)

func (h *Handler) vocabularyConcordancePage(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	queryCtx, cancel := context.WithTimeout(r.Context(), h.interactiveReadTimeout())
	defer cancel()
	query := r.URL.Query()
	lookup := domain.ConcordanceLookup{
		Term: strings.TrimSpace(query.Get("term")), UPOS: strings.TrimSpace(query.Get("upos")),
		Kind: strings.TrimSpace(query.Get("kind")), Page: 1,
		Revision: strings.TrimSpace(query.Get("rev")),
	}
	focusTarget := strings.TrimSpace(query.Get("focus"))
	requestedLanguage, _ := activeStudyLanguageForContext(r.Context())
	lookup.Language = requestedLanguage
	pageProblem := ""
	if rawPage := query.Get("page"); rawPage != "" {
		if page, parseErr := strconv.Atoi(rawPage); parseErr == nil && page > 0 {
			lookup.Page = page
		} else {
			pageProblem = "That page is not valid. Restart the lookup from page 1."
		}
	}
	loadError := func(language string, err error) {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			h.renderConcordanceFailure(w, r, http.StatusGatewayTimeout, u, language, lookup, false)
			return
		}
		log.Printf("mouseion: load Concordance context: %v", err)
		h.renderConcordanceFailure(w, r, http.StatusInternalServerError, u, language, lookup, false)
	}
	languages, err := h.services.Store.StudyLanguages.ListStudyLanguages(queryCtx, u.ID)
	if err != nil {
		loadError(requestedLanguage, err)
		return
	}
	knownLanguages, err := h.services.Store.StudyLanguages.ListKnownVocabularyLanguages(queryCtx, u.ID)
	if err != nil {
		loadError(requestedLanguage, err)
		return
	}
	language := requestedLanguage
	if !learnerLanguagePresent(languages, knownLanguages, language) {
		language = ""
	}
	invalid := func(message string) {
		renderStatus(w, r, http.StatusBadRequest, VocabularyConcordancePageView(u, h.csrf(w, r), language, lookup, domain.ConcordanceResult{}, false, message, ""))
	}
	switch {
	case lookup.Kind != "" && lookup.Kind != domain.ConcordanceKindLemma && lookup.Kind != domain.ConcordanceKindForm:
		invalid("That lookup is not valid. Enter one lemma or word form.")
		return
	case lookup.UPOS != "" && (!concordanceUPOS.MatchString(lookup.UPOS) || lookup.Kind == domain.ConcordanceKindForm):
		invalid("That part of speech is not valid for this lookup. Enter one lemma or word form.")
		return
	case pageProblem != "":
		invalid(pageProblem)
		return
	case language == "" || lookup.Term == "":
		render(w, r, VocabularyConcordancePageView(u, h.csrf(w, r), language, lookup, domain.ConcordanceResult{}, false, "", ""))
		return
	case len(strings.Fields(lookup.Term)) > 1:
		invalid("Enter one lemma or word form.")
		return
	}
	result, err := h.services.Store.VocabularyConcordance.ListVocabularyConcordance(queryCtx, u.ID, language, lookup)
	if err != nil {
		log.Printf("mouseion: load Vocabulary Concordance: %v", err)
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		h.renderConcordanceFailure(w, r, status, u, language, lookup, false)
		return
	}
	if result.Stale {
		h.renderConcordanceFailure(w, r, http.StatusConflict, u, language, lookup, true)
		return
	}
	lookup.Revision = result.Revision
	lookup.Kind = result.Kind
	render(w, r, VocabularyConcordancePageView(u, h.csrf(w, r), language, lookup, result, true, "", focusTarget))
}

func (h *Handler) renderConcordanceFailure(w http.ResponseWriter, r *http.Request, status int, u domain.User, language string, lookup domain.ConcordanceLookup, changed bool) {
	csrf := h.csrf(w, r)
	if isPartialHTMXRequest(r) {
		if changed {
			renderStatus(w, r, status, VocabularyConcordanceChangedRecovery(lookup))
			return
		}
		renderStatus(w, r, status, VocabularyConcordanceErrorRecovery(lookup, status == http.StatusGatewayTimeout))
		return
	}
	if changed {
		renderStatus(w, r, status, VocabularyConcordanceChangedPageView(u, csrf, language, lookup))
		return
	}
	renderStatus(w, r, status, VocabularyConcordanceErrorPageView(u, csrf, language, lookup, status == http.StatusGatewayTimeout))
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
		case "language", "term", "upos", "kind", "rev", "page", "focus":
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
	if lookup.Language != "" {
		values.Set("language", lookup.Language)
	}
	values.Set("term", lookup.Term)
	if lookup.Kind != "" {
		values.Set("kind", lookup.Kind)
	}
	if lookup.UPOS != "" {
		values.Set("upos", lookup.UPOS)
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
