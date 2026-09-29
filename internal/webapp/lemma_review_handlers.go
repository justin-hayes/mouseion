package webapp

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lexical"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

type lemmaDecisionProposal struct {
	Action      string
	Lemma       string
	Occurrences []domain.LemmaReviewOccurrence
	Indices     []int
	Fingerprint string
	Changes     []lemmaOccurrenceChange
	Impacts     []lemmaIdentityImpact
}

type lemmaOccurrenceChange struct{ Sentence, Before, After string }

type lemmaIdentityImpact struct {
	Lemma          string
	UPOS           string
	BeforeCount    int64
	AfterCount     int64
	BeforeKnown    bool
	AfterKnown     bool
	BeforeReserved bool
	AfterReserved  bool
	BeforeEligible bool
	AfterEligible  bool
}

func (h *Handler) lemmaReview(w http.ResponseWriter, r *http.Request) {
	owner := user(r)
	bookID := strings.TrimSpace(r.PathValue("bookID"))
	store := h.services.Store.LemmaReview
	if store == nil {
		http.Error(w, "Occurrence review is unavailable.", http.StatusServiceUnavailable)
		return
	}
	detail, err := h.services.Store.Books.GetBookDetail(r.Context(), owner.ID, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" || detail.Book.LanguageTag != language {
		http.Error(w, "Choose this Book's study language in Reading before reviewing occurrences.", http.StatusConflict)
		return
	}
	current, err := h.services.Store.CurrentReading.GetCurrentReading(r.Context(), owner.ID, language)
	if err != nil {
		fail(w, err)
		return
	}
	canCorrect := !(current.IsActive() && current.BookID == bookID)
	form := strings.TrimSpace(r.URL.Query().Get("form"))
	var occurrences []domain.LemmaReviewOccurrence
	if form != "" {
		occurrences, err = store.ListLemmaReviewOccurrences(r.Context(), owner.ID, bookID, form)
		if err != nil {
			fail(w, err)
			return
		}
	}
	pageError := ""
	if !canCorrect {
		pageError = "This Book is current reading. Stop reading before changing its vocabulary; the frozen reading snapshot remains unchanged."
	}
	var proposal *lemmaDecisionProposal
	render(w, r, LemmaReviewPage(owner, h.csrf(w, r), bookID, detail.Book.Title, form, pageError, canCorrect, occurrences, proposal))
}

func (h *Handler) correctLemma(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r) {
		return
	}
	owner := user(r)
	bookID := strings.TrimSpace(r.PathValue("bookID"))
	store := h.services.Store.LemmaReview
	if store == nil {
		http.Error(w, "Occurrence review is unavailable.", http.StatusServiceUnavailable)
		return
	}
	if r.FormValue("stage") == "preview" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid review form.", http.StatusBadRequest)
			return
		}
		form := strings.TrimSpace(r.FormValue("form"))
		if !h.lemmaReviewWritable(w, r, owner, bookID) {
			return
		}
		occurrences, err := store.ListLemmaReviewOccurrences(r.Context(), owner.ID, bookID, form)
		if err != nil {
			fail(w, err)
			return
		}
		proposal, err := h.lemmaProposal(r, occurrences)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		page, err := h.services.Store.Books.GetBookDetail(r.Context(), owner.ID, bookID)
		if errors.Is(err, persistence.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
		render(w, r, LemmaReviewPage(owner, h.csrf(w, r), bookID, page.Book.Title, form, "", true, occurrences, proposal))
		return
	}
	if r.FormValue("stage") == "confirm" {
		if !h.lemmaReviewWritable(w, r, owner, bookID) {
			return
		}
		h.confirmLemmaProposal(w, r, owner)
		return
	}
	http.Error(w, "Review the proposed consequence before confirming a decision.", http.StatusBadRequest)
}

