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
	// Gates is the per-Work gate report of every Latin Work, in Work-key
	// order. It is empty for a manifest of Greek Works alone.
	Gates []WorkGateReport
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
	return Result{Manifest: manifest, Notes: builder.notes.String(), Definitions: builder.definitions, Gates: builder.gates}, nil
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
	gates           []WorkGateReport
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
	key       string
	start     string
	end       string
	wordCount int
	total     int
	// contents lists the constituent books of a whole-work unit whose Work
	// names books.
	contents []classical.ConstituentBookSpan
	// opening and translation are carried over from the committed
	// definitions a new unit replaces, for runs without the upstream bytes.
	opening     string
	translation *classical.TranslationPin
}

// buildWork generates one work's selected Reading units. It returns an error
// only when a candidate that already exists in the committed manifest fails a
// check; a new candidate that fails is recorded in the notes and skipped.
func (b *builder) buildWork(override classical.Override) error {
	sourceLayer, translationLayer, hasTranslationLayer, err := b.resolveWorkLayers(override)
	if err != nil {
		return err
	}
	if err := checkSourceContentLayers(override, sourceLayer, translationLayer, hasTranslationLayer); err != nil {
		return b.candidateFailure(override.WorkKey, "", err)
	}

	source, sourcePin, sourceAvailable, sourceErr := b.loadSource(override, sourceLayer)
	if sourceErr != nil {
		return b.candidateFailure(override.WorkKey, "", sourceErr)
	}

	units, total, coverage, err := b.planUnits(override, source, sourceAvailable)
	if errors.Is(err, errContentsOverride) {
		return err
	}
	if err != nil {
		return b.candidateFailure(override.WorkKey, "", err)
	}
	for index := range units {
		units[index].total = total
	}
	b.writeWorkNotes(override, units, total, sourceAvailable, coverage)
	return b.buildSelectedUnits(override, units, sourcePin, source, sourceAvailable, translationLayer, hasTranslationLayer)
}

