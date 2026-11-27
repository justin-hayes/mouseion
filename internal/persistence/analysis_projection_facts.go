package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// This file holds the persisted analysis facts the Current reading freeze reads
// (ADR 0106): the token evidence and lemma corrections of one analysis run, the
// sentences its candidates reference, and the learner's vocabulary state.

func loadAnalysisProjectionFactsTx(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, sourceMaterialID, analysisRunID, corpusID string) (analyzer.Result, []domain.OccurrenceLemmaCorrection, error) {
	rows, err := q.ListAnalysisTokenEvidence(ctx, sqlcgen.ListAnalysisTokenEvidenceParams{Owner: owner, Corpus: corpusID, AnalysisRun: uuidArg(analysisRunID)})
	if err != nil {
		return analyzer.Result{}, nil, err
	}
	bookID, err := sourceMaterialBookID(ctx, tx, owner, sourceMaterialID)
	if err != nil {
		return analyzer.Result{}, nil, err
	}
	correctionRows, err := listOccurrenceLemmaCorrectionsTx(ctx, q, owner, analysisRunID, corpusID, bookID)
	if err != nil {
		return analyzer.Result{}, nil, err
	}
	analysis, err := analysisFromTokenEvidence(rows)
	if err != nil {
		return analyzer.Result{}, nil, err
	}
	corrections, err := occurrenceLemmaCorrections(correctionRows)
	if err != nil {
		return analyzer.Result{}, nil, err
	}
	return analysis, corrections, nil
}

