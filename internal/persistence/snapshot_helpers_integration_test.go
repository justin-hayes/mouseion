//go:build integration

package persistence

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// loadSnapshotSourceUnits reads the exact snapshot's units by identity so a
// unit ID recurring in another snapshot never supplies the wrong speaker text.
func loadSnapshotSourceUnits(ctx context.Context, db sqlcgen.DBTX, owner, sourceID, snapshotID string) (map[string]domain.ExtractedUnit, error) {
	rows, err := sqlcgen.New(db).ListExtractedUnitsForSnapshot(ctx, sqlcgen.ListExtractedUnitsForSnapshotParams{OwnerID: owner, SourceMaterialID: sourceID, SnapshotID: snapshotID})
	if err != nil {
		return nil, err
	}
	units := make(map[string]domain.ExtractedUnit, len(rows))
	for _, row := range rows {
		unit, err := extractedUnitFromRow(row)
		if err != nil {
			return nil, err
		}
		units[unit.ID] = unit
	}
	return units, nil
}

// analysisRunSnapshotID resolves the exact snapshot an analysis run was bound
// to. The run identifier is analysis_runs' primary key, and the snapshot column
// is NOT NULL.
func analysisRunSnapshotID(ctx context.Context, tx pgx.Tx, owner, runID string) (string, error) {
	var snapshotID string
	if err := tx.QueryRow(ctx, `SELECT snapshot_id::text FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner, runID).Scan(&snapshotID); err != nil {
		return "", fmt.Errorf("load analysis run snapshot: %w", err)
	}
	return snapshotID, nil
}
