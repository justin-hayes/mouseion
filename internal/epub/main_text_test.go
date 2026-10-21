package epub

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				Identified: true, Applies: true, LandmarkApplies: true, BodyMatterStart: 1, BackMatterStart: 2,
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
				Identified: true, Applies: true, LandmarkApplies: true, BodyMatterStart: 1, BackMatterStart: 3,
				SelectedUnitIDs: []string{"unit-1", "unit-2"}, ExcludedUnitIDs: []string{"unit-0"},
			},
		},
		{
			name:  "every back matter class ends the run",
			units: mainTextUnits([]string{"bodymatter"}, []string{"backmatter"}, []string{"glossary"}, []string{"index"}, []string{"colophon"}),
			want: MainTextSelection{
				Identified: true, Applies: true, LandmarkApplies: true, BodyMatterStart: 0, BackMatterStart: 1,
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

func TestIdentifyMainTextRecognisesProjectGutenbergPackaging(t *testing.T) {
	const (
		headerText = "The Project Gutenberg eBook of Beispiel.\n\n" +
			"This eBook is for the use of anyone anywhere.\n\n" +
			"*** START OF THE PROJECT GUTENBERG EBOOK BEISPIEL ***\n\n" +
			"Erstes Buchkapitel.\n\nZweites Buchkapitel."
		footerText = "*** END OF THE PROJECT GUTENBERG EBOOK BEISPIEL ***\n\n" +
			"Full English licence text."
	)
	units := mainTextTextUnits(
		mainTextSpec{manifestID: "pg-header", text: headerText},
		mainTextSpec{manifestID: "chapter", text: "Drittes Buchkapitel."},
		mainTextSpec{manifestID: "pg-footer", text: footerText},
		mainTextSpec{manifestID: "illustration", text: "back"},
	)
	headerTrimEnd := uint64(len([]rune("The Project Gutenberg eBook of Beispiel.\n\nThis eBook is for the use of anyone anywhere.\n\n*** START OF THE PROJECT GUTENBERG EBOOK BEISPIEL ***")))
	footerTrimStart := uint64(0)
	footerTrimEnd := uint64(len([]rune(footerText)))

	want := MainTextSelection{
		Applies:         true,
		SelectedUnitIDs: []string{"unit-0", "unit-1", "unit-2"},
		ExcludedUnitIDs: []string{"unit-3"},
		Distributor: DistributorSelection{
			Recognised:      true,
			Convention:      DistributorConventionGutenberg,
			ExcludedUnitIDs: []string{"unit-3"},
			TrimmedRanges: []UnitTextRange{
				{UnitID: "unit-0", StartOffset: 0, EndOffset: headerTrimEnd},
				{UnitID: "unit-2", StartOffset: footerTrimStart, EndOffset: footerTrimEnd},
			},
		},
	}
	assert.Equal(t, want, IdentifyMainText(units))
}

func TestIdentifyMainTextKeepsBookTextInTheHeaderUnit(t *testing.T) {
	const headerText = "The Project Gutenberg eBook of Werther.\n\n" +
		"*** START OF THE PROJECT GUTENBERG EBOOK WERTHER ***\n\n" +
		"Erstes Buch.\n\nZweites Buch."
	const footerText = "*** END OF THE PROJECT GUTENBERG EBOOK WERTHER ***\n\nLicence."
	units := mainTextTextUnits(
		mainTextSpec{manifestID: "pg-header", text: headerText},
		mainTextSpec{manifestID: "pg-footer", text: footerText},
	)
	headerTrimEnd := uint64(len([]rune("The Project Gutenberg eBook of Werther.\n\n*** START OF THE PROJECT GUTENBERG EBOOK WERTHER ***")))

	want := MainTextSelection{
		Applies:         true,
		SelectedUnitIDs: []string{"unit-0", "unit-1"},
		ExcludedUnitIDs: []string{},
		Distributor: DistributorSelection{
			Recognised:      true,
			Convention:      DistributorConventionGutenberg,
			ExcludedUnitIDs: []string{},
			TrimmedRanges: []UnitTextRange{
				{UnitID: "unit-0", StartOffset: 0, EndOffset: headerTrimEnd},
				{UnitID: "unit-1", StartOffset: 0, EndOffset: uint64(len([]rune(footerText)))},
			},
		},
	}
	assert.Equal(t, want, IdentifyMainText(units))
}

func TestIdentifyMainTextTrimsAMarkerPairInOneUnit(t *testing.T) {
	const (
		headerText = "Preamble.\n\n*** START OF THE PROJECT GUTENBERG EBOOK X ***\n\nBook.\n\n*** END OF THE PROJECT GUTENBERG EBOOK X ***\n\nLicence."
		startLine  = "*** START OF THE PROJECT GUTENBERG EBOOK X ***"
	)
	units := mainTextTextUnits(mainTextSpec{manifestID: "pg-header", text: headerText})
	startEnd := uint64(len([]rune("Preamble.\n\n" + startLine)))
	endStart := uint64(len([]rune("Preamble.\n\n" + startLine + "\n\nBook.\n\n")))

	decision := IdentifyMainText(units)
	assert.True(t, decision.Distributor.Recognised)
	assert.Equal(t, []string{"unit-0"}, decision.SelectedUnitIDs)
	assert.Equal(t, []UnitTextRange{
		{UnitID: "unit-0", StartOffset: 0, EndOffset: startEnd},
		{UnitID: "unit-0", StartOffset: endStart, EndOffset: uint64(len([]rune(headerText)))},
	}, decision.Distributor.TrimmedRanges)
}

func TestIdentifyMainTextRecognisesALicenceUnitWithAGenericManifestID(t *testing.T) {
	units := mainTextTextUnits(
		mainTextSpec{manifestID: "pg-header", text: "Preamble.\n\n*** START OF THE PROJECT GUTENBERG EBOOK X ***\n\nBook."},
		mainTextSpec{manifestID: "item7", text: "*** END OF THE PROJECT GUTENBERG EBOOK X ***\n\nLicence."},
	)
	decision := IdentifyMainText(units)
	assert.True(t, decision.Distributor.Recognised)
	require.Len(t, decision.Distributor.TrimmedRanges, 2)
	assert.Equal(t, "unit-1", decision.Distributor.TrimmedRanges[1].UnitID)
	assert.Empty(t, decision.Distributor.ExcludedUnitIDs)
}

func TestIdentifyMainTextFallsBackWhenGutenbergPackagingIsAmbiguous(t *testing.T) {
	const (
		startLine = "*** START OF THE PROJECT GUTENBERG EBOOK X ***"
		endLine   = "*** END OF THE PROJECT GUTENBERG EBOOK X ***"
	)
	tests := []struct {
		name  string
		units []ExtractedUnit
	}{
		{
			name: "missing pg-header",
			units: mainTextTextUnits(
				mainTextSpec{manifestID: "header", text: "Preamble.\n\n" + startLine + "\n\nBook."},
				mainTextSpec{manifestID: "item7", text: endLine + "\n\nLicence."},
			),
		},
		{
			name: "duplicate pg-header",
			units: mainTextTextUnits(
				mainTextSpec{manifestID: "pg-header", text: "Preamble.\n\n" + startLine + "\n\nBook."},
				mainTextSpec{manifestID: "pg-header", text: "Another header."},
				mainTextSpec{manifestID: "item7", text: endLine + "\n\nLicence."},
			),
		},
		{
			name: "repeated START line",
			units: mainTextTextUnits(
				mainTextSpec{manifestID: "pg-header", text: "Preamble.\n\n" + startLine + "\n\nBook.\n\n" + startLine},
				mainTextSpec{manifestID: "item7", text: endLine + "\n\nLicence."},
			),
		},
		{
			name: "missing START line",
			units: mainTextTextUnits(
				mainTextSpec{manifestID: "pg-header", text: "Preamble only."},
				mainTextSpec{manifestID: "item7", text: endLine + "\n\nLicence."},
			),
		},
		{
			name: "missing END line",
			units: mainTextTextUnits(
				mainTextSpec{manifestID: "pg-header", text: "Preamble.\n\n" + startLine + "\n\nBook."},
			),
		},
		{
			name: "repeated END line",
			units: mainTextTextUnits(
				mainTextSpec{manifestID: "pg-header", text: "Preamble.\n\n" + startLine + "\n\nBook."},
				mainTextSpec{manifestID: "item7", text: endLine + "\n\nLicence.\n\n" + endLine},
			),
		},
		{
			name: "END before START",
			units: mainTextTextUnits(
				mainTextSpec{manifestID: "pg-header", text: endLine + "\n\nPreamble.\n\n" + startLine + "\n\nBook."},
			),
		},
		{
			name: "START outside pg-header",
			units: mainTextTextUnits(
				mainTextSpec{manifestID: "pg-header", text: "Preamble only."},
				mainTextSpec{manifestID: "chapter", text: startLine + "\n\nBook."},
				mainTextSpec{manifestID: "item7", text: endLine + "\n\nLicence."},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := IdentifyMainText(tt.units)
			assert.False(t, decision.Distributor.Recognised)
			assert.Equal(t, extractedUnitIDs(tt.units), decision.SelectedUnitIDs)
			assert.Empty(t, decision.ExcludedUnitIDs)
		})
	}
}

func TestIdentifyMainTextCombinesLandmarksAndDistributorText(t *testing.T) {
	units := mainTextTextUnits(
		mainTextSpec{manifestID: "title", text: "Title page.", landmarks: []string{"titlepage"}},
		mainTextSpec{manifestID: "pg-header", text: "Preamble.\n\n*** START OF THE PROJECT GUTENBERG EBOOK X ***\n\nBook.", landmarks: []string{"bodymatter"}},
		mainTextSpec{manifestID: "chapter", text: "Second chapter."},
		mainTextSpec{manifestID: "item7", text: "*** END OF THE PROJECT GUTENBERG EBOOK X ***\n\nLicence.", landmarks: []string{"bibliography"}},
		mainTextSpec{manifestID: "illustration", text: "back"},
	)
	decision := IdentifyMainText(units)
	assert.True(t, decision.Identified)
	assert.True(t, decision.LandmarkApplies)
	assert.True(t, decision.Applies)
	assert.True(t, decision.Distributor.Recognised)
	assert.Equal(t, []string{"unit-1", "unit-2"}, decision.SelectedUnitIDs)
	assert.Equal(t, []string{"unit-0", "unit-3", "unit-4"}, decision.ExcludedUnitIDs)
	assert.Equal(t, []string{"unit-4"}, decision.Distributor.ExcludedUnitIDs)
	assert.NotEmpty(t, decision.Distributor.TrimmedRanges)
}

func TestIdentifyMainTextLeavesNonGutenbergEPUBSUnchanged(t *testing.T) {
	units := mainTextTextUnits(
		mainTextSpec{manifestID: "front", text: "Vorwort.", landmarks: []string{"titlepage"}},
		mainTextSpec{manifestID: "chapter-one", text: "Kapitel eins.", landmarks: []string{"bodymatter"}},
		mainTextSpec{manifestID: "bibliography", text: "Zusatztext.", landmarks: []string{"bibliography"}},
	)
	decision := IdentifyMainText(units)
	assert.True(t, decision.Identified)
	assert.True(t, decision.LandmarkApplies)
	assert.False(t, decision.Distributor.Recognised)
	assert.Equal(t, []string{"unit-1"}, decision.SelectedUnitIDs)
	assert.Equal(t, []string{"unit-0", "unit-2"}, decision.ExcludedUnitIDs)
	assert.Empty(t, decision.Distributor.TrimmedRanges)
	assert.Empty(t, decision.Distributor.ExcludedUnitIDs)
}

func extractedUnitIDs(units []ExtractedUnit) []string {
	ids := make([]string, 0, len(units))
	for _, unit := range units {
		ids = append(ids, unit.ID)
	}
	return ids
}

func mainTextUnits(landmarks ...[]string) []ExtractedUnit {
	units := make([]ExtractedUnit, len(landmarks))
	for i, values := range landmarks {
		units[i] = ExtractedUnit{ID: "unit-" + strconv.Itoa(i), Order: uint64(i), LandmarkTypes: values}
	}
	return units
}

type mainTextSpec struct {
	manifestID string
	text       string
	landmarks  []string
}

func mainTextTextUnits(specs ...mainTextSpec) []ExtractedUnit {
	units := make([]ExtractedUnit, len(specs))
	for i, spec := range specs {
		units[i] = ExtractedUnit{
			ID: "unit-" + strconv.Itoa(i), Order: uint64(i),
			ManifestID: spec.manifestID, Text: spec.text, LandmarkTypes: spec.landmarks,
		}
	}
	return units
}
