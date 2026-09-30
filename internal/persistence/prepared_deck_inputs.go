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
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/checked"
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
	DeckName           string
	CorpusID           string
	Candidates         []domain.SelectionCandidate
	Analysis           *analyzer.Result
	Corrections        []domain.OccurrenceLemmaCorrection
	GoalSnapshotActive bool
	GoalSnapshot       []domain.SelectionCandidate
	Known              []domain.KnownVocabulary
	Generated          []domain.GeneratedVocabulary // historical provenance, never an eligibility exclusion
	Reserved           []domain.DeckPreparationVocabulary
}

// LoadPreparedDeckInputFactsTx reads candidate and vocabulary facts through
// the supplied transaction. Render and sentence facts are loaded only after
// the planner has applied recurring-vocabulary selection.
func (s *PostgresStore) LoadPreparedDeckInputFactsTx(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation) (PreparedDeckInputFacts, error) {
	if s == nil || tx == nil || preparation.OwnerID == "" || preparation.SourceMaterialID == "" {
		return PreparedDeckInputFacts{}, ErrInvalidTransition
	}
	q := sqlcgen.New(tx)
	deckName, corpusID, candidates, err := preparedDeckScopeTx(ctx, tx, q, preparation)
	if err != nil {
		return PreparedDeckInputFacts{}, err
	}
	if preparation.GoalSnapshotID == "" && corpusID != "" {
		var bookID, analysisRunID string
		lookupErr := tx.QueryRow(ctx, `SELECT sm.book_id::text,c.analysis_run_id::text FROM source_materials sm JOIN corpora c ON c.owner_id=sm.owner_id AND c.id=$3 WHERE sm.owner_id=$1 AND sm.id=$2 AND sm.book_id IS NOT NULL`, preparation.OwnerID, preparation.SourceMaterialID, corpusID).Scan(&bookID, &analysisRunID)
		if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
			return PreparedDeckInputFacts{}, lookupErr
		}
		if lookupErr == nil {
			var lockedBookID string
			if lockErr := tx.QueryRow(ctx, `SELECT id::text FROM books WHERE owner_id=$1 AND id=$2 FOR UPDATE`, preparation.OwnerID, bookID).Scan(&lockedBookID); lockErr != nil {
				return PreparedDeckInputFacts{}, lockErr
			}
			blocked, gateErr := unresolvedLemmaReviewFlags(ctx, tx, preparation.OwnerID, bookID, analysisRunID)
			if gateErr != nil {
				return PreparedDeckInputFacts{}, gateErr
			}
			if blocked {
				return PreparedDeckInputFacts{}, ErrUnresolvedLemmaReviewFlags
			}
		}
	}

	languages := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		languages[canonicalization.NormalizeLanguage(candidate.Language)] = struct{}{}
	}
	result := PreparedDeckInputFacts{DeckName: deckName, CorpusID: corpusID, Candidates: candidates}
	if snapshot, snapshotErr := q.GetActivePrimaryGoalSnapshotForPreparation(ctx, sqlcgen.GetActivePrimaryGoalSnapshotForPreparationParams{Owner: preparation.OwnerID, Preparation: preparation.ID}); snapshotErr == nil {
		result.GoalSnapshotActive = true
		result.GoalSnapshot, snapshotErr = listPrimaryGoalSnapshotVocabulary(ctx, q, preparation.OwnerID, snapshot.SID)
		if snapshotErr != nil {
			return PreparedDeckInputFacts{}, snapshotErr
		}
	} else if preparation.GoalSnapshotID != "" {
		if errors.Is(snapshotErr, pgx.ErrNoRows) {
			return PreparedDeckInputFacts{}, fmt.Errorf("prepared deck Goal snapshot %q is unavailable: %w", preparation.GoalSnapshotID, ErrNotFound)
		}
		return PreparedDeckInputFacts{}, snapshotErr
	} else if !errors.Is(snapshotErr, pgx.ErrNoRows) {
		return PreparedDeckInputFacts{}, snapshotErr
	}
	if !result.GoalSnapshotActive && corpusID != "" {
		analysis, corrections, loadErr := loadAnalysisProjectionFactsTx(ctx, tx, q, preparation, corpusID)
		err = loadErr
		if err != nil {
			return PreparedDeckInputFacts{}, err
		}
		if len(analysis.Sentences) > 0 {
			result.Analysis = &analysis
			languages[canonicalization.NormalizeLanguage(analysis.Language)] = struct{}{}
		}
		result.Corrections = corrections
	}
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

