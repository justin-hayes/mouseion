package persistence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// CompletePreparedDeck atomically stores the immutable artifact and every card
// and generated-vocabulary row. A failure rolls back all assignment state.
func (s *PostgresStore) CompletePreparedDeck(ctx context.Context, owner, id string, artifact cardexport.Artifact) (result domain.DeckPreparation, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	ready, err := completePreparedDeckTx(ctx, tx, owner, id, artifact, cardexport.RenderInputVersion, cardexport.PresentationVersion)
	if err != nil {
		return ready, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return ready, nil
}

// CompletePreparedDeckRun extends the existing atomic artifact boundary with a
// current-run and finalizer-token fence. A retry after commit is a no-op only
// when both the immutable artifact and completed run match.
func (s *PostgresStore) CompletePreparedDeckRun(ctx context.Context, owner, preparationID, runID, claimToken string, artifact cardexport.Artifact) (result domain.DeckPreparation, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	ready, err := completePreparedDeckRunTx(ctx, tx, owner, preparationID, runID, claimToken, artifact)
	if err != nil {
		return ready, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return ready, nil
}

// completePreparedDeckRunTx locks the preparation then its run, completes the
// artifact for the run's versions, and advances the run. An already-committed
// run is returned unchanged; any mismatch in the finalization claim is fenced.
func completePreparedDeckRunTx(ctx context.Context, tx pgx.Tx, owner, preparationID, runID, claimToken string, artifact cardexport.Artifact) (domain.DeckPreparation, error) {
	q := sqlcgen.New(tx)
	preparationModel, err := q.GetDeckPreparationForUpdate(ctx, sqlcgen.GetDeckPreparationForUpdateParams{Owner: owner, ID: preparationID})
	if err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	preparationState := domain.DeckPreparationState(preparationModel.State)
	currentRunID := uuidString(preparationModel.CurrentRunID)
	runModel, err := q.GetPreparedDeckRunForUpdate(ctx, sqlcgen.GetPreparedDeckRunForUpdateParams{OwnerID: owner, PreparationID: preparationID, ID: runID})
	if err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	run := preparedDeckRunFromModel(runModel)
	if preparationState == domain.DeckPreparationReady && run.State == domain.PreparedDeckRunCompleted && currentRunID == runID {
		return completePreparedDeckTx(ctx, tx, owner, preparationID, artifact, run.RenderInputVersion, run.PresentationVersion)
	}
	leaseActive := runModel.FinalizationLeaseExpiresAt.Valid && runModel.FinalizationLeaseExpiresAt.Time.After(time.Now())
	if !preparedDeckFinalizationClaimsRun(currentRunID, runID, run, claimToken, leaseActive) {
		return domain.DeckPreparation{}, ErrFenced
	}
	ready, err := completePreparedDeckTx(ctx, tx, owner, preparationID, artifact, run.RenderInputVersion, run.PresentationVersion)
	if err != nil {
		return ready, err
	}
	if _, err = q.CompletePreparedDeckRun(ctx, sqlcgen.CompletePreparedDeckRunParams{OwnerID: owner, PreparationID: preparationID, ID: runID, FinalizationClaimToken: uuidArg(claimToken)}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DeckPreparation{}, ErrFenced
		}
		return domain.DeckPreparation{}, err
	}
	return ready, nil
}

// preparedDeckFinalizationClaimsRun reports whether the stored run still
// belongs to the claiming run: the current run, finalizing state, completed
// translation, claim token, and an unexpired lease must all match.
func preparedDeckFinalizationClaimsRun(currentRunID, runID string, run domain.PreparedDeckRun, claimToken string, leaseActive bool) bool {
	if currentRunID != runID || run.State != domain.PreparedDeckRunFinalizing || run.TranslationState != domain.PreparedDeckTranslationCompleted {
		return false
	}
	if run.FinalizationClaimToken == "" || run.FinalizationClaimToken != claimToken {
		return false
	}
	return leaseActive
}

// SupersedePreparedDeckArtifact replaces one completed run's presentation in
// place. The run-version predicate and preparation lock make concurrent or
// repeated rerenders publish one revision at most.
func (s *PostgresStore) SupersedePreparedDeckArtifact(ctx context.Context, owner, preparationID, runID string, presentationVersion int, artifact cardexport.Artifact) (result domain.DeckPreparation, err error) {
	if presentationVersion < 1 {
		return domain.DeckPreparation{}, ErrInvalidTransition
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	updated, err := supersedePreparedDeckArtifactTx(ctx, tx, owner, preparationID, runID, presentationVersion, artifact)
	if err != nil {
		return updated, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return updated, nil
}

// supersedePreparedDeckArtifactTx checks the run's current presentation before
// writing a replacement. An up-to-date run is returned unchanged.
func supersedePreparedDeckArtifactTx(ctx context.Context, tx pgx.Tx, owner, preparationID, runID string, presentationVersion int, artifact cardexport.Artifact) (domain.DeckPreparation, error) {
	q := sqlcgen.New(tx)
	preparationModel, err := q.GetDeckPreparationForUpdate(ctx, sqlcgen.GetDeckPreparationForUpdateParams{Owner: owner, ID: preparationID})
	if err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	preparation := deckPreparationFromModel(preparationModel)
	runModel, err := q.GetPreparedDeckRunForUpdate(ctx, sqlcgen.GetPreparedDeckRunForUpdateParams{OwnerID: owner, PreparationID: preparationID, ID: runID})
	if err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	run := preparedDeckRunFromModel(runModel)
	nextPresentationVersion, upToDate, err := preparedDeckRerenderDecision(preparation, run, runID, presentationVersion)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if upToDate {
		return preparation, nil
	}
	if artifact.Filename != preparation.Filename || artifact.DeckName != preparation.DeckName {
		return domain.DeckPreparation{}, fmt.Errorf("%w: rerendered artifact identity changed", ErrPreparedDeckIdentity)
	}
	if err := putPreparedDeckArtifactCards(ctx, q, owner, artifact); err != nil {
		return domain.DeckPreparation{}, err
	}
	return writeSupersededPreparedDeckArtifact(ctx, q, owner, preparationID, runID, run, nextPresentationVersion, artifact)
}

// preparedDeckRerenderDecision validates that a completed run can be
// superseded and returns the presentation version to publish. upToDate reports
// that the stored run already meets or exceeds the requested versions.
func preparedDeckRerenderDecision(preparation domain.DeckPreparation, run domain.PreparedDeckRun, runID string, presentationVersion int) (int, bool, error) {
	if preparation.CurrentRunID != runID || preparation.State != domain.DeckPreparationReady || preparation.RetiredAt != nil || run.State != domain.PreparedDeckRunCompleted {
		return 0, false, ErrInvalidTransition
	}
	if run.PresentationVersion > presentationVersion {
		presentationVersion = run.PresentationVersion
	}
	upToDate := run.PresentationVersion >= presentationVersion && run.RenderInputVersion >= cardexport.RenderInputVersion
	return presentationVersion, upToDate, nil
}

// putPreparedDeckArtifactCards republishes every generated card through the
// deck and card upserts a rerender owns.
func putPreparedDeckArtifactCards(ctx context.Context, q *sqlcgen.Queries, owner string, artifact cardexport.Artifact) error {
	for _, item := range artifact.Generated {
		if _, err := putPreparedDeckCardForRecord(ctx, q, owner, item); err != nil {
			return err
		}
	}
	return nil
}

// writeSupersededPreparedDeckArtifact advances the run's presentation revision,
// treating a lost version predicate as a lost claim.
func writeSupersededPreparedDeckArtifact(ctx context.Context, q *sqlcgen.Queries, owner, preparationID, runID string, run domain.PreparedDeckRun, presentationVersion int, artifact cardexport.Artifact) (domain.DeckPreparation, error) {
	updatedModel, err := q.SupersedePreparedDeckArtifact(ctx, sqlcgen.SupersedePreparedDeckArtifactParams{
		Artifact: artifact.APKG, TotalCards: artifact.Completeness.TotalCards,
		CardsWithEnglish:                        artifact.Completeness.CardsWithEnglish,
		CardsWithContextualSentenceTranslations: artifact.Completeness.CardsWithEnglishSentence,
		CardsWithFallbackGloss:                  artifact.Completeness.CardsWithFallbackGloss,
		QualityOmissions:                        artifact.Completeness.QualityOmitted,
		RenderInputVersion:                      max(run.RenderInputVersion, cardexport.RenderInputVersion), PresentationVersion: presentationVersion,
		Owner: owner, Preparation: preparationID, Run: runID,
		ExpectedPresentationVersion: run.PresentationVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeckPreparation{}, ErrPreparedDeckClaimLost
	}
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	return deckPreparationFromModel(updatedModel), nil
}

// putPreparedDeckCardForRecord upserts the deck named by one generated record
// and its prepared-deck card, returning the deck identity.
func putPreparedDeckCardForRecord(ctx context.Context, q *sqlcgen.Queries, owner string, item cardexport.GeneratedRecord) (string, error) {
	deck, err := q.PutDeck(ctx, sqlcgen.PutDeckParams{OwnerID: owner, Language: item.Input.Language, Name: item.Note.BookTitle})
	if err != nil {
		return "", err
	}
	if err := q.PutPreparedDeckCard(ctx, sqlcgen.PutPreparedDeckCardParams{Owner: owner, Deck: deck.ID, DedupKey: item.Note.Key, CanonicalLemma: item.Input.CanonicalLemma, Upos: item.Input.UPOS, Front: item.Note.Text, Back: item.Note.BackExtra}); err != nil {
		return "", err
	}
	return deck.ID, nil
}

func completePreparedDeckTx(ctx context.Context, tx pgx.Tx, owner, id string, artifact cardexport.Artifact, renderInputVersion, presentationVersion int) (domain.DeckPreparation, error) {
	q := sqlcgen.New(tx)
	current, err := q.GetDeckPreparationForUpdate(ctx, sqlcgen.GetDeckPreparationForUpdateParams{Owner: owner, ID: id})
	if err != nil {
		return domain.DeckPreparation{}, missing(err)
	}
	switch domain.DeckPreparationState(current.State) {
	case domain.DeckPreparationReady:
		return completedPreparedDeckResult(ctx, q, owner, id, artifact, renderInputVersion, presentationVersion)
	case domain.DeckPreparationPreparing:
		if err := publishPreparedDeckArtifact(ctx, q, owner, id, artifact); err != nil {
			return domain.DeckPreparation{}, err
		}
		return writeCompletedDeckPreparation(ctx, q, owner, id, artifact, renderInputVersion, presentationVersion)
	default:
		return domain.DeckPreparation{}, ErrInvalidTransition
	}
}

// completedPreparedDeckResult returns the stored ready preparation when the
// repeated completion matches it exactly, and ErrImmutable when any published
// value differs.
func completedPreparedDeckResult(ctx context.Context, q *sqlcgen.Queries, owner, id string, artifact cardexport.Artifact, renderInputVersion, presentationVersion int) (domain.DeckPreparation, error) {
	model, err := q.GetDeckPreparation(ctx, sqlcgen.GetDeckPreparationParams{Owner: owner, ID: id})
	p := deckPreparationFromModel(model)
	if err != nil {
		return p, err
	}
	if !preparedDeckCompletionMatches(p, artifact, renderInputVersion, presentationVersion) {
		return p, ErrImmutable
	}
	return p, nil
}

// preparedDeckCompletionMatches reports whether a ready preparation already
// holds exactly the artifact and versions a repeated completion supplies.
func preparedDeckCompletionMatches(p domain.DeckPreparation, artifact cardexport.Artifact, renderInputVersion, presentationVersion int) bool {
	return bytes.Equal(p.Artifact, artifact.APKG) &&
		p.Filename == artifact.Filename &&
		p.DeckName == artifact.DeckName &&
		p.TotalCards == artifact.Completeness.TotalCards &&
		p.CardsWithEnglish == artifact.Completeness.CardsWithEnglish &&
		p.CardsWithContextualSentenceTranslations == artifact.Completeness.CardsWithEnglishSentence &&
		p.CardsWithFallbackGloss == artifact.Completeness.CardsWithFallbackGloss &&
		p.QualityOmissions == artifact.Completeness.QualityOmitted &&
		p.RenderInputVersion == renderInputVersion &&
		p.PresentationVersion == presentationVersion
}

// publishPreparedDeckArtifact writes every generated card, its generated
// vocabulary provenance, and the preparation's vocabulary assignment.
func publishPreparedDeckArtifact(ctx context.Context, q *sqlcgen.Queries, owner, id string, artifact cardexport.Artifact) error {
	for _, item := range artifact.Generated {
		entry := item.Input
		deckID, err := putPreparedDeckCardForRecord(ctx, q, owner, item)
		if err != nil {
			return err
		}
		if err := q.InsertGeneratedVocabulary(ctx, sqlcgen.InsertGeneratedVocabularyParams{Owner: owner, Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS, Deck: deckID, Preparation: id}); err != nil {
			return err
		}
		if err := q.AttachDeckPreparationVocabulary(ctx, sqlcgen.AttachDeckPreparationVocabularyParams{Preparation: id, Owner: owner, Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, Upos: entry.UPOS}); err != nil {
			return err
		}
	}
	return nil
}

// writeCompletedDeckPreparation stores the final artifact exactly once and
// returns the ready preparation.
func writeCompletedDeckPreparation(ctx context.Context, q *sqlcgen.Queries, owner, id string, artifact cardexport.Artifact, renderInputVersion, presentationVersion int) (domain.DeckPreparation, error) {
	readyModel, err := q.CompleteDeckPreparation(ctx, sqlcgen.CompleteDeckPreparationParams{Artifact: artifact.APKG, Filename: artifact.Filename, DeckName: artifact.DeckName, TotalCards: artifact.Completeness.TotalCards, CardsWithEnglish: artifact.Completeness.CardsWithEnglish, CardsWithContextualSentenceTranslations: artifact.Completeness.CardsWithEnglishSentence, CardsWithFallbackGloss: artifact.Completeness.CardsWithFallbackGloss, QualityOmissions: artifact.Completeness.QualityOmitted, RenderInputVersion: renderInputVersion, PresentationVersion: presentationVersion, Owner: owner, ID: id})
	err = missing(err)
	ready := deckPreparationFromModel(readyModel)
	return ready, err
}

// ListDeckPreparationVocabulary returns the immutable identity snapshot for a
// prepared deck. Only graduation timestamps can change after completion.
func (s *PostgresStore) ListDeckPreparationVocabulary(ctx context.Context, owner, preparationID string) ([]domain.DeckPreparationVocabulary, error) {
	rows, err := s.queries().ListDeckPreparationVocabulary(ctx, sqlcgen.ListDeckPreparationVocabularyParams{Owner: owner, Preparation: preparationID})
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

// ListReservedVocabulary returns the vocabulary currently reserved by the
// owner's active reading snapshot, scoped to one language. Reserved vocabulary is
// neither counted as known nor eligible for another deck until the reading is
// changed, cleared, or completed.
func (s *PostgresStore) ListReservedVocabulary(ctx context.Context, owner, language string) ([]domain.DeckPreparationVocabulary, error) {
	rows, err := s.queries().ListReservedDeckVocabulary(ctx, sqlcgen.ListReservedDeckVocabularyParams{Owner: owner, Language: language})
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

// CreateDeckPreparation creates the current preparation for a Book. A new
// analysis retires the prior current row, while retries of the same analysis
// return its existing row. Unlinked legacy sources retain their old identity.
func (s *PostgresStore) CreateDeckPreparation(ctx context.Context, p domain.DeckPreparation) (result domain.DeckPreparation, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	created, _, err := CreateDeckPreparationTx(ctx, tx, p)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return created, nil
}

// LockSourceMaterialBookForUpdateTx resolves and locks the Book that owns the
// source material, so the source-to-Book identity cannot change while the
// caller writes. An unlinked legacy source returns an empty Book ID; a missing
// source material is ErrNotFound.
func (s *PostgresStore) LockSourceMaterialBookForUpdateTx(ctx context.Context, tx pgx.Tx, owner, sourceMaterialID string) (string, error) {
	return lockSourceMaterialBookForUpdate(ctx, sqlcgen.New(tx), owner, sourceMaterialID)
}

// RetireDeckPreparationsForBookTx retires every unretired preparation the Book
// owns. A malformed Book identity is an error rather than a silent no-op, so a
// bad request cannot retire nothing and report success.
func (s *PostgresStore) RetireDeckPreparationsForBookTx(ctx context.Context, tx pgx.Tx, owner, bookID string) error {
	return retireDeckPreparationsForBook(ctx, sqlcgen.New(tx), owner, bookID)
}

func lockSourceMaterialBookForUpdate(ctx context.Context, q *sqlcgen.Queries, owner, sourceMaterialID string) (string, error) {
	bookID, err := q.GetSourceMaterialBookForUpdate(ctx, sqlcgen.GetSourceMaterialBookForUpdateParams{OwnerID: owner, ID: sourceMaterialID})
	return bookID, missing(err)
}

func retireDeckPreparationsForBook(ctx context.Context, q *sqlcgen.Queries, owner, bookID string) error {
	parsed, err := uuid.Parse(bookID)
	if err != nil {
		return fmt.Errorf("invalid Current reading book identity: %w", err)
	}
	return q.RetireDeckPreparationsForBook(ctx, sqlcgen.RetireDeckPreparationsForBookParams{Owner: owner, Book: pgtype.UUID{Bytes: parsed, Valid: true}})
}

// CreateDeckPreparationTx is the transactional form used by the River
// submission path so retirement, preparation creation, and job insertion share
// one commit boundary.
func CreateDeckPreparationTx(ctx context.Context, tx pgx.Tx, p domain.DeckPreparation) (domain.DeckPreparation, bool, error) {
	// These row-locking queries fence the book/source identity while the
	// preparation is created.
	q := sqlcgen.New(tx)
	bookID, err := lockSourceMaterialBookForUpdate(ctx, q, p.OwnerID, p.SourceMaterialID)
	if err != nil {
		return domain.DeckPreparation{}, false, err
	}
	if bookID != "" {
		return createBookDeckPreparationTx(ctx, q, p, bookID)
	}
	return createUnownedDeckPreparationTx(ctx, q, p)
}

// createBookDeckPreparationTx reuses the Book's unretired preparation for this
// analysis (or content hash), or retires the Book's current preparations and
// creates the new one.
func createBookDeckPreparationTx(ctx context.Context, q *sqlcgen.Queries, p domain.DeckPreparation, bookID string) (domain.DeckPreparation, bool, error) {
	if _, err := q.GetBookForUpdate(ctx, sqlcgen.GetBookForUpdateParams{Owner: p.OwnerID, ID: bookID}); err != nil {
		return domain.DeckPreparation{}, false, missing(err)
	}
	existing, found, err := findUnretiredDeckPreparationTx(ctx, q, p, true)
	if err != nil {
		return domain.DeckPreparation{}, false, err
	}
	if found {
		return existing, false, nil
	}
	if err := retireDeckPreparationsForBook(ctx, q, p.OwnerID, bookID); err != nil {
		return domain.DeckPreparation{}, false, err
	}
	model, err := q.CreateDeckPreparation(ctx, sqlcgen.CreateDeckPreparationParams{Owner: p.OwnerID, SourceMaterial: p.SourceMaterialID, BookID: uuidArg(bookID), AnalysisRun: nullableUUIDArg(p.AnalysisRunID), Snapshot: nullableUUIDArg(p.SnapshotID), Filename: p.Filename, DeckName: p.DeckName, ContentHash: p.ContentHash})
	return deckPreparationFromModel(model), true, err
}

// createUnownedDeckPreparationTx reuses an unlinked legacy source's existing
// preparation for this analysis (or content hash), or creates the first one.
func createUnownedDeckPreparationTx(ctx context.Context, q *sqlcgen.Queries, p domain.DeckPreparation) (domain.DeckPreparation, bool, error) {
	existing, found, err := findUnretiredDeckPreparationTx(ctx, q, p, false)
	if err != nil {
		return domain.DeckPreparation{}, false, err
	}
	if found {
		return existing, false, nil
	}
	model, err := q.CreateDeckPreparation(ctx, sqlcgen.CreateDeckPreparationParams{Owner: p.OwnerID, SourceMaterial: p.SourceMaterialID, AnalysisRun: nullableUUIDArg(p.AnalysisRunID), Snapshot: nullableUUIDArg(p.SnapshotID), Filename: p.Filename, DeckName: p.DeckName, ContentHash: p.ContentHash})
	return deckPreparationFromModel(model), true, err
}

// findUnretiredDeckPreparationTx reports whether the source already has an
// unretired preparation matching this preparation's analysis run, or its
// content hash when the preparation has no analysis. A missing row is not an
// error; any other lookup failure propagates.
func findUnretiredDeckPreparationTx(ctx context.Context, q *sqlcgen.Queries, p domain.DeckPreparation, bookScoped bool) (domain.DeckPreparation, bool, error) {
	model, err := lookupDeckPreparationForSource(ctx, q, p, bookScoped)
	existing := deckPreparationFromModel(model)
	err = missing(err)
	if err == nil {
		return existing, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return domain.DeckPreparation{}, false, err
	}
	return domain.DeckPreparation{}, false, nil
}

// lookupDeckPreparationForSource selects the existing-preparation query for a
// Book-linked or unlinked source.
func lookupDeckPreparationForSource(ctx context.Context, q *sqlcgen.Queries, p domain.DeckPreparation, bookScoped bool) (sqlcgen.DeckPreparation, error) {
	switch {
	case bookScoped && p.AnalysisRunID != "":
		return q.GetUnretiredDeckPreparationBySourceAnalysis(ctx, sqlcgen.GetUnretiredDeckPreparationBySourceAnalysisParams{Owner: p.OwnerID, SourceMaterial: p.SourceMaterialID, AnalysisRun: uuidArg(p.AnalysisRunID)})
	case bookScoped:
		return q.GetUnretiredDeckPreparationBySourceHash(ctx, sqlcgen.GetUnretiredDeckPreparationBySourceHashParams{Owner: p.OwnerID, SourceMaterial: p.SourceMaterialID, ContentHash: p.ContentHash})
	case p.AnalysisRunID != "":
		return q.GetDeckPreparationBySourceAnalysis(ctx, sqlcgen.GetDeckPreparationBySourceAnalysisParams{Owner: p.OwnerID, SourceMaterial: p.SourceMaterialID, AnalysisRun: uuidArg(p.AnalysisRunID)})
	default:
		return q.GetDeckPreparationBySourceHashWithoutAnalysis(ctx, sqlcgen.GetDeckPreparationBySourceHashWithoutAnalysisParams{Owner: p.OwnerID, SourceMaterial: p.SourceMaterialID, ContentHash: p.ContentHash})
	}
}

// ClaimDeckPreparation atomically grants one worker the queued preparation.
func (s *PostgresStore) ClaimDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	model, err := s.queries().ClaimDeckPreparation(ctx, sqlcgen.ClaimDeckPreparationParams{Owner: owner, ID: id})
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
	legacyVersionRequest := ready.RenderInputVersion == 0 && ready.PresentationVersion == 0
	if ready.RenderInputVersion == 0 {
		ready.RenderInputVersion = cardexport.RenderInputVersion
	}
	if ready.PresentationVersion == 0 {
		ready.PresentationVersion = cardexport.PresentationVersion
	}
	model, err := s.queries().CompleteDeckPreparation(ctx, sqlcgen.CompleteDeckPreparationParams{Artifact: ready.Artifact, Filename: ready.Filename, DeckName: ready.DeckName, TotalCards: ready.TotalCards, CardsWithEnglish: ready.CardsWithEnglish, CardsWithContextualSentenceTranslations: ready.CardsWithContextualSentenceTranslations, CardsWithFallbackGloss: ready.CardsWithFallbackGloss, QualityOmissions: ready.QualityOmissions, RenderInputVersion: ready.RenderInputVersion, PresentationVersion: ready.PresentationVersion, Owner: owner, ID: id})
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
	versionsMatch := existing.RenderInputVersion == ready.RenderInputVersion && existing.PresentationVersion == ready.PresentationVersion
	if legacyVersionRequest {
		versionsMatch = versionsMatch || (existing.RenderInputVersion == 0 && existing.PresentationVersion == 0)
	}
	if bytes.Equal(existing.Artifact, ready.Artifact) && existing.Filename == ready.Filename && existing.DeckName == ready.DeckName && existing.TotalCards == ready.TotalCards && existing.CardsWithEnglish == ready.CardsWithEnglish && existing.CardsWithContextualSentenceTranslations == ready.CardsWithContextualSentenceTranslations && existing.CardsWithFallbackGloss == ready.CardsWithFallbackGloss && existing.QualityOmissions == ready.QualityOmissions && versionsMatch {
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
func (s *PostgresStore) FailPreparedDeckFinalization(ctx context.Context, owner, preparationID, runID, claimToken, errorClass, errorCode string) (err error) {
	if err := validateBoundedError(errorClass, errorCode); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err = sqlcgen.New(tx).FailPreparedDeckFinalizationRun(ctx, sqlcgen.FailPreparedDeckFinalizationRunParams{OwnerID: owner, PreparationID: preparationID, ID: runID, FinalizationClaimToken: uuidArg(claimToken), ErrorClass: errorClass, ErrorCode: errorCode}); err != nil {
		return missing(err)
	}
	if _, err = sqlcgen.New(tx).FailDeckPreparationTranslation(ctx, sqlcgen.FailDeckPreparationTranslationParams{OwnerID: owner, ID: preparationID, CurrentRunID: uuidArg(runID), Error: "prepared-deck translation was incomplete"}); err != nil {
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

func (s *PostgresStore) transitionDeckPreparation(ctx context.Context, owner, id string, next domain.DeckPreparationState, message string, from ...string) (result domain.DeckPreparation, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	model, err := sqlcgen.New(tx).TransitionDeckPreparation(ctx, sqlcgen.TransitionDeckPreparationParams{NextState: string(next), Message: message, Owner: owner, ID: id, FromStates: from})
	err = missing(err)
	p := deckPreparationFromModel(model)
	if errors.Is(err, ErrNotFound) {
		rollbackErr := txcleanup.Rollback(ctx, tx)
		if _, getErr := s.GetDeckPreparation(ctx, owner, id); getErr != nil {
			return p, errors.Join(getErr, rollbackErr)
		}
		return p, errors.Join(ErrInvalidTransition, rollbackErr)
	}
	if err != nil {
		return p, err
	}
	details, marshalErr := json.Marshal(map[string]any{"preparation_id": id, "from": from, "message": message})
	if marshalErr != nil {
		return p, marshalErr
	}
	if err := sqlcgen.New(tx).InsertDeckPreparationHistory(ctx, sqlcgen.InsertDeckPreparationHistoryParams{Owner: owner, Status: string(next), Details: details}); err != nil {
		return p, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	return p, nil
}

func (s *PostgresStore) GetDeckPreparation(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	model, err := s.queries().GetDeckPreparation(ctx, sqlcgen.GetDeckPreparationParams{Owner: owner, ID: id})
	return deckPreparationFromModel(model), missing(err)
}

// ListStalePreparedDecks returns only the identities needed to enqueue a
// presentation-only rerender. The current run is authoritative for the
// version comparison; retired and already surfaced preparations are omitted.
func (s *PostgresStore) ListStalePreparedDecks(ctx context.Context, presentationVersion, limit int) ([]domain.PreparedDeckRerenderWork, error) {
	if presentationVersion < 1 || limit < 1 {
		return nil, ErrInvalidTransition
	}
	sqlLimit, err := checked.Int32FromInt(limit)
	if err != nil {
		return nil, fmt.Errorf("invalid stale-deck limit: %w", err)
	}
	rows, err := s.queries().ListStalePreparedDecks(ctx, sqlcgen.ListStalePreparedDecksParams{
		PresentationVersion:        presentationVersion,
		RenderInputVersion:         cardexport.RenderInputVersion,
		RequiresRepreparationError: domain.DeckPreparationRequiresRepreparationError,
		Limit:                      sqlLimit,
	})
	if err != nil {
		return nil, err
	}
	work := make([]domain.PreparedDeckRerenderWork, 0, len(rows))
	for _, row := range rows {
		work = append(work, domain.PreparedDeckRerenderWork{
			OwnerID: row.POwnerID, PreparationID: row.PreparationID, RunID: row.RunID,
			PresentationVersion: presentationVersion,
		})
	}
	return work, nil
}

// MarkPreparedDeckRequiresRepreparation leaves the ready artifact available
// but records that its missing frozen inputs cannot be recovered safely.
func (s *PostgresStore) MarkPreparedDeckRequiresRepreparation(ctx context.Context, owner, preparationID, runID string) error {
	return s.queries().MarkPreparedDeckRequiresRepreparation(ctx, sqlcgen.MarkPreparedDeckRequiresRepreparationParams{
		Owner: owner, Preparation: preparationID, Run: uuidArg(runID),
		Error: domain.DeckPreparationRequiresRepreparationError,
	})
}

// GetDeckPreparationForAnalysis returns the one owner-scoped preparation
// associated with an exact completed analysis. The unique preparation identity
// makes this lookup safe for result pages and prevents a mutable latest-deck
// lookup from selecting the wrong analysis.
func (s *PostgresStore) GetDeckPreparationForAnalysis(ctx context.Context, owner, sourceMaterialID, analysisRunID string) (domain.DeckPreparation, error) {
	model, err := s.queries().GetDeckPreparationForAnalysis(ctx, sqlcgen.GetDeckPreparationForAnalysisParams{Owner: owner, SourceMaterial: sourceMaterialID, AnalysisRun: uuidArg(analysisRunID)})
	return deckPreparationFromModel(model), missing(err)
}

// GetDeckPreparationForSnapshot returns the current, owner-scoped
// preparation bound to one exact immutable reading snapshot.
func (s *PostgresStore) GetDeckPreparationForSnapshot(ctx context.Context, owner, snapshotID string) (domain.DeckPreparation, error) {
	model, err := s.queries().GetDeckPreparationForSnapshot(ctx, sqlcgen.GetDeckPreparationForSnapshotParams{Owner: owner, Snapshot: uuidArg(snapshotID)})
	return deckPreparationFromModel(model), missing(err)
}

// ListDeckPreparationsForSourceMaterial returns the owner's preparation history
// for one Book's acquired source, including released and graduated studies.
func (s *PostgresStore) ListDeckPreparationsForSourceMaterial(ctx context.Context, owner, sourceMaterialID string) ([]domain.DeckPreparation, error) {
	rows, err := s.queries().ListDeckPreparationsForSourceMaterial(ctx, sqlcgen.ListDeckPreparationsForSourceMaterialParams{Owner: owner, SourceMaterial: sourceMaterialID})
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
	model, err := s.queries().DownloadDeckPreparation(ctx, sqlcgen.DownloadDeckPreparationParams{Owner: owner, ID: id})
	err = missing(err)
	p := deckPreparationFromModel(model)
	if errors.Is(err, ErrNotFound) {
		var exists bool
		exists, checkErr := s.queries().DeckPreparationExists(ctx, sqlcgen.DeckPreparationExistsParams{Owner: owner, ID: id})
		if checkErr != nil {
			return p, checkErr
		}
		if exists {
			return p, ErrInvalidTransition
		}
	}
	return p, err
}
