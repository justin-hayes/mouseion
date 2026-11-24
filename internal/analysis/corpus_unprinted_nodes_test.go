package analysis

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCorpusTokenChecksUnprintedHead(t *testing.T) {
	nodes := map[string]struct{}{"2": {}}
	cases := []struct {
		name    string
		token   analyzer.Token
		ordinal int
		wantErr string
	}{
		{"a token headed by a known node is accepted", analyzer.Token{Dependency: "SBJ", Head: 1, HeadUnprintedNode: "2"}, 1, ""},
		{"a node head must be self-headed", analyzer.Token{Dependency: "SBJ", Head: 0, HeadUnprintedNode: "2"}, 1, "must be self-headed"},
		{"a node head must exist in the sentence", analyzer.Token{Dependency: "SBJ", Head: 1, HeadUnprintedNode: "9"}, 1, "names no unprinted node"},
		{"a plain token needs no node", analyzer.Token{Dependency: "SBJ", Head: 0}, 1, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCorpusToken(0, tc.ordinal, 3, tc.token, nodes)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestValidateUnprintedNodesRefusesBrokenNodes(t *testing.T) {
	printed := uint32(1)
	missing := uint32(7)
	both := analyzer.UnprintedSyntaxNode{SourceID: "2", Dependency: "OBJ", HeadTokenOrdinal: &printed, HeadUnprintedNode: "3"}
	cases := []struct {
		name    string
		nodes   []analyzer.UnprintedSyntaxNode
		wantErr string
	}{
		{"valid", []analyzer.UnprintedSyntaxNode{{SourceID: "2", Dependency: "PRED"}, {SourceID: "3", Dependency: "OBJ", HeadUnprintedNode: "2"}, {SourceID: "4", Dependency: "ATR", HeadTokenOrdinal: &printed}}, ""},
		{"empty source id", []analyzer.UnprintedSyntaxNode{{Dependency: "PRED"}}, "source id is required"},
		{"empty dependency", []analyzer.UnprintedSyntaxNode{{SourceID: "2"}}, "dependency is required"},
		{"duplicate source id", []analyzer.UnprintedSyntaxNode{{SourceID: "2", Dependency: "PRED"}, {SourceID: "2", Dependency: "OBJ"}}, "duplicate"},
		{"two heads", []analyzer.UnprintedSyntaxNode{both, {SourceID: "3", Dependency: "PRED"}}, "both a printed and an unprinted head"},
		{"printed head out of range", []analyzer.UnprintedSyntaxNode{{SourceID: "2", Dependency: "OBJ", HeadTokenOrdinal: &missing}}, "out of range"},
		{"unknown node head", []analyzer.UnprintedSyntaxNode{{SourceID: "2", Dependency: "OBJ", HeadUnprintedNode: "5"}}, "names no unprinted node"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateUnprintedNodes(0, 3, tc.nodes)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestCorpusTokenRowBindsHeadUnprintedNode(t *testing.T) {
	tuple, args, err := corpusTokenRow("o", "grc", "r", "c", 0, 1, 1, analyzer.Token{Dependency: "SBJ", HeadUnprintedNode: "2"})
	require.NoError(t, err)
	assert.Len(t, args, corpusTokenParameterCount)
	assert.Contains(t, tuple, "$19")
	assert.Equal(t, "2", args[corpusTokenParameterCount-1])

	_, args, err = corpusTokenRow("o", "grc", "r", "c", 0, 1, 1, analyzer.Token{Dependency: "SBJ"})
	require.NoError(t, err)
	assert.Nil(t, args[corpusTokenParameterCount-1], "no node head is a NULL, not an empty string")
}
