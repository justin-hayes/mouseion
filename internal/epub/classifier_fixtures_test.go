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
			name: "Issue 287 complete German book regression",
			file: "issue-287-complete-german-book.json",
			want: []expectedClassification{
				{CategoryFrontMatter, 95, false, fixtureReasons("landmark_titlepage", "spine_front")},
				{CategoryFrontMatter, 85, false, fixtureReasons("label_copyright", "path_copyright", "spine_central")},
				{CategoryFrontMatter, 85, false, fixtureReasons("label_acknowledgments", "path_acknowledgments", "spine_central")},
				{CategoryFrontMatter, 95, false, fixtureReasons("landmark_frontmatter", "spine_central")},
				{CategoryFrontMatter, 70, false, fixtureReasons("label_preface", "path_preface", "spine_central", "text_sentence_density", "label_precedence")},
				{CategoryFrontMatter, 85, false, fixtureReasons("label_contents", "path_contents", "spine_central")},
				{CategoryMainMatter, 95, true, fixtureReasons("heading_roman_numeral", "heading_repeated_pattern", "path_chapter", "spine_central", "text_sentence_density")},
				{CategoryMainMatter, 95, true, fixtureReasons("heading_roman_numeral", "heading_repeated_pattern", "path_chapter", "spine_central", "text_sentence_density")},
				{CategoryMainMatter, 95, true, fixtureReasons("heading_roman_numeral", "heading_repeated_pattern", "path_chapter", "spine_central", "text_sentence_density")},
				{CategoryMainMatter, 95, true, fixtureReasons("heading_roman_numeral", "heading_repeated_pattern", "path_chapter", "spine_central", "text_sentence_density")},
				{CategoryMainMatter, 95, true, fixtureReasons("heading_roman_numeral", "heading_repeated_pattern", "path_chapter", "spine_central", "text_sentence_density")},
				{CategoryMainMatter, 95, true, fixtureReasons("heading_roman_numeral", "heading_repeated_pattern", "path_chapter", "spine_central", "text_sentence_density")},
				{CategoryMainMatter, 95, true, fixtureReasons("heading_roman_numeral", "heading_repeated_pattern", "path_chapter", "spine_central", "text_sentence_density")},
				{CategoryMainMatter, 95, true, fixtureReasons("heading_roman_numeral", "heading_repeated_pattern", "path_chapter", "spine_central", "text_sentence_density")},
				{CategoryMainMatter, 95, true, fixtureReasons("heading_roman_numeral", "heading_repeated_pattern", "path_chapter", "spine_central", "text_sentence_density")},
				{CategoryBackMatter, 85, false, fixtureReasons("label_appendix", "path_appendix", "spine_central")},
				{CategoryBackMatter, 85, false, fixtureReasons("label_notes", "path_notes", "spine_central")},
				{CategoryBackMatter, 85, false, fixtureReasons("label_image_credits", "path_image_credits", "spine_central")},
				{CategoryBackMatter, 95, false, fixtureReasons("label_references", "path_references", "spine_central", "text_reference_density")},
				{CategoryBackMatter, 85, false, fixtureReasons("label_index", "path_index", "spine_central")},
				{CategoryFrontMatter, 85, false, fixtureReasons("label_copyright", "path_copyright", "spine_back")},
			},
		},
		{
			name: "Issue 285 German reference markers with preserved multilingual rules",
			file: "issue-285-reference-markers.json",
			want: []expectedClassification{
				{CategoryFrontMatter, 85, false, fixtureReasons("label_copyright", "path_copyright", "spine_front")},
				{CategoryMainMatter, 95, true, fixtureReasons("label_chapter", "path_chapter", "spine_central", "text_sentence_density")},
				{CategoryBackMatter, 85, false, fixtureReasons("label_image_credits", "path_image_credits", "spine_central")},
				{CategoryBackMatter, 95, false, fixtureReasons("label_references", "path_references", "spine_central", "text_reference_density")},
				{CategoryBackMatter, 85, false, fixtureReasons("label_notes", "path_notes", "spine_central")},
				{CategoryBackMatter, 85, false, fixtureReasons("label_appendix", "path_appendix", "spine_back")},
			},
		},
		{
			name: "Reported German Roman-numeral book shape",
			file: "issue-284-german-book.json",
			want: []expectedClassification{
				{CategoryFrontMatter, 70, false, fixtureReasons("label_preface", "spine_front", "text_sentence_density", "label_precedence")},
				{CategoryFrontMatter, 70, false, fixtureReasons("label_contents", "spine_central")},
				{CategoryMainMatter, 95, true, fixtureReasons("heading_roman_numeral", "heading_repeated_pattern", "spine_central", "text_sentence_density")},
				{CategoryMainMatter, 95, true, fixtureReasons("heading_roman_numeral", "heading_repeated_pattern", "spine_central", "text_sentence_density")},
				{CategoryBackMatter, 70, false, fixtureReasons("label_appendix", "spine_central", "text_sentence_density", "label_precedence")},
				{CategoryBackMatter, 95, false, fixtureReasons("label_references", "spine_central", "text_reference_density")},
				{CategoryBackMatter, 70, false, fixtureReasons("label_index", "spine_back", "text_sentence_density", "label_precedence")},
			},
		},
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
				{CategoryBackMatter, 95, false, fixtureReasons("landmark_bibliography", "label_chapter", "path_chapter", "spine_back", "landmark_precedence")},
			},
		},
		{
			name: "Phase 4 observed German Italian and neutral patterns",
			file: "phase4-observed-patterns.json",
			want: []expectedClassification{
				{CategoryFrontMatter, 85, false, fixtureReasons("label_editorial", "path_editorial", "spine_front")},
				{CategoryMainMatter, 95, true, fixtureReasons("label_chapter", "path_chapter", "text_repeated_header", "text_repeated_footer", "spine_central", "text_sentence_density")},
				{CategoryMainMatter, 95, true, fixtureReasons("label_chapter", "path_chapter", "text_repeated_header", "text_repeated_footer", "spine_central", "text_sentence_density")},
				{CategoryUnknown, 35, false, fixtureReasons("label_caption", "spine_central", "review_required")},
				{CategoryBackMatter, 95, false, fixtureReasons("label_bibliography", "spine_central", "text_bibliography_cluster", "text_reference_density")},
				{CategoryUnknown, 35, false, fixtureReasons("label_structural_fragment", "spine_central", "review_required")},
				{CategoryUnknown, 35, false, fixtureReasons("text_repeated_header", "spine_central", "review_required")},
				{CategoryUnknown, 35, false, fixtureReasons("text_repeated_header", "spine_central", "review_required")},
				{CategoryMainMatter, 70, false, fixtureReasons("label_chapter", "spine_back", "text_bibliography_cluster", "text_reference_density", "label_precedence")},
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

func TestIssue287CompleteGermanBookSelectionAndGroups(t *testing.T) {
	fixture := readClassifierFixture(t, "issue-287-complete-german-book.json")
	if len(fixture.Units.Units) != 21 {
		t.Fatalf("units=%d want=21", len(fixture.Units.Units))
	}
	classifications, err := ClassifyUnits(fixture.SnapshotID, fixture.Units)
	if err != nil {
		t.Fatal(err)
	}
	allCharacters, selectedCharacters, selectedUnits := 0, 0, 0
	for i, unit := range fixture.Units.Units {
		if unit.Order != uint64(i) || unit.SpineIndex != uint64(i) || classifications[i].SourceUnitSnapshot.UnitID != unit.ID {
			t.Fatalf("unit %d is out of deterministic snapshot order: unit=%+v classification=%+v", i, unit, classifications[i])
		}
		characters := len([]rune(unit.Text))
		allCharacters += characters
		if classifications[i].RecommendedInclusion {
			selectedUnits++
			selectedCharacters += characters
		}
	}
	if fixture.Units.Units[0].TitleSource != UnitTitleManifestID || fixture.Units.Units[0].Title != "titlepage" {
		t.Fatalf("fallback-title front matter was not preserved: %+v", fixture.Units.Units[0])
	}
	if selectedUnits != 9 || allCharacters != 1997 || selectedCharacters != 1333 || selectedCharacters <= allCharacters-selectedCharacters {
		t.Fatalf("safe totals: selected=%d selectedCharacters=%d allCharacters=%d", selectedUnits, selectedCharacters, allCharacters)
	}
	groups := BuildUnitGroups(fixture.Units.Units)
	var foundMain, foundApparatus bool
	for _, group := range groups {
		switch group.Label {
		case "Haupttext":
			foundMain = len(group.UnitIDs) == 9 && group.UnitIDs[0] == fixture.Units.Units[6].ID && group.UnitIDs[8] == fixture.Units.Units[14].ID
		case "Apparat":
			foundApparatus = len(group.UnitIDs) == 6 && group.UnitIDs[0] == fixture.Units.Units[15].ID && group.UnitIDs[5] == fixture.Units.Units[20].ID
		}
	}
	if !foundMain || !foundApparatus {
		t.Fatalf("expected ordered main/apparatus groups, got %+v", groups)
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
		"landmark_frontmatter":      "The EPUB landmark identifies front matter.",
		"landmark_titlepage":        "The EPUB landmark identifies front matter.",
		"landmark_bodymatter":       "The EPUB landmark identifies main matter.",
		"landmark_backmatter":       "The EPUB landmark identifies back matter.",
		"landmark_bibliography":     "The EPUB landmark identifies back matter.",
		"landmark_index":            "The EPUB landmark identifies back matter.",
		"landmark_endnotes":         "The EPUB landmark identifies back matter.",
		"landmark_appendix":         "The EPUB landmark identifies back matter.",
		"label_contents":            "A title or navigation label matches the contents marker.",
		"label_preface":             "A title or navigation label matches the preface marker.",
		"label_chapter":             "A title or navigation label matches the chapter marker.",
		"label_bibliography":        "A title or navigation label matches the bibliography marker.",
		"label_notes":               "A title or navigation label matches the notes marker.",
		"label_index":               "A title or navigation label matches the index marker.",
		"label_glossary":            "A title or navigation label matches the glossary marker.",
		"label_appendix":            "A title or navigation label matches the appendix marker.",
		"label_editorial":           "A title or navigation label matches the editorial marker.",
		"label_acknowledgments":     "A title or navigation label matches the acknowledgments marker.",
		"label_copyright":           "A title or navigation label matches the copyright marker.",
		"label_image_credits":       "A title or navigation label matches the image credits marker.",
		"label_references":          "A title or navigation label matches the references marker.",
		"label_caption":             "A title or navigation label matches the caption marker for non-prose structural content.",
		"label_structural_fragment": "A title or navigation label matches the structural fragment marker for non-prose structural content.",
		"heading_roman_numeral":     "The title begins with a Roman numeral followed by a chapter-like heading.",
		"heading_repeated_pattern":  "Multiple titles in this snapshot share the same numbered-heading pattern.",
		"path_contents":             "The package path matches the contents marker.",
		"path_preface":              "The package path matches the preface marker.",
		"path_chapter":              "The package path matches the chapter marker.",
		"path_bibliography":         "The package path matches the bibliography marker.",
		"path_notes":                "The package path matches the notes marker.",
		"path_index":                "The package path matches the index marker.",
		"path_glossary":             "The package path matches the glossary marker.",
		"path_appendix":             "The package path matches the appendix marker.",
		"path_editorial":            "The package path matches the editorial marker.",
		"path_acknowledgments":      "The package path matches the acknowledgments marker.",
		"path_copyright":            "The package path matches the copyright marker.",
		"path_image_credits":        "The package path matches the image credits marker.",
		"path_references":           "The package path matches the references marker.",
		"spine_front":               "The unit is at the beginning of the readable spine.",
		"spine_central":             "The unit is in the central readable spine range.",
		"spine_back":                "The unit is at the end of the readable spine.",
		"text_sentence_density":     "The text contains sustained sentence-like prose.",
		"text_reference_density":    "The text has a high density of citations, years, ISBN/URL, or page-reference markers.",
		"text_bibliography_cluster": "The text contains a cluster of bibliography-style entries.",
		"text_repeated_header":      "The same short header appears in multiple readable units.",
		"text_repeated_footer":      "The same short footer appears in multiple readable units.",
		"review_required":           "Structural-fragment evidence requires learner review instead of automatic promotion.",
		"navigation_only":           "The manifest identifies this unit as a navigation document.",
		"insufficient_evidence":     "The available signals do not establish a structural category.",
		"contradictory_evidence":    "Strong signals support conflicting structural categories.",
		"label_precedence":          "The explicit title or navigation label outranks conflicting path, reference-density, spine-position, and prose-shape evidence.",
		"landmark_precedence":       "The EPUB landmark outranks conflicting title, navigation, path, spine-position, and prose-shape evidence.",
	}
	reasons := make([]ClassificationReason, len(signals))
	for i, signal := range signals {
		reasons[i] = ClassificationReason{Signal: signal, Message: messages[signal]}
	}
	return reasons
}
