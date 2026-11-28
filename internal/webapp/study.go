package webapp

import (
	"context"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lemmadisplay"
	"github.com/justin-hayes/mouseion/internal/study"
)

// studyPageView is what the Study page renders for one study language.
type studyPageView struct {
	Language, LanguageLabel string
	// Available is false for a language whose card policy has no contextual
	// Gloss to anchor a learned meaning (Latin and Ancient Greek).
	Available bool
	// SourceNotices are a source-evidence language's imported-source notices,
	// each with its pin and licence (ADR 0138 §3).
	SourceNotices []string
	Counts        domain.StudyMaterialCounts
	// IdiomCounts is the part of Counts that is Idioms, so the page can count
	// words and Idioms each by name.
	IdiomCounts domain.StudyMaterialCounts
	// Card is the word to show now, if one is due.
	Card *studyCardView
	// Offer is how many words the day's New-word batch holds; zero means no
	// batch is on offer. ShortBatch is true when the batch holds fewer words
	// than a full batch because more are still being prepared.
	Offer      int
	ShortBatch bool
	// OfferIdioms is how many of the Offer are Idioms.
	OfferIdioms int
	// OfferNumber is the ordinal of the batch on offer among the day's
	// batches. Declining presents it back, so a stale tap on a batch that is no
	// longer the one offered is ignored.
	OfferNumber int
	// Repeating is how many words are in learning and come back later today.
	Repeating int
	// RepeatingIdioms is how many of the Repeating are Idioms.
	RepeatingIdioms int
	// Remaining is how many of Today's due reviews are still unanswered, the
	// same-day repeats included. Finished is true once none is left, and stays
	// true until the next Study day.
	Remaining int
	Finished  bool
	// Reintroducing is how many due reviews wait for new Study material after a
	// correction; Withdrawn names the words a correction left with no valid
	// occurrence. Their progress is kept.
	Reintroducing int
	// FirstMaterial is how many due reviews of words migrated from Known
	// vocabulary wait for their first Study material.
	FirstMaterial int
	// Held is how many words are held by an open Wrong-word report.
	Held      int
	Withdrawn []string
	// NoSuitableContext names the words withdrawn because every context was
	// reported as unsuitable or fails the sentence-quality gate (ADR 0135 §3).
	// Their progress is kept, and they return when a new Book provides a context.
	NoSuitableContext []string
	// Reported is the card the learner has just reported as an unsuitable
	// context, shown once more in place of the next card.
	Reported *studyReportedView
	// WaitingForSentence names the words a correction left with valid
	// occurrences but no usable sentence yet, so their replacement material
	// could not be prepared. They stay out of Today, and return when the
	// evidence changes.
	WaitingForSentence []string
	// FluencyUnlocked is true once the finish line is reached and no offered
	// batch awaits an answer (domain.FluencyUnlocked). Fluency is the card
	// shown then, if any context is ready, and a batch the learner declined
	// stays takeable beside it.
	FluencyUnlocked bool
	Fluency         *fluencyCardView
	// NothingMature is true when Fluency is unlocked and no word is mature yet.
	NothingMature bool
	// FluencyPreparing is true when words are mature but none of their contexts
	// is ready.
	FluencyPreparing bool
}

// cardFace is what a graded card and a Fluency card present alike: the
// sentence with the target word marked, and the answer revealed by Show answer.
type cardFace struct {
	// Before, Target and After are the sentence split around the marked word.
	Before, Target, After string
	Marked                bool
	// Idiom marks an Idiom's card: Segments are the sentence with every matched
	// word marked, and TranslationHTML is the translation with its aligned
	// excerpts highlighted (owned, escaped markup).
	Idiom           bool
	Segments        []sentenceSegment
	TranslationHTML string
	ShowAnswer      bool
	Headword        string
	Gloss           string
	Translation     string
	// Source is a source-evidence word's dictionary outline and passage; it is
	// zero for every other card.
	Source sourceCardView
}

// newCardFace builds the face of a card from its sentence and the word's
// learned meaning.
func newCardFace(language string, progress domain.StudyProgress, sentence, targetWord, translation string, showAnswer bool) cardFace {
	face := cardFace{
		ShowAnswer:  showAnswer,
		Headword:    lemmadisplay.Format(language, progress.CanonicalLemma, progress.UPOS),
		Gloss:       progress.LearnedMeaning,
		Translation: translation,
	}
	face.Before, face.Target, face.After, face.Marked = markTarget(sentence, targetWord)
	return face
}

