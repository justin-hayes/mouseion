package epub

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestClassifyUnitsFixtureMatrix(t *testing.T) {
	units := []ExtractedUnit{
		classifierUnit(0, "inhalt", "Inhaltsverzeichnis", "Text/front.xhtml", "Eine Übersicht."),
		classifierUnit(1, "capitolo", "Capitolo 1", "Text/one.xhtml", "Questa è una frase abbastanza lunga. Questa è una seconda frase del racconto. Questa è una terza frase con altre parole."),
		classifierUnit(2, "bibliography", "Bibliography", "Text/back/bibliography.xhtml", "Smith (2020), pp. 12. https://example.org ISBN 978-1-4028-9462-6. Jones (2019), p. 8."),
		classifierUnit(3, "nav", "Contents", "Navigation/nav.xhtml", "Chapter one"),
		classifierUnit(4, "notes", "Anmerkungen", "Text/notes.xhtml", "1. Hinweis. 2. Hinweis."),
		classifierUnit(5, "appendice", "Appendice A", "Text/appendice.xhtml", "Materiale supplementare."),
		classifierUnit(6, "ambiguous", "  ??? \x00 ", "Text/x-001.xhtml", "Fragment ohne klare Struktur"),
		classifierUnit(7, "conflict", "Chapter 9", "Text/chapter.xhtml", "Short fragment."),
	}
	units[2].LandmarkTypes = []string{"bibliography"}
	units[3].Properties = []string{"nav"}
	units[7].LandmarkTypes = []string{"bibliography"}

	got, err := ClassifyUnits("sha256:fixture-matrix", ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: units})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		category   UnitCategory
		confidence uint8
		include    bool
		reasons    []ClassificationReason
	}{
		{CategoryFrontMatter, 70, false, []ClassificationReason{{Signal: "label_contents", Message: "A title or navigation label matches the contents marker."}, {Signal: "spine_front", Message: "The unit is at the beginning of the readable spine."}}},
		{CategoryMainMatter, 95, true, []ClassificationReason{{Signal: "label_chapter", Message: "A title or navigation label matches the chapter marker."}, {Signal: "spine_central", Message: "The unit is in the central readable spine range."}, {Signal: "text_sentence_density", Message: "The text contains sustained sentence-like prose."}}},
		{CategoryBackMatter, 95, false, []ClassificationReason{{Signal: "landmark_bibliography", Message: "The EPUB landmark identifies back matter."}, {Signal: "label_bibliography", Message: "A title or navigation label matches the bibliography marker."}, {Signal: "path_bibliography", Message: "The package path matches the bibliography marker."}, {Signal: "spine_central", Message: "The unit is in the central readable spine range."}, {Signal: "text_reference_density", Message: "The text has a high density of citations, years, ISBN/URL, or page-reference markers."}}},
		{CategoryUnknown, 20, false, []ClassificationReason{{Signal: "navigation_only", Message: "The manifest identifies this unit as a navigation document."}}},
		{CategoryBackMatter, 85, false, []ClassificationReason{{Signal: "label_notes", Message: "A title or navigation label matches the notes marker."}, {Signal: "path_notes", Message: "The package path matches the notes marker."}, {Signal: "spine_central", Message: "The unit is in the central readable spine range."}}},
		{CategoryBackMatter, 85, false, []ClassificationReason{{Signal: "label_appendix", Message: "A title or navigation label matches the appendix marker."}, {Signal: "path_appendix", Message: "The package path matches the appendix marker."}, {Signal: "spine_central", Message: "The unit is in the central readable spine range."}}},
		{CategoryUnknown, 20, false, []ClassificationReason{{Signal: "spine_central", Message: "The unit is in the central readable spine range."}, {Signal: "insufficient_evidence", Message: "The available signals do not establish a structural category."}}},
		{CategoryUnknown, 35, false, []ClassificationReason{{Signal: "landmark_bibliography", Message: "The EPUB landmark identifies back matter."}, {Signal: "label_chapter", Message: "A title or navigation label matches the chapter marker."}, {Signal: "path_chapter", Message: "The package path matches the chapter marker."}, {Signal: "spine_back", Message: "The unit is at the end of the readable spine."}, {Signal: "contradictory_evidence", Message: "Strong signals support conflicting structural categories."}}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d classifications, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Category != want[i].category || got[i].Confidence != want[i].confidence || got[i].RecommendedInclusion != want[i].include || !reflect.DeepEqual(got[i].Reasons, want[i].reasons) {
			t.Errorf("unit %d mismatch:\n got: %#v\nwant: %#v", i, got[i], want[i])
		}
		if got[i].Classifier != (ClassifierIdentity{Name: ClassifierName, Version: ClassifierVersion}) || got[i].SourceUnitSnapshot.SnapshotID != "sha256:fixture-matrix" || got[i].SourceUnitSnapshot.UnitID != units[i].ID {
			t.Errorf("unit %d did not preserve classifier/snapshot identity: %#v", i, got[i])
		}
	}

	first, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := ClassifyUnits("sha256:fixture-matrix", ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: units})
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(repeated)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("classification is not byte-for-byte repeatable:\n%s\n%s", first, second)
	}
}

