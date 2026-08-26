package epub

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func validReviewedScope() (ReviewedScopeSnapshot, ScopeSourceSnapshot) {
	units := ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{
		{ID: UnitID(0, "front"), Order: 0, SpineIndex: 0, ManifestID: "front", Text: "Preface", EndOffset: 7},
		{ID: UnitID(1, "chapter"), Order: 1, SpineIndex: 1, ManifestID: "chapter", Text: "Readable chapter", StartOffset: 9, EndOffset: 25},
		{ID: UnitID(2, "notes"), Order: 2, SpineIndex: 2, ManifestID: "notes", Text: "Notes", StartOffset: 27, EndOffset: 32},
	}}
	scope := ReviewedScopeSnapshot{
		SchemaVersion:      ReviewedScopeSchemaVersion,
		ScopeID:            "scope:review-1",
		OwnerID:            "owner-1",
		SourceMaterialID:   "source-1",
		SourceContent:      domain.EPUBContentRevisionIdentity{RevisionID: "revision:1", Digest: domain.EPUBContentDigest([]byte("epub bytes")), DigestVersion: 1},
		SourceUnitSnapshot: UnitSnapshotIdentity{SnapshotID: "snapshot:rendition-1", ExtractedUnitsSchemaVersion: ExtractedUnitsSchemaVersion},
		Classifier:         ClassifierIdentity{Name: "mouseion-epub-structure", Version: "1.0.0"},
		SelectionMode:      ScopeSelectionRecommended,
		SelectedUnits:      []SelectedUnitReference{{UnitID: units.Units[1].ID, Order: 1}, {UnitID: units.Units[2].ID, Order: 2}},
	}
	source := ScopeSourceSnapshot{OwnerID: scope.OwnerID, SourceMaterialID: scope.SourceMaterialID, SourceContent: scope.SourceContent, SnapshotID: scope.SourceUnitSnapshot.SnapshotID, ExtractedUnits: units}
	return scope, source
}

func TestReviewedScopeValidatesExactOwnerSourceAndSnapshot(t *testing.T) {
	scope, source := validReviewedScope()
	if err := scope.ValidateAgainst(source); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ScopeSourceSnapshot){
		func(v *ScopeSourceSnapshot) { v.OwnerID = "other-owner" },
		func(v *ScopeSourceSnapshot) { v.SourceMaterialID = "other-source" },
		func(v *ScopeSourceSnapshot) { v.SourceContent.RevisionID = "revision:other" },
		func(v *ScopeSourceSnapshot) { v.SourceContent.Digest = domain.EPUBContentDigest([]byte("other bytes")) },
		func(v *ScopeSourceSnapshot) { v.SnapshotID = "snapshot:new-rendition" },
		func(v *ScopeSourceSnapshot) { v.ExtractedUnits.SchemaVersion++ },
	} {
		changed := source
		mutate(&changed)
		if err := scope.ValidateAgainst(changed); err == nil {
			t.Fatal("mismatched source snapshot accepted")
		}
	}
}

func TestReviewedScopeRejectsEmptyUnreadableForeignDuplicateAndUnstableSelection(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ReviewedScopeSnapshot, *ScopeSourceSnapshot)
	}{
		{"empty", func(s *ReviewedScopeSnapshot, _ *ScopeSourceSnapshot) { s.SelectedUnits = nil }},
		{"unreadable", func(_ *ReviewedScopeSnapshot, source *ScopeSourceSnapshot) {
			source.ExtractedUnits.Units[1].Text = " \n"
		}},
		{"foreign unit", func(s *ReviewedScopeSnapshot, _ *ScopeSourceSnapshot) {
			s.SelectedUnits[0].UnitID = UnitID(99, "foreign")
		}},
		{"duplicate", func(s *ReviewedScopeSnapshot, _ *ScopeSourceSnapshot) { s.SelectedUnits[1] = s.SelectedUnits[0] }},
		{"forged order", func(s *ReviewedScopeSnapshot, _ *ScopeSourceSnapshot) { s.SelectedUnits[0].Order = 0 }},
		{"reversed order", func(s *ReviewedScopeSnapshot, _ *ScopeSourceSnapshot) {
			s.SelectedUnits[0], s.SelectedUnits[1] = s.SelectedUnits[1], s.SelectedUnits[0]
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, source := validReviewedScope()
			tt.mutate(&scope, &source)
			if err := scope.ValidateAgainst(source); err == nil {
				t.Fatal("invalid reviewed scope accepted")
			}
		})
	}
}

