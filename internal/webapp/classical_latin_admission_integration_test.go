//go:build integration

package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const latinAdmissionUnitKey = "admission-work"

// latinAdmissionConllu is a synthetic OLA CoNLL-U whose lemmas exercise Latin
// admission: a praenomen abbreviation, a Greek-script lemma, an artefact-digit
// lemma, an enclitic, and cum as a preposition and as a conjunction.
const latinAdmissionConllu = `# absolute_id = 0
# clause_id = c_1
# file = synthetic.xml
1	M.	M.	n	_	Case=n|Gender=m|Number=s	2	SBJ	_	t_1
2	uidet	uideo	v	_	Mood=i|Number=s|Person=3|Tense=p|Voice=a	0	PRED	_	t_2
3	regem	rex1	n	_	Case=a|Gender=m|Number=s	2	OBJ	_	t_3
4	λόγον	λόγος	n	_	Case=a|Gender=m|Number=s	2	OBJ	_	t_4
5	cum	cum	r	_	_	2	AuxP	_	t_5
6	duce	dux	n	_	Case=b|Gender=m|Number=s	5	ADV	_	t_6
7	cum	cum	c	_	_	2	AuxC	_	t_7
8	militibus	miles	n	_	Case=b|Gender=m|Number=p	7	ADV	_	t_8
9	que	que	c	_	_	2	COORD	_	t_9
10	.	.	u	_	_	2	AuxK	_	t_10
11	nugae	nug2ae3	n	_	Case=n|Gender=f|Number=p	2	SBJ	_	t_11

`

