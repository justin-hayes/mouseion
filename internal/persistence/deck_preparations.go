package persistence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// CompletePreparedDeck atomically stores the immutable artifact and every card
// and generated-vocabulary row. A failure rolls back all assignment state.
func (s *PostgresStore) CompletePreparedDeck(ctx context.Context, owner, id string, artifact cardexport.Artifact) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	ready, err := completePreparedDeckTx(ctx, tx, owner, id, artifact)
	if err != nil {
		return ready, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return ready, nil
}

// CompletePreparedDeckRun extends the existing atomic artifact boundary with a
// current-run and finalizer-token fence. A retry after commit is a no-op only
// when both the immutable artifact and completed run match.
func (s *PostgresStore) CompletePreparedDeckRun(ctx context.Context, owner, preparationID, runID, claimToken string, artifact cardexport.Artifact) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	preparation, err := sqlcgen.New(tx).GetDeckPreparationForUpdate(ctx, sqlcgen.GetDeckPreparationForUpdateParams{Owner: uuidArg(owner), ID: uuidArg(preparationID)})
	if err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	preparationState := domain.DeckPreparationState(preparation.State)
	currentRunID := uuidString(preparation.CurrentRunID)
	runModel, err := sqlcgen.New(tx).GetPreparedDeckRunForUpdate(ctx, sqlcgen.GetPreparedDeckRunForUpdateParams{OwnerID: uuidArg(owner), PreparationID: uuidArg(preparationID), ID: uuidArg(runID)})
	if err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	run := preparedDeckRunFromModel(runModel)
	if preparationState == domain.DeckPreparationReady && run.State == domain.PreparedDeckRunCompleted && currentRunID == runID {
		ready, completeErr := completePreparedDeckTx(ctx, tx, owner, preparationID, artifact)
		if completeErr != nil {
			return ready, completeErr
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.DeckPreparation{}, err
		}
		return ready, nil
	}
	leaseActive := runModel.FinalizationLeaseExpiresAt.Valid && runModel.FinalizationLeaseExpiresAt.Time.After(time.Now())
	if currentRunID != runID || run.State != domain.PreparedDeckRunFinalizing || run.TranslationState != domain.PreparedDeckTranslationCompleted || run.FinalizationClaimToken == "" || run.FinalizationClaimToken != claimToken || !leaseActive {
		return domain.DeckPreparation{}, ErrFenced
	}
	ready, err := completePreparedDeckTx(ctx, tx, owner, preparationID, artifact)
	if err != nil {
		return ready, err
	}
	if _, err = sqlcgen.New(tx).CompletePreparedDeckRun(ctx, sqlcgen.CompletePreparedDeckRunParams{OwnerID: uuidArg(owner), PreparationID: uuidArg(preparationID), ID: uuidArg(runID), FinalizationClaimToken: uuidArg(claimToken)}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DeckPreparation{}, ErrFenced
		}
		return domain.DeckPreparation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return ready, nil
}

