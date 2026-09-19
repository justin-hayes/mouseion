package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// PreparedDeckCandidateFacts contains only persisted facts for one selected
// candidate. Selection and presentation policy remain outside persistence.
type PreparedDeckCandidateFacts struct {
	Candidate domain.SelectionCandidate
	Entry     cardexport.Entry
	Sentences map[int64]analyzer.Sentence
}

// PreparedDeckInputFacts is the transaction-consistent read model used by the
// prepared-deck planner. Vocabulary slices are intentionally unclassified;
// the planner owns recurring-vocabulary selection and exclusion.
type PreparedDeckInputFacts struct {
	DeckName   string
	Candidates []domain.SelectionCandidate
	Known      []domain.KnownVocabulary
	Generated  []domain.GeneratedVocabulary // historical provenance, never an eligibility exclusion
	Reserved   []domain.DeckPreparationVocabulary
}

// LoadPreparedDeckInputFactsTx reads candidate and vocabulary facts through
// the supplied transaction. Render and sentence facts are loaded only after
// the planner has applied recurring-vocabulary selection.
func (s *PostgresStore) LoadPreparedDeckInputFactsTx(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation) (PreparedDeckInputFacts, error) {
	if s == nil || tx == nil || preparation.OwnerID == "" || preparation.SourceMaterialID == "" {
		return PreparedDeckInputFacts{}, ErrInvalidTransition
	}
	q := sqlcgen.New(tx)
	deckName, _, candidates, err := preparedDeckScopeTx(ctx, tx, q, preparation)
	if err != nil {
		return PreparedDeckInputFacts{}, err
	}

	languages := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		languages[canonicalization.NormalizeLanguage(candidate.Language)] = struct{}{}
	}
	result := PreparedDeckInputFacts{DeckName: deckName, Candidates: candidates}
	for language := range languages {
		known, knownErr := listKnownVocabulary(ctx, tx, preparation.OwnerID, language)
		if knownErr != nil {
			return PreparedDeckInputFacts{}, knownErr
		}
		generated, generatedErr := listUnattachedGeneratedVocabulary(ctx, tx, preparation.OwnerID, language)
		if generatedErr != nil {
			return PreparedDeckInputFacts{}, generatedErr
		}
		reserved, reservedErr := listReservedVocabulary(ctx, tx, preparation.OwnerID, language)
		if reservedErr != nil {
			return PreparedDeckInputFacts{}, reservedErr
		}
		result.Known = append(result.Known, known...)
		result.Generated = append(result.Generated, generated...)
		result.Reserved = append(result.Reserved, reserved...)
	}
	return result, nil
}

// LoadPreparedDeckCandidateFactsTx loads render and sentence facts for the
// selected candidates and rejects partially available persisted sentences.
func (s *PostgresStore) LoadPreparedDeckCandidateFactsTx(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation, selected []domain.SelectionCandidate) ([]PreparedDeckCandidateFacts, error) {
	if s == nil || tx == nil || preparation.OwnerID == "" || preparation.SourceMaterialID == "" {
		return nil, ErrInvalidTransition
	}
	if len(selected) == 0 {
		return nil, nil
	}
	q := sqlcgen.New(tx)
	_, corpusID, err := preparedDeckCoverageScopeTx(ctx, q, preparation)
	if err != nil {
		return nil, err
	}

	ordinalsByCorpus := make(map[string]map[int64]struct{})
	for _, candidate := range selected {
		var refs []struct {
			SentenceIndex int `json:"sentence_index"`
		}
		if json.Unmarshal(candidate.SentenceReferences, &refs) != nil {
			continue
		}
		ordinals := ordinalsByCorpus[candidate.CorpusID]
		if ordinals == nil {
			ordinals = make(map[int64]struct{})
			ordinalsByCorpus[candidate.CorpusID] = ordinals
		}
		for _, ref := range refs {
			if ref.SentenceIndex >= 0 {
				ordinals[int64(ref.SentenceIndex)] = struct{}{}
			}
		}
	}
	sentencesByCorpus := make(map[string]map[int64]analyzer.Sentence, len(ordinalsByCorpus))
	for id, ordinalSet := range ordinalsByCorpus {
		ordinals := make([]int64, 0, len(ordinalSet))
		for ordinal := range ordinalSet {
			ordinals = append(ordinals, ordinal)
		}
		sentences, sentenceErr := listCorpusSentences(ctx, tx, preparation.OwnerID, id, ordinals)
		if sentenceErr != nil {
			return nil, fmt.Errorf("list persisted corpus sentences for %s: %w", id, sentenceErr)
		}
		if len(sentences) > 0 {
			for ordinal := range ordinalSet {
				if _, ok := sentences[ordinal]; !ok {
					return nil, fmt.Errorf("list persisted corpus sentences for %s: missing sentence ordinal %d", id, ordinal)
				}
			}
			sentencesByCorpus[id] = sentences
		}
	}

	result := make([]PreparedDeckCandidateFacts, 0, len(selected))
	for _, candidate := range selected {
		var entry cardexport.Entry
		if corpusID != "" {
			entry, err = getCoverageEntryForCorpus(ctx, tx, preparation.OwnerID, corpusID, candidate)
		} else {
			entry, err = getCoverageEntryForBook(ctx, tx, preparation.OwnerID, preparation.SourceMaterialID, candidate)
		}
		if err != nil {
			return nil, fmt.Errorf("load coverage facts for %s: %w", candidateIdentity(candidate), err)
		}
		result = append(result, PreparedDeckCandidateFacts{Candidate: candidate, Entry: entry, Sentences: sentencesByCorpus[candidate.CorpusID]})
	}
	return result, nil
}

