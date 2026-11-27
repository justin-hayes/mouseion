//go:build integration

package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/study"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// studyProvider answers per word, so the order River runs jobs in never
// matters: "hund" has no defensible meaning, and "tisch" fails once the way a
// provider outage does.
type studyProvider struct {
	*testutil.FakeTranslationProvider
	mu       sync.Mutex
	failures map[string]int
	omitted  map[string]bool
	// disagree names the sentences a Fluency agreement request answers "another
	// sense" for, and fluency records the Fluency agreement requests it received.
	disagree map[string]bool
	fluency  []enrichment.TranslationRequest
	// pass records the evidence-assisted pass requests a source-evidence word's
	// material sent.
	pass []enrichment.TranslationRequest
	// citeLast makes the pass cite its last candidate sense instead of its first,
	// so a test can cite a later homograph entry.
	citeLast bool
	// idiomAnswers and idiom are the fake idiom answers and the idiom-card
	// requests received. See study_idiom_harness_integration_test.go.
	idiomAnswers map[string]idiomAnswer
	idiom        []enrichment.TranslationRequest
}

// passRequests are the evidence-assisted pass requests received so far.
func (p *studyProvider) passRequests() []enrichment.TranslationRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]enrichment.TranslationRequest(nil), p.pass...)
}

// fluencyRequests are the Fluency agreement requests received so far.
func (p *studyProvider) fluencyRequests() []enrichment.TranslationRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]enrichment.TranslationRequest(nil), p.fluency...)
}

// omit makes the provider answer that no sense of the lemma fits, or stop.
func (p *studyProvider) omit(lemma string, omitted bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.omitted == nil {
		p.omitted = map[string]bool{}
	}
	p.omitted[lemma] = omitted
}

func (p *studyProvider) Translate(ctx context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	if _, err := p.FakeTranslationProvider.Translate(ctx, request); err != nil {
		return enrichment.TranslationResponse{}, err
	}
	if request.RequireIdiomCard {
		return p.answerIdiom(request)
	}
	if request.RequireFluencyAgreement {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.fluency = append(p.fluency, request)
		if p.disagree[request.ExampleSentence] {
			return enrichment.TranslationResponse{FluencyReason: enrichment.FluencyReasonDifferentMeaning, Gloss: "another sense", SentenceTranslation: "Another sense: " + request.ExampleSentence}, nil
		}
		return enrichment.TranslationResponse{FluencyReason: enrichment.FluencyReasonSameMeaning, Gloss: request.LearnedMeaning, SentenceTranslation: "Fluency: " + request.ExampleSentence, SentenceTranslationTargets: []string{request.CanonicalLemma}}, nil
	}
	if request.RequireEvidenceAssistedPass {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.pass = append(p.pass, request)
		if len(request.CandidateSenses) == 0 {
			return enrichment.TranslationResponse{}, errors.New("the pass sent no candidate senses")
		}
		cited := request.CandidateSenses[0].EvidenceID
		if p.citeLast {
			cited = request.CandidateSenses[len(request.CandidateSenses)-1].EvidenceID
		}
		return enrichment.TranslationResponse{
			Translation: "word", Gloss: "word (gloss)", EvidenceIDs: []string{cited},
			SentenceTranslation: "He sees the word.", SentenceTranslationTargets: []string{"word"},
		}, nil
	}
	p.mu.Lock()
	failing := p.failures[request.CanonicalLemma] > 0
	if failing {
		p.failures[request.CanonicalLemma]--
	}
	omitted := p.omitted[request.CanonicalLemma]
	p.mu.Unlock()
	if failing {
		return enrichment.TranslationResponse{}, errors.New("provider unavailable")
	}
	response := enrichment.TranslationResponse{
		ContextOnly: true, SentenceTranslation: "Anna is not with the " + request.CanonicalLemma + ".", SentenceTranslationTargets: []string{request.CanonicalLemma},
		Translation: request.CanonicalLemma, Gloss: "the " + request.CanonicalLemma + " (gloss)",
	}
	if request.CanonicalLemma == "hund" || omitted {
		response.Gloss, response.Translation = "", ""
		response.UnresolvedKind, response.UnresolvedReason = enrichment.ContextualGlossUnresolvedAmbiguous, "no sense fits"
	}
	return response, nil
}