func completePreparedDeckTx(ctx context.Context, tx pgx.Tx, owner, id string, artifact cardexport.Artifact) (domain.DeckPreparation, error) {
	q := sqlcgen.New(tx)
	current, err := q.GetDeckPreparationForUpdate(ctx, sqlcgen.GetDeckPreparationForUpdateParams{Owner: uuidArg(owner), ID: uuidArg(id)})
	if err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	state := domain.DeckPreparationState(current.State)
	if state == domain.DeckPreparationReady {
		model, getErr := q.GetDeckPreparation(ctx, sqlcgen.GetDeckPreparationParams{Owner: uuidArg(owner), ID: uuidArg(id)})
		p := deckPreparationFromModel(model)
		if getErr != nil {
			return p, getErr
		}
		if !bytes.Equal(p.Artifact, artifact.APKG) || p.Filename != artifact.Filename || p.DeckName != artifact.DeckName || p.TotalCards != artifact.Completeness.TotalCards || p.CardsWithEnglish != artifact.Completeness.CardsWithEnglish || p.CardsWithContextualSentenceTranslations != artifact.Completeness.CardsWithEnglishSentence || p.QualityOmissions != artifact.Completeness.QualityOmitted {
			return p, ErrImmutable
		}
		return p, nil
	}
	if state != domain.DeckPreparationPreparing {
		return domain.DeckPreparation{}, ErrInvalidTransition
	}
	for _, item := range artifact.Generated {
		entry, note := item.Entry, item.Note
		var vocabularyState string
		vocabularyState, err = q.GetVocabularyStateForUpdate(ctx, sqlcgen.GetVocabularyStateForUpdateParams{OwnerID: uuidArg(owner), Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS})
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DeckPreparation{}, ErrNotFound
		}
		if err != nil {
			return domain.DeckPreparation{}, err
		}
		if vocabularyState != "candidate" && vocabularyState != "accepted" && vocabularyState != "generated" {
			return domain.DeckPreparation{}, fmt.Errorf("cardexport: vocabulary state is %s", vocabularyState)
		}
		deck, err := q.PutDeck(ctx, sqlcgen.PutDeckParams{OwnerID: uuidArg(owner), Language: entry.Language, Name: note.BookTitle})
		if err != nil {
			return domain.DeckPreparation{}, err
		}
		deckID := deck.ID
		if err = q.PutPreparedDeckCard(ctx, sqlcgen.PutPreparedDeckCardParams{Owner: uuidArg(owner), Deck: uuidArg(deckID), DedupKey: note.Key, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, Front: note.Text, Back: note.BackExtra}); err != nil {
			return domain.DeckPreparation{}, err
		}
		if err = q.InsertGeneratedVocabulary(ctx, sqlcgen.InsertGeneratedVocabularyParams{Owner: uuidArg(owner), Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, Deck: uuidArg(deckID), Preparation: uuidArg(id)}); err != nil {
			return domain.DeckPreparation{}, err
		}
		if err = q.AttachDeckPreparationVocabulary(ctx, sqlcgen.AttachDeckPreparationVocabularyParams{Preparation: uuidArg(id), Owner: uuidArg(owner), Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS}); err != nil {
			return domain.DeckPreparation{}, err
		}
		if vocabularyState != "generated" {
			if err = q.SetVocabularyStateGenerated(ctx, sqlcgen.SetVocabularyStateGeneratedParams{Owner: uuidArg(owner), Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS}); err != nil {
				return domain.DeckPreparation{}, err
			}
			details, _ := json.Marshal(map[string]string{"language": entry.Language, "canonical_lemma": entry.CanonicalLemma, "upos": entry.UPOS, "from": vocabularyState, "to": "generated"})
			if err = q.InsertProcessingHistoryWithoutCorpus(ctx, sqlcgen.InsertProcessingHistoryWithoutCorpusParams{OwnerID: uuidArg(owner), Operation: "vocabulary.transition", Status: "completed", Details: details, CompletedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}}); err != nil {
				return domain.DeckPreparation{}, err
			}
		}
	}
	readyModel, err := q.CompleteDeckPreparation(ctx, sqlcgen.CompleteDeckPreparationParams{Artifact: artifact.APKG, Filename: artifact.Filename, DeckName: artifact.DeckName, TotalCards: int32(artifact.Completeness.TotalCards), CardsWithEnglish: int32(artifact.Completeness.CardsWithEnglish), CardsWithContextualSentenceTranslations: int32(artifact.Completeness.CardsWithEnglishSentence), QualityOmissions: int32(artifact.Completeness.QualityOmitted), Owner: uuidArg(owner), ID: uuidArg(id)})
	err = missing(err)
	ready := deckPreparationFromModel(readyModel)
	if err != nil {
		return ready, err
	}
	return ready, nil
}

// ListDeckPreparationVocabulary returns the immutable identity snapshot for a
// prepared deck. Only graduation timestamps can change after completion.
func (s *PostgresStore) ListDeckPreparationVocabulary(ctx context.Context, owner, preparationID string) ([]domain.DeckPreparationVocabulary, error) {
	rows, err := s.queries().ListDeckPreparationVocabulary(ctx, sqlcgen.ListDeckPreparationVocabularyParams{Owner: uuidArg(owner), Preparation: uuidArg(preparationID)})
	if err != nil {
		return nil, err
	}
	result := make([]domain.DeckPreparationVocabulary, 0, len(rows))
	for _, row := range rows {
		result = append(result, deckPreparationVocabularyFromModel(row))
	}
	return result, nil
}

