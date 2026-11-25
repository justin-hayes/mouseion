package webapp

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lemmadisplay"
)

// studyPageView is what the Study page renders for one study language.
type studyPageView struct {
	Language, LanguageLabel string
	// Available is false for a language whose card policy has no contextual
	// Gloss to anchor a learned meaning (Latin and Ancient Greek).
	Available bool
	Counts    domain.StudyMaterialCounts
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
}

// studyCardView is one card: the sentence with the target word marked, and the
// answer side revealed by Show answer.
type studyCardView struct {
	ProgressID string
	// Reps is the number of grades recorded when the card was shown. Grading
	// presents it back, so a double tap grades the card once.
	Reps int64
	// Before, Target and After are the sentence split around the marked word.
	Before, Target, After string
	Marked                bool
	Introduction          bool
	ShowAnswer            bool
	Headword              string
	Gloss                 string
	Translation           string
}

// studyAvailable reports whether Study serves a study language: only those on
// the modern-language provider route (ADR 0133, ADR 0134 §1).
func studyAvailable(language string) bool {
	return !cardpolicy.For(language).SourceEvidence
}

// study serves /{language}/study. Opening it asks for Study material to be
// prepared ahead of the new words Today will offer. It then shows the card
// that is due, or the day's New-word batch, and reports how many words are
// ready and being prepared. It never calls a provider.
func (h *Handler) study(w http.ResponseWriter, r *http.Request) {
	owner := user(r).ID
	language, label := activeStudyLanguageForContext(r.Context())
	view := studyPageView{Language: language, LanguageLabel: label, Available: studyAvailable(language)}
	if view.Available {
		if h.services.Study != nil {
			// Preparation is best effort: the page still reports what is ready.
			if err := h.services.Study.EnqueueStudyPreparation(r.Context(), owner, language); err != nil {
				log.Printf("mouseion: enqueue study preparation: %v", err)
			}
		}
		if err := h.loadStudyView(r, owner, &view); err != nil {
			fail(w, err)
			return
		}
	}
	render(w, r, StudyPage(user(r), h.csrf(w, r), view))
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
	card, err := store.NextStudyCard(ctx, owner, view.Language, now)
	switch {
	case err == nil:
		view.Card = newStudyCardView(view.Language, card, r.URL.Query().Get("answer") == "1")
		return nil
	case !errors.Is(err, domain.ErrNotFound):
		return err
	}
	for _, item := range progress {
		if !item.Withdrawn && (item.Memory.State == domain.StudyStateLearning || item.Memory.State == domain.StudyStateRelearning) {
			view.Repeating++
		}
	}
	if !today.Finished {
		// New words come only after the due reviews are answered.
		return nil
	}
	offer, err := h.studyBatchOffer(r, owner, view.Language, progress, now)
	if err != nil {
		return err
	}
	view.Offer = len(offer.Batch)
	if view.Offer == 0 {
		return nil
	}
	view.ShortBatch = domain.StudyBatchIsShort(offer.Batch, counts.Preparing)
	view.OfferNumber = offer.Ledger.Decided() + 1
	// The ledger counts a batch as offered the first time Today shows it.
	return store.RecordStudyBatchOffer(ctx, owner, view.Language, domain.StudyDay(now), offer.Ledger.Decided()+1)
}

// studyBatchOffer is what Today offers now: the batch, and the day's ledger
// and batch limit it was decided under. The batch is empty when none is on
// offer: the due reviews are not all answered, words taken earlier are not yet
// introduced, the day's cap or the backlog brake is reached, the learner
// declined, or no word is ready. Open tells the first five apart from the last.
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
// not in study.
func (h *Handler) nextStudyBatch(r *http.Request, owner, language string, progress []domain.StudyProgress, now time.Time) ([]domain.StudyBatchWord, error) {
	store := h.services.Store.Study
	candidates, err := store.ListStudyCandidates(r.Context(), owner, language)
	if err != nil {
		return nil, err
	}
	material, err := store.ListCurrentStudyMaterial(r.Context(), owner, language)
	if err != nil {
		return nil, err
	}
	return domain.NextStudyBatch(domain.OrderStudyStream(candidates), material, progress, now), nil
}

func newStudyCardView(language string, card domain.StudyCard, showAnswer bool) *studyCardView {
	view := &studyCardView{
		ProgressID: card.Progress.ID, Reps: card.Progress.Memory.Reps, Introduction: card.Progress.Memory.State == domain.StudyStateNew || card.Progress.Memory.State == domain.StudyStateLearning, ShowAnswer: showAnswer,
		Headword:    lemmadisplay.Format(language, card.Progress.CanonicalLemma, card.Progress.UPOS),
		Gloss:       card.Progress.LearnedMeaning,
		Translation: card.Material.SentenceTranslation,
	}
	view.Before, view.Target, view.After, view.Marked = markTarget(card.Material.Sentence, card.Material.TargetWord)
	return view
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
	if language == "" || !studyAvailable(language) {
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
	if number, numberErr := strconv.Atoi(r.FormValue("offer")); !offer.Open || len(offer.Batch) == 0 || numberErr != nil || number != offer.Ledger.Decided()+1 {
		// A stale tap: the batch it answers is not the one on offer now.
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