type studyHarness struct {
	t        *testing.T
	ctx      context.Context
	store    *persistence.PostgresStore
	h        *Handler
	cookies  []*http.Cookie
	owner    domain.User
	provider *studyProvider
	client   *river.Client[pgx.Tx]
	clock    *testutil.ManualClock
	// planner is the Study service the planning job runs, so a test can run a
	// plan at a chosen point instead of waiting on River.
	planner *study.Service
}

func newStudyHarness(t *testing.T) *studyHarness {
	t.Helper()
	return newStudyHarnessWith(t, studyHarnessOptions{capabilities: readyGerman()})
}

// studyHarnessOptions say what a Study harness serves beyond German.
type studyHarnessOptions struct {
	capabilities staticCapabilities
	// evidence is a source-evidence language's dictionary, translations and pass
	// gate; nil for a modern-language harness.
	evidence func(store *persistence.PostgresStore) *study.SourceEvidence
	// dictionary is the lexicon the card reads its cited entries from.
	dictionary enrichment.LexicalProvider
	// dictionaryPin is the loaded dictionary's pin Study planning compares a
	// no-dictionary-entry omission with; nil when no dictionary is loaded.
	dictionaryPin func(language string) string
	// idioms is the Idiom avoidance the material and Fluency workers get; nil
	// is every language's idiom switch off. See germanIdiomAvoidance.
	idioms *study.IdiomAvoidance
}

func newStudyHarnessWith(t *testing.T, options studyHarnessOptions) *studyHarness {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "study-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	provider := &studyProvider{FakeTranslationProvider: testutil.NewFakeTranslationProvider(), failures: map[string]int{"tisch": 1}, disagree: map[string]bool{}}
	enricher := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, provider, nil)
	workers := river.NewWorkers()
	planWorker := study.AddPlanWorker(workers)
	var evidence *study.SourceEvidence
	if options.evidence != nil {
		evidence = options.evidence(store)
	}
	study.AddMaterialWorker(workers, store, enricher, evidence, options.idioms)
	study.AddFluencyWorker(workers, store, enricher, func(string) error { return nil }, nil, nil, options.idioms)
	client, err := river.NewClient[pgx.Tx](riverpgxv5.New(store.Pool()), &river.Config{
		Queues: map[string]river.QueueConfig{study.Queue: {MaxWorkers: 2}}, Workers: workers,
		// A short retry keeps the provider-omission scenario fast.
		RetryPolicy: &river.DefaultClientRetryPolicy{},
	})
	require.NoError(t, err)

	clock := testutil.NewManualClock(time.Date(2026, 11, 25, 4, 0, 0, 0, time.UTC))
	studyService := study.NewService(store, client).WithDictionaryPin(options.dictionaryPin).WithClock(clock.Func())
	planWorker.Planner = studyService
	webAuth := auth.New(store, time.Hour)
	h := New(Services{
		Auth: webAuth, WebAuth: webauth.New(webAuth, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), nil), Capabilities: options.capabilities, Dictionary: options.dictionary,
		Study: studyService, StudyScheduler: study.NewScheduler(), Clock: clock.Func(), SessionLifetime: time.Hour,
	})
	owner := createAccount(t, ctx, store, "study-learner", "study-password")
	cookies, _ := loginCookies(t, h, owner.Username, "study-password")
	return &studyHarness{t: t, ctx: ctx, store: store, h: h, cookies: cookies, owner: owner, provider: provider, client: client, clock: clock, planner: studyService}
}

// seedBook gives the learner an analyzed German Book whose analysis holds each
// word in the given number of gate-passing sentences, or in fragments the
// sentence-quality gate rejects.
func (s *studyHarness) seedBook(key string, words ...testutil.CorpusWord) domain.Book {
	s.t.Helper()
	return s.seedBookIn("de", key, words...)
}

