package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
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
		sentence, ok := result[row.SentenceOrdinal]
		if !ok {
			sentence = analyzer.Sentence{Text: row.SentenceText, Tokens: make([]analyzer.Token, 0)}
		}
		if row.TokenOrdinal >= 0 {
			morphology := make(map[string]string)
			if err := json.Unmarshal(row.Morphology, &morphology); err != nil {
				return nil, fmt.Errorf("decode morphology for sentence %d token %d: %w", row.SentenceOrdinal, row.TokenOrdinal, err)
			}
			sentence.Tokens = append(sentence.Tokens, analyzer.Token{
				Surface: row.Surface, RawLemma: row.RawLemma, CanonicalLemma: row.CanonicalLemma,
				UPOS: row.Upos, Dependency: row.Dependency, Head: uint32(row.Head), Morphology: morphology,
			})
		}
		result[row.SentenceOrdinal] = sentence
	}
	return result, nil
}

func listSelectionCandidatesForBook(ctx context.Context, q sqlcgen.DBTX, owner, bookID string) ([]domain.SelectionCandidate, error) {
	rows, err := sqlcgen.New(q).ListSelectionCandidatesForBook(ctx, sqlcgen.ListSelectionCandidatesForBookParams{Owner: owner, Book: bookID})
	if err != nil {
		return nil, err
	}
	var candidates []domain.SelectionCandidate
	for _, row := range rows {
		candidates = append(candidates, selectionCandidateFromFields(row.OwnerID, row.CorpusID, row.Language, row.CanonicalLemma, row.Upos, row.OccurrenceCount, row.ObservedForms, row.EligibleSentenceRefs, row.Provenance, row.SelectedAt, row.FirstEncounter))
	}
	return candidates, nil
}

func listSelectionCandidatesForCorpus(ctx context.Context, q sqlcgen.DBTX, owner, corpusID string) ([]domain.SelectionCandidate, error) {
	rows, err := sqlcgen.New(q).ListSelectionCandidatesForCorpus(ctx, sqlcgen.ListSelectionCandidatesForCorpusParams{Owner: owner, Corpus: corpusID})
	if err != nil {
		return nil, err
	}
	var candidates []domain.SelectionCandidate
	for _, row := range rows {
		candidates = append(candidates, selectionCandidateFromFields(row.OwnerID, row.CorpusID, row.Language, row.CanonicalLemma, row.Upos, row.OccurrenceCount, row.ObservedForms, row.EligibleSentenceRefs, row.Provenance, row.SelectedAt, row.FirstEncounter))
	}
	return candidates, nil
}

func getCoverageEntryForBook(ctx context.Context, q sqlcgen.DBTX, owner, bookID string, candidate domain.SelectionCandidate) (cardexport.Entry, error) {
	row, err := sqlcgen.New(q).GetCoverageEntryForBook(ctx, sqlcgen.GetCoverageEntryForBookParams{
		FirstEncounter: candidate.FirstEncounter, Owner: owner, Book: bookID,
		Corpus: candidate.CorpusID, Language: candidate.Language, CanonicalLemma: candidate.CanonicalLemma, Upos: candidate.UPOS,
	})
	if err = missing(err); err != nil {
		return cardexport.Entry{}, err
	}
	entry := cardexport.Entry{OwnerID: row.OwnerID, Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, Sentence: row.Sentence, Translation: row.Translation, TargetWord: row.TargetWord, Morphology: row.Morphology, SourceDocument: row.SourceDocument, Notes: row.Notes, FirstEncounter: row.FirstEncounter}
	return entry, nil
}

func getCoverageEntryForCorpus(ctx context.Context, q sqlcgen.DBTX, owner, corpusID string, candidate domain.SelectionCandidate) (cardexport.Entry, error) {
	row, err := sqlcgen.New(q).GetCoverageEntryForCorpus(ctx, sqlcgen.GetCoverageEntryForCorpusParams{
		FirstEncounter: candidate.FirstEncounter, Owner: owner, Corpus: corpusID,
		Language: candidate.Language, CanonicalLemma: candidate.CanonicalLemma, Upos: candidate.UPOS,
	})
	if err = missing(err); err != nil {
		return cardexport.Entry{}, err
	}
	entry := cardexport.Entry{OwnerID: row.OwnerID, Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, Sentence: row.Sentence, Translation: row.Translation, TargetWord: row.TargetWord, Morphology: row.Morphology, SourceDocument: row.SourceDocument, Notes: row.Notes, FirstEncounter: row.FirstEncounter}
	return entry, nil
}

// RecordGeneratedVocabulary records first provenance for an owner-scoped
// vocabulary identity. Repeated records intentionally preserve the first row.
func (s *PostgresStore) RecordGeneratedVocabulary(ctx context.Context, value domain.GeneratedVocabulary) (domain.GeneratedVocabulary, error) {
	firstSourceMaterialID := ""
	if value.FirstSourceMaterialID != nil {
		firstSourceMaterialID = *value.FirstSourceMaterialID
	}
	if err := s.queries().PutGeneratedVocabulary(ctx, sqlcgen.PutGeneratedVocabularyParams{OwnerID: value.OwnerID, Language: value.Language, CanonicalLemma: value.CanonicalLemma, Upos: value.UPOS, FirstDeckID: value.FirstDeckID, FirstSourceMaterialID: nullableUUIDArg(firstSourceMaterialID)}); err != nil {
		return domain.GeneratedVocabulary{}, err
	}
	return s.getGeneratedVocabulary(ctx, value.OwnerID, value.Language, value.CanonicalLemma, value.UPOS)
}

func (s *PostgresStore) getGeneratedVocabulary(ctx context.Context, owner, language, lemma, upos string) (value domain.GeneratedVocabulary, err error) {
	row, err := s.queries().GetGeneratedVocabulary(ctx, sqlcgen.GetGeneratedVocabularyParams{OwnerID: owner, Language: language, CanonicalLemma: lemma, Upos: upos})
	if err != nil {
		return value, missing(err)
	}
	return generatedVocabularyFromFields(row.OwnerID, row.Language, row.CanonicalLemma, row.Upos, row.FirstDeckID, row.FirstSourceMaterialID, row.FirstGeneratedAt), nil
}

func (s *PostgresStore) ListGeneratedVocabulary(ctx context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
	rows, err := s.queries().ListGeneratedVocabulary(ctx, sqlcgen.ListGeneratedVocabularyParams{OwnerID: owner, Language: language})
	if err != nil {
		return nil, err
	}
	var result []domain.GeneratedVocabulary
	for _, row := range rows {
		result = append(result, generatedVocabularyFromFields(row.OwnerID, row.Language, row.CanonicalLemma, row.Upos, row.FirstDeckID, row.FirstSourceMaterialID, row.FirstGeneratedAt))
	}
	return result, nil
}
