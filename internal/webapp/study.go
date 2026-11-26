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
	// Card is the word to show now, if one is due.
	Card *studyCardView
	// Offer is how many words the day's New-word batch holds; zero means no
	// batch is on offer. ShortBatch is true when the batch holds fewer words
	// than a full batch because more are still being prepared.
	Offer      int
	ShortBatch bool
	// OfferNumber is the ordinal of the batch on offer among the day's
	// batches. Declining presents it back, so a stale tap on a batch that is no
	// longer the one offered is ignored.
	OfferNumber int
	// Repeating is how many words are in learning and come back later today.
	Repeating int
	// Remaining is how many of Today's due reviews are still unanswered, the
	// same-day repeats included. Finished is true once none is left, and stays
	// true until the next Study day.
	Remaining int
	Finished  bool
	// Reintroducing is how many due reviews wait for new Study material after a
	// correction; Withdrawn names the words a correction left with no valid
	// occurrence. Their progress is kept.
	Reintroducing int
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
	ShowAnswer            bool
	Headword              string
	Gloss                 string
	Translation           string
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
	// Corrected marks the re-introduction of a word after a correction.
	Corrected bool
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
	pin := dictionaryPin(h.services.Dictionary, policy)
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
			occurrences, err := h.services.Store.Study.FindStudyOccurrences(ctx, owner, item.StudyIdentity, domain.StudyOccurrenceRef{})
			if err != nil {
				return err
			}
			word := lemmadisplay.Format(view.Language, item.CanonicalLemma, item.UPOS)
			if len(occurrences) == 0 {
				view.Withdrawn = append(view.Withdrawn, word)
			} else {
				view.WaitingForSentence = append(view.WaitingForSentence, word)
			}
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
	held := map[domain.StudyIdentity]struct{}{}
	for _, identity := range domain.StudyHeldIdentities(reports) {
		held[identity] = struct{}{}
	}
	return len(held), nil
}

// studyRepeatingCount is how many words are still in learning or relearning
// once no card is due: they come back later today.
func studyRepeatingCount(progress []domain.StudyProgress) int {
	repeating := 0
	for _, item := range progress {
		if !item.Withdrawn && !item.Held && (item.Memory.State == domain.StudyStateLearning || item.Memory.State == domain.StudyStateRelearning) {
			repeating++
		}
	}
	return repeating
}

func (h *Handler) loadStudyView(r *http.Request, owner string, view *studyPageView) error {
	ctx, store, now := r.Context(), h.services.Store.Study, h.now()
	counts, err := store.CountStudyMaterial(ctx, owner, view.Language)
	if err != nil {
		return err
	}
	view.Counts = counts
	progress, err := store.ListStudyProgress(ctx, owner, view.Language)
	if err != nil {
		return err
	}
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
		attachSourceEvidence(ctx, h.services.Dictionary, view.Language, card, view.Card)
		return nil
	case !errors.Is(err, domain.ErrNotFound):
		return err
	}
	view.Repeating = studyRepeatingCount(progress)
	if !today.Finished {
		// New words come only after the due reviews are answered.
		return nil
	}
	offer, err := h.studyBatchOffer(r, owner, view.Language, progress, now)
	if err != nil {
		return err
	}
	view.Offer = len(offer.Batch)
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

func newFluencyCardView(language string, card domain.FluencyCard, showAnswer bool) *fluencyCardView {
	return &fluencyCardView{
		ContextID: card.Context.ID,
		cardFace:  newCardFace(language, card.Progress, card.Context.Sentence, card.Context.TargetWord, card.Context.SentenceTranslation, showAnswer),
	}
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
	Batch  []domain.StudyBatchWord
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
func (h *Handler) nextStudyBatch(r *http.Request, owner, language string, progress []domain.StudyProgress, now time.Time) ([]domain.StudyBatchWord, error) {
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
	candidates, err := store.ListStudyCandidatesAmong(r.Context(), owner, language, domain.StudyBatchLookup(material, progress, held))
	if err != nil {
		return nil, err
	}
	return domain.NextStudyBatch(domain.OrderStudyStream(candidates), material, progress, held, now), nil
}

func newStudyCardView(language string, card domain.StudyCard, showAnswer bool) *studyCardView {
	memory := card.Progress.Memory
	return &studyCardView{
		cardFace:   newCardFace(language, card.Progress, card.Material.Sentence, card.Material.TargetWord, card.Material.SentenceTranslation, showAnswer),
		ProgressID: card.Progress.ID, MaterialID: card.Progress.MaterialID,
		Reps:         memory.Reps,
		Introduction: memory.State == domain.StudyStateNew || memory.State == domain.StudyStateLearning,
		Corrected:    card.Progress.Corrected(),
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
	var current *domain.StudyProgress
	for i := range progress {
		if progress[i].ID == progressID {
			current = &progress[i]
		}
	}
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
