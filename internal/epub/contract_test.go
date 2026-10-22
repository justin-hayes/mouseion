package epub

import (
	"encoding/json"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEPUBContentDigestHashesExactContainerBytes(t *testing.T) {
	first := []byte("epub bytes")
	second := append([]byte(nil), first...)
	got, want := domain.EPUBContentDigest(first), domain.EPUBContentDigest(second)
	assert.Equal(t, want, got, "equal bytes produced different digests")
	assert.NotEqual(t, domain.EPUBContentDigest([]byte("metadata-only")), got, "digest did not identify changed source bytes")
	require.Len(t, got, len("sha256:")+64, "digest format")
	assert.Equal(t, "sha256:", got[:7], "digest format")
}

func TestExtractedUnitIdentityIsSpineBased(t *testing.T) {
	first := UnitID(2, "chapter")
	duplicateTitle := UnitID(5, "chapter-copy")
	repeatedResource := UnitID(8, "chapter")
	assert.Equal(t, "epub-unit-v1:2:chapter", first)
	assert.NotEqual(t, duplicateTitle, first, "distinct spine identities collided")
	assert.NotEqual(t, repeatedResource, first, "distinct spine identities collided")
	assert.NotEqual(t, repeatedResource, duplicateTitle, "distinct spine identities collided")
}

func TestExtractedUnitsValidateUnicodeOffsetsAndSpineGaps(t *testing.T) {
	fullText := "Grüße 👋\n\n東京"
	units := ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{
		{ID: UnitID(1, "nested-a"), Order: 0, SpineIndex: 1, ManifestID: "nested-a", Text: "Grüße 👋", StartOffset: 0, EndOffset: 7},
		{ID: UnitID(4, "nested-b"), Order: 1, SpineIndex: 4, ManifestID: "nested-b", Text: "東京", StartOffset: 9, EndOffset: 11},
	}}
	require.NoError(t, units.ValidateOffsets(fullText))

	units.Units[1].StartOffset = uint64(len([]byte("Grüße 👋\n\n")))
	assert.Error(t, units.ValidateOffsets(fullText), "UTF-8 byte offsets accepted as Unicode code-point offsets")
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
	require.NoError(t, err)
	got := string(encoded)
	for _, required := range []string{`"schema_version":2`, `"units":[`, `"source_href":"Text/part/chapter.xhtml"`, `"properties":[]`, `"navigation_labels":[]`, `"landmark_types":[]`} {
		assert.Contains(t, got, required, "serialized contract %s does not contain %s", got, required)
	}
	require.NoError(t, document.ValidateOffsets("Text"))
	document.SchemaVersion += 2
	err = document.ValidateOffsets("Text")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
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
			assert.Error(t, document.ValidateOffsets("é"), "invalid contract accepted")
		})
	}
}

// TestExtractedUnitsRejectOffsetInvariantViolations drives each offset check
// that ValidateOffsets makes on its own: duplicate identity, a first unit that
// does not start at zero, a later unit that does not preserve the FullText
// separator, and a final unit that does not end at the FullText length.
func TestExtractedUnitsRejectOffsetInvariantViolations(t *testing.T) {
	fullText := "ab\n\ncd"
	unit := func(order, spine uint64, manifest, text string, start, end uint64) ExtractedUnit {
		return ExtractedUnit{ID: UnitID(spine, manifest), Order: order, SpineIndex: spine, ManifestID: manifest, Text: text, StartOffset: start, EndOffset: end}
	}
	require.NoError(t, ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{
		unit(0, 0, "a", "ab", 0, 2),
		unit(1, 1, "b", "cd", 4, 6),
	}}.ValidateOffsets(fullText), "valid fixture")

	tests := []struct {
		name  string
		units []ExtractedUnit
		want  string
	}{
		{
			name:  "duplicate identity",
			units: []ExtractedUnit{unit(0, 0, "a", "ab", 0, 2), unit(1, 0, "a", "cd", 4, 6)},
			want:  "duplicate unit identity",
		},
		{
			name:  "first unit does not start at zero",
			units: []ExtractedUnit{unit(0, 0, "a", "b", 1, 2), unit(1, 1, "b", "cd", 4, 6)},
			want:  "first unit starts at offset 1",
		},
		{
			name:  "missing FullText separator",
			units: []ExtractedUnit{unit(0, 0, "a", "ab", 0, 2), unit(1, 1, "b", "cd", 5, 6)},
			want:  "does not preserve the FullText separator",
		},
		{
			name:  "final unit does not reach FullText length",
			units: []ExtractedUnit{unit(0, 0, "a", "ab", 0, 2), unit(1, 1, "b", "c", 4, 5)},
			want:  "final unit does not end at FullText length",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: tt.units}.ValidateOffsets(fullText)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestExtractedUnitsTreatMissingOrEmptyLegacyDataAsUnavailable(t *testing.T) {
	for _, document := range []ExtractedUnits{
		{},
		{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{}},
	} {
		assert.ErrorIs(t, document.ValidateOffsets(""), ErrExtractedUnitsUnavailable, "legacy document error")
	}
}
