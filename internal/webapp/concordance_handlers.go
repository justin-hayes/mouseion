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

// concordanceNoticeTermRequired and the other notices below are the learner
// copy for unsupported lookup state; none is a successful applied lookup.
const (
	concordanceNoticeOneTerm = "Enter one lemma or word form."
	concordanceNoticeState   = "This Concordance link is not valid. Look up a lemma or word form to start again."
	concordanceNoticePage    = "That results page is not valid. Start again from page 1."
)

func (h *Handler) vocabularyConcordancePage(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	queryCtx, cancel := context.WithTimeout(r.Context(), h.interactiveReadTimeout())
	defer cancel()
	query := r.URL.Query()
	lookup := domain.ConcordanceLookup{
		Term: strings.TrimSpace(query.Get("term")), Match: strings.TrimSpace(query.Get("as")),
		UPOS: strings.TrimSpace(query.Get("upos")), Page: 1,
		Revision: strings.TrimSpace(query.Get("rev")),
	}
	focusTarget := strings.TrimSpace(query.Get("focus"))
	pageValid := true
	if raw := query.Get("page"); raw != "" {
		if page, parseErr := strconv.Atoi(raw); parseErr == nil && page > 0 {
			lookup.Page = page
		} else {
			pageValid = false
		}
	}
	requestedLanguage, _ := activeStudyLanguageForContext(r.Context())
	lookup.Language = requestedLanguage
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
	invite := func(status int, message string) {
		renderStatus(w, r, status, VocabularyConcordancePageView(u, h.csrf(w, r), language, lookup, domain.ConcordanceResult{}, false, message, ""))
	}
	switch {
	case !pageValid:
		invite(http.StatusBadRequest, concordanceNoticePage)
		return
	case lookup.Match != "" && lookup.Match != domain.ConcordanceMatchLemma && lookup.Match != domain.ConcordanceMatchForm,
		lookup.Match == "" && lookup.UPOS != "",
		lookup.Match == domain.ConcordanceMatchForm && lookup.UPOS != "",
		lookup.Match != "" && lookup.Term == "":
		invite(http.StatusBadRequest, concordanceNoticeState)
		return
	case language == "" || lookup.Term == "":
		invite(http.StatusOK, "")
		return
	case len(strings.Fields(lookup.Term)) > 1:
		invite(http.StatusBadRequest, concordanceNoticeOneTerm)
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
	lookup = concordanceAppliedLookup(lookup, result)
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
		case "language", "term", "as", "upos", "rev", "page", "focus":
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
	if lookup.Match != "" {
		values.Set("as", lookup.Match)
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

// vocabularyConcordanceRestartURL starts the same applied lookup over from
// page 1 against current evidence.
func vocabularyConcordanceRestartURL(lookup domain.ConcordanceLookup) string {
	lookup.Revision = ""
	return vocabularyConcordancePageURL(1, lookup)
}
