package classicalmanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
)

// MaxSourceBytes is the hard ceiling for one pinned upstream file. It matches
// the acquisition limit; a larger file is refused before it is read.
const MaxSourceBytes int64 = 512 << 20

// Options are the reviewed inputs of one generate or verify run.
type Options struct {
	// Layers is layers.json.
	Layers []byte
	// Overrides is overrides.jsonl.
	Overrides []byte
	// Manifest is the previously committed manifest.jsonl. It is empty on a
	// first generation.
	Manifest []byte
	// UpstreamDir holds the pinned upstream files by file name. It may be
	// empty in verify mode, where the committed manifest supplies pins and
	// word counts for unchanged entries.
	UpstreamDir string
	// AllowMissingUpstream lets verify mode reproduce the committed manifest
	// without the real files. CI runs this mode; generation always requires
	// them.
	AllowMissingUpstream bool
}

// Result is the deterministic output of one run.
type Result struct {
	Manifest    []byte
	Notes       string
	Definitions []classical.Definition
}

// Generate builds the manifest from reviewed inputs and checks every existing
// entry. New candidates that fail a check are skipped quietly; an existing
// entry that fails stops the run.
func Generate(options Options) (Result, error) {
	layers, err := classical.ParseLayers(options.Layers)
	if err != nil {
		return Result{}, err
	}
	overrides, err := classical.ParseOverrides(options.Overrides)
	if err != nil {
		return Result{}, err
	}
	previous, err := parsePrevious(options.Manifest)
	if err != nil {
		return Result{}, err
	}
	previousByKey := make(map[string]classical.Definition, len(previous))
	for _, definition := range previous {
		if _, duplicate := previousByKey[definition.Key]; duplicate {
			return Result{}, fmt.Errorf("classical: duplicate committed definition %q", definition.Key)
		}
		previousByKey[definition.Key] = definition
	}
	workOverrides, unitOverrides, err := indexOverrides(overrides)
	if err != nil {
		return Result{}, err
	}

	builder := &builder{
		options:         options,
		layers:          layers,
		workOverrides:   workOverrides,
		unitOverrides:   unitOverrides,
		previousByKey:   previousByKey,
		generated:       map[string]classical.Definition{},
		matchedUnitKeys: map[string]bool{},
	}
	if err := builder.build(); err != nil {
		return Result{}, err
	}
	if err := builder.finish(); err != nil {
		return Result{}, err
	}
	manifest, err := classical.RenderDefinitions(builder.definitions)
	if err != nil {
		return Result{}, err
	}
	return Result{Manifest: manifest, Notes: builder.notes.String(), Definitions: builder.definitions}, nil
}

// parsePrevious reads an optional committed manifest.
func parsePrevious(data []byte) ([]classical.Definition, error) {
	if len(data) == 0 {
		return nil, nil
	}
	return classical.ParseDefinitions(data)
}

// indexOverrides validates the hand-maintained override file and indexes it by
// Work key and Reading-unit key.
func indexOverrides(overrides []classical.Override) (map[string]classical.Override, map[string]classical.Override, error) {
	works := map[string]classical.Override{}
	units := map[string]classical.Override{}
	for _, override := range overrides {
		if override.IsWork() {
			if _, duplicate := works[override.WorkKey]; duplicate {
				return nil, nil, fmt.Errorf("classical: duplicate work override %q", override.WorkKey)
			}
			works[override.WorkKey] = override
			continue
		}
		if _, duplicate := units[override.Key]; duplicate {
			return nil, nil, fmt.Errorf("classical: duplicate unit override %q", override.Key)
		}
		units[override.Key] = override
	}
	return works, units, nil
}

// builder accumulates one run's definitions and report.
type builder struct {
	options         Options
	layers          classical.Layers
	workOverrides   map[string]classical.Override
	unitOverrides   map[string]classical.Override
	previousByKey   map[string]classical.Definition
	generated       map[string]classical.Definition
	matchedUnitKeys map[string]bool
	definitions     []classical.Definition
	notes           strings.Builder
}

// build generates every work override in stable order.
func (b *builder) build() error {
	workKeys := make([]string, 0, len(b.workOverrides))
	for key := range b.workOverrides {
		workKeys = append(workKeys, key)
	}
	sort.Strings(workKeys)
	fmt.Fprintf(&b.notes, "# Classical manifest notes\n\n## Candidate works\n")
	for _, workKey := range workKeys {
		if err := b.buildWork(b.workOverrides[workKey]); err != nil {
			return err
		}
	}
	return nil
}

// plannedUnit is one proposed Reading unit of a work.
type plannedUnit struct {
	book      int
	key       string
	start     string
	end       string
	wordCount int
	total     int
}

