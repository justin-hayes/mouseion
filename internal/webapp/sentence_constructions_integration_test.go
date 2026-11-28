//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// constructionRows is every Construction occurrence of a run, one line per
// occurrence, so a test can compare the rows before and after another change.
func constructionRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, run string) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT sentence_ordinal || ':' || construction_id || ':' || COALESCE(subtype_id, '-') || ':' ||
		       anchor_token_ordinal || ':' || extent_token_ordinals::text
		FROM construction_occurrences WHERE analysis_run_id=$1
		ORDER BY sentence_ordinal, anchor_token_ordinal, construction_id`, run)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		out = append(out, line)
	}
	require.NoError(t, rows.Err())
	return out
}

// TestAncientGreekSentenceStudyShowsConstructions is the HTTP acceptance seam
// for Construction occurrences: importing a native_agdt fixture writes the rows
// and a current marker in the publishing transaction, Sentence study lists the
// constructions with the note's wording and the Smyth link, and a lemma
// correction and an exclusion leave the rows exactly as they were (A9, A11,
// A12).
func TestAncientGreekSentenceStudyShowsConstructions(t *testing.T) {
	ctx, store, pool, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-constructions", "learner-password")
	registration, err := registerGreekDefinition(t, ctx, store, learner.ID, greekTestDefinition())
	require.NoError(t, err)
	definition, sourcePath := fixtureImportFrom(t, "oga-src-constructions.conllu")
	_, err = importGreekSource(ctx, greekImportRequest{
		OwnerID: learner.ID, BookID: registration.BookID, SourcePath: sourcePath, Definition: definition,
	}, analysis.NewGreekImportWriter(store))
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))
	cookies, _ := loginCookies(t, h, learner.Username, "learner-password")

	var run, corpus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text, corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, learner.ID, registration.BookID).Scan(&run, &corpus))
	unit := domain.ClassicalUnitID(definition.Key)
	studyOf := func(t *testing.T, sentence, target int64) string {
		t.Helper()
		query := url.Values{
			"book": {registration.BookID}, "run": {run}, "corpus": {corpus}, "unit": {unit},
			"sentence": {strconv.FormatInt(sentence, 10)}, "target": {strconv.FormatInt(target, 10)},
		}
		response := perform(t, h, http.MethodGet, "/grc/vocabulary/concordance/sentence?"+query.Encode(), nil, cookies)
		require.Equal(t, http.StatusOK, response.Code, "body=%s", response.Body.String())
		return response.Body.String()
	}

	rows := constructionRows(t, ctx, pool, run)
	t.Run("the publishing transaction wrote the rows and a current marker", func(t *testing.T) {
		assert.Equal(t, []string{
			"0:indirect_statement:-:1:{1,2,3,4}",
			"0:articular_infinitive:genitive:4:{3,4}",
			"1:indirect_statement:infinitive:2:{1,2}",
		}, rows)
		var version int
		require.NoError(t, pool.QueryRow(ctx, `SELECT detector_version FROM construction_run_markers WHERE analysis_run_id=$1`, run).Scan(&version))
		assert.Equal(t, constructions.AGDTDetectorVersion, version)
	})

	t.Run("Sentence study lists the constructions in order with the note's wording", func(t *testing.T) {
		body := studyOf(t, 0, 1)
		assert.Contains(t, body, `<h3 id="sentence-study-constructions-heading">Constructions</h3>`)
		assert.Contains(t, body, `<p class="sentence-study-construction-name"><strong>Indirect statement</strong></p>`, "an unlabelled subtype shows no label")
		assert.Contains(t, body, `<p class="sentence-study-construction-name"><strong>Articular infinitive (genitive)</strong></p>`, "a labelled subtype shows in parentheses")
		assert.Less(t, strings.Index(body, "Indirect statement"), strings.Index(body, "Articular infinitive (genitive)"))
		assert.Contains(t, body, `<p class="sentence-study-construction-nesting">Inside construction 1.</p>`)
		assert.Contains(t, body, `Signalled by <em>ὅτι</em>. The identified target signals this construction.`)
		assert.Contains(t, body, `Signalled by <em>λύειν</em>.`)
		assert.Contains(t, body, constructions.Notes[constructions.IndirectStatement].Definition)
		assert.Contains(t, body, `<p class="sr-only">Words: ὅτι ἔπαυσεν τοῦ λύειν</p>`)
		assert.Contains(t, body, `class="sentence-study-construction-extract" aria-hidden="true"`)
	})
	t.Run("the citation links to Smyth with the exact URL and no opener", func(t *testing.T) {
		body := studyOf(t, 0, 1)
		note := constructions.Notes[constructions.ArticularInfinitive]
		assert.Contains(t, body, `<a href="`+constructions.CitationURL(note.Citation)+`" target="_blank" rel="noopener noreferrer">`+constructions.CitationText(note.Citation)+`</a>`)
		assert.Equal(t, "https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0007:smythp=2025", constructions.CitationURL(note.Citation))
	})
	t.Run("a target inside an extent is named, and a labelled infinitive statement shows its label", func(t *testing.T) {
		assert.Contains(t, studyOf(t, 0, 4), `Contains the identified target.`)
		assert.Contains(t, studyOf(t, 1, 2), `<strong>Indirect statement (infinitive)</strong>`)
	})
	t.Run("a sentence with no occurrence shows no region", func(t *testing.T) {
		body := studyOf(t, 2, 0)
		assert.NotContains(t, body, "Constructions")
		assert.NotContains(t, body, "sentence-study-constructions")
	})

	t.Run("a marker behind the detector version hides the region without a notice", func(t *testing.T) {
		_, err := pool.Exec(ctx, `UPDATE construction_run_markers SET detector_version=detector_version+1 WHERE analysis_run_id=$1`, run)
		require.NoError(t, err)
		body := studyOf(t, 0, 1)
		assert.NotContains(t, body, "Constructions")
		assert.NotContains(t, body, "updating")
		_, err = pool.Exec(ctx, `UPDATE construction_run_markers SET detector_version=$2 WHERE analysis_run_id=$1`, run, constructions.AGDTDetectorVersion)
		require.NoError(t, err)
		assert.Contains(t, studyOf(t, 0, 1), "Constructions")
	})

	t.Run("a lemma correction and an exclusion leave the rows unchanged", func(t *testing.T) {
		occurrences, err := store.ListLemmaReviewOccurrences(ctx, learner.ID, registration.BookID, "λύειν")
		require.NoError(t, err)
		require.Len(t, occurrences, 2)
		decide := func(decision, lemma, occurrence string) {
			t.Helper()
			confirmLemmaDecisionForm(t, h, learner.ID, registration.BookID, "λύειν", decision, lemma, occurrence)
		}
		decide("correct", "λέγω", occurrences[0].ID())
		decide("exclude", "", occurrences[1].ID())
		after, err := store.ListLemmaReviewOccurrences(ctx, learner.ID, registration.BookID, "λύειν")
		require.NoError(t, err)
		require.Len(t, after, 2)
		assert.True(t, after[0].CorrectedLemma != "" || after[1].CorrectedLemma != "", "a correction was recorded")
		assert.True(t, after[0].Excluded || after[1].Excluded, "an exclusion was recorded")
		assert.Equal(t, rows, constructionRows(t, ctx, pool, run))
		assert.Contains(t, studyOf(t, 0, 1), `Signalled by <em>λύειν</em>.`, "the page still names the same constructions")
	})
}

// TestImportedAbsentRunGetsNoConstructions keeps A9's other half: a run that
// stores no syntax has no rows and no marker, so its page shows no region.
func TestImportedAbsentRunGetsNoConstructions(t *testing.T) {
	ctx, store, pool, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-constructions-absent", "learner-password")
	registration, err := registerGreekDefinition(t, ctx, store, learner.ID, greekTestDefinition())
	require.NoError(t, err)
	definition, sourcePath := fixtureImportFrom(t, "oga-src-constructions.conllu")
	legacy := &legacyGreekWriter{next: analysis.NewGreekImportWriter(store)}
	legacy.legacy.Store(true)
	_, err = importGreekSource(ctx, greekImportRequest{
		OwnerID: learner.ID, BookID: registration.BookID, SourcePath: sourcePath, Definition: definition,
	}, legacy)
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))
	cookies, _ := loginCookies(t, h, learner.Username, "learner-password")

	var run, corpus, state string
	require.NoError(t, pool.QueryRow(ctx, `SELECT c.analysis_run_id::text, c.corpus_id::text, r.syntax_state FROM current_analysis_identity c JOIN analysis_runs r ON r.id=c.analysis_run_id WHERE c.owner_id=$1 AND c.book_id=$2`, learner.ID, registration.BookID).Scan(&run, &corpus, &state))
	require.Equal(t, "absent", state)
	assert.Empty(t, constructionRows(t, ctx, pool, run))
	var markers int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM construction_run_markers WHERE analysis_run_id=$1`, run).Scan(&markers))
	assert.Zero(t, markers)

	query := url.Values{"book": {registration.BookID}, "run": {run}, "corpus": {corpus}, "unit": {domain.ClassicalUnitID(definition.Key)}, "sentence": {"0"}, "target": {"1"}}
	response := perform(t, h, http.MethodGet, "/grc/vocabulary/concordance/sentence?"+query.Encode(), nil, cookies)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.NotContains(t, response.Body.String(), "Constructions")
}

