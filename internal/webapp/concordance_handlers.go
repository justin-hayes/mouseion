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

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/domain"
	"golang.org/x/text/unicode/norm"
)

// concordanceNoticeTermRequired and the other notices below are the learner
// copy for unsupported lookup state; none is a successful applied lookup.
const (
	concordanceNoticeOneTerm = "Enter one lemma or word form."
	concordanceNoticeState   = "This Concordance link is not valid. Look up a lemma or word form to start again."
	concordanceNoticePage    = "That results page is not valid. Start again from page 1."
)

func (h *Handler) corpusConcordancePage(w http.ResponseWriter, r *http.Request) {
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
		if lookup.Match == domain.ConcordanceMatchConstruction {
			lookup = concordanceSafeRestartLookup(lookup)
		}
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
	if lookup.Priority == "" && lookup.Match != domain.ConcordanceMatchConstruction {
		lookup.Priority = currentPriority
	}
	result, err := h.services.Store.Vocabulary.ListVocabularyConcordance(queryCtx, u.ID, language, lookup)
	if err != nil {
		log.Printf("mouseion: load Vocabulary Concordance: %v", err)
		h.renderConcordanceFailure(w, r, concordanceResultErrorStatus(err, queryCtx), u, language, lookup, origin, false)
		return
	}
	lookup = trustedOriginLemma(lookup, result)
	if result.Stale {
		h.renderConcordanceFailure(w, r, http.StatusConflict, u, language, lookup, origin, true)
		return
	}
	if result.InvalidPriority || result.InvalidOrigin {
		lookup = concordanceSafeRestartLookup(lookup)
		invite(http.StatusBadRequest, concordanceNoticeState)
		return
	}
	lookup = concordanceAppliedLookup(lookup, result)
	h.markUnsearchedBookUpdates(queryCtx, u.ID, &result)
	// A parallels page is anchored to its origin Book, not to the reading.
	readingChanged := lookup.Match != domain.ConcordanceMatchConstruction && lookup.Priority != currentPriority
	render(w, r, VocabularyConcordancePageView(u, h.csrf(w, r), language, lookup, result, true, readingChanged, "", request.focusTarget, origin))
}

// markUnsearchedBookUpdates flags each unsearched Book that has an Analysis
// update offer under the existing rule. A failed read leaves the Books named
// without the suffix: the line is a hint, never a reason to fail the page.
func (h *Handler) markUnsearchedBookUpdates(ctx context.Context, owner string, result *domain.ConcordanceResult) {
	if len(result.UnsearchedBooks) == 0 {
		return
	}
	books, err := h.services.Store.Reading.ListMyBooksWithEvidence(ctx, owner)
	if err != nil {
		log.Printf("mouseion: read Analysis update offers for Concordance: %v", err)
		return
	}
	h.annotateAnalysisUpdates(ctx, owner, books)
	offered := make(map[string]bool, len(books))
	for _, book := range books {
		if offer, _ := myBookAnalysisUpdate(book); offer != domain.AnalysisUpdateNone {
			offered[book.Book.ID] = true
		}
	}
	for i := range result.UnsearchedBooks {
		result.UnsearchedBooks[i].UpdateAvailable = result.UnsearchedBooks[i].UpdateAvailable || offered[result.UnsearchedBooks[i].BookID]
	}
}

