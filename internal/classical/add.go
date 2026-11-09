package classical

import "errors"

// LegacyRowsCode is the stable code the catalog reports while un-converted
// legacy Ancient Greek rows exist. Adding and preparation are unavailable in
// that state; browsing and existing Books remain readable.
const LegacyRowsCode = "classical_catalog_legacy_rows"

// LegacyRowsReason is the stable learner-facing reason paired with
// LegacyRowsCode. It names the remedy rather than leaving the learner at a
// blocked action.
const LegacyRowsReason = "Adding is unavailable until this installation's legacy Ancient Greek rows are converted. Ask the operator to finish the conversion. Code: " + LegacyRowsCode + "."

// ErrLegacyRows refuses an Add while the installation still holds legacy rows.
var ErrLegacyRows = errors.New("classical: legacy " + Language + " rows exist (" + LegacyRowsCode + ")")

// AddedBook is the outcome of adding one Reading unit for one learner.
type AddedBook struct {
	ReadingUnitKey string
	BookID         string
	Created        bool
}

// AddReport is the outcome of one catalog Add. A whole-work Add reports one
// entry per current Reading unit in constituent-book order.
type AddReport struct {
	Books []AddedBook
}

// CreatedBooks counts the Books this Add created. Re-adding an existing Book
// reports it as opened instead.
func (r AddReport) CreatedBooks() int {
	created := 0
	for _, book := range r.Books {
		if book.Created {
			created++
		}
	}
	return created
}

// OwnedReadingUnit is one learner Book associated with a Reading unit. It
// carries no preparation state: the catalog only needs ownership.
type OwnedReadingUnit struct {
	ReadingUnitKey string
	BookID         string
}

// Unit returns the Work and unit that carry a Reading-unit key.
func (c *Catalog) Unit(key string) (CatalogWork, CatalogUnit, bool) {
	for _, work := range c.works {
		for _, unit := range work.Units {
			if unit.Key == key {
				return work, unit, true
			}
		}
	}
	return CatalogWork{}, CatalogUnit{}, false
}
