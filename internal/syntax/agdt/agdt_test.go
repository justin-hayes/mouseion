package agdt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justin-hayes/mouseion/internal/syntax"
)

func TestCategoryMapsEveryAGDTRelation(t *testing.T) {
	cases := map[string]syntax.Category{
		"PRED": syntax.CategoryPredicate, "SBJ": syntax.CategorySubject, "OBJ": syntax.CategoryObject,
		"ATR": syntax.CategoryAttribute, "ADV": syntax.CategoryAdverbial, "PNOM": syntax.CategoryPredicateNominal,
		"OCOMP": syntax.CategoryObjectComplement, "ATV": syntax.CategoryPredicativeComplement,
		"AtvV": syntax.CategoryPredicativeComplement, "APOS": syntax.CategoryApposition,
		"COORD": syntax.CategoryCoordination, "AuxC": syntax.CategorySubordinatingConjunction,
		"AuxP": syntax.CategoryPreposition, "AuxY": syntax.CategorySentenceParticle,
		"AuxZ": syntax.CategoryEmphasisingParticle, "AuxV": syntax.CategoryAuxiliary,
		"ExD": syntax.CategoryOutsideStructure, "AuxX": syntax.CategoryPunctuation,
		"AuxK": syntax.CategoryPunctuation, "AuxG": syntax.CategoryPunctuation,
		"_": syntax.CategoryOther, "": syntax.CategoryOther, "OBJ_CO": syntax.CategoryOther, "NOPE": syntax.CategoryOther,
	}
	for relation, want := range cases {
		t.Run(relation, func(t *testing.T) {
			assert.Equal(t, want, Category(relation))
		})
	}
}

func TestWordingNamesEveryNonPunctuationCategory(t *testing.T) {
	for category := syntax.CategoryOther; category <= syntax.CategoryPunctuation; category++ {
		if category == syntax.CategoryPunctuation {
			continue
		}
		t.Run(RelationName(category), func(t *testing.T) {
			assert.NotEmpty(t, RelationName(category))
			assert.NotEmpty(t, RolePhrase(category))
		})
	}
	t.Run("punctuation falls back to the unlabelled wording", func(t *testing.T) {
		assert.Equal(t, "relation not labelled", RelationName(syntax.CategoryPunctuation))
		assert.Equal(t, "attached to", RolePhrase(syntax.CategoryPunctuation))
	})
}

// coordinated is: 0 λέγει (PRED, root) <- 1 καί (COORD) <- 2 ἄνδρας (OBJ_CO is
// not mapped, so the children use plain OBJ/SBJ) and 3 παῖς (SBJ); 4 ὅτι
// (AuxC) heads an unprinted node e_1 (ExD) whose child is 5 ἐστί (PRED).
func coordinated() syntax.Tree {
	return Adapt([]Token{
		{Ordinal: 0, Surface: "λέγει", Relation: "PRED", Head: 0},
		{Ordinal: 1, Surface: "καί", Relation: "COORD", Head: 0},
		{Ordinal: 2, Surface: "ἄνδρας", Relation: "OBJ", Head: 1},
		{Ordinal: 3, Surface: "παῖς", Relation: "OBJ", Head: 1},
		{Ordinal: 4, Surface: "ὅτι", Relation: "AuxC", Head: 0},
		{Ordinal: 5, Surface: "ἐστί", Relation: "PRED", Head: 5, HeadUnprintedNode: "e_1"},
	}, []UnprintedNode{{SourceID: "e_1", Relation: "ExD", HeadToken: new(int64(4))}})
}

func TestAdaptResolvesStoredHeads(t *testing.T) {
	tree := coordinated()
	require.Len(t, tree.Nodes, 7)
	t.Run("a self-headed token is the root", func(t *testing.T) {
		assert.Equal(t, syntax.Root, tree.Nodes[0].Head)
	})
	t.Run("a printed head is the printed node", func(t *testing.T) {
		assert.Equal(t, 1, tree.Nodes[2].Head)
	})
	t.Run("an unprinted head is checked before the ordinal", func(t *testing.T) {
		assert.Equal(t, 6, tree.Nodes[5].Head)
		assert.False(t, tree.Nodes[6].Printed)
		assert.Equal(t, "e_1", tree.Nodes[6].SourceID)
	})
	t.Run("an unprinted node depends on a printed token", func(t *testing.T) {
		assert.Equal(t, 4, tree.Nodes[6].Head)
	})
}