// buildWork generates one work's selected Reading units. It returns an error
// only when a candidate that already exists in the committed manifest fails a
// check; a new candidate that fails is recorded in the notes and skipped.
func (b *builder) buildWork(override classical.Override) error {
	sourceLayer, translationLayer, hasTranslationLayer, err := b.resolveWorkLayers(override)
	if err != nil {
		return err
	}

	sourceData, sourcePin, sourceAvailable, sourceErr := b.loadSource(override.WorkKey, sourceLayer, *override.Source)
	if sourceErr != nil {
		return b.candidateFailure(override.WorkKey, "", sourceErr)
	}

	units, total, coverage, err := b.planUnits(override, sourceData, sourceAvailable)
	if err != nil {
		return b.candidateFailure(override.WorkKey, "", err)
	}
	for index := range units {
		units[index].total = total
	}
	b.writeWorkNotes(override, units, total, sourceAvailable, coverage)

	selected, err := selectUnits(units, override.Books, override.Split)
	if err != nil {
		return b.candidateFailure(override.WorkKey, "", err)
	}
	return b.buildSelectedUnits(override, selected, sourcePin, sourceData, sourceAvailable, translationLayer, hasTranslationLayer)
}

// resolveWorkLayers validates a work's curatorial fields, source format and
// layer references.
func (b *builder) resolveWorkLayers(override classical.Override) (classical.Layer, classical.Layer, bool, error) {
	if override.WorkTitle == "" || override.WorkAuthor == "" || override.EditionLabel == "" {
		return classical.Layer{}, classical.Layer{}, false, fmt.Errorf("classical: work override %q needs workTitle, workAuthor and editionLabel", override.WorkKey)
	}
	if override.Source.Format != classical.SourceFormatOGACoNLLU {
		return classical.Layer{}, classical.Layer{}, false, fmt.Errorf("classical: work override %q uses unsupported source format %q", override.WorkKey, override.Source.Format)
	}
	sourceLayer, ok := b.layers.Layer(override.Source.Layer)
	if !ok {
		return classical.Layer{}, classical.Layer{}, false, fmt.Errorf("classical: work override %q names unknown source layer %q", override.WorkKey, override.Source.Layer)
	}
	if override.Translation == nil {
		return sourceLayer, classical.Layer{}, false, nil
	}
	translationLayer, ok := b.layers.Layer(override.Translation.Layer)
	if !ok {
		return classical.Layer{}, classical.Layer{}, false, fmt.Errorf("classical: work override %q names unknown translation layer %q", override.WorkKey, override.Translation.Layer)
	}
	return sourceLayer, translationLayer, true, nil
}

// writeWorkNotes records the proposed split, crossings and source availability.
func (b *builder) writeWorkNotes(override classical.Override, units []plannedUnit, total int, sourceAvailable bool, coverage *ancientgreek.CoverageReport) {
	fmt.Fprintf(&b.notes, "\n### %s (%s, %s)\n", override.WorkKey, override.WorkTitle, override.WorkAuthor)
	fmt.Fprintf(&b.notes, "- Split: %s, %d constituent unit(s)\n", override.Split, total)
	if sourceAvailable {
		fmt.Fprintf(&b.notes, "- Proposed units: %s\n", describeUnits(units))
	} else {
		fmt.Fprintf(&b.notes, "- Source files were not present; the committed pins and word counts were retained.\n")
	}
	if coverage == nil {
		return
	}
	if len(coverage.Crossings) == 0 {
		fmt.Fprintf(&b.notes, "- Crossing sentences: none\n")
		return
	}
	for _, crossing := range coverage.Crossings {
		fmt.Fprintf(&b.notes, "- Crossing sentence in %s: %s\n", crossing.UnitKey, strings.Join(crossing.Citations, ", "))
	}
}

// buildSelectedUnits builds and checks the units a Work override selects.
func (b *builder) buildSelectedUnits(override classical.Override, selected []plannedUnit, sourcePin classical.SourcePin, sourceData []byte, sourceAvailable bool, translationLayer classical.Layer, hasTranslationLayer bool) error {
	for _, unit := range selected {
		unitOverride, hasUnitOverride := b.unitOverrides[unit.key]
		if hasUnitOverride {
			b.matchedUnitKeys[unit.key] = true
		}
		previous, hadPrevious := b.previousByKey[unit.key]
		definition, err := b.buildDefinition(override, unitOverride, unit, sourcePin, sourceData, sourceAvailable, previous, hadPrevious, translationLayer, hasTranslationLayer)
		if err != nil {
			return b.candidateFailure(override.WorkKey, unit.key, err)
		}
		if err := checkRevision(definition, previous, hadPrevious); err != nil {
			return b.candidateFailure(override.WorkKey, unit.key, err)
		}
		if hadPrevious {
			if err := checkIdentity(previous, definition); err != nil {
				return b.candidateFailure(override.WorkKey, unit.key, err)
			}
		}
		b.generated[definition.Key] = definition
		fmt.Fprintf(&b.notes, "- %s: %s-%s, %d words", definition.Key, definition.CitationStart, definition.CitationEnd, definition.WordCount)
		if definition.Translation != nil {
			fmt.Fprintf(&b.notes, ", translation available")
		} else {
			fmt.Fprintf(&b.notes, ", no translation")
		}
		fmt.Fprintf(&b.notes, "\n")
	}
	return nil
}