func preparedDeckScopeTx(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, preparation domain.DeckPreparation) (string, string, []domain.SelectionCandidate, error) {
	deckName, corpusID, err := preparedDeckCoverageScopeTx(ctx, q, preparation)
	if err != nil {
		return "", "", nil, err
	}
	var candidates []domain.SelectionCandidate
	if corpusID == "" {
		candidates, err = listSelectionCandidatesForBook(ctx, tx, preparation.OwnerID, preparation.SourceMaterialID)
	} else {
		candidates, err = listSelectionCandidatesForCorpus(ctx, tx, preparation.OwnerID, corpusID)
	}
	return deckName, corpusID, candidates, err
}

func preparedDeckCoverageScopeTx(ctx context.Context, q *sqlcgen.Queries, preparation domain.DeckPreparation) (string, string, error) {
	source, err := q.GetSourceMaterial(ctx, sqlcgen.GetSourceMaterialParams{OwnerID: preparation.OwnerID, ID: preparation.SourceMaterialID})
	if err != nil {
		return "", "", missing(err)
	}
	if preparation.AnalysisRunID == "" {
		return source.Title, "", nil
	}
	corpus, err := q.GetCorpusForAnalysis(ctx, sqlcgen.GetCorpusForAnalysisParams{OwnerID: preparation.OwnerID, ID: preparation.AnalysisRunID})
	if err != nil {
		return "", "", missing(err)
	}
	if corpus.CSourceMaterialID != preparation.SourceMaterialID {
		return "", "", ErrNotFound
	}
	return source.Title, corpus.CID, nil
}

func listKnownVocabulary(ctx context.Context, q sqlcgen.DBTX, owner, language string) ([]domain.KnownVocabulary, error) {
	rows, err := sqlcgen.New(q).ListKnownVocabulary(ctx, sqlcgen.ListKnownVocabularyParams{OwnerID: owner, Language: canonicalization.NormalizeLanguage(language)})
	if err != nil {
		return nil, err
	}
	result := make([]domain.KnownVocabulary, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.KnownVocabulary{ID: row.KvID, OwnerID: row.KvOwnerID, Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, Provenance: row.Provenance, CreatedAt: row.CreatedAt})
	}
	return result, nil
}

func listUnattachedGeneratedVocabulary(ctx context.Context, q sqlcgen.DBTX, owner, language string) ([]domain.GeneratedVocabulary, error) {
	rows, err := sqlcgen.New(q).ListUnattachedGeneratedVocabulary(ctx, sqlcgen.ListUnattachedGeneratedVocabularyParams{OwnerID: owner, Language: canonicalization.NormalizeLanguage(language)})
	if err != nil {
		return nil, err
	}
	result := make([]domain.GeneratedVocabulary, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.GeneratedVocabulary{OwnerID: row.OwnerID, Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, FirstDeckID: row.FirstDeckID, FirstSourceMaterialID: uuidStringPtr(row.FirstSourceMaterialID), FirstGeneratedAt: row.FirstGeneratedAt})
	}
	return result, nil
}

func listReservedVocabulary(ctx context.Context, q sqlcgen.DBTX, owner, language string) ([]domain.DeckPreparationVocabulary, error) {
	rows, err := sqlcgen.New(q).ListReservedDeckVocabulary(ctx, sqlcgen.ListReservedDeckVocabularyParams{Owner: owner, Language: canonicalization.NormalizeLanguage(language)})
	if err != nil {
		return nil, err
	}
	result := make([]domain.DeckPreparationVocabulary, 0, len(rows))
	for _, row := range rows {
		result = append(result, deckPreparationVocabularyFromModel(row))
	}
	return result, nil
}

func candidateIdentity(candidate domain.SelectionCandidate) string {
	return candidate.CorpusID + "\x00" + candidate.Language + "\x00" + candidate.CanonicalLemma + "\x00" + candidate.UPOS
}
