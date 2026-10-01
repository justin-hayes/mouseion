package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

var ErrCustomDeckPreparationUnavailable = errors.New("custom deck has no current eligible evidence")
var ErrCustomDeckPreparationEvidenceChanged = errors.New("current evidence availability changed since review")

const customDeckPreparationColumns = `id::text,owner_id::text,custom_deck_id::text,language,deck_name,filename,state,
	 frozen_spec,artifact,total_cards,selected_identities,error,created_at,started_at,completed_at,omissions`

func scanCustomDeckPreparation(row pgx.Row) (domain.CustomDeckPreparation, error) {
	var p domain.CustomDeckPreparation
	err := row.Scan(&p.ID, &p.OwnerID, &p.DeckID, &p.Language, &p.DeckName, &p.Filename, &p.State,
		&p.FrozenSpec, &p.Artifact, &p.TotalCards, &p.SelectedIdentities, &p.Error, &p.CreatedAt, &p.StartedAt, &p.CompletedAt, &p.Omissions)
	return p, err
}

// CreateCustomDeckPreparation records one explicit, idempotent generation.
// The unique action key makes a browser retry resolve to its original row.
func (s *PostgresStore) CreateCustomDeckPreparation(ctx context.Context, owner, deckID, actionKey, expectedEvidence string) (result domain.CustomDeckPreparation, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.CustomDeckPreparation{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	result, err = CreateCustomDeckPreparationTx(ctx, tx, owner, deckID, actionKey, expectedEvidence)
	if err != nil {
		return domain.CustomDeckPreparation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.CustomDeckPreparation{}, err
	}
	return result, nil
}

func (s *PostgresStore) CustomDeckEvidenceFingerprint(ctx context.Context, owner, deckID string) (string, error) {
	rows, err := s.pool.Query(ctx, customDeckEvidenceFingerprintQuery, owner, deckID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	return fingerprintCustomDeckEvidence(rows)
}

const customDeckEvidenceFingerprintQuery = `SELECT i.canonical_lemma,i.upos,EXISTS (
 SELECT 1 FROM concordance_occurrences o
 JOIN current_analysis_identity cai ON cai.owner_id=o.owner_id AND cai.book_id::text=o.book_id
  AND cai.analysis_run_id::text=o.analysis_run_id AND cai.corpus_id::text=o.corpus_id
 JOIN books b ON b.owner_id=o.owner_id AND b.id::text=o.book_id AND b.language_tag=o.language AND b.language_state='chosen'
 LEFT JOIN occurrence_lemma_corrections c ON c.owner_id=o.owner_id AND c.book_id::text=o.book_id
  AND c.corpus_id::text=o.corpus_id AND c.analysis_run_id::text=o.analysis_run_id
  AND c.source_document_id=o.unit_id AND c.start_offset=o.unit_start_offset AND c.end_offset=o.unit_end_offset
 WHERE o.owner_id=i.owner_id AND o.language=i.language AND o.upos=i.upos
  AND COALESCE(c.canonical_lemma,o.canonical_lemma)=i.canonical_lemma AND NOT COALESCE(c.excluded,false)
  AND o.upos IN ('NOUN','VERB','ADJ','ADV') AND o.dependency <> 'compound:prt'
  AND COALESCE(c.canonical_lemma,o.canonical_lemma) ~ '[[:alpha:]]'
) FROM custom_vocabulary_deck_identities i WHERE i.owner_id=$1 AND i.deck_id=$2
 ORDER BY i.canonical_lemma,i.upos`

type customDeckEvidenceRows interface {
	Next() bool
	Scan(...any) error
	Err() error
}

func fingerprintCustomDeckEvidence(rows customDeckEvidenceRows) (string, error) {
	h := sha256.New()
	for rows.Next() {
		var lemma, upos string
		var hasEvidence bool
		if err := rows.Scan(&lemma, &upos, &hasEvidence); err != nil {
			return "", err
		}
		if _, err := fmt.Fprintf(h, "%d:%s:%s:%t\n", len(lemma), lemma, upos, hasEvidence); err != nil {
			return "", err
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func CreateCustomDeckPreparationTx(ctx context.Context, tx pgx.Tx, owner, deckID, actionKey, expectedEvidence string) (domain.CustomDeckPreparation, error) {
	existing, lookupErr := scanCustomDeckPreparation(tx.QueryRow(ctx, `SELECT `+customDeckPreparationColumns+`
	 FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND custom_deck_id=$2 AND submission_key=$3`, owner, deckID, actionKey))
	if lookupErr == nil {
		return existing, nil
	}
	if !errors.Is(lookupErr, pgx.ErrNoRows) {
		return domain.CustomDeckPreparation{}, lookupErr
	}
	rows, err := tx.Query(ctx, customDeckEvidenceFingerprintQuery, owner, deckID)
	if err != nil {
		return domain.CustomDeckPreparation{}, err
	}
	currentEvidence, err := fingerprintCustomDeckEvidence(rows)
	rows.Close()
	if err != nil {
		return domain.CustomDeckPreparation{}, err
	}
	if expectedEvidence == "" || currentEvidence != expectedEvidence {
		return domain.CustomDeckPreparation{}, ErrCustomDeckPreparationEvidenceChanged
	}
	p, err := scanCustomDeckPreparation(tx.QueryRow(ctx, `
WITH eligible AS (
 SELECT count(*) FILTER (WHERE EXISTS (
   SELECT 1 FROM concordance_occurrences o
   JOIN current_analysis_identity cai ON cai.owner_id=o.owner_id AND cai.book_id::text=o.book_id
    AND cai.analysis_run_id::text=o.analysis_run_id AND cai.corpus_id::text=o.corpus_id
   JOIN books b ON b.owner_id=o.owner_id AND b.id::text=o.book_id AND b.language_tag=o.language AND b.language_state='chosen'
   LEFT JOIN occurrence_lemma_corrections c ON c.owner_id=o.owner_id AND c.book_id::text=o.book_id
    AND c.corpus_id::text=o.corpus_id AND c.analysis_run_id::text=o.analysis_run_id
    AND c.source_document_id=o.unit_id AND c.start_offset=o.unit_start_offset AND c.end_offset=o.unit_end_offset
   WHERE o.owner_id=i.owner_id AND o.language=i.language AND o.upos=i.upos
    AND COALESCE(c.canonical_lemma,o.canonical_lemma)=i.canonical_lemma
    AND NOT COALESCE(c.excluded,false) AND o.upos IN ('NOUN','VERB','ADJ','ADV')
    AND o.dependency <> 'compound:prt' AND COALESCE(c.canonical_lemma,o.canonical_lemma) ~ '[[:alpha:]]'
 ))::integer AS evidenced, count(*)::integer AS selected
 FROM custom_vocabulary_deck_identities i WHERE i.owner_id=$1 AND i.deck_id=$2
)
INSERT INTO custom_vocabulary_deck_preparations(owner_id,custom_deck_id,submission_key,language,deck_name,filename,selected_identities)
SELECT d.owner_id,d.id,$3,d.language,d.name,
 regexp_replace(lower(d.name),'[^a-z0-9]+','-','g')||'.apkg',eligible.selected
FROM custom_vocabulary_decks d CROSS JOIN eligible
WHERE d.owner_id=$1 AND d.id=$2 AND eligible.evidenced>0 AND eligible.selected>0
ON CONFLICT(owner_id,custom_deck_id,submission_key) DO UPDATE SET submission_key=EXCLUDED.submission_key
RETURNING `+customDeckPreparationColumns, owner, deckID, actionKey))
	if errors.Is(err, pgx.ErrNoRows) {
		var deckExists bool
		if lookupErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM custom_vocabulary_decks WHERE owner_id=$1 AND id=$2)`, owner, deckID).Scan(&deckExists); lookupErr != nil {
			return domain.CustomDeckPreparation{}, lookupErr
		}
		if !deckExists {
			return domain.CustomDeckPreparation{}, ErrCustomVocabularyDeckNotFound
		}
		return domain.CustomDeckPreparation{}, ErrCustomDeckPreparationUnavailable
	}
	if err != nil {
		return domain.CustomDeckPreparation{}, fmt.Errorf("create Custom deck preparation: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO custom_vocabulary_deck_preparation_identities(owner_id,preparation_id,language,canonical_lemma,upos)
 SELECT d.owner_id,$3,d.language,i.canonical_lemma,i.upos
 FROM custom_vocabulary_decks d JOIN custom_vocabulary_deck_identities i ON i.owner_id=d.owner_id AND i.deck_id=d.id
 WHERE d.owner_id=$1 AND d.id=$2 AND NOT EXISTS (
   SELECT 1 FROM custom_vocabulary_deck_preparation_identities frozen WHERE frozen.owner_id=$1 AND frozen.preparation_id=$3
 ) ON CONFLICT DO NOTHING`, owner, deckID, p.ID); err != nil {
		return domain.CustomDeckPreparation{}, fmt.Errorf("freeze Custom deck identity selection: %w", err)
	}
	return p, nil
}

func (s *PostgresStore) GetCustomDeckPreparation(ctx context.Context, owner, id string) (domain.CustomDeckPreparation, error) {
	p, err := scanCustomDeckPreparation(s.pool.QueryRow(ctx, `SELECT `+customDeckPreparationColumns+` FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND id=$2`, owner, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomDeckPreparation{}, ErrNotFound
	}
	return p, err
}

func (s *PostgresStore) ClaimCustomDeckPreparation(ctx context.Context, owner, id string) (domain.CustomDeckPreparation, error) {
	p, err := scanCustomDeckPreparation(s.pool.QueryRow(ctx, `UPDATE custom_vocabulary_deck_preparations
 SET state='preparing',started_at=COALESCE(started_at,now()) WHERE owner_id=$1 AND id=$2 AND state='queued'
 RETURNING `+customDeckPreparationColumns, owner, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return s.GetCustomDeckPreparation(ctx, owner, id)
	}
	return p, err
}

func FreezeCustomDeckPreparationTx(ctx context.Context, tx pgx.Tx, owner, id string, spec []byte) error {
	tag, err := tx.Exec(ctx, `UPDATE custom_vocabulary_deck_preparations SET frozen_spec=$3::jsonb
	 WHERE owner_id=$1 AND id=$2 AND state='queued' AND frozen_spec IS NULL`, owner, id, spec)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var same bool
		if err := tx.QueryRow(ctx, `SELECT frozen_spec=$3::jsonb FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND id=$2`, owner, id, spec).Scan(&same); err != nil {
			return missing(err)
		}
		if !same {
			return ErrImmutable
		}
	}
	return nil
}

func (s *PostgresStore) CompleteCustomDeckPreparation(ctx context.Context, owner, id string, artifact []byte, total int, omissions []domain.CustomDeckPreparationOmission) (domain.CustomDeckPreparation, error) {
	encodedOmissions, err := json.Marshal(omissions)
	if err != nil {
		return domain.CustomDeckPreparation{}, err
	}
	state := "ready"
	if len(omissions) > 0 {
		state = "complete_with_omissions"
	}
	p, err := scanCustomDeckPreparation(s.pool.QueryRow(ctx, `UPDATE custom_vocabulary_deck_preparations
	 SET state=$5,artifact=$3,total_cards=$4,completed_at=now(),error='',omissions=$6::jsonb
	 WHERE owner_id=$1 AND id=$2 AND state='preparing' AND frozen_spec IS NOT NULL AND $4>0
	 RETURNING `+customDeckPreparationColumns, owner, id, artifact, total, state, encodedOmissions))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomDeckPreparation{}, ErrInvalidTransition
	}
	return p, err
}

func (s *PostgresStore) FailCustomDeckPreparation(ctx context.Context, owner, id, message string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE custom_vocabulary_deck_preparations SET state='failed',error=$3,completed_at=now()
 WHERE owner_id=$1 AND id=$2 AND state IN ('queued','preparing')`, owner, id, message)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidTransition
	}
	return nil
}

func (s *PostgresStore) CancelCustomDeckPreparation(ctx context.Context, owner, id string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE custom_vocabulary_deck_preparations
	 SET state='cancelled',error='Preparation cancelled.',completed_at=now()
	 WHERE owner_id=$1 AND id=$2 AND state IN ('queued','preparing')`, owner, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidTransition
	}
	return nil
}

func (s *PostgresStore) CustomDeckPreparationCancelled(ctx context.Context, owner, id string) (bool, error) {
	var cancelled bool
	err := s.pool.QueryRow(ctx, `SELECT state='cancelled' FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND id=$2`, owner, id).Scan(&cancelled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return cancelled, err
}

func (s *PostgresStore) LatestCustomDeckPreparation(ctx context.Context, owner, deckID string) (domain.CustomDeckPreparation, error) {
	p, err := scanCustomDeckPreparation(s.pool.QueryRow(ctx, `SELECT `+customDeckPreparationColumns+`
 FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND custom_deck_id=$2
 ORDER BY created_at DESC,id DESC LIMIT 1`, owner, deckID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomDeckPreparation{}, ErrNotFound
	}
	return p, err
}

func (s *PostgresStore) ListCustomDeckPreparations(ctx context.Context, owner, deckID string) ([]domain.CustomDeckPreparation, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+customDeckPreparationColumns+`
	 FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND custom_deck_id=$2
	 ORDER BY created_at DESC,id DESC`, owner, deckID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var preparations []domain.CustomDeckPreparation
	for rows.Next() {
		p, scanErr := scanCustomDeckPreparation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		preparations = append(preparations, p)
	}
	return preparations, rows.Err()
}

func (s *PostgresStore) LatestReadyCustomDeckPreparation(ctx context.Context, owner, deckID string) (domain.CustomDeckPreparation, error) {
	p, err := scanCustomDeckPreparation(s.pool.QueryRow(ctx, `SELECT `+customDeckPreparationColumns+`
 FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND custom_deck_id=$2
 AND state IN ('ready','complete_with_omissions') ORDER BY created_at DESC,id DESC LIMIT 1`, owner, deckID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomDeckPreparation{}, ErrNotFound
	}
	return p, err
}

func (s *PostgresStore) DownloadCustomDeckPreparation(ctx context.Context, owner, id string) (domain.CustomDeckPreparation, error) {
	p, err := scanCustomDeckPreparation(s.pool.QueryRow(ctx, `SELECT `+customDeckPreparationColumns+`
	 FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND id=$2 AND state IN ('ready','complete_with_omissions')
	 AND id=(SELECT p2.id FROM custom_vocabulary_deck_preparations p2
	   WHERE p2.owner_id=$1 AND p2.custom_deck_id=custom_vocabulary_deck_preparations.custom_deck_id AND p2.state IN ('ready','complete_with_omissions')
   ORDER BY p2.created_at DESC,p2.id DESC LIMIT 1)`, owner, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomDeckPreparation{}, ErrNotFound
	}
	return p, err
}

// LoadCustomDeckProjections chooses one current effective occurrence for each
// saved identity. It deliberately ignores Browse filters and Book disposition.
func (s *PostgresStore) LoadCustomDeckProjectionsTx(ctx context.Context, tx pgx.Tx, owner, deckID, preparationID string) (projections []cardexport.CandidateProjection, selected int, deckName string, evidence []domain.CustomDeckPreparationEvidence, omissions []domain.CustomDeckPreparationOmission, err error) {
	var language string
	if err := tx.QueryRow(ctx, `SELECT language,deck_name FROM custom_vocabulary_deck_preparations WHERE owner_id=$1 AND custom_deck_id=$2 AND id=$3`, owner, deckID, preparationID).Scan(&language, &deckName); err != nil {
		return nil, 0, "", nil, nil, missing(err)
	}
	identityRows, err := tx.Query(ctx, `SELECT canonical_lemma,upos FROM custom_vocabulary_deck_preparation_identities WHERE owner_id=$1 AND preparation_id=$2 ORDER BY canonical_lemma,upos`, owner, preparationID)
	if err != nil {
		return nil, 0, "", nil, nil, err
	}
	var identities []domain.VocabularyIdentity
	for identityRows.Next() {
		var identity domain.VocabularyIdentity
		if err := identityRows.Scan(&identity.CanonicalLemma, &identity.UPOS); err != nil {
			identityRows.Close()
			return nil, 0, "", nil, nil, err
		}
		identities = append(identities, identity)
	}
	if err := identityRows.Err(); err != nil {
		identityRows.Close()
		return nil, 0, "", nil, nil, err
	}
	identityRows.Close()
	projections = make([]cardexport.CandidateProjection, 0, len(identities))
	for _, identity := range identities {
		rows, queryErr := tx.Query(ctx, `
SELECT o.book_id,o.source_material_id,o.analysis_run_id,o.corpus_id,o.sentence_ordinal,o.sentence_text,
       o.surface,o.book_title,o.unit_id,o.unit_start_offset,o.unit_end_offset,o.canonical_lemma,t.raw_lemma
FROM concordance_occurrences o
JOIN current_analysis_identity cai ON cai.owner_id=o.owner_id AND cai.book_id::text=o.book_id
 AND cai.analysis_run_id::text=o.analysis_run_id AND cai.corpus_id::text=o.corpus_id
JOIN books b ON b.owner_id=o.owner_id AND b.id::text=o.book_id AND b.language_tag=o.language AND b.language_state='chosen'
JOIN corpus_tokens t ON t.owner_id=o.owner_id AND t.analysis_run_id::text=o.analysis_run_id
 AND t.corpus_id::text=o.corpus_id AND t.sentence_ordinal=o.sentence_ordinal AND t.token_ordinal=o.token_ordinal
LEFT JOIN occurrence_lemma_corrections c ON c.owner_id=o.owner_id AND c.book_id::text=o.book_id
 AND c.corpus_id::text=o.corpus_id AND c.analysis_run_id::text=o.analysis_run_id
 AND c.source_document_id=o.unit_id AND c.start_offset=o.unit_start_offset AND c.end_offset=o.unit_end_offset
WHERE o.owner_id=$1 AND o.language=$2 AND o.upos=$4
 AND COALESCE(c.canonical_lemma,o.canonical_lemma)=$3 AND NOT COALESCE(c.excluded,false)
 AND o.upos IN ('NOUN','VERB','ADJ','ADV') AND o.dependency <> 'compound:prt'
 AND COALESCE(c.canonical_lemma,o.canonical_lemma) ~ '[[:alpha:]]'
ORDER BY o.book_id,o.unit_order,o.sentence_ordinal,o.token_ordinal`, owner, language, identity.CanonicalLemma, identity.UPOS)
		if queryErr != nil {
			return nil, 0, "", nil, nil, queryErr
		}
		var candidates []customDeckOccurrence
		for rows.Next() {
			var candidate customDeckOccurrence
			if queryErr = rows.Scan(&candidate.BookID, &candidate.SourceID, &candidate.AnalysisRunID, &candidate.CorpusID, &candidate.SentenceOrdinal, &candidate.Sentence, &candidate.Surface, &candidate.BookTitle, &candidate.UnitID, &candidate.Offset, &candidate.EndOffset, &candidate.AnalyzerLemma, &candidate.RawLemma); queryErr != nil {
				rows.Close()
				return nil, 0, "", nil, nil, queryErr
			}
			candidate.Quality = cardexport.ScoreSentenceQuality(candidate.Sentence, candidate.Surface, candidate.Offset)
			candidates = append(candidates, candidate)
		}
		queryErr = rows.Err()
		rows.Close()
		if queryErr != nil {
			return nil, 0, "", nil, nil, queryErr
		}
		chosen, ok := chooseCustomDeckOccurrence(candidates)
		if !ok {
			kind, reason := "evidence", "No current eligible evidence exists for this identity."
			if len(candidates) > 0 {
				kind, reason = "quality", "No current representative sentence passed the sentence-quality gate."
			}
			omissions = append(omissions, domain.CustomDeckPreparationOmission{Lemma: identity.CanonicalLemma, UPOS: identity.UPOS, Kind: kind, Reason: reason})
			continue
		}
		refs, marshalErr := json.Marshal([]map[string]any{{"sentence_index": 0, "text": chosen.Sentence, "location": map[string]any{"start_offset": chosen.Offset}}})
		if marshalErr != nil {
			return nil, 0, "", nil, nil, marshalErr
		}
		forms, marshalErr := json.Marshal([]string{chosen.Surface})
		if marshalErr != nil {
			return nil, 0, "", nil, nil, marshalErr
		}
		candidate := domain.SelectionCandidate{OwnerID: owner, CorpusID: chosen.CorpusID, Language: language, CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS, OccurrenceCount: 1, FirstEncounter: chosen.Offset, ObservedForms: forms, SentenceReferences: refs}
		entry := cardexport.Entry{OwnerID: owner, Language: language, CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS, CorpusID: chosen.CorpusID, SentenceOrdinal: chosen.SentenceOrdinal, Sentence: chosen.Sentence, TargetWord: chosen.Surface, SourceDocument: chosen.BookTitle, FirstEncounter: chosen.Offset}
		tokens, tokenErr := loadCustomDeckSentenceTokens(ctx, tx, owner, chosen.CorpusID, chosen.SentenceOrdinal)
		if tokenErr != nil {
			return nil, 0, "", nil, nil, tokenErr
		}
		projections = append(projections, cardexport.CandidateProjection{OwnerID: owner, DeckName: deckName, Candidate: candidate, Entry: entry, Sentences: map[int64]analyzer.Sentence{0: {Text: chosen.Sentence, Tokens: tokens}}})
		evidence = append(evidence, domain.CustomDeckPreparationEvidence{
			BookID: chosen.BookID, BookTitle: chosen.BookTitle, SourceMaterialID: chosen.SourceID,
			AnalysisRunID: chosen.AnalysisRunID, CorpusID: chosen.CorpusID, UnitID: chosen.UnitID,
			RawLemma: chosen.RawLemma, AnalyzerLemma: chosen.AnalyzerLemma, Lemma: identity.CanonicalLemma, UPOS: identity.UPOS,
			Sentence: chosen.Sentence, Target: chosen.Surface, SentenceOrdinal: chosen.SentenceOrdinal,
			StartOffset: chosen.Offset, EndOffset: chosen.EndOffset,
		})
	}
	return projections, len(identities), deckName, evidence, omissions, nil
}

func loadCustomDeckSentenceTokens(ctx context.Context, tx pgx.Tx, owner, corpusID string, ordinal int64) ([]analyzer.Token, error) {
	rows, err := tx.Query(ctx, `SELECT s.unit_id,t.surface,t.raw_lemma,t.canonical_lemma,t.upos,t.dependency,t.head,t.morphology,t.start_offset,t.end_offset
 FROM corpus_tokens t JOIN corpus_sentences s ON s.owner_id=t.owner_id AND s.analysis_run_id=t.analysis_run_id
  AND s.corpus_id=t.corpus_id AND s.sentence_ordinal=t.sentence_ordinal
 WHERE t.owner_id=$1 AND t.corpus_id=$2 AND t.sentence_ordinal=$3 ORDER BY t.token_ordinal`, owner, corpusID, ordinal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []analyzer.Token
	for rows.Next() {
		var unitID, surface, rawLemma, lemma, upos, dependency string
		var head, start, end int64
		var rawMorphology []byte
		if err := rows.Scan(&unitID, &surface, &rawLemma, &lemma, &upos, &dependency, &head, &rawMorphology, &start, &end); err != nil {
			return nil, err
		}
		headIndex, err := checked.Uint32FromInt64(head)
		if err != nil || start < 0 || end < 0 {
			return nil, errors.New("custom deck sentence contains invalid token offsets or head")
		}
		var morphology map[string]string
		if err := json.Unmarshal(rawMorphology, &morphology); err != nil {
			return nil, err
		}
		tokens = append(tokens, analyzer.Token{Surface: surface, RawLemma: rawLemma, CanonicalLemma: lemma, UPOS: upos, Dependency: dependency, Head: headIndex, Morphology: morphology, Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: uint64(start), EndOffset: uint64(end)}})
	}
	return tokens, rows.Err()
}

type customDeckOccurrence struct {
	BookID, SourceID, AnalysisRunID, CorpusID, Sentence, Surface, BookTitle, UnitID, AnalyzerLemma, RawLemma string
	SentenceOrdinal, Offset, EndOffset                                                                       int64
	Quality                                                                                                  cardexport.SentenceQuality
}

func chooseCustomDeckOccurrence(candidates []customDeckOccurrence) (customDeckOccurrence, bool) {
	accepted := candidates[:0]
	for _, candidate := range candidates {
		if candidate.Quality.Accepted {
			accepted = append(accepted, candidate)
		}
	}
	if len(accepted) == 0 {
		return customDeckOccurrence{}, false
	}
	sort.SliceStable(accepted, func(i, j int) bool {
		a, b := accepted[i], accepted[j]
		if a.Quality.GDEXScore != b.Quality.GDEXScore {
			return a.Quality.GDEXScore > b.Quality.GDEXScore
		}
		if a.BookID != b.BookID {
			return a.BookID < b.BookID
		}
		if a.SourceID != b.SourceID {
			return a.SourceID < b.SourceID
		}
		if a.Offset != b.Offset {
			return a.Offset < b.Offset
		}
		return a.Sentence < b.Sentence
	})
	return accepted[0], true
}