// loadSource resolves one work file. When verify mode is allowed to run
// without upstream files, it returns the committed pin and no bytes.
func (b *builder) loadSource(workKey string, layer classical.Layer, spec classical.SourceSpec) ([]byte, classical.SourcePin, bool, error) {
	previousPin, hasPreviousPin, err := b.previousSourcePin(workKey)
	if err != nil {
		return nil, classical.SourcePin{}, false, err
	}
	path, pathErr := resolveUpstream(b.options.UpstreamDir, spec.FileName)
	if pathErr == nil {
		data, size, digest, readErr := readUpstream(path)
		if readErr == nil {
			if err := verifyLayerPin(layer, spec.FileName, size, digest); err != nil {
				return nil, classical.SourcePin{}, false, err
			}
			return data, classical.SourcePin{Layer: spec.Layer, FileName: spec.FileName, Size: size, SHA256: digest, Format: spec.Format}, true, nil
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return nil, classical.SourcePin{}, false, readErr
		}
	}
	if !b.options.AllowMissingUpstream || !hasPreviousPin {
		if pathErr != nil {
			return nil, classical.SourcePin{}, false, pathErr
		}
		return nil, classical.SourcePin{}, false, fmt.Errorf("classical: source file %q for %s is missing", spec.FileName, workKey)
	}
	if previousPin.Layer != spec.Layer || previousPin.FileName != spec.FileName || previousPin.Format != spec.Format {
		return nil, classical.SourcePin{}, false, fmt.Errorf("classical: source pin changed for %s; run generate with the upstream files", workKey)
	}
	return nil, previousPin, false, nil
}

// previousSourcePin returns the shared source pin committed for one Work.
func (b *builder) previousSourcePin(workKey string) (classical.SourcePin, bool, error) {
	var pin classical.SourcePin
	found := false
	keys := make([]string, 0, len(b.previousByKey))
	for key := range b.previousByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		definition := b.previousByKey[key]
		if definition.WorkKey != workKey {
			continue
		}
		if !found {
			pin = definition.Source
			found = true
			continue
		}
		if !sameSourceIdentity(pin, definition.Source) || pin.Size != definition.Source.Size || pin.SHA256 != definition.Source.SHA256 {
			return classical.SourcePin{}, false, fmt.Errorf("classical: committed definitions of %s disagree on their source pin", workKey)
		}
	}
	return pin, found, nil
}

// planUnits derives or reads the whole set of units for a work, checks that
// every sentence belongs to exactly one, and returns the selected unit count.
func (b *builder) planUnits(override classical.Override, sourceData []byte, sourceAvailable bool) ([]plannedUnit, int, *ancientgreek.CoverageReport, error) {
	if len(override.Units) > 0 {
		units, total, err := explicitUnits(override)
		if err != nil {
			return nil, 0, nil, err
		}
		if !sourceAvailable {
			return units, total, nil, nil
		}
		coverage, err := ancientgreek.CheckUnitCoverage(sourceData, unitRanges(units))
		if err != nil {
			return nil, 0, nil, err
		}
		return units, total, &coverage, nil
	}

	if sourceAvailable {
		proposed, err := ancientgreek.ProposeUnits(sourceData)
		if err != nil {
			return nil, 0, nil, err
		}
		units, total, err := b.proposedUnits(override, proposed)
		if err != nil {
			return nil, 0, nil, err
		}
		coverage, err := ancientgreek.CheckUnitCoverage(sourceData, unitRanges(units))
		if err != nil {
			return nil, 0, nil, err
		}
		return units, total, &coverage, nil
	}

	units, total, err := b.committedUnits(override)
	if err != nil {
		return nil, 0, nil, err
	}
	return units, total, nil, nil
}

// proposedUnits converts the adapter's proposed ranges into units, either one
// per constituent book or one whole-work unit.
func (b *builder) proposedUnits(override classical.Override, proposed []ancientgreek.UnitRange) ([]plannedUnit, int, error) {
	if len(proposed) == 0 {
		return nil, 0, fmt.Errorf("classical: source for %s proposes no units", override.WorkKey)
	}
	if override.Split == classical.SplitWholeWork {
		unit := plannedUnit{key: workUnitKey(override.WorkKey, 0), start: proposed[0].Start, end: proposed[len(proposed)-1].End}
		return []plannedUnit{unit}, 1, nil
	}
	units := make([]plannedUnit, 0, len(proposed))
	for _, rangeValue := range proposed {
		book, err := firstCitationNumber(rangeValue.Start)
		if err != nil {
			return nil, 0, err
		}
		units = append(units, plannedUnit{book: book, key: workUnitKey(override.WorkKey, book), start: rangeValue.Start, end: rangeValue.End})
	}
	return units, len(units), nil
}