func TestAdaptTreatsDanglingAndCyclicHeadsAsRoot(t *testing.T) {
	tree := Adapt([]Token{
		{Ordinal: 0, Surface: "a", Relation: "PRED", Head: 9},
		{Ordinal: 1, Surface: "b", Relation: "OBJ", Head: 1, HeadUnprintedNode: "missing"},
		{Ordinal: 2, Surface: "c", Relation: "OBJ", Head: 3},
		{Ordinal: 3, Surface: "d", Relation: "OBJ", Head: 2},
		{Ordinal: 4, Surface: "e", Relation: "OBJ", Head: 1, HeadUnprintedNode: "e_1"},
	}, []UnprintedNode{{SourceID: "e_1", Relation: "ExD", HeadUnprintedNode: "e_1"}})
	t.Run("a dangling ordinal is the root", func(t *testing.T) {
		assert.Equal(t, syntax.Root, tree.Nodes[0].Head)
	})
	t.Run("a dangling unprinted reference is the root", func(t *testing.T) {
		assert.Equal(t, syntax.Root, tree.Nodes[1].Head)
	})
	t.Run("a head cycle is broken so no node reaches itself", func(t *testing.T) {
		assert.Equal(t, syntax.Root, tree.Nodes[2].Head)
		assert.Equal(t, 2, tree.Nodes[3].Head)
	})
	t.Run("an unprinted node that heads itself is a root", func(t *testing.T) {
		assert.Equal(t, syntax.Root, tree.Nodes[5].Head)
	})
}

func TestTreeLooksDownThroughCoordination(t *testing.T) {
	tree := coordinated()
	t.Run("children look down through the coordination", func(t *testing.T) {
		assert.Equal(t, []int{2, 3, 4}, tree.EffectiveChildren(0, syntax.CategoryObject, syntax.CategorySubordinatingConjunction))
		assert.Equal(t, []int{2, 3}, tree.EffectiveChildren(0, syntax.CategoryObject))
	})
	t.Run("the coordination node itself is never an effective child", func(t *testing.T) {
		assert.NotContains(t, tree.EffectiveChildren(0), 1)
	})
	t.Run("the stored head is unchanged", func(t *testing.T) {
		assert.Equal(t, 1, tree.Nodes[2].Head)
	})
}

func TestTreeSubtreeAndPreviousPrinted(t *testing.T) {
	tree := coordinated()
	assert.Equal(t, []int{1, 2, 3}, tree.PrintedSubtree(1))
	assert.Equal(t, []int{5}, tree.PrintedSubtree(6))
	assert.Equal(t, 3, tree.PreviousPrinted(4))
	assert.Equal(t, syntax.Root, tree.PreviousPrinted(0))
	assert.Equal(t, syntax.Root, tree.PreviousPrinted(6))
}

// TestAdaptInterleavesUnprintedNodesInSourceOrder uses the shape of the stored
// fixture: source ids 1..12 where 2, 3, 9 and 10 are unprinted, so the printed
// tokens (ordinals 0..7) carry the ids 1, 4, 5, 6, 7, 8, 11, 12. Unprinted ids
// compare numerically, so "10" follows "9" and "2" precedes "10".
func TestAdaptInterleavesUnprintedNodesInSourceOrder(t *testing.T) {
	tokens := make([]Token, 8)
	for i := range tokens {
		tokens[i] = Token{Ordinal: int64(i), Surface: string(rune('a' + i)), Relation: "ATR", Head: 0}
	}
	tree := Adapt(tokens, []UnprintedNode{
		{SourceID: "10", Relation: "OBJ"}, {SourceID: "2", Relation: "PRED"},
		{SourceID: "9", Relation: "OBJ", HeadUnprintedNode: "10"}, {SourceID: "3", Relation: "OBJ", HeadUnprintedNode: "2"},
	})
	var order []string
	for _, node := range tree.Nodes {
		if node.Printed {
			order = append(order, node.Surface)
		} else {
			order = append(order, "e"+node.SourceID)
		}
	}
	t.Run("nodes are in source order", func(t *testing.T) {
		assert.Equal(t, []string{"a", "e2", "e3", "b", "c", "d", "e", "f", "e9", "e10", "g", "h"}, order)
	})
	t.Run("heads follow the reordered indexes", func(t *testing.T) {
		assert.Equal(t, 1, tree.Nodes[2].Head, "e3 depends on e2")
		assert.Equal(t, 9, tree.Nodes[8].Head, "e9 depends on e10")
	})
	t.Run("a printed token depending on an unprinted node resolves it", func(t *testing.T) {
		withHead := Adapt([]Token{{Ordinal: 0, Surface: "a", Relation: "SBJ", Head: 0, HeadUnprintedNode: "2"}, {Ordinal: 1, Surface: "b", Relation: "PRED", Head: 1}},
			[]UnprintedNode{{SourceID: "2", Relation: "PRED"}})
		assert.Equal(t, 1, withHead.Nodes[0].Head)
		assert.Equal(t, "2", withHead.Nodes[1].SourceID)
	})
	t.Run("PreviousPrinted skips unprinted nodes", func(t *testing.T) {
		assert.Equal(t, 0, tree.PreviousPrinted(3))
	})
}
