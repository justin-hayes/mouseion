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
	"github.com/justin-hayes/mouseion/internal/checked"
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
		return out, errors.New("persist EPUB source: content identity is required")
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
			OwnerID: v.OwnerID, Language: v.Language, SourceIdentifier: v.SourceIdentifier,
			Title: v.Title, MediaType: v.MediaType, ContentHash: digest, Content: storedContent, FullText: v.FullText,
		})
		if err != nil {
			return err
		}
		out = sourceMaterialFromFields(row.ID, row.OwnerID, row.Language, row.SourceIdentifier, row.Title, row.MediaType, row.ContentHash, "", "", row.Content, row.FullText, 0, row.CreatedAt)
		revision := contentRevisionInput{fullText: v.FullText, digest: digest, digestVersion: digestVersion, content: storedContent}
		revisionID, changed, err := putSourceMaterialRevision(ctx, q, out, revision)
		if err != nil {
			return err
		}
		if changed {
			if err := writeExtractedUnits(ctx, q, out, revisionID, units); err != nil {
				return err
			}
		}
		current, err := q.GetCurrentSourceMaterialContent(ctx, sqlcgen.GetCurrentSourceMaterialContentParams{OwnerID: out.OwnerID, ID: out.ID})
		if err != nil {
			return err
		}
		out.Content = current.Content
		out.FullText = current.FullText
		out.ContentHash = current.ContentHash
		out.ContentDigest = current.ContentDigest
		out.ContentRevisionID = current.ContentRevisionID
		out.ContentDigestVersion = current.DigestVersion
		return nil
	})
	return out, err
}

// contentRevisionInput carries the incoming revision's identity and bytes from
// the content-identity step to the revision write. fullText is kept apart from
// the stored source because the upsert does not rewrite an existing source's
// full text.
type contentRevisionInput struct {
	fullText      string
	digest        string
	digestVersion int
	content       []byte
}

// putSourceMaterialRevision resolves the content revision for revision while
// holding the source-material row lock, inserting the revision when the digest
// is new and making it current when it differs from the stored current
// revision. It reports whether the current revision changed, which is the
// signal to record the matching extracted-unit snapshot.
func putSourceMaterialRevision(ctx context.Context, q *sqlcgen.Queries, source domain.SourceMaterial, revision contentRevisionInput) (revisionID string, changed bool, err error) {
	currentRevisionID, err := q.GetCurrentContentRevisionForUpdate(ctx, sqlcgen.GetCurrentContentRevisionForUpdateParams{OwnerID: source.OwnerID, ID: source.ID})
	if err != nil {
		return "", false, err
	}
	revisionID, err = q.GetSourceContentRevisionByDigest(ctx, sqlcgen.GetSourceContentRevisionByDigestParams{OwnerID: source.OwnerID, SourceMaterialID: source.ID, ContentDigest: revision.digest})
	if errors.Is(err, pgx.ErrNoRows) {
		revisionID, err = q.InsertSourceContentRevision(ctx, sqlcgen.InsertSourceContentRevisionParams{OwnerID: source.OwnerID, SourceMaterialID: source.ID, DigestVersion: revision.digestVersion, ContentDigest: revision.digest, Content: revision.content, FullText: revision.fullText})
	}
	if err != nil {
		return "", false, err
	}
	if revisionID == currentRevisionID {
		return revisionID, false, nil
	}
	if err := q.SetCurrentContentRevision(ctx, sqlcgen.SetCurrentContentRevisionParams{OwnerID: source.OwnerID, ID: source.ID, CurrentContentRevisionID: uuidArg(revisionID)}); err != nil {
		return "", false, err
	}
	return revisionID, true, nil
}

// writeExtractedUnits records the snapshot that carries a revision's units,
// makes it current, and inserts each unit. Snapshots and units are immutable;
// a later revision adds a new snapshot instead of rewriting this one.
func writeExtractedUnits(ctx context.Context, q *sqlcgen.Queries, source domain.SourceMaterial, revisionID string, units domain.ExtractedUnits) error {
	snapshotID, err := q.InsertSourceMaterialUnitSnapshot(ctx, sqlcgen.InsertSourceMaterialUnitSnapshotParams{OwnerID: source.OwnerID, SourceMaterialID: source.ID, ContentRevisionID: revisionID, SchemaVersion: units.SchemaVersion})
	if err != nil {
		return err
	}
	if err := q.SetCurrentSnapshot(ctx, sqlcgen.SetCurrentSnapshotParams{OwnerID: source.OwnerID, ID: source.ID, CurrentSnapshotID: uuidArg(snapshotID)}); err != nil {
		return err
	}
	for _, unit := range units.Units {
		if err := insertExtractedUnit(ctx, q, source, snapshotID, unit); err != nil {
			return err
		}
	}
	return nil
}