func (h *Handler) lemmaProposal(r *http.Request, matches []domain.LemmaReviewOccurrence) (*lemmaDecisionProposal, error) {
	index, err := strconv.Atoi(r.FormValue("target"))
	if err != nil || index < 0 || index >= len(matches) {
		return nil, errors.New("The selected occurrence changed. Find it again before reviewing.")
	}
	selected := map[int]bool{index: true}
	for _, raw := range r.Form["also"] {
		i, parseErr := strconv.Atoi(raw)
		if parseErr != nil || i < 0 || i >= len(matches) {
			return nil, errors.New("One selected occurrence is no longer available. Find the exact form again.")
		}
		selected[i] = true
	}
	chosen := make([]domain.LemmaReviewOccurrence, 0, len(selected))
	for i := range matches {
		if selected[i] {
			chosen = append(chosen, matches[i])
		}
	}
	action := r.FormValue("decision")
	lemma := ""
	if action == "correct" {
		profile, profileErr := canonicalization.For(func() string { language, _ := activeStudyLanguageForContext(r.Context()); return language }())
		if profileErr != nil {
			return nil, errors.New("Choose a supported study language before correcting a lemma.")
		}
		lemma = profile.Canonical(strings.TrimSpace(r.FormValue("lemma")))
		if !lexical.IsLemma(lemma) {
			return nil, errors.New("Enter one valid canonical lemma without spaces.")
		}
	} else if action != "keep" && action != "exclude" {
		return nil, errors.New("Choose a decision to preview.")
	}
	indices := make([]int, 0, len(selected))
	for i := range matches {
		if selected[i] {
			indices = append(indices, i)
		}
	}
	fingerprint, err := h.lemmaDecisionStateFingerprint(r, user(r).ID, matches)
	if err != nil {
		return nil, err
	}
	p := &lemmaDecisionProposal{Action: action, Lemma: lemma, Occurrences: chosen, Indices: indices, Fingerprint: lemmaProposalFingerprint(fingerprint, action, lemma, indices)}
	insights, err := h.services.Store.LemmaReview.GetAnalysisCorpusVocabulary(r.Context(), user(r).ID, matches[index].CorpusID)
	if err != nil {
		return nil, err
	}
	// The language is taken from the analyzed Book, not inferred from a lemma.
	// The caller's active language is already checked by the review route.
	language, _ := activeStudyLanguageForContext(r.Context())
	known, err := h.services.Store.LemmaReview.ListKnownVocabulary(r.Context(), user(r).ID, language)
	if err != nil {
		return nil, err
	}
	knownIdentity := func(lemma, upos string) bool {
		for _, item := range known {
			if item.CanonicalLemma == lemma && (item.UPOS == "" || item.UPOS == upos) {
				return true
			}
		}
		return false
	}
	counts := make(map[string]int64)
	uposByIdentity := make(map[string]string)
	reservedByIdentity := make(map[string]bool)
	for _, item := range insights.Lemmas {
		key := item.CanonicalLemma + "\x00" + item.UPOS
		counts[key] = item.OccurrenceCount
		uposByIdentity[key] = item.UPOS
		reserved, reserveErr := h.services.Store.LemmaReview.IsReservedVocabulary(r.Context(), user(r).ID, item.Language, item.CanonicalLemma, item.UPOS)
		if reserveErr != nil {
			return nil, reserveErr
		}
		reservedByIdentity[key] = reserved
	}
	beforeCounts := make(map[string]int64, len(counts))
	maps.Copy(beforeCounts, counts)
	affectedKeys := make(map[string]bool)
	for _, occurrence := range chosen {
		before := occurrence.CanonicalLemma
		if occurrence.CorrectedLemma != "" {
			before = occurrence.CorrectedLemma
		}
		beforeDisplay := before
		if occurrence.Excluded {
			beforeDisplay = "excluded"
			before = ""
		}
		after := lemma
		if action == "keep" {
			after = occurrence.CanonicalLemma
		}
		if action == "exclude" {
			after = ""
		}
		if before != "" {
			key := before + "\x00" + occurrence.UPOS
			affectedKeys[key] = true
			counts[key]--
			uposByIdentity[key] = occurrence.UPOS
		}
		if after != "" {
			key := after + "\x00" + occurrence.UPOS
			affectedKeys[key] = true
			counts[key]++
			uposByIdentity[key] = occurrence.UPOS
		}
		afterDisplay := after
		if afterDisplay == "" {
			afterDisplay = "excluded"
		}
		p.Changes = append(p.Changes, lemmaOccurrenceChange{Sentence: occurrence.SentenceText, Before: beforeDisplay, After: afterDisplay})
	}
	impactKeys := make(map[string]bool)
	for key := range affectedKeys {
		if _, ok := reservedByIdentity[key]; !ok {
			parts := strings.SplitN(key, "\x00", 2)
			reserved, reserveErr := h.services.Store.LemmaReview.IsReservedVocabulary(r.Context(), user(r).ID, language, parts[0], parts[1])
			if reserveErr != nil {
				return nil, reserveErr
			}
			reservedByIdentity[key] = reserved
		}
		if beforeCounts[key] >= 3 || counts[key] >= 3 {
			impactKeys[key] = true
		}
	}
	orderedImpactKeys := make([]string, 0, len(impactKeys))
	for key := range impactKeys {
		orderedImpactKeys = append(orderedImpactKeys, key)
	}
	sort.Strings(orderedImpactKeys)
	for _, key := range orderedImpactKeys {
		parts := strings.SplitN(key, "\x00", 2)
		lemmaName, upos := parts[0], uposByIdentity[key]
		before, after := beforeCounts[key], counts[key]
		wasKnown, isKnown := knownIdentity(lemmaName, upos), knownIdentity(lemmaName, upos)
		reserved := reservedByIdentity[key]
		p.Impacts = append(p.Impacts, lemmaIdentityImpact{Lemma: lemmaName, UPOS: upos, BeforeCount: before, AfterCount: after, BeforeKnown: wasKnown, AfterKnown: isKnown, BeforeReserved: reserved, AfterReserved: reserved, BeforeEligible: before >= 3 && !wasKnown && !reserved, AfterEligible: after >= 3 && !isKnown && !reserved})
	}
	return p, nil
}

