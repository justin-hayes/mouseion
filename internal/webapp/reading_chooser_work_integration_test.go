//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readingChooserWorkHarness provisions the reviewed Work fixture and a handler
// whose real store holds the learner's Classical Books.
func readingChooserWorkHarness(t *testing.T) (context.Context, *persistence.PostgresStore, *Handler, domain.User) {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "reading-chooser-work-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, store.ReconcileClassicalManifest(ctx, workDefinitions()))
	learner := createAccount(t, ctx, store, "reading-chooser-work", "learner-password")
	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth:             authService,
		WebAuth:          webauth.New(authService, false, time.Hour),
		Store:            storeDependencies(store),
		Analysis:         analysis.NewService(store.Pool(), nil),
		AnalysisInsights: analysisinsights.NewService(store),
		PreparedDeck:     prepareddeck.NewService(store, nil),
		Classical:        workCatalog(t),
		SessionLifetime:  time.Hour,
	})
	return ctx, store, h, learner
}

// seedClassicalReadingEvidence attaches a current analysis to an existing
// Classical Book. known and total are token counts over one Known lemma.
func seedClassicalReadingEvidence(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner string, book domain.Book, unitKey string, known int64) {
	t.Helper()
	const total int64 = 100
	text := strings.Repeat("λόγος ", int(total))
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner, Language: classical.Language, SourceIdentifier: "reading-chooser-" + unitKey,
		Title: book.Title, MediaType: "application/epub+zip", Content: []byte(text), FullText: text,
	}, domain.ExtractedUnits{SchemaVersion: 1, Units: []domain.ExtractedUnit{{
		ID: domain.ClassicalUnitID(unitKey), Order: 0, SpineIndex: 0, ManifestID: unitKey,
		Text: text, EndOffset: uint64(len([]rune(text))), MediaType: "application/xhtml+xml", Linear: true,
	}}})
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner, book.ID, source.ID))
	require.NoError(t, store.PutArtifact(ctx, domain.NormalizedArtifact{
		ContentHash: source.ContentHash, Language: classical.Language, SchemaVersion: "1",
		NormalizationProfile: "reading-chooser", NormalizationVersion: "1", AnalyzerName: "reading-chooser-fixture", AnalyzerVersion: "1",
	}, []domain.SharedLemma{
		{Language: classical.Language, CanonicalLemma: "λόγος", UPOS: "NOUN", Frequency: known, Morphology: []byte(`{}`)},
		{Language: classical.Language, CanonicalLemma: "ἀνήρ", UPOS: "NOUN", Frequency: total - known, Morphology: []byte(`{}`)},
	}))
	corpus, err := store.PutCorpus(ctx, owner, source.ID, source.ContentHash)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analyzable_token_count=$2,distinct_lemma_count=$3,sentence_count=1,normalized_token_count=$2,empty_sentence_count=0,median_sentence_token_count=1,p90_sentence_token_count=1,long_sentence_count=0 WHERE owner_id=$1 AND id=$4`, owner, total, 2, corpus.ID)
	require.NoError(t, err)
	var snapshotID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, source.ID).Scan(&snapshotID))
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'reading-chooser-fixture','1',$5,'completed',now()) RETURNING id::text`, owner, source.ID, source.ContentRevisionID, snapshotID, unitKey).Scan(&runID))
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, runID, owner, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, owner, runID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner, book.ID, source.ID, runID)
	require.NoError(t, err)

	knownLemmas := make([]string, total)
	for i := range knownLemmas {
		if int64(i) < known {
			knownLemmas[i] = "λόγος"
		} else {
			knownLemmas[i] = "ἀνήρ"
		}
	}
	seedBrowseEvidenceTokensForLanguage(t, ctx, store, source, corpus, domain.ClassicalUnitID(unitKey), classical.Language, knownLemmas)
	buildBrowseProjection(t, ctx, store, book.ID)
}

