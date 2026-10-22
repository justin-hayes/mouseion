package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// snapshotIdentity is the exact snapshot a read resolves through, together with
// the contract and convention set it recorded.
type snapshotIdentity struct {
	snapshotID        string
	contentRevisionID string
	schemaVersion     int
	conventionSet     string
}

func snapshotIdentityFromCurrent(row sqlcgen.GetCurrentExtractedUnitSnapshotIdentityRow) snapshotIdentity {
	return snapshotIdentity{snapshotID: row.SnapshotID, contentRevisionID: row.ContentRevisionID, schemaVersion: row.SchemaVersion, conventionSet: row.StructureConventionSet}
}

func snapshotIdentityFromExact(row sqlcgen.GetExtractedUnitSnapshotIdentityRow) snapshotIdentity {
	return snapshotIdentity{snapshotID: row.SnapshotID, contentRevisionID: row.ContentRevisionID, schemaVersion: row.SchemaVersion, conventionSet: row.StructureConventionSet}
}

// GetCurrentExtractedSnapshot reads the current owner-scoped snapshot and, for a
// v2 contract, its recorded structure. A v1 snapshot reads as structure not
// recorded; its text is never used to infer prose, speakers or source lines.
func (s *PostgresStore) GetCurrentExtractedSnapshot(ctx context.Context, owner, sourceID string) (domain.ExtractedSnapshot, error) {
	identity, err := s.queries().GetCurrentExtractedUnitSnapshotIdentity(ctx, sqlcgen.GetCurrentExtractedUnitSnapshotIdentityParams{OwnerID: owner, SourceMaterialID: sourceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ExtractedSnapshot{}, domain.ErrExtractedUnitsUnavailable
	}
	if err != nil {
		return domain.ExtractedSnapshot{}, err
	}
	return s.loadExtractedSnapshot(ctx, owner, sourceID, snapshotIdentityFromCurrent(identity))
}

// GetExtractedSnapshotByID reads one exact owner/source-scoped snapshot. Every
// unit, role span and heading reset resolves through this identity, so a unit ID
// that recurs in another snapshot never resolves to the wrong evidence.
func (s *PostgresStore) GetExtractedSnapshotByID(ctx context.Context, owner, sourceID, snapshotID string) (domain.ExtractedSnapshot, error) {
	identity, err := s.queries().GetExtractedUnitSnapshotIdentity(ctx, sqlcgen.GetExtractedUnitSnapshotIdentityParams{OwnerID: owner, SourceMaterialID: sourceID, SnapshotID: snapshotID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ExtractedSnapshot{}, domain.ErrExtractedUnitsUnavailable
	}
	if err != nil {
		return domain.ExtractedSnapshot{}, err
	}
	return s.loadExtractedSnapshot(ctx, owner, sourceID, snapshotIdentityFromExact(identity))
}

// loadExtractedSnapshot loads one snapshot's units and structure, validating the
// units against text reconstructed from their own rows rather than a content
// revision's FullText.
func (s *PostgresStore) loadExtractedSnapshot(ctx context.Context, owner, sourceID string, identity snapshotIdentity) (domain.ExtractedSnapshot, error) {
	out := domain.ExtractedSnapshot{SnapshotID: identity.snapshotID, ContentRevisionID: identity.contentRevisionID}
	out.Units.SchemaVersion = identity.schemaVersion
	rows, err := s.queries().ListExtractedUnitsForSnapshot(ctx, sqlcgen.ListExtractedUnitsForSnapshotParams{OwnerID: owner, SourceMaterialID: sourceID, SnapshotID: identity.snapshotID})
	if err != nil {
		return domain.ExtractedSnapshot{}, err
	}
	for _, row := range rows {
		unit, err := extractedUnitFromRow(row)
		if err != nil {
			return domain.ExtractedSnapshot{}, err
		}
		out.Units.Units = append(out.Units.Units, unit)
	}
	if err = out.Units.ValidateSnapshot(); err != nil {
		return domain.ExtractedSnapshot{}, fmt.Errorf("validate persisted extracted units: %w", err)
	}
	out.Structure, err = s.loadSnapshotStructure(ctx, owner, sourceID, identity)
	if err != nil {
		return domain.ExtractedSnapshot{}, err
	}
	if err = out.Structure.Validate(identity.schemaVersion, out.Units.Units); err != nil {
		return domain.ExtractedSnapshot{}, fmt.Errorf("validate persisted source structure: %w", err)
	}
	return out, nil
}

// loadSnapshotStructure reads the role spans and heading resets recorded by one
// exact v2 snapshot. A v1 snapshot carries no structure rows and reads as
// structure not recorded.
func (s *PostgresStore) loadSnapshotStructure(ctx context.Context, owner, sourceID string, identity snapshotIdentity) (domain.SnapshotStructure, error) {
	if identity.schemaVersion < domain.ExtractedUnitsSchemaVersionV2 {
		return domain.SnapshotStructure{}, nil
	}
	out := domain.SnapshotStructure{Recorded: true, ConventionSet: identity.conventionSet}
	spanRows, err := s.queries().ListSourceMaterialUnitRoleSpans(ctx, sqlcgen.ListSourceMaterialUnitRoleSpansParams{OwnerID: owner, SourceMaterialID: sourceID, SnapshotID: identity.snapshotID})
	if err != nil {
		return domain.SnapshotStructure{}, err
	}
	for _, row := range spanRows {
		start, err := checked.Uint64FromInt(row.StartOffset)
		if err != nil {
			return domain.SnapshotStructure{}, fmt.Errorf("validate persisted role span start: %w", err)
		}
		end, err := checked.Uint64FromInt(row.EndOffset)
		if err != nil {
			return domain.SnapshotStructure{}, fmt.Errorf("validate persisted role span end: %w", err)
		}
		out.RoleSpans = append(out.RoleSpans, domain.UnitRoleSpan{UnitID: row.UnitID, Role: domain.SourceStructureRole(row.Role), StartOffset: start, EndOffset: end})
	}
	resetRows, err := s.queries().ListSourceMaterialUnitHeadingResets(ctx, sqlcgen.ListSourceMaterialUnitHeadingResetsParams{OwnerID: owner, SourceMaterialID: sourceID, SnapshotID: identity.snapshotID})
	if err != nil {
		return domain.SnapshotStructure{}, err
	}
	for _, row := range resetRows {
		offset, err := checked.Uint64FromInt(row.Offset)
		if err != nil {
			return domain.SnapshotStructure{}, fmt.Errorf("validate persisted heading reset offset: %w", err)
		}
		out.HeadingResets = append(out.HeadingResets, domain.UnitHeadingReset{UnitID: row.UnitID, Offset: offset})
	}
	return out, nil
}

// RecordSourceStructureCheck stores one successful owner/source-scoped structure
// check. The exact key is idempotent: a retry or double confirmation reuses the
// existing durable record. A different result for the same key is an error,
// never a silent replacement or a silently reused mismatched identity.
func (s *PostgresStore) RecordSourceStructureCheck(ctx context.Context, check domain.SourceStructureCheck) error {
	if !check.Outcome.Valid() {
		return fmt.Errorf("record source structure check: unknown outcome %q", check.Outcome)
	}
	if check.TargetContractVersion < domain.ExtractedUnitsSchemaVersionV2 {
		return fmt.Errorf("record source structure check: target contract %d is not v2", check.TargetContractVersion)
	}
	if err := s.queries().InsertSourceStructureCheck(ctx, sqlcgen.InsertSourceStructureCheckParams{
		OwnerID: check.OwnerID, SourceMaterialID: check.SourceMaterialID,
		ContentRevisionID: check.ContentRevisionID, SnapshotID: check.SnapshotID,
		TargetContractVersion: check.TargetContractVersion, StructureConventionSet: check.ConventionSet,
		Outcome: string(check.Outcome), ResultIdentity: check.ResultIdentity,
	}); err != nil {
		return err
	}
	stored, found, err := s.GetSourceStructureCheck(ctx, check.Key())
	if err != nil {
		return err
	}
	if !found {
		return errors.New("record source structure check: the check was not stored")
	}
	if stored.Outcome != check.Outcome || stored.ResultIdentity != check.ResultIdentity {
		return errors.New("record source structure check: the exact key already holds a different result")
	}
	return nil
}

// GetSourceStructureCheck returns the durable check for an exact key, if one was
// recorded. A failed check is never recorded as a successful result.
func (s *PostgresStore) GetSourceStructureCheck(ctx context.Context, key domain.SourceStructureCheckKey) (domain.SourceStructureCheck, bool, error) {
	row, err := s.queries().GetSourceStructureCheck(ctx, sqlcgen.GetSourceStructureCheckParams{
		OwnerID: key.OwnerID, SourceMaterialID: key.SourceMaterialID, ContentRevisionID: key.ContentRevisionID,
		SnapshotID: key.SnapshotID, TargetContractVersion: key.TargetContractVersion,
		StructureConventionSet: key.ConventionSet,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SourceStructureCheck{}, false, nil
	}
	if err != nil {
		return domain.SourceStructureCheck{}, false, err
	}
	return domain.SourceStructureCheck{
		OwnerID: row.OwnerID, SourceMaterialID: row.SourceMaterialID, ContentRevisionID: row.ContentRevisionID,
		SnapshotID: row.SnapshotID, TargetContractVersion: row.TargetContractVersion,
		ConventionSet: row.StructureConventionSet, Outcome: domain.SourceStructureOutcome(row.Outcome),
		ResultIdentity: row.ResultIdentity, CheckedAt: row.CheckedAt,
	}, true, nil
}

// ListSourceStructureCheckCandidates returns one bounded page of current EPUB
// Book snapshots whose exact check key has no successful record and whose
// contract or convention set is older or different from the target. Imported
// classical sources and sources without a Book are excluded.
func (s *PostgresStore) ListSourceStructureCheckCandidates(ctx context.Context, targetContractVersion int, conventionSet, afterSourceID string, limit int) ([]domain.SourceStructureCheckKey, error) {
	if limit < 1 {
		return nil, errors.New("list source structure check candidates: limit must be positive")
	}
	pageSize, err := checked.Int32FromInt(limit)
	if err != nil {
		return nil, fmt.Errorf("list source structure check candidates: %w", err)
	}
	rows, err := s.queries().ListSourceStructureCheckCandidates(ctx, sqlcgen.ListSourceStructureCheckCandidatesParams{
		TargetContractVersion: targetContractVersion, StructureConventionSet: conventionSet,
		AfterSourceID: afterSourceID, PageSize: pageSize,
	})
	if err != nil {
		return nil, err
	}
	keys := make([]domain.SourceStructureCheckKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, domain.SourceStructureCheckKey{
			OwnerID: row.SourceOwnerID, SourceMaterialID: row.SourceID,
			ContentRevisionID: row.SourceCurrentContentRevisionID, SnapshotID: row.SourceCurrentSnapshotID,
			TargetContractVersion: targetContractVersion, ConventionSet: conventionSet,
		})
	}
	return keys, nil
}
