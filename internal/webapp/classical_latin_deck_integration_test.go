//go:build integration

package webapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	latinDeckUnitKey          = "synthetic-latin-history"
	latinDeckTranslationFile  = "phi0474.phi001.perseus-eng9.xml"
	latinDeckTranslationLabel = "Synthetic (1900), Fixture Hist."
	latinDeckTranslationPin   = "0123456789abcdef0123456789abcdef01234567"
)

// latinDeckConllu is a synthetic OLA CoNLL-U with domus recurring in three
// sentences, each ending in a full stop and starting in lowercase.
const latinDeckConllu = `# absolute_id = 0
# clause_id = c_1
# file = synthetic.xml
1	domus	domus	n	_	Case=n|Gender=f|Number=s	5	SBJ	_	t_1
2	magna	magnus	a	_	Case=n|Gender=f|Number=s	1	ATR	_	t_2
3	in	in	r	_	_	5	AuxP	_	t_3
4	urbe	urbs	n	_	Case=b|Gender=f|Number=s	3	ADV	_	t_4
5	stat	sto	v	_	Mood=i|Number=s|Person=3|Tense=p|Voice=a	0	PRED	_	t_5
6	et	et1	c	_	_	5	AuxY	_	t_6
7	dominus	dominus	n	_	Case=n|Gender=m|Number=s	9	SBJ	_	t_7
8	bene	bene	d	_	_	9	ADV	_	t_8
9	habitat	habito	v	_	Mood=i|Number=s|Person=3|Tense=p|Voice=a	5	PRED_CO	_	t_9
10	.	.	u	_	_	5	AuxK	_	t_10

# absolute_id = 1
# clause_id = c_2
# file = synthetic.xml
1	domus	domus	n	_	Case=n|Gender=f|Number=s	5	SBJ	_	t_11
2	parva	paruus	a	_	Case=n|Gender=f|Number=s	1	ATR	_	t_12
3	in	in	r	_	_	5	AuxP	_	t_13
4	agro	ager	n	_	Case=b|Gender=m|Number=s	3	ADV	_	t_14
5	stat	sto	v	_	Mood=i|Number=s|Person=3|Tense=p|Voice=a	0	PRED	_	t_15
6	et	et1	c	_	_	5	AuxY	_	t_16
7	seruus	seruus	n	_	Case=n|Gender=m|Number=s	9	SBJ	_	t_17
8	diu	diu	d	_	_	9	ADV	_	t_18
9	laborat	laboro	v	_	Mood=i|Number=s|Person=3|Tense=p|Voice=a	5	PRED_CO	_	t_19
10	.	.	u	_	_	5	AuxK	_	t_20

# absolute_id = 2
# clause_id = c_3
# file = synthetic.xml
1	domus	domus	n	_	Case=n|Gender=f|Number=s	5	SBJ	_	t_21
2	alta	altus	a	_	Case=n|Gender=f|Number=s	1	ATR	_	t_22
3	in	in	r	_	_	5	AuxP	_	t_23
4	colle	collis	n	_	Case=b|Gender=m|Number=s	3	ADV	_	t_24
5	stat	sto	v	_	Mood=i|Number=s|Person=3|Tense=p|Voice=a	0	PRED	_	t_25
6	et	et1	c	_	_	5	AuxY	_	t_26
7	filius	filius	n	_	Case=n|Gender=m|Number=s	9	SBJ	_	t_27
8	semper	semper	d	_	_	9	ADV	_	t_28
9	ludit	ludo	v	_	Mood=i|Number=s|Person=3|Tense=p|Voice=a	5	PRED_CO	_	t_29
10	.	.	u	_	_	5	AuxK	_	t_30
`

