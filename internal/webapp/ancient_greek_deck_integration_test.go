//go:build integration

package webapp

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAncientGreekBookOneDeckPreparationAndExport extends the complete loop to
// Ancient Greek: a Reading snapshot of the imported Book 1 is prepared into a
// deck and exported with a fake provider. Lowercase-initial Greek sentences
// ending in the source terminators are accepted, the δέω homographs stay
// separate cards, the source notices are exported, and preparation never marks
// vocabulary Known.
func TestAncientGreekBookOneDeckPreparationAndExport(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "ancient-greek-deck-integration-secret-0123456789")
	recordAncientGreekAssessmentForTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	learner := createAccount(t, ctx, store, "greek-deck-learner", "learner-password")
	registration, err := ancientgreek.NewService(store).Register(ctx, learner.Username, ancientgreek.AnabasisBook1)
	require.NoError(t, err)
	importGreekFixtureWithDefinition(t, ctx, store, learner.ID, registration.BookID, "oga-src-deck.conllu", ancientgreek.AnabasisBook1)
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, registration.BookID, domain.BookDispositionToRead))
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))

	provider := &greekDeckTranslationProvider{}
	client := startGreekDeckWorkers(t, ctx, store, pool, provider)
	deckService := prepareddeck.NewService(store, client)
	webAuth := auth.New(store, time.Hour)
	h := New(Services{
		Auth: webAuth, WebAuth: webauth.New(webAuth, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), client), PreparedDeck: deckService,
		Capabilities: readyGerman(), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	started := perform(t, h, http.MethodPost, "/reading/books/"+registration.BookID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code, "start body=%s", started.Body.String())
	reading, err := store.GetCurrentReading(ctx, learner.ID, "grc")
	require.NoError(t, err)
	require.NotEmpty(t, reading.SnapshotID)
	require.Equal(t, 3, reading.SnapshotSize, "δέω 1, δέω 2 and λόγος are the recurring Book vocabulary")

	submitted := perform(t, h, http.MethodPost, "/reading/books/"+registration.BookID+"/deck/retry", url.Values{
		"csrf_token": {csrf}, "expected_current_snapshot_id": {reading.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, submitted.Code, "prepare body=%s", submitted.Body.String())
	preparation := waitForPreparedDeck(t, ctx, store, learner.ID, reading.SnapshotID)
	require.Equal(t, domain.DeckPreparationReady, preparation.State, "preparation error=%q", preparation.Error)
	assert.Equal(t, 3, preparation.TotalCards, "one card per frozen identity")

	// Homographs stay separate cards with separate deduplication keys.
	cards := queryGreekDeckCards(t, ctx, pool, learner.ID)
	require.Len(t, cards, 3)
	deckKeys := map[string]struct{}{}
	deoKeys := map[string]struct{}{}
	for _, card := range cards {
		deckKeys[card.lemma+"|"+card.upos+"|"+card.dedup] = struct{}{}
		if card.lemma == "δέω" {
			deoKeys[card.dedup] = struct{}{}
		}
	}
	assert.Len(t, deoKeys, 2, "δέω 1 and δέω 2 are separate cards: %+v", cards)
	assert.Len(t, deckKeys, 3)

	// The manifest keeps each imported identity's discriminator and citation
	// headword, and the accepted sentences use the Greek source terminators.
	items := queryGreekManifestItems(t, ctx, pool, learner.ID)
	require.Len(t, items, 3)
	deoDiscriminators := map[string]struct{}{}
	sentences := make([]string, 0, len(items))
	for _, item := range items {
		if item.lemma == "δέω" {
			deoDiscriminators[item.discriminator] = struct{}{}
		}
		assert.NotEmpty(t, item.headword, "a frozen imported item carries its citation headword")
		sentences = append(sentences, item.sentence)
	}
	assert.Equal(t, map[string]struct{}{"morpheus:1": {}, "morpheus:2": {}}, deoDiscriminators, "each δέω homograph is its own frozen item")
	assert.Contains(t, sentences, "ἄνθρωπος ὁρᾷ τὸν λόγον καὶ δέω βραδέως;", "a lowercase Greek sentence ending in ';' is accepted")
	assert.Contains(t, sentences, "ἀνὴρ ἀκούει τὸν λόγον μέγαν καὶ δέω νῦν·", "a lowercase Greek sentence ending in '·' is accepted")

	// Every matching dictionary entry reached the contextual Gloss step.
	provider.mu.Lock()
	senses := provider.sensesByLemma["δέω"]
	provider.mu.Unlock()
	assert.Equal(t, []string{"wikt:deo-bind", "wikt:deo-lack"}, senses, "both δέω entries are passed as meaning evidence")

	// Generated and deck-preparation vocabulary carry the discriminated
	// identities and the frozen headword.
	generated := queryRows(t, ctx, pool, `SELECT canonical_lemma, upos, lexical_discriminator, headword FROM generated_vocabulary WHERE owner_id=$1 ORDER BY canonical_lemma, lexical_discriminator`, learner.ID)
	assert.Len(t, generated, 3)
	assert.Contains(t, generated, [4]string{"δέω", "VERB", "morpheus:1", "δέω"})
	assert.Contains(t, generated, [4]string{"δέω", "VERB", "morpheus:2", "δέω"})
	assert.Contains(t, generated, [4]string{"λόγοσ", "NOUN", "", "λόγος"})
	prepared := queryRows(t, ctx, pool, `SELECT canonical_lemma, upos, lexical_discriminator, headword FROM deck_preparation_vocabulary WHERE owner_id=$1 ORDER BY canonical_lemma, lexical_discriminator`, learner.ID)
	assert.Equal(t, generated, prepared)

	// The exported APKG carries the OGA, Perseus/canonical-greekLit and oga_src
	// notices alongside the Wiktionary notice.
	for _, notice := range []string{"OGA", "Perseus/canonical-greekLit", "oga_src", "Wiktionary"} {
		assert.True(t, bytes.Contains(preparation.Artifact, []byte(notice)), "exported deck is missing notice %q", notice)
	}

	// Card preparation never writes Known.
	var known int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='grc'`, learner.ID).Scan(&known))
	assert.Zero(t, known, "preparing a deck never marks vocabulary Known")
}

// greekDeckTranslationProvider is the fake configured provider. It records the
// candidate senses each request carried and returns a valid contextual Gloss
// drawn from the frozen evidence.
type greekDeckTranslationProvider struct {
	mu            sync.Mutex
	sensesByLemma map[string][]string
}

func (p *greekDeckTranslationProvider) Name() string    { return "openai-compatible" }
func (p *greekDeckTranslationProvider) Version() string { return "greek-deck" }
func (p *greekDeckTranslationProvider) Enabled() bool   { return true }

func (p *greekDeckTranslationProvider) Translate(_ context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	p.mu.Lock()
	if p.sensesByLemma == nil {
		p.sensesByLemma = map[string][]string{}
	}
	ids := make([]string, 0, len(request.CandidateSenses))
	for _, sense := range request.CandidateSenses {
		ids = append(ids, sense.EvidenceID)
	}
	if len(ids) > 0 {
		p.sensesByLemma[request.CanonicalLemma] = ids
	}
	p.mu.Unlock()

	response := enrichment.TranslationResponse{Translation: "word", SentenceTranslation: "The man sees the word."}
	if !request.RequireContextualGloss {
		response.FallbackGloss = "word"
		return response, nil
	}
	if len(request.CandidateSenses) > 0 && request.CandidateSenses[0].EvidenceID != "" {
		response.Gloss = request.CandidateSenses[0].Gloss
		response.EvidenceIDs = []string{request.CandidateSenses[0].EvidenceID}
		return response, nil
	}
	response.Gloss = "word"
	response.ContextOnly = true
	return response, nil
}

// greekDeckLexicon is the fake local dictionary index. Every matching entry for
// an identity is returned, so the contextual Gloss step chooses from context.
type greekDeckLexicon struct{}

func (greekDeckLexicon) Name() string    { return "greek-deck-dictionary" }
func (greekDeckLexicon) Version() string { return "kaikki-grc-test" }

func (greekDeckLexicon) Lookup(_ context.Context, request enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	switch request.CanonicalLemma {
	case "δέω":
		senses := []enrichment.LexicalSense{
			{EvidenceID: "wikt:deo-bind", Source: "wiktionary", Gloss: "bind, tie", MatchStrength: "exact"},
			{EvidenceID: "wikt:deo-lack", Source: "wiktionary", Gloss: "lack, need", MatchStrength: "lemma_only_missing_pos"},
		}
		return enrichment.LexicalEntry{Senses: senses, CandidateSenses: senses}, true, nil
	case "λόγοσ":
		senses := []enrichment.LexicalSense{{EvidenceID: "wikt:logos", Source: "wiktionary", Gloss: "word, speech", MatchStrength: "exact"}}
		return enrichment.LexicalEntry{Senses: senses, CandidateSenses: senses}, true, nil
	default:
		return enrichment.LexicalEntry{}, false, nil
	}
}

// recordAncientGreekAssessmentForTest simulates Justin's #2125 human
// checkpoint: the short-handle contextual Gloss payload has been reviewed and
// recorded for personal use, so the provider-assessment gate admits it. The
// compiled assessment deliberately stays unrecorded (see
// enrichment.AncientGreekProviderAssessment) so production fails closed.
func recordAncientGreekAssessmentForTest(t *testing.T) {
	t.Helper()
	previous := enrichment.AncientGreekProviderAssessment
	enrichment.AncientGreekProviderAssessment.Recorded = true
	t.Cleanup(func() { enrichment.AncientGreekProviderAssessment = previous })
}

// startGreekDeckWorkers wires the full durable prepared-deck pipeline with the
// fake provider and starts the River client. The Greek import never runs an
// analysis job, but the client is created through the shared analysis helper so
// the queue configuration matches production.
func startGreekDeckWorkers(t *testing.T, ctx context.Context, store *persistence.PostgresStore, pool *pgxpool.Pool, provider enrichment.TranslationProvider) *river.Client[pgx.Tx] {
	t.Helper()
	workers := river.NewWorkers()
	presentation := cardexport.NewPresentation(greekDeckLexicon{})
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "greek-deck-model", BaseURL: "http://example.invalid/v1"})
	require.NoError(t, err)
	preparedConfig := prepareddeck.PreparedDeckConfig{
		TranslationMode:        prepareddeck.DefaultTranslationMode,
		StandardMaxConcurrency: 1,
		StandardMaxAttempts:    1,
		StandardRetryBaseDelay: time.Millisecond,
		StandardRetryMaxDelay:  time.Millisecond,
	}
	batchConfig := prepareddeck.BatchConfig{MaxRequests: 1, PollInterval: time.Millisecond}
	metrics := prepareddeck.NewMetricsCollector()

	unusedAnalyzer := &analyzertest.Fake{AnalyzeFunc: func(context.Context, analyzer.AnalyzeRequest) (analyzer.Result, error) {
		return analyzer.Result{}, errors.New("the imported Ancient Greek Book never calls the analyzer")
	}}
	client, err := analysis.NewClientWithPreparedDeckConcurrency(pool, unusedAnalyzer, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store), 1, workers)
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "analysis River client", client.Stop)

	prepareddeck.AddPreparedDeckWorker(workers, store, presentation, client, codec, batchConfig, preparedConfig, true)
	prepareddeck.AddStandardTranslationWorkerWithDependencies(workers, store, client, provider, preparedConfig, time.Second, metrics)
	prepareddeck.AddBatchSubmitWorkerWithMetrics(workers, store, client, nil, nil, metrics)
	prepareddeck.AddBatchPollWorker(workers, &prepareddeck.BatchPollWorker{Store: store, Client: client, PollInterval: batchConfig.PollInterval, Metrics: metrics})
	prepareddeck.AddFinalizeWorker(workers, &prepareddeck.DurableFinalizer{Store: store, Renderer: presentation, Metrics: metrics})
	prepareddeck.AddBatchCleanupWorker(workers, &prepareddeck.BatchCleanupWorker{Store: store, Metrics: metrics})
	prepareddeck.AddRecoveryWorkerWithMetrics(workers, store, client, batchConfig.PollInterval, metrics)
	require.NoError(t, prepareddeck.EnsureRecoveryJob(ctx, store, client))
	require.NoError(t, client.Start(ctx))
	return client
}

func waitForPreparedDeck(t *testing.T, ctx context.Context, store *persistence.PostgresStore, ownerID, snapshotID string) domain.DeckPreparation {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var preparation domain.DeckPreparation
	for {
		var err error
		preparation, err = store.GetDeckPreparationForSnapshot(ctx, ownerID, snapshotID)
		if err == nil && (preparation.State == domain.DeckPreparationReady || preparation.State == domain.DeckPreparationFailed) {
			return preparation
		}
		if time.Now().After(deadline) {
			t.Fatalf("prepared deck did not finish: state=%+v err=%v", preparation, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

type greekDeckCard struct {
	dedup, lemma, upos string
}

func queryGreekDeckCards(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ownerID string) []greekDeckCard {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT dedup_key, canonical_lemma, upos FROM cards WHERE owner_id=$1 ORDER BY dedup_key`, ownerID)
	require.NoError(t, err)
	defer rows.Close()
	var cards []greekDeckCard
	for rows.Next() {
		var card greekDeckCard
		require.NoError(t, rows.Scan(&card.dedup, &card.lemma, &card.upos))
		cards = append(cards, card)
	}
	require.NoError(t, rows.Err())
	return cards
}

type greekDeckManifestItem struct {
	lemma, upos, discriminator, headword, sentence string
}

func queryGreekManifestItems(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ownerID string) []greekDeckManifestItem {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT canonical_lemma, upos, lexical_discriminator, headword, source_sentence FROM deck_preparation_manifest_items WHERE owner_id=$1 ORDER BY ordinal`, ownerID)
	require.NoError(t, err)
	defer rows.Close()
	var items []greekDeckManifestItem
	for rows.Next() {
		var item greekDeckManifestItem
		require.NoError(t, rows.Scan(&item.lemma, &item.upos, &item.discriminator, &item.headword, &item.sentence))
		items = append(items, item)
	}
	require.NoError(t, rows.Err())
	return items
}

func queryRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) [][4]string {
	t.Helper()
	rows, err := pool.Query(ctx, query, args...)
	require.NoError(t, err)
	defer rows.Close()
	var out [][4]string
	for rows.Next() {
		var row [4]string
		require.NoError(t, rows.Scan(&row[0], &row[1], &row[2], &row[3]))
		out = append(out, row)
	}
	require.NoError(t, rows.Err())
	return out
}
