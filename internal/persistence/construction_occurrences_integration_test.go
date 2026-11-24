//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const greekConstructionSentence = "ἔφη ὅτι ἔπαυσεν τοῦ λύειν"

// constructionRun seeds a run whose one sentence triggers an indirect
// statement (ὅτι, an unlabelled subtype) and an articular infinitive
// (λύειν, genitive), the first containing the second. It returns the owner,
// book, run, corpus and the unit id.
func constructionRun(t *testing.T, ctx context.Context, store *PostgresStore, name, syntaxState string) (owner domain.User, book domain.Book, runID, corpusID, unit string) {
	t.Helper()
	owner, err := store.CreateUser(ctx, name, false)
	require.NoError(t, err)
	var source domain.SourceMaterial
	book, source = createConcordanceBook(t, ctx, store, owner.ID, "Constructions "+name, name, false, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter", greekConstructionSentence, 0, uint64(len([]rune(greekConstructionSentence)))),
	})
	unit = "epub-unit-v1:0:chapter"
	finite := map[string]string{"VerbForm": "Fin", "Mood": "Ind"}
	article := map[string]string{"Gender": "Neut", "Number": "Sing", "Case": "Gen"}
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{
		{UnitID: unit, Ordinal: 0, Text: greekConstructionSentence, Start: 0, End: 26, Tokens: []concordanceToken{
			{Surface: "ἔφη", Lemma: "φημί", Upos: "VERB", Dependency: "PRED", Head: 0, Morphology: finite, Start: 0, End: 3},
			{Surface: "ὅτι", Lemma: "ὅτι", Upos: "SCONJ", Dependency: "AuxC", Head: 0, Start: 4, End: 7},
			{Surface: "ἔπαυσεν", Lemma: "παύω", Upos: "VERB", Dependency: "OBJ", Head: 1, Morphology: finite, Start: 8, End: 15},
			{Surface: "τοῦ", Lemma: "ὁ", Upos: "DET", Dependency: "ATR", Head: 4, Morphology: article, Start: 16, End: 19},
			{Surface: "λύειν", Lemma: "λύω", Upos: "VERB", Dependency: "OBJ", Head: 2, Morphology: map[string]string{"VerbForm": "Inf"}, Start: 20, End: 25},
		}},
	})
	corpusID = corpusIDForSource(t, ctx, store, owner.ID, source.ID)
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, owner.ID, corpusID).Scan(&runID))
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET syntax_state=$3 WHERE owner_id=$1 AND id=$2`, owner.ID, runID, syntaxState)
	require.NoError(t, err)
	return owner, book, runID, corpusID, unit
}

func buildConstructions(t *testing.T, ctx context.Context, store *PostgresStore, owner, run string) {
	t.Helper()
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, BuildConstructionOccurrencesTx(ctx, tx, owner, run))
	require.NoError(t, tx.Commit(ctx))
}

type storedConstruction struct {
	Construction, Subtype string
	Anchor                int64
	Extent                []int64
}

func storedConstructions(t *testing.T, ctx context.Context, store *PostgresStore, run string) []storedConstruction {
	t.Helper()
	rows, err := store.Pool().Query(ctx, `
		SELECT construction_id, COALESCE(subtype_id, ''), anchor_token_ordinal, extent_token_ordinals
		FROM construction_occurrences WHERE analysis_run_id=$1 ORDER BY extent_token_ordinals[1], anchor_token_ordinal`, run)
	require.NoError(t, err)
	defer rows.Close()
	var out []storedConstruction
	for rows.Next() {
		var row storedConstruction
		require.NoError(t, rows.Scan(&row.Construction, &row.Subtype, &row.Anchor, &row.Extent))
		out = append(out, row)
	}
	require.NoError(t, rows.Err())
	return out
}

func constructionMarkerCount(t *testing.T, ctx context.Context, store *PostgresStore, run string) (count int, version int) {
	t.Helper()
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*), COALESCE(max(detector_version), 0) FROM construction_run_markers WHERE analysis_run_id=$1`, run).Scan(&count, &version))
	return count, version
}

