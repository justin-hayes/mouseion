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
	DeckName                     string
	CorpusID                     string
	Candidates                   []domain.SelectionCandidate
	Analysis                     *analyzer.Result
	Corrections                  []domain.OccurrenceLemmaCorrection
	CurrentReadingSnapshotActive bool
	CurrentReadingSnapshot       []domain.SelectionCandidate
	Known                        []domain.KnownVocabulary
	Generated                    []domain.GeneratedVocabulary // historical provenance, never an eligibility exclusion
	Reserved                     []domain.DeckPreparationVocabulary
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
	result := PreparedDeckInputFacts{DeckName: deckName, CorpusID: corpusID, Candidates: candidates}
	languages := candidateLanguages(candidates)
	if err := attachPreparedDeckCurrentReadingSnapshotTx(ctx, q, preparation, &result); err != nil {
		return PreparedDeckInputFacts{}, err
	}
	if err := attachPreparedDeckAnalysisFactsTx(ctx, tx, q, preparation, corpusID, &result, languages); err != nil {
		return PreparedDeckInputFacts{}, err
	}
	if err := attachPreparedDeckVocabularyFacts(ctx, tx, preparation.OwnerID, languages, &result); err != nil {
		return PreparedDeckInputFacts{}, err
	}
	return result, nil
}

// candidateLanguages collects the normalized languages of the selected
// candidates, so vocabulary facts are loaded once per language.
func candidateLanguages(candidates []domain.SelectionCandidate) map[string]struct{} {
	languages := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		languages[canonicalization.NormalizeLanguage(candidate.Language)] = struct{}{}
	}
	return languages
}

// attachPreparedDeckCurrentReadingSnapshotTx resolves the Current reading
// snapshot a preparation is bound to. A bound snapshot that is missing is an
// error, while a stale unbound lookup is ignored; any other failure propagates.
func attachPreparedDeckCurrentReadingSnapshotTx(ctx context.Context, q *sqlcgen.Queries, preparation domain.DeckPreparation, result *PreparedDeckInputFacts) error {
	snapshot, err := q.GetActiveCurrentReadingSnapshotForPreparation(ctx, sqlcgen.GetActiveCurrentReadingSnapshotForPreparationParams{Owner: preparation.OwnerID, Preparation: preparation.ID})
	if err == nil {
		result.CurrentReadingSnapshotActive = true
		result.CurrentReadingSnapshot, err = listCurrentReadingSnapshotVocabulary(ctx, q, preparation.OwnerID, snapshot.SID)
		return err
	}
	if preparation.SnapshotID != "" {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("prepared deck Current reading snapshot %q is unavailable: %w", preparation.SnapshotID, ErrNotFound)
		}
		return err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
}

// attachPreparedDeckAnalysisFactsTx loads the analysis projection and
// corrections for a direct corpus-scoped preparation. A Current reading
// snapshot already carries its own vocabulary, so the projection is skipped.
func attachPreparedDeckAnalysisFactsTx(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, preparation domain.DeckPreparation, corpusID string, result *PreparedDeckInputFacts, languages map[string]struct{}) error {
	if result.CurrentReadingSnapshotActive || corpusID == "" {
		return nil
	}
	analysis, corrections, err := loadAnalysisProjectionFactsTx(ctx, tx, q, preparation, corpusID)
	if err != nil {
		return err
	}
	if len(analysis.Sentences) > 0 {
		result.Analysis = &analysis
		languages[canonicalization.NormalizeLanguage(analysis.Language)] = struct{}{}
	}
	result.Corrections = corrections
	return nil
}