// seedBrowseEvidenceTokensForLanguage mirrors the Browse fixture helper for a
// non-German language.
func seedBrowseEvidenceTokensForLanguage(t *testing.T, ctx context.Context, store *persistence.PostgresStore, source domain.SourceMaterial, corpus domain.Corpus, unitID, language string, lemmas []string) {
	t.Helper()
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, source.OwnerID, corpus.ID).Scan(&runID))
	for ordinal, lemma := range lemmas {
		start := int64(ordinal * 20)
		end := start + int64(len(lemma))
		_, err := store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, source.OwnerID, runID, corpus.ID, unitID, ordinal, lemma, start, end)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,0,$6,$6,$6,'NOUN','root',0,'{}',$7,$8)`, source.OwnerID, language, runID, corpus.ID, ordinal, lemma, start, end)
		require.NoError(t, err)
	}
}

func TestReadingChooserGroupsRealWorkBooksAndShowsPosition(t *testing.T) {
	ctx, store, h, learner := readingChooserWorkHarness(t)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))
	report, err := store.AddClassicalReadingUnits(ctx, learner.ID, []string{
		"homer-iliad-1", "homer-iliad-2", "homer-iliad-3", "homer-iliad-4", "homer-iliad-5", "homer-iliad-6",
		"xenophon-anabasis-1", "xenophon-hellenica-1",
	})
	require.NoError(t, err)
	byUnit := map[string]string{}
	for _, added := range report.Books {
		byUnit[added.ReadingUnitKey] = added.BookID
	}
	book := func(t *testing.T, unit string) domain.Book {
		t.Helper()
		detail, err := store.GetBookDetail(ctx, learner.ID, byUnit[unit])
		require.NoError(t, err)
		return detail.Book
	}
	_, err = store.PutKnownVocabulary(ctx, learner.ID, classical.Language, "λόγος", "NOUN")
	require.NoError(t, err)
	seedClassicalReadingEvidence(t, ctx, store, learner.ID, book(t, "homer-iliad-1"), "homer-iliad-1", 99)
	seedClassicalReadingEvidence(t, ctx, store, learner.ID, book(t, "homer-iliad-2"), "homer-iliad-2", 92)
	seedClassicalReadingEvidence(t, ctx, store, learner.ID, book(t, "homer-iliad-3"), "homer-iliad-3", 96)
	seedClassicalReadingEvidence(t, ctx, store, learner.ID, book(t, "xenophon-anabasis-1"), "xenophon-anabasis-1", 96)
	seedClassicalReadingEvidence(t, ctx, store, learner.ID, book(t, "xenophon-hellenica-1"), "xenophon-hellenica-1", 99)

	for _, unit := range []string{"homer-iliad-1", "homer-iliad-2", "homer-iliad-3", "homer-iliad-4", "homer-iliad-5", "xenophon-anabasis-1", "xenophon-hellenica-1"} {
		require.NoError(t, store.SetBookDisposition(ctx, learner.ID, byUnit[unit], domain.BookDispositionToRead))
	}
	_, err = store.SetBookHidden(ctx, learner.ID, byUnit["homer-iliad-5"], 0, true)
	require.NoError(t, err)

	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	page := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	body := page.Body.String()
	for _, want := range []string{
		"4 To Read Books, 3 assessed",
		"Known coverage 92.0%–99.0%",
		"<dt>99% or above</dt>",
		"<dt>Below 95%</dt>",
		"<dt>95% to below 97%</dt>",
		"<dt>Not assessed</dt>",
		"Iliad, Book 1, Known coverage 99.0%",
		"Iliad, Book 3, Known coverage 96.0%",
		"Book 1 of 24",
		"Book 4 of 24",
		`href="#reading-chooser-book-` + byUnit["homer-iliad-1"] + `"`,
		`action="/reading/books/` + byUnit["homer-iliad-4"] + `/reanalyze"`,
		"95% to below 97%",
		"Book 1 of 7",
	} {
		assert.Contains(t, body, want, "chooser missing %q", want)
	}
	for _, excluded := range []string{byUnit["homer-iliad-5"], byUnit["homer-iliad-6"]} {
		assert.NotContains(t, body, `href="#reading-chooser-book-`+excluded+`"`, "Read, hidden and non-To-Read Books get no cell")
	}
	assert.NotContains(t, body, "Book 1 of 1", "a whole-work unit shows no position")

	started := perform(t, h, http.MethodPost, "/reading/books/"+byUnit["homer-iliad-1"]+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code, started.Body.String())
	assert.NotContains(t, started.Header().Get("Location"), "error=")
	reading := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, reading.Code)
	assert.Contains(t, reading.Body.String(), "Book 1 of 24", "current Reading shows its reviewed position")

	switching := perform(t, h, http.MethodGet, "/reading/switch", nil, cookies)
	require.Equal(t, http.StatusOK, switching.Code)
	switchBody := switching.Body.String()
	assert.Contains(t, switchBody, `action="/reading/books/`+byUnit["homer-iliad-2"]+`/switch"`)
	assert.Contains(t, switchBody, `action="/reading/books/`+byUnit["homer-iliad-4"]+`/reanalyze"`)
	assert.NotContains(t, switchBody, `href="#reading-chooser-book-`+byUnit["homer-iliad-1"]+`"`, "the current Book gets no cell")
}