// resolveWorkLayers validates a work's curatorial fields, source format and
// layer references.
func (b *builder) resolveWorkLayers(override classical.Override) (classical.Layer, classical.Layer, bool, error) {
	if override.WorkTitle == "" || override.WorkAuthor == "" || override.EditionLabel == "" {
		return classical.Layer{}, classical.Layer{}, false, fmt.Errorf("classical: work override %q needs workTitle, workAuthor and editionLabel", override.WorkKey)
	}
	if override.Source.Format != classical.SourceFormatOGACoNLLU && override.Source.Format != classical.SourceFormatOLACoNLLU {
		return classical.Layer{}, classical.Layer{}, false, fmt.Errorf("classical: work override %q uses unsupported source format %q", override.WorkKey, override.Source.Format)
	}
	if override.Source.Format == classical.SourceFormatOLACoNLLU && override.Language != classical.LanguageLatin {
		return classical.Layer{}, classical.Layer{}, false, fmt.Errorf("classical: work override %q reads an %s source and needs language %q", override.WorkKey, classical.SourceFormatOLACoNLLU, classical.LanguageLatin)
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

// checkSourceContentLayers refuses a work whose source or translation names a
// layer of another licence class. Another class may legitimately carry an
// unclear or non-redistributable restriction, so it is never admissible as a
// work's source or translation.
func checkSourceContentLayers(override classical.Override, sourceLayer, translationLayer classical.Layer, hasTranslation bool) error {
	if override.Source.Format == classical.SourceFormatOLACoNLLU {
		// OLA's annotations are an Analysis resource and its text Source
		// content (ADR 0132); the one archive layer may carry either class.
		if sourceLayer.FetchKind != classical.FetchKindArchiveRange {
			return fmt.Errorf("classical: source layer %q for %s is not an archive-range layer", sourceLayer.Key, override.WorkKey)
		}
		if !sourceLayer.IsSourceContent() && sourceLayer.LicenceClass != classical.LicenceClassAnalysisResource {
			return fmt.Errorf("classical: source layer %q for %s is neither Source content nor an Analysis resource", sourceLayer.Key, override.WorkKey)
		}
	} else if !sourceLayer.IsSourceContent() {
		return fmt.Errorf("classical: source layer %q for %s is not Source content", sourceLayer.Key, override.WorkKey)
	}
	if hasTranslation && !translationLayer.IsSourceContent() {
		return fmt.Errorf("classical: translation layer %q for %s is not Source content", translationLayer.Key, override.WorkKey)
	}
	return nil
}

// writeWorkNotes records the proposed split, crossings and source availability.
func (b *builder) writeWorkNotes(override classical.Override, units []plannedUnit, total int, sourceAvailable bool, coverage *ancientgreek.CoverageReport) {
	fmt.Fprintf(&b.notes, "\n### %s (%s, %s)\n", override.WorkKey, override.WorkTitle, override.WorkAuthor)
	fmt.Fprintf(&b.notes, "- Split: %s, %d constituent book(s)\n", override.Split, total)
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
func (b *builder) buildSelectedUnits(override classical.Override, selected []plannedUnit, sourcePin classical.SourcePin, source *ancientgreek.ManifestSource, sourceAvailable bool, translationLayer classical.Layer, hasTranslationLayer bool) error {
	for _, unit := range selected {
		unitOverride, hasUnitOverride := b.unitOverrides[unit.key]
		if hasUnitOverride {
			b.matchedUnitKeys[unit.key] = true
		}
		previous, hadPrevious := b.previousByKey[unit.key]
		definition, err := b.buildDefinition(override, unitOverride, unit, sourcePin, source, sourceAvailable, previous, hadPrevious, translationLayer, hasTranslationLayer)
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

// loadSource resolves one Work's source. When verify mode is allowed to run
// without upstream files, it returns the committed pin and no source.
func (b *builder) loadSource(override classical.Override, layer classical.Layer) (*ancientgreek.ManifestSource, classical.SourcePin, bool, error) {
	spec := *override.Source
	previousPin, hasPreviousPin, err := b.previousSourcePin(override.WorkKey)
	if err != nil {
		return nil, classical.SourcePin{}, false, err
	}
	var source *ancientgreek.ManifestSource
	var pin classical.SourcePin
	var found bool
	var missing error
	if spec.Format == classical.SourceFormatOLACoNLLU {
		source, pin, found, missing, err = b.readLatinSource(override, layer)
	} else {
		source, pin, found, missing, err = b.readGreekSource(override.WorkKey, layer, spec)
	}
	if err != nil {
		return nil, classical.SourcePin{}, false, err
	}
	if found {
		return source, pin, true, nil
	}
	if !b.options.AllowMissingUpstream || !hasPreviousPin {
		return nil, classical.SourcePin{}, false, missing
	}
	if previousPin.Layer != spec.Layer || previousPin.FileName != spec.FileName || previousPin.Format != spec.Format || !sameCitationMember(previousPin, spec) {
		return nil, classical.SourcePin{}, false, fmt.Errorf("classical: source pin changed for %s; run generate with the upstream files", override.WorkKey)
	}
	if isLatinWork(override) {
		b.recordUnevaluatedGates(override)
	}
	return nil, previousPin, false, nil
}

// sameCitationMember reports whether a committed pin's PAULA member is the one
// the override names. A source with no citation member always agrees.
func sameCitationMember(pin classical.SourcePin, spec classical.SourceSpec) bool {
	if spec.CitationFileName == "" {
		return len(pin.Members) == 0
	}
	return len(pin.Members) == 2 && pin.Members[1].Name == spec.CitationFileName
}

// readGreekSource reads and parses one oga_src file. A source that is not
// present is reported through missing, not as an error.
func (b *builder) readGreekSource(workKey string, layer classical.Layer, spec classical.SourceSpec) (*ancientgreek.ManifestSource, classical.SourcePin, bool, error, error) {
	path, pathErr := resolveUpstream(b.options.UpstreamDir, spec.FileName)
	if pathErr != nil {
		return nil, classical.SourcePin{}, false, pathErr, nil
	}
	data, size, digest, readErr := readUpstream(path)
	if errors.Is(readErr, os.ErrNotExist) {
		return nil, classical.SourcePin{}, false, fmt.Errorf("classical: source file %q for %s is missing", spec.FileName, workKey), nil
	}
	if readErr != nil {
		return nil, classical.SourcePin{}, false, nil, readErr
	}
	if err := verifyLayerPin(layer, spec.FileName, size, digest); err != nil {
		return nil, classical.SourcePin{}, false, nil, err
	}
	source, err := ancientgreek.ParseManifestSource(data)
	if err != nil {
		return nil, classical.SourcePin{}, false, nil, err
	}
	return source, classical.SourcePin{Layer: spec.Layer, FileName: spec.FileName, Size: size, SHA256: digest, Format: spec.Format}, true, nil, nil
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

// planUnits derives or reads the whole-work unit of a work and checks that
// every sentence belongs to it. The returned count is the number of
// constituent books the Work's citation scheme names.
func (b *builder) planUnits(override classical.Override, source *ancientgreek.ManifestSource, sourceAvailable bool) ([]plannedUnit, int, *ancientgreek.CoverageReport, error) {
	if len(override.Units) > 0 {
		return b.planExplicitUnits(override, source, sourceAvailable)
	}

	if sourceAvailable {
		proposed, err := source.ProposeUnits()
		if err != nil {
			return nil, 0, nil, err
		}
		unit, err := proposedUnit(override, source, proposed)
		if err != nil {
			return nil, 0, nil, err
		}
		units := []plannedUnit{unit}
		coverage, err := source.CheckUnitCoverage(unitRanges(units))
		if err != nil {
			return nil, 0, nil, err
		}
		return units, max(len(unit.contents), 1), &coverage, nil
	}

	unit, err := b.committedUnit(override)
	if err != nil {
		return nil, 0, nil, err
	}
	if override.Contents == classical.ContentsBooks && len(unit.contents) < 2 {
		return nil, 0, nil, fmt.Errorf("classical: work override %q lists book contents but the committed manifest has no book ranges; verify with upstream files", override.WorkKey)
	}
	return []plannedUnit{unit}, max(len(unit.contents), 1), nil, nil
}

func (b *builder) planExplicitUnits(override classical.Override, source *ancientgreek.ManifestSource, sourceAvailable bool) ([]plannedUnit, int, *ancientgreek.CoverageReport, error) {
	spec := override.Units[0] // The override validator allows at most one.
	order, err := classical.CompareCitations(spec.CitationStart, spec.CitationEnd)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("classical: work override %q: %w", override.WorkKey, err)
	}
	if order > 0 {
		return nil, 0, nil, fmt.Errorf("classical: work override %q has a backwards citation range", override.WorkKey)
	}
	unit := plannedUnit{key: workUnitKey(override.WorkKey, 0), start: spec.CitationStart, end: spec.CitationEnd}
	units := []plannedUnit{unit}
	if !sourceAvailable {
		if definition := b.previousByKey[unit.key]; len(definition.Contents) > 0 {
			return nil, 0, nil, bookRangeOverrideError(override)
		}
		return units, 1, nil, nil
	}
	coverage, err := source.CheckUnitCoverage(unitRanges(units))
	if err != nil {
		return nil, 0, nil, err
	}
	proposed, err := source.ProposeUnits()
	if err != nil {
		return nil, 0, nil, err
	}
	// A Latin Work's outer Citation level is not necessarily a book (a
	// speech's chapters are not), so only Greek refuses on its shape.
	if !isLatinWork(override) && hasBookRanges(proposed) {
		return nil, 0, nil, bookRangeOverrideError(override)
	}
	return units, 1, &coverage, nil
}

func bookRangeOverrideError(override classical.Override) error {
	return fmt.Errorf("classical: work override %q cannot replace derived book contents with a hand-reviewed range", override.WorkKey)
}

// errContentsOverride marks a work override whose contents: books disagrees
// with its source. It always fails generation, even for a new Work.
var errContentsOverride = errors.New("classical: contents override disagrees with the source")

// proposedUnit converts the adapter's proposed ranges into the one whole-work
// unit. When the work override asks for contents: books, each proposed range is a
// constituent book and is recorded, with its word count, as contents.
func proposedUnit(override classical.Override, source *ancientgreek.ManifestSource, proposed []ancientgreek.UnitRange) (plannedUnit, error) {
	unit := plannedUnit{key: workUnitKey(override.WorkKey, 0), start: proposed[0].Start, end: proposed[len(proposed)-1].End}
	hasBooks := hasBookRanges(proposed)
	switch {
	case override.Contents == classical.ContentsBooks && !hasBooks:
		return plannedUnit{}, fmt.Errorf("classical: work override %q lists book contents but its source has no multiple book ranges: %w", override.WorkKey, errContentsOverride)
	case override.Contents != classical.ContentsBooks && hasBooks && !isLatinWork(override):
		return plannedUnit{}, fmt.Errorf("classical: work override %q has a source with multiple book ranges but no contents: %q: %w", override.WorkKey, classical.ContentsBooks, errContentsOverride)
	case !hasBooks || override.Contents != classical.ContentsBooks:
		// A Latin Work's Citation scheme names books only when its override
		// says so: the outer level of a speech's is a chapter.
		return unit, nil
	}
	for _, rangeValue := range proposed {
		book, err := firstCitationNumber(rangeValue.Start)
		if err != nil {
			return plannedUnit{}, err
		}
		summary, err := source.SummarizeUnit(rangeValue.Start, rangeValue.End)
		if err != nil {
			return plannedUnit{}, err
		}
		unit.contents = append(unit.contents, classical.ConstituentBookSpan{Book: book, CitationStart: rangeValue.Start, CitationEnd: rangeValue.End, WordCount: summary.WordCount})
	}
	return unit, nil
}

// OGA sources cite multi-book Works with the book as their outer level.
// Flat citations, such as Stephanus pages, are not book numbers.
func hasBookRanges(proposed []ancientgreek.UnitRange) bool {
	return len(proposed) > 1 && strings.Contains(proposed[0].Start, ".")
}

// committedUnit reconstructs the whole-work unit from the committed manifest
// when verify mode has no upstream files. Before the whole-work key exists it
// aggregates the Work's per-book definitions of ADR 0116, so the first
// generation can derive the new unit offline-consistently.
func (b *builder) committedUnit(override classical.Override) (plannedUnit, error) {
	key := workUnitKey(override.WorkKey, 0)
	if definition, ok := b.previousByKey[key]; ok {
		return plannedUnit{key: key, start: definition.CitationStart, end: definition.CitationEnd, wordCount: definition.WordCount, contents: definition.Contents}, nil
	}
	books := make([]classical.Definition, 0)
	for _, definition := range b.previousByKey {
		if definition.WorkKey == override.WorkKey && definition.ConstituentBook != nil {
			books = append(books, definition)
		}
	}
	if len(books) == 0 {
		return plannedUnit{}, fmt.Errorf("classical: new work %s needs the upstream source file", override.WorkKey)
	}
	sort.Slice(books, func(i, j int) bool { return *books[i].ConstituentBook < *books[j].ConstituentBook })
	unit := plannedUnit{key: key, start: books[0].CitationStart, end: books[len(books)-1].CitationEnd, opening: books[0].Opening, translation: books[0].Translation}
	for _, definition := range books {
		unit.wordCount += definition.WordCount
		unit.contents = append(unit.contents, classical.ConstituentBookSpan{Book: *definition.ConstituentBook, CitationStart: definition.CitationStart, CitationEnd: definition.CitationEnd, WordCount: definition.WordCount})
	}
	return unit, nil
}

// buildDefinition assembles one definition from the work and unit overrides.
func (b *builder) buildDefinition(override, unitOverride classical.Override, unit plannedUnit, sourcePin classical.SourcePin, source *ancientgreek.ManifestSource, sourceAvailable bool, previous classical.Definition, hadPrevious bool, translationLayer classical.Layer, hasTranslationLayer bool) (classical.Definition, error) {
	definition := baseDefinition(override, unitOverride, unit, sourcePin, previous, hadPrevious)
	if err := checkUnitDeparture(override, unitOverride, unit.key); err != nil {
		return classical.Definition{}, err
	}
	if err := b.checkUpstreamDeparture(definition); err != nil {
		return classical.Definition{}, err
	}
	if err := applyGeneratedFacts(&definition, unit, source, sourceAvailable, previous, hadPrevious); err != nil {
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
		OriginalTitle:         firstNonEmpty(unitOverride.OriginalTitle, override.OriginalTitle, previous.OriginalTitle),
		EditionKey:            override.EditionKey,
		EditionLabel:          firstNonEmpty(unitOverride.EditionLabel, override.EditionLabel, previous.EditionLabel),
		Language:              firstNonEmpty(override.Language, previous.Language, classical.Language),
		ConstituentBooks:      unit.total,
		CitationStart:         unit.start,
		CitationEnd:           unit.end,
		WordCount:             unit.wordCount,
		Contents:              unit.contents,
		Source:                sourcePin,
		InterpretationVersion: firstNonEmpty(defaultInterpretationVersion(override.Source.Format), previous.InterpretationVersion),
		Revision:              effectiveRevision(unitOverride, override, previous, hadPrevious),
		Note:                  firstNonEmpty(unitOverride.Note, override.Note, previous.Note),
	}
	if definition.Language == classical.LanguageLatin && definition.OriginalTitle == definition.WorkTitle {
		// The Latin title is kept only where it differs from the English one.
		definition.OriginalTitle = ""
	}
	return definition
}

// applyGeneratedFacts reads the word count and Opening from the source in one
// pass, or retains the committed values when verify mode has no bytes.
func applyGeneratedFacts(definition *classical.Definition, unit plannedUnit, source *ancientgreek.ManifestSource, sourceAvailable bool, previous classical.Definition, hadPrevious bool) error {
	if sourceAvailable {
		summary, err := source.SummarizeUnit(unit.start, unit.end)
		if err != nil {
			return err
		}
		definition.WordCount = summary.WordCount
		definition.Opening = summary.Opening
	}
	if definition.WordCount <= 0 {
		if !hadPrevious {
			return fmt.Errorf("classical: %s has no generated word count", unit.key)
		}
		definition.WordCount = previous.WordCount
	}
	if strings.TrimSpace(definition.Opening) != "" {
		return nil
	}
	if hadPrevious {
		definition.Opening = previous.Opening
	} else {
		definition.Opening = unit.opening
	}
	if strings.TrimSpace(definition.Opening) == "" {
		return fmt.Errorf("classical: %s has no generated Opening", unit.key)
	}
	return nil
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
	retained := unit.translation
	if hadPrevious {
		retained = previous.Translation
	}
	if !b.options.AllowMissingUpstream || retained == nil {
		return nil, "", fmt.Errorf("classical: translation file %q is missing", spec.FileName)
	}
	if retained.Layer != spec.Layer || retained.FileName != spec.FileName {
		return nil, "", fmt.Errorf("classical: translation pin changed for %s; run generate with the upstream files", unit.key)
	}
	committed := *retained
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
	case unit.OriginalTitle != "" && unit.OriginalTitle != work.OriginalTitle:
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
	case upstreamOriginalTitle(file, definition.Language) != "" && upstreamOriginalTitle(file, definition.Language) != definition.OriginalTitle:
		return fmt.Errorf("classical: definition %s departs from the upstream original title without a note", definition.Key)
	case file.EditionLabel != "" && file.EditionLabel != definition.EditionLabel:
		return fmt.Errorf("classical: definition %s departs from the upstream Edition label without a note", definition.Key)
	}
	return nil
}

// upstreamOriginalTitle is a layer file's original title as a Definition of
// the language would keep it.
func upstreamOriginalTitle(file classical.LayerFile, language string) string {
	if language == classical.LanguageLatin && file.OriginalTitle == file.WorkTitle {
		return ""
	}
	return file.OriginalTitle
}

// isLatinWork reports whether a Work override reads an OLA source.
func isLatinWork(override classical.Override) bool {
	return override.Source != nil && override.Source.Format == classical.SourceFormatOLACoNLLU
}

// candidateFailure stops on an existing entry and records a new one as
// rejected. A unit with no Opening is never skipped: the generator must fail
// rather than publish a unit whose first sentence it could not derive.
func (b *builder) candidateFailure(workKey, key string, err error) error {
	var refusal *ancientgreek.ImportError
	if errors.As(err, &refusal) && refusal.Code == ancientgreek.CodeOpeningMissing {
		return fmt.Errorf("classical: %s has no Opening: %w", displayKey(workKey, key), err)
	}
	// A Work with committed definitions never fails quietly: skipping its
	// new unit would withdraw the Books its learners already own.
	existing := false
	for _, definition := range b.previousByKey {
		if definition.WorkKey == workKey {
			existing = true
			break
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
	switch format {
	case classical.SourceFormatOGACoNLLU:
		return ancientgreek.InterpretationVersionNativeAGDT
	case classical.SourceFormatOLACoNLLU:
		return classical.InterpretationVersionOLA
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
		if len(unit.contents) > 0 {
			label = fmt.Sprintf("whole work (%d books)", len(unit.contents))
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
