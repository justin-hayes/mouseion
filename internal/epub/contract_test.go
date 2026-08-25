package epub

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestExtractedUnitIdentityIsSpineBased(t *testing.T) {
	first := UnitID(2, "chapter")
	duplicateTitle := UnitID(5, "chapter-copy")
	repeatedResource := UnitID(8, "chapter")
	if first != "epub-unit-v1:2:chapter" {
		t.Fatalf("unit ID = %q", first)
	}
	if first == duplicateTitle || first == repeatedResource || duplicateTitle == repeatedResource {
		t.Fatalf("distinct spine identities collided: %q, %q, %q", first, duplicateTitle, repeatedResource)
	}
}

func TestExtractedUnitsValidateUnicodeOffsetsAndSpineGaps(t *testing.T) {
	fullText := "Grüße 👋\n\n東京"
	units := ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{
		{ID: UnitID(1, "nested-a"), Order: 0, SpineIndex: 1, ManifestID: "nested-a", Text: "Grüße 👋", StartOffset: 0, EndOffset: 7},
		{ID: UnitID(4, "nested-b"), Order: 1, SpineIndex: 4, ManifestID: "nested-b", Text: "東京", StartOffset: 9, EndOffset: 11},
	}}
	if err := units.ValidateOffsets(fullText); err != nil {
		t.Fatal(err)
	}

	units.Units[1].StartOffset = uint64(len([]byte("Grüße 👋\n\n")))
	if err := units.ValidateOffsets(fullText); err == nil {
		t.Fatal("UTF-8 byte offsets accepted as Unicode code-point offsets")
	}
}

func TestExtractedUnitsSerializationIsVersionedAndUsesEmptyMetadataArrays(t *testing.T) {
	document := ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{{
		ID: UnitID(3, "part-1"), Order: 0, SpineIndex: 3, Title: "Same title", TitleSource: UnitTitleHeading,
		Text: "Text", EndOffset: 4, PackagePath: "OPS/package.opf", ManifestID: "part-1",
		SourceHref: "Text/part/chapter.xhtml", ResolvedHref: "OPS/Text/part/chapter.xhtml",
		MediaType: "application/xhtml+xml", Properties: []string{}, Linear: true,
		NavigationLabels: []string{}, LandmarkTypes: []string{},
	}}}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	got := string(encoded)
	for _, required := range []string{`"schema_version":1`, `"units":[`, `"source_href":"Text/part/chapter.xhtml"`, `"properties":[]`, `"navigation_labels":[]`, `"landmark_types":[]`} {
		if !strings.Contains(got, required) {
			t.Fatalf("serialized contract %s does not contain %s", got, required)
		}
	}
	if err := document.ValidateOffsets("Text"); err != nil {
		t.Fatal(err)
	}
	document.SchemaVersion++
	if err := document.ValidateOffsets("Text"); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported version error = %v", err)
	}
}

func TestExtractedUnitsRejectIdentityOrderAndTextDrift(t *testing.T) {
	tests := []struct {
		name string
		unit ExtractedUnit
	}{
		{"identity", ExtractedUnit{ID: "title-derived", ManifestID: "a", Text: "é", EndOffset: 1}},
		{"order", ExtractedUnit{ID: UnitID(0, "a"), Order: 1, ManifestID: "a", Text: "é", EndOffset: 1}},
		{"text", ExtractedUnit{ID: UnitID(0, "a"), ManifestID: "a", Text: "e", EndOffset: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document := ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{tt.unit}}
			if err := document.ValidateOffsets("é"); err == nil {
				t.Fatal("invalid contract accepted")
			}
		})
	}
}

func TestExtractedUnitsTreatMissingOrEmptyLegacyDataAsUnavailable(t *testing.T) {
	for _, document := range []ExtractedUnits{
		{},
		{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{}},
	} {
		if err := document.ValidateOffsets(""); !errors.Is(err, ErrExtractedUnitsUnavailable) {
			t.Fatalf("legacy document error = %v", err)
		}
	}
}
