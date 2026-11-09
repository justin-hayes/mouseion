package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// ErrClassicalConversionChanged reports that the published source or analysis
// changed between the evidence comparison and the conversion write. Nothing is
// rewritten; the next startup retries against the new evidence.
var ErrClassicalConversionChanged = errors.New("persistence: classical conversion evidence changed")

// ConvertLegacyClassicalSource rewrites one pre-upgrade Book's source material
// from the whole work file to its Reading unit's own text. It keeps the source
// row and its historical revisions, adds the unit as a new content revision and
// snapshot, re-points the published analysis run at that evidence, sets the
// source identifier to the permanent Reading-unit key and records the
// prepared-definition row without creating another analysis run.
//
// "Discarding the whole-file content" means the source's current content is the
// unit: every current read resolves the new revision. Historical revisions stay
// immutable because a frozen current-reading snapshot or completed Reading can
// still name them as evidence.
func (s *PostgresStore) ConvertLegacyClassicalSource(ctx context.Context, conversion domain.ClassicalSourceConversion) (bool, error) {
	if err := validateClassicalSourceConversion(conversion); err != nil {
		return false, err
	}
	converted := false
	err := withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		converted, err = convertLegacyClassicalSourceTx(ctx, tx, conversion)
		return err
	})
	if err != nil {
		return false, err
	}
	return converted, nil
}

// convertLegacyClassicalSourceTx holds every read and write in one transaction.
// A retry that finds the source already keyed by the Reading unit changes
// nothing.
func convertLegacyClassicalSourceTx(ctx context.Context, tx pgx.Tx, conversion domain.ClassicalSourceConversion) (bool, error) {
	identifier, revisionID, snapshotID, err := lockConvertedSource(ctx, tx, conversion)
	if err != nil {
		return false, err
	}
	if identifier == conversion.ReadingUnitKey {
		return false, nil
	}
	if revisionID != conversion.ExpectedContentRevisionID || snapshotID != conversion.ExpectedSnapshotID {
		return false, ErrClassicalConversionChanged
	}
	if err := verifyPublishedConversionRun(ctx, tx, conversion); err != nil {
		return false, err
	}
	newRevisionID, err := writeConversionRevision(ctx, tx, conversion)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_materials
		SET source_identifier = $3, current_content_revision_id = $4
		WHERE owner_id = $1 AND id = $2`,
		conversion.OwnerID, conversion.SourceMaterialID, conversion.ReadingUnitKey, newRevisionID); err != nil {
		return false, fmt.Errorf("rewrite converted source identity: %w", err)
	}
	newSnapshotID, err := writeConversionSnapshot(ctx, tx, conversion, newRevisionID)
	if err != nil {
		return false, err
	}
	if err := tx.QueryRow(ctx, `UPDATE analysis_runs
		SET content_revision_id = $3, snapshot_id = $4, updated_at = now()
		WHERE owner_id = $1 AND id = $2
		RETURNING id::text`, conversion.OwnerID, conversion.AnalysisRunID, newRevisionID, newSnapshotID).Scan(new(string)); err != nil {
		return false, fmt.Errorf("re-point converted analysis run: %w", err)
	}
	if err := recordConvertedPreparedDefinition(ctx, tx, conversion); err != nil {
		return false, err
	}
	return true, nil
}

// lockConvertedSource reads the source's current identity under a row lock.
func lockConvertedSource(ctx context.Context, tx pgx.Tx, conversion domain.ClassicalSourceConversion) (identifier, revisionID, snapshotID string, err error) {
	var revision, snapshot pgtype.Text
	err = tx.QueryRow(ctx, `
		SELECT source_identifier, current_content_revision_id::text, current_snapshot_id::text
		FROM source_materials
		WHERE owner_id = $1 AND id = $2
		FOR UPDATE`, conversion.OwnerID, conversion.SourceMaterialID).Scan(&identifier, &revision, &snapshot)
	if err != nil {
		return "", "", "", fmt.Errorf("lock classical conversion source: %w", err)
	}
	return identifier, revision.String, snapshot.String, nil
}

// verifyPublishedConversionRun confirms the run still publishes this Book's
// current source before its evidence is re-pointed.
func verifyPublishedConversionRun(ctx context.Context, tx pgx.Tx, conversion domain.ClassicalSourceConversion) error {
	var runID string
	err := tx.QueryRow(ctx, `
		SELECT analysis_run_id::text
		FROM book_current_analyses
		WHERE owner_id = $1 AND book_id = $2 AND source_material_id = $3
		FOR UPDATE`, conversion.OwnerID, conversion.BookID, conversion.SourceMaterialID).Scan(&runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrClassicalConversionChanged
	}
	if err != nil {
		return fmt.Errorf("lock converted published analysis: %w", err)
	}
	if runID != conversion.AnalysisRunID {
		return ErrClassicalConversionChanged
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id = $1 AND id = $2 FOR UPDATE`,
		conversion.OwnerID, conversion.AnalysisRunID).Scan(&state); err != nil {
		return fmt.Errorf("lock converted analysis run: %w", err)
	}
	if state != "completed" {
		return ErrClassicalConversionChanged
	}
	return nil
}

