package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/require"
)

func TestFailedOutcomeBecomesOmissionUsesTheFrozenKind(t *testing.T) {
	for _, test := range []struct {
		name        string
		kind        enrichment.ItemKind
		class, code string
		want        bool
	}{
		{
			name: "an unset kind follows the lemma rule", class: "validation", code: "invalid_response", want: true,
		},
		{
			name: "an unset kind does not tolerate another error class", class: "transport", code: "timeout", want: false,
		},
		{
			name: "a lemma follows the lemma rule", kind: enrichment.ItemKindLemma, class: "validation", code: "invalid_response", want: true,
		},
		{
			name: "an idiom tolerates any failure", kind: enrichment.ItemKindIdiom, class: "transport", code: "timeout", want: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, failedOutcomeBecomesOmission(test.kind, test.class, test.code))
		})
	}
}
