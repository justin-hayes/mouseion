package sourcecheck

import (
	"context"
	"errors"
	"fmt"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
)

// loadEvidence resolves the current EPUB source evidence a check may compare.
// A missing source, an imported analysis or an absent snapshot is not a check
// failure: it simply has nothing this contract can compare.
func loadEvidence(ctx context.Context, store Store, ownerID, sourceMaterialID string) (source domain.SourceMaterial, current domain.ExtractedSnapshot, ok bool, err error) {
	source, err = store.GetSourceMaterial(ctx, ownerID, sourceMaterialID)
	if errors.Is(err, domain.ErrNotFound) {
		return source, current, false, nil
	}
	if err != nil {
		return source, current, false, fmt.Errorf("load source for structure check: %w", err)
	}
	if source.MediaType != epub.MediaType() {
		return source, current, false, nil // imported analyses are excluded
	}
	current, err = store.GetCurrentExtractedSnapshot(ctx, ownerID, sourceMaterialID)
	if errors.Is(err, domain.ErrExtractedUnitsUnavailable) {
		return source, current, false, nil
	}
	if err != nil {
		return source, current, false, fmt.Errorf("load current snapshot for structure check: %w", err)
	}
	return source, current, true, nil
}
