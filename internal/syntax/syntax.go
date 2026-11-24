// Package syntax is the source-neutral view of one sentence's dependency
// structure. A scheme adapter (internal/syntax/agdt for Ancient Greek) turns
// stored rows into a Tree; the Word relations table and the construction
// detectors read only the Tree, so a further scheme needs no redesign.
package syntax

import "slices"

// Category is the closed, scheme-independent role of a node.
type Category int

const (
	CategoryOther Category = iota
	CategoryPredicate
	CategorySubject
	CategoryObject
	CategoryAttribute
	CategoryAdverbial
	CategoryPredicateNominal
	CategoryObjectComplement
	CategoryPredicativeComplement
	CategoryApposition
	CategoryCoordination
	CategorySubordinatingConjunction
	CategoryPreposition
	CategorySentenceParticle
	CategoryEmphasisingParticle
	CategoryAuxiliary
	CategoryOutsideStructure
	CategoryPunctuation
)

// Root is the Head of a node that depends on nothing.
const Root = -1

// Node is one syntax node. A printed node has a token ordinal, surface, source
// lemma, UPOS and morphology. An unprinted node has only its source id.
type Node struct {
	// Printed reports a node with a token in the sentence.
	Printed bool
	// Ordinal is the 0-based token ordinal of a printed node.
	Ordinal int64
	// SourceID is the source's id of an unprinted node.
	SourceID string

	Surface, Lemma, UPOS string
	Morphology           map[string]string

	// Relation is the raw source relation string.
	Relation string
	Category Category
	// Head is an index into Tree.Nodes, or Root.
	Head int
}

// Tree holds one sentence's nodes in source order, printed and unprinted
// interleaved.
type Tree struct {
	Nodes []Node
}

// Children returns the indexes of the nodes whose Head is n, in node order.
func (t Tree) Children(n int) []int {
	var out []int
	for i, node := range t.Nodes {
		if node.Head == n && i != n {
			out = append(out, i)
		}
	}
	return out
}

// EffectiveChildren returns the children of n looking down through
// coordination groups: a coordination child is replaced by its own effective
// children, recursively. When categories are given, only nodes of those
// categories are returned (the coordination node itself is never returned).
func (t Tree) EffectiveChildren(n int, categories ...Category) []int {
	var out []int
	seen := map[int]bool{n: true}
	var walk func(parent int)
	walk = func(parent int) {
		for _, child := range t.Children(parent) {
			if seen[child] {
				continue
			}
			seen[child] = true
			if t.Nodes[child].Category == CategoryCoordination {
				walk(child)
				continue
			}
			out = append(out, child)
		}
	}
	walk(n)
	if len(categories) == 0 {
		return out
	}
	var kept []int
	for _, child := range out {
		if slices.Contains(categories, t.Nodes[child].Category) {
			kept = append(kept, child)
		}
	}
	return kept
}

// EffectiveHead returns the head of n looking up through coordination groups,
// or Root. A cycle ends as Root.
func (t Tree) EffectiveHead(n int) int {
	seen := map[int]bool{n: true}
	head := t.Nodes[n].Head
	for head != Root && t.Nodes[head].Category == CategoryCoordination {
		if seen[head] {
			return Root
		}
		seen[head] = true
		head = t.Nodes[head].Head
	}
	return head
}

// PrintedSubtree returns the printed nodes in n's subtree, n included when it
// is printed, in token order.
func (t Tree) PrintedSubtree(n int) []int {
	inside := map[int]bool{n: true}
	stack := []int{n}
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, child := range t.Children(node) {
			if !inside[child] {
				inside[child] = true
				stack = append(stack, child)
			}
		}
	}
	var out []int
	for i, node := range t.Nodes {
		if inside[i] && node.Printed {
			out = append(out, i)
		}
	}
	return out
}

// PreviousPrinted returns the printed node before n in token order, or Root
// when n is the first printed node.
func (t Tree) PreviousPrinted(n int) int {
	previous := Root
	for i, node := range t.Nodes {
		if !node.Printed {
			continue
		}
		if i == n {
			return previous
		}
		previous = i
	}
	return Root
}