// committedUnits reconstructs units from the committed manifest when verify
// mode has no upstream files. A selected unit that was never generated needs
// the source file, so verify fails until the maintainer regenerates.
func (b *builder) committedUnits(override classical.Override) ([]plannedUnit, int, error) {
	definitions := make([]classical.Definition, 0)
	for _, definition := range b.previousByKey {
		if definition.WorkKey == override.WorkKey {
			definitions = append(definitions, definition)
		}
	}
	if len(definitions) == 0 {
		return nil, 0, fmt.Errorf("classical: new work %s needs the upstream source file", override.WorkKey)
	}
	sort.Slice(definitions, func(i, j int) bool {
		left, right := definitions[i].ConstituentBook, definitions[j].ConstituentBook
		if left == nil || right == nil {
			return definitions[i].Key < definitions[j].Key
		}
		return *left < *right
	})
	total := definitions[0].ConstituentBooks
	units := make([]plannedUnit, 0, len(definitions))
	for _, definition := range definitions {
		if definition.ConstituentBooks != total {
			return nil, 0, fmt.Errorf("classical: committed definitions of %s disagree on their constituent book count", override.WorkKey)
		}
		book := 0
		if definition.ConstituentBook != nil {
			book = *definition.ConstituentBook
		}
		units = append(units, plannedUnit{book: book, key: definition.Key, start: definition.CitationStart, end: definition.CitationEnd, wordCount: definition.WordCount})
	}
	selected, err := selectUnits(units, override.Books, override.Split)
	if err != nil {
		return nil, 0, err
	}
	return selected, total, nil
}

// explicitUnits validates a hand-reviewed split.
func explicitUnits(override classical.Override) ([]plannedUnit, int, error) {
	if override.Split == classical.SplitWholeWork && len(override.Units) != 1 {
		return nil, 0, fmt.Errorf("classical: whole-work override %q needs one unit", override.WorkKey)
	}
	units := make([]plannedUnit, 0, len(override.Units))
	seenBooks := map[int]struct{}{}
	for _, spec := range override.Units {
		unit, err := explicitUnit(override, spec, seenBooks)
		if err != nil {
			return nil, 0, err
		}
		units = append(units, unit)
	}
	sort.Slice(units, func(i, j int) bool { return units[i].book < units[j].book })
	if err := checkExplicitOrder(units, override.WorkKey); err != nil {
		return nil, 0, err
	}
	return units, len(units), nil
}

// explicitUnit validates one hand-reviewed range.
func explicitUnit(override classical.Override, spec classical.UnitSpec, seenBooks map[int]struct{}) (plannedUnit, error) {
	book := spec.Book
	if override.Split == classical.SplitWholeWork {
		book = 0
	}
	if _, duplicate := seenBooks[book]; duplicate {
		return plannedUnit{}, fmt.Errorf("classical: work override %q repeats book %d", override.WorkKey, book)
	}
	seenBooks[book] = struct{}{}
	if compareCitations(spec.CitationStart, spec.CitationEnd) > 0 {
		return plannedUnit{}, fmt.Errorf("classical: work override %q has a backwards citation range", override.WorkKey)
	}
	if override.Split == classical.SplitConstituentBooks {
		if err := checkConstituentUnitRange(spec, override.WorkKey); err != nil {
			return plannedUnit{}, err
		}
	}
	return plannedUnit{book: book, key: workUnitKey(override.WorkKey, book), start: spec.CitationStart, end: spec.CitationEnd}, nil
}

// checkExplicitOrder refuses overlapping ranges.
func checkExplicitOrder(units []plannedUnit, workKey string) error {
	for index := 1; index < len(units); index++ {
		if compareCitations(units[index].start, units[index-1].end) <= 0 {
			return fmt.Errorf("classical: work override %q has overlapping unit ranges", workKey)
		}
	}
	return nil
}

// selectUnits applies Books to the proposed units.
// checkConstituentUnitRange keeps an explicit unit inside one constituent
// book; the split may not hand-pick a passage or merge two books.
func checkConstituentUnitRange(spec classical.UnitSpec, workKey string) error {
	startBook, err := firstCitationNumber(spec.CitationStart)
	if err != nil {
		return err
	}
	endBook, err := firstCitationNumber(spec.CitationEnd)
	if err != nil {
		return err
	}
	if startBook != spec.Book || endBook != spec.Book {
		return fmt.Errorf("classical: work override %q unit %d spans or misses its constituent book", workKey, spec.Book)
	}
	return nil
}

func selectUnits(units []plannedUnit, books []int, split string) ([]plannedUnit, error) {
	if len(books) == 0 {
		if split == classical.SplitWholeWork && len(units) != 1 {
			return nil, errors.New("classical: a whole-work override has one unit")
		}
		return units, nil
	}
	byBook := make(map[int]plannedUnit, len(units))
	for _, unit := range units {
		byBook[unit.book] = unit
	}
	selected := make([]plannedUnit, 0, len(books))
	seen := map[int]bool{}
	for _, book := range books {
		if seen[book] {
			return nil, fmt.Errorf("classical: book %d is selected twice", book)
		}
		seen[book] = true
		unit, ok := byBook[book]
		if !ok {
			return nil, fmt.Errorf("classical: selected book %d has no citation range", book)
		}
		selected = append(selected, unit)
	}
	return selected, nil
}

