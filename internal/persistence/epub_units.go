package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// PutSourceMaterialWithExtractedUnits atomically adds an EPUB content revision
// and extracted snapshot. Existing revisions, units, and scopes are never
// rewritten.
func (s *PostgresStore) PutSourceMaterialWithExtractedUnits(ctx context.Context, v domain.SourceMaterial, units domain.ExtractedUnits) (out domain.SourceMaterial, err error) {
	if err = units.ValidateOffsets(v.FullText); err != nil {
		return out, fmt.Errorf("validate extracted units: %w", err)
	}
	digest := v.ContentHash
	digestVersion := 0
	if v.Content != nil {
		sum := sha256.Sum256(v.Content)
		digest = "sha256:" + hex.EncodeToString(sum[:])
		digestVersion = 1
	}
	if digest == "" {
		return out, fmt.Errorf("persist EPUB source: content identity is required")
	}
	if digestVersion == 0 && !strings.HasPrefix(digest, "legacy:") {
		digest = "legacy:" + digest
	}
	storedContent := v.Content
	if storedContent == nil {
		storedContent = []byte{}
	}
	err = withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		row, err := q.UpsertSourceMaterialForExtractedUnits(ctx, sqlcgen.UpsertSourceMaterialForExtractedUnitsParams{
			OwnerID: uuidArg(v.OwnerID), Language: v.Language, SourceIdentifier: v.SourceIdentifier,
			Title: v.Title, MediaType: v.MediaType, ContentHash: digest, Content: storedContent, FullText: v.FullText,
		})
		if err != nil {
			return err
		}
		out = sourceMaterialFromFields(row.ID, row.OwnerID, row.Language, row.SourceIdentifier, row.Title, row.MediaType, row.ContentHash, "", "", row.Content, row.FullText, 0, row.CreatedAt)
		currentRevisionID, err := q.GetCurrentContentRevisionForUpdate(ctx, sqlcgen.GetCurrentContentRevisionForUpdateParams{OwnerID: uuidArg(out.OwnerID), ID: uuidArg(out.ID)})
		if err != nil {
			return err
		}
		revisionID, err := q.GetSourceContentRevisionByDigest(ctx, sqlcgen.GetSourceContentRevisionByDigestParams{OwnerID: uuidArg(out.OwnerID), SourceMaterialID: uuidArg(out.ID), ContentDigest: digest})
		if errors.Is(err, pgx.ErrNoRows) {
			revisionID, err = q.InsertSourceContentRevision(ctx, sqlcgen.InsertSourceContentRevisionParams{OwnerID: uuidArg(out.OwnerID), SourceMaterialID: uuidArg(out.ID), DigestVersion: int32(digestVersion), ContentDigest: digest, Content: storedContent, FullText: v.FullText})
		}
		if err != nil {
			return err
		}
		if revisionID != currentRevisionID {
			if err = q.SetCurrentContentRevision(ctx, sqlcgen.SetCurrentContentRevisionParams{OwnerID: uuidArg(out.OwnerID), ID: uuidArg(out.ID), CurrentContentRevisionID: uuidArg(revisionID)}); err != nil {
				return err
			}
			snapshotID, err := q.InsertSourceMaterialUnitSnapshot(ctx, sqlcgen.InsertSourceMaterialUnitSnapshotParams{OwnerID: uuidArg(out.OwnerID), SourceMaterialID: uuidArg(out.ID), ContentRevisionID: uuidArg(revisionID), SchemaVersion: int32(units.SchemaVersion)})
			if err != nil {
				return err
			}
			if err = q.SetCurrentSnapshot(ctx, sqlcgen.SetCurrentSnapshotParams{OwnerID: uuidArg(out.OwnerID), ID: uuidArg(out.ID), CurrentSnapshotID: uuidArg(snapshotID)}); err != nil {
				return err
			}
			for _, unit := range units.Units {
				properties, marshalErr := json.Marshal(unit.Properties)
				if marshalErr != nil {
					return marshalErr
				}
				navigationLabels, marshalErr := json.Marshal(unit.NavigationLabels)
				if marshalErr != nil {
					return marshalErr
				}
				landmarkTypes, marshalErr := json.Marshal(unit.LandmarkTypes)
				if marshalErr != nil {
					return marshalErr
				}
				if err = q.InsertSourceMaterialUnit(ctx, sqlcgen.InsertSourceMaterialUnitParams{
					OwnerID: uuidArg(out.OwnerID), SourceMaterialID: uuidArg(out.ID), SnapshotID: uuidArg(snapshotID), UnitID: unit.ID,
					UnitOrder: int64(unit.Order), SpineIndex: int64(unit.SpineIndex), Title: unit.Title, TitleSource: unit.TitleSource,
					Text: unit.Text, StartOffset: int64(unit.StartOffset), EndOffset: int64(unit.EndOffset), PackagePath: unit.PackagePath,
					ManifestID: unit.ManifestID, SourceHref: unit.SourceHref, ResolvedHref: unit.ResolvedHref, MediaType: unit.MediaType,
					Properties: properties, Linear: unit.Linear, NavigationLabels: navigationLabels, LandmarkTypes: landmarkTypes,
				}); err != nil {
					return err
				}
			}
		}
		current, err := q.GetCurrentSourceMaterialContent(ctx, sqlcgen.GetCurrentSourceMaterialContentParams{OwnerID: uuidArg(out.OwnerID), ID: uuidArg(out.ID)})
		if err != nil {
			return err
		}
		out.Content = current.Content
		out.FullText = current.FullText
		out.ContentHash = current.ContentHash
		out.ContentDigest = current.ContentDigest
		out.ContentRevisionID = current.ContentRevisionID
		out.ContentDigestVersion = int(current.DigestVersion)
		return nil
	})
	return out, err
}