// fluencyCardView is one Fluency card: another sentence for a mature word.
type fluencyCardView struct {
	cardFace
	ContextID string
}

// studyCardView is one card of Today.
type studyCardView struct {
	cardFace
	ProgressID, MaterialID string
	// Reps is the number of grades recorded when the card was shown. Grading
	// presents it back, so a double tap grades the card once.
	Reps         int64
	Introduction bool
	// AlreadyKnown offers the Already known answer: only on the review that
	// introduces a word, never on a later one.
	AlreadyKnown bool
	// Corrected marks the re-introduction of a word after a correction.
	Corrected bool
	// Idiom marks the card of an Idiom, whose Report offers Unsuitable context
	// only.
	Idiom bool
	// NoReport withholds the Report control. An Idiom's Report replaces its
	// material, which cannot be prepared while the language's idiom switch is
	// off, so it is not offered then.
	NoReport bool
}

// study serves /{language}/study. Opening it asks for Study material to be
// prepared ahead of the new words Today will offer. It then shows the card
// that is due, or the day's New-word batch, and reports how many words are
// ready and being prepared. It never calls a provider.
func (h *Handler) study(w http.ResponseWriter, r *http.Request) {
	owner := user(r).ID
	language, label := activeStudyLanguageForContext(r.Context())
	view := studyPageView{Language: language, LanguageLabel: label, Available: study.Available(language)}
	if view.Available {
		if err := h.prepareStudyView(r, owner, &view); err != nil {
			fail(w, err)
			return
		}
		view.SourceNotices = h.pageSourceNotices(r.Context(), owner, view)
	}
	render(w, r, StudyPage(user(r), h.csrf(w, r), view))
}

// pageSourceNotices are the notices of a source-evidence language's Study page,
// each with its pin and licence (ADR 0138 §3). The dictionary's pin is the
// loaded index's own, and the translation pins are those of the passages the
// learner's Study material carries and the card shows, so an idle page credits
// what a card would, with no card to read them from.
func (h *Handler) pageSourceNotices(ctx context.Context, owner string, view studyPageView) []string {
	policy := cardpolicy.For(view.Language)
	pin := study.DictionaryPin(h.services.Dictionary, policy)
	var passagePins []string
	if view.Card != nil {
		if pin == "" {
			pin = view.Card.Source.dictionaryPin
		}
		passagePins = append(passagePins, view.Card.Source.passagePin)
	}
	if material, err := h.services.Store.Study.ListCurrentStudyMaterial(ctx, owner, view.Language); err == nil {
		for _, item := range material {
			passagePins = append(passagePins, item.Passage.TranslationPin)
		}
	} else {
		log.Printf("mouseion: list study material for source notices: %v", err)
	}
	slices.Sort(passagePins)
	return sourceNotices(policy, pin, passagePins...)
}

// prepareStudyView reconciles the learner's Study reports and corrections,
// asks for material and Fluency contexts to be prepared ahead, and loads what
// the page shows.
func (h *Handler) prepareStudyView(r *http.Request, owner string, view *studyPageView) error {
	language := view.Language
	// Corrections are applied first, so a word whose sentence no longer
	// belongs to it is prepared again in the same request.
	if err := study.ReconcileReports(r.Context(), h.services.Store.Study, owner, language, h.now()); err != nil {
		return err
	}
	if err := h.reconcileStudyCorrections(r.Context(), owner, language, h.now()); err != nil {
		return err
	}
	if h.services.Study != nil {
		// Preparation is best effort: the page still reports what is ready.
		if err := h.services.Study.EnqueueStudyPreparation(r.Context(), owner, language); err != nil {
			log.Printf("mouseion: enqueue study preparation: %v", err)
		}
		// Fluency contexts are prepared for mature words by a durable job
		// too, never by this request.
		if err := h.services.Study.EnqueueFluencyPreparation(r.Context(), owner, language); err != nil {
			log.Printf("mouseion: enqueue fluency preparation: %v", err)
		}
	}
	// The Undo offer of a report the learner has acted past is over: the report
	// ends as retired (ADR 0135 §3).
	if err := h.services.Store.Study.RetireClosedStudyReports(r.Context(), owner, language, h.now()); err != nil {
		return err
	}
	// The reported card is read first: while its Undo offer is shown, the page
	// offers no new batch behind it.
	if err := h.loadReportedCard(r, owner, view); err != nil {
		return err
	}
	return h.loadStudyView(r, owner, view)
}