func TestBuildConstructionOccurrencesWritesRowsAndMarkerForNativeRuns(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, _, run, _, _ := constructionRun(t, ctx, store, "construction-native", "native_agdt")

	buildConstructions(t, ctx, store, owner.ID, run)

	t.Run("rows carry identifiers, the gated subtype, the anchor and the sorted extent", func(t *testing.T) {
		assert.Equal(t, []storedConstruction{
			{Construction: string(constructions.IndirectStatement), Subtype: "", Anchor: 1, Extent: []int64{1, 2, 3, 4}},
			{Construction: string(constructions.ArticularInfinitive), Subtype: string(constructions.SubtypeGenitive), Anchor: 4, Extent: []int64{3, 4}},
		}, storedConstructions(t, ctx, store, run))
	})
	t.Run("the marker records the current detector version", func(t *testing.T) {
		count, version := constructionMarkerCount(t, ctx, store, run)
		assert.Equal(t, 1, count)
		assert.Equal(t, constructions.AGDTDetectorVersion, version)
	})
	t.Run("the language is denormalised from the run", func(t *testing.T) {
		var language string
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT DISTINCT language FROM construction_occurrences WHERE analysis_run_id=$1`, run).Scan(&language))
		assert.Equal(t, "de", language, "the run's source material language")
	})
	t.Run("a rebuild replaces rows and keeps one marker", func(t *testing.T) {
		_, err := store.Pool().Exec(ctx, `DELETE FROM construction_occurrences WHERE analysis_run_id=$1 AND construction_id='articular_infinitive'`, run)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `UPDATE construction_run_markers SET detector_version=detector_version+1 WHERE analysis_run_id=$1`, run)
		require.NoError(t, err)
		buildConstructions(t, ctx, store, owner.ID, run)
		assert.Len(t, storedConstructions(t, ctx, store, run), 2)
		count, version := constructionMarkerCount(t, ctx, store, run)
		assert.Equal(t, 1, count)
		assert.Equal(t, constructions.AGDTDetectorVersion, version)
	})
}

func TestBuildConstructionOccurrencesIsANoOpForOtherSyntaxStates(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	for _, state := range []string{"absent", "ud"} {
		t.Run(state, func(t *testing.T) {
			owner, _, run, _, _ := constructionRun(t, ctx, store, "construction-"+state, state)
			buildConstructions(t, ctx, store, owner.ID, run)
			assert.Empty(t, storedConstructions(t, ctx, store, run))
			count, _ := constructionMarkerCount(t, ctx, store, run)
			assert.Zero(t, count)
		})
	}
}

func TestGetVocabularySentenceStudyReadsCurrentConstructions(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, book, run, corpus, unit := constructionRun(t, ctx, store, "construction-study", "native_agdt")
	read := func(t *testing.T) domain.SentenceStudy {
		t.Helper()
		study, err := store.GetVocabularySentenceStudy(ctx, owner.ID, book.ID, run, corpus, unit, 0, 1, "ὅτι")
		require.NoError(t, err)
		return study
	}

	t.Run("no marker means no constructions", func(t *testing.T) {
		assert.Empty(t, read(t).Constructions)
	})
	buildConstructions(t, ctx, store, owner.ID, run)
	t.Run("a current marker returns the sentence's occurrences", func(t *testing.T) {
		assert.Equal(t, []domain.SentenceStudyConstruction{
			{ConstructionID: "indirect_statement", Anchor: 1, Extent: []int{1, 2, 3, 4}},
			{ConstructionID: "articular_infinitive", SubtypeID: "genitive", Anchor: 4, Extent: []int{3, 4}},
		}, read(t).Constructions)
	})
	t.Run("a marker that is not current means no constructions", func(t *testing.T) {
		_, err := store.Pool().Exec(ctx, `UPDATE construction_run_markers SET detector_version=detector_version+1 WHERE analysis_run_id=$1`, run)
		require.NoError(t, err)
		assert.Empty(t, read(t).Constructions)
	})
}

func TestConstructionOccurrencesRejectMalformedExtents(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, _, run, corpus, _ := constructionRun(t, ctx, store, "construction-extent-check", "native_agdt")
	insert := func(anchor int, extent string) error {
		_, err := store.Pool().Exec(ctx, `
			INSERT INTO construction_occurrences(owner_id, corpus_id, analysis_run_id, sentence_ordinal, language,
				construction_id, anchor_token_ordinal, extent_token_ordinals)
			VALUES ($1,$2,$3,0,'grc','purpose',$4,`+extent+`)`, owner.ID, corpus, run, anchor)
		if err == nil {
			_, err = store.Pool().Exec(ctx, `DELETE FROM construction_occurrences WHERE analysis_run_id=$1`, run)
			require.NoError(t, err)
			return nil
		}
		return err
	}
	for name, test := range map[string]struct {
		anchor int
		extent string
	}{
		"unsorted":           {1, "ARRAY[2,1]::bigint[]"},
		"a NULL element":     {1, "ARRAY[0,NULL]::bigint[]"},
		"a missing anchor":   {1, "ARRAY[2,3]::bigint[]"},
		"an empty extent":    {1, "ARRAY[]::bigint[]"},
		"a negative ordinal": {0, "ARRAY[-1,0]::bigint[]"},
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			require.Error(t, insert(test.anchor, test.extent))
		})
	}
	t.Run("accepts a sorted extent containing the anchor", func(t *testing.T) {
		require.NoError(t, insert(1, "ARRAY[0,1,3]::bigint[]"))
	})
}
