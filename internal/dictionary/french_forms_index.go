package dictionary

import (
	"context"
	"errors"
	"fmt"
)

// frenchLanguage is the language tag the French form-of probe uses.
const frenchLanguage = "fr"

// ErrFrenchFormsUnavailable reports that the index predates the French form-of
// data French lemma existence needs.
var ErrFrenchFormsUnavailable = errors.New("dictionary: French form-of data requires a rebuilt index")

// indexFormatWithFrenchForms is the first non-LSJ index format carrying the
// French form-of links and headword membership (ticket #2244). LSJ indexes start
// at format 6. The table probe, not the version alone, is authoritative: a
// historical format-4 LSJ index has the number but not the tables.
const indexFormatWithFrenchForms = 4

// loadFrenchFormsAvailability records whether the index carries both French
// form-of tables. A missing table or an older format reports French lemma
// existence unavailable instead of failing the whole index.
func (i *Index) loadFrenchFormsAvailability(ctx context.Context) error {
	version, ok, err := i.indexFormat(ctx)
	if err != nil {
		return err
	}
	if !ok || version < indexFormatWithFrenchForms {
		return nil
	}
	var available bool
	err = i.db.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'french_forms')
		AND EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'french_headwords')`).Scan(&available)
	if err != nil {
		return fmt.Errorf("%w: French form schema: %w", ErrInvalidIndex, err)
	}
	i.frenchFormsAvailable = available
	return nil
}
