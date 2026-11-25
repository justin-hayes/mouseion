package agdt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justin-hayes/mouseion/internal/syntax"
)

func TestReadLatinRelationStripsOLASuffixes(t *testing.T) {
	cases := []struct {
		relation string
		base     string
		conjunct bool
	}{
		{"SBJ", "SBJ", false},
		{"SBJ_CO", "SBJ", true},
		{"ATR_AP", "ATR", false},
		{"ExD", "ExD", false},
		{"ADV_ExD", "ADV", false},
		{"PRED_CO_AP", "PRED", true},
		{"OBJ_AP_CO", "OBJ", true},
		{"ExD_CO", "ExD", true},
		{"AuxP_CO", "AuxP", true},
		{"_", "_", false},
		{"", "", false},
		{"_CO", "_CO", false},
		{"SBJ_XX", "SBJ_XX", false},
		{"SBJ_CO_XX", "SBJ_CO_XX", false},
	}
	for _, c := range cases {
		t.Run(c.relation, func(t *testing.T) {
			base, conjunct := ReadLatinRelation(c.relation)
			assert.Equal(t, c.base, base)
			assert.Equal(t, c.conjunct, conjunct)
		})
	}
}

func TestAdaptLatinReadsBaseCategoryAndConjunctFlag(t *testing.T) {
	tree := AdaptLatin([]Token{
		{Ordinal: 0, Surface: "incolunt", Relation: "PRED", Head: 0},
		{Ordinal: 1, Surface: "Belgae", Relation: "SBJ_CO", Head: 0},
		{Ordinal: 2, Surface: "et", Relation: "COORD", Head: 0},
	}, nil)
	require.Len(t, tree.Nodes, 3)
	assert.Equal(t, syntax.CategorySubject, tree.Nodes[1].Category)
	assert.True(t, tree.Nodes[1].Conjunct)
	assert.Equal(t, "SBJ_CO", tree.Nodes[1].Relation, "the stored relation stays verbatim")
	assert.False(t, tree.Nodes[0].Conjunct)

	greek := Adapt([]Token{{Ordinal: 0, Surface: "x", Relation: "SBJ_CO", Head: 0}}, nil)
	assert.Equal(t, syntax.CategoryOther, greek.Nodes[0].Category, "the Greek reader is unchanged")
}