// noteCorrections records the words a correction left waiting for new
// material, waiting for a usable sentence, or withdrawn. A word the study job
// could not prepare new material for is withdrawn from Today either way; it is
// reported as withdrawn only when no valid occurrence of it is left.
func (h *Handler) noteCorrections(ctx context.Context, owner string, view *studyPageView, progress []domain.StudyProgress, now time.Time) error {
	for _, item := range progress {
		switch {
		case item.Withdrawn && item.WithdrawnReason == domain.StudyWithdrawnNoSuitableContext:
			view.NoSuitableContext = append(view.NoSuitableContext, lemmadisplay.Format(view.Language, item.CanonicalLemma, item.UPOS))
		case item.Withdrawn:
			occurrences, err := h.services.Store.Study.FindStudyOccurrences(ctx, owner, item.StudyItem, domain.StudyOccurrenceRef{})
			if err != nil {
				return err
			}
			word := lemmadisplay.Format(view.Language, item.CanonicalLemma, item.UPOS)
			if len(occurrences) == 0 {
				view.Withdrawn = append(view.Withdrawn, word)
			} else {
				view.WaitingForSentence = append(view.WaitingForSentence, word)
			}
		case domain.StudyAwaitsMaterial(item, now) && item.AwaitingFirstMaterial:
			view.FirstMaterial++
		case domain.StudyAwaitsMaterial(item, now):
			view.Reintroducing++
		}
	}
	return nil
}

// studyHeldCount is how many words an open Wrong-word report holds, read
// from the open reports rather than Study progress: a new word reported on its
// introduction card has no Study progress left (ADR 0135 §2). The open reports
// are few, so the read stays bounded.
func (h *Handler) studyHeldCount(ctx context.Context, owner, language string) (int, error) {
	reports, err := h.services.Store.Study.ListOpenStudyReports(ctx, owner, language, domain.StudyReportWrongWord)
	if err != nil {
		return 0, err
	}
	held := map[domain.StudyItem]struct{}{}
	for _, identity := range domain.StudyHeldIdentities(reports) {
		held[identity] = struct{}{}
	}
	return len(held), nil
}

// studyRepeatingCount is how many words are still in learning or relearning
// once no card is due: they come back later today.
func studyRepeatingCount(progress []domain.StudyProgress) (repeating, idioms int) {
	for _, item := range progress {
		if !item.Withdrawn && !item.Held && (item.Memory.State == domain.StudyStateLearning || item.Memory.State == domain.StudyStateRelearning) {
			repeating++
			if item.IsIdiom() {
				idioms++
			}
		}
	}
	return repeating, idioms
}

// studyIdiomsOf counts the Idioms among a batch.
func studyIdiomsOf(batch []domain.StudyBatchItem) int {
	idioms := 0
	for _, item := range batch {
		if item.IsIdiom() {
			idioms++
		}
	}
	return idioms
}