func latinAdmissionPaula() string {
	var layer strings.Builder
	layer.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="no"?>` + "\n" +
		`<paula version="1.1"><header paula_id="synthetic"/><featList xmlns:xlink="http://www.w3.org/1999/xlink" type="cts" xml:base="x.tok01.xml">` + "\n")
	for index := 1; index <= 11; index++ {
		fmt.Fprintf(&layer, `<feat id="u_%d" xlink:href="#t_%d" value="1.1"/>`+"\n", index, index)
	}
	layer.WriteString("</featList></paula>\n")
	return layer.String()
}

// TestClassicalLatinAdmissionKeepsTokensOutOfVocabulary prepares a synthetic
// Latin Work and checks the identities Browse lists, the citation headword and
// lemma review (correction and exclusion) on a Latin occurrence.
func TestClassicalLatinAdmissionKeepsTokensOutOfVocabulary(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "classical-latin-admission-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	build := buildLatinLoopArchiveFrom(t, "ola-admission-layer", latinAdmissionConllu, latinAdmissionPaula())
	server := &rangeServer{archive: build.archive}
	httpServer := httptest.NewServer(server.handler())
	t.Cleanup(httpServer.Close)
	definition := classical.Definition{
		Key: latinAdmissionUnitKey, WorkKey: latinAdmissionUnitKey, WorkTitle: "The Admission Work", WorkAuthor: "Anon",
		OriginalTitle: "De Admissione", EditionKey: "admission-work-perseus", EditionLabel: "Perseus Latin",
		Language: classical.LanguageLatin, ConstituentBooks: 1, CitationStart: "1.1", CitationEnd: "1.1",
		WordCount: 9, Opening: "M. uidet regem.", Source: build.source,
		InterpretationVersion: classical.InterpretationVersionOLA, Revision: 1,
	}
	definitions := []classical.Definition{definition}
	require.NoError(t, store.ReconcileClassicalManifest(ctx, definitions))
	catalog, err := classical.NewCatalog(definitions)
	require.NoError(t, err)
	layers := classical.Layers{Layers: []classical.Layer{{
		Key: "ola-admission-layer", Release: "v0.2.0", FetchKind: classical.FetchKindArchiveRange,
		ArchiveURL: httpServer.URL + latinLoopArchive, ArchiveSize: int64(len(build.archive)), Record: "10.5281/zenodo.15183688",
		LicenceClass: classical.LicenceClassAnalysisResource, Restriction: classical.RestrictionUnclear,
		DeployerRegisterRow: "synthetic OLA layer",
	}}}
	require.NoError(t, layers.Validate())
	config := analysis.ClassicalPreparationConfig{Layers: layers, Definitions: definitions, HTTPClient: httpServer.Client(), TempDir: t.TempDir()}
	workers := river.NewWorkers()
	analysis.AddClassicalPreparationWorker(workers, store, config)
	unusedAnalyzer := &analyzertest.Fake{AnalyzeFunc: func(context.Context, analyzer.AnalyzeRequest) (analyzer.Result, error) {
		return analyzer.Result{}, errors.New("the Classical catalog never calls the NLP service")
	}}
	client, err := analysis.NewClient(pool, unusedAnalyzer, analyzertest.CapabilityProvider{}, selection.NewService(store), workers)
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "latin admission client", client.Stop)
	service := analysis.NewService(store.Pool(), client).WithClassicalPreparationStore(store)
	authService := auth.New(store, time.Hour)
	handler := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Analysis: service, AnalysisUpdates: service, AnalysisInsights: analysisinsights.NewService(store),
		Capabilities:         analyzertest.CapabilityProvider{},
		ClassicalPreparation: service, Classical: catalog,
		CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), SessionLifetime: time.Hour,
	})
	learner := createAccount(t, ctx, store, "latin-admission-learner", "learner-password")
	cookies, csrf := loginCookies(t, handler, learner.Username, "learner-password")

	added := perform(t, handler, http.MethodPost, "/catalogs/classical/units/"+latinAdmissionUnitKey+"/add", url.Values{
		"csrf_token": {csrf}, "return": {"index"}, "catalog_language": {"la"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code, added.Body.String())
	bookID := booksByReadingUnit(t, store, learner.ID)[latinAdmissionUnitKey]
	require.NotEmpty(t, bookID)
	require.NoError(t, client.Start(ctx))
	testutil.Eventually(t, 60*time.Second, "Latin Book analysed", func() (bool, string) {
		return countRows(t, pool, `SELECT count(*) FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, learner.ID, bookID) == 1, "analysis not published"
	})
	var run, corpus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text, corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, learner.ID, bookID).Scan(&run, &corpus))
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "la"))

	// Every token stays in the text and in Reading.
	unit := domain.ClassicalUnitID(latinAdmissionUnitKey)
	study := perform(t, handler, http.MethodGet, "/la/vocabulary/concordance/sentence?"+url.Values{
		"book": {bookID}, "run": {run}, "corpus": {corpus}, "unit": {unit}, "sentence": {"0"}, "target": {"0"},
	}.Encode(), nil, cookies)
	require.Equal(t, http.StatusOK, study.Code, study.Body.String())
	assert.Contains(t, study.Body.String(), "λόγον")
	assert.Contains(t, study.Body.String(), "M.")
	assert.Contains(t, study.Body.String(), "nugae", "a lemma with a digit left after the strip stays in the text")

	// Browse lists only admitted identities, keyed by the latin/1 profile:
	// no abbreviation, no Greek-script lemma, no enclitic, no cum (ADP and
	// CCONJ/SCONJ are not vocabulary parts of speech).
	counts := greekBrowseCounts(t, ctx, pool, learner.ID, bookID)
	assert.Equal(t, map[string]int64{"uideo|VERB|": 1, "rex|NOUN|": 1, "dux|NOUN|": 1, "miles|NOUN|": 1}, counts)
	page, err := store.ListVocabularyBrowsePage(ctx, learner.ID, "la", domain.VocabularyBrowseQuery{Page: 1})
	require.NoError(t, err)
	headwords := map[string]string{}
	for _, row := range page.Rows {
		headwords[row.CanonicalLemma] = row.Headword
		assert.Empty(t, row.LexicalDiscriminator)
	}
	assert.Equal(t, "rex", headwords["rex"], "the citation headword is the source lemma without the artefact digit")
	assert.Equal(t, "uideo", headwords["uideo"])

	// Lemma review works on a Latin occurrence: a correction is normalized by
	// the recorded latin profile, and an exclusion drops the identity.
	regem, err := store.ListLemmaReviewOccurrences(ctx, learner.ID, bookID, "regem")
	require.NoError(t, err)
	require.Len(t, regem, 1)
	confirmLemmaDecisionForm(t, handler, learner.ID, bookID, "regem", "correct", "Rēgīna", regem[0].ID())
	militibus, err := store.ListLemmaReviewOccurrences(ctx, learner.ID, bookID, "militibus")
	require.NoError(t, err)
	require.Len(t, militibus, 1)
	confirmLemmaDecisionForm(t, handler, learner.ID, bookID, "militibus", "exclude", "", militibus[0].ID())
	var profile, version, corrected string
	require.NoError(t, pool.QueryRow(ctx, `SELECT normalization_profile,normalization_version,canonical_lemma FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2 AND canonical_lemma IS NOT NULL`, learner.ID, bookID).Scan(&profile, &version, &corrected))
	assert.Equal(t, "latin", profile)
	assert.Equal(t, "1", version)
	assert.Equal(t, "regina", corrected)
	assert.Equal(t, map[string]int64{"uideo|VERB|": 1, "regina|NOUN|": 1, "dux|NOUN|": 1}, greekBrowseCounts(t, ctx, pool, learner.ID, bookID))

	// A correction to a lemma Latin admission rejects is refused.
	duce, err := store.ListLemmaReviewOccurrences(ctx, learner.ID, bookID, "duce")
	require.NoError(t, err)
	require.Len(t, duce, 1)
	request := lemmaDecisionRequest(t, handler, learner.ID, "duce", "correct", "M.")
	_, err = handler.previewLemmaDecisionStep(request, learner.ID, bookID, []string{duce[0].ID()})
	require.ErrorIs(t, err, domain.ErrLemmaReviewLemma, "no confirmable preview for an inadmissible lemma")
}
