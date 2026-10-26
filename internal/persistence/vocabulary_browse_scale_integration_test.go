//go:build integration

package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
)

// browseScaleRescanLimit is where a rescanned CTE stops being incidental. A
// nested loop that rescans a CTE of at least this many rows for at least this
// many outer rows does library-squared work: the plan that took Browse past
// its deadline on HERKUNFT (#2287) rescanned 71,284 rows for 8,445 outer rows.
const browseScaleRescanLimit = 1000

// browseScaleLibrary seeds one study language at roughly a deployed library's
// shape. Each Book holds a shared core of identities plus a window that
// overlaps the next Book's, so the library inventory is several times one
// Book's and both grow with the library.
type browseScaleLibrary struct {
	language      string
	books         int
	perBook       int
	core          int
	lemma         string // SQL over id yielding the canonical lemma
	discriminator string // SQL over id yielding the lexical discriminator
	headword      string // SQL over id yielding the citation headword, '' for none
}

var browseScaleLibraries = []browseScaleLibrary{{
	// The deployed German library: about 20 Books of about 9k identities.
	language: "de", books: 20, perBook: 9000, core: 3000,
	lemma:         `'wort'||lpad(id::text,6,'0')`,
	discriminator: `''`,
	headword:      `''`,
}, {
	// Ancient Greek: many short Books whose tokens carry citation headwords,
	// with numbered homographs beside a bare form of the same lemma.
	language: "grc", books: 40, perBook: 100, core: 30,
	lemma:         `CASE WHEN (id/3)%2=0 THEN 'δέω'||(id/3) ELSE 'λόγος'||lpad(id::text,6,'0') END`,
	discriminator: `CASE WHEN (id/3)%2=0 THEN (ARRAY['','1','2'])[id%3+1] ELSE '' END`,
	headword:      `CASE WHEN (id/3)%2=0 THEN 'δέω'||(id/3) WHEN id%4=0 THEN 'λόγος' ELSE '' END`,
}}

type browseScaleView struct {
	name  string
	query domain.VocabularyBrowseQuery
}

func browseScaleViews(current, other, prefix string) []browseScaleView {
	return []browseScaleView{
		{"current reading", domain.VocabularyBrowseQuery{CurrentBookID: current, Page: 1}},
		{"current reading all", domain.VocabularyBrowseQuery{CurrentBookID: current, IncludeAll: true, Page: 1}},
		{"library", domain.VocabularyBrowseQuery{Page: 1}},
		{"library NOUN", domain.VocabularyBrowseQuery{UPOS: []string{"NOUN"}, Page: 1}},
		{"one Book", domain.VocabularyBrowseQuery{BookIDs: []string{other}, Page: 1}},
		{"prefix", domain.VocabularyBrowseQuery{Prefix: prefix, Page: 1}},
	}
}

// TestVocabularyBrowseViewsAvoidLibraryScaleRescans explains every Browse view
// over a library-sized projection and fails when the executed plan rescans a
// CTE at library scale. It inspects actual loop and row counts, never time, so
// it is deterministic; the CTEs' own row estimates are what misled the
// planner, so estimates alone cannot show the problem.
//
//nolint:tparallel // testutil.Postgres may call t.Setenv, which a parallel test cannot.
func TestVocabularyBrowseViewsAvoidLibraryScaleRescans(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	for _, library := range browseScaleLibraries {
		t.Run(library.language, func(t *testing.T) {
			t.Parallel()
			owner, err := store.CreateUser(ctx, "browse-scale-"+library.language, false)
			require.NoError(t, err)
			books := seedBrowseScaleLibrary(t, ctx, store, owner.ID, library)
			var prefix string
			require.NoError(t, store.Pool().QueryRow(ctx, `SELECT left(canonical_lemma,length(canonical_lemma)-2) FROM vocabulary_browse_counts WHERE owner_id=$1 ORDER BY canonical_lemma LIMIT 1`, owner.ID).Scan(&prefix))
			for _, view := range browseScaleViews(books[0], books[1], prefix) {
				t.Run(view.name, func(t *testing.T) {
					t.Parallel()
					rescans := explainBrowseRescans(t, ctx, store, owner.ID, library.language, view.query)
					require.Empty(t, rescans, "Browse %s view rescans a CTE at library scale", view.name)
				})
			}
		})
	}
}

func seedBrowseScaleLibrary(t *testing.T, ctx context.Context, store *PostgresStore, owner string, library browseScaleLibrary) []string {
	t.Helper()
	window := library.perBook - library.core
	books := make([]string, 0, library.books)
	for i := range library.books {
		// Book i holds the core and the window starting i half-windows past it,
		// so each window identity is shared with one neighbouring Book.
		identities := fmt.Sprintf(`SELECT id, %s AS lemma, %s AS discriminator, %s AS headword,
				(ARRAY['NOUN','VERB','ADJ','ADV'])[id%%4+1] AS upos, 1+id%%50 AS occurrences
			FROM (SELECT CASE WHEN j<%[4]d THEN j ELSE %[4]d+%[5]d+(j-%[4]d) END AS id FROM generate_series(0,%[6]d-1) j) ids`,
			library.lemma, library.discriminator, library.headword, library.core, i*window/2, library.perBook)
		books = append(books, seedBrowseBook(t, ctx, store, owner, library.language, fmt.Sprintf("Scale %s %02d", library.language, i), identities))
	}
	// A deployed database has statistics for these tables; plan against them.
	_, err := store.Pool().Exec(ctx, `ANALYZE books, current_analysis_identity, vocabulary_browse_counts, vocabulary_browse_count_readiness, corpus_tokens`)
	require.NoError(t, err)
	return books
}