// insertExtractedUnit converts one extracted unit's unsigned offsets and
// marshals its optional collections before inserting the row.
func insertExtractedUnit(ctx context.Context, q *sqlcgen.Queries, source domain.SourceMaterial, snapshotID string, unit domain.ExtractedUnit) error {
	unitOrder, err := checked.Int64FromUint64(unit.Order)
	if err != nil {
		return err
	}
	spineIndex, err := checked.Int64FromUint64(unit.SpineIndex)
	if err != nil {
		return err
	}
	startOffset, err := checked.Int64FromUint64(unit.StartOffset)
	if err != nil {
		return err
	}
	endOffset, err := checked.Int64FromUint64(unit.EndOffset)
	if err != nil {
		return err
	}
	properties, err := json.Marshal(unit.Properties)
	if err != nil {
		return err
	}
	navigationLabels, err := json.Marshal(unit.NavigationLabels)
	if err != nil {
		return err
	}
	landmarkTypes, err := json.Marshal(unit.LandmarkTypes)
	if err != nil {
		return err
	}
	return q.InsertSourceMaterialUnit(ctx, sqlcgen.InsertSourceMaterialUnitParams{
		OwnerID: source.OwnerID, SourceMaterialID: source.ID, SnapshotID: snapshotID, UnitID: unit.ID,
		UnitOrder: unitOrder, SpineIndex: spineIndex, Title: unit.Title, TitleSource: unit.TitleSource,
		Text: unit.Text, StartOffset: startOffset, EndOffset: endOffset, PackagePath: unit.PackagePath,
		ManifestID: unit.ManifestID, SourceHref: unit.SourceHref, ResolvedHref: unit.ResolvedHref, MediaType: unit.MediaType,
		Properties: properties, Linear: unit.Linear, NavigationLabels: navigationLabels, LandmarkTypes: landmarkTypes,
	})
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
	snapshot, err := s.queries().GetCurrentExtractedUnitSnapshot(ctx, sqlcgen.GetCurrentExtractedUnitSnapshotParams{OwnerID: owner, SourceMaterialID: sourceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", out, domain.ErrExtractedUnitsUnavailable
	}
	if err != nil {
		return "", out, err
	}
	out.SchemaVersion = snapshot.SchemaVersion
	rows, err := s.queries().ListCurrentExtractedUnits(ctx, sqlcgen.ListCurrentExtractedUnitsParams{OwnerID: owner, SourceMaterialID: sourceID})
	if err != nil {
		return "", out, err
	}
	for _, row := range rows {
		order, conversionErr := checked.Uint64FromInt64(row.UnitOrder)
		if conversionErr != nil {
			return "", out, fmt.Errorf("validate persisted unit order: %w", conversionErr)
		}
		spineIndex, conversionErr := checked.Uint64FromInt64(row.SpineIndex)
		if conversionErr != nil {
			return "", out, fmt.Errorf("validate persisted spine index: %w", conversionErr)
		}
		startOffset, conversionErr := checked.Uint64FromInt64(row.StartOffset)
		if conversionErr != nil {
			return "", out, fmt.Errorf("validate persisted unit start offset: %w", conversionErr)
		}
		endOffset, conversionErr := checked.Uint64FromInt64(row.EndOffset)
		if conversionErr != nil {
			return "", out, fmt.Errorf("validate persisted unit end offset: %w", conversionErr)
		}
		var unit domain.ExtractedUnit
		unit.ID, unit.Order, unit.SpineIndex = row.UnitID, order, spineIndex
		unit.Title, unit.TitleSource, unit.Text = row.Title, row.TitleSource, row.Text
		unit.StartOffset, unit.EndOffset = startOffset, endOffset
		unit.PackagePath, unit.ManifestID, unit.SourceHref = row.PackagePath, row.ManifestID, row.SourceHref
		unit.ResolvedHref, unit.MediaType, unit.Linear = row.ResolvedHref, row.MediaType, row.Linear
		if err := json.Unmarshal(row.Properties, &unit.Properties); err != nil {
			return "", out, err
		}
		if err := json.Unmarshal(row.NavigationLabels, &unit.NavigationLabels); err != nil {
			return "", out, err
		}
		if err := json.Unmarshal(row.LandmarkTypes, &unit.LandmarkTypes); err != nil {
			return "", out, err
		}
		out.Units = append(out.Units, unit)
	}
	if err = out.ValidateOffsets(snapshot.FullText); err != nil {
		return "", out, fmt.Errorf("validate persisted extracted units: %w", err)
	}
	return snapshot.USnapshotID, out, nil
}