// ListReservedVocabulary returns the vocabulary currently reserved by the
// owner's book-anchored study: the snapshotted vocabulary of every studying
// deck that has not yet graduated, scoped to one language. Reserved vocabulary
// is neither counted as known nor eligible for another deck until the study is
// resolved.
func (s *PostgresStore) ListReservedVocabulary(ctx context.Context, owner, language string) ([]domain.DeckPreparationVocabulary, error) {
	rows, err := s.queries().ListReservedDeckVocabulary(ctx, sqlcgen.ListReservedDeckVocabularyParams{Owner: uuidArg(owner), Language: language})
	if err != nil {
		return nil, err
	}
	result := make([]domain.DeckPreparationVocabulary, 0, len(rows))
	for _, row := range rows {
		result = append(result, deckPreparationVocabularyFromModel(row))
	}
	return result, nil
}

// CountDeckPreparationVocabularyToGraduate supports the consequential review
// confirmation without making the count authoritative for the transaction.
func (s *PostgresStore) CountDeckPreparationVocabularyToGraduate(ctx context.Context, owner, preparationID string) (int, error) {
	count, err := s.queries().CountDeckPreparationVocabularyToGraduate(ctx, sqlcgen.CountDeckPreparationVocabularyToGraduateParams{Owner: uuidArg(owner), Preparation: uuidArg(preparationID)})
	return int(count), err
}

// StartDeckVocabularyStudy reserves one ready, non-empty deck for its owner.
// The partial unique index enforces the owner-wide lease atomically.
func (s *PostgresStore) StartDeckVocabularyStudy(ctx context.Context, owner, preparationID string) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	q := sqlcgen.New(tx)
	currentModel, err := q.GetDeckPreparationForUpdate(ctx, sqlcgen.GetDeckPreparationForUpdateParams{Owner: uuidArg(owner), ID: uuidArg(preparationID)})
	if err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	current := deckPreparationFromModel(currentModel)
	if current.State != domain.DeckPreparationReady || current.TotalCards == 0 || current.GraduatedAt != nil || current.ReviewedAt != nil {
		return domain.DeckPreparation{}, ErrInvalidTransition
	}
	if current.StudyingAt != nil {
		return current, tx.Commit(ctx)
	}
	snapshotCount, err := q.CountDeckPreparationVocabulary(ctx, sqlcgen.CountDeckPreparationVocabularyParams{Owner: uuidArg(owner), Preparation: uuidArg(preparationID)})
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if snapshotCount == 0 {
		// Ready rows created before the snapshot write can be repaired from their
		// immutable generated-vocabulary provenance on first study.
		if err = q.RepairDeckPreparationVocabulary(ctx, sqlcgen.RepairDeckPreparationVocabularyParams{Preparation: uuidArg(preparationID), Owner: uuidArg(owner), SourceMaterial: uuidArg(current.SourceMaterialID)}); err != nil {
			return domain.DeckPreparation{}, err
		}
		snapshotCount, err = q.CountDeckPreparationVocabulary(ctx, sqlcgen.CountDeckPreparationVocabularyParams{Owner: uuidArg(owner), Preparation: uuidArg(preparationID)})
		if err != nil {
			return domain.DeckPreparation{}, err
		}
	}
	if snapshotCount == 0 {
		return domain.DeckPreparation{}, ErrInvalidTransition
	}
	updatedModel, err := q.StartDeckVocabularyStudy(ctx, sqlcgen.StartDeckVocabularyStudyParams{Owner: uuidArg(owner), Preparation: uuidArg(preparationID)})
	err = missing(err)
	updated := deckPreparationFromModel(updatedModel)
	if err != nil {
		if isConstraint(err, "deck_preparations_one_studying_per_owner") {
			return domain.DeckPreparation{}, ErrActiveVocabularyStudy
		}
		return domain.DeckPreparation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return updated, nil
}

