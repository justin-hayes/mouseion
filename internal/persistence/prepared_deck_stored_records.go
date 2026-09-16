package persistence

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// PreparedDeckStoredRecord is an exact cache row associated with one frozen
// accepted item. Persistence returns the row as stored; presentation decides
// how its fields are interpreted during finalization.
type PreparedDeckStoredRecord struct {
	Ordinal  int
	CacheKey enrichment.CacheKey
	Entry    enrichment.CacheEntry
	Found    bool
}

// LoadPreparedDeckStoredRecords returns cache records in frozen manifest
// order. Missing records are represented explicitly so callers can apply the
// run's required-result policy without persistence constructing a semantic
// enrichment result.
func (s *PostgresStore) LoadPreparedDeckStoredRecords(ctx context.Context, owner, preparationID, runID string) ([]PreparedDeckStoredRecord, error) {
	projection, _, err := s.LoadPreparedDeckStorageProjection(ctx, owner, preparationID, runID)
	if err != nil {
		return nil, err
	}
	records := make([]PreparedDeckStoredRecord, 0)
	for _, item := range projection.Items {
		if item.Disposition != cardexport.ManifestAccepted || item.CacheKey == nil {
			continue
		}
		key := *item.CacheKey
		entry, found, err := s.Get(ctx, key)
		if err != nil {
			return nil, err
		}
		records = append(records, PreparedDeckStoredRecord{Ordinal: item.Ordinal, CacheKey: key, Entry: entry, Found: found})
	}
	return records, nil
}