// concordanceRequestRejection reports the client-visible rejection a Concordance
// request earns before any evidence is read. A blank language or term is a valid
// empty invitation, not a rejection.
func concordanceRequestRejection(request concordanceRequest, lookup domain.ConcordanceLookup, language string) (int, string, bool) {
	switch {
	case !request.pageValid:
		return http.StatusBadRequest, concordanceNoticePage, true
	case lookup.Match == domain.ConcordanceMatchConstruction:
		if constructionRequestInvalid(lookup, language) {
			return http.StatusBadRequest, concordanceNoticeState, true
		}
		return http.StatusOK, "", false
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

// concordanceSafeRestartLookup is the lookup shown beside a safe-restart notice.
// An invalid priority is dropped so a retry recaptures it; an invalid
// construction lookup has no safe partial form, so it restarts with an empty
// search.
func concordanceSafeRestartLookup(lookup domain.ConcordanceLookup) domain.ConcordanceLookup {
	if lookup.Match == domain.ConcordanceMatchConstruction {
		return domain.ConcordanceLookup{Language: lookup.Language}
	}
	lookup.Priority = ""
	return lookup
}

// constructionRequestInvalid reports a construction lookup that must get the
// safe restart: another language, an unknown construction or unlabelled subtype,
// a part of speech, or an origin that is not complete. The captured priority is
// the origin Book, so a missing one is never filled from the current reading.
func constructionRequestInvalid(lookup domain.ConcordanceLookup, language string) bool {
	if language != domain.ConcordanceConstructionLanguage || lookup.UPOS != "" || lookup.Origin == nil ||
		lookup.Priority == "" || lookup.Priority == domain.ConcordancePriorityNone {
		return true
	}
	return !constructionTermValid(lookup.Term, lookup.Subtype)
}

// constructionTermValid reports a known construction id with, when given, a
// subtype the shipping gate labels.
func constructionTermValid(term, subtype string) bool {
	if _, known := constructions.Notes[constructions.ID(term)]; !known {
		return false
	}
	if subtype == "" {
		return true
	}
	_, labelled := constructions.SubtypeLabel(constructions.ID(term), constructions.SubtypeID(subtype))
	return labelled
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
	if request.lookup.Match == domain.ConcordanceMatchConstruction {
		request.lookup.Subtype = strings.TrimSpace(query.Get("subtype"))
		request.lookup.Origin = parseConstructionOrigin(query)
		request.lookup.Lemma = constructionLemmaKey(query.Get("lemma"))
		request.lookup.OriginLemma = strings.TrimSpace(query.Get("from_lemma"))
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

// trustedOriginLemma sets the offered lemma of a parallels lookup to the origin
// anchor's, read from storage: the URL's from_lemma is only a hint and is never
// trusted.
func trustedOriginLemma(lookup domain.ConcordanceLookup, result domain.ConcordanceResult) domain.ConcordanceLookup {
	if lookup.Match == domain.ConcordanceMatchConstruction && !result.InvalidOrigin {
		lookup.OriginLemma = result.Origin.AnchorLemma
	}
	return lookup
}

// constructionLemmaKey normalises the lemma filter to its lemma key once, at
// parsing, so every generated URL carries the key. Parallels exist only for the
// one construction language; a value its profile cannot read stays as typed.
func constructionLemmaKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	profile, err := canonicalization.For(domain.ConcordanceConstructionLanguage)
	if err != nil {
		return raw
	}
	return profile.Canonical(norm.NFC.String(raw))
}

// constructionRunPattern is the canonical text form of an analysis run identity.
var constructionRunPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// parseConstructionOrigin reads the origin occurrence of a parallels lookup. All
// three values must be present and well formed, or there is no origin.
func parseConstructionOrigin(query url.Values) *domain.ConcordanceConstructionOrigin {
	run := strings.TrimSpace(query.Get("from_run"))
	sentence, sentenceErr := strconv.Atoi(strings.TrimSpace(query.Get("from_sentence")))
	anchor, anchorErr := strconv.Atoi(strings.TrimSpace(query.Get("from_anchor")))
	if !constructionRunPattern.MatchString(run) || sentenceErr != nil || anchorErr != nil || sentence < 0 || anchor < 0 {
		return nil
	}
	return &domain.ConcordanceConstructionOrigin{RunID: run, Sentence: sentence, Anchor: anchor}
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
	progressLanguages, err := h.services.Store.Shell.ListStudyProgressLanguages(ctx, u.ID)
	if err != nil {
		return requestedLanguage, origin, err
	}
	language := requestedLanguage
	if !learnerLanguagePresent(languages, progressLanguages, language) {
		language = ""
	}
	// The origin is only a pointer back to a Browse excursion. It must name an
	// owned Book in the study language, or it is dropped; it never exposes
	// another owner's Book and authorizes nothing.
	if origin.Active() && language != "" {
		detail, detailErr := h.services.Store.Vocabulary.GetBookDetail(ctx, u.ID, origin.BookID)
		switch {
		case errors.Is(detailErr, domain.ErrNotFound):
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

func (h *Handler) corpusSentenceStudyPage(w http.ResponseWriter, r *http.Request) {
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
	language, _ := activeStudyLanguageForContext(r.Context())
	// The URL names the language; a sentence analyzed in another one is not
	// this page's (ADR 0131).
	if study.Language != "" && canonicalization.NormalizeLanguage(study.Language) != language {
		http.NotFound(w, r)
		return
	}
	back := safeConcordanceReturnURL(query.Get("return"), language)
	// A sentence overlapping a recorded speaker label is raw Concordance
	// evidence, not study context. The same rule that withholds the Study
	// affordance refuses a direct request (ADR 0103).
	if study.SpeakerLabelOverlap {
		renderStatus(w, r, http.StatusForbidden, VocabularySentenceStudyUnavailablePageView(u, h.csrf(w, r), back))
		return
	}
	render(w, r, VocabularySentenceStudyPageView(u, h.csrf(w, r), study, back, query.Get("construction")))
}

// safeConcordanceReturnURL accepts only a Concordance address in the page's own
// study language; anything else returns to that language's Concordance landing.
// A return path under the retired Vocabulary address is rewritten to its
// successor, so an old Sentence study link keeps its saved search and focus.
func safeConcordanceReturnURL(candidate, language string) string {
	landing := studyLanguagePath(pageKindConcordance, language)
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil || parsed.RawPath != "" {
		return landing
	}
	if parsed.Path == "/"+language+"/vocabulary/concordance" {
		parsed.Path = landing
	}
	if parsed.Path != landing {
		return landing
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return landing
	}
	focus := values.Get("focus")
	if focus == "" || parsed.Fragment != focus || values.Get("term") == "" {
		return landing
	}
	if !safeConcordanceReturnKeys(values) {
		return landing
	}
	page, err := strconv.Atoi(values.Get("page"))
	if err != nil || page < 1 {
		return landing
	}
	if hasOriginKeys(values) && !parseBrowseOrigin(values).Active() {
		return landing
	}
	if values.Get("as") == domain.ConcordanceMatchConstruction && !safeConstructionReturn(values, language) {
		return landing
	}
	return parsed.String()
}

// safeConcordanceReturnKeys reports whether every key of a return URL is one a
// Concordance page carries. The construction keys belong to a construction
// lookup only.
func safeConcordanceReturnKeys(values url.Values) bool {
	for key := range values {
		switch key {
		case "term", "as", "upos", "priority", "rev", "page", "focus":
		case "subtype", "from_run", "from_sentence", "from_anchor", "lemma", "from_lemma":
			if values.Get("as") != domain.ConcordanceMatchConstruction {
				return false
			}
		case originKeyBook, originKeyPrefix, originKeyPage, originKeyRow, originKeyRevision:
		default:
			return false
		}
	}
	return true
}

// safeConstructionReturn validates a construction lookup the way the request
// does: a known construction, an optional shipped subtype, the origin Book as
// the priority and a complete origin occurrence.
func safeConstructionReturn(values url.Values, language string) bool {
	lookup := domain.ConcordanceLookup{
		UPOS: values.Get("upos"), Term: values.Get("term"), Subtype: values.Get("subtype"), Priority: values.Get("priority"),
		Origin: parseConstructionOrigin(values),
	}
	return !constructionRequestInvalid(lookup, language)
}

func vocabularyConcordancePageURL(page int, lookup domain.ConcordanceLookup) string {
	values := url.Values{}
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
	if lookup.Match == domain.ConcordanceMatchConstruction {
		if lookup.Subtype != "" {
			values.Set("subtype", lookup.Subtype)
		}
		if lookup.Lemma != "" {
			values.Set("lemma", lookup.Lemma)
		}
		if lookup.OriginLemma != "" {
			values.Set("from_lemma", lookup.OriginLemma)
		}
		if origin := lookup.Origin; origin != nil {
			values.Set("from_run", origin.RunID)
			values.Set("from_sentence", strconv.Itoa(origin.Sentence))
			values.Set("from_anchor", strconv.Itoa(origin.Anchor))
		}
	}
	if page > 0 {
		values.Set("page", strconv.Itoa(page))
	}
	return studyLanguagePath(pageKindConcordance, lookup.Language) + "?" + values.Encode()
}

// vocabularyConcordanceRestartURL starts the applied lookup over from page 1
// against current evidence and the current reading. An exact Browse identity
// (lemma with a part of speech) keeps its meaning; an independent lookup
// recognizes its term again.
func vocabularyConcordanceRestartURL(lookup domain.ConcordanceLookup) string {
	lookup.Revision = ""
	if lookup.Match == domain.ConcordanceMatchConstruction {
		// The priority is the origin Book, so a restart passes the origin rule.
		// It carries no page, so it opens on the first.
		return vocabularyConcordancePageURL(0, lookup)
	}
	lookup.Priority = ""
	if lookup.UPOS == "" {
		lookup.Match = ""
	}
	return vocabularyConcordancePageURL(1, lookup)
}
