package epub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type classifierFixture struct {
	Description string         `json:"description"`
	EPUBVersion string         `json:"epub_version"`
	SnapshotID  string         `json:"snapshot_id"`
	Units       ExtractedUnits `json:"units"`
}

type expectedClassification struct {
	category   UnitCategory
	confidence uint8
	include    bool
	reasons    []ClassificationReason
}

func TestClassifierReadableEPUBFixtureSnapshots(t *testing.T) {
	tests := []struct {
		name string
		file string
		want []expectedClassification
	}{
		{
			name: "EPUB 2 German NCX labels",
			file: "epub2-german.json",
			want: []expectedClassification{
				{CategoryFrontMatter, 85, false, fixtureReasons("label_contents", "path_contents", "spine_front")},
				{CategoryMainMatter, 95, true, fixtureReasons("label_chapter", "path_chapter", "spine_central", "text_sentence_density")},
				{CategoryBackMatter, 85, false, fixtureReasons("label_notes", "path_notes", "spine_central")},
				{CategoryBackMatter, 85, false, fixtureReasons("label_glossary", "path_glossary", "spine_central")},
				{CategoryBackMatter, 85, false, fixtureReasons("label_appendix", "path_appendix", "spine_back")},
			},
		},
		{
			name: "EPUB 3 Italian landmarks and edge cases",
			file: "epub3-italian-edge-cases.json",
			want: []expectedClassification{
				{CategoryFrontMatter, 95, false, fixtureReasons("landmark_frontmatter", "label_preface", "path_preface", "spine_front")},
				{CategoryMainMatter, 95, true, fixtureReasons("landmark_bodymatter", "label_chapter", "path_chapter", "spine_central", "text_sentence_density")},
				{CategoryBackMatter, 95, false, fixtureReasons("landmark_backmatter", "label_bibliography", "path_bibliography", "spine_central", "text_reference_density")},
				{CategoryBackMatter, 95, false, fixtureReasons("landmark_index", "label_index", "path_index", "spine_central")},
				{CategoryBackMatter, 95, false, fixtureReasons("landmark_endnotes", "label_notes", "path_notes", "spine_central")},
				{CategoryBackMatter, 95, false, fixtureReasons("landmark_appendix", "label_appendix", "path_appendix", "spine_central")},
				{CategoryUnknown, 20, false, fixtureReasons("navigation_only")},
				{CategoryUnknown, 20, false, fixtureReasons("spine_central", "insufficient_evidence")},
				{CategoryUnknown, 35, false, fixtureReasons("landmark_bibliography", "label_chapter", "path_chapter", "spine_back", "contradictory_evidence")},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := readClassifierFixture(t, test.file)
			if fixture.Description == "" || fixture.EPUBVersion == "" {
				t.Fatalf("fixture provenance is incomplete: %+v", fixture)
			}
			got, err := ClassifyUnits(fixture.SnapshotID, fixture.Units)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("classifications=%d want=%d", len(got), len(test.want))
			}
			for i, want := range test.want {
				classification := got[i]
				if classification.SchemaVersion != ClassificationSchemaVersion || classification.Classifier != (ClassifierIdentity{Name: ClassifierName, Version: ClassifierVersion}) {
					t.Fatalf("unit %d classifier contract=%+v", i, classification)
				}
				identity := SourceUnitSnapshotIdentity{SnapshotID: fixture.SnapshotID, ExtractedUnitsSchemaVersion: ExtractedUnitsSchemaVersion, UnitID: fixture.Units.Units[i].ID}
				if classification.SourceUnitSnapshot != identity {
					t.Fatalf("unit %d identity=%+v want=%+v", i, classification.SourceUnitSnapshot, identity)
				}
				if classification.Category != want.category || classification.Confidence != want.confidence || classification.RecommendedInclusion != want.include || !reflect.DeepEqual(classification.Reasons, want.reasons) {
					t.Fatalf("unit %d classification=%+v\nwant category=%s confidence=%d include=%t reasons=%+v", i, classification, want.category, want.confidence, want.include, want.reasons)
				}
			}
		})
	}
}

func readClassifierFixture(t *testing.T, name string) classifierFixture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testfixtures", "classifier", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixture classifierFixture
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func fixtureReasons(signals ...string) []ClassificationReason {
	messages := map[string]string{
		"landmark_frontmatter":   "The EPUB landmark identifies front matter.",
		"landmark_bodymatter":    "The EPUB landmark identifies main matter.",
		"landmark_backmatter":    "The EPUB landmark identifies back matter.",
		"landmark_bibliography":  "The EPUB landmark identifies back matter.",
		"landmark_index":         "The EPUB landmark identifies back matter.",
		"landmark_endnotes":      "The EPUB landmark identifies back matter.",
		"landmark_appendix":      "The EPUB landmark identifies back matter.",
		"label_contents":         "A title or navigation label matches the contents marker.",
		"label_preface":          "A title or navigation label matches the preface marker.",
		"label_chapter":          "A title or navigation label matches the chapter marker.",
		"label_bibliography":     "A title or navigation label matches the bibliography marker.",
		"label_notes":            "A title or navigation label matches the notes marker.",
		"label_index":            "A title or navigation label matches the index marker.",
		"label_glossary":         "A title or navigation label matches the glossary marker.",
		"label_appendix":         "A title or navigation label matches the appendix marker.",
		"path_contents":          "The package path matches the contents marker.",
		"path_preface":           "The package path matches the preface marker.",
		"path_chapter":           "The package path matches the chapter marker.",
		"path_bibliography":      "The package path matches the bibliography marker.",
		"path_notes":             "The package path matches the notes marker.",
		"path_index":             "The package path matches the index marker.",
		"path_glossary":          "The package path matches the glossary marker.",
		"path_appendix":          "The package path matches the appendix marker.",
		"spine_front":            "The unit is at the beginning of the readable spine.",
		"spine_central":          "The unit is in the central readable spine range.",
		"spine_back":             "The unit is at the end of the readable spine.",
		"text_sentence_density":  "The text contains sustained sentence-like prose.",
		"text_reference_density": "The text has a high density of citations, years, ISBN/URL, or page-reference markers.",
		"navigation_only":        "The manifest identifies this unit as a navigation document.",
		"insufficient_evidence":  "The available signals do not establish a structural category.",
		"contradictory_evidence": "Strong signals support conflicting structural categories.",
	}
	reasons := make([]ClassificationReason, len(signals))
	for i, signal := range signals {
		reasons[i] = ClassificationReason{Signal: signal, Message: messages[signal]}
	}
	return reasons
}
