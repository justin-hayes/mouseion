package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
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

func (s *PostgresStore) ListSelectionCandidatesForBook(ctx context.Context, owner, bookID string) ([]domain.SelectionCandidate, error) {
	return listSelectionCandidatesForBook(ctx, s.pool, owner, bookID)
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

func (s *PostgresStore) ListSelectionCandidatesForCorpus(ctx context.Context, owner, corpusID string) ([]domain.SelectionCandidate, error) {
	return listSelectionCandidatesForCorpus(ctx, s.pool, owner, corpusID)
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

func (s *PostgresStore) GetCoverageEntryForBook(ctx context.Context, owner, bookID string, candidate domain.SelectionCandidate) (cardexport.Entry, error) {
	return getCoverageEntryForBook(ctx, s.pool, owner, bookID, candidate, true, true)
}

// GetPreparedCoverageEntryForBook loads only immutable render inputs. Exact
// enrichment is applied later from the preparation manifest.
func (s *PostgresStore) GetPreparedCoverageEntryForBook(ctx context.Context, owner, bookID string, candidate domain.SelectionCandidate) (cardexport.Entry, error) {
	return getCoverageEntryForBook(ctx, s.pool, owner, bookID, candidate, false, false)
}

func getCoverageEntryForBook(ctx context.Context, q sqlcgen.DBTX, owner, bookID string, candidate domain.SelectionCandidate, includeLegacyEnrichment, applyCandidateEvidence bool) (cardexport.Entry, error) {
	row, err := sqlcgen.New(q).GetCoverageEntryForBook(ctx, sqlcgen.GetCoverageEntryForBookParams{
		FirstEncounter: candidate.FirstEncounter, Owner: owner, Book: bookID,
		Corpus: candidate.CorpusID, Language: candidate.Language, CanonicalLemma: candidate.CanonicalLemma, Upos: candidate.UPOS,
	})
	if err = missing(err); err != nil {
		return cardexport.Entry{}, err
	}
	entry := cardexport.Entry{OwnerID: row.OwnerID, Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, Sentence: row.Sentence, Translation: row.Translation, TargetWord: row.TargetWord, Morphology: row.Morphology, SourceDocument: row.SourceDocument, Notes: row.Notes, FirstEncounter: row.FirstEncounter}
	if applyCandidateEvidence {
		if evidence, ok := cardexport.BestSentenceEvidence(candidate); ok {
			entry.Sentence = evidence.Sentence
			entry.TargetWord = evidence.Target
			entry.FirstEncounter = evidence.FirstEncounter
		}
	}
	if !includeLegacyEnrichment {
		return entry, nil
	}
	enrichmentRow, err := sqlcgen.New(q).GetLegacyEnrichmentForSentence(ctx, sqlcgen.GetLegacyEnrichmentForSentenceParams{
		Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, SentenceHash: enrichment.SentenceHash(entry.Sentence),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return entry, nil
	}
	if err != nil {
		return entry, err
	}
	entry.Translation = enrichmentRow.Translation
	entry.SentenceTranslation = enrichmentRow.SentenceTranslation
	entry.SentenceTranslationTarget = enrichmentRow.SentenceTranslationTarget
	return entry, nil
}

func (s *PostgresStore) GetCoverageEntryForCorpus(ctx context.Context, owner, corpusID string, candidate domain.SelectionCandidate) (cardexport.Entry, error) {
	return getCoverageEntryForCorpus(ctx, s.pool, owner, corpusID, candidate, true, true)
}

// GetPreparedCoverageEntryForCorpus loads only immutable render inputs. It
// deliberately performs no broad provider/version cache lookup.
func (s *PostgresStore) GetPreparedCoverageEntryForCorpus(ctx context.Context, owner, corpusID string, candidate domain.SelectionCandidate) (cardexport.Entry, error) {
	return getCoverageEntryForCorpus(ctx, s.pool, owner, corpusID, candidate, false, false)
}

func getCoverageEntryForCorpus(ctx context.Context, q sqlcgen.DBTX, owner, corpusID string, candidate domain.SelectionCandidate, includeLegacyEnrichment, applyCandidateEvidence bool) (cardexport.Entry, error) {
	row, err := sqlcgen.New(q).GetCoverageEntryForCorpus(ctx, sqlcgen.GetCoverageEntryForCorpusParams{
		FirstEncounter: candidate.FirstEncounter, Owner: owner, Corpus: corpusID,
		Language: candidate.Language, CanonicalLemma: candidate.CanonicalLemma, Upos: candidate.UPOS,
	})
	if err = missing(err); err != nil {
		return cardexport.Entry{}, err
	}
	entry := cardexport.Entry{OwnerID: row.OwnerID, Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, Sentence: row.Sentence, Translation: row.Translation, TargetWord: row.TargetWord, Morphology: row.Morphology, SourceDocument: row.SourceDocument, Notes: row.Notes, FirstEncounter: row.FirstEncounter}
	if applyCandidateEvidence {
		if evidence, ok := cardexport.BestSentenceEvidence(candidate); ok {
			entry.Sentence = evidence.Sentence
			entry.TargetWord = evidence.Target
			entry.FirstEncounter = evidence.FirstEncounter
		}
	}
	if !includeLegacyEnrichment {
		return entry, nil
	}
	enrichmentRow, err := sqlcgen.New(q).GetLegacyEnrichmentForSentence(ctx, sqlcgen.GetLegacyEnrichmentForSentenceParams{
		Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, SentenceHash: enrichment.SentenceHash(entry.Sentence),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return entry, nil
	}
	if err != nil {
		return entry, err
	}
	entry.Translation = enrichmentRow.Translation
	entry.SentenceTranslation = enrichmentRow.SentenceTranslation
	entry.SentenceTranslationTarget = enrichmentRow.SentenceTranslationTarget
	return entry, nil
}

func (s *PostgresStore) GetCorpusForAnalysis(ctx context.Context, owner, analysisRunID string) (domain.Corpus, error) {
	row, err := s.queries().GetCorpusForAnalysis(ctx, sqlcgen.GetCorpusForAnalysisParams{OwnerID: owner, ID: analysisRunID})
	if err != nil {
		return domain.Corpus{}, missing(err)
	}
	return domain.Corpus{ID: row.CID, OwnerID: row.COwnerID, SourceMaterialID: row.CSourceMaterialID, ArtifactHash: row.ArtifactHash, AnalysisRunID: row.AnalysisRunID, Status: row.Status, CreatedAt: row.CreatedAt}, nil
}

func (s *PostgresStore) RecordGenerated(ctx context.Context, owner, deckName string, entry cardexport.RenderInput, note cardexport.Note) error {
	return s.recordGenerated(ctx, owner, "", deckName, entry, note)
}

// RecordGeneratedForBook atomically persists the exported card and its first
// generated-vocabulary provenance for the source material that produced it.
func (s *PostgresStore) RecordGeneratedForBook(ctx context.Context, owner, bookID, deckName string, entry cardexport.RenderInput, note cardexport.Note) error {
	return s.recordGenerated(ctx, owner, bookID, deckName, entry, note)
}

func (s *PostgresStore) recordGenerated(ctx context.Context, owner, bookID, deckName string, entry cardexport.RenderInput, note cardexport.Note) error {
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		state, err := q.GetVocabularyStateForUpdate(ctx, sqlcgen.GetVocabularyStateForUpdateParams{OwnerID: owner, Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if state != "candidate" && state != "accepted" && state != "generated" {
			return fmt.Errorf("cardexport: vocabulary state is %s", state)
		}
		deck, err := q.PutDeck(ctx, sqlcgen.PutDeckParams{OwnerID: owner, Language: entry.Language, Name: deckName})
		if err != nil {
			return err
		}
		if err = q.PutGeneratedCard(ctx, sqlcgen.PutGeneratedCardParams{OwnerID: owner, DeckID: deck.ID, DedupKey: note.Key, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, Front: note.Text, Back: note.BackExtra}); err != nil {
			return err
		}
		if err = q.PutGeneratedVocabulary(ctx, sqlcgen.PutGeneratedVocabularyParams{OwnerID: owner, Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, FirstDeckID: deck.ID, FirstSourceMaterialID: nullableUUIDArg(bookID)}); err != nil {
			return err
		}
		// Keep the generated lifecycle state as legacy bookkeeping for compatibility.
		// Cross-book exclusion is driven exclusively by generated_vocabulary above.
		if state != "generated" {
			if err = q.SetVocabularyStateGenerated(ctx, sqlcgen.SetVocabularyStateGeneratedParams{Owner: owner, Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS}); err != nil {
				return err
			}
			details, _ := json.Marshal(map[string]string{"language": entry.Language, "canonical_lemma": entry.CanonicalLemma, "upos": entry.UPOS, "from": state, "to": "generated"})
			completed := time.Now().UTC()
			return q.InsertProcessingHistoryWithoutCorpus(ctx, sqlcgen.InsertProcessingHistoryWithoutCorpusParams{OwnerID: owner, Operation: "vocabulary.transition", Status: "completed", Details: details, CompletedAt: pgtype.Timestamptz{Time: completed, Valid: true}})
		}
		return nil
	})
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