// ConfirmDeckVocabularyReview graduates exactly the snapshot linked to this
// prepared deck and records the review in one transaction.
func (s *PostgresStore) ConfirmDeckVocabularyReview(ctx context.Context, owner, preparationID string) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	q := sqlcgen.New(tx)
	currentModel, err := q.GetDeckPreparationForUpdate(ctx, sqlcgen.GetDeckPreparationForUpdateParams{Owner: uuidArg(owner), ID: uuidArg(preparationID)})
	if err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	current := deckPreparationFromModel(currentModel)
	if current.GraduatedAt != nil {
		return current, tx.Commit(ctx)
	}
	if current.StudyingAt == nil {
		return domain.DeckPreparation{}, ErrInvalidTransition
	}
	if err = q.GraduateDeckPreparationVocabularyStates(ctx, sqlcgen.GraduateDeckPreparationVocabularyStatesParams{Owner: uuidArg(owner), Preparation: uuidArg(preparationID)}); err != nil {
		return domain.DeckPreparation{}, fmt.Errorf("graduate deck vocabulary state: %w", err)
	}
	if err = q.RecordGraduatedDeckVocabulary(ctx, sqlcgen.RecordGraduatedDeckVocabularyParams{Owner: uuidArg(owner), Preparation: uuidArg(preparationID)}); err != nil {
		return domain.DeckPreparation{}, fmt.Errorf("record graduated deck vocabulary: %w", err)
	}
	if err = q.MarkDeckPreparationVocabularyGraduated(ctx, sqlcgen.MarkDeckPreparationVocabularyGraduatedParams{Owner: uuidArg(owner), Preparation: uuidArg(preparationID)}); err != nil {
		return domain.DeckPreparation{}, err
	}
	updatedModel, err := q.ConfirmDeckVocabularyReview(ctx, sqlcgen.ConfirmDeckVocabularyReviewParams{Owner: uuidArg(owner), Preparation: uuidArg(preparationID)})
	err = missing(err)
	updated := deckPreparationFromModel(updatedModel)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return updated, nil
}

// ReleaseDeckVocabularyStudy releases an unfinished reservation without
// deleting its immutable snapshot or generated provenance.
func (s *PostgresStore) ReleaseDeckVocabularyStudy(ctx context.Context, owner, preparationID string) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	q := sqlcgen.New(tx)
	currentModel, err := q.GetDeckPreparationForUpdate(ctx, sqlcgen.GetDeckPreparationForUpdateParams{Owner: uuidArg(owner), ID: uuidArg(preparationID)})
	if err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	current := deckPreparationFromModel(currentModel)
	if current.GraduatedAt != nil || current.ReviewedAt != nil {
		return domain.DeckPreparation{}, ErrInvalidTransition
	}
	if current.StudyingAt == nil {
		return current, tx.Commit(ctx)
	}
	updatedModel, err := q.ReleaseDeckVocabularyStudy(ctx, sqlcgen.ReleaseDeckVocabularyStudyParams{Owner: uuidArg(owner), Preparation: uuidArg(preparationID)})
	err = missing(err)
	updated := deckPreparationFromModel(updatedModel)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return updated, nil
}

func isConstraint(err error, name string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.ConstraintName == name
}

// CreateDeckPreparation creates the current preparation for a Book. A new
// analysis retires the prior current row, while retries of the same analysis
// return its existing row. Unlinked legacy sources retain their old identity.
func (s *PostgresStore) CreateDeckPreparation(ctx context.Context, p domain.DeckPreparation) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	created, _, err := CreateDeckPreparationTx(ctx, tx, p)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return created, nil
}

