package fixtures

import (
	"slices"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFixtureConstructionOccurrencesSatisfyTheStoredShape holds every fixture
// occurrence to what migration 000063's construction_extent_valid enforces: a
// non-empty, sorted extent with no negative ordinal that contains its anchor.
func TestFixtureConstructionOccurrencesSatisfyTheStoredShape(t *testing.T) {
	studies := map[string]domain.SentenceStudy{
		"syntax": fixtureSyntaxStudy("fixture-kwic-syntax", 0, 1, "ὅτι"),
		"long":   fixtureSyntaxLongStudy("fixture-kwic-syntax", 0, 5, "x"),
	}
	for name, study := range studies {
		require.NotEmpty(t, study.Constructions, name)
		for i, occurrence := range study.Constructions {
			t.Run(name+"/"+occurrence.ConstructionID, func(t *testing.T) {
				require.NotEmpty(t, occurrence.Extent, "extent %d", i)
				assert.True(t, slices.IsSorted(occurrence.Extent), "sorted")
				assert.GreaterOrEqual(t, occurrence.Extent[0], 0, "no negative ordinal")
				assert.Contains(t, occurrence.Extent, occurrence.Anchor, "the extent contains its anchor")
			})
		}
	}
}
