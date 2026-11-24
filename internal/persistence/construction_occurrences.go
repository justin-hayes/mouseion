package persistence

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/syntax/agdt"
)

// syntaxStateNativeAGDT is the analysis_runs.syntax_state of a run whose
// stored dependency and head are the source's own AGDT syntax.
const syntaxStateNativeAGDT = "native_agdt"

// BuildConstructionOccurrencesTx rebuilds a run's Construction occurrences from
// its stored syntax, inside the caller's transaction. It reads the run's printed
// tokens and unprinted syntax nodes, adapts each sentence, runs the detectors
// through the shipping gate, replaces the run's rows and upserts the marker with
// the current detector version. A run that is not native_agdt is left untouched:
// it gets no rows and no marker. Lemma corrections and Occurrence exclusions are
// never inputs; the detectors read only the raw lemma and stored morphology.
func BuildConstructionOccurrencesTx(ctx context.Context, tx pgx.Tx, ownerID, runID string) error {
	q := sqlcgen.New(tx)
	source, err := q.GetConstructionRunSource(ctx, sqlcgen.GetConstructionRunSourceParams{Owner: ownerID, AnalysisRun: runID})
	if err != nil {
		return err
	}
	if source.SyntaxState != syntaxStateNativeAGDT {
		return nil
	}
	tokenRows, err := q.ListConstructionTokens(ctx, sqlcgen.ListConstructionTokensParams{Owner: ownerID, AnalysisRun: runID})
	if err != nil {
		return err
	}
	nodeRows, err := q.ListConstructionUnprintedNodes(ctx, sqlcgen.ListConstructionUnprintedNodesParams{Owner: ownerID, AnalysisRun: runID})
	if err != nil {
		return err
	}
	if err := q.DeleteConstructionOccurrences(ctx, sqlcgen.DeleteConstructionOccurrencesParams{Owner: ownerID, AnalysisRun: runID}); err != nil {
		return err
	}
	for _, sentence := range constructionSentences(tokenRows, nodeRows) {
		tree := agdt.Adapt(sentence.tokens, sentence.nodes)
		for _, occurrence := range constructions.Detect(tree) {
			if err := q.InsertConstructionOccurrence(ctx, insertConstructionParams(ownerID, runID, source, sentence.ordinal, occurrence)); err != nil {
				return err
			}
		}
	}
	return q.UpsertConstructionRunMarker(ctx, sqlcgen.UpsertConstructionRunMarkerParams{
		Owner: ownerID, Corpus: source.CorpusID, AnalysisRun: runID, DetectorVersion: constructions.AGDTDetectorVersion,
	})
}

func insertConstructionParams(ownerID, runID string, source sqlcgen.GetConstructionRunSourceRow, sentence int64, occurrence constructions.Occurrence) sqlcgen.InsertConstructionOccurrenceParams {
	extent := make([]int64, len(occurrence.Extent))
	for i, ordinal := range occurrence.Extent {
		extent[i] = int64(ordinal)
	}
	return sqlcgen.InsertConstructionOccurrenceParams{
		Owner: ownerID, Corpus: source.CorpusID, AnalysisRun: runID, SentenceOrdinal: sentence,
		Language: source.Language, ConstructionID: string(occurrence.Construction),
		SubtypeID:          pgtype.Text{String: string(occurrence.Subtype), Valid: occurrence.Subtype != ""},
		AnchorTokenOrdinal: int64(occurrence.Anchor), ExtentTokenOrdinals: extent,
	}
}

// constructionSentence is one stored sentence's tokens and unprinted nodes.
type constructionSentence struct {
	ordinal int64
	tokens  []agdt.Token
	nodes   []agdt.UnprintedNode
}

// constructionSentences groups a run's stored tokens and unprinted nodes by
// sentence, in sentence order. A sentence with no tokens has nothing to detect.
func constructionSentences(tokenRows []sqlcgen.ListConstructionTokensRow, nodeRows []sqlcgen.ListConstructionUnprintedNodesRow) []constructionSentence {
	bySentence := map[int64]*constructionSentence{}
	sentence := func(ordinal int64) *constructionSentence {
		if bySentence[ordinal] == nil {
			bySentence[ordinal] = &constructionSentence{ordinal: ordinal}
		}
		return bySentence[ordinal]
	}
	for _, row := range tokenRows {
		morphology := map[string]string{}
		if len(row.Morphology) > 0 {
			if err := json.Unmarshal(row.Morphology, &morphology); err != nil {
				// A value the analyzer's string map cannot hold contributes no
				// features, as in Sentence study.
				morphology = map[string]string{}
			}
		}
		s := sentence(row.SentenceOrdinal)
		s.tokens = append(s.tokens, agdt.Token{
			Ordinal: row.TokenOrdinal, Surface: row.Surface, Lemma: row.RawLemma, UPOS: row.Upos,
			Morphology: morphology, Relation: row.Dependency, Head: row.Head, HeadUnprintedNode: row.HeadUnprintedNode,
		})
	}
	for _, row := range nodeRows {
		node := agdt.UnprintedNode{SourceID: row.SourceID, Relation: row.Dependency, HeadUnprintedNode: row.HeadUnprintedNode}
		if row.HeadTokenOrdinal.Valid {
			ordinal := row.HeadTokenOrdinal.Int64
			node.HeadToken = &ordinal
		}
		s := sentence(row.SentenceOrdinal)
		s.nodes = append(s.nodes, node)
	}
	out := make([]constructionSentence, 0, len(bySentence))
	for _, s := range bySentence {
		if len(s.tokens) > 0 {
			out = append(out, *s)
		}
	}
	slices.SortFunc(out, func(a, b constructionSentence) int { return int(a.ordinal - b.ordinal) })
	return out
}

// sentenceStudyConstructions reads one sentence's Construction occurrences when
// the run's marker carries the current detector version. No marker, or an older
// one, reads nothing.
func (s *PostgresStore) sentenceStudyConstructions(ctx context.Context, owner, run, corpus string, sentence int64) ([]domain.SentenceStudyConstruction, error) {
	rows, err := s.queries().GetSentenceStudyConstructions(ctx, sqlcgen.GetSentenceStudyConstructionsParams{
		DetectorVersion: constructions.AGDTDetectorVersion,
		Owner:           owner, AnalysisRun: run, Corpus: corpus, SentenceOrdinal: sentence,
	})
	if err != nil {
		return nil, err
	}
	var out []domain.SentenceStudyConstruction
	for _, row := range rows {
		extent := make([]int, len(row.ExtentTokenOrdinals))
		for i, ordinal := range row.ExtentTokenOrdinals {
			extent[i] = int(ordinal)
		}
		out = append(out, domain.SentenceStudyConstruction{
			ConstructionID: row.ConstructionID, SubtypeID: row.SubtypeID,
			Anchor: int(row.AnchorTokenOrdinal), Extent: extent,
		})
	}
	return out, nil
}
