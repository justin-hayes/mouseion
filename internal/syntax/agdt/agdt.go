// Package agdt adapts the native AGDT syntax Mouseion stores for an imported
// Ancient Greek run into a source-neutral syntax.Tree (ADR 0122).
package agdt

import (
	"sort"

	"github.com/justin-hayes/mouseion/internal/syntax"
)

// Token is one stored printed token.
type Token struct {
	Ordinal              int64
	Surface, Lemma, UPOS string
	Morphology           map[string]string
	// Relation is the stored AGDT relation (corpus_tokens.dependency).
	Relation string
	// Head is the stored head ordinal. A token that is its own head is a root,
	// unless HeadUnprintedNode is set.
	Head int64
	// HeadUnprintedNode is the source id of the unprinted node the token
	// depends on, or empty. It is checked before Head.
	HeadUnprintedNode string
}

// UnprintedNode is one stored unprinted syntax node.
type UnprintedNode struct {
	SourceID string
	Relation string
	// HeadToken is the printed token ordinal the node depends on, if any.
	HeadToken *int64
	// HeadUnprintedNode is the source id of the unprinted node it depends on.
	HeadUnprintedNode string
}

// Adapt builds a Tree. A dangling head reference and a head cycle both end as
// the root, so a damaged row never fails a page.
func Adapt(tokens []Token, unprinted []UnprintedNode) syntax.Tree {
	tokens = append([]Token(nil), tokens...)
	sort.SliceStable(tokens, func(i, j int) bool { return tokens[i].Ordinal < tokens[j].Ordinal })
	unprinted = append([]UnprintedNode(nil), unprinted...)
	sort.SliceStable(unprinted, func(i, j int) bool { return unprinted[i].SourceID < unprinted[j].SourceID })

	index := headIndex{byOrdinal: make(map[int64]int, len(tokens)), bySource: make(map[string]int, len(unprinted))}
	for i, token := range tokens {
		index.byOrdinal[token.Ordinal] = i
	}
	for i, node := range unprinted {
		index.bySource[node.SourceID] = len(tokens) + i
	}

	nodes := make([]syntax.Node, 0, len(tokens)+len(unprinted))
	for _, token := range tokens {
		nodes = append(nodes, syntax.Node{
			Printed: true, Ordinal: token.Ordinal, Surface: token.Surface, Lemma: token.Lemma,
			UPOS: token.UPOS, Morphology: token.Morphology, Relation: token.Relation,
			Category: Category(token.Relation), Head: index.tokenHead(token),
		})
	}
	for _, node := range unprinted {
		nodes = append(nodes, syntax.Node{
			SourceID: node.SourceID, Relation: node.Relation, Category: Category(node.Relation),
			Head: index.resolve(node.HeadToken, node.HeadUnprintedNode),
		})
	}
	breakCycles(nodes)
	return syntax.Tree{Nodes: nodes}
}

// headIndex resolves stored head references to node indexes.
type headIndex struct {
	byOrdinal map[int64]int
	bySource  map[string]int
}

// resolve returns the node a stored reference names: an unprinted node when a
// source id is given (checked first), else a printed token. A reference to
// nothing is the root.
func (h headIndex) resolve(ordinal *int64, source string) int {
	if source != "" {
		if index, ok := h.bySource[source]; ok {
			return index
		}
		return syntax.Root
	}
	if ordinal != nil {
		if index, ok := h.byOrdinal[*ordinal]; ok {
			return index
		}
	}
	return syntax.Root
}

// tokenHead resolves a printed token's head. A token that is its own head, and
// is not headed by an unprinted node, is the root.
func (h headIndex) tokenHead(token Token) int {
	if token.HeadUnprintedNode != "" {
		return h.resolve(nil, token.HeadUnprintedNode)
	}
	if token.Head == token.Ordinal {
		return syntax.Root
	}
	return h.resolve(&token.Head, "")
}

// breakCycles makes any node that reaches itself through heads a root.
func breakCycles(nodes []syntax.Node) {
	for start := range nodes {
		seen := map[int]bool{start: true}
		for at := nodes[start].Head; at != syntax.Root; at = nodes[at].Head {
			if at == start {
				nodes[start].Head = syntax.Root
				break
			}
			if seen[at] {
				break
			}
			seen[at] = true
		}
	}
}

// Category maps an AGDT relation to its normalised category, exactly. Anything
// else, including "_", is CategoryOther: the measured detector precision
// depends on these semantics, so no new normalisation is added.
func Category(relation string) syntax.Category {
	switch relation {
	case "PRED":
		return syntax.CategoryPredicate
	case "SBJ":
		return syntax.CategorySubject
	case "OBJ":
		return syntax.CategoryObject
	case "ATR":
		return syntax.CategoryAttribute
	case "ADV":
		return syntax.CategoryAdverbial
	case "PNOM":
		return syntax.CategoryPredicateNominal
	case "OCOMP":
		return syntax.CategoryObjectComplement
	case "ATV", "AtvV":
		return syntax.CategoryPredicativeComplement
	case "APOS":
		return syntax.CategoryApposition
	case "COORD":
		return syntax.CategoryCoordination
	case "AuxC":
		return syntax.CategorySubordinatingConjunction
	case "AuxP":
		return syntax.CategoryPreposition
	case "AuxY":
		return syntax.CategorySentenceParticle
	case "AuxZ":
		return syntax.CategoryEmphasisingParticle
	case "AuxV":
		return syntax.CategoryAuxiliary
	case "ExD":
		return syntax.CategoryOutsideStructure
	case "AuxX", "AuxK", "AuxG":
		return syntax.CategoryPunctuation
	}
	return syntax.CategoryOther
}
