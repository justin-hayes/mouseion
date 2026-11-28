//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
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

	addClassicalTestUnits(t, h, cookies, csrf, "synthetic-work-1", "synthetic-work-2")
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

	t.Run("Sentence study shows Word relations read from the stored syntax", func(t *testing.T) {
		var corpusID, unitID string
		require.NoError(t, pool.QueryRow(ctx, `SELECT r.corpus_id::text, s.unit_id FROM analysis_runs r
			JOIN corpus_sentences s ON s.analysis_run_id=r.id WHERE r.owner_id=$1 AND r.id=$2 AND s.sentence_ordinal=0`,
			learner.ID, runID).Scan(&corpusID, &unitID))
		query := url.Values{
			"book": {firstID}, "run": {runID}, "corpus": {corpusID}, "unit": {unitID},
			"sentence": {"0"}, "target": {"1"},
		}
		response := perform(t, h, http.MethodGet, "/grc/corpus/concordance/sentence?"+query.Encode(), nil, cookies)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		body := response.Body.String()
		assert.Contains(t, body, `<summary>Word relations</summary>`)
		assert.NotContains(t, body, "Syntax unavailable")
		assert.Contains(t, body, `<td>Κῦρος</td><td>subject</td><td>‹an unprinted word› (top of the sentence)</td>`)
		assert.Contains(t, body, `<strong>ἄνδρας<span class="sr-only"> (identified target)</span></strong></td><td>describes a noun</td><td>‹an unprinted word› (top of the sentence)</td>`, "a chain of unprinted heads ending at the root")
		assert.NotContains(t, body, `<td>;</td>`, "punctuation has no row")
		assert.Equal(t, 1, strings.Count(body, `id="sentence-study-unprinted-note"`))
	})

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

	t.Run("Sentence study offers Word relations rather than Syntax unavailable", func(t *testing.T) {
		var syntaxState string
		require.NoError(t, pool.QueryRow(ctx, `SELECT syntax_state FROM analysis_runs WHERE id=$1`, runID).Scan(&syntaxState))
		assert.Equal(t, "native_agdt", syntaxState)
		assert.False(t, sentenceStudySyntaxUnavailable(domain.SentenceStudy{SyntaxState: syntaxState}))
	})
}

// legacyGreekWriter publishes the run the interpretation before native syntax
// (oga-agdt-upos/5, syntax absent) wrote: the placeholder relation, every token
// its own head, no unprinted nodes and the earlier identity. Once legacy is
// false it passes requests through unchanged.
type legacyGreekWriter struct {
	next   ancientgreek.ImportWriter
	legacy atomic.Bool
}

func (w *legacyGreekWriter) WriteImportedAnalysis(ctx context.Context, request ancientgreek.ImportWriteRequest) (ancientgreek.ImportWriteResult, error) {
	if w.legacy.Load() {
		request.SyntaxState = ancientgreek.SyntaxStateAbsent
		request.AnalyzerVersion = strings.Replace(request.AnalyzerVersion, "oga-agdt-feats/2", "oga-agdt-feats/1", 1)
		request.ConfigIdentity = ancientgreek.ImportConfigIdentity(request.Definition, ancientgreek.SyntaxStateAbsent)
		sentences := make([]analyzer.Sentence, len(request.Result.Sentences))
		for i, sentence := range request.Result.Sentences {
			sentence.UnprintedNodes = nil
			sentence.Tokens = append([]analyzer.Token(nil), sentence.Tokens...)
			for j := range sentence.Tokens {
				sentence.Tokens[j].Dependency, sentence.Tokens[j].Head, sentence.Tokens[j].HeadUnprintedNode = "dep", 0, ""
			}
			sentences[i] = sentence
		}
		request.Result.Sentences = sentences
	}
	return w.next.WriteImportedAnalysis(ctx, request)
}