// buildDefinition assembles one definition from the work and unit overrides.
func (b *builder) buildDefinition(override, unitOverride classical.Override, unit plannedUnit, sourcePin classical.SourcePin, sourceData []byte, sourceAvailable bool, previous classical.Definition, hadPrevious bool, translationLayer classical.Layer, hasTranslationLayer bool) (classical.Definition, error) {
	definition := baseDefinition(override, unitOverride, unit, sourcePin, previous, hadPrevious)
	if err := checkUnitDeparture(override, unitOverride, unit.key); err != nil {
		return classical.Definition{}, err
	}
	if err := b.checkUpstreamDeparture(definition); err != nil {
		return classical.Definition{}, err
	}
	if err := applyWordCount(&definition, unit, sourceData, sourceAvailable, previous, hadPrevious); err != nil {
		return classical.Definition{}, err
	}
	if err := b.applyTranslation(&definition, override, unit, translationLayer, hasTranslationLayer, previous, hadPrevious); err != nil {
		return classical.Definition{}, err
	}
	if err := definition.Validate(); err != nil {
		return classical.Definition{}, err
	}
	return definition, nil
}

// baseDefinition applies the work and unit overrides to the committed values.
func baseDefinition(override, unitOverride classical.Override, unit plannedUnit, sourcePin classical.SourcePin, previous classical.Definition, hadPrevious bool) classical.Definition {
	definition := classical.Definition{
		Key:                   unit.key,
		WorkKey:               override.WorkKey,
		WorkTitle:             firstNonEmpty(unitOverride.WorkTitle, override.WorkTitle, previous.WorkTitle),
		WorkAuthor:            firstNonEmpty(unitOverride.WorkAuthor, override.WorkAuthor, previous.WorkAuthor),
		GreekTitle:            firstNonEmpty(unitOverride.GreekTitle, override.GreekTitle, previous.GreekTitle),
		EditionKey:            override.EditionKey,
		EditionLabel:          firstNonEmpty(unitOverride.EditionLabel, override.EditionLabel, previous.EditionLabel),
		Language:              classical.Language,
		ConstituentBooks:      unit.total,
		CitationStart:         unit.start,
		CitationEnd:           unit.end,
		WordCount:             unit.wordCount,
		Source:                sourcePin,
		InterpretationVersion: firstNonEmpty(defaultInterpretationVersion(override.Source.Format), previous.InterpretationVersion),
		Revision:              effectiveRevision(unitOverride, override, previous, hadPrevious),
		Note:                  firstNonEmpty(unitOverride.Note, override.Note, previous.Note),
	}
	if unit.book != 0 {
		book := unit.book
		definition.ConstituentBook = &book
	}
	return definition
}

// applyWordCount reads the generated word count from the source, or retains the
// committed value when verify mode has no bytes.
func applyWordCount(definition *classical.Definition, unit plannedUnit, sourceData []byte, sourceAvailable bool, previous classical.Definition, hadPrevious bool) error {
	if sourceAvailable {
		words, err := ancientgreek.UnitWordCount(sourceData, unit.start, unit.end)
		if err != nil {
			return err
		}
		definition.WordCount = words
	}
	if definition.WordCount > 0 {
		return nil
	}
	if hadPrevious {
		definition.WordCount = previous.WordCount
		return nil
	}
	return fmt.Errorf("classical: %s has no generated word count", unit.key)
}

// applyTranslation verifies a declared translation pin and aligns it to the
// unit range, or retains the committed pin when no file is available.
func (b *builder) applyTranslation(definition *classical.Definition, override classical.Override, unit plannedUnit, layer classical.Layer, hasLayer bool, previous classical.Definition, hadPrevious bool) error {
	if override.Translation != nil {
		translation, note, err := b.loadTranslation(override, unit, layer, hasLayer, previous, hadPrevious)
		if err != nil {
			return err
		}
		if note != "" {
			fmt.Fprintf(&b.notes, "- Translation: %s\n", note)
		}
		definition.Translation = translation
	} else {
		definition.Translation = nil
	}
	if definition.Translation != nil {
		definition.Translation.CitationStart = unit.start
		definition.Translation.CitationEnd = unit.end
	}
	return nil
}

// loadTranslation verifies one translation file over the unit range, reusing
// the committed pin when verify mode has no file. A missing or misaligned
// section refuses the candidate.
func (b *builder) loadTranslation(override classical.Override, unit plannedUnit, layer classical.Layer, hasLayer bool, previous classical.Definition, hadPrevious bool) (*classical.TranslationPin, string, error) {
	if !hasLayer {
		return nil, "", fmt.Errorf("classical: translation layer for %s is not admissible", unit.key)
	}
	spec := override.Translation
	pin, note, found, err := b.readTranslationFile(layer, spec, unit)
	if err != nil {
		return nil, "", err
	}
	if found {
		return pin, note, nil
	}
	if !b.options.AllowMissingUpstream || !hadPrevious || previous.Translation == nil {
		return nil, "", fmt.Errorf("classical: translation file %q is missing", spec.FileName)
	}
	if previous.Translation.Layer != spec.Layer || previous.Translation.FileName != spec.FileName {
		return nil, "", fmt.Errorf("classical: translation pin changed for %s; run generate with the upstream files", unit.key)
	}
	committed := *previous.Translation
	committed.CitationStart = unit.start
	committed.CitationEnd = unit.end
	return &committed, "committed pin retained", nil
}

