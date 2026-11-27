package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/checked"
)

// ListCorpusSentences loads the requested persisted sentences in one query.
// The result is keyed by the sentence ordinal stored in selection references,
// so callers can join candidate evidence to its complete dependency context.
func (s *PostgresStore) ListCorpusSentences(ctx context.Context, owner, corpusID string, ordinals []int64) (map[int64]analyzer.Sentence, error) {
	return listCorpusSentences(ctx, s.pool, owner, corpusID, ordinals)
}

func listCorpusSentences(ctx context.Context, q sqlcgen.DBTX, owner, corpusID string, ordinals []int64) (map[int64]analyzer.Sentence, error) {
	result := make(map[int64]analyzer.Sentence, len(ordinals))
	if len(ordinals) == 0 {
		return result, nil
	}
	rows, err := sqlcgen.New(q).ListCorpusSentences(ctx, sqlcgen.ListCorpusSentencesParams{
		Owner: owner, Corpus: corpusID, SentenceOrdinals: ordinals,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		sentence, err := corpusSentenceFromRow(result, row)
		if err != nil {
			return nil, err
		}
		result[row.SentenceOrdinal] = sentence
	}
	return result, nil
}

// corpusSentenceFromRow folds one sentence/token row into the accumulating
// result, keeping the sentence's own start offset and each token's location.
func corpusSentenceFromRow(result map[int64]analyzer.Sentence, row sqlcgen.ListCorpusSentencesRow) (analyzer.Sentence, error) {
	sentence, ok := result[row.SentenceOrdinal]
	if !ok {
		sentenceStartOffset, err := checked.Uint64FromInt64(row.StartOffset)
		if err != nil {
			return analyzer.Sentence{}, fmt.Errorf("decode sentence start offset for sentence %d: %w", row.SentenceOrdinal, err)
		}
		sentence = analyzer.Sentence{Text: row.SentenceText, Tokens: make([]analyzer.Token, 0), Location: analyzer.SourceLocation{StartOffset: sentenceStartOffset}, Citations: append([]string(nil), row.Citations...)}
	}
	if row.TokenOrdinal < 0 {
		return sentence, nil
	}
	token, err := corpusTokenFromRow(row)
	if err != nil {
		return analyzer.Sentence{}, err
	}
	sentence.Tokens = append(sentence.Tokens, token)
	return sentence, nil
}

// corpusTokenFromRow decodes one persisted token, including the document
// absolute offsets the card-export anchor uses.
func corpusTokenFromRow(row sqlcgen.ListCorpusSentencesRow) (analyzer.Token, error) {
	head, err := checked.Uint32FromInt64(row.Head)
	if err != nil {
		return analyzer.Token{}, fmt.Errorf("decode head ordinal for sentence %d token %d: %w", row.SentenceOrdinal, row.TokenOrdinal, err)
	}
	startOffset, err := checked.Uint64FromInt64(row.TokenStartOffset)
	if err != nil {
		return analyzer.Token{}, fmt.Errorf("decode start offset for sentence %d token %d: %w", row.SentenceOrdinal, row.TokenOrdinal, err)
	}
	endOffset, err := checked.Uint64FromInt64(row.TokenEndOffset)
	if err != nil {
		return analyzer.Token{}, fmt.Errorf("decode end offset for sentence %d token %d: %w", row.SentenceOrdinal, row.TokenOrdinal, err)
	}
	morphology := make(map[string]string)
	if err := json.Unmarshal(row.Morphology, &morphology); err != nil {
		return analyzer.Token{}, fmt.Errorf("decode morphology for sentence %d token %d: %w", row.SentenceOrdinal, row.TokenOrdinal, err)
	}
	return analyzer.Token{
		Surface: row.Surface, RawLemma: row.RawLemma, CanonicalLemma: row.CanonicalLemma,
		UPOS: row.Upos, Dependency: row.Dependency, Head: head, Morphology: morphology,
		Location: analyzer.SourceLocation{StartOffset: startOffset, EndOffset: endOffset},
	}, nil
}