// prepareConstructionBooks adds the synthetic Work, moves both Books to To Read
// and returns the first Book's id with the harness handles. The caller starts
// the River client.
func prepareConstructionBooks(t *testing.T) (ctx context.Context, pool *pgxpool.Pool, h *Handler, client *river.Client[pgx.Tx], learner domain.User, firstID string, cookies []*http.Cookie) {
	t.Helper()
	ctx, store, pool, h, _, client, _, fixture := preparationHarnessForSource(t, preparationSource{
		file: "oga-src-constructions.conllu", firstStart: "1.1.1", firstEnd: "1.1.3", firstWords: 8, secondStart: "2.1.1", secondEnd: "2.1.1", secondWords: 1,
		interpretation: ancientgreek.InterpretationVersionNativeAGDT,
	}, nil)
	learner = createAccount(t, ctx, store, "classical-constructions", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalTestUnits(t, h, cookies, csrf, "synthetic-work-1", "synthetic-work-2")
	owned := booksByReadingUnit(t, store, learner.ID)
	firstID = owned[fixture.unitOneKey]
	require.NotEmpty(t, firstID)
	for _, id := range []string{firstID, owned[fixture.unitTwoKey]} {
		detail, err := store.GetBookDetail(ctx, learner.ID, id)
		require.NoError(t, err)
		moved := perform(t, h, http.MethodPost, "/library/books/"+id+"/to-read", url.Values{
			"csrf_token":        {csrf},
			"expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
		}, cookies)
		require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())
	}
	return ctx, pool, h, client, learner, firstID, cookies
}