// seedBookIn is seedBook for a Book in any language.
func (s *studyHarness) seedBookIn(language, key string, words ...testutil.CorpusWord) domain.Book {
	s.t.Helper()
	lemmas := make([]domain.LemmaOccurrence, 0, len(words))
	for _, word := range words {
		lemmas = append(lemmas, domain.LemmaOccurrence{Language: language, CanonicalLemma: word.Lemma, UPOS: word.UPOS, OccurrenceCount: int64(word.Occurrences)})
	}
	book, source, corpus, _ := seedAnalyzedBookIn(s.t, s.ctx, s.store, language, s.owner.ID, key, "Study "+key, lemmas)
	var runID string
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, s.owner.ID, book.ID).Scan(&runID))
	// The sentences sit in the Book's extracted unit, so the concordance and the
	// Browse counts can place them.
	unitID := domain.EPUBUnitID(0, key)
	var ordinal, offset int64
	for _, word := range words {
		sentence := testutil.CorpusSentence(language, word)
		for range word.Occurrences {
			ordinal, offset = testutil.SeedCorpusSentence(s.t, s.ctx, s.store.Pool(), s.owner.ID, corpus.ID, language, runID, unitID, ordinal, offset, sentence)
		}
	}
	tx, err := s.store.Pool().Begin(s.ctx)
	require.NoError(s.t, err)
	// Rolled back when an assertion aborts before the commit; a no-op after it.
	testutil.Cleanup(s.t, "transaction", func() error { return txcleanup.Rollback(s.ctx, tx) })
	require.NoError(s.t, persistence.BuildVocabularyBrowseCountsTx(s.ctx, tx, s.owner.ID, book.ID, source.ID, runID, corpus.ID, language))
	require.NoError(s.t, tx.Commit(s.ctx))
	require.NoError(s.t, s.store.SetBookDisposition(s.ctx, s.owner.ID, book.ID, domain.BookDispositionToRead))
	return book
}

func (s *studyHarness) lemmasByReservation() []string {
	s.t.Helper()
	rows, err := s.store.Pool().Query(s.ctx, `SELECT canonical_lemma FROM study_material WHERE owner_id=$1 ORDER BY created_at, id`, s.owner.ID)
	require.NoError(s.t, err)
	defer rows.Close()
	var lemmas []string
	for rows.Next() {
		var lemma string
		require.NoError(s.t, rows.Scan(&lemma))
		lemmas = append(lemmas, lemma)
	}
	require.NoError(s.t, rows.Err())
	return lemmas
}

