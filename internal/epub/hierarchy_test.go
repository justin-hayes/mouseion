package epub

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestBuildUnitGroupsNestedNavigationAndFlatFallback(t *testing.T) {
	units := []ExtractedUnit{
		{ID: UnitID(0, "eins"), Order: 0, PackagePath: "OPS/package.opf", ResolvedHref: "OPS/Text/Teil-Eins/eins.xhtml", NavigationLabels: []string{"Teil Eins"}},
		{ID: UnitID(1, "zwei"), Order: 1, PackagePath: "OPS/package.opf", ResolvedHref: "OPS/Text/Teil-Eins/Abschnitt/zwei.xhtml", NavigationLabels: []string{"Teil Eins"}},
		{ID: UnitID(2, "tre"), Order: 2, PackagePath: "OPS/package.opf", ResolvedHref: "OPS/Text/Parte-Due/tre.xhtml", NavigationLabels: []string{"Parte Due"}},
		{ID: UnitID(3, "quattro"), Order: 3, PackagePath: "OPS/package.opf", ResolvedHref: "OPS/Text/Parte-Due/quattro.xhtml", NavigationLabels: []string{"Parte Due"}},
	}
	first := BuildUnitGroups(units)
	second := BuildUnitGroups(units)
	if len(first) < 4 || len(first) != len(second) {
		t.Fatalf("groups = %#v", first)
	}
	for i := range first {
		if first[i].ID != second[i].ID || first[i].First != second[i].First {
			t.Fatalf("non-deterministic groups: %#v %#v", first, second)
		}
	}
	flat := []ExtractedUnit{{ID: UnitID(0, "a"), Order: 0, ResolvedHref: "a.xhtml"}, {ID: UnitID(1, "b"), Order: 1, ResolvedHref: "b.xhtml"}}
	if got := BuildUnitGroups(flat); len(got) != 0 {
		t.Fatalf("flat fallback groups = %#v", got)
	}
}

func TestBuildUnitGroupsRejectsContradictoryNavigation(t *testing.T) {
	units := []ExtractedUnit{{ID: UnitID(0, "a"), Order: 0, NavigationLabels: []string{"Part"}}, {ID: UnitID(1, "b"), Order: 1}, {ID: UnitID(2, "c"), Order: 2, NavigationLabels: []string{"Part"}}}
	if got := BuildUnitGroups(units); len(got) != 0 {
		t.Fatalf("contradictory groups = %#v", got)
	}
}

func TestBuildUnitGroupsFixtures(t *testing.T) {
	for _, test := range []struct {
		name      string
		wantGroup bool
		wantNest  bool
	}{{"german", true, true}, {"italian", true, true}, {"nested", true, true}, {"flat", false, false}} {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile("testfixtures/hierarchy/" + test.name + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var units ExtractedUnits
			if err = json.Unmarshal(data, &units); err != nil {
				t.Fatal(err)
			}
			groups := BuildUnitGroups(units.Units)
			if repeated := BuildUnitGroups(units.Units); !reflect.DeepEqual(groups, repeated) {
				t.Fatalf("group expansion is not deterministic:\nfirst=%#v\nsecond=%#v", groups, repeated)
			}
			if (len(groups) > 0) != test.wantGroup {
				t.Fatalf("groups = %#v", groups)
			}
			if test.wantNest {
				nested := false
				for _, group := range groups {
					nested = nested || group.ParentID != ""
				}
				if !nested {
					t.Fatalf("no parent relationship in %#v", groups)
				}
			}
			for _, group := range groups {
				var want []string
				for _, unit := range units.Units {
					for _, id := range group.UnitIDs {
						if unit.ID == id {
							want = append(want, unit.ID)
						}
					}
				}
				if !reflect.DeepEqual(group.UnitIDs, want) {
					t.Fatalf("group %q members are not the intended spine-ordered units: got=%v want=%v", group.ID, group.UnitIDs, want)
				}
			}
		})
	}
}