func loadAnalysisProjectionFactsTx(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, preparation domain.DeckPreparation, corpusID string) (analyzer.Result, []domain.OccurrenceLemmaCorrection, error) {
	rows, err := q.ListAnalysisTokenEvidence(ctx, sqlcgen.ListAnalysisTokenEvidenceParams{Owner: preparation.OwnerID, Corpus: corpusID, AnalysisRun: uuidArg(preparation.AnalysisRunID)})
	if err != nil {
		return analyzer.Result{}, nil, err
	}
	var bookID *string
	if err = tx.QueryRow(ctx, `SELECT book_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, preparation.OwnerID, preparation.SourceMaterialID).Scan(&bookID); err != nil {
		return analyzer.Result{}, nil, err
	}
	var correctionRows []sqlcgen.ListOccurrenceLemmaCorrectionsRow
	if bookID != nil {
		correctionRows, err = q.ListOccurrenceLemmaCorrections(ctx, sqlcgen.ListOccurrenceLemmaCorrectionsParams{Owner: preparation.OwnerID, Book: *bookID, Corpus: corpusID, AnalysisRun: preparation.AnalysisRunID})
		if err != nil {
			return analyzer.Result{}, nil, err
		}
	}
	var analysis analyzer.Result
	sentences := make(map[int64]int)
	for _, row := range rows {
		index, ok := sentences[row.SentenceOrdinal]
		if !ok {
			index = len(analysis.Sentences)
			sentences[row.SentenceOrdinal] = index
			analysis.Sentences = append(analysis.Sentences, analyzer.Sentence{Text: row.SentenceText})
		}
		head, convertErr := checked.Uint32FromInt64(row.Head)
		if convertErr != nil || row.StartOffset < 0 || row.EndOffset < 0 {
			return analyzer.Result{}, nil, errors.New("decode persisted token location: invalid offsets or head")
		}
		var morphology map[string]string
		if err = json.Unmarshal(row.Morphology, &morphology); err != nil {
			return analyzer.Result{}, nil, fmt.Errorf("decode persisted token morphology: %w", err)
		}
		analysis.Language = row.Language
		analysis.Sentences[index].Tokens = append(analysis.Sentences[index].Tokens, analyzer.Token{
			Surface: row.Surface, RawLemma: row.RawLemma, CanonicalLemma: row.CanonicalLemma,
			UPOS: row.Upos, Dependency: row.Dependency, Head: head, Morphology: morphology,
			Location: analyzer.SourceLocation{SourceDocumentID: row.UnitID, StartOffset: uint64(row.StartOffset), EndOffset: uint64(row.EndOffset)},
		})
	}
	corrections := make([]domain.OccurrenceLemmaCorrection, 0, len(correctionRows))
	for _, row := range correctionRows {
		if row.StartOffset < 0 || row.EndOffset < 0 {
			return analyzer.Result{}, nil, errors.New("decode persisted lemma correction: invalid offsets")
		}
		lemma := row.CanonicalLemma.String
		corrections = append(corrections, domain.OccurrenceLemmaCorrection{SourceDocumentID: row.SourceDocumentID, StartOffset: row.StartOffset, EndOffset: row.EndOffset, CanonicalLemma: lemma, Excluded: row.Excluded})
	}
	return analysis, corrections, nil
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
	useProjectedEntry := preparation.GoalSnapshotID != ""
	if !useProjectedEntry && corpusID != "" {
		var bookID string
		if err = tx.QueryRow(ctx, `SELECT COALESCE(book_id::text, '') FROM source_materials WHERE owner_id=$1 AND id=$2`, preparation.OwnerID, preparation.SourceMaterialID).Scan(&bookID); err != nil {
			return nil, err
		}
		if bookID != "" {
			corrections, correctionErr := q.ListOccurrenceLemmaCorrections(ctx, sqlcgen.ListOccurrenceLemmaCorrectionsParams{
				Owner: preparation.OwnerID, Book: bookID, Corpus: corpusID, AnalysisRun: preparation.AnalysisRunID,
			})
			if correctionErr != nil {
				return nil, correctionErr
			}
			useProjectedEntry = len(corrections) > 0
		}
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
		if useProjectedEntry {
			entry, err = projectedCandidateEntry(ctx, tx, preparation, candidate)
		} else if corpusID != "" {
			entry, err = getCoverageEntryForCorpus(ctx, tx, preparation.OwnerID, corpusID, candidate)
		} else {
			entry, err = getCoverageEntryForBook(ctx, tx, preparation.OwnerID, preparation.SourceMaterialID, candidate)
		}
		if err != nil {
			if corpusID == "" || !errors.Is(err, ErrNotFound) {
				return nil, fmt.Errorf("load coverage facts for %s: %w", candidateIdentity(candidate), err)
			}
			entry, err = projectedCandidateEntry(ctx, tx, preparation, candidate)
			if err != nil {
				return nil, fmt.Errorf("load projected coverage facts for %s: %w", candidateIdentity(candidate), err)
			}
		}
		result = append(result, PreparedDeckCandidateFacts{Candidate: candidate, Entry: entry, Sentences: sentencesByCorpus[candidate.CorpusID]})
	}
	return result, nil
}

func projectedCandidateEntry(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation, candidate domain.SelectionCandidate) (cardexport.Entry, error) {
	var title string
	if err := tx.QueryRow(ctx, `SELECT title FROM source_materials WHERE owner_id=$1 AND id=$2`, preparation.OwnerID, preparation.SourceMaterialID).Scan(&title); err != nil {
		return cardexport.Entry{}, err
	}
	var refs []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(candidate.SentenceReferences, &refs); err != nil || len(refs) == 0 {
		return cardexport.Entry{}, errors.New("projected sentence references are unavailable")
	}
	var forms []string
	if err := json.Unmarshal(candidate.ObservedForms, &forms); err != nil || len(forms) == 0 {
		return cardexport.Entry{}, errors.New("projected observed forms are unavailable")
	}
	return cardexport.Entry{
		OwnerID: preparation.OwnerID, Language: candidate.Language,
		CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS,
		Sentence: refs[0].Text, TargetWord: forms[0], Morphology: "[]",
		SourceDocument: title, FirstEncounter: candidate.FirstEncounter,
	}, nil
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
		result = append(result, domain.DeckPreparationVocabulary{
			OwnerID: row.OwnerID, DeckPreparationID: row.DeckPreparationID,
			Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos,
			GeneratedAt: row.GeneratedAt, GraduatedAt: pgTimePtr(row.GraduatedAt),
		})
	}
	return result, nil
}

func candidateIdentity(candidate domain.SelectionCandidate) string {
	return candidate.CorpusID + "\x00" + candidate.Language + "\x00" + candidate.CanonicalLemma + "\x00" + candidate.UPOS
}
