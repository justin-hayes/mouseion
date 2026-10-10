package prepareddeck

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/riverqueue/river"
)

// CurrentDeck is the Book deck state the focused deck page reads. Admission is
// the Submit decision against the Current reading snapshot, so a page can tell
// "no deck is needed" and "not the current reading" apart from "a deck exists".
type CurrentDeck struct {
	Current     domain.CurrentReading
	Preparation *domain.DeckPreparation
	Admission   domain.DeckAdmission
}

// PreparationAdmissions says which generation actions a specific preparation
// admits right now. The status page offers only the actions that are admitted.
type PreparationAdmissions struct {
	Retry     domain.DeckAdmission
	Reprepare domain.DeckAdmission
}

// PrepareCurrentReadingDeck creates or advances the Book's deck for the exact
// Current reading snapshot the request named. Every Book deck request enters
// here: the Book, the Current reading, and any live preparation are locked, and
// the domain admission decision is applied before anything is written.
func (s *Service) PrepareCurrentReadingDeck(ctx context.Context, owner, bookID, expectedSnapshotID string) (result Handle, err error) {
	if s == nil || s.pool == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" {
		return Handle{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if err = lockBookForUpdate(ctx, tx, owner, bookID); err != nil {
		return Handle{}, err
	}
	current, analysis, err := s.store.LockCurrentReadingDeckFacts(ctx, tx, owner, bookID)
	if err != nil {
		return Handle{}, err
	}
	live, hasLive, err := lockLivePreparationForSnapshot(ctx, tx, owner, current.SnapshotID)
	if err != nil {
		return Handle{}, err
	}
	var livePreparation *domain.DeckPreparation
	if hasLive {
		livePreparation = &live
	}
	admission := domain.DecideDeckAdmission(domain.DeckAdmissionFacts{Action: domain.DeckActionSubmit, BookID: bookID, Current: current, Analysis: analysis, ExpectedSnapshotID: expectedSnapshotID, Preparation: livePreparation})
	if err = admission.Err(); err != nil {
		return Handle{}, err
	}
	if hasLive {
		if live.SourceMaterialID != analysis.SourceMaterialID || live.AnalysisRunID != analysis.AnalysisRunID {
			return Handle{}, persistence.ErrInvalidTransition
		}
		result, err = s.advanceTx(ctx, tx, owner, live.ID, false)
	} else {
		result, err = s.createCurrentReadingDeckTx(ctx, tx, owner, bookID, current, analysis)
	}
	if err != nil {
		return Handle{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return result, nil
}

// Reprepare rolls a Ready preparation forward to a new generation for the exact
// Current reading snapshot the request named.
func (s *Service) Reprepare(ctx context.Context, owner, id, expectedSnapshotID string) (result Handle, err error) {
	if s == nil || s.pool == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(id) == "" {
		return Handle{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	p, err := s.admitPreparationTx(ctx, tx, owner, id, expectedSnapshotID, domain.DeckActionReprepare)
	if err != nil {
		return Handle{}, err
	}
	result, err = s.advanceTx(ctx, tx, owner, p.ID, true)
	if err != nil {
		return Handle{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return result, nil
}

// Rerender queues a presentation-only rebuild of a Ready preparation for the
// exact Current reading snapshot the request named. The run identity is captured
// before enqueueing so a newer preparation cannot be rendered into this artifact.
func (s *Service) Rerender(ctx context.Context, owner, id, expectedSnapshotID string) (result Handle, err error) {
	if s == nil || s.pool == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(id) == "" {
		return Handle{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	p, err := s.admitPreparationTx(ctx, tx, owner, id, expectedSnapshotID, domain.DeckActionRerender)
	if err != nil {
		return Handle{}, err
	}
	if p.PresentationVersion >= cardexport.PresentationVersion {
		if err = tx.Commit(ctx); err != nil {
			return Handle{}, err
		}
		return Handle{Preparation: p}, nil
	}
	args := RerenderJobArgs{OwnerID: owner, PreparationID: p.ID, RunID: p.CurrentRunID, PresentationVersion: cardexport.PresentationVersion}
	inserted, err := s.client.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: Queue, MaxAttempts: durableJobMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}})
	if err != nil {
		return Handle{}, err
	}
	var jobID int64
	if inserted != nil && inserted.Job != nil && isLivePreparationJobState(inserted.Job.State) {
		jobID = inserted.Job.ID
	} else if confirmed, confirmErr := liveRerenderJobID(ctx, tx, owner, p.ID, p.CurrentRunID, cardexport.PresentationVersion); confirmErr == nil {
		jobID = confirmed
	} else if !errors.Is(confirmErr, pgx.ErrNoRows) {
		return Handle{}, confirmErr
	} else {
		return Handle{}, errors.New("River did not return a live rerender job")
	}
	if err = tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return Handle{Preparation: p, JobID: jobID}, nil
}

// CurrentReadingDeck reads the Book's Current reading deck state without
// changing it, apart from the same reconciliation a status read performs.
func (s *Service) CurrentReadingDeck(ctx context.Context, owner, bookID string) (CurrentDeck, error) {
	if s == nil || s.store == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" {
		return CurrentDeck{}, ErrInvalidInput
	}
	current, analysis, err := s.store.CurrentReadingDeckFacts(ctx, owner, bookID)
	if err != nil {
		return CurrentDeck{}, err
	}
	var live *domain.DeckPreparation
	if current.SnapshotID != "" {
		found, findErr := s.store.GetDeckPreparationForSnapshot(ctx, owner, current.SnapshotID)
		switch {
		case findErr == nil:
			reconciled, reconcileErr := s.Reconcile(ctx, owner, found.ID)
			if reconcileErr != nil {
				return CurrentDeck{}, reconcileErr
			}
			live = &reconciled
		case !errors.Is(findErr, persistence.ErrNotFound):
			return CurrentDeck{}, findErr
		}
	}
	admission := domain.DecideDeckAdmission(domain.DeckAdmissionFacts{Action: domain.DeckActionSubmit, BookID: bookID, Current: current, Analysis: analysis, ExpectedSnapshotID: current.SnapshotID, Preparation: live})
	return CurrentDeck{Current: current, Preparation: live, Admission: admission}, nil
}

// PreparationAdmissions reports which generation actions the preparation
// admits, judged against the Book's current reading. It reads only.
func (s *Service) PreparationAdmissions(ctx context.Context, owner, id string) (PreparationAdmissions, error) {
	if s == nil || s.store == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(id) == "" {
		return PreparationAdmissions{}, ErrInvalidInput
	}
	p, err := s.store.GetDeckPreparation(ctx, owner, id)
	if err != nil {
		return PreparationAdmissions{}, err
	}
	current, analysis, err := s.store.CurrentReadingDeckFacts(ctx, owner, p.BookID)
	if err != nil {
		return PreparationAdmissions{}, err
	}
	facts := domain.DeckAdmissionFacts{BookID: p.BookID, Current: current, Analysis: analysis, ExpectedSnapshotID: p.SnapshotID, Preparation: &p}
	retry := facts
	retry.Action = domain.DeckActionSubmit
	reprepare := facts
	reprepare.Action = domain.DeckActionReprepare
	return PreparationAdmissions{Retry: domain.DecideDeckAdmission(retry), Reprepare: domain.DecideDeckAdmission(reprepare)}, nil
}

// admitPreparationTx locks the preparation's Book, Current reading, and
// preparation, then returns the preparation if the action is admitted.
func (s *Service) admitPreparationTx(ctx context.Context, tx pgx.Tx, owner, id, expectedSnapshotID string, action domain.DeckAction) (domain.DeckPreparation, error) {
	p, err := lockPreparationForUpdate(ctx, tx, owner, id)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	current, analysis, err := s.store.LockCurrentReadingDeckFacts(ctx, tx, owner, p.BookID)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	admission := domain.DecideDeckAdmission(domain.DeckAdmissionFacts{Action: action, BookID: p.BookID, Current: current, Analysis: analysis, ExpectedSnapshotID: expectedSnapshotID, Preparation: &p})
	if err = admission.Err(); err != nil {
		return domain.DeckPreparation{}, err
	}
	return p, nil
}

// createCurrentReadingDeckTx starts the Book's first preparation for a snapshot.
// Earlier preparations of the Book are retired in the same transaction.
func (s *Service) createCurrentReadingDeckTx(ctx context.Context, tx pgx.Tx, owner, bookID string, current domain.CurrentReading, analysis domain.CurrentAnalysis) (Handle, error) {
	completed, err := loadCompletedAnalysis(ctx, tx, owner, analysis.AnalysisRunID)
	if err != nil {
		return Handle{}, err
	}
	if completed.Source.ID != analysis.SourceMaterialID || completed.RunID != analysis.AnalysisRunID {
		return Handle{}, fmt.Errorf("%w: Current reading snapshot does not match the completed analysis", ErrAnalysisUnavailable)
	}
	source := completed.Source
	if err = sqlcRetireDeckPreparationsForBook(ctx, tx, owner, bookID); err != nil {
		return Handle{}, err
	}
	p, _, err := persistence.CreateDeckPreparationTx(ctx, tx, domain.DeckPreparation{OwnerID: owner, SourceMaterialID: source.ID, AnalysisRunID: completed.RunID, SnapshotID: current.SnapshotID, Filename: cardexport.DownloadFilename(source.Title), DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash})
	if err != nil {
		return Handle{}, err
	}
	p, jobID, err := s.ensurePreparationJob(ctx, tx, p)
	if err != nil {
		return Handle{}, fmt.Errorf("enqueue prepared deck: %w", err)
	}
	return Handle{Preparation: p, JobID: jobID}, nil
}

// advanceTx moves a live preparation forward inside the caller's transaction.
// The caller has already admitted the request under the Book and Current reading
// locks. forceReprepare rolls a Ready preparation to a new generation.
func (s *Service) advanceTx(ctx context.Context, tx pgx.Tx, owner, id string, forceReprepare bool) (Handle, error) {
	p, err := lockPreparationForUpdate(ctx, tx, owner, id)
	if err != nil {
		return Handle{}, err
	}
	if p.RetiredAt != nil {
		return Handle{}, persistence.ErrInvalidTransition
	}
	if p.State == domain.DeckPreparationReady && (p.Error == domain.DeckPreparationRequiresRepreparationError || forceReprepare) {
		// Either the immutable specification cannot be rendered or the learner
		// explicitly requested current meaning evidence. Retire the current row
		// and roll forward to a new specification bound to the same exact
		// analysis and Current reading snapshot.
		if p.BookID != "" {
			if err = sqlcRetireDeckPreparationsForBook(ctx, tx, owner, p.BookID); err != nil {
				return Handle{}, err
			}
		} else if _, err = tx.Exec(ctx, `UPDATE deck_preparations SET retired_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$2 AND retired_at IS NULL`, owner, p.ID); err != nil {
			return Handle{}, err
		}
		var created bool
		p, created, err = persistence.CreateDeckPreparationTx(ctx, tx, p)
		if err != nil {
			return Handle{}, err
		}
		if !created {
			return Handle{}, errors.New("re-preparation did not create a new specification")
		}
		id = p.ID
	} else if p.State == domain.DeckPreparationFailed || p.State == domain.DeckPreparationCancelled {
		previousState := p.State
		p, err = scanPreparation(tx.QueryRow(ctx, requeueFailedPreparationSQL, owner, id))
		if err != nil {
			return Handle{}, err
		}
		if err = recordPreparationHistoryTx(ctx, tx, owner, id, string(domain.DeckPreparationQueued), map[string]any{"from": previousState}); err != nil {
			return Handle{}, err
		}
	} else if p.State != domain.DeckPreparationQueued && p.State != domain.DeckPreparationPreparing {
		return Handle{}, persistence.ErrInvalidTransition
	}
	p, jobID, err := s.ensurePreparationJob(ctx, tx, p)
	if err != nil {
		p, err = markPreparationFailedTx(ctx, tx, owner, id, fmt.Sprintf("could not enqueue preparation: %v", err))
		if err != nil {
			return Handle{}, err
		}
	}
	return Handle{Preparation: p, JobID: jobID}, nil
}

// lockBookForUpdate fences the Book before the Current reading and preparation
// rows. Every Book deck writer takes this lock first.
func lockBookForUpdate(ctx context.Context, tx pgx.Tx, owner, bookID string) error {
	_, err := sqlcgen.New(tx).GetBookForUpdate(ctx, sqlcgen.GetBookForUpdateParams{Owner: owner, ID: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return persistence.ErrNotFound
	}
	return err
}

// The statements are package constants; the column list is a constant too, so
// no request value is ever spliced into SQL text.
const (
	requeueFailedPreparationSQL   = "UPDATE deck_preparations SET state='queued',error='',current_run_id=NULL,started_at=NULL,completed_at=NULL,updated_at=now() WHERE owner_id=$1 AND id=$2 RETURNING " + preparationColumns
	livePreparationForSnapshotSQL = "SELECT " + preparationColumns + " FROM deck_preparations WHERE owner_id=$1 AND goal_snapshot_id=$2::uuid AND retired_at IS NULL FOR UPDATE"
)

// lockLivePreparationForSnapshot returns the non-retired preparation bound to
// the snapshot, locked for update. found is false when none exists.
func lockLivePreparationForSnapshot(ctx context.Context, tx pgx.Tx, owner, snapshotID string) (p domain.DeckPreparation, found bool, err error) {
	if snapshotID == "" {
		return domain.DeckPreparation{}, false, nil
	}
	p, err = scanPreparation(tx.QueryRow(ctx, livePreparationForSnapshotSQL, owner, snapshotID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeckPreparation{}, false, nil
	}
	if err != nil {
		return domain.DeckPreparation{}, false, err
	}
	return p, true, nil
}
