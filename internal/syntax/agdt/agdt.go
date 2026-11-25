// Package agdt adapts the native AGDT syntax Mouseion stores for an imported
// Ancient Greek run into a source-neutral syntax.Tree (ADR 0122).
package agdt

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
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

// Adapt builds a Tree in source order. The stored printed tokens carry no
// source id, so it is inferred: source ids run 1..N within a sentence, the
// unprinted nodes hold the ids they name, and the printed tokens, in ordinal
// order, hold the remaining ids. An unprinted node therefore sits before the
// printed token that follows it in the source. Unprinted ids compare
// numerically, so "10" follows "9"; a non-numeric id sorts last, by text. A
// dangling head reference and a head cycle both end as the root, so a damaged
// row never fails a page.
func Adapt(tokens []Token, unprinted []UnprintedNode) syntax.Tree {
	return adapt(tokens, unprinted, func(relation string) (syntax.Category, bool) { return Category(relation), false })
}

// AdaptLatin is Adapt for an Opera Latina Adnotata run: each relation is read
// through ReadLatinRelation, so a node carries its base category and a conjunct
// flag. Stored relations stay verbatim (ADR 0122 decision 3, ADR 0132).
func AdaptLatin(tokens []Token, unprinted []UnprintedNode) syntax.Tree {
	return adapt(tokens, unprinted, func(relation string) (syntax.Category, bool) {
		base, conjunct := ReadLatinRelation(relation)
		return Category(base), conjunct
	})
}

func adapt(tokens []Token, unprinted []UnprintedNode, read func(string) (syntax.Category, bool)) syntax.Tree {
	tokens = append([]Token(nil), tokens...)
	sort.SliceStable(tokens, func(i, j int) bool { return tokens[i].Ordinal < tokens[j].Ordinal })
	unprinted = append([]UnprintedNode(nil), unprinted...)
	sort.SliceStable(unprinted, func(i, j int) bool { return unprinted[i].SourceID < unprinted[j].SourceID })

	entries := sourceOrder(len(tokens), unprinted)

	index := headIndex{byOrdinal: make(map[int64]int, len(tokens)), bySource: make(map[string]int, len(unprinted))}
	for position, e := range entries {
		if e.printed {
			index.byOrdinal[tokens[e.at].Ordinal] = position
		} else {
			index.bySource[unprinted[e.at].SourceID] = position
		}
	}

	nodes := make([]syntax.Node, 0, len(entries))
	for _, e := range entries {
		if e.printed {
			token := tokens[e.at]
			category, conjunct := read(token.Relation)
			nodes = append(nodes, syntax.Node{
				Printed: true, Ordinal: token.Ordinal, Surface: token.Surface, Lemma: token.Lemma,
				UPOS: token.UPOS, Morphology: token.Morphology, Relation: token.Relation,
				Category: category, Conjunct: conjunct, Head: index.tokenHead(token),
			})
			continue
		}
		node := unprinted[e.at]
		category, conjunct := read(node.Relation)
		nodes = append(nodes, syntax.Node{
			SourceID: node.SourceID, Relation: node.Relation, Category: category, Conjunct: conjunct,
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

// entry is one node's place in source order: a printed token or an unprinted
// node, by index into its own slice.
type entry struct {
	key     int64
	printed bool
	at      int
	source  string
}

// sourceOrder places the printed tokens and unprinted nodes in source order by
// their (inferred) source ids.
func sourceOrder(printed int, unprinted []UnprintedNode) []entry {
	taken := map[int64]bool{}
	entries := make([]entry, 0, printed+len(unprinted))
	for i, node := range unprinted {
		key := int64(math.MaxInt64)
		if id, err := strconv.ParseInt(node.SourceID, 10, 64); err == nil && id > 0 {
			key = id
			taken[id] = true
		}
		entries = append(entries, entry{key: key, at: i, source: node.SourceID})
	}
	next := int64(1)
	for i := range printed {
		for taken[next] {
			next++
		}
		entries = append(entries, entry{key: next, printed: true, at: i})
		next++
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].key != entries[j].key {
			return entries[i].key < entries[j].key
		}
		return entries[i].source < entries[j].source
	})
	return entries
}

// FromSentence adapts one freshly imported sentence, which carries its
// unprinted nodes, without a database.
func FromSentence(sentence analyzer.Sentence) syntax.Tree {
	tokens := make([]Token, len(sentence.Tokens))
	for i, token := range sentence.Tokens {
		tokens[i] = Token{
			Ordinal: int64(i), Surface: token.Surface, Lemma: token.RawLemma, UPOS: token.UPOS,
			Morphology: token.Morphology, Relation: token.Dependency, Head: int64(token.Head),
			HeadUnprintedNode: token.HeadUnprintedNode,
		}
	}
	nodes := make([]UnprintedNode, len(sentence.UnprintedNodes))
	for i, node := range sentence.UnprintedNodes {
		nodes[i] = UnprintedNode{SourceID: node.SourceID, Relation: node.Dependency, HeadUnprintedNode: node.HeadUnprintedNode}
		if node.HeadTokenOrdinal != nil {
			ordinal := int64(*node.HeadTokenOrdinal)
			nodes[i].HeadToken = &ordinal
		}
	}
	return Adapt(tokens, nodes)
}

// ReadLatinRelation reads an OLA relation: it strips the _CO, _AP and _ExD
// suffixes, in any order and combination, and reports whether _CO (a conjunct
// of a coordination) was among them. A relation with no base before its
// suffixes, or with any unknown suffix, is returned unchanged.
func ReadLatinRelation(relation string) (base string, conjunct bool) {
	rest := relation
	for {
		cut := strings.LastIndexByte(rest, '_')
		if cut <= 0 {
			break
		}
		switch rest[cut:] {
		case "_CO":
			conjunct = true
		case "_AP", "_ExD":
		default:
			cut = -1
		}
		if cut < 0 {
			return relation, false
		}
		rest = rest[:cut]
	}
	if strings.HasPrefix(rest, "_") {
		return relation, false
	}
	return rest, conjunct
}
