package ancientgreek

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSplitWord(t *testing.T) {
	for surface, want := range map[string]bool{
		"x": true, "q": true, "B": true, "ū": true,
		"a": false, "E": false, "o": false, "i": false, // real one-letter words
		"M": false, "C": false, "L": false, "T": false, // praenomen initials
		"et": false, ".": false, "": false,
	} {
		assert.Equal(t, want, isSplitWord(surface, "n"), surface)
	}
	assert.False(t, isSplitWord("X", "m"), "a Roman numeral letter is a numeral")
}

func TestScanLatinWorkCountsUncitedBackwardsAndSplitTokens(t *testing.T) {
	conllu, paula := olaFixture([][]olaToken{
		{{"arma", "arma", "n", "_", "OBJ", "1.2", 0}, {"u", "u", "n", "_", "SBJ", "1.2", 0}, {".", ".", "u", "_", "AuxK", "1.2", 0}},
		{{"uirum", "uir", "n", "_", "OBJ", "1.1", 0}, {"que", "que", "c", "_", "COORD", "", 0}},
	})
	paula = []byte(strings.Replace(string(paula), `<feat id="u_5" xlink:href="#t_5" value=""/>`+"\n", "", 1))
	scan, err := ScanLatinWork(conllu, paula)
	require.NoError(t, err)
	assert.Equal(t, LatinScan{Tokens: 5, Words: 4, Uncited: 1, Backwards: 1, FirstBackwards: "1.1", SplitWords: 1}, scan)
	assert.InDelta(t, 0.25, scan.SplitWordRate(), 1e-9)
}