// GetExtractedUnits returns the current owner-scoped snapshot in unit order.
// Legacy source materials intentionally return ErrExtractedUnitsUnavailable.
func (s *PostgresStore) GetExtractedUnits(ctx context.Context, owner, sourceID string) (domain.ExtractedUnits, error) {
	_, out, err := s.GetExtractedUnitSnapshot(ctx, owner, sourceID)
	return out, err
}

// GetExtractedUnitSnapshot returns the immutable identity and contents of the
// current owner-scoped extracted-unit snapshot.
func (s *PostgresStore) GetExtractedUnitSnapshot(ctx context.Context, owner, sourceID string) (string, domain.ExtractedUnits, error) {
	var out domain.ExtractedUnits
	snapshot, err := s.queries().GetCurrentExtractedUnitSnapshot(ctx, sqlcgen.GetCurrentExtractedUnitSnapshotParams{OwnerID: uuidArg(owner), SourceMaterialID: uuidArg(sourceID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", out, domain.ErrExtractedUnitsUnavailable
	}
	if err != nil {
		return "", out, err
	}
	out.SchemaVersion = int(snapshot.SchemaVersion)
	rows, err := s.queries().ListCurrentExtractedUnits(ctx, sqlcgen.ListCurrentExtractedUnitsParams{OwnerID: uuidArg(owner), SourceMaterialID: uuidArg(sourceID)})
	if err != nil {
		return "", out, err
	}
	for _, row := range rows {
		var unit domain.ExtractedUnit
		unit.ID, unit.Order, unit.SpineIndex = row.UnitID, uint64(row.UnitOrder), uint64(row.SpineIndex)
		unit.Title, unit.TitleSource, unit.Text = row.Title, row.TitleSource, row.Text
		unit.StartOffset, unit.EndOffset = uint64(row.StartOffset), uint64(row.EndOffset)
		unit.PackagePath, unit.ManifestID, unit.SourceHref = row.PackagePath, row.ManifestID, row.SourceHref
		unit.ResolvedHref, unit.MediaType, unit.Linear = row.ResolvedHref, row.MediaType, row.Linear
		if err = json.Unmarshal(row.Properties, &unit.Properties); err != nil {
			return "", out, err
		}
		if err = json.Unmarshal(row.NavigationLabels, &unit.NavigationLabels); err != nil {
			return "", out, err
		}
		if err = json.Unmarshal(row.LandmarkTypes, &unit.LandmarkTypes); err != nil {
			return "", out, err
		}
		out.Units = append(out.Units, unit)
	}
	if err = out.ValidateOffsets(snapshot.FullText); err != nil {
		return "", out, fmt.Errorf("validate persisted extracted units: %w", err)
	}
	return snapshot.USnapshotID, out, nil
}