func TestReviewedScopeModesVersionsAndLegacyUnavailable(t *testing.T) {
	for _, mode := range []ScopeSelectionMode{ScopeSelectionRecommended, ScopeSelectionOverridden} {
		scope, source := validReviewedScope()
		scope.SelectionMode = mode
		if err := scope.ValidateAgainst(source); err != nil {
			t.Fatalf("mode %q: %v", mode, err)
		}
	}

	scope, source := validReviewedScope()
	scope.SelectionMode = "automatic"
	if err := scope.ValidateAgainst(source); err == nil {
		t.Fatal("unsupported selection mode accepted")
	}
	scope, source = validReviewedScope()
	scope.SchemaVersion++
	if err := scope.ValidateAgainst(source); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported scope version error = %v", err)
	}
	scope, _ = validReviewedScope()
	if err := scope.ValidateAgainst(ScopeSourceSnapshot{}); !errors.Is(err, ErrReviewedScopeUnavailable) {
		t.Fatalf("legacy source error = %v", err)
	}
}

func TestReviewedScopeSerializationIsDeterministicAndContainsNoText(t *testing.T) {
	scope, source := validReviewedScope()
	if err := scope.ValidateAgainst(source); err != nil {
		t.Fatal(err)
	}
	first, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("serialization changed:\n%s\n%s", first, second)
	}
	serialized := string(first)
	if strings.Contains(serialized, "Readable chapter") || strings.Contains(serialized, `"text"`) {
		t.Fatalf("browser-authoritative text leaked into scope: %s", serialized)
	}
	if strings.Index(serialized, scope.SelectedUnits[0].UnitID) > strings.Index(serialized, scope.SelectedUnits[1].UnitID) {
		t.Fatalf("selected source order was not preserved: %s", serialized)
	}
}

func TestReviewedScopeSnapshotModelsImmutableHistory(t *testing.T) {
	first, source := validReviewedScope()
	second := first
	second.ScopeID = "scope:review-2"
	second.SelectionMode = ScopeSelectionOverridden
	second.SelectedUnits = []SelectedUnitReference{{UnitID: source.ExtractedUnits.Units[1].ID, Order: 1}}
	if err := first.ValidateAgainst(source); err != nil {
		t.Fatal(err)
	}
	if err := second.ValidateAgainst(source); err != nil {
		t.Fatal(err)
	}
	if first.ScopeID == second.ScopeID || len(first.SelectedUnits) == len(second.SelectedUnits) {
		t.Fatal("a new review did not remain a distinct immutable snapshot")
	}
}

func TestReviewedScopeConfirmationKeyExcludesRequestScopeID(t *testing.T) {
	first, source := validReviewedScope()
	key, err := first.ConfirmationKey()
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.ScopeID = "scope:retry-request"
	secondKey, err := second.ConfirmationKey()
	if err != nil {
		t.Fatal(err)
	}
	if key != secondKey {
		t.Fatalf("equivalent confirmations differed: %q != %q", key, secondKey)
	}
	second.SelectedUnits = []SelectedUnitReference{{UnitID: source.ExtractedUnits.Units[1].ID, Order: 1}}
	changedKey, err := second.ConfirmationKey()
	if err != nil {
		t.Fatal(err)
	}
	if key == changedKey {
		t.Fatal("changed selection reused confirmation identity")
	}
}