// readTranslationFile verifies a translation file when it is present.
func (b *builder) readTranslationFile(layer classical.Layer, spec *classical.TranslationSpec, unit plannedUnit) (*classical.TranslationPin, string, bool, error) {
	path, pathErr := resolveUpstream(b.options.UpstreamDir, spec.FileName)
	if pathErr != nil {
		return nil, "", false, nil //nolint:nilerr // no upstream directory is not an error in verify mode.
	}
	data, size, digest, readErr := readUpstream(path)
	if errors.Is(readErr, os.ErrNotExist) {
		return nil, "", false, nil
	}
	if readErr != nil {
		return nil, "", false, readErr
	}
	if err := verifyLayerPin(layer, spec.FileName, size, digest); err != nil {
		return nil, "", false, err
	}
	if err := requirePassageAttribution(layer, spec.FileName); err != nil {
		return nil, "", false, err
	}
	if _, err := ancientgreek.InspectTranslation(data, unit.start, unit.end); err != nil {
		return nil, "", false, err
	}
	pin := &classical.TranslationPin{
		Layer: spec.Layer, FileName: spec.FileName, Size: size, SHA256: digest,
		CitationStart: unit.start, CitationEnd: unit.end,
	}
	return pin, fmt.Sprintf("%s aligned over %s-%s", spec.FileName, unit.start, unit.end), true, nil
}

// checkUnitDeparture requires a note when a unit override departs from its
// work's curatorial values.
func checkUnitDeparture(work, unit classical.Override, key string) error {
	if unit.Note != "" {
		return nil
	}
	switch {
	case unit.WorkTitle != "" && unit.WorkTitle != work.WorkTitle:
		return fmt.Errorf("classical: unit override %q departs from its Work title without a note", key)
	case unit.WorkAuthor != "" && unit.WorkAuthor != work.WorkAuthor:
		return fmt.Errorf("classical: unit override %q departs from its Work author without a note", key)
	case unit.GreekTitle != "" && unit.GreekTitle != work.GreekTitle:
		return fmt.Errorf("classical: unit override %q departs from its Work Greek title without a note", key)
	case unit.EditionLabel != "" && unit.EditionLabel != work.EditionLabel:
		return fmt.Errorf("classical: unit override %q departs from its Work Edition label without a note", key)
	}
	return nil
}

// checkUpstreamDeparture requires a note when a curatorial field differs from
// the upstream metadata recorded in layers.json.
func (b *builder) checkUpstreamDeparture(definition classical.Definition) error {
	if definition.Note != "" {
		return nil
	}
	layer, ok := b.layers.Layer(definition.Source.Layer)
	if !ok {
		return fmt.Errorf("classical: definition %s names unknown layer %q", definition.Key, definition.Source.Layer)
	}
	file, ok := layer.File(definition.Source.FileName)
	if !ok {
		return nil
	}
	switch {
	case file.WorkTitle != "" && file.WorkTitle != definition.WorkTitle:
		return fmt.Errorf("classical: definition %s departs from the upstream Work title without a note", definition.Key)
	case file.WorkAuthor != "" && file.WorkAuthor != definition.WorkAuthor:
		return fmt.Errorf("classical: definition %s departs from the upstream Work author without a note", definition.Key)
	case file.GreekTitle != "" && file.GreekTitle != definition.GreekTitle:
		return fmt.Errorf("classical: definition %s departs from the upstream Greek title without a note", definition.Key)
	case file.EditionLabel != "" && file.EditionLabel != definition.EditionLabel:
		return fmt.Errorf("classical: definition %s departs from the upstream Edition label without a note", definition.Key)
	}
	return nil
}

// candidateFailure stops on an existing entry and records a new one as
// rejected.
func (b *builder) candidateFailure(workKey, key string, err error) error {
	existing := false
	if key != "" {
		_, existing = b.previousByKey[key]
	} else {
		for _, definition := range b.previousByKey {
			if definition.WorkKey == workKey {
				existing = true
				break
			}
		}
	}
	if existing {
		return fmt.Errorf("classical: existing %s failed a check: %w", displayKey(workKey, key), err)
	}
	fmt.Fprintf(&b.notes, "- Rejected new candidate %s: %s\n", displayKey(workKey, key), err)
	return nil
}

// finish adds withdrawn tombstones for removed entries, rejects unknown unit
// overrides, sorts by Key and validates the result.
func (b *builder) finish() error {
	if err := b.checkUnitOverrides(); err != nil {
		return err
	}
	b.addTombstones()
	if err := b.finalizeDefinitions(); err != nil {
		return err
	}
	return b.checkRetainedTranslations()
}

// checkUnitOverrides refuses a unit override that matched no generated unit.
func (b *builder) checkUnitOverrides() error {
	for key := range b.unitOverrides {
		if !b.matchedUnitKeys[key] && b.generated[key].Key == "" {
			return fmt.Errorf("classical: unit override %q does not match a generated Reading unit", key)
		}
	}
	return nil
}

// addTombstones withdraws every committed entry the run did not regenerate.
func (b *builder) addTombstones() {
	keys := make([]string, 0, len(b.previousByKey))
	for key := range b.previousByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, generated := b.generated[key]; generated {
			continue
		}
		tombstone := b.previousByKey[key]
		tombstone.Withdrawn = true
		b.generated[key] = tombstone
	}
}

