package epub

import (
	"encoding/json"
	"strings"
	"testing"
)

func validClassification() UnitClassification {
	return UnitClassification{
		SchemaVersion: ClassificationSchemaVersion,
		Classifier:    ClassifierIdentity{Name: "mouseion-epub-structure", Version: "1.0.0"},
		SourceUnitSnapshot: SourceUnitSnapshotIdentity{
			SnapshotID:                  "sha256:rendition-snapshot",
			ExtractedUnitsSchemaVersion: ExtractedUnitsSchemaVersion,
			UnitID:                      UnitID(3, "chapter-1"),
		},
		Category:             CategoryMainMatter,
		Confidence:           90,
		Reasons:              []ClassificationReason{{Signal: "landmark_bodymatter", Message: "The EPUB landmark identifies body matter."}, {Signal: "spine_position", Message: "The unit is in the central spine range."}},
		RecommendedInclusion: true,
	}
}

func TestClassificationContractCategoriesAndConfidence(t *testing.T) {
	for _, category := range []UnitCategory{CategoryFrontMatter, CategoryMainMatter, CategoryBackMatter, CategoryUnknown} {
		classification := validClassification()
		classification.Category = category
		if category == CategoryUnknown {
			classification.Confidence = ConfidenceLowMaximum
		} else if category != CategoryMainMatter {
			classification.RecommendedInclusion = false
		}
		if err := classification.Validate(); err != nil {
			t.Fatalf("category %q: %v", category, err)
		}
	}

	tests := []struct {
		name   string
		mutate func(*UnitClassification)
	}{
		{"invalid category", func(c *UnitClassification) { c.Category = "appendix_maybe" }},
		{"confidence above bound", func(c *UnitClassification) { c.Confidence = ConfidenceMaximum + 1 }},
		{"unknown high confidence", func(c *UnitClassification) { c.Category, c.Confidence = CategoryUnknown, ConfidenceLowMaximum+1 }},
		{"excluded high-confidence main matter", func(c *UnitClassification) { c.RecommendedInclusion = false }},
		{"included high-confidence front matter", func(c *UnitClassification) { c.Category = CategoryFrontMatter }},
		{"included high-confidence back matter", func(c *UnitClassification) { c.Category = CategoryBackMatter }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classification := validClassification()
			tt.mutate(&classification)
			if err := classification.Validate(); err == nil {
				t.Fatal("invalid classification accepted")
			}
		})
	}
}

func TestClassificationContractRequiresCompleteIdentity(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*UnitClassification)
	}{
		{"classifier name", func(c *UnitClassification) { c.Classifier.Name = "" }},
		{"classifier version", func(c *UnitClassification) { c.Classifier.Version = "" }},
		{"snapshot", func(c *UnitClassification) { c.SourceUnitSnapshot.SnapshotID = "" }},
		{"unstable snapshot", func(c *UnitClassification) { c.SourceUnitSnapshot.SnapshotID = "snapshot\nchanged" }},
		{"unit", func(c *UnitClassification) { c.SourceUnitSnapshot.UnitID = "" }},
		{"unit schema", func(c *UnitClassification) { c.SourceUnitSnapshot.ExtractedUnitsSchemaVersion++ }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			classification := validClassification()
			tt.mutate(&classification)
			if err := classification.Validate(); err == nil {
				t.Fatal("missing or invalid identity accepted")
			}
		})
	}
}

func TestClassificationReasonsAreOrderedUniqueAndStable(t *testing.T) {
	classification := validClassification()
	first, err := json.Marshal(classification)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(classification)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("serialization changed:\n%s\n%s", first, second)
	}
	if strings.Index(string(first), "landmark_bodymatter") > strings.Index(string(first), "spine_position") {
		t.Fatalf("reason precedence was not preserved: %s", first)
	}

	for _, reasons := range [][]ClassificationReason{
		nil,
		{},
		{{Signal: "duplicate", Message: "First."}, {Signal: "duplicate", Message: "Second."}},
		{{Signal: " leading-space", Message: "Unstable signal."}},
		{{Signal: "stable", Message: " trailing space "}},
		{{Signal: "stable", Message: "line\nbreak"}},
	} {
		classification := validClassification()
		classification.Reasons = reasons
		if _, err := json.Marshal(classification); err == nil {
			t.Fatalf("invalid reasons accepted: %#v", reasons)
		}
	}
}

func TestUnknownRecommendationIsExplicitNotInferred(t *testing.T) {
	for _, include := range []bool{false, true} {
		classification := validClassification()
		classification.Category = CategoryUnknown
		classification.Confidence = 20
		classification.RecommendedInclusion = include
		classification.Reasons = []ClassificationReason{{Signal: "insufficient_evidence", Message: "The available signals do not establish a structural category."}}
		if err := classification.Validate(); err != nil {
			t.Fatalf("unknown include=%t: %v", include, err)
		}
	}
}
