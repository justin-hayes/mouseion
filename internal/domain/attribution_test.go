package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func attributionUnit(text string) ExtractedUnit {
	return ExtractedUnit{ID: EPUBUnitID(0, "unit"), Text: text, EndOffset: uint64(len([]rune(text)))}
}

func TestSnapshotStructureAttributionAt(t *testing.T) {
	t.Run("nearest preceding speaker label", func(t *testing.T) {
		unit := attributionUnit("Erster. Zweiter. Satz.")
		structure := SnapshotStructure{Recorded: true, ConventionSet: StructureConventionSet, RoleSpans: []UnitRoleSpan{
			{UnitID: unit.ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 7},
			{UnitID: unit.ID, Role: SourceStructureRoleSpeaker, StartOffset: 8, EndOffset: 16},
		}}
		require.Equal(t, SourceAttribution{Kind: AttributionSpeaker, Speaker: "Zweiter"}, structure.AttributionAt(unit, 17, 22))
	})

	t.Run("heading resets preceding attribution", func(t *testing.T) {
		unit := attributionUnit("Erster.\n\nAuftritt\n\nSatz.")
		structure := SnapshotStructure{Recorded: true, ConventionSet: StructureConventionSet,
			RoleSpans:     []UnitRoleSpan{{UnitID: unit.ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 7}},
			HeadingResets: []UnitHeadingReset{{UnitID: unit.ID, Offset: 9}},
		}
		require.Equal(t, SourceAttribution{}, structure.AttributionAt(unit, 20, 24))
	})

	t.Run("a label after a heading supplies attribution again", func(t *testing.T) {
		unit := attributionUnit("Erster.\n\nAuftritt\n\nZweiter. Satz.")
		structure := SnapshotStructure{Recorded: true, ConventionSet: StructureConventionSet,
			RoleSpans: []UnitRoleSpan{
				{UnitID: unit.ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 7},
				{UnitID: unit.ID, Role: SourceStructureRoleSpeaker, StartOffset: 19, EndOffset: 27},
			},
			HeadingResets: []UnitHeadingReset{{UnitID: unit.ID, Offset: 9}},
		}
		require.Equal(t, SourceAttribution{Kind: AttributionSpeaker, Speaker: "Zweiter"}, structure.AttributionAt(unit, 28, 32))
	})

	t.Run("a direction has no speaker and does not clear the preceding one", func(t *testing.T) {
		unit := attributionUnit("Erster. Er tritt ein. Satz.")
		structure := SnapshotStructure{Recorded: true, ConventionSet: StructureConventionSet, RoleSpans: []UnitRoleSpan{
			{UnitID: unit.ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 7},
			{UnitID: unit.ID, Role: SourceStructureRoleDirection, StartOffset: 8, EndOffset: 21},
		}}
		require.Equal(t, SourceAttribution{Kind: AttributionStageDirection}, structure.AttributionAt(unit, 8, 21))
		require.Equal(t, SourceAttribution{Kind: AttributionSpeaker, Speaker: "Erster"}, structure.AttributionAt(unit, 22, 26))
	})

	t.Run("no preceding label has no attribution", func(t *testing.T) {
		unit := attributionUnit("Nur ein Satz.")
		structure := SnapshotStructure{Recorded: true, ConventionSet: StructureConventionSet}
		require.Equal(t, SourceAttribution{}, structure.AttributionAt(unit, 0, 4))
	})

	t.Run("structure not recorded has no attribution", func(t *testing.T) {
		unit := attributionUnit("Erster. Satz.")
		structure := SnapshotStructure{Recorded: false}
		require.Equal(t, SourceAttribution{}, structure.AttributionAt(unit, 8, 12))
	})

	t.Run("unknown convention markup establishes no label", func(t *testing.T) {
		unit := attributionUnit("Erster. Satz.")
		structure := SnapshotStructure{Recorded: true, ConventionSet: "epub-structure/unknown"}
		require.Equal(t, SourceAttribution{}, structure.AttributionAt(unit, 8, 12))
	})

	t.Run("a nested direction is omitted from the displayed name", func(t *testing.T) {
		unit := attributionUnit("Valentin fällt. O weh!")
		structure := SnapshotStructure{Recorded: true, ConventionSet: StructureConventionSet, RoleSpans: []UnitRoleSpan{
			{UnitID: unit.ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 15},
			{UnitID: unit.ID, Role: SourceStructureRoleDirection, StartOffset: 9, EndOffset: 15},
		}}
		require.Equal(t, SourceAttribution{Kind: AttributionSpeaker, Speaker: "Valentin"}, structure.AttributionAt(unit, 16, 22))
	})

	t.Run("one terminal period is removed from the displayed name", func(t *testing.T) {
		unit := attributionUnit("Director. Satz.")
		structure := SnapshotStructure{Recorded: true, ConventionSet: StructureConventionSet, RoleSpans: []UnitRoleSpan{
			{UnitID: unit.ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 9},
		}}
		require.Equal(t, SourceAttribution{Kind: AttributionSpeaker, Speaker: "Director"}, structure.AttributionAt(unit, 10, 15))
	})

	t.Run("invalid spans have no attribution", func(t *testing.T) {
		unit := attributionUnit("Satz.")
		structure := SnapshotStructure{Recorded: true, ConventionSet: StructureConventionSet, RoleSpans: []UnitRoleSpan{
			{UnitID: unit.ID, Role: SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 5},
		}}
		require.Equal(t, SourceAttribution{}, structure.AttributionAt(unit, 5, 5))
		require.Equal(t, SourceAttribution{}, structure.AttributionAt(unit, 0, 99))
	})
}

func TestSpeakerDisplayNameRejectsInvalidLabel(t *testing.T) {
	t.Run("label past the text", func(t *testing.T) {
		require.Empty(t, speakerDisplayName([]rune("Text."), UnitRoleSpan{StartOffset: 0, EndOffset: 99}, nil))
	})
	t.Run("empty label", func(t *testing.T) {
		require.Empty(t, speakerDisplayName([]rune("Text."), UnitRoleSpan{StartOffset: 3, EndOffset: 3}, nil))
	})
}
