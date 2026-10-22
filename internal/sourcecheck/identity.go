package sourcecheck

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/justin-hayes/mouseion/internal/domain"
)

const resultIdentityVersion = "source-structure-result/1"

// identityUnit is the deterministic structural/provenance view of one unit.
// Presentation choices are deliberately absent: a later renderer may change
// without making the checked extraction different.
type identityUnit struct {
	ID               string   `json:"id"`
	Order            uint64   `json:"order"`
	SpineIndex       uint64   `json:"spine_index"`
	Title            string   `json:"title"`
	TitleSource      string   `json:"title_source"`
	Text             string   `json:"text"`
	StartOffset      uint64   `json:"start_offset"`
	EndOffset        uint64   `json:"end_offset"`
	PackagePath      string   `json:"package_path"`
	ManifestID       string   `json:"manifest_id"`
	SourceHref       string   `json:"source_href"`
	ResolvedHref     string   `json:"resolved_href"`
	MediaType        string   `json:"media_type"`
	Properties       []string `json:"properties"`
	Linear           bool     `json:"linear"`
	NavigationLabels []string `json:"navigation_labels"`
	LandmarkTypes    []string `json:"landmark_types"`
}

type identityRoleSpan struct {
	UnitID      string `json:"unit_id"`
	Role        string `json:"role"`
	StartOffset uint64 `json:"start_offset"`
	EndOffset   uint64 `json:"end_offset"`
}

type identityHeadingReset struct {
	UnitID string `json:"unit_id"`
	Offset uint64 `json:"offset"`
}

type identityPayload struct {
	Version       string                 `json:"version"`
	SchemaVersion int                    `json:"schema_version"`
	ConventionSet string                 `json:"structure_convention_set"`
	Units         []identityUnit         `json:"units"`
	RoleSpans     []identityRoleSpan     `json:"role_spans"`
	HeadingResets []identityHeadingReset `json:"heading_resets"`
}

// ResultIdentity deterministically identifies an extraction's text, roles and
// structural/provenance metadata. The same bytes extracted under the same
// contract and convention set produce the same identity, so a later
// confirmation can verify the offered result instead of adopting different
// evidence.
func ResultIdentity(snapshot domain.ExtractedSnapshot) string {
	payload := identityPayload{
		Version:       resultIdentityVersion,
		SchemaVersion: snapshot.Units.SchemaVersion,
		ConventionSet: snapshot.Structure.ConventionSet,
		Units:         make([]identityUnit, 0, len(snapshot.Units.Units)),
		RoleSpans:     make([]identityRoleSpan, 0, len(snapshot.Structure.RoleSpans)),
		HeadingResets: make([]identityHeadingReset, 0, len(snapshot.Structure.HeadingResets)),
	}
	for _, unit := range snapshot.Units.Units {
		payload.Units = append(payload.Units, identityUnit{
			ID: unit.ID, Order: unit.Order, SpineIndex: unit.SpineIndex,
			Title: unit.Title, TitleSource: unit.TitleSource, Text: unit.Text,
			StartOffset: unit.StartOffset, EndOffset: unit.EndOffset,
			PackagePath: unit.PackagePath, ManifestID: unit.ManifestID,
			SourceHref: unit.SourceHref, ResolvedHref: unit.ResolvedHref,
			MediaType: unit.MediaType, Properties: unit.Properties,
			Linear: unit.Linear, NavigationLabels: unit.NavigationLabels,
			LandmarkTypes: unit.LandmarkTypes,
		})
	}
	sort.SliceStable(payload.Units, func(i, j int) bool {
		if payload.Units[i].Order != payload.Units[j].Order {
			return payload.Units[i].Order < payload.Units[j].Order
		}
		return payload.Units[i].ID < payload.Units[j].ID
	})
	for _, span := range snapshot.Structure.RoleSpans {
		payload.RoleSpans = append(payload.RoleSpans, identityRoleSpan{
			UnitID: span.UnitID, Role: string(span.Role),
			StartOffset: span.StartOffset, EndOffset: span.EndOffset,
		})
	}
	sort.SliceStable(payload.RoleSpans, func(i, j int) bool {
		first, second := payload.RoleSpans[i], payload.RoleSpans[j]
		if first.UnitID != second.UnitID {
			return first.UnitID < second.UnitID
		}
		if first.StartOffset != second.StartOffset {
			return first.StartOffset < second.StartOffset
		}
		if first.EndOffset != second.EndOffset {
			return first.EndOffset < second.EndOffset
		}
		return first.Role < second.Role
	})
	for _, reset := range snapshot.Structure.HeadingResets {
		payload.HeadingResets = append(payload.HeadingResets, identityHeadingReset{UnitID: reset.UnitID, Offset: reset.Offset})
	}
	sort.SliceStable(payload.HeadingResets, func(i, j int) bool {
		if payload.HeadingResets[i].UnitID != payload.HeadingResets[j].UnitID {
			return payload.HeadingResets[i].UnitID < payload.HeadingResets[j].UnitID
		}
		return payload.HeadingResets[i].Offset < payload.HeadingResets[j].Offset
	})
	encoded, _ := json.Marshal(payload) //nolint:errcheck,errchkjson // every identity field is a plain scalar or slice
	return hashBytes(encoded)
}

type unavailableIdentityPayload struct {
	Version               string `json:"version"`
	Outcome               string `json:"outcome"`
	TargetContractVersion int    `json:"target_contract_version"`
	ConventionSet         string `json:"structure_convention_set"`
	ContentRevisionID     string `json:"content_revision_id"`
	SnapshotID            string `json:"snapshot_id"`
	ContentDigest         string `json:"content_digest"`
}

// UnavailableIdentity identifies a check that could not extract because the
// source's retained bytes are missing. It is deterministic for the same base
// evidence and target, but it deliberately offers nothing to adopt.
func UnavailableIdentity(key domain.SourceStructureCheckKey, contentDigest string) string {
	encoded, _ := json.Marshal(unavailableIdentityPayload{ //nolint:errcheck,errchkjson // every identity field is a plain scalar
		Version: resultIdentityVersion, Outcome: string(domain.SourceStructureOriginalUnavailable),
		TargetContractVersion: key.TargetContractVersion, ConventionSet: key.ConventionSet,
		ContentRevisionID: key.ContentRevisionID, SnapshotID: key.SnapshotID, ContentDigest: contentDigest,
	})
	return hashBytes(encoded)
}

// classifyCheck compares the current snapshot with the target extraction. A v1
// snapshot whose text is identical and whose target records no roles is a
// no-op: the current snapshot and its analysis remain exactly as they are.
// Every other difference offers an update.
func classifyCheck(current, target domain.ExtractedSnapshot) domain.SourceStructureOutcome {
	if current.Units.SchemaVersion == target.Units.SchemaVersion &&
		current.Structure.ConventionSet == target.Structure.ConventionSet &&
		ResultIdentity(current) == ResultIdentity(target) {
		return domain.SourceStructureUnchanged
	}
	if current.Units.SchemaVersion < domain.ExtractedUnitsSchemaVersionV2 &&
		current.Structure.ConventionSet == "" &&
		current.Units.ReconstructText() == target.Units.ReconstructText() &&
		len(target.Structure.RoleSpans) == 0 {
		return domain.SourceStructureUnchanged
	}
	return domain.SourceStructureUpdateAvailable
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}
