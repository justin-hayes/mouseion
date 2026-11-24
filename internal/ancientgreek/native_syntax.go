package ancientgreek

import (
	"strconv"

	"github.com/justin-hayes/mouseion/internal/analyzer"
)

// zetaDefectRelation is the AGDT relation AuxZ written with a Greek capital
// Zeta (U+0396) for its final letter; four Anabasis tokens carry it. It is an
// encoding defect, not a different relation.
const zetaDefectRelation = "AuxΖ"

// repairRelation returns the source relation verbatim, apart from the one
// encoding defect: the Greek-script AuxZ becomes the Latin-script AuxZ. It runs
// at import so no reader ever sees the defective spelling.
func repairRelation(relation string) string {
	if relation == zetaDefectRelation {
		return "AuxZ"
	}
	return relation
}

// sentenceSyntax resolves AGDT source head ids within one sentence. A source
// head id names a printed token, an unprinted (ellipsis) node, or 0 for the
// root; the printed ordinals are those of the kept tokens and never change
// with the syntax.
type sentenceSyntax struct {
	printed   map[int]uint32
	unprinted map[int]struct{}
}

// newSentenceSyntax indexes a sentence's printed and unprinted source ids.
func newSentenceSyntax(sentence sourceSentence) sentenceSyntax {
	syntax := sentenceSyntax{printed: map[int]uint32{}, unprinted: map[int]struct{}{}}
	var ordinal uint32
	for _, token := range sentence.tokens {
		if isEllipsis(token.pos) {
			syntax.unprinted[token.id] = struct{}{}
			continue
		}
		syntax.printed[token.id] = ordinal
		ordinal++
	}
	return syntax
}

// resolvedHead is the stored head of one source token: the printed ordinal, or
// the source id of the unprinted node.
type resolvedHead struct {
	root      bool
	ordinal   uint32
	unprinted string
}

// resolve maps a source head id to a printed ordinal, the root, or an
// unprinted node; an id the sentence does not contain is a malformed source.
func (s sentenceSyntax) resolve(head int) (resolvedHead, error) {
	if head == 0 {
		return resolvedHead{root: true}, nil
	}
	if ordinal, ok := s.printed[head]; ok {
		return resolvedHead{ordinal: ordinal}, nil
	}
	if _, ok := s.unprinted[head]; ok {
		return resolvedHead{unprinted: strconv.Itoa(head)}, nil
	}
	return resolvedHead{}, malformed(0, "head names no token of its sentence")
}

// storedHead returns the printed token's stored head and unprinted-node id: a
// root and an unprinted head are both self-headed, and the node id says which.
func (s sentenceSyntax) storedHead(token sourceToken, ordinal uint32) (uint32, string, error) {
	head, err := s.resolve(token.head)
	if err != nil {
		return 0, "", err
	}
	if head.root || head.unprinted != "" {
		return ordinal, head.unprinted, nil
	}
	return head.ordinal, "", nil
}

// unprintedNodes lists the sentence's unprinted syntax nodes in source order.
func (s sentenceSyntax) unprintedNodes(sentence sourceSentence) ([]analyzer.UnprintedSyntaxNode, error) {
	var nodes []analyzer.UnprintedSyntaxNode
	for _, token := range sentence.tokens {
		if !isEllipsis(token.pos) {
			continue
		}
		head, err := s.resolve(token.head)
		if err != nil {
			return nil, err
		}
		node := analyzer.UnprintedSyntaxNode{SourceID: strconv.Itoa(token.id), Dependency: repairRelation(token.relation), HeadUnprintedNode: head.unprinted}
		if !head.root && head.unprinted == "" {
			ordinal := head.ordinal
			node.HeadTokenOrdinal = &ordinal
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}