// latinDeckPaula cites the first sentence 1.pr.1, the second across 1.pr.1 and
// 1.pr.2, and the third 1.pr.2.
func latinDeckPaula() string {
	var layer strings.Builder
	layer.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="no"?>` + "\n" +
		`<paula version="1.1"><header paula_id="synthetic"/><featList xmlns:xlink="http://www.w3.org/1999/xlink" type="cts" xml:base="x.tok01.xml">` + "\n")
	for index := range 30 {
		citation := "1.pr.1"
		if index >= 25 || (index >= 15 && index < 20) {
			citation = "1.pr.2"
		}
		fmt.Fprintf(&layer, `<feat id="u_%d" xlink:href="#t_%d" value="%s"/>`+"\n", index+1, index+1, citation)
	}
	layer.WriteString("</featList></paula>\n")
	return layer.String()
}

// latinDeckTranslation is a synthetic perseus-engN file over the two sections.
const latinDeckTranslation = `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="pr">` +
	`<div type="textpart" subtype="section" n="1"><p>The first synthetic section.</p></div>` +
	`<div type="textpart" subtype="section" n="2"><p>The second synthetic section.</p></div>` +
	`</div></div></TEI>`

// latinDeckLexicon is a Wiktionary-style lexical provider carrying data for
// Latin, which a Latin deck must never use as meaning evidence.
type latinDeckLexicon struct{}

func (latinDeckLexicon) Name() string    { return "latin-deck-dictionary" }
func (latinDeckLexicon) Version() string { return "kaikki-la-test" }
func (latinDeckLexicon) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	senses := []enrichment.LexicalSense{{EvidenceID: "wikt:domus", Source: "wiktionary", Gloss: "house, home", MatchStrength: "exact"}}
	return enrichment.LexicalEntry{Senses: senses, CandidateSenses: senses}, true, nil
}

const latinDeckLSPin = "56061ca127f4a2844980baffc5f2b6d1332897b3"

// LookupLatinLS answers the Lewis & Short capability: two homograph entries
// for domus, one with a citation of the Work's author and one of another, and
// none for sto.
func (latinDeckLexicon) LookupLatinLS(_ context.Context, lemma string) ([]enrichment.LatinLSEntry, bool) {
	if lemma != "domus" {
		return nil, false
	}
	return []enrichment.LatinLSEntry{
		{EntryID: "n100", Headword: "domus", Orth: "domus", Pin: latinDeckLSPin, Senses: []enrichment.LatinLSSense{
			{ID: "n100.0", Number: "I", Level: 1, Definitions: []string{"a house"}, Glosses: []string{"a house"},
				Citations: []enrichment.LatinLSCitation{{Author: "Anon.", Text: "Anon. 1, 2"}, {Author: "Cic.", Text: "Cic. Att. 3, 4"},
					{Author: "phi0474", Text: "urn own 5, 6"}, {Author: "phi0914", Text: "urn other 7, 8"}}},
		}},
		{EntryID: "n101", Headword: "domus", Orth: "domus", Pin: latinDeckLSPin, Senses: []enrichment.LatinLSSense{
			{ID: "n101.0", Number: "I", Level: 1, Definitions: []string{"a household"}, Glosses: []string{"a household"}},
		}},
	}, true
}

// TestClassicalLatinDeckHasPassageContextWithNoProvider extends the Latin loop
// to a prepared deck with no provider configured and Idiom cards listed for
// Latin: the Work's pinned translation is fetched at preparation, every card
// shows its labelled passage context, and the deck carries no Wiktionary
// meaning evidence, no Idiom card, no analysis-models notice and the Latin
// source and translation notices.
func TestClassicalLatinDeckHasPassageContextWithNoProvider(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "classical-latin-deck-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	build := buildLatinLoopArchiveFrom(t, "ola-deck-layer", latinDeckConllu, latinDeckPaula())
	archiveServer := &rangeServer{archive: build.archive}
	archiveHTTP := httptest.NewServer(archiveServer.handler())
	t.Cleanup(archiveHTTP.Close)
	translationHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, writeErr := w.Write([]byte(latinDeckTranslation)); writeErr != nil {
			return
		}
	}))
	t.Cleanup(translationHTTP.Close)
	translationSum := sha256.Sum256([]byte(latinDeckTranslation))
	translationSHA := hex.EncodeToString(translationSum[:])

	definition := classical.Definition{
		Key: latinDeckUnitKey, WorkKey: latinDeckUnitKey, WorkTitle: "A Synthetic History", WorkAuthor: "Anon",
		OriginalTitle: "Historia", EditionKey: latinDeckUnitKey + "-edition", EditionLabel: "Perseus Latin",
		Language: classical.LanguageLatin, ConstituentBooks: 1, CitationStart: "1.pr.1", CitationEnd: "1.pr.2",
		WordCount: 27, Opening: "domus magna in urbe stat et dominus bene habitat.", Source: build.source,
		Translation: &classical.TranslationPin{
			Layer: "perseus-latin-translation", FileName: latinDeckTranslationFile, Size: int64(len(latinDeckTranslation)),
			SHA256: translationSHA, CitationStart: "1.pr.1", CitationEnd: "1.pr.2",
		},
		InterpretationVersion: classical.InterpretationVersionOLA, Revision: 1,
	}
	definitions := []classical.Definition{definition}
	require.NoError(t, store.ReconcileClassicalManifest(ctx, definitions))
	catalog, err := classical.NewCatalog(definitions)
	require.NoError(t, err)

	layers := classical.Layers{Layers: []classical.Layer{
		{
			Key: "ola-deck-layer", Release: "v0.2.0", FetchKind: classical.FetchKindArchiveRange,
			ArchiveURL: archiveHTTP.URL + latinLoopArchive, ArchiveSize: int64(len(build.archive)), Record: "10.5281/zenodo.15183688",
			LicenceClass: classical.LicenceClassAnalysisResource, Restriction: classical.RestrictionUnclear,
			DeployerRegisterRow: "synthetic OLA layer",
		},
		{
			Key: "perseus-latin-translation", Commit: "synthetic", BaseURL: translationHTTP.URL + "/",
			LicenceClass: classical.LicenceClassSourceContent, Restriction: classical.RestrictionShareAlike,
			DeployerRegisterRow: "synthetic translation layer",
		},
	}}
	require.NoError(t, layers.Validate())
	restoreRegister := cardexport.UsePassageRegister(func(pin string) (classical.PassageAttribution, bool, error) {
		if strings.TrimPrefix(pin, "sha256:") != translationSHA {
			return classical.PassageAttribution{}, false, nil
		}
		return classical.PassageAttribution{
			Label: latinDeckTranslationLabel,
			Notice: "Passage context: synthetic translation in the Perseus perseus-eng9 digitisation (PerseusDL/canonical-latinLit@" +
				latinDeckTranslationPin + ", SHA-256 " + translationSHA + "). CC BY-SA 4.0; derived data remains share-alike.",
		}, true, nil
	})
	t.Cleanup(restoreRegister)

	config := analysis.ClassicalPreparationConfig{Layers: layers, Definitions: definitions, HTTPClient: archiveHTTP.Client(), TempDir: t.TempDir()}
	workers := river.NewWorkers()
	preparationWorker := analysis.AddClassicalPreparationWorker(workers, store, config)
	analysis.AddClassicalTranslationWorker(workers, store, config)
	unusedAnalyzer := &analyzertest.Fake{AnalyzeFunc: func(context.Context, analyzer.AnalyzeRequest) (analyzer.Result, error) {
		return analyzer.Result{}, errors.New("a Latin Work never calls the NLP service")
	}}
	client, err := analysis.NewClientWithPreparedDeckConcurrency(pool, unusedAnalyzer, analyzertest.CapabilityProvider{}, selection.NewService(store), 1, workers)
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "latin deck client", client.Stop)
	preparationWorker.Client = client

	presentation := cardexport.NewPresentation(latinDeckLexicon{})
	preparedConfig := prepareddeck.PreparedDeckConfig{
		TranslationMode: prepareddeck.DefaultTranslationMode, StandardMaxConcurrency: 1, StandardMaxAttempts: 1,
		StandardRetryBaseDelay: time.Millisecond, StandardRetryMaxDelay: time.Millisecond,
		IdiomCardLanguages: map[string]bool{"la": true},
	}
	batchConfig := prepareddeck.BatchConfig{MaxRequests: 1, PollInterval: time.Millisecond}
	metrics := prepareddeck.NewMetricsCollector()
	provider := &greekDeckRecordingProvider{}
	prepareddeck.AddPreparedDeckWorker(workers, store, presentation, client, nil, batchConfig, preparedConfig, false)
	prepareddeck.AddStandardTranslationWorkerWithDependencies(workers, store, client, provider, preparedConfig, time.Second, metrics)
	prepareddeck.AddBatchSubmitWorkerWithMetrics(workers, store, client, nil, nil, metrics)
	prepareddeck.AddBatchPollWorker(workers, &prepareddeck.BatchPollWorker{Store: store, Client: client, PollInterval: batchConfig.PollInterval, Metrics: metrics})
	prepareddeck.AddFinalizeWorker(workers, &prepareddeck.DurableFinalizer{Store: store, Renderer: presentation, Metrics: metrics})
	prepareddeck.AddBatchCleanupWorker(workers, &prepareddeck.BatchCleanupWorker{Store: store, Metrics: metrics})
	prepareddeck.AddRecoveryWorkerWithMetrics(workers, store, client, batchConfig.PollInterval, metrics)
	require.NoError(t, prepareddeck.EnsureRecoveryJob(ctx, store, client))
	require.NoError(t, client.Start(ctx))

	service := analysis.NewService(store.Pool(), client).WithClassicalPreparationStore(store)
	authService := auth.New(store, time.Hour)
	handler := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Analysis: service, AnalysisUpdates: service, PreparedDeck: prepareddeck.NewService(store, client),
		Capabilities: analyzertest.CapabilityProvider{}, ClassicalPreparation: service, Classical: catalog,
		CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), SessionLifetime: time.Hour,
	})
	learner := createAccount(t, ctx, store, "latin-deck-learner", "learner-password")
	cookies, csrf := loginCookies(t, handler, learner.Username, "learner-password")

	added := perform(t, handler, http.MethodPost, "/catalogs/classical/units/"+latinDeckUnitKey+"/add", url.Values{
		"csrf_token": {csrf}, "return": {"index"}, "catalog_language": {"la"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code, added.Body.String())
	bookID := booksByReadingUnit(t, store, learner.ID)[latinDeckUnitKey]
	require.NotEmpty(t, bookID)
	testutil.Eventually(t, 60*time.Second, "Latin Book analysed", func() (bool, string) {
		return countRows(t, pool, `SELECT count(*) FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, learner.ID, bookID) == 1, "analysis not published"
	})
	testutil.Eventually(t, 60*time.Second, "translation fetched at preparation", func() (bool, string) {
		return countRows(t, pool, `SELECT count(*) FROM reference_translations WHERE translation_pin_sha256=$1 AND language='la'`, translationSHA) == 2, "translation rows not stored"
	})
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "la"))

	started := perform(t, handler, http.MethodPost, "/reading/books/"+bookID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code, started.Body.String())
	reading, err := store.GetCurrentReading(ctx, learner.ID, "la")
	require.NoError(t, err)
	require.Equal(t, 2, reading.SnapshotSize, "domus and sto recur in every sentence")

	submitted := perform(t, handler, http.MethodPost, "/reading/books/"+bookID+"/deck/retry", url.Values{
		"csrf_token": {csrf}, "expected_current_snapshot_id": {reading.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, submitted.Code, submitted.Body.String())
	preparation := waitForPreparedDeck(t, ctx, store, learner.ID, reading.SnapshotID)
	require.Equal(t, domain.DeckPreparationReady, preparation.State, "preparation error=%q", preparation.Error)
	assert.Equal(t, 2, preparation.TotalCards, "one default card per identity and no Idiom card")
	assert.Zero(t, provider.calls, "no provider request is made for a Latin deck")

	rows, err := pool.Query(ctx, `SELECT render_payload, passage_context, passage_citation, brownson_pin, canonical_lemma, lsj_entry_id, lsj_pin FROM deck_preparation_manifest_items WHERE owner_id=$1`, learner.ID)
	require.NoError(t, err)
	defer rows.Close()
	cards := 0
	for rows.Next() {
		var payload []byte
		var passage, citation, pin, lemma, entryID, entryPin string
		require.NoError(t, rows.Scan(&payload, &passage, &citation, &pin, &lemma, &entryID, &entryPin))
		var item struct {
			DictionaryEntry string            `json:"dictionary_entry"`
			Gloss           string            `json:"gloss"`
			CandidateSenses []json.RawMessage `json:"candidate_senses"`
		}
		require.NoError(t, json.Unmarshal(payload, &item))
		assert.Empty(t, item.Gloss, "a Latin deck resolves no contextual Gloss")
		assert.Empty(t, item.CandidateSenses, "a Latin deck freezes no Wiktionary meaning evidence")
		assert.Contains(t, passage, "synthetic section", "the card froze the pinned English passage")
		assert.Contains(t, citation, "1.pr.")
		assert.Equal(t, "sha256:"+translationSHA, pin)
		if lemma == "domus" {
			assert.Equal(t, "n100,n101", entryID, "the frozen item records every Lewis & Short entry")
			assert.Equal(t, latinDeckLSPin, entryPin)
			assert.Contains(t, item.DictionaryEntry, "a house")
			assert.Contains(t, item.DictionaryEntry, "a household")
			assert.Contains(t, item.DictionaryEntry, "Anon. 1, 2")
			assert.Contains(t, item.DictionaryEntry, "urn own 5, 6", "a URN-only citation matches the Work's CTS textgroup from its file name")
			assert.NotContains(t, item.DictionaryEntry, "urn other 7, 8")
			assert.NotContains(t, item.DictionaryEntry, "Cic. Att.", "the full entry keeps only the Work author's citations")
		} else {
			assert.Empty(t, entryID, "a lemma with no entry freezes none")
			assert.Empty(t, item.DictionaryEntry, "and its card has no dictionary block")
		}
		cards++
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, 2, cards)

	for _, want := range []string{"Passage context: " + latinDeckTranslationLabel + " 1.pr.", "Not a translation of this sentence", "Opera Latina Adnotata", "canonical-latinLit", latinDeckTranslationPin, "PerseusDL/lexica@" + latinDeckLSPin, "a household"} {
		assert.True(t, bytes.Contains(preparation.Artifact, []byte(want)), "exported Latin deck is missing %q", want)
	}
	for _, unwanted := range []string{"Wiktionary", "house, home", "Liddell", "OGA v0.2.0", "Stanza"} {
		assert.False(t, bytes.Contains(preparation.Artifact, []byte(unwanted)), "exported Latin deck must not carry %q", unwanted)
	}
}
