//go:build integration

package webapp

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/study"
	"github.com/justin-hayes/mouseion/internal/testutil"
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
}

func (p *studyProvider) Translate(ctx context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	if _, err := p.FakeTranslationProvider.Translate(ctx, request); err != nil {
		return enrichment.TranslationResponse{}, err
	}
	p.mu.Lock()
	failing := p.failures[request.CanonicalLemma] > 0
	if failing {
		p.failures[request.CanonicalLemma]--
	}
	p.mu.Unlock()
	if failing {
		return enrichment.TranslationResponse{}, errors.New("provider unavailable")
	}
	response := enrichment.TranslationResponse{
		ContextOnly: true, SentenceTranslation: "Anna is not with the " + request.CanonicalLemma + ".", SentenceTranslationTargets: []string{request.CanonicalLemma},
		Translation: request.CanonicalLemma, Gloss: "the " + request.CanonicalLemma + " (gloss)",
	}
	if request.CanonicalLemma == "hund" {
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
}

func newStudyHarness(t *testing.T) *studyHarness {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "study-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	provider := &studyProvider{FakeTranslationProvider: testutil.NewFakeTranslationProvider(), failures: map[string]int{"tisch": 1}}
	enricher := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, provider, nil)
	workers := river.NewWorkers()
	study.AddMaterialWorker(workers, store, enricher)
	client, err := river.NewClient[pgx.Tx](riverpgxv5.New(store.Pool()), &river.Config{
		Queues: map[string]river.QueueConfig{study.Queue: {MaxWorkers: 2}}, Workers: workers,
		// A short retry keeps the provider-omission scenario fast.
		RetryPolicy: &river.DefaultClientRetryPolicy{},
	})
	require.NoError(t, err)

	clock := testutil.NewManualClock(time.Date(2026, 11, 25, 4, 0, 0, 0, time.UTC))
	webAuth := auth.New(store, time.Hour)
	h := New(Services{
		Auth: webAuth, WebAuth: webauth.New(webAuth, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), nil), Capabilities: readyGerman(),
		Study: study.NewService(store, client), Clock: clock.Func(), SessionLifetime: time.Hour,
	})
	owner := createAccount(t, ctx, store, "study-learner", "study-password")
	cookies, _ := loginCookies(t, h, owner.Username, "study-password")
	return &studyHarness{t: t, ctx: ctx, store: store, h: h, cookies: cookies, owner: owner, provider: provider, client: client, clock: clock}
}

// seedBook gives the learner an analyzed German Book whose analysis holds each
// word in the given number of gate-passing sentences, or in fragments the
// sentence-quality gate rejects.
func (s *studyHarness) seedBook(key string, words ...testutil.CorpusWord) domain.Book {
	s.t.Helper()
	lemmas := make([]domain.LemmaOccurrence, 0, len(words))
	for _, word := range words {
		lemmas = append(lemmas, domain.LemmaOccurrence{Language: "de", CanonicalLemma: word.Lemma, UPOS: word.UPOS, OccurrenceCount: int64(word.Occurrences)})
	}
	book, source, corpus, _ := seedMigrationAnalyzedBook(s.t, s.ctx, s.store, s.owner.ID, key, "Study "+key, lemmas)
	var runID string
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, s.owner.ID, book.ID).Scan(&runID))
	// The sentences sit in the Book's extracted unit, so the concordance and the
	// Browse counts can place them.
	unitID := domain.EPUBUnitID(0, key)
	var ordinal, offset int64
	for _, word := range words {
		sentence := testutil.CorpusSentence("de", word)
		for range word.Occurrences {
			ordinal, offset = testutil.SeedCorpusSentence(s.t, s.ctx, s.store.Pool(), s.owner.ID, corpus.ID, "de", runID, unitID, ordinal, offset, sentence)
		}
	}
	tx, err := s.store.Pool().Begin(s.ctx)
	require.NoError(s.t, err)
	require.NoError(s.t, persistence.BuildVocabularyBrowseCountsTx(s.ctx, tx, s.owner.ID, book.ID, source.ID, runID, corpus.ID, "de"))
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

	// Opening the page enqueues preparation and never calls a provider: no
	// River client is running yet, so a provider call here would be a request
	// making one.
	page := getAs(t, s.h, "/de/study", s.cookies)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "0 new words are ready.")
	assert.Contains(t, page.Body.String(), "6 more words are being prepared.")
	assert.Zero(t, s.provider.Calls(), "a page request never calls a provider")
	assert.Equal(t, []string{"haus", "buch", "hund", "schule", "kind", "tisch"}, s.lemmasByReservation(),
		"Current-reading words come first, then the library by Personal analyzed-Book occurrences")

	// Opening it again does not enqueue a second job for the same word.
	getAs(t, s.h, "/de/study", s.cookies)
	assert.Len(t, s.lemmasByReservation(), 6)

	require.NoError(t, s.client.Start(s.ctx))
	testutil.StopOnCleanup(t, "study River client", s.client.Stop)
	testutil.Eventually(t, 60*time.Second, "every word to settle", func() (bool, string) {
		counts := s.counts()
		return counts.Preparing == 0, "preparing words remain"
	})

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

func TestStudyIsNotAvailableForLatinAndAncientGreek(t *testing.T) {
	s := newStudyHarness(t)
	s.seedBook("german", testutil.CorpusWord{Lemma: "haus", UPOS: "NOUN", Occurrences: 3})
	for _, language := range []string{"la", "grc"} {
		book, err := s.store.CreateBook(s.ctx, domain.Book{OwnerID: s.owner.ID, Title: "Book in " + language, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: language})
		require.NoError(t, err)
		require.NoError(t, s.store.SetBookDisposition(s.ctx, s.owner.ID, book.ID, domain.BookDispositionToRead))
	}

	for _, language := range []string{"la", "grc"} {
		page := getAs(t, s.h, "/"+language+"/study", s.cookies)
		require.Equal(t, http.StatusOK, page.Code, language)
		assert.Contains(t, page.Body.String(), "Study isn't available for this language yet", language)
	}
	assert.Empty(t, s.lemmasByReservation(), "no preparation is enqueued for a language without Study")
	assert.Zero(t, s.provider.Calls())
}