func lemmaReviewFingerprint(occurrences []domain.LemmaReviewOccurrence) string {
	h := sha256.New()
	for _, o := range occurrences {
		_, _ = h.Write([]byte(strings.Join([]string{o.AnalysisRunID, o.SourceDocumentID, strconv.FormatInt(o.StartOffset, 10), strconv.FormatInt(o.EndOffset, 10), o.Surface, o.RawLemma, o.CanonicalLemma, o.UPOS, o.CorrectedLemma, strconv.FormatBool(o.Excluded)}, "\x00") + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (h *Handler) lemmaDecisionStateFingerprint(r *http.Request, owner string, occurrences []domain.LemmaReviewOccurrence) (string, error) {
	digest := sha256.New()
	_, _ = digest.Write([]byte(lemmaReviewFingerprint(occurrences)))
	if len(occurrences) == 0 {
		return hex.EncodeToString(digest.Sum(nil)), nil
	}
	insights, err := h.services.Store.LemmaReview.GetAnalysisCorpusVocabulary(r.Context(), owner, occurrences[0].CorpusID)
	if err != nil {
		return "", err
	}
	identities := make([]string, 0, len(insights.Lemmas))
	for _, item := range insights.Lemmas {
		reserved, reserveErr := h.services.Store.LemmaReview.IsReservedVocabulary(r.Context(), owner, item.Language, item.CanonicalLemma, item.UPOS)
		if reserveErr != nil {
			return "", reserveErr
		}
		identities = append(identities, strings.Join([]string{item.CanonicalLemma, item.UPOS, strconv.FormatInt(item.OccurrenceCount, 10), strconv.FormatBool(reserved)}, "\x00"))
	}
	sort.Strings(identities)
	for _, item := range identities {
		_, _ = digest.Write([]byte(item + "\n"))
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	known, err := h.services.Store.LemmaReview.ListKnownVocabulary(r.Context(), owner, language)
	if err != nil {
		return "", err
	}
	knownIdentities := make([]string, 0, len(known))
	for _, item := range known {
		knownIdentities = append(knownIdentities, item.CanonicalLemma+"\x00"+item.UPOS)
	}
	sort.Strings(knownIdentities)
	for _, item := range knownIdentities {
		_, _ = digest.Write([]byte(item + "\n"))
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func lemmaProposalFingerprint(state, action, lemma string, selected []int) string {
	values := make([]string, len(selected))
	for i, value := range selected {
		values[i] = strconv.Itoa(value)
	}
	sort.Strings(values)
	digest := sha256.Sum256([]byte(state + "\x00" + action + "\x00" + lemma + "\x00" + strings.Join(values, ",")))
	return hex.EncodeToString(digest[:])
}

func (h *Handler) lemmaReviewWritable(w http.ResponseWriter, r *http.Request, owner domain.User, bookID string) bool {
	detail, err := h.services.Store.Books.GetBookDetail(r.Context(), owner.ID, bookID)
	if errors.Is(err, persistence.ErrNotFound) {
		http.NotFound(w, r)
		return false
	}
	if err != nil {
		fail(w, err)
		return false
	}
	language, _ := activeStudyLanguageForContext(r.Context())
	if language == "" || detail.Book.LanguageTag != language {
		http.Error(w, "Choose this Book's study language in Reading before reviewing occurrences.", http.StatusConflict)
		return false
	}
	current, err := h.services.Store.CurrentReading.GetCurrentReading(r.Context(), owner.ID, language)
	if err != nil {
		fail(w, err)
		return false
	}
	if current.IsActive() && current.BookID == bookID {
		http.Error(w, "Stop this Book's current reading before changing its vocabulary.", http.StatusConflict)
		return false
	}
	return true
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func (h *Handler) confirmLemmaProposal(w http.ResponseWriter, r *http.Request, owner domain.User) {
	bookID, form := strings.TrimSpace(r.PathValue("bookID")), strings.TrimSpace(r.FormValue("form"))
	store := h.services.Store.LemmaReview
	matches, err := store.ListLemmaReviewOccurrences(r.Context(), owner.ID, bookID, form)
	if err != nil {
		fail(w, err)
		return
	}
	selected := r.Form["selected"]
	if len(selected) == 0 {
		http.Error(w, "The preview contains no selected occurrences.", http.StatusBadRequest)
		return
	}
	action, lemma := r.FormValue("decision"), strings.TrimSpace(r.FormValue("lemma"))
	language, _ := activeStudyLanguageForContext(r.Context())
	profile, profileErr := canonicalization.For(language)
	if profileErr != nil {
		http.Error(w, "Choose a supported study language before confirming.", http.StatusConflict)
		return
	}
	if action == "correct" {
		lemma = profile.Canonical(lemma)
		if !lexical.IsLemma(lemma) {
			http.Error(w, "Enter one valid canonical lemma without spaces.", http.StatusBadRequest)
			return
		}
	}
	if action != "correct" && action != "keep" && action != "exclude" {
		http.Error(w, "Invalid decision.", http.StatusBadRequest)
		return
	}
	indices := make(map[int]bool, len(selected))
	orderedIndices := make([]int, 0, len(selected))
	for _, raw := range selected {
		i, parseErr := strconv.Atoi(raw)
		if parseErr != nil || i < 0 || i >= len(matches) {
			http.Error(w, "The preview selection is invalid.", http.StatusBadRequest)
			return
		}
		indices[i] = true
		orderedIndices = append(orderedIndices, i)
	}
	stateFingerprint, err := h.lemmaDecisionStateFingerprint(r, owner.ID, matches)
	if err != nil {
		fail(w, err)
		return
	}
	if lemmaProposalFingerprint(stateFingerprint, action, lemma, orderedIndices) != r.FormValue("fingerprint") {
		http.Error(w, "Learner vocabulary state or this proposal changed after preview. No decision was saved; review it again.", http.StatusConflict)
		return
	}
	decisions := make([]domain.LemmaReviewDecision, 0, len(indices))
	for i, occurrence := range matches {
		if !indices[i] {
			continue
		}
		decision := lemma
		excluded := action == "exclude"
		if action == "keep" {
			decision = occurrence.CanonicalLemma
		}
		decisions = append(decisions, domain.LemmaReviewDecision{Occurrence: occurrence, CanonicalLemma: decision, Excluded: excluded, NormalizationProfile: profile.Name(), NormalizationVersion: profile.Version()})
	}
	if err := store.PutLemmaDecisions(r.Context(), decisions); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			http.Error(w, "This occurrence changed or its vocabulary has frozen. No decision was saved; review it again.", http.StatusConflict)
			return
		}
		fail(w, err)
		return
	}
	redirect(w, r, "/reading/books/"+url.PathEscape(bookID)+"/lemma-review?form="+url.QueryEscape(form))
}
