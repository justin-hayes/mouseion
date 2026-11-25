//go:build integration

package webapp

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
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
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	latinLoopUnitKey = "caesar-gallic-war"
	latinLoopArchive = "/opera_latina_adnotata_v0.2.0.zip"
)

// latinLoopConllu is a synthetic OLA CoNLL-U for one Work: a preface sentence
// cited 1.pr.1 and a chapter sentence whose Citation carries a letter suffix.
const latinLoopConllu = `# absolute_id = 0
# clause_id = c_1
# file = synthetic.xml
1	Gallia	Gallia	n	_	Case=n|Gender=f|Number=s	4	SBJ	_	t_1
2	est	sum	v	_	Mood=i|Number=s|Person=3|Tense=p|Voice=a	0	PRED	_	t_2
3	omnis	omnis	a	_	Case=n|Gender=f|Number=s	1	ATR	_	t_3
4	diuisa	diuido	v	_	Case=n|Gender=f|Mood=p|Number=s|Tense=r|Voice=p	2	PNOM	_	t_4
5	.	.	u	_	_	2	AuxK	_	t_5

# absolute_id = 1
# clause_id = c_2
# file = synthetic.xml
1	Belgae	Belgae	n	_	Case=n|Gender=m|Number=p	3	SBJ	_	t_6
2	et	et1	c	_	_	3	COORD	_	t_7
3	Aquitani	Aquitani	n	_	Case=n|Gender=m|Number=p	4	SBJ_CO	_	t_8
4	incolunt	incolo	v	_	Mood=i|Number=p|Person=3|Tense=p|Voice=a	0	PRED	_	t_9
5	.	.	u	_	_	4	AuxK	_	t_10

`

