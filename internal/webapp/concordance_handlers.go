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
	"github.com/justin-hayes/mouseion/internal/persistence"
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
	request := parseConcordanceRequest(r)
	requestedLanguage, _ := activeStudyLanguageForContext(r.Context())
	lookup, origin := request.lookup, request.origin
	lookup.Language = requestedLanguage
	loadError := func(language string, err error) {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(queryCtx.Err(), context.DeadlineExceeded) {
			h.renderConcordanceFailure(w, r, http.StatusGatewayTimeout, u, language, lookup, origin, false)
			return
		}
		log.Printf("mouseion: load Concordance context: %v", err)
		h.renderConcordanceFailure(w, r, http.StatusInternalServerError, u, language, lookup, origin, false)
	}
	language, origin, err := h.loadConcordanceBrowseContext(queryCtx, u, requestedLanguage, origin)
	if err != nil {
		loadError(language, err)
		return
	}
	invite := func(status int, message string) {
		renderStatus(w, r, status, VocabularyConcordancePageView(u, h.csrf(w, r), language, lookup, domain.ConcordanceResult{}, false, false, message, "", origin))
	}
	if status, message, rejected := concordanceRequestRejection(request, lookup, language); rejected {
		invite(status, message)
		return
	}
	// A fresh lookup captures the then-current Book, or its explicit absence.
	// Supported applied state keeps what it captured, even after the learner
	// starts, ends, or switches reading.
	currentPriority, err := h.concordanceCurrentPriority(queryCtx, u.ID, language)
	if err != nil {
		loadError(language, err)
		return
	}
	if lookup.Priority == "" {
		lookup.Priority = currentPriority
	}
	result, err := h.services.Store.Vocabulary.ListVocabularyConcordance(queryCtx, u.ID, language, lookup)
	if err != nil {
		log.Printf("mouseion: load Vocabulary Concordance: %v", err)
		h.renderConcordanceFailure(w, r, concordanceResultErrorStatus(err, queryCtx), u, language, lookup, origin, false)
		return
	}
	if result.Stale {
		h.renderConcordanceFailure(w, r, http.StatusConflict, u, language, lookup, origin, true)
		return
	}
	if result.InvalidPriority {
		lookup.Priority = ""
		invite(http.StatusBadRequest, concordanceNoticeState)
		return
	}
	lookup = concordanceAppliedLookup(lookup, result)
	readingChanged := lookup.Priority != currentPriority
	render(w, r, VocabularyConcordancePageView(u, h.csrf(w, r), language, lookup, result, true, readingChanged, "", request.focusTarget, origin))
}

// concordanceRequestRejection reports the client-visible rejection a Concordance
// request earns before any evidence is read. A blank language or term is a valid
// empty invitation, not a rejection.
func concordanceRequestRejection(request concordanceRequest, lookup domain.ConcordanceLookup, language string) (int, string, bool) {
	switch {
	case !request.pageValid:
		return http.StatusBadRequest, concordanceNoticePage, true
	case lookup.Match != "" && lookup.Match != domain.ConcordanceMatchLemma && lookup.Match != domain.ConcordanceMatchForm,
		lookup.Match == "" && lookup.UPOS != "",
		lookup.Match == domain.ConcordanceMatchForm && lookup.UPOS != "",
		lookup.Match != "" && lookup.Term == "",
		lookup.Priority == "" && lookup.Revision != "":
		return http.StatusBadRequest, concordanceNoticeState, true
	case language == "" || lookup.Term == "":
		return http.StatusOK, "", true
	case len(strings.Fields(lookup.Term)) > 1:
		return http.StatusBadRequest, concordanceNoticeOneTerm, true
	}
	return http.StatusOK, "", false
}

// concordanceCurrentPriority names the Book a fresh lookup should prefer, or the
// explicit absence of a Current reading.
func (h *Handler) concordanceCurrentPriority(ctx context.Context, owner, language string) (string, error) {
	current, err := h.services.Store.Vocabulary.GetCurrentReading(ctx, owner, language)
	if err != nil {
		return "", err
	}
	if !current.IsActive() {
		return domain.ConcordancePriorityNone, nil
	}
	return current.BookID, nil
}

// concordanceResultErrorStatus maps a failed Concordance read to the status its
// failure page carries.
func concordanceResultErrorStatus(err error, ctx context.Context) int {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	return http.StatusInternalServerError
}

// concordanceRequest is a Concordance page request parsed from the query before
// it is resolved against the learner's evidence.
type concordanceRequest struct {
	lookup      domain.ConcordanceLookup
	focusTarget string
	origin      browseOrigin
	pageValid   bool
}

