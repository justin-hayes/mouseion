package epub

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdentifyMainText(t *testing.T) {
	tests := []struct {
		name  string
		units []ExtractedUnit
		want  MainTextSelection
	}{
		{
			name:  "no bodymatter falls back to whole snapshot",
			units: mainTextUnits(nil, nil, []string{"bibliography"}),
			want:  MainTextSelection{SelectedUnitIDs: []string{"unit-0", "unit-1", "unit-2"}, ExcludedUnitIDs: []string{}},
		},
		{
			name:  "duplicate bodymatter falls back to whole snapshot",
			units: mainTextUnits([]string{"bodymatter"}, []string{"BODYMATTER"}, nil),
			want:  MainTextSelection{SelectedUnitIDs: []string{"unit-0", "unit-1", "unit-2"}, ExcludedUnitIDs: []string{}},
		},
		{
			name:  "backmatter on body start is an empty run",
			units: mainTextUnits([]string{"bodymatter", "index"}, nil, nil),
			want:  MainTextSelection{SelectedUnitIDs: []string{"unit-0", "unit-1", "unit-2"}, ExcludedUnitIDs: []string{}},
		},
		{
			name:  "front and back matter are excluded",
			units: mainTextUnits([]string{"titlepage"}, []string{"bodymatter", "chapter"}, []string{"Bibliography"}, []string{"colophon"}),
			want: MainTextSelection{
				Identified: true, Applies: true, BodyMatterStart: 1, BackMatterStart: 2,
				SelectedUnitIDs: []string{"unit-1"}, ExcludedUnitIDs: []string{"unit-0", "unit-2", "unit-3"},
			},
		},
		{
			name:  "bodymatter at the beginning without back matter is a no-op",
			units: mainTextUnits([]string{"BODYMATTER"}, nil, []string{"appendix"}, []string{"notes"}, []string{"endnotes"}),
			want: MainTextSelection{
				Identified: true, BodyMatterStart: 0, BackMatterStart: 5,
				SelectedUnitIDs: []string{"unit-0", "unit-1", "unit-2", "unit-3", "unit-4"}, ExcludedUnitIDs: []string{},
			},
		},
		{
			name:  "back matter before body does not end the run",
			units: mainTextUnits([]string{"bibliography"}, []string{"bodymatter"}, nil),
			want: MainTextSelection{
				Identified: true, Applies: true, BodyMatterStart: 1, BackMatterStart: 3,
				SelectedUnitIDs: []string{"unit-1", "unit-2"}, ExcludedUnitIDs: []string{"unit-0"},
			},
		},
		{
			name:  "every back matter class ends the run",
			units: mainTextUnits([]string{"bodymatter"}, []string{"backmatter"}, []string{"glossary"}, []string{"index"}, []string{"colophon"}),
			want: MainTextSelection{
				Identified: true, Applies: true, BodyMatterStart: 0, BackMatterStart: 1,
				SelectedUnitIDs: []string{"unit-0"}, ExcludedUnitIDs: []string{"unit-1", "unit-2", "unit-3", "unit-4"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IdentifyMainText(tt.units))
		})
	}
}

func mainTextUnits(landmarks ...[]string) []ExtractedUnit {
	units := make([]ExtractedUnit, len(landmarks))
	for i, values := range landmarks {
		units[i] = ExtractedUnit{ID: "unit-" + strconv.Itoa(i), Order: uint64(i), LandmarkTypes: values}
	}
	return units
}
