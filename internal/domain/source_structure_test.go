package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func structureFixtureUnits() []ExtractedUnit {
	first, second := "Mephistopheles. Was fällt euch an?", "Entzückung oder Schmerzen?"
	firstEnd := uint64(len([]rune(first)))
	secondStart := firstEnd + 2
	return []ExtractedUnit{
		{ID: EPUBUnitID(0, "one"), Order: 0, SpineIndex: 0, ManifestID: "one", Text: first, StartOffset: 0, EndOffset: firstEnd},
		{ID: EPUBUnitID(1, "two"), Order: 1, SpineIndex: 1, ManifestID: "two", Text: second, StartOffset: secondStart, EndOffset: secondStart + uint64(len([]rune(second)))},
	}
}

func TestExtractedUnitsReconstructsSnapshotTextFromUnits(t *testing.T) {
	units := ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersionV2, Units: structureFixtureUnits()}
	require.Equal(t, "Mephistopheles. Was fällt euch an?\n\nEntzückung oder Schmerzen?", units.ReconstructText())
	require.NoError(t, units.ValidateSnapshot())
}

func TestExtractedUnitsReadsBothContracts(t *testing.T) {
	for _, version := range []int{ExtractedUnitsSchemaVersionV1, ExtractedUnitsSchemaVersionV2} {
		units := ExtractedUnits{SchemaVersion: version, Units: structureFixtureUnits()}
		require.NoError(t, units.ValidateSnapshot(), "schema version %d", version)
	}
}