// finalizeDefinitions validates, pins and sorts the generated entries.
func (b *builder) finalizeDefinitions() error {
	b.definitions = make([]classical.Definition, 0, len(b.generated))
	for _, definition := range b.generated {
		if err := definition.Validate(); err != nil {
			return fmt.Errorf("classical: generated definition %q is invalid: %w", definition.Key, err)
		}
		if !definition.Withdrawn {
			if err := b.validateLayerPins(definition); err != nil {
				return err
			}
		}
		b.definitions = append(b.definitions, definition)
	}
	sort.Slice(b.definitions, func(i, j int) bool { return b.definitions[i].Key < b.definitions[j].Key })
	return nil
}

// validateLayerPins checks the committed definition against the hand-maintained
// byte pins in layers.json. It catches a hand edit even when verify mode has no
// upstream file.
func (b *builder) validateLayerPins(definition classical.Definition) error {
	if err := b.validateLayerPin(definition.Source.Layer, definition.Source.FileName, definition.Source.Size, definition.Source.SHA256); err != nil {
		return err
	}
	if definition.Translation == nil {
		return nil
	}
	if err := b.validateLayerPin(definition.Translation.Layer, definition.Translation.FileName, definition.Translation.Size, definition.Translation.SHA256); err != nil {
		return err
	}
	layer, _ := b.layers.Layer(definition.Translation.Layer)
	return requirePassageAttribution(layer, definition.Translation.FileName)
}

// requirePassageAttribution refuses a translation file that cannot credit the
// passage context a card freezes from it.
func requirePassageAttribution(layer classical.Layer, fileName string) error {
	file, ok := layer.File(fileName)
	if !ok || !file.HasPassageAttribution() {
		return fmt.Errorf("classical: translation %q needs a passageLabel and deckNotice in layers.json", fileName)
	}
	return nil
}

// checkRetainedTranslations keeps every translation the committed manifest
// pinned, including withdrawn and superseded pins, creditable in layers.json.
// Prepared decks freeze the pin and find their credit by it, so a translation
// file entry is never removed once it has shipped.
func (b *builder) checkRetainedTranslations() error {
	keys := make([]string, 0, len(b.previousByKey))
	for key := range b.previousByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		translation := b.previousByKey[key].Translation
		if translation == nil {
			continue
		}
		if _, ok := b.layers.PassageAttribution(translation.SHA256); !ok {
			return fmt.Errorf("classical: %s was pinned by %s and must stay in layers.json with its passage credit", translation.FileName, key)
		}
	}
	return nil
}

// validateLayerPin compares one recorded pin with layers.json. Verify mode
// relies on this when it has no upstream bytes.
func (b *builder) validateLayerPin(layerKey, fileName string, size int64, sha256 string) error {
	layer, ok := b.layers.Layer(layerKey)
	if !ok {
		return fmt.Errorf("classical: %s names unknown layer %q", fileName, layerKey)
	}
	file, ok := layer.File(fileName)
	if !ok || file.Size == 0 || file.SHA256 == "" {
		return fmt.Errorf("classical: %s has no size and SHA-256 pin in layers.json", fileName)
	}
	if file.Size != size || file.SHA256 != sha256 {
		return fmt.Errorf("classical: %s pin does not match layers.json", fileName)
	}
	return nil
}

// checkIdentity refuses a changed extent or shared identity under an existing
// key.
func checkIdentity(previous, next classical.Definition) error {
	if previous.WorkKey != next.WorkKey || previous.EditionKey != next.EditionKey {
		return fmt.Errorf("classical: %s changed its Work or Edition key; use a new Reading-unit Key", next.Key)
	}
	if previous.CitationStart != next.CitationStart || previous.CitationEnd != next.CitationEnd {
		return fmt.Errorf("classical: %s changed its citation range; use a new Reading-unit Key", next.Key)
	}
	if bookNumber(previous) != bookNumber(next) {
		return fmt.Errorf("classical: %s changed its constituent book; use a new Reading-unit Key", next.Key)
	}
	return nil
}

// checkRevision enforces monotonic revisions and a bump for content changes.
func checkRevision(next classical.Definition, previous classical.Definition, hadPrevious bool) error {
	if !hadPrevious {
		return nil
	}
	if next.Revision < previous.Revision {
		return fmt.Errorf("classical: %s revision decreased from %d to %d", next.Key, previous.Revision, next.Revision)
	}
	if contentChanged(previous, next) && next.Revision <= previous.Revision {
		return fmt.Errorf("classical: %s content changed without a revision bump", next.Key)
	}
	return nil
}

// contentChanged reports whether a source pin, translation pin or
// interpretation version differs.
func contentChanged(previous, next classical.Definition) bool {
	return !sameSourceIdentity(previous.Source, next.Source) ||
		previous.Source.Size != next.Source.Size ||
		previous.Source.SHA256 != next.Source.SHA256 ||
		!sameTranslation(previous.Translation, next.Translation) ||
		previous.InterpretationVersion != next.InterpretationVersion
}

