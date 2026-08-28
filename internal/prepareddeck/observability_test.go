package prepareddeck

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
)

type testLogger struct{ bytes.Buffer }

func (l *testLogger) Printf(format string, args ...any) {
	_, _ = fmt.Fprintf(&l.Buffer, format, args...)
}

func TestLogObserverIsContentFree(t *testing.T) {
	logger := &testLogger{}
	NewLogObserver(logger).Observe(Observation{
		Event: "prepared_deck_preparation", Outcome: "failed",
		Counts: OutcomeCounts{Selected: 3, ProviderCalls: 2},
		Errors: ErrorCounts{Other: 1}, Completeness: completenessOf(cardexport.Completeness{TotalCards: 3}),
	})
	encoded := strings.ToLower(logger.String())
	for _, prohibited := range []string{"owner", "lemma", "title", "sentence", "prompt", "response", "credential", "raw"} {
		if strings.Contains(encoded, prohibited) {
			t.Fatalf("log contains prohibited field %q: %s", prohibited, logger.String())
		}
	}
}
