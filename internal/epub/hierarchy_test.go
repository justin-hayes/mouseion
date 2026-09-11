package epub

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.GreaterOrEqual(t, len(first), 4)
	assert.Len(t, second, len(first))
	for i := range first {
		assert.Equal(t, second[i].ID, first[i].ID, "non-deterministic groups")
		assert.Equal(t, second[i].First, first[i].First, "non-deterministic groups")
	}
	flat := []ExtractedUnit{{ID: UnitID(0, "a"), Order: 0, ResolvedHref: "a.xhtml"}, {ID: UnitID(1, "b"), Order: 1, ResolvedHref: "b.xhtml"}}
	assert.Empty(t, BuildUnitGroups(flat), "flat fallback groups")
}

func TestBuildUnitGroupsRejectsContradictoryNavigation(t *testing.T) {
	units := []ExtractedUnit{{ID: UnitID(0, "a"), Order: 0, NavigationLabels: []string{"Part"}}, {ID: UnitID(1, "b"), Order: 1}, {ID: UnitID(2, "c"), Order: 2, NavigationLabels: []string{"Part"}}}
	assert.Empty(t, BuildUnitGroups(units), "contradictory groups")
}

func TestBuildUnitGroupsFixtures(t *testing.T) {
	for _, test := range []struct {
		name      string
		wantGroup bool
		wantNest  bool
	}{{"german", true, true}, {"italian", true, true}, {"nested", true, true}, {"flat", false, false}} {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile("testfixtures/hierarchy/" + test.name + ".json")
			require.NoError(t, err)
			var units ExtractedUnits
			require.NoError(t, json.Unmarshal(data, &units))
			groups := BuildUnitGroups(units.Units)
			assert.Equal(t, BuildUnitGroups(units.Units), groups, "group expansion is not deterministic")
			assert.Equal(t, test.wantGroup, len(groups) > 0)
			if test.wantNest {
				nested := false
				for _, group := range groups {
					nested = nested || group.ParentID != ""
				}
				assert.True(t, nested, "no parent relationship in %#v", groups)
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
				assert.Equal(t, want, group.UnitIDs, "group %q members are not the intended spine-ordered units", group.ID)
			}
		})
	}
}
