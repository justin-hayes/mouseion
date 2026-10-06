package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// VocabularyBrowseSelectionExtent records the number of retired Browse
// selections owned by one learner in one language.
type VocabularyBrowseSelectionExtent struct {
	OwnerID  string `json:"owner_id"`
	Language string `json:"language"`
	Rows     int64  `json:"rows"`
}

// VocabularyBrowseSelectionCutover removes the now-inaccessible, owner-scoped
// Browse selections. It deliberately does not touch Known vocabulary,
// Reading snapshots, Custom decks, or prepared artifacts.
type VocabularyBrowseSelectionCutover struct {
	Extents []VocabularyBrowseSelectionExtent `json:"extents"`
	Deleted int64                             `json:"deleted"`
}

// CutoverVocabularyBrowseSelections deletes obsolete saved selections in one
// transaction. The table lock serializes against any old writer during
// deployment; rerunning after commit returns an empty extent and deletes zero.
func (s *PostgresStore) CutoverVocabularyBrowseSelections(ctx context.Context) (report VocabularyBrowseSelectionCutover, err error) {
	report.Extents = make([]VocabularyBrowseSelectionExtent, 0)
	if s == nil || s.pool == nil {
		return report, errors.New("vocabulary selection cutover: store is unavailable")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return report, fmt.Errorf("begin vocabulary selection cutover: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err = tx.Exec(ctx, `LOCK TABLE vocabulary_browse_selections IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return report, fmt.Errorf("lock Browse selections for cutover: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT owner_id::text, language, count(*)::bigint
		FROM vocabulary_browse_selections GROUP BY owner_id, language ORDER BY owner_id, language`)
	if err != nil {
		return report, fmt.Errorf("inventory Browse selections: %w", err)
	}
	for rows.Next() {
		var extent VocabularyBrowseSelectionExtent
		if err = rows.Scan(&extent.OwnerID, &extent.Language, &extent.Rows); err != nil {
			rows.Close()
			return report, fmt.Errorf("read Browse selection inventory: %w", err)
		}
		report.Extents = append(report.Extents, extent)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return report, fmt.Errorf("read Browse selection inventory: %w", err)
	}
	rows.Close()
	tag, err := tx.Exec(ctx, `DELETE FROM vocabulary_browse_selections`)
	if err != nil {
		return report, fmt.Errorf("delete obsolete Browse selections: %w", err)
	}
	report.Deleted = tag.RowsAffected()
	var remaining int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM vocabulary_browse_selections`).Scan(&remaining); err != nil {
		return report, fmt.Errorf("verify Browse selection cutover: %w", err)
	}
	if remaining != 0 {
		return report, fmt.Errorf("verify Browse selection cutover: %d rows remain", remaining)
	}
	var inventoried int64
	for _, extent := range report.Extents {
		inventoried += extent.Rows
	}
	if inventoried != report.Deleted {
		return report, fmt.Errorf("verify Browse selection cutover: inventoried %d rows but deleted %d", inventoried, report.Deleted)
	}
	if err = tx.Commit(ctx); err != nil {
		return report, fmt.Errorf("commit Browse selection cutover: %w", err)
	}
	return report, nil
}