func (h *Handler) loadStudyView(r *http.Request, owner string, view *studyPageView) error {
	ctx, store, now := r.Context(), h.services.Store.Study, h.now()
	counts, err := store.CountStudyMaterial(ctx, owner, view.Language)
	if err != nil {
		return err
	}
	progress, err := store.ListStudyProgress(ctx, owner, view.Language)
	if err != nil {
		return err
	}
	if counts, view.IdiomCounts, err = h.idiomMaterialCounts(r, owner, view.Language, counts, progress); err != nil {
		return err
	}
	view.Counts = counts
	today := domain.StudyFinishLine(progress, now)
	view.Remaining, view.Finished = today.Remaining, today.Finished
	if view.Held, err = h.studyHeldCount(ctx, owner, view.Language); err != nil {
		return err
	}
	if err := h.noteCorrections(ctx, owner, view, progress, now); err != nil {
		return err
	}
	if view.Reported != nil {
		// The reported card is shown with Undo; it is not a new card or offer.
		return nil
	}
	card, err := store.NextStudyCard(ctx, owner, view.Language, now)
	switch {
	case err == nil:
		view.Card = newStudyCardView(view.Language, card, r.URL.Query().Get("answer") == "1")
		view.Card.NoReport = card.Progress.IsIdiom() && (h.services.Study == nil || !h.services.Study.IdiomsActive(view.Language))
		attachSourceEvidence(ctx, h.services.Dictionary, view.Language, card, view.Card)
		return h.markIdiomCard(ctx, owner, card, view.Card)
	case !errors.Is(err, domain.ErrNotFound):
		return err
	}
	view.Repeating, view.RepeatingIdioms = studyRepeatingCount(progress)
	if !today.Finished {
		// New words come only after the due reviews are answered.
		return nil
	}
	offer, err := h.studyBatchOffer(r, owner, view.Language, progress, now)
	if err != nil {
		return err
	}
	view.Offer, view.OfferIdioms = len(offer.Batch), studyIdiomsOf(offer.Batch)
	if view.Offer > 0 {
		view.ShortBatch = domain.StudyBatchIsShort(offer.Batch, counts.Preparing)
		view.OfferNumber = offer.Ledger.OfferNumber()
		// The ledger counts a batch as offered the first time Today shows it.
		if err := store.RecordStudyBatchOffer(ctx, owner, view.Language, domain.StudyDay(now), view.OfferNumber); err != nil {
			return err
		}
	}
	view.FluencyUnlocked = domain.FluencyUnlocked(today, offer.Ledger, view.Offer > 0)
	if !view.FluencyUnlocked {
		return nil
	}
	return h.loadFluencyView(r, owner, view, progress, now)
}

// loadFluencyView picks the Fluency card to show with the weighted rotation over
// the mature words' contexts (domain.NextFluencyCard). Fluency only reads Study
// progress.
func (h *Handler) loadFluencyView(r *http.Request, owner string, view *studyPageView, progress []domain.StudyProgress, now time.Time) error {
	mature := false
	for _, item := range progress {
		mature = mature || domain.IsStudyMature(item)
	}
	if !mature {
		view.NothingMature = true
		return nil
	}
	contexts, err := h.services.Store.Study.ListStudyFluencyContexts(r.Context(), owner, view.Language)
	if err != nil {
		return err
	}
	// Show answer sends back the card it revealed, so the card cannot change
	// between its question and its answer.
	card, ok := domain.FluencyCardByID(contexts, progress, r.URL.Query().Get("context"))
	if !ok || r.URL.Query().Get("answer") != "1" {
		card, ok = domain.NextFluencyCard(domain.FluencyRotation{Contexts: contexts, Progress: progress, Recall: h.fluencyRecall(progress, now), Now: now})
	}
	if !ok {
		view.FluencyPreparing = true
		return nil
	}
	view.Fluency = newFluencyCardView(view.Language, card, r.URL.Query().Get("answer") == "1")
	return nil
}

// fluencyRecall is each mature word's recall now, for the rotation's weights. A
// word whose recall cannot be computed has no entry and so no recall term.
func (h *Handler) fluencyRecall(progress []domain.StudyProgress, now time.Time) map[string]float64 {
	recaller, ok := h.services.StudyScheduler.(domain.StudyRecaller)
	if !ok {
		return nil
	}
	recall := map[string]float64{}
	for _, item := range progress {
		if !domain.IsStudyMature(item) {
			continue
		}
		if value, err := recaller.Recall(item.Memory, now); err == nil {
			recall[item.ID] = value
		}
	}
	return recall
}

// markIdiomCard marks an Idiom card's matched words from the positions its
// material stored: the surface of each matched token of the sentence. An Idiom
// whose sentence can no longer be read keeps the marking taken from its target
// word.
func (h *Handler) markIdiomCard(ctx context.Context, owner string, card domain.StudyCard, view *studyCardView) error {
	if !card.Progress.IsIdiom() || len(card.Material.MatchedTokenOrdinals) == 0 {
		return nil
	}
	material := card.Material
	sentences, err := h.services.Store.Study.ListCorpusSentences(ctx, owner, material.CorpusID, []int64{material.SentenceOrdinal})
	if err != nil {
		return err
	}
	forms := matchedSurfaces(sentences[material.SentenceOrdinal].Tokens, material.MatchedTokenOrdinals)
	if len(forms) > 0 {
		view.markIdiom(material.Sentence, forms, material.SentenceTranslation, material.SentenceTranslationTargets)
	}
	return nil
}

