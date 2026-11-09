package fixtures

// Contract status: illustrative. The browser fixture has no Classical registry.

import (
	"context"
	"errors"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// The fixture server keeps no Classical Reading-unit registry. The catalog
// still browses the embedded release; these stubs report no legacy rows, no
// ownership, and an Add that the fixture path does not support.

func (s *Store) ListLegacyLanguageRows(context.Context, string) ([]domain.LegacyLanguageRows, error) {
	return nil, nil
}

func (s *Store) ListOwnedClassicalReadingUnits(context.Context, string) ([]classical.OwnedReadingUnit, error) {
	return nil, nil
}

func (s *Store) AddClassicalReadingUnits(context.Context, string, []string) (classical.AddReport, error) {
	return classical.AddReport{}, errors.New("fixtures: classical Add is not available")
}