// writeConversionRevision reuses or inserts the unit-text content revision and
// makes it the source's current revision.
func writeConversionRevision(ctx context.Context, tx pgx.Tx, conversion domain.ClassicalSourceConversion) (string, error) {
	content := []byte(conversion.UnitText)
	sum := sha256.Sum256(content)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	q := sqlcgen.New(tx)
	revisionID, err := q.GetSourceContentRevisionByDigest(ctx, sqlcgen.GetSourceContentRevisionByDigestParams{
		OwnerID: conversion.OwnerID, SourceMaterialID: conversion.SourceMaterialID, ContentDigest: digest,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		revisionID, err = q.InsertSourceContentRevision(ctx, sqlcgen.InsertSourceContentRevisionParams{
			OwnerID: conversion.OwnerID, SourceMaterialID: conversion.SourceMaterialID,
			DigestVersion: 1, ContentDigest: digest, Content: content, FullText: conversion.UnitText,
		})
	}
	if err != nil {
		return "", fmt.Errorf("write converted content revision: %w", err)
	}
	if err := q.SetCurrentContentRevision(ctx, sqlcgen.SetCurrentContentRevisionParams{
		OwnerID: conversion.OwnerID, ID: conversion.SourceMaterialID, CurrentContentRevisionID: uuidArg(revisionID),
	}); err != nil {
		return "", fmt.Errorf("set converted content revision: %w", err)
	}
	return revisionID, nil
}

// writeConversionSnapshot writes the single-unit snapshot that carries the
// converted revision and makes it current.
func writeConversionSnapshot(ctx context.Context, tx pgx.Tx, conversion domain.ClassicalSourceConversion, revisionID string) (string, error) {
	q := sqlcgen.New(tx)
	var existing string
	err := tx.QueryRow(ctx, `
		SELECT snapshot_id::text
		FROM source_material_unit_snapshots
		WHERE owner_id = $1 AND source_material_id = $2 AND content_revision_id = $3 AND schema_version = 1`,
		conversion.OwnerID, conversion.SourceMaterialID, revisionID).Scan(&existing)
	switch {
	case err == nil:
	case errors.Is(err, pgx.ErrNoRows):
		existing, err = q.InsertSourceMaterialUnitSnapshot(ctx, sqlcgen.InsertSourceMaterialUnitSnapshotParams{
			OwnerID: conversion.OwnerID, SourceMaterialID: conversion.SourceMaterialID,
			ContentRevisionID: revisionID, SchemaVersion: domain.ExtractedUnitsSchemaVersionV1,
			StructureConventionSet: pgtype.Text{},
		})
		if err != nil {
			return "", fmt.Errorf("write converted source snapshot: %w", err)
		}
		if err := insertConvertedUnit(ctx, tx, conversion, existing); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("find converted source snapshot: %w", err)
	}
	if err := q.SetCurrentSnapshot(ctx, sqlcgen.SetCurrentSnapshotParams{
		OwnerID: conversion.OwnerID, ID: conversion.SourceMaterialID, CurrentSnapshotID: uuidArg(existing),
	}); err != nil {
		return "", fmt.Errorf("set converted source snapshot: %w", err)
	}
	return existing, nil
}

// insertConvertedUnit writes the one unit the converted snapshot carries.
func insertConvertedUnit(ctx context.Context, tx pgx.Tx, conversion domain.ClassicalSourceConversion, snapshotID string) error {
	length := int64(len([]rune(conversion.UnitText)))
	properties := []byte("[]")
	navigationLabels := []byte("[]")
	landmarkTypes := []byte("[]")
	err := sqlcgen.New(tx).InsertSourceMaterialUnit(ctx, sqlcgen.InsertSourceMaterialUnitParams{
		OwnerID: conversion.OwnerID, SourceMaterialID: conversion.SourceMaterialID, SnapshotID: snapshotID,
		UnitID: conversion.UnitID, UnitOrder: 0, SpineIndex: 0,
		Title: conversion.UnitTitle, TitleSource: domain.UnitTitleManifestID,
		Text: conversion.UnitText, StartOffset: 0, EndOffset: length,
		PackagePath: conversion.SourceFileName, ManifestID: conversion.ReadingUnitKey,
		SourceHref: conversion.SourceFileName, ResolvedHref: conversion.SourceFileName,
		MediaType: domain.ImportedSourceMediaType, Properties: properties, Linear: true,
		NavigationLabels: navigationLabels, LandmarkTypes: landmarkTypes,
	})
	if err != nil {
		return fmt.Errorf("write converted source unit: %w", err)
	}
	return nil
}

// recordConvertedPreparedDefinition makes the Book prepared under the current
// definition revision without a new analysis run.
func recordConvertedPreparedDefinition(ctx context.Context, tx pgx.Tx, conversion domain.ClassicalSourceConversion) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO classical_prepared_definitions(
			owner_id, book_id, reading_unit_key, definition_revision,
			source_file_sha256, interpretation_version, analysis_run_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (owner_id, book_id) DO UPDATE SET
			reading_unit_key = EXCLUDED.reading_unit_key,
			definition_revision = EXCLUDED.definition_revision,
			source_file_sha256 = EXCLUDED.source_file_sha256,
			interpretation_version = EXCLUDED.interpretation_version,
			analysis_run_id = EXCLUDED.analysis_run_id,
			prepared_at = now()`,
		conversion.OwnerID, conversion.BookID, conversion.ReadingUnitKey, conversion.DefinitionRevision,
		conversion.SourceFileSHA256, conversion.InterpretationVersion, conversion.AnalysisRunID)
	if err != nil {
		return fmt.Errorf("record converted prepared definition: %w", err)
	}
	return nil
}

// validateClassicalSourceConversion refuses an incomplete conversion before the
// transaction starts.
func validateClassicalSourceConversion(conversion domain.ClassicalSourceConversion) error {
	switch {
	case conversion.OwnerID == "":
		return errors.New("persistence: classical conversion owner is required")
	case conversion.BookID == "":
		return errors.New("persistence: classical conversion Book is required")
	case conversion.SourceMaterialID == "":
		return errors.New("persistence: classical conversion source is required")
	case conversion.ReadingUnitKey == "":
		return errors.New("persistence: classical conversion Reading unit is required")
	case conversion.UnitID == "":
		return errors.New("persistence: classical conversion unit is required")
	case conversion.UnitText == "":
		return errors.New("persistence: classical conversion text is required")
	case conversion.DefinitionRevision <= 0:
		return errors.New("persistence: classical conversion revision must be positive")
	case len(conversion.SourceFileSHA256) != 64:
		return errors.New("persistence: classical conversion source SHA-256 is invalid")
	case conversion.InterpretationVersion == "":
		return errors.New("persistence: classical conversion interpretation version is required")
	case conversion.AnalysisRunID == "":
		return errors.New("persistence: classical conversion analysis run is required")
	case conversion.ExpectedContentRevisionID == "" || conversion.ExpectedSnapshotID == "":
		return errors.New("persistence: classical conversion expected evidence is required")
	}
	return nil
}