// matchedSurfaces are the surfaces of the tokens at the matched ordinals, in
// sentence order.
func matchedSurfaces(tokens []analyzer.Token, ordinals []int) []string {
	forms := make([]string, 0, len(ordinals))
	for _, ordinal := range ordinals {
		if ordinal >= 0 && ordinal < len(tokens) {
			forms = append(forms, tokens[ordinal].Surface)
		}
	}
	return forms
}

func newFluencyCardView(language string, card domain.FluencyCard, showAnswer bool) *fluencyCardView {
	face := newCardFace(language, card.Progress, card.Context.Sentence, card.Context.TargetWord, card.Context.SentenceTranslation, showAnswer)
	if card.Progress.IsIdiom() {
		face.markIdiom(card.Context.Sentence, idiomForms(card.Context.TargetWord), card.Context.SentenceTranslation, card.Context.SentenceTranslationTargets)
	}
	return &fluencyCardView{ContextID: card.Context.ID, cardFace: face}
}

// studyBatchOffer is what Today offers now: the batch, and the day's ledger
// and batch limit it was decided under. The batch is empty when none is on
// offer: the due reviews are not all answered, words taken earlier are not yet
// introduced, the day's cap or the backlog brake is reached, or no word is
// ready. Declining does not empty it: Today stops offering, but the batch stays
// takeable from inside Fluency. Open tells the first four apart from the last.
type studyBatchOffer struct {
	// Open is true when a batch may be offered, whether or not a word is ready.
	Open   bool
	Batch  []domain.StudyBatchItem
	Ledger domain.StudyBatchLedger
	Limit  int
}

func (h *Handler) studyBatchOffer(r *http.Request, owner, language string, progress []domain.StudyProgress, now time.Time) (studyBatchOffer, error) {
	ledger, err := h.services.Store.Study.StudyBatchLedger(r.Context(), owner, language, domain.StudyDay(now))
	if err != nil {
		return studyBatchOffer{}, err
	}
	forecast := domain.StudyForecast(progress, now)
	offer := studyBatchOffer{Ledger: ledger, Limit: domain.StudyBatchesAllowed(forecast)}
	// New words come only after the due reviews are answered, and the words
	// already taken lead them.
	if !domain.StudyFinishLine(progress, now).Finished || len(domain.StudyUnintroduced(progress, now)) > 0 || !domain.StudyBatchOfferable(ledger, forecast) {
		return offer, nil
	}
	offer.Open = true
	offer.Batch, err = h.nextStudyBatch(r, owner, language, progress, now)
	return offer, err
}

// nextStudyBatch is the batch Today would offer now: the words carried over
// from earlier Study days, then the first ready words in stream order that are
// not in study and not held by a Wrong-word report.
func (h *Handler) nextStudyBatch(r *http.Request, owner, language string, progress []domain.StudyProgress, now time.Time) ([]domain.StudyBatchItem, error) {
	store := h.services.Store.Study
	material, err := store.ListCurrentStudyMaterial(r.Context(), owner, language)
	if err != nil {
		return nil, err
	}
	// A word held by a report that left it no Study progress is passed over too;
	// the open reports are few, and their identities are all that is read.
	reports, err := store.ListOpenStudyReports(r.Context(), owner, language, domain.StudyReportWrongWord)
	if err != nil {
		return nil, err
	}
	held := domain.StudyHeldIdentities(reports)
	// Only a word whose material is ready can be offered, so only those words
	// need their place in the stream: the rest of the library's candidates are
	// never read.
	lookup := domain.StudyBatchLookup(material, progress, held)
	candidates, err := store.ListStudyCandidatesAmong(r.Context(), owner, language, lookup)
	if err != nil {
		return nil, err
	}
	idioms, err := h.readyIdiomCandidates(r, owner, language, lookup)
	if err != nil {
		return nil, err
	}
	candidates = append(candidates, idioms...)
	coverage, err := store.GetStudyReadingCoverage(r.Context(), owner, language)
	if err != nil {
		return nil, err
	}
	return domain.NextStudyBatch(domain.OrderStudyStream(candidates, coverage.BoostsCurrentReading()), material, progress, held, now), nil
}