func TestSnapshotStructureValidate(t *testing.T) {
	units := structureFixtureUnits()
	speaker := UnitRoleSpan{UnitID: units[0].ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 22}
	directionInsideSpeaker := UnitRoleSpan{UnitID: units[0].ID, Role: SourceStructureRoleDirection, StartOffset: 16, EndOffset: 22}
	tests := []struct {
		name      string
		version   int
		structure SnapshotStructure
		want      string
	}{
		{
			name:      "v1 structure not recorded",
			version:   ExtractedUnitsSchemaVersionV1,
			structure: SnapshotStructure{},
		},
		{
			name:    "v1 with empty role data still rejects recorded structure",
			version: ExtractedUnitsSchemaVersionV1,
			structure: SnapshotStructure{
				ConventionSet: StructureConventionSet,
			},
			want: "v1 snapshot must not record source structure",
		},
		{
			name:      "v2 requires the recorded flag",
			version:   ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{ConventionSet: StructureConventionSet},
			want:      "v2 snapshot must record source structure",
		},
		{
			name:      "v2 requires a convention set",
			version:   ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{Recorded: true},
			want:      "must name its structure convention set",
		},
		{
			name:    "direction nested in a speaker label is proper nesting",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				RoleSpans:     []UnitRoleSpan{speaker, directionInsideSpeaker},
				HeadingResets: []UnitHeadingReset{{UnitID: units[0].ID, Offset: 0}},
			},
		},
		{
			name:    "disjoint role spans",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				RoleSpans: []UnitRoleSpan{
					{UnitID: units[0].ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 5},
					{UnitID: units[0].ID, Role: SourceStructureRoleDirection, StartOffset: 6, EndOffset: 12},
				},
			},
		},
		{
			name:    "spans in different units do not interact",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				RoleSpans: []UnitRoleSpan{
					{UnitID: units[0].ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 5},
					{UnitID: units[1].ID, Role: SourceStructureRoleDirection, StartOffset: 0, EndOffset: 5},
				},
			},
		},
		{
			name:    "nested spans accept the inner-first order",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				RoleSpans: []UnitRoleSpan{
					{UnitID: units[0].ID, Role: SourceStructureRoleDirection, StartOffset: 16, EndOffset: 22},
					{UnitID: units[0].ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 22},
				},
			},
		},
		{
			name:    "inner-first order still rejects a speaker inside a direction",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				RoleSpans: []UnitRoleSpan{
					{UnitID: units[0].ID, Role: SourceStructureRoleSpeaker, StartOffset: 16, EndOffset: 22},
					{UnitID: units[0].ID, Role: SourceStructureRoleDirection, StartOffset: 0, EndOffset: 22},
				},
			},
			want: "nests a speaker role inside a direction",
		},
		{
			name:    "unknown role",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				RoleSpans: []UnitRoleSpan{{UnitID: units[0].ID, Role: "heading", StartOffset: 0, EndOffset: 4}},
			},
			want: `unknown source structure role "heading"`,
		},
		{
			name:    "empty span",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				RoleSpans: []UnitRoleSpan{{UnitID: units[0].ID, Role: SourceStructureRoleSpeaker, StartOffset: 4, EndOffset: 4}},
			},
			want: "outside its text",
		},
		{
			name:    "span past unit text",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				RoleSpans: []UnitRoleSpan{{UnitID: units[0].ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 100}},
			},
			want: "outside its text",
		},
		{
			name:    "span in another snapshot unit",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				RoleSpans: []UnitRoleSpan{{UnitID: "epub-unit-v1:9:other", Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 4}},
			},
			want: "outside the snapshot",
		},
		{
			name:    "spans that cross without nesting",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				RoleSpans: []UnitRoleSpan{
					{UnitID: units[0].ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 10},
					{UnitID: units[0].ID, Role: SourceStructureRoleDirection, StartOffset: 5, EndOffset: 15},
				},
			},
			want: "overlap without nesting",
		},
		{
			name:    "same bounds with contradictory roles",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				RoleSpans: []UnitRoleSpan{
					{UnitID: units[0].ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 10},
					{UnitID: units[0].ID, Role: SourceStructureRoleDirection, StartOffset: 0, EndOffset: 10},
				},
			},
			want: "contradictory role spans",
		},
		{
			name:    "speaker nested inside a direction is contradictory",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				RoleSpans: []UnitRoleSpan{
					{UnitID: units[0].ID, Role: SourceStructureRoleDirection, StartOffset: 0, EndOffset: 10},
					{UnitID: units[0].ID, Role: SourceStructureRoleSpeaker, StartOffset: 2, EndOffset: 6},
				},
			},
			want: "nests a speaker role inside a direction",
		},
		{
			name:    "duplicate role span",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				RoleSpans: []UnitRoleSpan{speaker, speaker},
			},
			want: "duplicate role span",
		},
		{
			name:    "heading reset past unit text",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				HeadingResets: []UnitHeadingReset{{UnitID: units[0].ID, Offset: 100}},
			},
			want: "outside its text",
		},
		{
			name:    "heading reset in another snapshot unit",
			version: ExtractedUnitsSchemaVersionV2,
			structure: SnapshotStructure{
				Recorded: true, ConventionSet: StructureConventionSet,
				HeadingResets: []UnitHeadingReset{{UnitID: "epub-unit-v1:9:other", Offset: 1}},
			},
			want: "outside the snapshot",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.structure.Validate(tt.version, units)
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestSourceStructureOutcomeValid(t *testing.T) {
	for _, outcome := range []SourceStructureOutcome{SourceStructureUnchanged, SourceStructureUpdateAvailable, SourceStructureOriginalUnavailable} {
		require.True(t, outcome.Valid())
	}
	require.False(t, SourceStructureOutcome("failed").Valid())
}

// TestSnapshotStructureOverlapsSpeakerLabel holds the Study-context rule: only
// a recorded speaker label overlapping the queried span restricts it, and a v1
// snapshot (structure not recorded) derives no restriction.
func TestSnapshotStructureOverlapsSpeakerLabel(t *testing.T) {
	units := structureFixtureUnits()
	unitID := units[0].ID
	structure := SnapshotStructure{Recorded: true, ConventionSet: StructureConventionSet, RoleSpans: []UnitRoleSpan{
		{UnitID: unitID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 9},
		{UnitID: unitID, Role: SourceStructureRoleDirection, StartOffset: 5, EndOffset: 14},
	}}
	for name, span := range map[string]struct {
		start, end uint64
		want       bool
	}{
		"inside label":   {2, 8, true},
		"same bounds":    {0, 9, true},
		"partial at end": {7, 12, true},
		"adjacent after": {9, 12, false},
		"before label":   {0, 0, false},
		"empty span":     {4, 4, false},
		"another unit":   {0, 9, false},
	} {
		t.Run(name, func(t *testing.T) {
			spanUnit := unitID
			if name == "another unit" {
				spanUnit = "epub-unit-v1:9:other"
			}
			assert.Equal(t, span.want, structure.OverlapsSpeakerLabel(spanUnit, span.start, span.end))
		})
	}

	directionOnly := SnapshotStructure{Recorded: true, ConventionSet: StructureConventionSet, RoleSpans: []UnitRoleSpan{
		{UnitID: unitID, Role: SourceStructureRoleDirection, StartOffset: 0, EndOffset: 9},
	}}
	assert.False(t, directionOnly.OverlapsSpeakerLabel(unitID, 0, 9), "a standalone direction is ordinary study context")

	assert.False(t, SnapshotStructure{}.OverlapsSpeakerLabel(unitID, 0, 9), "v1 records no labels, so no restriction is derived")
}