// parseConcordanceRequest reads the query into a lookup, keeping an explicitly
// requested page only when it is a positive integer.
func parseConcordanceRequest(r *http.Request) concordanceRequest {
	query := r.URL.Query()
	request := concordanceRequest{
		lookup: domain.ConcordanceLookup{
			Term: strings.TrimSpace(query.Get("term")), Match: strings.TrimSpace(query.Get("as")),
			UPOS: strings.TrimSpace(query.Get("upos")), Page: 1,
			Revision: strings.TrimSpace(query.Get("rev")),
			Priority: strings.TrimSpace(query.Get("priority")),
		},
		focusTarget: strings.TrimSpace(query.Get("focus")),
		origin:      parseBrowseOrigin(query),
		pageValid:   true,
	}
	if raw := query.Get("page"); raw != "" {
		if page, err := strconv.Atoi(raw); err == nil && page > 0 {
			request.lookup.Page = page
		} else {
			request.pageValid = false
		}
	}
	return request
}

// loadConcordanceBrowseContext resolves the study language against the learner's
// languages and validates the origin pointer to an owned Book in that language.
// It returns the language to render with even when it fails, so a failure page
// keeps the requested scope.
func (h *Handler) loadConcordanceBrowseContext(ctx context.Context, u domain.User, requestedLanguage string, origin browseOrigin) (string, browseOrigin, error) {
	languages, err := h.services.Store.Shell.ListStudyLanguages(ctx, u.ID)
	if err != nil {
		return requestedLanguage, origin, err
	}
	knownLanguages, err := h.services.Store.Shell.ListKnownVocabularyLanguages(ctx, u.ID)
	if err != nil {
		return requestedLanguage, origin, err
	}
	language := requestedLanguage
	if !learnerLanguagePresent(languages, knownLanguages, language) {
		language = ""
	}
	// The origin is only a pointer back to a Browse excursion. It must name an
	// owned Book in the study language, or it is dropped; it never exposes
	// another owner's Book and authorizes nothing.
	if origin.Active() && language != "" {
		detail, detailErr := h.services.Store.Vocabulary.GetBookDetail(ctx, u.ID, origin.BookID)
		switch {
		case errors.Is(detailErr, persistence.ErrNotFound):
			origin = browseOrigin{}
		case detailErr != nil:
			return language, origin, detailErr
		case detail.Book.LanguageTag != language:
			origin = browseOrigin{}
		}
	}
	return language, origin, nil
}

func (h *Handler) renderConcordanceFailure(w http.ResponseWriter, r *http.Request, status int, u domain.User, language string, lookup domain.ConcordanceLookup, origin browseOrigin, changed bool) {
	csrf := h.csrf(w, r)
	if isPartialHTMXRequest(r) {
		if changed {
			renderStatus(w, r, status, VocabularyConcordanceChangedRecovery(lookup, origin))
			return
		}
		renderStatus(w, r, status, VocabularyConcordanceErrorRecovery(lookup, status == http.StatusGatewayTimeout, origin))
		return
	}
	if changed {
		renderStatus(w, r, status, VocabularyConcordanceChangedPageView(u, csrf, language, lookup, origin))
		return
	}
	renderStatus(w, r, status, VocabularyConcordanceErrorPageView(u, csrf, language, lookup, status == http.StatusGatewayTimeout, origin))
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
	study, err := h.services.Store.Vocabulary.GetVocabularySentenceStudy(r.Context(), u.ID,
		query.Get("book"), query.Get("run"), query.Get("corpus"), query.Get("unit"), sentence, target, query.Get("target_surface"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	back := safeConcordanceReturnURL(query.Get("return"))
	// A sentence overlapping a recorded speaker label is raw Concordance
	// evidence, not study context. The same rule that withholds the Study
	// affordance refuses a direct request (ADR 0103).
	if study.SpeakerLabelOverlap {
		renderStatus(w, r, http.StatusForbidden, VocabularySentenceStudyUnavailablePageView(u, h.csrf(w, r), back))
		return
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
		case "language", "term", "as", "upos", "priority", "rev", "page", "focus":
		case originKeyBook, originKeySnapshot, originKeyPrefix, originKeyAll, originKeyPage, originKeyRow, originKeyRevision:
		default:
			return "/vocabulary/concordance"
		}
	}
	page, err := strconv.Atoi(values.Get("page"))
	if err != nil || page < 1 {
		return "/vocabulary/concordance"
	}
	if hasOriginKeys(values) && !parseBrowseOrigin(values).Active() {
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
	if lookup.Priority != "" {
		values.Set("priority", lookup.Priority)
	}
	if lookup.Revision != "" {
		values.Set("rev", lookup.Revision)
	}
	values.Set("page", strconv.Itoa(page))
	return "/vocabulary/concordance?" + values.Encode()
}

// vocabularyConcordanceRestartURL starts the applied lookup over from page 1
// against current evidence and the current reading. An exact Browse identity
// (lemma with a part of speech) keeps its meaning; an independent lookup
// recognizes its term again.
func vocabularyConcordanceRestartURL(lookup domain.ConcordanceLookup) string {
	lookup.Revision, lookup.Priority = "", ""
	if lookup.UPOS == "" {
		lookup.Match = ""
	}
	return vocabularyConcordancePageURL(1, lookup)
}
