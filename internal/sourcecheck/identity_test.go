package sourcecheck

import (
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snapshotFixture(schema int, text string, convention string, spans []domain.UnitRoleSpan, resets []domain.UnitHeadingReset) domain.ExtractedSnapshot {
	unit := domain.ExtractedUnit{
		ID: "unit-1", Order: 0, SpineIndex: 0, Title: "Chapter", TitleSource: domain.UnitTitleHeading,
		Text: text, StartOffset: 0, EndOffset: uint64(len([]rune(text))), PackagePath: "OPS/content.opf",
		ManifestID: "chapter", SourceHref: "chapter.xhtml", ResolvedHref: "OPS/chapter.xhtml",
		MediaType: "application/xhtml+xml", Linear: true,
		Properties: []string{}, NavigationLabels: []string{"Chapter"}, LandmarkTypes: []string{},
	}
	return domain.ExtractedSnapshot{
		SnapshotID: "snapshot-1", ContentRevisionID: "revision-1",
		Units: domain.ExtractedUnits{SchemaVersion: schema, Units: []domain.ExtractedUnit{unit}},
		Structure: domain.SnapshotStructure{
			Recorded: schema >= domain.ExtractedUnitsSchemaVersionV2, ConventionSet: convention,
			RoleSpans: spans, HeadingResets: resets,
		},
	}
}

func TestResultIdentityIsDeterministicAndCoversStructure(t *testing.T) {
	spans := []domain.UnitRoleSpan{
		{UnitID: "unit-1", Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 6},
		{UnitID: "unit-1", Role: domain.SourceStructureRoleDirection, StartOffset: 7, EndOffset: 11},
	}
	resets := []domain.UnitHeadingReset{{UnitID: "unit-1", Offset: 0}}
	base := snapshotFixture(domain.ExtractedUnitsSchemaVersionV2, "Sprecher. Text", domain.StructureConventionSet, spans, resets)
	identity := ResultIdentity(base)
	require.True(t, strings.HasPrefix(identity, "sha256:"))
	assert.Len(t, identity, len("sha256:")+64)
	assert.Equal(t, identity, ResultIdentity(base), "the same extraction must produce the same identity")

	reordered := base
	reordered.Structure.RoleSpans = []domain.UnitRoleSpan{spans[1], spans[0]}
	reordered.Structure.HeadingResets = []domain.UnitHeadingReset{{UnitID: "unit-1", Offset: 0}}
	assert.Equal(t, identity, ResultIdentity(reordered), "identity must not depend on row order")

	changedText := base
	changedText.Units.Units = append([]domain.ExtractedUnit(nil), base.Units.Units...)
	changedText.Units.Units[0].Text = "Sprecher. Anderer Text"
	assert.NotEqual(t, identity, ResultIdentity(changedText))

	changedRole := base
	changedRole.Structure.RoleSpans = []domain.UnitRoleSpan{
		{UnitID: "unit-1", Role: domain.SourceStructureRoleDirection, StartOffset: 0, EndOffset: 6},
		spans[1],
	}
	assert.NotEqual(t, identity, ResultIdentity(changedRole))

	changedMetadata := base
	changedMetadata.Units.Units = append([]domain.ExtractedUnit(nil), base.Units.Units...)
	changedMetadata.Units.Units[0].Title = "Other chapter"
	assert.NotEqual(t, identity, ResultIdentity(changedMetadata), "provenance metadata belongs to result identity")

	changedConvention := base
	changedConvention.Structure.ConventionSet = "epub-structure/2"
	assert.NotEqual(t, identity, ResultIdentity(changedConvention))
}

func TestResultIdentityOrdersTiesDeterministically(t *testing.T) {
	unitA := domain.ExtractedUnit{ID: "unit-a", Order: 0, Text: "A", EndOffset: 1}
	unitB := domain.ExtractedUnit{ID: "unit-b", Order: 0, Text: "B", EndOffset: 1}
	spans := []domain.UnitRoleSpan{
		{UnitID: "unit-a", Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 1},
		{UnitID: "unit-a", Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 2},
		{UnitID: "unit-a", Role: domain.SourceStructureRoleDirection, StartOffset: 0, EndOffset: 1},
		{UnitID: "unit-b", Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 1},
	}
	resets := []domain.UnitHeadingReset{{UnitID: "unit-b", Offset: 2}, {UnitID: "unit-a", Offset: 1}, {UnitID: "unit-a", Offset: 0}}
	base := domain.ExtractedSnapshot{
		Units:     domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersionV2, Units: []domain.ExtractedUnit{unitB, unitA}},
		Structure: domain.SnapshotStructure{Recorded: true, ConventionSet: domain.StructureConventionSet, RoleSpans: spans, HeadingResets: resets},
	}
	identity := ResultIdentity(base)
	reordered := base
	reordered.Units.Units = []domain.ExtractedUnit{unitA, unitB}
	reordered.Structure.RoleSpans = []domain.UnitRoleSpan{spans[3], spans[2], spans[1], spans[0]}
	reordered.Structure.HeadingResets = []domain.UnitHeadingReset{resets[2], resets[0], resets[1]}
	assert.Equal(t, identity, ResultIdentity(reordered))
}