// CreateDeckPreparationTx is the transactional form used by the River
// submission path so retirement, preparation creation, and job insertion share
// one commit boundary.
func CreateDeckPreparationTx(ctx context.Context, tx pgx.Tx, p domain.DeckPreparation) (domain.DeckPreparation, bool, error) {
	// These row-locking queries fence the book/source identity while the
	// preparation is created.
	bookID, err := sqlcgen.New(tx).GetSourceMaterialBookForUpdate(ctx, sqlcgen.GetSourceMaterialBookForUpdateParams{OwnerID: uuidArg(p.OwnerID), ID: uuidArg(p.SourceMaterialID)})
	if err != nil {
		return domain.DeckPreparation{}, false, missing(err)
	}
	if bookID != "" {
		if _, err := sqlcgen.New(tx).GetBookForUpdate(ctx, sqlcgen.GetBookForUpdateParams{Owner: uuidArg(p.OwnerID), ID: uuidArg(bookID)}); err != nil {
			return domain.DeckPreparation{}, false, missing(err)
		}
		var existing domain.DeckPreparation
		if p.AnalysisRunID != "" {
			model, queryErr := sqlcgen.New(tx).GetUnretiredDeckPreparationBySourceAnalysis(ctx, sqlcgen.GetUnretiredDeckPreparationBySourceAnalysisParams{Owner: uuidArg(p.OwnerID), SourceMaterial: uuidArg(p.SourceMaterialID), AnalysisRun: uuidArg(p.AnalysisRunID)})
			existing, err = deckPreparationFromModel(model), missing(queryErr)
		} else {
			model, queryErr := sqlcgen.New(tx).GetUnretiredDeckPreparationBySourceHash(ctx, sqlcgen.GetUnretiredDeckPreparationBySourceHashParams{Owner: uuidArg(p.OwnerID), SourceMaterial: uuidArg(p.SourceMaterialID), ContentHash: p.ContentHash})
			existing, err = deckPreparationFromModel(model), missing(queryErr)
		}
		if err == nil {
			return existing, false, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return domain.DeckPreparation{}, false, err
		}
		if err = sqlcgen.New(tx).RetireDeckPreparationsForBook(ctx, sqlcgen.RetireDeckPreparationsForBookParams{Owner: uuidArg(p.OwnerID), Book: uuidArg(bookID)}); err != nil {
			return domain.DeckPreparation{}, false, err
		}
		model, err := sqlcgen.New(tx).CreateDeckPreparation(ctx, sqlcgen.CreateDeckPreparationParams{Owner: uuidArg(p.OwnerID), SourceMaterial: uuidArg(p.SourceMaterialID), BookID: uuidArg(bookID), AnalysisRun: nullableUUIDArg(p.AnalysisRunID), Filename: p.Filename, DeckName: p.DeckName, ContentHash: p.ContentHash})
		return deckPreparationFromModel(model), true, err
	}

	var existing domain.DeckPreparation
	if p.AnalysisRunID != "" {
		model, queryErr := sqlcgen.New(tx).GetDeckPreparationBySourceAnalysis(ctx, sqlcgen.GetDeckPreparationBySourceAnalysisParams{Owner: uuidArg(p.OwnerID), SourceMaterial: uuidArg(p.SourceMaterialID), AnalysisRun: uuidArg(p.AnalysisRunID)})
		existing, err = deckPreparationFromModel(model), missing(queryErr)
	} else {
		model, queryErr := sqlcgen.New(tx).GetDeckPreparationBySourceHashWithoutAnalysis(ctx, sqlcgen.GetDeckPreparationBySourceHashWithoutAnalysisParams{Owner: uuidArg(p.OwnerID), SourceMaterial: uuidArg(p.SourceMaterialID), ContentHash: p.ContentHash})
		existing, err = deckPreparationFromModel(model), missing(queryErr)
	}
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return domain.DeckPreparation{}, false, err
	}
	model, err := sqlcgen.New(tx).CreateDeckPreparation(ctx, sqlcgen.CreateDeckPreparationParams{Owner: uuidArg(p.OwnerID), SourceMaterial: uuidArg(p.SourceMaterialID), AnalysisRun: nullableUUIDArg(p.AnalysisRunID), Filename: p.Filename, DeckName: p.DeckName, ContentHash: p.ContentHash})
	return deckPreparationFromModel(model), true, err
}