// sourceMaterialBookID returns the Book a preparation's source is linked
// to, or nil for an unlinked legacy source.
func sourceMaterialBookID(ctx context.Context, tx pgx.Tx, owner, sourceMaterialID string) (*string, error) {
	var bookID *string
	if err := tx.QueryRow(ctx, `SELECT book_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, sourceMaterialID).Scan(&bookID); err != nil {
		return nil, err
	}
	return bookID, nil
}

// listOccurrenceLemmaCorrectionsTx loads the corrections recorded against the
// source's Book. An unlinked source has no Book to correct.
func listOccurrenceLemmaCorrectionsTx(ctx context.Context, q *sqlcgen.Queries, owner, analysisRunID, corpusID string, bookID *string) ([]sqlcgen.ListOccurrenceLemmaCorrectionsRow, error) {
	if bookID == nil {
		return nil, nil
	}
	rows, err := q.ListOccurrenceLemmaCorrections(ctx, sqlcgen.ListOccurrenceLemmaCorrectionsParams{Owner: owner, Book: *bookID, Corpus: corpusID, AnalysisRun: analysisRunID})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// analysisFromTokenEvidence rebuilds the analysed corpus from persisted token
// rows, grouping tokens into sentences by their stored ordinal. Sentences the
// corpus holds without tokens are padded with their own empty sentence, so a
// sentence's index is the stored ordinal the deck's sentence loader resolves
// (ADR 0106): a reference always names the sentence its token came from.
func analysisFromTokenEvidence(rows []sqlcgen.ListAnalysisTokenEvidenceRow) (analyzer.Result, error) {
	var analysis analyzer.Result
	indexByOrdinal := make(map[int64]int)
	for _, row := range rows {
		index, ok := indexByOrdinal[row.SentenceOrdinal]
		if !ok {
			if row.SentenceOrdinal < 0 {
				return analyzer.Result{}, errors.New("decode persisted token location: invalid sentence ordinal")
			}
			for int64(len(analysis.Sentences)) <= row.SentenceOrdinal {
				analysis.Sentences = append(analysis.Sentences, analyzer.Sentence{})
			}
			index = int(row.SentenceOrdinal)
			indexByOrdinal[row.SentenceOrdinal] = index
			location, err := persistedSentenceLocation(row)
			if err != nil {
				return analyzer.Result{}, err
			}
			analysis.Sentences[index].Text = row.SentenceText
			analysis.Sentences[index].Location = location
		}
		token, err := decodeTokenEvidence(row)
		if err != nil {
			return analyzer.Result{}, err
		}
		analysis.Language = row.Language
		analysis.Sentences[index].Tokens = append(analysis.Sentences[index].Tokens, token)
	}
	return analysis, nil
}

// persistedSentenceLocation decodes one persisted sentence's unit-relative
// range, so structural facts can resolve against the exact source snapshot.
func persistedSentenceLocation(row sqlcgen.ListAnalysisTokenEvidenceRow) (analyzer.SourceLocation, error) {
	start, err := checked.Uint64FromInt64(row.SentenceStartOffset)
	if err != nil {
		return analyzer.SourceLocation{}, fmt.Errorf("decode persisted sentence start offset: %w", err)
	}
	end, err := checked.Uint64FromInt64(row.SentenceEndOffset)
	if err != nil {
		return analyzer.SourceLocation{}, fmt.Errorf("decode persisted sentence end offset: %w", err)
	}
	return analyzer.SourceLocation{SourceDocumentID: row.UnitID, StartOffset: start, EndOffset: end}, nil
}

// decodeTokenEvidence decodes one persisted token row, rejecting an invalid
// location or head ordinal.
func decodeTokenEvidence(row sqlcgen.ListAnalysisTokenEvidenceRow) (analyzer.Token, error) {
	head, err := checked.Uint32FromInt64(row.Head)
	if err != nil || row.StartOffset < 0 || row.EndOffset < 0 {
		return analyzer.Token{}, errors.New("decode persisted token location: invalid offsets or head")
	}
	var morphology map[string]string
	if err := json.Unmarshal(row.Morphology, &morphology); err != nil {
		return analyzer.Token{}, fmt.Errorf("decode persisted token morphology: %w", err)
	}
	return analyzer.Token{
		Surface: row.Surface, RawLemma: row.RawLemma, CanonicalLemma: row.CanonicalLemma,
		UPOS: row.Upos, Dependency: row.Dependency, Head: head, Morphology: morphology,
		LexicalDiscriminator: row.LexicalDiscriminator, Headword: row.Headword,
		Location: analyzer.SourceLocation{SourceDocumentID: row.UnitID, StartOffset: uint64(row.StartOffset), EndOffset: uint64(row.EndOffset)},
	}, nil
}

// occurrenceLemmaCorrections decodes the persisted occurrence lemma
// corrections, rejecting an invalid source offset.
func occurrenceLemmaCorrections(rows []sqlcgen.ListOccurrenceLemmaCorrectionsRow) ([]domain.OccurrenceLemmaCorrection, error) {
	corrections := make([]domain.OccurrenceLemmaCorrection, 0, len(rows))
	for _, row := range rows {
		if row.StartOffset < 0 || row.EndOffset < 0 {
			return nil, errors.New("decode persisted lemma correction: invalid offsets")
		}
		lemma := row.CanonicalLemma.String
		corrections = append(corrections, domain.OccurrenceLemmaCorrection{SourceDocumentID: row.SourceDocumentID, StartOffset: row.StartOffset, EndOffset: row.EndOffset, CanonicalLemma: lemma, Excluded: row.Excluded})
	}
	return corrections, nil
}

// candidateSentenceOrdinalsByCorpus groups the sentence ordinals each selected
// candidate references by corpus. A candidate whose references do not decode
// contributes no ordinals.
func candidateSentenceOrdinalsByCorpus(selected []domain.SelectionCandidate) map[string]map[int64]struct{} {
	ordinalsByCorpus := make(map[string]map[int64]struct{})
	for _, candidate := range selected {
		ordinals, ok := decodeCandidateSentenceOrdinals(candidate)
		if !ok {
			continue
		}
		corpusOrdinals := ordinalsByCorpus[candidate.CorpusID]
		if corpusOrdinals == nil {
			corpusOrdinals = make(map[int64]struct{})
			ordinalsByCorpus[candidate.CorpusID] = corpusOrdinals
		}
		for _, ordinal := range ordinals {
			corpusOrdinals[ordinal] = struct{}{}
		}
	}
	return ordinalsByCorpus
}

// decodeCandidateSentenceOrdinals returns the non-negative sentence ordinals a
// candidate references. ok is false when its references do not decode.
func decodeCandidateSentenceOrdinals(candidate domain.SelectionCandidate) (ordinals []int64, ok bool) {
	var refs []struct {
		SentenceIndex int `json:"sentence_index"`
	}
	if json.Unmarshal(candidate.SentenceReferences, &refs) != nil {
		return nil, false
	}
	ordinals = make([]int64, 0, len(refs))
	for _, ref := range refs {
		if ref.SentenceIndex >= 0 {
			ordinals = append(ordinals, int64(ref.SentenceIndex))
		}
	}
	return ordinals, true
}

// loadCorpusSentenceOrdinals loads the persisted sentences one corpus
// references and rejects a missing ordinal instead of silently dropping
// context. A corpus that holds none of the ordinals yields an empty map.
func loadCorpusSentenceOrdinals(ctx context.Context, tx pgx.Tx, owner, corpusID string, ordinalSet map[int64]struct{}) (map[int64]analyzer.Sentence, error) {
	ordinals := make([]int64, 0, len(ordinalSet))
	for ordinal := range ordinalSet {
		ordinals = append(ordinals, ordinal)
	}
	sentences, err := listCorpusSentences(ctx, tx, owner, corpusID, ordinals)
	if err != nil {
		return nil, fmt.Errorf("list persisted corpus sentences for %s: %w", corpusID, err)
	}
	if len(sentences) == 0 {
		return sentences, nil
	}
	for ordinal := range ordinalSet {
		if _, ok := sentences[ordinal]; !ok {
			return nil, fmt.Errorf("list persisted corpus sentences for %s: missing sentence ordinal %d", corpusID, ordinal)
		}
	}
	return sentences, nil
}

func listKnownVocabulary(ctx context.Context, q sqlcgen.DBTX, owner, language string) ([]domain.KnownVocabulary, error) {
	rows, err := sqlcgen.New(q).ListKnownVocabulary(ctx, sqlcgen.ListKnownVocabularyParams{OwnerID: owner, Language: canonicalization.NormalizeLanguage(language)})
	if err != nil {
		return nil, err
	}
	result := make([]domain.KnownVocabulary, 0, len(rows))
	for _, row := range rows {
		result = append(result, knownVocabularyFromFields(row.KvID, row.KvOwnerID, row.Language, row.CanonicalLemma, row.Upos, row.LexicalDiscriminator, row.Headword, row.CreatedAt))
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
		result = append(result, domain.DeckPreparationVocabulary{
			OwnerID: row.OwnerID, DeckPreparationID: row.DeckPreparationID,
			Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos,
			LexicalDiscriminator: row.LexicalDiscriminator, Headword: row.Headword,
			GeneratedAt: row.GeneratedAt, GraduatedAt: pgTimePtr(row.GraduatedAt),
		})
	}
	return result, nil
}