// TestClassicalPreparationBuildsConstructionsBeforePublishing is the
// preparation-transaction seam (A9): Add, To Read, the worker; the published
// run has its occurrence rows and a current marker, and Sentence study over HTTP
// lists them.
func TestClassicalPreparationBuildsConstructionsBeforePublishing(t *testing.T) {
	ctx, pool, h, client, learner, firstID, cookies := prepareConstructionBooks(t)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "classical preparation client", client.Stop)
	waitForPreparedBooks(t, pool, learner.ID, 2)

	var run, corpus, unit string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT p.analysis_run_id::text, r.corpus_id::text, s.unit_id
		FROM book_current_analyses p JOIN analysis_runs r ON r.id = p.analysis_run_id
		JOIN corpus_sentences s ON s.analysis_run_id = r.id AND s.sentence_ordinal = 0
		WHERE p.owner_id=$1 AND p.book_id=$2`, learner.ID, firstID).Scan(&run, &corpus, &unit))

	assert.Equal(t, []string{
		"0:indirect_statement:-:1:{1,2,3,4}",
		"0:articular_infinitive:genitive:4:{3,4}",
		"1:indirect_statement:infinitive:2:{1,2}",
	}, constructionRows(t, ctx, pool, run))
	var version int
	require.NoError(t, pool.QueryRow(ctx, `SELECT detector_version FROM construction_run_markers WHERE analysis_run_id=$1`, run).Scan(&version))
	assert.Equal(t, constructions.AGDTDetectorVersion, version)

	query := url.Values{"book": {firstID}, "run": {run}, "corpus": {corpus}, "unit": {unit}, "sentence": {"0"}, "target": {"1"}}
	response := perform(t, h, http.MethodGet, "/grc/vocabulary/concordance/sentence?"+query.Encode(), nil, cookies)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	body := response.Body.String()
	assert.Contains(t, body, `<h3 id="sentence-study-constructions-heading">Constructions</h3>`)
	assert.Contains(t, body, `<strong>Articular infinitive (genitive)</strong>`)
	assert.Contains(t, body, `Inside construction 1.`)
}

// TestClassicalPreparationPublishesNothingWhenTheConstructionBuildFails proves
// the build shares the publishing transaction (A9): a database failure while
// the marker is written rolls the whole run back, so no analysis, corpus rows,
// occurrences or current-analysis pointer survive.
func TestClassicalPreparationPublishesNothingWhenTheConstructionBuildFails(t *testing.T) {
	ctx, pool, _, client, learner, _, _ := prepareConstructionBooks(t)
	_, err := pool.Exec(ctx, `
		CREATE FUNCTION fail_construction_marker() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'construction build failed'; END $$;
		CREATE TRIGGER fail_construction_marker BEFORE INSERT ON construction_run_markers
		FOR EACH ROW EXECUTE FUNCTION fail_construction_marker();`)
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "classical preparation client", client.Stop)

	require.Eventually(t, func() bool {
		var attempted int
		return pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND attempt >= 1`, analysis.ClassicalPreparationQueue).Scan(&attempted) == nil && attempted >= 1
	}, 60*time.Second, 50*time.Millisecond, "the preparation job never ran")
	// Wait for the first attempt to finish failing, not merely start.
	require.Eventually(t, func() bool {
		var failing int
		return pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND attempt >= 1 AND state <> 'running'`, analysis.ClassicalPreparationQueue).Scan(&failing) == nil && failing >= 1
	}, 60*time.Second, 50*time.Millisecond, "the preparation attempt never finished")

	var failure string
	require.NoError(t, pool.QueryRow(ctx, `SELECT errors::text FROM river_job WHERE kind=$1 AND attempt >= 1 LIMIT 1`, analysis.ClassicalPreparationQueue).Scan(&failure))
	assert.Contains(t, failure, "construction build failed", "the build, not another step, failed the attempt")

	count := func(query string) int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, query, learner.ID).Scan(&n))
		return n
	}
	assert.Zero(t, count(`SELECT count(*) FROM book_current_analyses WHERE owner_id=$1`), "nothing published")
	assert.Zero(t, count(`SELECT count(*) FROM analysis_runs WHERE owner_id=$1 AND state='completed'`), "no completed run survives")
	assert.Zero(t, count(`SELECT count(*) FROM corpus_tokens WHERE owner_id=$1`), "no token rows survive")
	assert.Zero(t, count(`SELECT count(*) FROM construction_occurrences WHERE owner_id=$1`), "no occurrence rows survive")
	assert.Zero(t, count(`SELECT count(*) FROM construction_run_markers WHERE owner_id=$1`), "no marker survives")
}
