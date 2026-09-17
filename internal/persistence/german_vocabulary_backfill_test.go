package persistence

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeSelectionCandidatesRejectsInvalidMinimumOccurrences(t *testing.T) {
	for name, provenance := range map[string]string{
		"negative":   `{"min_occurrences":-1}`,
		"fractional": `{"min_occurrences":1.5}`,
		"overflow":   `{"min_occurrences":9223372036854775808}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := mergeSelectionCandidates([]selectionCandidateRow{{
				provenance: []byte(`{"min_occurrences": 1}`),
			}, {
				provenance: []byte(provenance),
			}})
			require.Error(t, err)
		})
	}
}