// TestClassicalBookPreparedBeforeNativeSyntaxIsOfferedTheNewAnalysis proves the
// migration path from a real old run: a Book prepared under oga-agdt-upos/5
// (syntax absent) with a correction and an exclusion is offered "A newer
// analysis is available" once the catalog's interpretation version moves to
// oga-agdt-syntax/1; confirming publishes a native_agdt run and carries both
// decisions onto its identical tokens.
func TestClassicalBookPreparedBeforeNativeSyntaxIsOfferedTheNewAnalysis(t *testing.T) {
	legacy := &legacyGreekWriter{}
	legacy.legacy.Store(true)
	ctx, store, pool, h, service, client, _, fixture := preparationHarnessForSource(t, preparationSource{
		file: "oga-src-book1.conllu", firstStart: "1.1.1", firstEnd: "1.1.2", firstWords: 4, secondStart: "2.1.1", secondEnd: "2.1.1", secondWords: 2,
		wrapWriter: func(next ancientgreek.ImportWriter) ancientgreek.ImportWriter { legacy.next = next; return legacy },
	}, nil)
	learner := createAccount(t, ctx, store, "native-syntax-update", "learner-password")
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	addClassicalTestUnits(t, h, cookies, csrf, "synthetic-work-1", "synthetic-work-2")
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "native syntax update client", client.Stop)
	waitForPreparedBooks(t, pool, learner.ID, 2)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, classical.Language))
	h.services.AnalysisUpdates = service
	legacy.legacy.Store(false)

	owned := booksByReadingUnit(t, store, learner.ID)
	firstID, secondID := owned[fixture.unitOneKey], owned[fixture.unitTwoKey]
	require.NotEmpty(t, firstID)
	require.NotEmpty(t, secondID)
	oldRun := preparedRunID(t, pool, learner.ID, firstID)

	var oldState, oldVersion string
	require.NoError(t, pool.QueryRow(ctx, `SELECT syntax_state, analyzer_version FROM analysis_runs WHERE id=$1`, oldRun).Scan(&oldState, &oldVersion))
	assert.Equal(t, "absent", oldState)
	assert.True(t, strings.HasPrefix(oldVersion, "oga-agdt-upos/5"), oldVersion)
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM corpus_unprinted_syntax_nodes WHERE analysis_run_id=$1`, oldRun))
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM corpus_tokens WHERE analysis_run_id=$1 AND dependency <> 'dep'`, oldRun))
	assert.NotContains(t, readingGet(t, h, "/reading", cookies), "A newer analysis is available", "a prepared Book starts current")

	// A correction on the first token and an exclusion on the second, both real.
	var corpusID, documentID string
	var starts, ends [2]int
	require.NoError(t, pool.QueryRow(ctx, `SELECT r.corpus_id::text, s.unit_id FROM analysis_runs r
		JOIN corpus_sentences s ON s.owner_id=r.owner_id AND s.analysis_run_id=r.id
		WHERE r.owner_id=$1 AND r.id=$2 AND s.sentence_ordinal=0`, learner.ID, oldRun).Scan(&corpusID, &documentID))
	for ordinal := range 2 {
		require.NoError(t, pool.QueryRow(ctx, `SELECT start_offset, end_offset FROM corpus_tokens
			WHERE analysis_run_id=$1 AND sentence_ordinal=0 AND token_ordinal=$2`, oldRun, ordinal).Scan(&starts[ordinal], &ends[ordinal]))
	}
	_, err := pool.Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded)
		VALUES($1,$2,$3,$4,$5,$6,$7,'ἄγω','ancient-greek','1',false)`, learner.ID, firstID, corpusID, oldRun, documentID, starts[0], ends[0])
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,excluded)
		VALUES($1,$2,$3,$4,$5,$6,$7,true)`, learner.ID, firstID, corpusID, oldRun, documentID, starts[1], ends[1])
	require.NoError(t, err)

	bumpInterpretation(t, pool, ancientgreek.InterpretationVersionNativeAGDT, fixture.unitOneKey, fixture.unitTwoKey)
	assert.Contains(t, readingGet(t, h, "/reading", cookies), "A newer analysis is available", "every Greek Book is offered the new version")

	harness := workUpdateHarness{
		ctx: ctx, store: store, pool: pool, h: h, updates: service,
		learnerID: learner.ID, cookies: cookies, csrf: csrf, fixture: fixture,
		firstID: firstID, secondID: secondID,
	}
	for _, id := range []string{firstID, secondID} {
		submitted := submitBookAnalysisUpdate(t, harness, harness.book(t, id))
		require.Equal(t, http.StatusSeeOther, submitted.Code, submitted.Body.String())
	}

	require.Eventually(t, func() bool {
		var count int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM classical_prepared_definitions
			WHERE owner_id=$1 AND interpretation_version=$2`, learner.ID, ancientgreek.InterpretationVersionNativeAGDT).Scan(&count); err != nil {
			return false
		}
		return count == 2
	}, 60*time.Second, 50*time.Millisecond, "both updates publish the new interpretation")

	newRun := preparedRunID(t, pool, learner.ID, firstID)
	assert.NotEqual(t, oldRun, newRun)
	var published, state, version string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, learner.ID, firstID).Scan(&published))
	assert.Equal(t, newRun, published, "the update publishes the new analysis")
	require.NoError(t, pool.QueryRow(ctx, `SELECT syntax_state, analyzer_version FROM analysis_runs WHERE id=$1`, newRun).Scan(&state, &version))
	assert.Equal(t, "native_agdt", state)
	assert.True(t, strings.HasPrefix(version, ancientgreek.InterpretationVersionNativeAGDT), version)
	assert.Positive(t, countRows(t, pool, `SELECT count(*) FROM corpus_unprinted_syntax_nodes WHERE analysis_run_id=$1`, newRun), "the new run stores the unprinted nodes")

	t.Run("the correction and the exclusion carry onto the identical tokens", func(t *testing.T) {
		assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3 AND NOT excluded AND canonical_lemma='ἄγω' AND start_offset=$4`, learner.ID, firstID, newRun, starts[0]))
		assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3 AND excluded AND start_offset=$4`, learner.ID, firstID, newRun, starts[1]))
		assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM analysis_decision_carry_overs WHERE owner_id=$1 AND analysis_run_id=$2 AND carried=2 AND total=2`, learner.ID, newRun))
		assert.Equal(t, 2, countRows(t, pool, `SELECT count(*) FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3`, learner.ID, firstID, oldRun), "the earlier decisions stay frozen on the old run")
	})
	t.Run("the offer clears once the new analysis is published", func(t *testing.T) {
		assert.NotContains(t, readingGet(t, h, "/reading", cookies), "A newer analysis is available")
	})
}