func TestClassifyUnitsExplicitExclusionAndWholeBookFallback(t *testing.T) {
	nonLinear := classifierUnit(0, "hidden", "Chapter", "Text/hidden.xhtml", "Long prose. Another long sentence follows here. A third long sentence follows here.")
	nonLinear.Linear = false
	ambiguous := classifierUnit(1, "fragment", "Fragment", "Text/fragment.xhtml", "A fragment")
	got, err := ClassifyUnits("snapshot:fallback", ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{nonLinear, ambiguous}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].RecommendedInclusion || !reflect.DeepEqual(got[0].Reasons, []ClassificationReason{{Signal: "linear_no", Message: "The spine marks this unit as non-linear content."}}) {
		t.Fatalf("linear=no must remain explicitly excluded: %#v", got[0])
	}
	wantReasons := []ClassificationReason{{Signal: "insufficient_evidence", Message: "The available signals do not establish a structural category."}, {Signal: "whole_book_fallback", Message: "Included because no high-confidence main matter was found in the source snapshot."}}
	if !got[1].RecommendedInclusion || got[1].Category != CategoryUnknown || !reflect.DeepEqual(got[1].Reasons, wantReasons) {
		t.Fatalf("fallback mismatch: %#v", got[1])
	}
}

func TestClassifyUnitsDoesNotTrustMainTitleAndPathAlone(t *testing.T) {
	unit := classifierUnit(0, "chapter12", "Chapter 12", "Text/chapter12.xhtml", "Fragment")
	got, err := ClassifyUnits("snapshot:title-path", ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{unit}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Category != CategoryMainMatter || got[0].Confidence >= ConfidenceHighMinimum {
		t.Fatalf("title and filename alone produced high-confidence main matter: %#v", got[0])
	}
}

func TestClassifyUnitsRejectsInvalidEnvelope(t *testing.T) {
	unit := classifierUnit(0, "one", "Chapter 1", "one.xhtml", "Text.")
	for _, test := range []struct {
		name     string
		snapshot string
		units    ExtractedUnits
	}{
		{"missing snapshot", "", ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{unit}}},
		{"wrong schema", "snapshot:x", ExtractedUnits{SchemaVersion: 99, Units: []ExtractedUnit{unit}}},
		{"no units", "snapshot:x", ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ClassifyUnits(test.snapshot, test.units); err == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
}

func classifierUnit(spine uint64, id, title, href, text string) ExtractedUnit {
	return ExtractedUnit{ID: UnitID(spine, id), Order: spine, SpineIndex: spine, ManifestID: id, Title: title, TitleSource: UnitTitleHeading, Text: text, SourceHref: href, ResolvedHref: "OPS/" + href, PackagePath: "OPS/package.opf", MediaType: "application/xhtml+xml", Linear: true, Properties: []string{}, NavigationLabels: []string{}, LandmarkTypes: []string{}}
}
