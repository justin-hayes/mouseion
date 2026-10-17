package canonicalization

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFrenchNormalizationMatchesProducerParityFixture pins the Go French
// profile to the same committed fixture the Python producer reads, so the two
// canonical-lemma implementations cannot drift.
func TestFrenchNormalizationMatchesProducerParityFixture(t *testing.T) {
	file, err := os.Open("../../nlp/testdata/french_normalization_parity.jsonl")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })

	profile := French()
	scanner := bufio.NewScanner(file)
	rows := 0
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var fixture struct {
			Raw       string `json:"raw"`
			Canonical string `json:"canonical"`
		}
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &fixture))
		rows++
		t.Run(fixture.Raw, func(t *testing.T) {
			require.Equal(t, fixture.Canonical, profile.Canonical(fixture.Raw))
		})
	}
	require.NoError(t, scanner.Err())
	require.NotEmpty(t, rows)
}