// seedBrowseBook creates one analyzed Book with a ready Browse projection.
// identities is SQL yielding id, lemma, discriminator, headword, upos and
// occurrences: an identity with occurrences is counted, and one with a
// headword carries it on an analysis token. An identity with a headword and
// no occurrences is a token Browse never counts.
func seedBrowseBook(t *testing.T, ctx context.Context, store *PostgresStore, owner, language, title, identities string) string {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{
		OwnerID: owner, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState: domain.LanguageChosen, LanguageTag: language,
	})
	require.NoError(t, err)
	suffix := "browse-" + book.ID
	units := []domain.ExtractedUnit{concordanceUnit(0, suffix, "text", 0, 4)}
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner, Language: language, SourceIdentifier: suffix,
		Title: book.Title, MediaType: "application/epub+zip", Content: []byte(suffix), FullText: "text",
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersionV1, Units: units})
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner, book.ID, source.ID))
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{{UnitID: units[0].ID, Text: "text", End: 4}})
	var run, corpus string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner, book.ID).Scan(&run, &corpus))
	_, err = store.Pool().Exec(ctx, `
		INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count,lexical_discriminator)
		SELECT $1,$2,$3,$4,$5,lemma,upos,occurrences,discriminator FROM (`+identities+`) i WHERE occurrences>0`,
		owner, book.ID, language, run, corpus)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `
		INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,start_offset,end_offset,lexical_discriminator,headword)
		SELECT $1,$2,$3,$4,0,id,lemma,lemma,lemma,upos,'root',0,0,4,discriminator,headword
		FROM (`+identities+`) i WHERE headword<>''`,
		owner, language, run, corpus)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_count_readiness(owner_id,book_id,language,analysis_run_id,corpus_id,builder_version) VALUES($1,$2,$3,$4,$5,$6)`,
		owner, book.ID, language, run, corpus, vocabularyBrowseCountBuilderVersion)
	require.NoError(t, err)
	return book.ID
}

// browsePlanNode is the part of an EXPLAIN (FORMAT JSON) node the rescan check
// reads. Row figures are per-loop averages.
type browsePlanNode struct {
	NodeType            string           `json:"Node Type"`
	ParentRelationship  string           `json:"Parent Relationship"`
	SubplanName         string           `json:"Subplan Name"`
	CTEName             string           `json:"CTE Name"`
	ActualLoops         float64          `json:"Actual Loops"`
	ActualRows          float64          `json:"Actual Rows"`
	RowsRemovedByFilter float64          `json:"Rows Removed by Filter"`
	Plans               []browsePlanNode `json:"Plans"`
}

// explainBrowseRescans executes the Browse query for view under EXPLAIN
// ANALYZE and describes every CTE scan the plan repeats at library scale.
//
// The planner cannot estimate the CTEs, so whether it joins two of them with a
// hash or a nested loop turns on estimates of a few rows either way; the same
// query can pick either plan from one ANALYZE to the next. The check therefore
// plans with hash and merge joins disabled, which is the plan those estimates
// make reachable, and passes only when that plan is safe too.
func explainBrowseRescans(t *testing.T, ctx context.Context, store *PostgresStore, owner, language string, view domain.VocabularyBrowseQuery) []browseRescan {
	t.Helper()
	view = normalizeVocabularyBrowseQuery(view)
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer rollbackIntegrationTx(t, ctx, tx)
	_, err = tx.Exec(ctx, `SET LOCAL enable_hashjoin=off; SET LOCAL enable_mergejoin=off`)
	require.NoError(t, err)
	var raw []byte
	require.NoError(t, tx.QueryRow(ctx, `EXPLAIN (ANALYZE, FORMAT JSON) `+vocabularyBrowseSQL, vocabularyBrowseArgs(owner, language, view)...).Scan(&raw))
	var explained []struct {
		Plan browsePlanNode `json:"Plan"`
	}
	require.NoError(t, json.Unmarshal(raw, &explained))
	require.Len(t, explained, 1)
	return browseRescans(explained[0].Plan, "")
}

// browseRescan is one CTE scan a plan repeats at library scale.
type browseRescan struct {
	driver             string
	cte                string
	outerRows, perScan float64
}

func (r browseRescan) String() string {
	return fmt.Sprintf("%s rescans CTE %q for %.0f outer rows, reading %.0f rows each time", r.driver, r.cte, r.outerRows, r.perScan)
}

// browseRescans walks node, remembering what repeats each subtree: the inner
// side of a nested loop or a correlated subplan. A CTE scan has no index, so
// each repeat reads the whole materialized result again.
func browseRescans(node browsePlanNode, driver string) []browseRescan {
	var found []browseRescan
	if node.NodeType == "CTE Scan" && driver != "" {
		perScan := node.ActualRows + node.RowsRemovedByFilter
		if node.ActualLoops >= browseScaleRescanLimit && perScan >= browseScaleRescanLimit {
			found = append(found, browseRescan{driver: driver, cte: node.CTEName, outerRows: node.ActualLoops, perScan: perScan})
		}
	}
	for _, child := range node.Plans {
		childDriver := driver
		switch {
		case child.ParentRelationship == "SubPlan":
			childDriver = "subplan " + child.SubplanName
		case node.NodeType == "Nested Loop" && child.ParentRelationship == "Inner":
			childDriver = "nested loop"
		}
		found = append(found, browseRescans(child, childDriver)...)
	}
	return found
}