// attachPreparedDeckVocabularyFacts appends the known, generated and reserved
// vocabulary of every language the candidates reference.
func attachPreparedDeckVocabularyFacts(ctx context.Context, tx pgx.Tx, owner string, languages map[string]struct{}, result *PreparedDeckInputFacts) error {
	for language := range languages {
		known, err := listKnownVocabulary(ctx, tx, owner, language)
		if err != nil {
			return err
		}
		generated, err := listUnattachedGeneratedVocabulary(ctx, tx, owner, language)
		if err != nil {
			return err
		}
		reserved, err := listReservedVocabulary(ctx, tx, owner, language)
		if err != nil {
			return err
		}
		result.Known = append(result.Known, known...)
		result.Generated = append(result.Generated, generated...)
		result.Reserved = append(result.Reserved, reserved...)
	}
	return nil
}

func loadAnalysisProjectionFactsTx(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, preparation domain.DeckPreparation, corpusID string) (analyzer.Result, []domain.OccurrenceLemmaCorrection, error) {
	rows, err := q.ListAnalysisTokenEvidence(ctx, sqlcgen.ListAnalysisTokenEvidenceParams{Owner: preparation.OwnerID, Corpus: corpusID, AnalysisRun: uuidArg(preparation.AnalysisRunID)})
	if err != nil {
		return analyzer.Result{}, nil, err
	}
	bookID, err := preparedDeckSourceBookID(ctx, tx, preparation)
	if err != nil {
		return analyzer.Result{}, nil, err
	}
	correctionRows, err := listOccurrenceLemmaCorrectionsTx(ctx, q, preparation, corpusID, bookID)
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

// preparedDeckSourceBookID returns the Book a preparation's source is linked
// to, or nil for an unlinked legacy source.
func preparedDeckSourceBookID(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation) (*string, error) {
	var bookID *string
	if err := tx.QueryRow(ctx, `SELECT book_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, preparation.OwnerID, preparation.SourceMaterialID).Scan(&bookID); err != nil {
		return nil, err
	}
	return bookID, nil
}

// listOccurrenceLemmaCorrectionsTx loads the corrections recorded against the
// source's Book. An unlinked source has no Book to correct.
func listOccurrenceLemmaCorrectionsTx(ctx context.Context, q *sqlcgen.Queries, preparation domain.DeckPreparation, corpusID string, bookID *string) ([]sqlcgen.ListOccurrenceLemmaCorrectionsRow, error) {
	if bookID == nil {
		return nil, nil
	}
	rows, err := q.ListOccurrenceLemmaCorrections(ctx, sqlcgen.ListOccurrenceLemmaCorrectionsParams{Owner: preparation.OwnerID, Book: *bookID, Corpus: corpusID, AnalysisRun: preparation.AnalysisRunID})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// analysisFromTokenEvidence rebuilds the analysed corpus from persisted token
// rows, grouping tokens into sentences by their stored ordinal.
func analysisFromTokenEvidence(rows []sqlcgen.ListAnalysisTokenEvidenceRow) (analyzer.Result, error) {
	var analysis analyzer.Result
	indexByOrdinal := make(map[int64]int)
	for _, row := range rows {
		index, ok := indexByOrdinal[row.SentenceOrdinal]
		if !ok {
			index = len(analysis.Sentences)
			indexByOrdinal[row.SentenceOrdinal] = index
			analysis.Sentences = append(analysis.Sentences, analyzer.Sentence{Text: row.SentenceText})
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

// LoadPreparedDeckCandidateFactsTx loads render and sentence facts for the
// selected candidates and rejects partially available persisted sentences. It
// resolves the coverage scope once, then loads one fact kind at a time, so a
// new fact kind adds a loader rather than growing this orchestration.
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
	useProjectedEntry, err := preparedDeckProjectedEntryScopeTx(ctx, tx, q, preparation, corpusID)
	if err != nil {
		return nil, err
	}
	sentencesByCorpus, err := loadPreparedDeckCandidateSentencesTx(ctx, tx, preparation, selected)
	if err != nil {
		return nil, err
	}
	result := make([]PreparedDeckCandidateFacts, 0, len(selected))
	for _, candidate := range selected {
		entry, entryErr := loadPreparedDeckCandidateEntryTx(ctx, tx, preparation, corpusID, useProjectedEntry, candidate)
		if entryErr != nil {
			return nil, entryErr
		}
		result = append(result, PreparedDeckCandidateFacts{Candidate: candidate, Entry: entry, Sentences: sentencesByCorpus[candidate.CorpusID]})
	}
	return result, nil
}

// preparedDeckProjectedEntryScopeTx reports whether a candidate's coverage
// entry must be projected from stored sentence references instead of read back
// from a coverage entry. A Current reading snapshot always projects; a direct
// preparation projects only once the Book holds occurrence lemma corrections.
func preparedDeckProjectedEntryScopeTx(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, preparation domain.DeckPreparation, corpusID string) (bool, error) {
	if preparation.SnapshotID != "" {
		return true, nil
	}
	if corpusID == "" {
		return false, nil
	}
	var bookID string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(book_id::text, '') FROM source_materials WHERE owner_id=$1 AND id=$2`, preparation.OwnerID, preparation.SourceMaterialID).Scan(&bookID); err != nil {
		return false, err
	}
	if bookID == "" {
		return false, nil
	}
	corrections, err := q.ListOccurrenceLemmaCorrections(ctx, sqlcgen.ListOccurrenceLemmaCorrectionsParams{
		Owner: preparation.OwnerID, Book: bookID, Corpus: corpusID, AnalysisRun: preparation.AnalysisRunID,
	})
	if err != nil {
		return false, err
	}
	return len(corrections) > 0, nil
}

// loadPreparedDeckCandidateSentencesTx loads the persisted sentence context
// every selected candidate references, keyed by corpus and ordinal. A
// candidate whose references do not decode contributes no ordinals.
func loadPreparedDeckCandidateSentencesTx(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation, selected []domain.SelectionCandidate) (map[string]map[int64]analyzer.Sentence, error) {
	ordinalsByCorpus := candidateSentenceOrdinalsByCorpus(selected)
	sentencesByCorpus := make(map[string]map[int64]analyzer.Sentence, len(ordinalsByCorpus))
	for corpusID, ordinalSet := range ordinalsByCorpus {
		sentences, err := loadCorpusSentenceOrdinals(ctx, tx, preparation.OwnerID, corpusID, ordinalSet)
		if err != nil {
			return nil, err
		}
		if len(sentences) > 0 {
			sentencesByCorpus[corpusID] = sentences
		}
	}
	return sentencesByCorpus, nil
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

// loadPreparedDeckCandidateEntryTx loads one candidate's coverage entry. When
// the coverage entry is absent and the candidate carries projected sentence
// references, the entry is projected instead.
func loadPreparedDeckCandidateEntryTx(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation, corpusID string, useProjectedEntry bool, candidate domain.SelectionCandidate) (cardexport.Entry, error) {
	var entry cardexport.Entry
	var err error
	if useProjectedEntry {
		entry, err = projectedCandidateEntry(ctx, tx, preparation, candidate)
	} else if corpusID != "" {
		entry, err = getCoverageEntryForCorpus(ctx, tx, preparation.OwnerID, corpusID, candidate)
	} else {
		entry, err = getCoverageEntryForBook(ctx, tx, preparation.OwnerID, preparation.SourceMaterialID, candidate)
	}
	if err != nil {
		if corpusID == "" || !errors.Is(err, ErrNotFound) {
			return cardexport.Entry{}, fmt.Errorf("load coverage facts for %s: %w", candidateIdentity(candidate), err)
		}
		entry, err = projectedCandidateEntry(ctx, tx, preparation, candidate)
		if err != nil {
			return cardexport.Entry{}, fmt.Errorf("load projected coverage facts for %s: %w", candidateIdentity(candidate), err)
		}
	}
	return entry, nil
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
		result = append(result, domain.KnownVocabulary{ID: row.KvID, OwnerID: row.KvOwnerID, Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, CreatedAt: row.CreatedAt})
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