// planJobs counts the Study planning jobs River has not finished.
func (s *studyHarness) planJobs() int {
	s.t.Helper()
	var n int
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT count(*) FROM river_job WHERE kind='study_plan_preparation' AND state IN ('available','pending','retryable','scheduled','running')`).Scan(&n))
	return n
}

// settled reports that no planning job is waiting and no word is preparing.
func (s *studyHarness) settled() (bool, string) {
	if s.planJobs() > 0 {
		return false, "planning jobs remain"
	}
	return s.counts().Preparing == 0, "preparing words remain"
}

func (s *studyHarness) counts() domain.StudyMaterialCounts {
	s.t.Helper()
	counts, err := s.store.CountStudyMaterial(s.ctx, s.owner.ID, "de")
	require.NoError(s.t, err)
	return counts
}

func TestStudyPreparesMaterialAheadInStreamOrder(t *testing.T) {
	s := newStudyHarness(t)
	reading := s.seedBook("reading", testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2}, testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3})
	s.seedBook("library",
		testutil.CorpusWord{Lemma: "hund", UPOS: "NOUN", Occurrences: 6},
		testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 2},
		testutil.CorpusWord{Lemma: "schule", UPOS: "NOUN", Occurrences: 4},
		testutil.CorpusWord{Lemma: "kind", UPOS: "NOUN", Occurrences: 3, Fragment: true},
		testutil.CorpusWord{Lemma: "tisch", UPOS: "NOUN", Occurrences: 1})
	_, err := s.store.Pool().Exec(s.ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,'de',$2)`, s.owner.ID, reading.ID)
	require.NoError(t, err)
	// A Known word is still prepared and offered: Study never consults Known.
	_, err = s.store.Pool().Exec(s.ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos,lexical_discriminator) VALUES($1,'de','haus','NOUN','')`, s.owner.ID)
	require.NoError(t, err)

	// Opening the page queues one planning job and never calls a provider or
	// plans: no River client is running yet, so any planning or provider call
	// here would be the request making it.
	page := getAs(t, s.h, "/de/study", s.cookies)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "0 new words are ready.")
	t.Run("a page request never calls a provider", func(t *testing.T) { require.Zero(t, s.provider.Calls()) })
	t.Run("a page request reserves no material; the planning job does", func(t *testing.T) { require.Empty(t, s.lemmasByReservation()) })
	t.Run("a page request queues one planning job", func(t *testing.T) { require.Equal(t, 1, s.planJobs()) })

	// Opening it again while the job is pending queues no duplicate.
	getAs(t, s.h, "/de/study", s.cookies)
	assert.Equal(t, 1, s.planJobs())

	require.NoError(t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(t, "study River client", s.client.Stop)
	testutil.Eventually(t, 60*time.Second, "every word to settle", func() (bool, string) {
		return s.settled()
	})
	assert.Equal(t, []string{"haus", "buch", "hund", "schule", "kind", "tisch"}, s.lemmasByReservation(),
		"Current-reading words come first, then the library by Personal analyzed-Book occurrences")

	assert.Equal(t, domain.StudyMaterialCounts{Ready: 4, Unpreparable: 2}, s.counts(),
		"the Known word, the Current reading's words and the retried word are ready; the other two are skipped")
	reasons := map[string]string{}
	material, err := s.store.ListCurrentStudyMaterial(s.ctx, s.owner.ID, "de")
	require.NoError(t, err)
	for _, item := range material {
		reasons[item.CanonicalLemma] = item.OmissionReason
		if item.State == domain.StudyMaterialReady {
			assert.NotEmpty(t, item.Sentence)
			assert.NotEmpty(t, item.Gloss)
			assert.NotEmpty(t, item.SentenceTranslation)
		}
	}
	assert.Equal(t, domain.StudyOmissionMeaning, reasons["hund"], "a meaning omission is recorded and skipped")
	assert.Equal(t, domain.StudyOmissionSentenceQuality, reasons["kind"], "a sentence-quality omission is recorded and skipped")
	assert.Empty(t, reasons["tisch"], "a provider omission is retried, never recorded as an omission")
	assert.GreaterOrEqual(t, s.provider.Calls(), 5)

	page = getAs(t, s.h, "/de/study", s.cookies)
	assert.Contains(t, page.Body.String(), "4 new words are ready.")
	assert.Contains(t, page.Body.String(), "2 words were skipped")
}

// A language whose Study switch is off gets the plain page and no preparation.
// Every language is on in production, so Latin's switch is forced off here.
func TestStudyIsNotAvailableForALanguageWithItsSwitchOff(t *testing.T) {
	t.Cleanup(cardpolicy.OverrideStudyForTest("la", false))
	s := newStudyHarness(t)
	s.seedBook("german", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3})
	for _, language := range []string{"la"} {
		book, err := s.store.CreateBook(s.ctx, domain.Book{OwnerID: s.owner.ID, Title: "Book in " + language, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: language})
		require.NoError(t, err)
		require.NoError(t, s.store.SetBookDisposition(s.ctx, s.owner.ID, book.ID, domain.BookDispositionToRead))
	}

	for _, language := range []string{"la"} {
		page := getAs(t, s.h, "/"+language+"/study", s.cookies)
		require.Equal(t, http.StatusOK, page.Code, language)
		assert.Contains(t, page.Body.String(), "Study isn't available for this language yet", language)
	}
	assert.Empty(t, s.lemmasByReservation(), "no preparation is enqueued for a language without Study")
	assert.Zero(t, s.planJobs(), "no planning job is queued for a language without Study")
	assert.Zero(t, s.provider.Calls())
}

// prepared seeds a Book with two words, opens the page so their material is
// enqueued and waits until both are ready.
func (s *studyHarness) prepared() {
	s.t.Helper()
	s.seedBook("reading", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3}, testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2})
	require.Equal(s.t, http.StatusOK, getAs(s.t, s.h, "/de/study", s.cookies).Code)
	require.NoError(s.t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(s.t, "study River client", s.client.Stop)
	testutil.Eventually(s.t, 60*time.Second, "material to settle", func() (bool, string) {
		return s.settled()
	})
}

func (s *studyHarness) post(path string, form url.Values) *httptest.ResponseRecorder {
	s.t.Helper()
	form.Set("csrf_token", s.cookies[0].Value)
	return perform(s.t, s.h, http.MethodPost, path, form, s.cookies)
}

func (s *studyHarness) progress() []domain.StudyProgress {
	s.t.Helper()
	progress, err := s.store.ListStudyProgress(s.ctx, s.owner.ID, "de")
	require.NoError(s.t, err)
	return progress
}

func (s *studyHarness) reviewLog(progressID string) []domain.StudyReviewLog {
	s.t.Helper()
	logs, err := s.store.ListStudyReviewLog(s.ctx, s.owner.ID, progressID)
	require.NoError(s.t, err)
	return logs
}

func TestStudyDecliningABatchIntroducesNoWord(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()

	page := getAs(t, s.h, "/de/study", s.cookies).Body.String()
	assert.Contains(t, page, "new words are ready to learn.")
	assert.Contains(t, page, "Take these words")

	response := s.post("/study/batches/decline", url.Values{"language": {"de"}, "offer": {"1"}})
	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/de/study", response.Header().Get("Location"))
	assert.Empty(t, s.progress(), "declining introduces none")
	page = getAs(t, s.h, "/de/study", s.cookies).Body.String()
	assert.Contains(t, page, "Nothing is mature yet", "declining the batch opens Fluency")
	assert.NotContains(t, page, "Not today", "Today offers no further decline")

	// The learner can change their mind inside Fluency while the cap allows.
	assert.Contains(t, page, "Take these words")
	s.post("/study/batches/take", url.Values{"language": {"de"}})
	assert.NotEmpty(t, s.progress(), "the declined batch was taken from inside Fluency")

	// A new Study day offers a batch again.
	s.clock.Advance(24 * time.Hour)
	assert.Contains(t, getAs(t, s.h, "/de/study", s.cookies).Body.String(), "Take these words")
}

func TestStudyTakingABatchIntroducesWordsAndGradesThemOnTheCard(t *testing.T) {
	s := newStudyHarness(t)
	s.prepared()
	taken := s.clock.Now()

	response := s.post("/study/batches/take", url.Values{"language": {"de"}})
	require.Equal(t, http.StatusSeeOther, response.Code)
	progress := s.progress()
	require.Len(t, progress, 2, "taking the batch introduces its words")
	for _, word := range progress {
		assert.Equal(t, "the "+word.CanonicalLemma+" (gloss)", word.LearnedMeaning, "the Gloss shown on introduction is the learned meaning")
		assert.True(t, taken.Equal(word.IntroducedAt))
		assert.Equal(t, domain.StudyStateNew, word.Memory.State)
	}
	// Taking again on the same day introduces nothing more.
	s.post("/study/batches/take", url.Values{"language": {"de"}})
	assert.Len(t, s.progress(), 2)

	// The card shows the sentence with the word marked, and the answer only
	// after Show answer.
	page := getAs(t, s.h, "/de/study", s.cookies).Body.String()
	assert.Contains(t, page, "Show answer")
	assert.Contains(t, page, `<mark class="study-card__target">`)
	assert.NotContains(t, page, "(gloss)", "the answer stays hidden until it is asked for")
	assert.NotContains(t, page, `value="got-it"`)
	page = getAs(t, s.h, "/de/study?answer=1", s.cookies).Body.String()
	assert.Contains(t, page, "(gloss)")
	assert.Contains(t, page, `id="study-translation"`)
	assert.Contains(t, page, `id="study-headword"`)
	assert.Contains(t, page, `value="got-it"`)
	assert.Contains(t, page, `value="again"`)
	assert.NotContains(t, page, `value="hard"`)

	first := s.currentCard()
	// Got it on first sight: due at the start of the next Study day.
	grade := func(card domain.StudyProgress, grade string) *httptest.ResponseRecorder {
		return s.post("/study/grades", url.Values{"language": {"de"}, "progress": {card.ID}, "reps": {strconv.FormatInt(card.Memory.Reps, 10)}, "grade": {grade}})
	}
	require.Equal(t, http.StatusSeeOther, grade(first, "got-it").Code)
	after := s.progressByID(first.ID)
	assert.Equal(t, domain.StudyStateReview, after.Memory.State)
	assert.True(t, domain.NextStudyDayStart(taken).Equal(after.Memory.Due), "Got it on first sight is due the next Study day: %v", after.Memory.Due)
	logs := s.reviewLog(first.ID)
	require.Len(t, logs, 1)
	assert.Equal(t, domain.StudyGradeGotIt, logs[0].Grade)
	assert.Equal(t, domain.StudyStateNew, logs[0].Before.State)
	assert.Equal(t, domain.StudyStateReview, logs[0].After.State)
	assert.Equal(t, domain.StudyDay(taken), logs[0].StudyDay)

	// A repeated tap on the same card grades it once.
	grade(first, "got-it")
	grade(first, "again")
	assert.Len(t, s.reviewLog(first.ID), 1)

	// Again on the other word repeats it the same day.
	second := s.currentCard()
	require.NotEqual(t, first.ID, second.ID)
	require.Equal(t, http.StatusSeeOther, grade(second, "again").Code)
	repeated := s.progressByID(second.ID)
	assert.Equal(t, domain.StudyStateLearning, repeated.Memory.State)
	assert.True(t, repeated.Memory.Due.After(taken))
	assert.True(t, repeated.Memory.Due.Before(domain.NextStudyDayStart(taken)), "Again repeats the word the same Study day")
	page = getAs(t, s.h, "/de/study", s.cookies).Body.String()
	assert.Contains(t, page, "1 word comes back later today.")
	assert.NotContains(t, page, "Show answer")

	s.clock.Advance(5 * time.Minute)
	assert.Equal(t, second.ID, s.currentCard().ID, "the word comes back once its repeat is due")
	require.Equal(t, http.StatusSeeOther, grade(s.currentCard(), "got-it").Code)
	assert.Len(t, s.reviewLog(second.ID), 2, "every grade writes a review-log row")
	assert.True(t, domain.NextStudyDayStart(taken).Equal(s.progressByID(second.ID).Memory.Due))
	assert.Contains(t, getAs(t, s.h, "/de/study", s.cookies).Body.String(), "Nothing to study right now.")
}

func (s *studyHarness) progressByID(id string) domain.StudyProgress {
	s.t.Helper()
	for _, item := range s.progress() {
		if item.ID == id {
			return item
		}
	}
	s.t.Fatalf("no study progress %s", id)
	return domain.StudyProgress{}
}

// currentCard is the word the Study page would show now.
func (s *studyHarness) currentCard() domain.StudyProgress {
	s.t.Helper()
	card, err := s.store.NextStudyCard(s.ctx, s.owner.ID, "de", s.clock.Now())
	require.NoError(s.t, err)
	return card.Progress
}

func TestStudyActionsAreProtected(t *testing.T) {
	t.Cleanup(cardpolicy.OverrideStudyForTest("la", false))
	s := newStudyHarness(t)
	s.prepared()

	forged := perform(t, s.h, http.MethodPost, "/study/batches/take", url.Values{"csrf_token": {"forged"}, "language": {"de"}}, s.cookies)
	assert.Equal(t, http.StatusForbidden, forged.Code)
	assert.Empty(t, s.progress())
	anonymous := perform(t, s.h, http.MethodPost, "/study/batches/take", url.Values{"language": {"de"}}, nil)
	assert.NotEqual(t, http.StatusOK, anonymous.Code, "a signed-out request is refused")
	assert.Empty(t, s.progress())
	assert.Equal(t, http.StatusNotFound, s.post("/study/batches/take", url.Values{"language": {"la"}}).Code, "Study is not available for a language with its switch off")
	assert.Equal(t, http.StatusBadRequest, s.post("/study/grades", url.Values{"language": {"de"}, "progress": {"x"}, "reps": {"0"}, "grade": {"hard"}}).Code, "Study has two grades")
	assert.Empty(t, s.progress())
}

// A Browse-count rebuild (publication is covered in internal/analysis) prepares Study material with no Study
// page ever opened, so the first open finds the first words ready.
func TestStudyMaterialIsReadyAfterBrowseCountRebuildWithoutOpeningStudy(t *testing.T) {
	s := newStudyHarness(t)
	book := s.seedBook("published", testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2}, testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3})
	require.Zero(t, s.planJobs(), "seeding the counts directly queues nothing")

	// The Browse-count rebuild publishes the counts and queues the planning.
	require.NoError(t, (&analysis.BrowseCountsRebuildWorker{Pool: s.store.Pool()}).Work(s.ctx, &river.Job[analysis.BrowseCountsRebuildArgs]{Args: analysis.BrowseCountsRebuildArgs{
		OwnerID: s.owner.ID, BookID: book.ID, RunID: s.currentRun(book.ID)}}))
	require.Equal(t, 1, s.planJobs())

	require.NoError(t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(t, "study River client", s.client.Stop)
	testutil.Eventually(t, 60*time.Second, "the words to be prepared", func() (bool, string) {
		return s.settled()
	})
	assert.Equal(t, 2, s.counts().Ready)
	assert.Positive(t, s.provider.Calls())
}

func TestStudyPlanningIsNotQueuedForUnavailableLanguages(t *testing.T) {
	t.Cleanup(cardpolicy.OverrideStudyForTest("la", false))
	s := newStudyHarness(t)
	for _, language := range []string{"la", ""} {
		tx, err := s.store.Pool().Begin(s.ctx)
		require.NoError(t, err)
		require.NoError(t, study.EnqueuePlanningTx(s.ctx, tx, s.owner.ID, language))
		require.NoError(t, tx.Commit(s.ctx))
	}
	assert.Zero(t, s.planJobs())
	tx, err := s.store.Pool().Begin(s.ctx)
	require.NoError(t, err)
	require.NoError(t, study.EnqueuePlanningTx(s.ctx, tx, s.owner.ID, "de"))
	require.NoError(t, tx.Rollback(s.ctx))
	assert.Zero(t, s.planJobs(), "a rolled-back publication queues nothing")
}

func (s *studyHarness) currentRun(bookID string) string {
	s.t.Helper()
	var runID string
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, s.owner.ID, bookID).Scan(&runID))
	return runID
}

// A word omitted for having no dictionary entry is prepared again only after the
// dictionary's pin changes; every other omission stays skipped (#2775).
func TestStudyRetriesANoDictionaryEntryWordOnlyWhenTheDictionaryPinChanges(t *testing.T) {
	var mu sync.Mutex
	pin := "pin-1"
	setPin := func(next string) { mu.Lock(); pin = next; mu.Unlock() }
	s := newStudyHarnessWith(t, studyHarnessOptions{capabilities: readyGerman(), dictionaryPin: func(string) string {
		mu.Lock()
		defer mu.Unlock()
		return pin
	}})
	s.seedBook("library", testutil.CorpusWord{Lemma: "buch", UPOS: "NOUN", Occurrences: 2}, testutil.CorpusWord{Lemma: "schule", UPOS: "NOUN", Occurrences: 2})
	omit := func(lemma string, record func(id string) error) string {
		id, created, err := s.store.ReserveStudyMaterial(s.ctx, s.owner.ID, domain.StudyIdentity{Language: "de", CanonicalLemma: lemma, UPOS: "NOUN"})
		require.NoError(t, err)
		require.True(t, created)
		require.NoError(t, record(id))
		return id
	}
	noEntry := omit("buch", func(id string) error {
		return s.store.RecordStudyMaterialNoDictionaryEntry(s.ctx, s.owner.ID, id, "pin-1")
	})
	meaning := omit("schule", func(id string) error {
		return s.store.RecordStudyMaterialUnpreparable(s.ctx, s.owner.ID, id, domain.StudyOmissionMeaning)
	})
	require.NoError(t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(t, "study River client", s.client.Stop)
	settle := func() {
		testutil.Eventually(t, 60*time.Second, "planning to settle", func() (bool, string) { return s.settled() })
	}
	state := func(id string) domain.StudyMaterial {
		item, err := s.store.GetStudyMaterial(s.ctx, s.owner.ID, id)
		require.NoError(t, err)
		return item
	}

	getAs(t, s.h, "/de/study", s.cookies)
	settle()
	assert.Equal(t, domain.StudyMaterialUnpreparable, state(noEntry).State, "with the pin unchanged the word is not retried")
	assert.Equal(t, "pin-1", state(noEntry).OmissionDictionaryPin)
	assert.Zero(t, s.provider.Calls(), "and no provider call is made")

	setPin("pin-2")
	getAs(t, s.h, "/de/study", s.cookies)
	settle()
	retried := state(noEntry)
	assert.Equal(t, noEntry, retried.ID, "the same record is prepared again")
	assert.Equal(t, domain.StudyMaterialReady, retried.State, "after the pin changes the word is prepared against the new dictionary")
	assert.Empty(t, retried.OmissionDictionaryPin)
	assert.Equal(t, domain.StudyMaterialUnpreparable, state(meaning).State, "a meaning omission is never retried")
	assert.Equal(t, domain.StudyOmissionMeaning, state(meaning).OmissionReason)
	assert.Equal(t, 1, s.provider.Calls(), "only the retried word asked the provider")
}
