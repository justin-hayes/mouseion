package constructions

import (
	"strings"

	"github.com/justin-hayes/mouseion/internal/syntax"
	"github.com/justin-hayes/mouseion/internal/syntax/agdt"
)

// w is one synthetic node. Head is an index into the list (-1 is the root);
// feats is "Key=Value,Key=Value"; an empty lemma makes an unprinted node.
type w struct {
	lemma, rel string
	head       int
	feats      string
}

const (
	inf   = "VerbForm=Inf"
	part  = "VerbForm=Part"
	subj  = "VerbForm=Fin,Mood=Sub"
	opt   = "VerbForm=Fin,Mood=Opt"
	indic = "VerbForm=Fin,Mood=Ind"
)

// build makes a syntax.Tree from nodes in source order. Printed nodes get
// ordinals in order; a DET lemma is the article.
func build(nodes ...w) syntax.Tree {
	tree := syntax.Tree{}
	ordinal := int64(0)
	for _, n := range nodes {
		node := syntax.Node{Relation: n.rel, Category: agdt.Category(n.rel), Head: n.head}
		if n.lemma != "" {
			node.Printed, node.Ordinal, node.Lemma, node.Surface = true, ordinal, n.lemma, n.lemma
			node.UPOS = "NOUN"
			if n.lemma == "ὁ" {
				node.UPOS = "DET"
			}
			node.Morphology = map[string]string{}
			var upos string
			for pair := range strings.SplitSeq(n.feats, ",") {
				key, value, ok := strings.Cut(pair, "=")
				switch {
				case !ok:
				case key == "UPOS":
					upos = value
				default:
					node.Morphology[key] = value
				}
			}
			if upos != "" {
				node.UPOS = upos
			} else if node.Morphology["VerbForm"] != "" {
				node.UPOS = "VERB"
			}
			ordinal++
		} else {
			node.SourceID = "u"
		}
		tree.Nodes = append(tree.Nodes, node)
	}
	return tree
}

func has(occurrences []Occurrence, id ID) (Occurrence, bool) {
	for _, o := range occurrences {
		if o.Construction == id {
			return o, true
		}
	}
	return Occurrence{}, false
}