// ClaimDeckPreparation atomically grants one worker the queued preparation.
func (s *PostgresStore) ClaimDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	model, err := s.queries().ClaimDeckPreparation(ctx, sqlcgen.ClaimDeckPreparationParams{Owner: uuidArg(owner), ID: uuidArg(id)})
	err = missing(err)
	p := deckPreparationFromModel(model)
	if !errors.Is(err, ErrNotFound) {
		return p, err
	}
	if _, getErr := s.GetDeckPreparation(ctx, owner, id); getErr != nil {
		return p, getErr
	}
	return p, ErrInvalidTransition
}

// CompleteDeckPreparation stores the final artifact exactly once. Repeating
// the identical completion is idempotent; any different ready value is rejected.
func (s *PostgresStore) CompleteDeckPreparation(ctx context.Context, owner, id string, ready domain.DeckPreparation) (domain.DeckPreparation, error) {
	model, err := s.queries().CompleteDeckPreparation(ctx, sqlcgen.CompleteDeckPreparationParams{Artifact: ready.Artifact, Filename: ready.Filename, DeckName: ready.DeckName, TotalCards: int32(ready.TotalCards), CardsWithEnglish: int32(ready.CardsWithEnglish), CardsWithContextualSentenceTranslations: int32(ready.CardsWithContextualSentenceTranslations), QualityOmissions: int32(ready.QualityOmissions), Owner: uuidArg(owner), ID: uuidArg(id)})
	err = missing(err)
	p := deckPreparationFromModel(model)
	if !errors.Is(err, ErrNotFound) {
		return p, err
	}
	existing, getErr := s.GetDeckPreparation(ctx, owner, id)
	if getErr != nil {
		return p, getErr
	}
	if existing.State != domain.DeckPreparationReady {
		return p, ErrInvalidTransition
	}
	if bytes.Equal(existing.Artifact, ready.Artifact) && existing.Filename == ready.Filename && existing.DeckName == ready.DeckName && existing.TotalCards == ready.TotalCards && existing.CardsWithEnglish == ready.CardsWithEnglish && existing.CardsWithContextualSentenceTranslations == ready.CardsWithContextualSentenceTranslations && existing.QualityOmissions == ready.QualityOmissions {
		return existing, nil
	}
	return p, ErrImmutable
}

func (s *PostgresStore) FailDeckPreparation(ctx context.Context, owner, id, message string) (domain.DeckPreparation, error) {
	return s.transitionDeckPreparation(ctx, owner, id, domain.DeckPreparationFailed, message, "preparing")
}

// FailPreparedDeckFinalization fences a completeness failure after a run has
// advanced to finalizing. No artifact is written, and the public preparation
// receives only a bounded message.
func (s *PostgresStore) FailPreparedDeckFinalization(ctx context.Context, owner, preparationID, runID, claimToken, errorClass, errorCode string) error {
	if err := validateBoundedError(errorClass, errorCode); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = sqlcgen.New(tx).FailPreparedDeckFinalizationRun(ctx, sqlcgen.FailPreparedDeckFinalizationRunParams{OwnerID: uuidArg(owner), PreparationID: uuidArg(preparationID), ID: uuidArg(runID), FinalizationClaimToken: uuidArg(claimToken), ErrorClass: errorClass, ErrorCode: errorCode}); err != nil {
		return missing(err)
	}
	if _, err = sqlcgen.New(tx).FailDeckPreparationTranslation(ctx, sqlcgen.FailDeckPreparationTranslationParams{OwnerID: uuidArg(owner), ID: uuidArg(preparationID), CurrentRunID: uuidArg(runID), Error: "prepared-deck translation was incomplete"}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) CancelDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	return s.CancelCurrentPreparedDeckRun(ctx, owner, id)
}

func (s *PostgresStore) RetryDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	return s.transitionDeckPreparation(ctx, owner, id, domain.DeckPreparationQueued, "", "failed", "cancelled")
}

