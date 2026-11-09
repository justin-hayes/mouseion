package classical

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// Reconciler writes one release's manifest into the shared Work, Edition and
// Reading-unit registry in one idempotent transaction.
type Reconciler interface {
	ReconcileClassicalManifest(context.Context, []Definition) error
}

// Reconcile parses and validates the embedded manifest, then reconciles it. It
// is the startup path: a broken manifest or a refused revision stops the
// process before any surface opens.
func Reconcile(ctx context.Context, store Reconciler, manifestJSONL []byte) error {
	definitions, err := ParseDefinitions(manifestJSONL)
	if err != nil {
		return err
	}
	if err := ValidateManifest(definitions); err != nil {
		return err
	}
	return store.ReconcileClassicalManifest(ctx, definitions)
}

// ValidateManifest checks the facts reconciliation relies on: permanent keys
// are unique, and every current definition of one Work or Edition agrees on
// the shared curatorial fields. A withdrawn tombstone keeps the values it was
// published with, so it is ignored when current definitions still exist.
func ValidateManifest(definitions []Definition) error {
	if len(definitions) == 0 {
		return errors.New("classical: manifest has no definitions")
	}
	if err := validateManifestKeys(definitions); err != nil {
		return err
	}
	if err := validateCurrentDefinitions(definitions); err != nil {
		return err
	}
	return validateConstituentBooks(definitions)
}

// validateManifestKeys refuses an incomplete definition or a repeated
// permanent Reading-unit key.
func validateManifestKeys(definitions []Definition) error {
	keys := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return err
		}
		if _, duplicate := keys[definition.Key]; duplicate {
			return fmt.Errorf("classical: manifest repeats Reading-unit key %q", definition.Key)
		}
		keys[definition.Key] = struct{}{}
	}
	return nil
}

// validateCurrentDefinitions checks the shared curatorial fields of every
// current Work and Edition.
func validateCurrentDefinitions(definitions []Definition) error {
	works := map[string]Definition{}
	editions := map[string]Definition{}
	for _, definition := range definitions {
		if definition.Withdrawn {
			continue
		}
		if err := validateWorkDefinition(works, definition); err != nil {
			return err
		}
		if err := validateEditionDefinition(editions, definition); err != nil {
			return err
		}
	}
	return nil
}

// validateWorkDefinition records one current definition per Work and refuses a
// disagreeing curatorial field.
func validateWorkDefinition(works map[string]Definition, definition Definition) error {
	previous, ok := works[definition.WorkKey]
	if !ok {
		works[definition.WorkKey] = definition
		return nil
	}
	switch {
	case previous.WorkTitle != definition.WorkTitle:
		return fmt.Errorf("classical: Work %q has conflicting titles", definition.WorkKey)
	case previous.WorkAuthor != definition.WorkAuthor:
		return fmt.Errorf("classical: Work %q has conflicting authors", definition.WorkKey)
	case previous.GreekTitle != definition.GreekTitle:
		return fmt.Errorf("classical: Work %q has conflicting Greek titles", definition.WorkKey)
	case previous.ConstituentBooks != definition.ConstituentBooks:
		return fmt.Errorf("classical: Work %q has conflicting constituent-book counts", definition.WorkKey)
	case (previous.ConstituentBook == nil) != (definition.ConstituentBook == nil):
		return fmt.Errorf("classical: Work %q mixes a whole-work unit with constituent books", definition.WorkKey)
	}
	return nil
}

// validateEditionDefinition records one current definition per Edition and
// refuses a disagreeing Work or label.
func validateEditionDefinition(editions map[string]Definition, definition Definition) error {
	previous, ok := editions[definition.EditionKey]
	if !ok {
		editions[definition.EditionKey] = definition
		return nil
	}
	if previous.WorkKey != definition.WorkKey {
		return fmt.Errorf("classical: Edition %q belongs to more than one Work", definition.EditionKey)
	}
	if previous.EditionLabel != definition.EditionLabel {
		return fmt.Errorf("classical: Edition %q has conflicting labels", definition.EditionKey)
	}
	return nil
}

// validateConstituentBooks refuses two current units claiming one constituent
// book of the same Work, because a constituent book is one Reading unit.
func validateConstituentBooks(definitions []Definition) error {
	bookNumbers := map[string]map[int]string{}
	for _, definition := range definitions {
		if definition.Withdrawn || definition.ConstituentBook == nil {
			continue
		}
		work := bookNumbers[definition.WorkKey]
		if work == nil {
			work = map[int]string{}
			bookNumbers[definition.WorkKey] = work
		}
		book := *definition.ConstituentBook
		if previous, duplicate := work[book]; duplicate {
			return fmt.Errorf("classical: Work %q repeats constituent book %d in %q and %q", definition.WorkKey, book, previous, definition.Key)
		}
		work[book] = definition.Key
	}
	return nil
}

// CatalogCounts states the running release's catalog: current works, authors
// and Reading units. Withdrawn tombstones are not counted.
type CatalogCounts struct {
	Works   int
	Authors int
	Units   int
}

// CatalogAuthor is one author in the author index with the works that carry a
// current Reading unit.
type CatalogAuthor struct {
	Name  string
	Works []CatalogWork
}

// CatalogWork is one Work as the author page and work page render it. Units are
// ordered by constituent book.
type CatalogWork struct {
	Key                  string
	Title                string
	Author               string
	GreekTitle           string
	ConstituentBooks     int
	WholeWork            bool
	Words                int
	TranslationAvailable bool
	Units                []CatalogUnit
}