// idiomMaterialCounts splits the Idioms out of the counts, so the page can
// count each kind by name, and returns the counts to show. While the language's
// idiom switch is off no Idiom is prepared or offered, so an Idiom that is not
// in study is taken out of the counts: a pending one must not hold the "still
// being prepared" note up. An Idiom already in study stays counted like a word
// in study, since it keeps being reviewed.
func (h *Handler) idiomMaterialCounts(r *http.Request, owner, language string, counts domain.StudyMaterialCounts, progress []domain.StudyProgress) (domain.StudyMaterialCounts, domain.StudyMaterialCounts, error) {
	var idioms domain.StudyMaterialCounts
	material, err := h.services.Store.Study.ListCurrentStudyMaterial(r.Context(), owner, language)
	if err != nil {
		return counts, idioms, err
	}
	active := h.services.Study != nil && h.services.Study.IdiomsActive(language)
	inStudy := map[domain.StudyItem]bool{}
	for _, item := range progress {
		inStudy[item.StudyItem] = true
	}
	for _, item := range material {
		if !item.IsIdiom() {
			continue
		}
		if !active && !inStudy[item.StudyItem] {
			switch item.State {
			case domain.StudyMaterialReady:
				counts.Ready--
			case domain.StudyMaterialPending:
				counts.Preparing--
			case domain.StudyMaterialUnpreparable:
				counts.Unpreparable--
			}
			continue
		}
		switch item.State {
		case domain.StudyMaterialReady:
			idioms.Ready++
		case domain.StudyMaterialPending:
			idioms.Preparing++
		case domain.StudyMaterialUnpreparable:
			idioms.Unpreparable++
		}
	}
	return counts, idioms, nil
}

// readyIdiomCandidates are the Idioms among the lookup that are still Study
// candidates: Idioms rank in the stream with words (ADR 0137 §4). None are
// while the language's idiom switch is off, so no new Idiom is offered then.
func (h *Handler) readyIdiomCandidates(r *http.Request, owner, language string, lookup []domain.StudyItem) ([]domain.StudyCandidate, error) {
	wanted := map[domain.StudyItem]bool{}
	for _, item := range lookup {
		if item.IsIdiom() {
			wanted[item] = true
		}
	}
	if len(wanted) == 0 || h.services.Study == nil {
		return nil, nil
	}
	all, err := h.services.Study.IdiomCandidates(r.Context(), owner, language)
	if err != nil {
		return nil, err
	}
	var candidates []domain.StudyCandidate
	for _, candidate := range all {
		if wanted[candidate.StudyItem] {
			candidates = append(candidates, candidate)
		}
	}
	return candidates, nil
}

func newStudyCardView(language string, card domain.StudyCard, showAnswer bool) *studyCardView {
	memory := card.Progress.Memory
	face := newCardFace(language, card.Progress, card.Material.Sentence, card.Material.TargetWord, card.Material.SentenceTranslation, showAnswer)
	if card.Progress.IsIdiom() {
		face.markIdiom(card.Material.Sentence, idiomForms(card.Material.TargetWord), card.Material.SentenceTranslation, card.Material.SentenceTranslationTargets)
	}
	return &studyCardView{
		cardFace:   face,
		ProgressID: card.Progress.ID, MaterialID: card.Progress.MaterialID,
		Reps:         memory.Reps,
		Introduction: memory.State == domain.StudyStateNew || memory.State == domain.StudyStateLearning,
		AlreadyKnown: domain.StudyAlreadyKnownOffered(card.Progress),
		Corrected:    card.Progress.Corrected(),
		Idiom:        card.Progress.IsIdiom(),
	}
}

// markTarget splits a sentence around the first whole-word occurrence of the
// target word, so the template can mark it.
func markTarget(sentence, target string) (before, marked, after string, found bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return sentence, "", "", false
	}
	for from := 0; from <= len(sentence); {
		index := strings.Index(sentence[from:], target)
		if index < 0 {
			break
		}
		start, end := from+index, from+index+len(target)
		if wordBoundary(sentence, start, end) {
			return sentence[:start], sentence[start:end], sentence[end:], true
		}
		from = start + 1
	}
	return sentence, "", "", false
}

// wordBoundary reports whether sentence[start:end] is not part of a longer
// word.
func wordBoundary(sentence string, start, end int) bool {
	if previous, _ := utf8.DecodeLastRuneInString(sentence[:start]); unicode.IsLetter(previous) || unicode.IsDigit(previous) {
		return false
	}
	if next, _ := utf8.DecodeRuneInString(sentence[end:]); unicode.IsLetter(next) || unicode.IsDigit(next) {
		return false
	}
	return true
}

