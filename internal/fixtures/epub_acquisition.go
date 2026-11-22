// Contract status: illustrative. Canned state for browser scenarios; not held
// to internal/storecontract parity (ADR 0088).

package fixtures

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/analysis"
)

// EPUBAcquisition reports a queued acquisition without a queue. The fixture
// server has no River client; the browser scenarios only assert the
// confirmation and the Book's membership.
type EPUBAcquisition struct{}

func (EPUBAcquisition) SubmitEPUBAcquisition(context.Context, string, string) (analysis.EPUBAcquisitionHandle, error) {
	return analysis.EPUBAcquisitionHandle{JobID: 1}, nil
}