// latinLoopPaula is the synthetic PAULA Citation layer for latinLoopConllu.
func latinLoopPaula() string {
	citations := []string{"1.pr.1", "1.pr.1", "1.pr.1", "1.pr.1", "1.pr.1", "1.1a", "1.1a", "1.1a", "1.1a", "1.1a"}
	var layer strings.Builder
	layer.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="no"?>` + "\n" +
		`<paula version="1.1"><header paula_id="synthetic"/><featList xmlns:xlink="http://www.w3.org/1999/xlink" type="cts" xml:base="x.tok01.xml">` + "\n")
	for index, citation := range citations {
		fmt.Fprintf(&layer, `<feat id="u_%d" xlink:href="#t_%d" value="%s"/>`+"\n", index+1, index+1, citation)
	}
	layer.WriteString("</featList></paula>\n")
	return layer.String()
}

// latinLoopFiller is incompressible, deterministic filler that makes the
// archive much larger than the Work's two members.
func latinLoopFiller(label string) []byte {
	var filler []byte
	sum := sha256.Sum256([]byte(label))
	for len(filler) < 32<<10 {
		filler = append(filler, sum[:]...)
		sum = sha256.Sum256(sum[:])
	}
	return filler
}

// latinLoopArchiveBuild is a synthetic OLA zip with the Work's two members among
// unrelated ones, and the pins the manifest generator would record for them.
type latinLoopArchiveBuild struct {
	archive []byte
	source  classical.SourcePin
}

func buildLatinLoopArchive(t *testing.T, layer string) latinLoopArchiveBuild {
	t.Helper()
	const prefix = "opera_latina_adnotata_v0.2.0/"
	conlluName := prefix + "workspace/conllu/synthetic.conllu"
	paulaName := prefix + "workspace/ola/synthetic.tok01_cts01.xml"
	contents := map[string][]byte{
		prefix + "README.md": bytes.Repeat([]byte("readme\n"), 50),
		conlluName:           []byte(latinLoopConllu),
		prefix + "workspace/ola/other.tok01_cts01.xml": latinLoopFiller("other"),
		paulaName: []byte(latinLoopPaula()),
	}
	order := []string{prefix + "README.md", conlluName, prefix + "workspace/ola/other.tok01_cts01.xml", paulaName}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	extra := make([]byte, 24)
	binary.LittleEndian.PutUint16(extra[0:], 0x7875)
	binary.LittleEndian.PutUint16(extra[2:], 20)
	for _, name := range order {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Extra: extra})
		require.NoError(t, err)
		_, err = entry.Write(contents[name])
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	require.NoError(t, err)
	pins := map[string]classical.ArchiveMember{}
	for _, file := range reader.File {
		dataOffset, err := file.DataOffset()
		require.NoError(t, err)
		sum := sha256.Sum256(contents[file.Name])
		pins[file.Name] = classical.ArchiveMember{
			Name:           file.Name,
			HeaderOffset:   dataOffset - 30 - int64(len(file.Name)) - int64(len(extra)),
			CompressedSize: int64(file.CompressedSize64),   //nolint:gosec // synthetic member sizes are tiny
			Size:           int64(file.UncompressedSize64), //nolint:gosec // synthetic member sizes are tiny
			CRC32:          fmt.Sprintf("%08x", crc32.ChecksumIEEE(contents[file.Name])),
			SHA256:         hex.EncodeToString(sum[:]),
		}
	}
	conllu, paula := pins[conlluName], pins[paulaName]
	return latinLoopArchiveBuild{archive: buffer.Bytes(), source: classical.SourcePin{
		Layer: layer, FileName: conllu.Name, Size: conllu.Size, SHA256: conllu.SHA256,
		Format: classical.SourceFormatOLACoNLLU, Members: []classical.ArchiveMember{conllu, paula},
	}}
}

// rangeServer serves one archive and honours a single Range request, as Zenodo
// does. It records every request so a test can prove only the members' byte
// windows were ever served.
type rangeServer struct {
	archive []byte
	mu      sync.Mutex
	ranges  []string
	served  int
}

func (s *rangeServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		spec, ok := strings.CutPrefix(r.Header.Get("Range"), "bytes=")
		first, last, found := strings.Cut(spec, "-")
		start, startErr := strconv.Atoi(first)
		end, endErr := strconv.Atoi(last)
		s.mu.Lock()
		s.ranges = append(s.ranges, r.Header.Get("Range"))
		s.mu.Unlock()
		if r.URL.Path != latinLoopArchive || !ok || !found || startErr != nil || endErr != nil || start > end || end >= len(s.archive) {
			http.Error(w, "range required", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		s.mu.Lock()
		s.served += end - start + 1
		s.mu.Unlock()
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(s.archive)))
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		if _, err := w.Write(s.archive[start : end+1]); err != nil {
			return
		}
	})
}

func (s *rangeServer) requests() ([]string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ranges...), s.served
}

// TestClassicalLatinCatalogLoop walks a Latin Work end to end through the real
// routes, store and workers with no NLP service: the Catalog filtered to Latin,
// Add, preparation fetching the Work's two archive members by Range, Reading and
// Sentence study with widened Citations and stored AGDT relations, the Latin
// study language, and no Construction views.
func TestClassicalLatinCatalogLoop(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "classical-latin-loop-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	build := buildLatinLoopArchive(t, "ola-loop-layer")
	server := &rangeServer{archive: build.archive}
	httpServer := httptest.NewServer(server.handler())
	t.Cleanup(httpServer.Close)

	definition := classical.Definition{
		Key: latinLoopUnitKey, WorkKey: latinLoopUnitKey, WorkTitle: "The Gallic War", WorkAuthor: "Caesar",
		OriginalTitle: "De Bello Gallico", EditionKey: "caesar-gallic-war-perseus", EditionLabel: "Perseus Latin",
		Language: classical.LanguageLatin, ConstituentBooks: 1, CitationStart: "1.pr.1", CitationEnd: "1.1a",
		WordCount: 8, Opening: "Gallia est omnis diuisa.", Source: build.source,
		InterpretationVersion: classical.InterpretationVersionOLA, Revision: 1,
	}
	definitions := []classical.Definition{definition}
	require.NoError(t, store.ReconcileClassicalManifest(ctx, definitions))
	catalog, err := classical.NewCatalog(definitions)
	require.NoError(t, err)

	layers := classical.Layers{Layers: []classical.Layer{{
		Key: "ola-loop-layer", Release: "v0.2.0", FetchKind: classical.FetchKindArchiveRange,
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
	client, err := analysis.NewClientWithPreparedDeckConcurrency(pool, unusedAnalyzer, analyzertest.CapabilityProvider{}, selection.NewService(store), 1, workers)
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "latin loop client", client.Stop)
	service := analysis.NewService(store.Pool(), client).WithClassicalPreparationStore(store)
	authService := auth.New(store, time.Hour)
	handler := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Analysis: service, AnalysisUpdates: service, AnalysisInsights: analysisinsights.NewService(store),
		PreparedDeck: prepareddeck.NewService(store, client), Capabilities: analyzertest.CapabilityProvider{},
		ClassicalPreparation: service, Classical: catalog,
		CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), SessionLifetime: time.Hour,
	})
	learner := createAccount(t, ctx, store, "latin-loop-learner", "learner-password")
	cookies, csrf := loginCookies(t, handler, learner.Username, "learner-password")

	// 1. The Catalog filtered to Latin lists the Work with its Latin title
	// marked as Latin.
	index := perform(t, handler, http.MethodGet, "/catalogs?language=la", nil, cookies)
	require.Equal(t, http.StatusOK, index.Code, index.Body.String())
	for _, want := range []string{"Classical catalog", "The Gallic War", `lang="la"`, "De Bello Gallico", `action="/catalogs/classical/units/` + latinLoopUnitKey + `/add"`} {
		assert.Contains(t, index.Body.String(), want, "Latin catalog missing %q", want)
	}

	// 2. Add gives one Latin Book, names Latin, and queues preparation.
	added := perform(t, handler, http.MethodPost, "/catalogs/classical/units/"+latinLoopUnitKey+"/add", url.Values{
		"csrf_token": {csrf}, "return": {"index"}, "catalog_language": {"la"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code, added.Body.String())
	books := booksByReadingUnit(t, store, learner.ID)
	bookID := books[latinLoopUnitKey]
	require.NotEmpty(t, bookID)
	var languageTag, displayName string
	require.NoError(t, pool.QueryRow(ctx, `SELECT language_tag FROM books WHERE owner_id=$1 AND id=$2`, learner.ID, bookID).Scan(&languageTag))
	require.NoError(t, pool.QueryRow(ctx, `SELECT display_name FROM supported_languages WHERE language=$1`, "la").Scan(&displayName))
	assert.Equal(t, "la", languageTag, "the Book is Latin, not Ancient Greek")
	assert.Equal(t, "Latin", displayName)
	assert.Zero(t, countRows(t, pool, `SELECT count(*) FROM supported_languages WHERE language='grc'`), "Adding Latin does not make Greek available")
	ranges, _ := server.requests()
	assert.Empty(t, ranges, "Add fetches nothing; the worker does")

	// 3. Preparation fetches exactly the two members by Range, never the archive.
	require.NoError(t, client.Start(ctx))
	testutil.Eventually(t, 60*time.Second, "Latin Book analysed", func() (bool, string) {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, learner.ID, bookID).Scan(&count); err != nil {
			return false, err.Error()
		}
		return count == 1, "analysis not published"
	})
	ranges, served := server.requests()
	require.Len(t, ranges, 2, "one Range request per pinned member")
	for _, requested := range ranges {
		assert.True(t, strings.HasPrefix(requested, "bytes="), requested)
	}
	assert.Less(t, served, len(build.archive), "the whole archive is never served")
	testutil.Eventually(t, 60*time.Second, "prepared definition recorded", func() (bool, string) {
		return countRows(t, pool, `SELECT count(*) FROM classical_prepared_definitions WHERE owner_id=$1 AND book_id=$2`, learner.ID, bookID) == 1, "not recorded"
	})
	var syntaxState, run, corpus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text, corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, learner.ID, bookID).Scan(&run, &corpus))
	require.NoError(t, pool.QueryRow(ctx, `SELECT syntax_state FROM analysis_runs WHERE owner_id=$1 AND id=$2`, learner.ID, run).Scan(&syntaxState))
	assert.Equal(t, "native_agdt", syntaxState)

	// 4. The prepared Book is never outdated: no analysis-update offer, even
	// though no NLP capability exists for Latin.
	var sourceID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT source_material_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, learner.ID, bookID).Scan(&sourceID))
	candidates, err := service.AnalysisUpdateCandidates(ctx, learner.ID, []string{sourceID})
	require.NoError(t, err)
	assert.Empty(t, candidates)
	library := perform(t, handler, http.MethodGet, "/library", nil, cookies)
	require.Equal(t, http.StatusOK, library.Code, library.Body.String())
	assert.NotContains(t, library.Body.String(), analysisUpdateNotice)

	// 5. Latin is a study language and /la/… works.
	reading := perform(t, handler, http.MethodGet, "/la/reading", nil, cookies)
	require.Equal(t, http.StatusOK, reading.Code, reading.Body.String())
	assert.Contains(t, reading.Body.String(), "The Gallic War")
	assert.Contains(t, reading.Body.String(), ">Latin<", "the study-language picker names Latin without a code")
	started := perform(t, handler, http.MethodPost, "/reading/books/"+bookID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code, started.Body.String())

	// 6. Concordance shows the widened Citations; the artefact digit is gone
	// from the lemma.
	found := perform(t, handler, http.MethodGet, "/la/vocabulary/concordance?"+url.Values{"term": {"gallia"}, "as": {domain.ConcordanceMatchLemma}}.Encode(), nil, cookies)
	require.Equal(t, http.StatusOK, found.Code, found.Body.String())
	assert.Contains(t, found.Body.String(), "1.pr.1")
	assert.NotContains(t, found.Body.String(), "No analyzed occurrences")
	suffixed := perform(t, handler, http.MethodGet, "/la/vocabulary/concordance?"+url.Values{"term": {"et"}, "as": {domain.ConcordanceMatchLemma}}.Encode(), nil, cookies)
	require.Equal(t, http.StatusOK, suffixed.Code, suffixed.Body.String())
	assert.Contains(t, suffixed.Body.String(), "1.1a", "a letter-suffixed Citation is shown as the Edition prints it")
	assert.NotContains(t, suffixed.Body.String(), "No analyzed occurrences", "the source lemma et1 is stored as et")

	// 7. Sentence study shows the stored AGDT relations and no Constructions.
	unit := domain.ClassicalUnitID(latinLoopUnitKey)
	study := perform(t, handler, http.MethodGet, "/la/vocabulary/concordance/sentence?"+url.Values{
		"book": {bookID}, "run": {run}, "corpus": {corpus}, "unit": {unit}, "sentence": {"1"}, "target": {"0"},
	}.Encode(), nil, cookies)
	require.Equal(t, http.StatusOK, study.Code, study.Body.String())
	body := study.Body.String()
	assert.Contains(t, body, "Belgae et Aquitani incolunt.")
	assert.Contains(t, body, "<summary>Word relations</summary>")
	assert.Contains(t, body, "<td>subject</td>", "the stored AGDT relation is shown as Word relations")
	assert.NotContains(t, body, "Syntax unavailable")
	assert.NotContains(t, body, "sentence-study-construction", "Constructions fail closed for Latin")
	assert.Contains(t, body, "1.1a")
	preface := perform(t, handler, http.MethodGet, "/la/vocabulary/concordance/sentence?"+url.Values{
		"book": {bookID}, "run": {run}, "corpus": {corpus}, "unit": {unit}, "sentence": {"0"}, "target": {"0"},
	}.Encode(), nil, cookies)
	require.Equal(t, http.StatusOK, preface.Code, preface.Body.String())
	assert.Contains(t, preface.Body.String(), "1.pr.1", "a named-section Citation is shown")

	constructionLookup := perform(t, handler, http.MethodGet, "/la/vocabulary/concordance?"+url.Values{
		"as": {domain.ConcordanceMatchConstruction}, "term": {"genitive-absolute"}, "origin": {bookID}, "priority": {bookID},
	}.Encode(), nil, cookies)
	assert.NotContains(t, constructionLookup.Body.String(), "concordance-occurrence", "no Construction occurrence is shown for Latin")

	// 8. "I've read this" records a Latin Book too, without preparing it.
	reader := createAccount(t, ctx, store, "latin-loop-reader", "learner-password")
	readerCookies, readerCSRF := loginCookies(t, handler, reader.Username, "learner-password")
	read := perform(t, handler, http.MethodPost, classicalUnitReadURL(latinLoopUnitKey), url.Values{"csrf_token": {readerCSRF}}, readerCookies)
	require.Equal(t, http.StatusSeeOther, read.Code, read.Body.String())
	var readTag string
	require.NoError(t, pool.QueryRow(ctx, `SELECT language_tag FROM books WHERE owner_id=$1`, reader.ID).Scan(&readTag))
	assert.Equal(t, "la", readTag)
}