// studyForm reads the language of a Study action and answers the request when
// it is missing or the language has no Study. It returns the language and the
// page to return to.
func (h *Handler) studyForm(w http.ResponseWriter, r *http.Request) (language, back string, ok bool) {
	if !h.checkCSRF(w, r) {
		return "", "", false
	}
	language = canonicalization.NormalizeLanguage(r.FormValue("language"))
	if language == "" || !study.Available(language) {
		http.NotFound(w, r)
		return "", "", false
	}
	return language, studyLanguagePath(pageKindStudy, language), true
}

// takeStudyBatch introduces the words of today's New-word batch.
func (h *Handler) takeStudyBatch(w http.ResponseWriter, r *http.Request) {
	language, back, ok := h.studyForm(w, r)
	if !ok {
		return
	}
	owner, store, now := user(r).ID, h.services.Store.Study, h.now()
	progress, err := store.ListStudyProgress(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	offer, err := h.studyBatchOffer(r, owner, language, progress, now)
	if err != nil {
		fail(w, err)
		return
	}
	if len(offer.Batch) == 0 {
		// Nothing is on offer: a repeated tap, or the day's limit is reached.
		redirect(w, r, back)
		return
	}
	_, err = store.TakeStudyBatch(r.Context(), owner, language, domain.StudyDay(now), now, offer.Limit, offer.Batch)
	switch {
	case errors.Is(err, domain.ErrStudyBatchLimit), errors.Is(err, domain.ErrNotFound):
		// A repeated tap, or a batch that is no longer there: show Today again.
		redirect(w, r, back)
		return
	case err != nil:
		fail(w, err)
		return
	}
	if h.services.Study != nil {
		// Taking a batch is one of the moments preparation runs ahead.
		if err := h.services.Study.EnqueueStudyPreparation(r.Context(), owner, language); err != nil {
			log.Printf("mouseion: enqueue study preparation: %v", err)
		}
	}
	redirect(w, r, back)
}

// declineStudyBatch declines today's New-word batch. It introduces no word.
func (h *Handler) declineStudyBatch(w http.ResponseWriter, r *http.Request) {
	language, back, ok := h.studyForm(w, r)
	if !ok {
		return
	}
	owner, now := user(r).ID, h.now()
	progress, err := h.services.Store.Study.ListStudyProgress(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	offer, err := h.studyBatchOffer(r, owner, language, progress, now)
	if err != nil {
		fail(w, err)
		return
	}
	if number, numberErr := strconv.Atoi(r.FormValue("offer")); !offer.Open || len(offer.Batch) == 0 || offer.Ledger.Declined > 0 || numberErr != nil || number != offer.Ledger.OfferNumber() {
		// A stale tap: the batch it answers is not the one on offer now, or
		// Today has already stopped offering.
		redirect(w, r, back)
		return
	}
	err = h.services.Store.Study.DeclineStudyBatch(r.Context(), owner, language, domain.StudyDay(now), offer.Limit)
	if err != nil && !errors.Is(err, domain.ErrStudyBatchLimit) {
		fail(w, err)
		return
	}
	redirect(w, r, back)
}

// gradeStudyCard records the learner's grade for the card they were shown.
func (h *Handler) gradeStudyCard(w http.ResponseWriter, r *http.Request) {
	language, back, ok := h.studyForm(w, r)
	if !ok {
		return
	}
	grade, ok := domain.ParseStudyGrade(r.FormValue("grade"))
	reps, repsErr := strconv.ParseInt(r.FormValue("reps"), 10, 64)
	progressID := strings.TrimSpace(r.FormValue("progress"))
	if !ok || repsErr != nil || reps < 0 || progressID == "" {
		http.Error(w, "invalid grade", http.StatusBadRequest)
		return
	}
	if h.services.StudyScheduler == nil {
		fail(w, errors.New("study scheduler is not configured"))
		return
	}
	owner, store, now := user(r).ID, h.services.Store.Study, h.now()
	progress, err := store.ListStudyProgress(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	current := findStudyProgress(progress, progressID)
	if current == nil {
		http.NotFound(w, r)
		return
	}
	if current.Memory.Reps != reps || !domain.StudyCardIsDue(*current, now) {
		redirect(w, r, back)
		return
	}
	after, err := h.services.StudyScheduler.Schedule(current.Memory, grade, now)
	if err != nil {
		fail(w, err)
		return
	}
	err = store.RecordStudyGrade(r.Context(), owner, domain.StudyGradeRecord{
		ProgressID: progressID, ExpectedReps: reps, Grade: grade, ReviewedAt: now, StudyDay: domain.StudyDay(now), After: after,
	})
	if err != nil && !errors.Is(err, domain.ErrStudyStale) {
		fail(w, err)
		return
	}
	redirect(w, r, back)
}

func findStudyProgress(progress []domain.StudyProgress, id string) *domain.StudyProgress {
	for i := range progress {
		if progress[i].ID == id {
			return &progress[i]
		}
	}
	return nil
}

// alreadyKnownStudyCard records the learner's Already known answer to the card
// that introduces a word (ADR 0136 §1). It is an answer, never a grade: an
// Already-known event is stored and the word enters study Mature, with the
// introduction's place in the taken batch complete. It is refused for any
// review after the introduction.
func (h *Handler) alreadyKnownStudyCard(w http.ResponseWriter, r *http.Request) {
	language, back, ok := h.studyForm(w, r)
	if !ok {
		return
	}
	reps, repsErr := strconv.ParseInt(r.FormValue("reps"), 10, 64)
	progressID := strings.TrimSpace(r.FormValue("progress"))
	if repsErr != nil || reps < 0 || progressID == "" {
		http.Error(w, "invalid answer", http.StatusBadRequest)
		return
	}
	owner, store, now := user(r).ID, h.services.Store.Study, h.now()
	progress, err := store.ListStudyProgress(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	current := findStudyProgress(progress, progressID)
	if current == nil {
		http.NotFound(w, r)
		return
	}
	if current.Memory.Reps != reps || !domain.StudyCardIsDue(*current, now) || !domain.StudyAlreadyKnownOffered(*current) {
		redirect(w, r, back)
		return
	}
	err = store.RecordStudyAlreadyKnown(r.Context(), owner, domain.StudyAlreadyKnownRecord{
		ProgressID: progressID, ExpectedReps: reps, AnsweredAt: now, StudyDay: domain.StudyDay(now),
		After: domain.StudyAlreadyKnownMemory(current.Memory, now),
	})
	if err != nil && !errors.Is(err, domain.ErrStudyStale) {
		fail(w, err)
		return
	}
	redirect(w, r, back)
}

// recordFluencyResponse records the learner's answer to a Fluency card. Fluency
// is ungraded: the answer is kept as evidence and never changes Study progress
// or the schedule.
func (h *Handler) recordFluencyResponse(w http.ResponseWriter, r *http.Request) {
	language, back, ok := h.studyForm(w, r)
	if !ok {
		return
	}
	response, ok := domain.ParseFluencyResponse(r.FormValue("response"))
	contextID := strings.TrimSpace(r.FormValue("context"))
	if !ok || contextID == "" {
		http.Error(w, "invalid response", http.StatusBadRequest)
		return
	}
	owner, store, now := user(r).ID, h.services.Store.Study, h.now()
	progress, err := store.ListStudyProgress(r.Context(), owner, language)
	if err != nil {
		fail(w, err)
		return
	}
	// A response is accepted only where the page shows Fluency; one sent before
	// is a stale tap on a card that is no longer there.
	unlocked, err := h.fluencyUnlocked(r, owner, language, progress, now)
	if err != nil {
		fail(w, err)
		return
	}
	if !unlocked {
		redirect(w, r, back)
		return
	}
	err = store.RecordStudyFluencyResponse(r.Context(), owner, domain.FluencyResponseRecord{ContextID: contextID, Response: response, RespondedAt: now})
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		fail(w, err)
		return
	}
	redirect(w, r, back)
}

// fluencyUnlocked applies the page's unlock rule (domain.FluencyUnlocked) to
// the learner's current state.
func (h *Handler) fluencyUnlocked(r *http.Request, owner, language string, progress []domain.StudyProgress, now time.Time) (bool, error) {
	today := domain.StudyFinishLine(progress, now)
	if !today.Finished {
		return false, nil
	}
	offer, err := h.studyBatchOffer(r, owner, language, progress, now)
	if err != nil {
		return false, err
	}
	return domain.FluencyUnlocked(today, offer.Ledger, len(offer.Batch) > 0), nil
}