// sameSourceIdentity compares the source pin's identity, excluding bytes.
func sameSourceIdentity(a, b classical.SourcePin) bool {
	return a.Layer == b.Layer && a.FileName == b.FileName && a.Format == b.Format
}

// sameTranslation compares two optional translation pins, including range.
func sameTranslation(a, b *classical.TranslationPin) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// bookNumber returns 0 for a whole-work unit.
func bookNumber(definition classical.Definition) int {
	if definition.ConstituentBook == nil {
		return 0
	}
	return *definition.ConstituentBook
}

// effectiveRevision resolves a unit-level revision, then a work-level one,
// then the committed revision, then 1.
func effectiveRevision(unitOverride, workOverride classical.Override, previous classical.Definition, hadPrevious bool) int {
	if unitOverride.Revision != nil {
		return *unitOverride.Revision
	}
	if workOverride.Revision != nil {
		return *workOverride.Revision
	}
	if hadPrevious {
		return previous.Revision
	}
	return 1
}

// defaultInterpretationVersion maps a source format to its adapter version.
func defaultInterpretationVersion(format string) string {
	if format == classical.SourceFormatOGACoNLLU {
		return ancientgreek.PosMappingVersion
	}
	return ""
}

// workUnitKey derives the permanent Reading-unit key from a Work key and a
// constituent book number. Zero means a whole-work unit.
func workUnitKey(workKey string, book int) string {
	if book == 0 {
		return workKey
	}
	return workKey + "-" + strconv.Itoa(book)
}

// describeUnits renders the proposed split for the maintainer report.
func describeUnits(units []plannedUnit) string {
	parts := make([]string, 0, len(units))
	for _, unit := range units {
		label := "whole work"
		if unit.book != 0 {
			label = "Book " + strconv.Itoa(unit.book)
		}
		parts = append(parts, fmt.Sprintf("%s %s-%s", label, unit.start, unit.end))
	}
	return strings.Join(parts, "; ")
}

// unitRanges maps planned units to the adapter's coverage input.
func unitRanges(units []plannedUnit) []ancientgreek.UnitRange {
	ranges := make([]ancientgreek.UnitRange, 0, len(units))
	for _, unit := range units {
		ranges = append(ranges, ancientgreek.UnitRange{Key: unit.key, Start: unit.start, End: unit.end})
	}
	return ranges
}

// firstCitationNumber returns the top-level number of a citation.
func firstCitationNumber(citation string) (int, error) {
	part, _, _ := strings.Cut(citation, ".")
	number, err := strconv.Atoi(part)
	if err != nil || number < 1 {
		return 0, fmt.Errorf("classical: citation %q has no constituent book number", citation)
	}
	return number, nil
}

// compareCitations compares two dotted citations component-wise.
func compareCitations(a, b string) int {
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	length := max(len(left), len(right))
	for index := range length {
		lv, rv := citationPart(left, index), citationPart(right, index)
		if lv != rv {
			if lv < rv {
				return -1
			}
			return 1
		}
	}
	return 0
}

// citationPart returns one parsed component, or zero for a missing or invalid
// one. Callers validate citations before comparing them.
func citationPart(parts []string, index int) int {
	if index >= len(parts) {
		return 0
	}
	number, err := strconv.Atoi(parts[index])
	if err != nil {
		return 0
	}
	return number
}

// resolveUpstream joins a pinned file name under the upstream directory and
// refuses absolute or traversing names.
func resolveUpstream(root, name string) (string, error) {
	if root == "" {
		return "", errors.New("classical: no upstream directory was supplied")
	}
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("classical: upstream file name %q is not a safe relative path", name)
	}
	return filepath.Join(root, clean), nil
}

// verifyLayerPin compares a read file with the reviewed pin in layers.json.
// Every file a definition pins must be listed with both its size and SHA-256.
func verifyLayerPin(layer classical.Layer, fileName string, size int64, digest string) error {
	file, ok := layer.File(fileName)
	if !ok {
		return fmt.Errorf("classical: upstream file %q is not listed in layers.json", fileName)
	}
	if file.Size == 0 || file.SHA256 == "" {
		return fmt.Errorf("classical: upstream file %q has no size and SHA-256 pin", fileName)
	}
	if file.Size != size || file.SHA256 != digest {
		return fmt.Errorf("classical: upstream pin mismatch for %q", fileName)
	}
	return nil
}

// readUpstream reads one file with the 512 MiB ceiling and returns its size and
// SHA-256. The size check happens before any read.
func readUpstream(path string) ([]byte, int64, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, "", err
	}
	if info.Size() > MaxSourceBytes {
		return nil, 0, "", fmt.Errorf("classical: upstream file %q is larger than %d bytes", filepath.Base(path), MaxSourceBytes)
	}
	data, err := os.ReadFile(path) //nolint:gosec // the path is resolved under the maintainer-supplied upstream root.
	if err != nil {
		return nil, 0, "", err
	}
	sum := sha256.Sum256(data)
	return data, int64(len(data)), hex.EncodeToString(sum[:]), nil
}

// firstNonEmpty returns the first non-empty value.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// displayKey names a candidate in the notes without printing source text.
func displayKey(workKey, key string) string {
	if key != "" {
		return key
	}
	return workKey
}