func TestUnavailableIdentityIsDeterministicAndTargetBound(t *testing.T) {
	key := domain.SourceStructureCheckKey{
		OwnerID: "owner", SourceMaterialID: "source", ContentRevisionID: "revision", SnapshotID: "snapshot",
		TargetContractVersion: domain.ExtractedUnitsSchemaVersionV2, ConventionSet: domain.StructureConventionSet,
	}
	identity := UnavailableIdentity(key, "sha256:digest")
	assert.Equal(t, identity, UnavailableIdentity(key, "sha256:digest"))
	assert.True(t, strings.HasPrefix(identity, "sha256:"))
	key.TargetContractVersion = domain.ExtractedUnitsSchemaVersionV2 + 1
	assert.NotEqual(t, identity, UnavailableIdentity(key, "sha256:digest"))
}

func TestClassifyCheck(t *testing.T) {
	v1 := snapshotFixture(domain.ExtractedUnitsSchemaVersionV1, "Sprecher. Text", "", nil, nil)
	v2NoRoles := snapshotFixture(domain.ExtractedUnitsSchemaVersionV2, "Sprecher. Text", domain.StructureConventionSet, nil, nil)
	v2Roles := snapshotFixture(domain.ExtractedUnitsSchemaVersionV2, "Sprecher. Text", domain.StructureConventionSet,
		[]domain.UnitRoleSpan{{UnitID: "unit-1", Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 8}}, nil)
	v2Changed := snapshotFixture(domain.ExtractedUnitsSchemaVersionV2, "Sprecher. Anderer Text", domain.StructureConventionSet, nil, nil)

	assert.Equal(t, domain.SourceStructureUnchanged, classifyCheck(v1, v2NoRoles), "identical v1 prose with no target roles is a no-op")
	assert.Equal(t, domain.SourceStructureUnchanged, classifyCheck(v1, v1))
	assert.Equal(t, domain.SourceStructureUpdateAvailable, classifyCheck(v1, v2Roles), "a role-only change is an update")
	assert.Equal(t, domain.SourceStructureUpdateAvailable, classifyCheck(v1, v2Changed), "changed text is an update")
	assert.Equal(t, domain.SourceStructureUnchanged, classifyCheck(v2NoRoles, v2NoRoles))
	assert.Equal(t, domain.SourceStructureUpdateAvailable, classifyCheck(v2NoRoles, v2Roles))
	oldConvention := snapshotFixture(domain.ExtractedUnitsSchemaVersionV2, "Sprecher. Text", "epub-structure/0", nil, nil)
	assert.Equal(t, domain.SourceStructureUpdateAvailable, classifyCheck(oldConvention, v2NoRoles), "a convention-only change is an update")
}

func TestTargetValidationAndNeedsCheck(t *testing.T) {
	assert.False(t, ConfiguredTarget().Active(), "checks stay inert until the v2 writer is configured")
	assert.False(t, Target{ContractVersion: domain.ExtractedUnitsSchemaVersionV1}.Active())
	require.Error(t, Target{ContractVersion: domain.ExtractedUnitsSchemaVersionV1}.Validate())
	require.Error(t, Target{ContractVersion: domain.ExtractedUnitsSchemaVersionV2}.Validate())
	require.Error(t, Target{ContractVersion: domain.ExtractedUnitsSchemaVersionV2, ConventionSet: "other"}.Validate())
	require.NoError(t, Target{ContractVersion: domain.ExtractedUnitsSchemaVersionV2, ConventionSet: domain.StructureConventionSet}.Validate())
	require.Error(t, Target{ContractVersion: domain.ExtractedUnitsSchemaVersionV2 + 1, ConventionSet: domain.StructureConventionSet}.Validate())

	v1 := snapshotFixture(domain.ExtractedUnitsSchemaVersionV1, "text", "", nil, nil)
	v2 := snapshotFixture(domain.ExtractedUnitsSchemaVersionV2, "text", domain.StructureConventionSet, nil, nil)
	target := Target{ContractVersion: domain.ExtractedUnitsSchemaVersionV2, ConventionSet: domain.StructureConventionSet}
	assert.True(t, needsCheck(v1, target))
	assert.False(t, needsCheck(v2, target))
	otherConvention := v2
	otherConvention.Structure.ConventionSet = "epub-structure/0"
	assert.True(t, needsCheck(otherConvention, target))
}