// CatalogUnit is one constituent book, or the whole work when ConstituentBook is zero.
type CatalogUnit struct {
	Key             string
	ConstituentBook int
	CitationStart   string
	CitationEnd     string
	Words           int
}

// Catalog is the read-only view of the embedded, reconciled manifest. It is
// built once from the release and never scoped to a learner's study language.
type Catalog struct {
	counts  CatalogCounts
	authors []CatalogAuthor
	works   map[string]CatalogWork
}

// LoadCatalog parses a release manifest and builds its browse view.
func LoadCatalog(manifestJSONL []byte) (*Catalog, error) {
	definitions, err := ParseDefinitions(manifestJSONL)
	if err != nil {
		return nil, err
	}
	return NewCatalog(definitions)
}

// NewCatalog builds the browse view from a manifest, hiding withdrawn
// definitions. It returns an error when the manifest is inconsistent, which can
// only be a broken build.
func NewCatalog(definitions []Definition) (*Catalog, error) {
	if err := ValidateManifest(definitions); err != nil {
		return nil, err
	}
	worksByKey := buildCatalogWorks(definitions)
	catalog := &Catalog{authors: []CatalogAuthor{}, works: map[string]CatalogWork{}}
	for key, work := range worksByKey {
		work.TranslationAvailable = translationAvailable(definitions, key)
		sortUnits(work.Units)
		catalog.works[key] = *work
	}
	catalog.authors = buildCatalogAuthors(worksByKey)
	units := 0
	for _, work := range catalog.works {
		units += len(work.Units)
	}
	catalog.counts = CatalogCounts{Works: len(worksByKey), Authors: len(catalog.authors), Units: units}
	return catalog, nil
}

// buildCatalogWorks groups current definitions into Works and their units.
func buildCatalogWorks(definitions []Definition) map[string]*CatalogWork {
	worksByKey := map[string]*CatalogWork{}
	for _, definition := range definitions {
		if definition.Withdrawn {
			continue
		}
		work := worksByKey[definition.WorkKey]
		if work == nil {
			work = &CatalogWork{
				Key:              definition.WorkKey,
				Title:            definition.WorkTitle,
				Author:           definition.WorkAuthor,
				GreekTitle:       definition.GreekTitle,
				ConstituentBooks: definition.ConstituentBooks,
			}
			worksByKey[definition.WorkKey] = work
		}
		if definition.ConstituentBook == nil {
			work.WholeWork = true
		}
		unit := CatalogUnit{
			Key:           definition.Key,
			CitationStart: definition.CitationStart,
			CitationEnd:   definition.CitationEnd,
			Words:         definition.WordCount,
		}
		if definition.ConstituentBook != nil {
			unit.ConstituentBook = *definition.ConstituentBook
		}
		work.Units = append(work.Units, unit)
		work.Words += definition.WordCount
	}
	return worksByKey
}

// buildCatalogAuthors groups works by author in author-name order.
func buildCatalogAuthors(worksByKey map[string]*CatalogWork) []CatalogAuthor {
	authorsByName := map[string]*CatalogAuthor{}
	for _, work := range worksByKey {
		author := authorsByName[work.Author]
		if author == nil {
			author = &CatalogAuthor{Name: work.Author}
			authorsByName[work.Author] = author
		}
		author.Works = append(author.Works, *work)
	}
	authors := make([]CatalogAuthor, 0, len(authorsByName))
	for _, author := range authorsByName {
		sortWorks(author.Works)
		authors = append(authors, *author)
	}
	sort.Slice(authors, func(i, j int) bool { return authors[i].Name < authors[j].Name })
	return authors
}

// translationAvailable reports whether every current unit of a work carries a
// translation pin, so the author page never promises a partial translation.
func translationAvailable(definitions []Definition, workKey string) bool {
	found := false
	for _, definition := range definitions {
		if definition.Withdrawn || definition.WorkKey != workKey {
			continue
		}
		if definition.Translation == nil {
			return false
		}
		found = true
	}
	return found
}

// sortWorks orders works by title, then key, so the author page is stable.
func sortWorks(works []CatalogWork) {
	sort.Slice(works, func(i, j int) bool {
		if works[i].Title != works[j].Title {
			return works[i].Title < works[j].Title
		}
		return works[i].Key < works[j].Key
	})
}

// sortUnits orders constituent books and leaves a whole-work unit first.
func sortUnits(units []CatalogUnit) {
	sort.Slice(units, func(i, j int) bool {
		if units[i].ConstituentBook != units[j].ConstituentBook {
			return units[i].ConstituentBook < units[j].ConstituentBook
		}
		return units[i].Key < units[j].Key
	})
}

// Counts returns the running release's catalog counts.
func (c *Catalog) Counts() CatalogCounts { return c.counts }

// Authors returns the author index in name order.
func (c *Catalog) Authors() []CatalogAuthor { return c.authors }

// Author returns one author by display name.
func (c *Catalog) Author(name string) (CatalogAuthor, bool) {
	for _, author := range c.authors {
		if author.Name == name {
			return author, true
		}
	}
	return CatalogAuthor{}, false
}

// Work returns one Work by its stable key.
func (c *Catalog) Work(key string) (CatalogWork, bool) {
	work, ok := c.works[key]
	return work, ok
}
