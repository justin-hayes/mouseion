//go:build integration

package webapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
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
// Ancient Greek with no provider configured: a Reading snapshot of the
// imported Book 1 is prepared from its representative sentences alone and
// exported. No provider request is made, lowercase-initial Greek sentences
// ending in the source terminators are accepted, the δέω homographs stay
// separate cards, the source notices are exported without any Wiktionary
// meaning evidence, and preparation never marks vocabulary Known.
func TestAncientGreekBookOneDeckPreparationAndExport(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "ancient-greek-deck-integration-secret-0123456789")
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

	provider := &greekDeckRecordingProvider{}
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

	// A Greek deck makes no provider call and produces no meaning or provider
	// omissions; only the sentence-quality gate can leave an item out.
	assert.Zero(t, provider.calls, "no provider request is made for a Greek deck")
	status, err := store.GetDeckPreparationStatus(ctx, learner.ID, preparation.ID)
	require.NoError(t, err)
	requireGreekDeckOmissionKinds(t, status, 0)

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

	// The frozen deck carries no Wiktionary meaning evidence, but each matched
	// identity freezes its LSJ entry: λόγος resolves one entry, while the two
	// δέω homographs are ambiguous and freeze no entry rather than borrowing one.
	byIdentity := make(map[string]greekDeckManifestItem, len(items))
	for _, item := range items {
		byIdentity[item.lemma+"|"+item.discriminator] = item
	}
	lsjEntryID := make(map[string]string, len(byIdentity))
	lsjPin := make(map[string]string, len(byIdentity))
	gloss := make(map[string]string, len(byIdentity))
	senses := make(map[string]int, len(byIdentity))
	for identity, item := range byIdentity {
		lsjEntryID[identity], lsjPin[identity] = item.lsjEntryID, item.lsjPin
		gloss[identity], senses[identity] = item.gloss, item.senses
	}
	assert.Equal(t, map[string]string{"δέω|morpheus:1": "", "δέω|morpheus:2": "", "λόγοσ|": "lsj-logos"}, lsjEntryID, "only the unambiguous identity freezes an LSJ entry")
	assert.Equal(t, map[string]string{"δέω|morpheus:1": "", "δέω|morpheus:2": "", "λόγοσ|": greekDeckLSJPin}, lsjPin, "the matched identity freezes the LSJ pin")
	assert.Equal(t, map[string]string{"δέω|morpheus:1": "", "δέω|morpheus:2": "", "λόγοσ|": ""}, gloss, "a Greek deck resolves no local Wiktionary gloss")
	assert.Equal(t, map[string]int{"δέω|morpheus:1": 0, "δέω|morpheus:2": 0, "λόγοσ|": 0}, senses, "a Greek deck freezes no Wiktionary candidate senses")

	logos := byIdentity["λόγοσ|"].dictionaryEntry
	assert.Contains(t, logos, `<span class="lsj-gloss">word</span>`)
	assert.Contains(t, logos, `<span class="lsj-gloss">account</span>`)
	assert.Less(t, strings.Index(logos, "word"), strings.Index(logos, "account"), "senses keep source order")
	assert.Contains(t, logos, `<span class="lsj-citation">X. An. 1.1.1</span>`)
	assert.Empty(t, byIdentity["δέω|morpheus:1"].dictionaryEntry, "an ambiguous homograph freezes no LSJ entry")

	// Generated and deck-preparation vocabulary carry the discriminated
	// identities and the frozen headword.
	generated := queryRows(t, ctx, pool, `SELECT canonical_lemma, upos, lexical_discriminator, headword FROM generated_vocabulary WHERE owner_id=$1 ORDER BY canonical_lemma, lexical_discriminator`, learner.ID)
	assert.Len(t, generated, 3)
	assert.Contains(t, generated, [4]string{"δέω", "VERB", "morpheus:1", "δέω"})
	assert.Contains(t, generated, [4]string{"δέω", "VERB", "morpheus:2", "δέω"})
	assert.Contains(t, generated, [4]string{"λόγοσ", "NOUN", "", "λόγος"})
	prepared := queryRows(t, ctx, pool, `SELECT canonical_lemma, upos, lexical_discriminator, headword FROM deck_preparation_vocabulary WHERE owner_id=$1 ORDER BY canonical_lemma, lexical_discriminator`, learner.ID)
	assert.Equal(t, generated, prepared)

	// The exported APKG carries the OGA, Perseus/canonical-greekLit, oga_src and
	// LSJ notices, and no Wiktionary notice because no meaning evidence was used.
	for _, notice := range []string{"OGA", "Perseus/canonical-greekLit", "oga_src", "LSJ", "PerseusDL/lexica@" + greekDeckLSJPin} {
		assert.True(t, bytes.Contains(preparation.Artifact, []byte(notice)), "exported deck is missing notice %q", notice)
	}
	assert.False(t, bytes.Contains(preparation.Artifact, []byte("Wiktionary")), "a providerless Greek deck carries no Wiktionary meaning evidence")

	// Re-deriving the dictionary index does not change an already-prepared deck:
	// the frozen projection re-renders byte-identically without consulting any
	// index, because the LSJ entry id, pin and rendered entry are frozen.
	projection, digest, err := store.LoadPreparedDeckStorageProjection(ctx, learner.ID, preparation.ID, preparation.CurrentRunID)
	require.NoError(t, err)
	require.NotEmpty(t, digest)
	restored, err := cardexport.NewPresentation(nil).Restore(projection)
	require.NoError(t, err)
	rerendered, _, err := cardexport.NewPresentation(nil).Finalize(ctx, restored, nil, cardexport.RunFacts{})
	require.NoError(t, err)
	assert.True(t, bytes.Equal(preparation.Artifact, rerendered.APKG), "a prepared deck re-renders unchanged without the index")

	// Card preparation never writes Known.
	var known int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='grc'`, learner.ID).Scan(&known))
	assert.Zero(t, known, "preparing a deck never marks vocabulary Known")
}

// TestAncientGreekDeckSentenceQualityOmissionWithoutProvider proves a Greek deck
// item whose sentences all fail the gate is a sentence-quality omission and that
// no other omission kind is produced for grc.
func TestAncientGreekDeckSentenceQualityOmissionWithoutProvider(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "ancient-greek-deck-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	learner := createAccount(t, ctx, store, "greek-omission-learner", "learner-password")
	registration, err := ancientgreek.NewService(store).Register(ctx, learner.Username, ancientgreek.AnabasisBook1)
	require.NoError(t, err)
	importGreekFixtureWithDefinition(t, ctx, store, learner.ID, registration.BookID, "oga-src-deck-omission.conllu", ancientgreek.AnabasisBook1)
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, registration.BookID, domain.BookDispositionToRead))
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))

	provider := &greekDeckRecordingProvider{}
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
	require.Equal(t, 2, reading.SnapshotSize, "the recurring δέω and λόγος identities are frozen")

	submitted := perform(t, h, http.MethodPost, "/reading/books/"+registration.BookID+"/deck/retry", url.Values{
		"csrf_token": {csrf}, "expected_current_snapshot_id": {reading.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, submitted.Code, "prepare body=%s", submitted.Body.String())
	preparation := waitForPreparedDeck(t, ctx, store, learner.ID, reading.SnapshotID)
	require.Equal(t, domain.DeckPreparationReady, preparation.State, "preparation error=%q", preparation.Error)
	assert.Equal(t, 1, preparation.TotalCards, "λόγος is the only accepted card")

	assert.Zero(t, provider.calls, "no provider request is made for a Greek deck")
	status, err := store.GetDeckPreparationStatus(ctx, learner.ID, preparation.ID)
	require.NoError(t, err)
	requireGreekDeckOmissionKinds(t, status, 1)
}

// requireGreekDeckOmissionKinds asserts a Greek deck produces only
// sentence-quality omissions: no meaning and no provider omissions.
func requireGreekDeckOmissionKinds(t *testing.T, status domain.DeckPreparation, wantQuality int) {
	t.Helper()
	require.Equal(t, wantQuality, status.QualityOmissions, "a Greek item whose sentences all fail the gate is a sentence-quality omission")
	require.Zero(t, status.ProviderOmissions, "a Greek deck produces no provider omissions")
	require.Empty(t, status.MeaningOmissions, "a Greek deck produces no meaning omissions")
}

// greekDeckRecordingProvider is a configured provider that records every call.
// A providerless Greek deck must never reach it.
type greekDeckRecordingProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *greekDeckRecordingProvider) Name() string    { return "openai-compatible" }
func (p *greekDeckRecordingProvider) Version() string { return "greek-deck" }
func (p *greekDeckRecordingProvider) Enabled() bool   { return true }

func (p *greekDeckRecordingProvider) Translate(context.Context, enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return enrichment.TranslationResponse{}, errors.New("a providerless Greek deck must make no provider request")
}

// greekDeckLexicon is the fake local dictionary index. A providerless Greek deck
// must not consult it for Wiktionary meaning evidence, so any resolved meaning
// would show up as a Wiktionary notice in the export. It answers the LSJ
// capability with exactly-one-entry-per-headword semantics: λόγος has a single
// entry, while the two δέω entries make that headword ambiguous.
type greekDeckLexicon struct{}

const greekDeckLSJPin = "56061ca127f4a2844980baffc5f2b6d1332897b3"

func (greekDeckLexicon) Name() string    { return "greek-deck-dictionary" }
func (greekDeckLexicon) Version() string { return "kaikki-grc-test" }

func (greekDeckLexicon) Lookup(_ context.Context, request enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	senses := []enrichment.LexicalSense{{EvidenceID: "wikt:logos", Source: "wiktionary", Gloss: "word, speech", MatchStrength: "exact"}}
	switch request.CanonicalLemma {
	case "δέω":
		senses = []enrichment.LexicalSense{
			{EvidenceID: "wikt:deo-bind", Source: "wiktionary", Gloss: "bind, tie", MatchStrength: "exact"},
			{EvidenceID: "wikt:deo-lack", Source: "wiktionary", Gloss: "lack, need", MatchStrength: "lemma_only_missing_pos"},
		}
	}
	return enrichment.LexicalEntry{Senses: senses, CandidateSenses: senses}, true, nil
}

func (greekDeckLexicon) LookupAncientGreekLSJ(_ context.Context, lemma string) (enrichment.AncientGreekLSJEntry, bool) {
	entries := map[string][]enrichment.AncientGreekLSJEntry{
		"λόγοσ": {{
			EntryID: "lsj-logos", Headword: "λόγοσ", Pin: greekDeckLSJPin,
			Senses: []enrichment.AncientGreekLSJSense{
				{ID: "lsj-logos.0", Number: "A", Level: 1, Glosses: []string{"word", "speech"}, Citations: []string{"X. An. 1.1.1"},
					Senses: []enrichment.AncientGreekLSJSense{{ID: "lsj-logos.0.1", Number: "2", Level: 2, Glosses: []string{"account"}}}},
			},
		}},
		"δέω": {
			{EntryID: "lsj-deo-bind", Headword: "δέω", Pin: greekDeckLSJPin, Senses: []enrichment.AncientGreekLSJSense{{ID: "lsj-deo-bind.0", Number: "A", Level: 1, Glosses: []string{"bind"}}}},
			{EntryID: "lsj-deo-lack", Headword: "δέω", Pin: greekDeckLSJPin, Senses: []enrichment.AncientGreekLSJSense{{ID: "lsj-deo-lack.0", Number: "A", Level: 1, Glosses: []string{"lack"}}}},
		},
	}
	candidates := entries[lemma]
	if len(candidates) != 1 {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	return candidates[0], true
}

// startGreekDeckWorkers wires the full durable prepared-deck pipeline with no
// configured provider. The standard-translation worker is registered with a
// recording spy so a leaked request fails the deck rather than the test.
func startGreekDeckWorkers(t *testing.T, ctx context.Context, store *persistence.PostgresStore, pool *pgxpool.Pool, provider enrichment.TranslationProvider) *river.Client[pgx.Tx] {
	t.Helper()
	workers := river.NewWorkers()
	presentation := cardexport.NewPresentation(greekDeckLexicon{})
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

	prepareddeck.AddPreparedDeckWorker(workers, store, presentation, client, nil, batchConfig, preparedConfig, false)
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
	gloss                                          string
	senses                                         int
	lsjEntryID, lsjPin, dictionaryEntry            string
}

func queryGreekManifestItems(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ownerID string) []greekDeckManifestItem {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT canonical_lemma, upos, lexical_discriminator, headword, source_sentence, render_payload, lsj_entry_id, lsj_pin FROM deck_preparation_manifest_items WHERE owner_id=$1 ORDER BY ordinal`, ownerID)
	require.NoError(t, err)
	defer rows.Close()
	var items []greekDeckManifestItem
	for rows.Next() {
		var item greekDeckManifestItem
		var payload []byte
		require.NoError(t, rows.Scan(&item.lemma, &item.upos, &item.discriminator, &item.headword, &item.sentence, &payload, &item.lsjEntryID, &item.lsjPin))
		var decoded struct {
			Gloss           string            `json:"gloss"`
			DictionaryEntry string            `json:"dictionary_entry"`
			CandidateSenses []json.RawMessage `json:"candidate_senses"`
		}
		require.NoError(t, json.Unmarshal(payload, &decoded))
		item.gloss, item.senses, item.dictionaryEntry = decoded.Gloss, len(decoded.CandidateSenses), decoded.DictionaryEntry
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
