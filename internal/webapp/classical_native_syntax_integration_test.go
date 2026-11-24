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

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClassicalPreparationStoresNativeAGDTSyntax is the acceptance seam for
// native syntax (A1): Add, To Read, the classical preparation worker, then the
// stored run, tokens and unprinted nodes.
func TestClassicalPreparationStoresNativeAGDTSyntax(t *testing.T) {
	ctx, store, pool, h, _, client, _, fixture := preparationHarnessForSource(t, preparationSource{
		file: "oga-src-syntax.conllu", firstStart: "1.1.1", firstEnd: "1.1.3", firstWords: 11, secondStart: "2.1.1", secondEnd: "2.1.1", secondWords: 1,
		interpretation: ancientgreek.InterpretationVersionNativeAGDT,
	}, nil)
	learner := createAccount(t, ctx, store, "classical-native-syntax", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")

	added := perform(t, h, http.MethodPost, "/catalogs/classical/works/synthetic-work/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code, added.Body.String())
	owned := booksByReadingUnit(t, store, learner.ID)
	firstID := owned[fixture.unitOneKey]
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
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "classical preparation client", client.Stop)
	waitForPreparedBooks(t, pool, learner.ID, 2)

	var runID, state, version string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT p.analysis_run_id::text, r.syntax_state, r.analyzer_version
		FROM book_current_analyses p JOIN analysis_runs r ON r.id = p.analysis_run_id
		WHERE p.owner_id=$1 AND p.book_id=$2`, learner.ID, firstID).Scan(&runID, &state, &version))

	t.Run("the run records native AGDT syntax under the new interpretation version", func(t *testing.T) {
		assert.Equal(t, "native_agdt", state)
		assert.True(t, strings.HasPrefix(version, ancientgreek.InterpretationVersionNativeAGDT), version)
	})

	type storedToken struct {
		surface, dependency, node string
		head                      int
	}
	rows, err := pool.Query(ctx, `
		SELECT surface, dependency, head, COALESCE(head_unprinted_node, '')
		FROM corpus_tokens WHERE analysis_run_id=$1 AND sentence_ordinal=$2 ORDER BY token_ordinal`, runID, 0)
	require.NoError(t, err)
	var first []storedToken
	for rows.Next() {
		var token storedToken
		require.NoError(t, rows.Scan(&token.surface, &token.dependency, &token.head, &token.node))
		first = append(first, token)
	}
	require.NoError(t, rows.Err())
	rows.Close()

	t.Run("relations are verbatim and heads follow the decided encoding", func(t *testing.T) {
		assert.Equal(t, []storedToken{
			{surface: "Κῦρος", dependency: "SBJ", head: 0, node: "2"},
			{surface: "ἄνδρας", dependency: "ATR", head: 1, node: "3"},
			{surface: ";", dependency: "AuxK", head: 2, node: "2"},
		}, first)
	})

	t.Run("unprinted nodes keep their relation and head", func(t *testing.T) {
		type storedNode struct {
			id, dependency string
			headToken      *int64
			headNode       string
		}
		nodeRows, err := pool.Query(ctx, `
			SELECT source_id, dependency, head_token_ordinal, COALESCE(head_unprinted_node, '')
			FROM corpus_unprinted_syntax_nodes WHERE analysis_run_id=$1 AND sentence_ordinal=0 ORDER BY source_id`, runID)
		require.NoError(t, err)
		defer nodeRows.Close()
		var nodes []storedNode
		for nodeRows.Next() {
			var node storedNode
			require.NoError(t, nodeRows.Scan(&node.id, &node.dependency, &node.headToken, &node.headNode))
			nodes = append(nodes, node)
		}
		require.NoError(t, nodeRows.Err())
		assert.Equal(t, []storedNode{{"2", "PRED", nil, ""}, {"3", "OBJ", nil, "2"}}, nodes)
	})

	t.Run("a comma root is self-headed and the Zeta defect is repaired", func(t *testing.T) {
		var rootDependency string
		var rootHead int
		require.NoError(t, pool.QueryRow(ctx, `SELECT dependency, head FROM corpus_tokens WHERE analysis_run_id=$1 AND sentence_ordinal=1 AND token_ordinal=0`, runID).Scan(&rootDependency, &rootHead))
		assert.Equal(t, "AuxK", rootDependency)
		assert.Zero(t, rootHead)
		var aux string
		require.NoError(t, pool.QueryRow(ctx, `SELECT dependency FROM corpus_tokens WHERE analysis_run_id=$1 AND surface='καί'`, runID).Scan(&aux))
		assert.Equal(t, []byte("AuxZ"), []byte(aux), "the stored relation is the Latin-script AuxZ")
	})

	t.Run("morphology maps VerbForm, Aspect, the perfect and common gender", func(t *testing.T) {
		morphology := func(surface string) map[string]string {
			var raw map[string]string
			require.NoError(t, pool.QueryRow(ctx, `SELECT morphology FROM corpus_tokens WHERE analysis_run_id=$1 AND surface=$2`, runID, surface).Scan(&raw))
			return raw
		}
		assert.Equal(t, "Perf", morphology("λέλυκε")["Aspect"])
		assert.Equal(t, "Past", morphology("λέλυκε")["Tense"])
		assert.Equal(t, "Imp", morphology("ἔλυε")["Aspect"])
		assert.Equal(t, map[string]string{"Tense": "Pres", "VerbForm": "Inf", "Voice": "Act"}, morphology("λύειν"))
		assert.Equal(t, "Part", morphology("λύων")["VerbForm"])
		assert.Equal(t, "Fem,Masc", morphology("παῖς")["Gender"])
	})

	t.Run("the Book still shows no syntax in Sentence study", func(t *testing.T) {
		var syntaxState string
		require.NoError(t, pool.QueryRow(ctx, `SELECT syntax_state FROM analysis_runs WHERE id=$1`, runID).Scan(&syntaxState))
		assert.True(t, sentenceStudySyntaxUnavailable(domain.SentenceStudy{SyntaxState: syntaxState}))
	})
}

// TestClassicalBookPreparedBeforeNativeSyntaxIsOfferedTheNewAnalysis proves the
// migration path: a Book prepared under oga-agdt-upos/5 is offered "A newer
// analysis is available" once the catalog's interpretation version moves to
// oga-agdt-syntax/1, and confirming it publishes a native_agdt run.
func TestClassicalBookPreparedBeforeNativeSyntaxIsOfferedTheNewAnalysis(t *testing.T) {
	ctx, store, pool, h, service, client, _, fixture := preparationHarness(t)
	learner := createAccount(t, ctx, store, "native-syntax-update", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalWork(t, h, cookies, csrf, "synthetic-work")
	moved := perform(t, h, http.MethodPost, "/library/works/synthetic-work/to-read", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code, moved.Body.String())
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "native syntax update client", client.Stop)
	waitForPreparedBooks(t, pool, learner.ID, 2)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))
	h.services.AnalysisUpdates = service

	owned := booksByReadingUnit(t, store, learner.ID)
	firstID, secondID := owned[fixture.unitOneKey], owned[fixture.unitTwoKey]
	require.NotEmpty(t, firstID)
	require.NotEmpty(t, secondID)
	assert.NotContains(t, readingGet(t, h, "/library", cookies), "A newer analysis is available", "a prepared Book starts current")
	oldRun := preparedRunID(t, pool, learner.ID, secondID)

	bumpInterpretation(t, pool, ancientgreek.InterpretationVersionNativeAGDT, fixture.unitOneKey, fixture.unitTwoKey)
	assert.Contains(t, readingGet(t, h, "/library", cookies), "A newer analysis is available", "every Greek Book is offered the new version")

	harness := workUpdateHarness{
		ctx: ctx, store: store, pool: pool, h: h, updates: service,
		learnerID: learner.ID, cookies: cookies, csrf: csrf, fixture: fixture,
		firstID: firstID, secondID: secondID,
	}
	form := workAnalysisUpdateForm(t, harness, harness.book(t, firstID), harness.book(t, secondID))
	submitted := perform(t, h, http.MethodPost, "/library/works/synthetic-work/analysis-update", form, cookies)
	require.Equal(t, http.StatusSeeOther, submitted.Code, submitted.Body.String())

	require.Eventually(t, func() bool {
		var count int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM classical_prepared_definitions
			WHERE owner_id=$1 AND interpretation_version=$2`, learner.ID, ancientgreek.InterpretationVersionNativeAGDT).Scan(&count); err != nil {
			return false
		}
		return count == 2
	}, 60*time.Second, 50*time.Millisecond, "both updates publish the new interpretation")

	newRun := preparedRunID(t, pool, learner.ID, secondID)
	assert.NotEqual(t, oldRun, newRun)
	var state, version string
	require.NoError(t, pool.QueryRow(ctx, `SELECT syntax_state, analyzer_version FROM analysis_runs WHERE id=$1`, newRun).Scan(&state, &version))
	assert.Equal(t, "native_agdt", state)
	assert.True(t, strings.HasPrefix(version, ancientgreek.InterpretationVersionNativeAGDT), version)
	assert.NotContains(t, readingGet(t, h, "/library", cookies), "A newer analysis is available", "the offer clears once the new analysis is published")
}
