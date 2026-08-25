package epub

import (
	"encoding/json"
	"fmt"
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
		{CategoryBackMatter, 95, false, []ClassificationReason{{Signal: "landmark_bibliography", Message: "The EPUB landmark identifies back matter."}, {Signal: "label_chapter", Message: "A title or navigation label matches the chapter marker."}, {Signal: "path_chapter", Message: "The package path matches the chapter marker."}, {Signal: "spine_back", Message: "The unit is at the end of the readable spine."}, {Signal: "landmark_precedence", Message: "The EPUB landmark outranks conflicting title, navigation, path, spine-position, and prose-shape evidence."}}},
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

func TestClassifyUnitsExplicitExclusionAndUnknownReviewPolicy(t *testing.T) {
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
	wantReasons := []ClassificationReason{{Signal: "insufficient_evidence", Message: "The available signals do not establish a structural category."}}
	if got[1].RecommendedInclusion || got[1].Category != CategoryUnknown || !reflect.DeepEqual(got[1].Reasons, wantReasons) {
		t.Fatalf("unknown review policy mismatch: %#v", got[1])
	}
}

func TestClassifyUnitsGermanBookWithoutHighConfidenceMainMatterUsesSafePolicy(t *testing.T) {
	titles := []string{
		"VORWORT", "INHALT",
		"I. Einleitung", "II. Die Welt der Paläste", "III. Herrschaft", "IV. Alltag", "V. Handel",
		"VI. Religion", "VII. Kunst", "VIII. Wandel", "IX. Ausblick",
		"ANHANG", "Anmerkungen", "Bildnachweis", "Hinweise zu Quellen und Literatur", "Register", "copyright",
	}
	units := make([]ExtractedUnit, len(titles))
	for i, title := range titles {
		text := "Kurzer struktureller Inhalt"
		if i >= 2 && i <= 10 {
			text = "Dies ist ein ausführlicher Satz des Kapitels. Ein zweiter Satz entwickelt den Gedanken mit genügend Wörtern. Ein dritter Satz schließt den Abschnitt verständlich ab."
		}
		if title == "Hinweise zu Quellen und Literatur" {
			text = "Müller (2020), S. 12. Schmidt (2019), S. 44. Weber (2018), S. 81."
		}
		units[i] = classifierUnit(uint64(i), fmt.Sprintf("unit-%d", i), title, fmt.Sprintf("Text/unit-%d.xhtml", i), text)
	}

	got, err := ClassifyUnits("snapshot:german-review", ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: units})
	if err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= 10; i++ {
		if got[i].Category != CategoryMainMatter || got[i].Confidence != 95 || !got[i].RecommendedInclusion || !hasReason(got[i].Reasons, "heading_repeated_pattern") {
			t.Errorf("substantive chapter %q was not included with medium-confidence main matter: %#v", titles[i], got[i])
		}
	}
	for _, i := range []int{0, 1, 11, 12, 13, 14, 15, 16} {
		if got[i].RecommendedInclusion {
			t.Errorf("reference/front/back unit %q was automatically included: %#v", titles[i], got[i])
		}
	}
	for _, classification := range got {
		for _, reason := range classification.Reasons {
			if reason.Signal == "whole_book_fallback" {
				t.Fatalf("book-level policy state leaked into unit classification evidence: %#v", classification)
			}
		}
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

func TestClassifyUnitsLandmarkOutranksConflictingLabelAndText(t *testing.T) {
	unit := classifierUnit(0, "chapter", "Chapter 9", "Text/chapter.xhtml", "Long prose appears here with enough words. Another complete sentence follows with more details. A third complete sentence finishes the passage.")
	unit.LandmarkTypes = []string{"bibliography"}
	got, err := ClassifyUnits("snapshot:landmark-precedence", ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{unit}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Category != CategoryBackMatter || got[0].Confidence < ConfidenceHighMinimum || got[0].RecommendedInclusion || !hasReason(got[0].Reasons, "landmark_precedence") {
		t.Fatalf("landmark did not outrank conflicting lower-tier evidence: %#v", got[0])
	}
}

func TestStructuralMarkerMatchingUsesNormalizedTokenBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		href     string
		category UnitCategory
		signal   string
	}{
		{"German image credit heading", "ABBILDUNGSNACHWEIS:", "Text/unit.xhtml", CategoryBackMatter, "label_image_credits"},
		{"German image credit path", "Unbenannt", "Text/Bildquellen-2.xhtml", CategoryBackMatter, "path_image_credits"},
		{"German sources navigation style", "Quellen & Literatur", "Text/unit.xhtml", CategoryBackMatter, "label_references"},
		{"German imprint", "Imprint", "Text/unit.xhtml", CategoryFrontMatter, "label_copyright"},
		{"Italian marker remains", "Bibliografia", "Text/unit.xhtml", CategoryBackMatter, "label_bibliography"},
		{"language neutral marker remains", "Appendix", "Text/unit.xhtml", CategoryBackMatter, "label_appendix"},
		{"no heading substring match", "Bildnachweiser gesucht", "Text/unit.xhtml", CategoryUnknown, ""},
		{"no path substring match", "Unbenannt", "Text/meinebildquellensammlung.xhtml", CategoryUnknown, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			unit := classifierUnit(0, "marker", test.title, test.href, "Kurzer Inhalt")
			got, err := ClassifyUnits("snapshot:marker", ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{unit}})
			if err != nil {
				t.Fatal(err)
			}
			if got[0].Category != test.category || (test.signal != "" && !hasReason(got[0].Reasons, test.signal)) {
				t.Fatalf("classification=%+v want category=%s signal=%q", got[0], test.category, test.signal)
			}
		})
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
