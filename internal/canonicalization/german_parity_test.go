package canonicalization

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGermanNormalizationMatchesDictionaryParityFixture(t *testing.T) {
	file, err := os.Open("../../nlp/testdata/german_normalization_parity.jsonl")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	profile := GermanPost1996()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var fixture struct {
			Word             string `json:"word"`
			RuntimeLookupKey string `json:"runtime_lookup_key"`
		}
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &fixture))
		t.Run(fixture.Word, func(t *testing.T) {
			require.Equal(t, fixture.RuntimeLookupKey, profile.Canonical(fixture.Word))
		})
	}
	require.NoError(t, scanner.Err())
}
