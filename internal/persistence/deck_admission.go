package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// The admission facts for one Book are read from the same rows the Current
// reading and Book lock protect. Locked reads take the Current reading and its
// snapshot in the order every Current reading writer uses.
const (
	currentReadingForBookSQL = `SELECT g.owner_id::text, g.language, g.book_id::text,
       COALESCE(s.id::text,'')::text, COALESCE(s.source_material_id::text,'')::text,
       COALESCE(s.analysis_run_id::text,'')::text, COALESCE(s.content_revision_id::text,'')::text,
       COALESCE(s.content_snapshot_id::text,'')::text, COALESCE(s.corpus_id::text,'')::text,
       g.created_at, g.updated_at
FROM primary_goals g
JOIN primary_goal_snapshots s ON s.owner_id=g.owner_id AND s.id=g.snapshot_id AND s.book_id=g.book_id AND s.language=g.language
WHERE g.owner_id=$1 AND g.book_id=$2::uuid AND s.released_at IS NULL`
	// currentReadingForBookLockedSQL is currentReadingForBookSQL under row locks.
	currentReadingForBookLockedSQL = `SELECT g.owner_id::text, g.language, g.book_id::text,
       COALESCE(s.id::text,'')::text, COALESCE(s.source_material_id::text,'')::text,
       COALESCE(s.analysis_run_id::text,'')::text, COALESCE(s.content_revision_id::text,'')::text,
       COALESCE(s.content_snapshot_id::text,'')::text, COALESCE(s.corpus_id::text,'')::text,
       g.created_at, g.updated_at
FROM primary_goals g
JOIN primary_goal_snapshots s ON s.owner_id=g.owner_id AND s.id=g.snapshot_id AND s.book_id=g.book_id AND s.language=g.language
WHERE g.owner_id=$1 AND g.book_id=$2::uuid AND s.released_at IS NULL FOR UPDATE OF g, s`
)

const currentAnalysisForBookSQL = `SELECT COALESCE(source_material_id::text,'')::text, COALESCE(analysis_run_id::text,'')::text,
       COALESCE(content_revision_id::text,'')::text, COALESCE(snapshot_id::text,'')::text, COALESCE(corpus_id::text,'')::text
FROM current_analysis_identity
WHERE owner_id=$1 AND book_id=$2::uuid`

// CurrentReadingDeckFacts returns the owner's current reading for one Book and
// the Book's current analysis identity. An absent reading or analysis is the
// zero value, which admits nothing.
func (s *PostgresStore) CurrentReadingDeckFacts(ctx context.Context, owner, bookID string) (domain.CurrentReading, domain.CurrentAnalysis, error) {
	return loadCurrentReadingDeckFacts(ctx, s.Pool(), owner, bookID, false)
}

// LockCurrentReadingDeckFacts is CurrentReadingDeckFacts read inside tx under
// row locks on the Current reading and its snapshot. A deck admission decision
// must read these facts through this method so it sees the state it commits.
func (s *PostgresStore) LockCurrentReadingDeckFacts(ctx context.Context, tx pgx.Tx, owner, bookID string) (domain.CurrentReading, domain.CurrentAnalysis, error) {
	return loadCurrentReadingDeckFacts(ctx, tx, owner, bookID, true)
}

func loadCurrentReadingDeckFacts(ctx context.Context, db sqlcgen.DBTX, owner, bookID string, lock bool) (domain.CurrentReading, domain.CurrentAnalysis, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" {
		return domain.CurrentReading{}, domain.CurrentAnalysis{}, nil
	}
	query := currentReadingForBookSQL
	if lock {
		query = currentReadingForBookLockedSQL
	}
	var current domain.CurrentReading
	err := db.QueryRow(ctx, query, owner, bookID).Scan(&current.OwnerID, &current.Language, &current.BookID,
		&current.SnapshotID, &current.SourceMaterialID, &current.AnalysisRunID, &current.ContentRevisionID,
		&current.ContentSnapshotID, &current.CorpusID, &current.CreatedAt, &current.UpdatedAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.CurrentReading{}, domain.CurrentAnalysis{}, nil
	case err != nil:
		return domain.CurrentReading{}, domain.CurrentAnalysis{}, err
	}
	current.SnapshotSize, err = snapshotIdentityCount(ctx, db, owner, current.SnapshotID)
	if err != nil {
		return domain.CurrentReading{}, domain.CurrentAnalysis{}, err
	}
	var analysis domain.CurrentAnalysis
	err = db.QueryRow(ctx, currentAnalysisForBookSQL, owner, bookID).Scan(&analysis.SourceMaterialID, &analysis.AnalysisRunID, &analysis.ContentRevisionID, &analysis.ContentSnapshotID, &analysis.CorpusID)
	if errors.Is(err, pgx.ErrNoRows) {
		return current, domain.CurrentAnalysis{}, nil
	}
	if err != nil {
		return domain.CurrentReading{}, domain.CurrentAnalysis{}, err
	}
	return current, analysis, nil
}

func snapshotIdentityCount(ctx context.Context, db sqlcgen.DBTX, owner, snapshotID string) (int, error) {
	if snapshotID == "" {
		return 0, nil
	}
	rows, err := sqlcgen.New(db).ListCurrentReadingSnapshotVocabulary(ctx, sqlcgen.ListCurrentReadingSnapshotVocabularyParams{Owner: owner, Snapshot: snapshotID})
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}