func (s *PostgresStore) transitionDeckPreparation(ctx context.Context, owner, id string, next domain.DeckPreparationState, message string, from ...string) (domain.DeckPreparation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	model, err := sqlcgen.New(tx).TransitionDeckPreparation(ctx, sqlcgen.TransitionDeckPreparationParams{NextState: string(next), Message: message, Owner: uuidArg(owner), ID: uuidArg(id), FromStates: from})
	err = missing(err)
	p := deckPreparationFromModel(model)
	if errors.Is(err, ErrNotFound) {
		_ = tx.Rollback(ctx)
		if _, getErr := s.GetDeckPreparation(ctx, owner, id); getErr != nil {
			return p, getErr
		}
		return p, ErrInvalidTransition
	}
	if err != nil {
		return p, err
	}
	details, marshalErr := json.Marshal(map[string]any{"preparation_id": id, "from": from, "message": message})
	if marshalErr != nil {
		return p, marshalErr
	}
	if err = sqlcgen.New(tx).InsertDeckPreparationHistory(ctx, sqlcgen.InsertDeckPreparationHistoryParams{Owner: uuidArg(owner), Status: string(next), Details: details}); err != nil {
		return p, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return p, nil
}

func (s *PostgresStore) GetDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	model, err := s.queries().GetDeckPreparation(ctx, sqlcgen.GetDeckPreparationParams{Owner: uuidArg(owner), ID: uuidArg(id)})
	return deckPreparationFromModel(model), missing(err)
}

// GetDeckPreparationForAnalysis returns the one owner-scoped preparation
// associated with an exact completed analysis. The unique preparation identity
// makes this lookup safe for result pages and prevents a mutable latest-deck
// lookup from selecting the wrong analysis.
func (s *PostgresStore) GetDeckPreparationForAnalysis(ctx context.Context, owner, sourceMaterialID, analysisRunID string) (domain.DeckPreparation, error) {
	model, err := s.queries().GetDeckPreparationForAnalysis(ctx, sqlcgen.GetDeckPreparationForAnalysisParams{Owner: uuidArg(owner), SourceMaterial: uuidArg(sourceMaterialID), AnalysisRun: uuidArg(analysisRunID)})
	return deckPreparationFromModel(model), missing(err)
}

func (s *PostgresStore) GetActiveDeckVocabularyStudy(ctx context.Context, owner, sourceMaterialID string) (domain.DeckPreparation, error) {
	model, err := s.queries().GetActiveDeckVocabularyStudy(ctx, sqlcgen.GetActiveDeckVocabularyStudyParams{Owner: uuidArg(owner), SourceMaterial: uuidArg(sourceMaterialID)})
	return deckPreparationFromModel(model), missing(err)
}

// ListDeckPreparationsForSourceMaterial returns the owner's preparation history
// for one Book's acquired source, including released and graduated studies.
func (s *PostgresStore) ListDeckPreparationsForSourceMaterial(ctx context.Context, owner, sourceMaterialID string) ([]domain.DeckPreparation, error) {
	rows, err := s.queries().ListDeckPreparationsForSourceMaterial(ctx, sqlcgen.ListDeckPreparationsForSourceMaterialParams{Owner: uuidArg(owner), SourceMaterial: uuidArg(sourceMaterialID)})
	if err != nil {
		return nil, err
	}
	preparations := make([]domain.DeckPreparation, 0, len(rows))
	for _, row := range rows {
		preparations = append(preparations, deckPreparationFromModel(row))
	}
	return preparations, nil
}

// DownloadDeckPreparation returns bytes only for a ready owner-scoped row.
func (s *PostgresStore) DownloadDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	model, err := s.queries().DownloadDeckPreparation(ctx, sqlcgen.DownloadDeckPreparationParams{Owner: uuidArg(owner), ID: uuidArg(id)})
	err = missing(err)
	p := deckPreparationFromModel(model)
	if errors.Is(err, ErrNotFound) {
		var exists bool
		exists, checkErr := s.queries().DeckPreparationExists(ctx, sqlcgen.DeckPreparationExistsParams{Owner: uuidArg(owner), ID: uuidArg(id)})
		if checkErr != nil {
			return p, checkErr
		}
		if exists {
			return p, ErrInvalidTransition
		}
	}
	return p, err
}
