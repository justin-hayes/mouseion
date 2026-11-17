package fixtures

// Contract status: illustrative. The fixture preparation seam substitutes a
// completed canned result for a real job; it writes no store state.

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/analysis"
)

// ClassicalPreparation stands in for the preparation job. Moving a fixture
// classical Book to To Read completes immediately; the Book's prepared evidence
// is projected from its To Read state rather than written to the store.
type ClassicalPreparation struct{}

// SubmitClassicalPreparation reports that the Book is already prepared.
func (ClassicalPreparation) SubmitClassicalPreparation(context.Context, string, string) (analysis.ClassicalPreparationHandle, error) {
	return analysis.ClassicalPreparationHandle{Complete: true}, nil
}
