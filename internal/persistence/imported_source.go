package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// PutImportedSourceMaterial stores an operator-imported source material and its
// extracted-unit snapshot for a Book, and links the source to the Book. It
// reuses the EPUB source persistence, so revisions, snapshots and offsets behave
// identically; a repeat with the same bytes returns the same current revision
// and writes nothing new. The Book is not re-validated here; the caller locks
// and verifies it before writing.
func (s *PostgresStore) PutImportedSourceMaterial(ctx context.Context, v domain.SourceMaterial, units domain.ExtractedUnits, bookID string) (out domain.SourceMaterial, err error) {
	if strings.TrimSpace(bookID) == "" {
		return out, errors.New("persistence: imported source Book is required")
	}
	if err = units.ValidateOffsets(v.FullText); err != nil {
		return out, fmt.Errorf("validate imported units: %w", err)
	}
	structure := domain.SnapshotStructure{}
	if err = structure.Validate(units.SchemaVersion, units.Units); err != nil {
		return out, fmt.Errorf("validate imported source structure: %w", err)
	}
	revision, err := sourceMaterialContentIdentity(v)
	if err != nil {
		return out, err
	}
	err = withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		written, err := putSourceMaterialWithExtractedUnits(ctx, q, v, units, structure, revision)
		if err != nil {
			return err
		}
		out = written
		return q.SetSourceMaterialBook(ctx, sqlcgen.SetSourceMaterialBookParams{OwnerID: written.OwnerID, ID: written.ID, BookID: uuidArg(bookID)})
	})
	return out, err
}
