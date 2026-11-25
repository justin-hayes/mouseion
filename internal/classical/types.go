// Package classical holds the reviewed Classical catalog artifacts that ship
// with Mouseion: the generated Reading-unit manifest, the hand-maintained
// overrides and the upstream layer pins.
package classical

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Language is the Ancient Greek language tag, the first classical language the
// Classical catalog ships. A Definition carries its own language; code that is
// not Greek-specific reads it from the Definition, not from this constant.
const Language = "grc"

// LanguageDisplayName is the learner-facing name recorded when an Add makes
// Ancient Greek available as a study language.
const LanguageDisplayName = "Ancient Greek"

// LanguageLatin is the Latin language tag, the second classical language
// (ADR 0132).
const LanguageLatin = "la"

// LatinDisplayName is the learner-facing name recorded when an Add makes
// Latin available as a study language.
const LatinDisplayName = "Latin"

// Languages lists the classical languages a Definition may carry, in display
// order.
func Languages() []string { return []string{Language, LanguageLatin} }

// IsLanguage reports whether language is a classical language.
func IsLanguage(language string) bool {
	return DisplayName(language) != ""
}

// DisplayName is the learner-facing name of a classical language, or "" for a
// language that is not classical.
func DisplayName(language string) string {
	switch language {
	case Language:
		return LanguageDisplayName
	case LanguageLatin:
		return LatinDisplayName
	default:
		return ""
	}
}

// SourceFormatOGACoNLLU names the single-file oga_src CoNLL-U adapter.
const SourceFormatOGACoNLLU = "oga-conllu"

// SourceFormatOLACoNLLU names the two-member Opera Latina Adnotata adapter: the
// Work's CoNLL-U and its PAULA Citation layer, both read from the pinned OLA
// archive (ADR 0132).
const SourceFormatOLACoNLLU = "ola-conllu"

// InterpretationVersionOLA is the interpretation version of the ola-conllu
// format: native AGDT syntax stored verbatim, with OLA's _CO, _AP and _ExD
// relation suffixes read at read time (ADR 0122).
const InterpretationVersionOLA = "ola-agdt-syntax/1"

// FetchKindArchiveRange is the layer fetch kind that reads individual members
// of one pinned archive with HTTP Range requests instead of fetching a whole
// file under a base URL.
const FetchKindArchiveRange = "archive-range"

// SplitWholeWork is the only split kind a work override may request: the whole
// pinned file is one Reading unit (ADR 0128).
const SplitWholeWork = "whole-work"

// ContentsBooks is the work override's authority that the whole-work unit
// records each constituent book. The generator fails when a source with
// multiple book ranges lacks it, and when a source without them asks for it.
const ContentsBooks = "books"

// ADR 0111 licence classes and restriction kinds.
const (
	LicenceClassSourceContent    = "source-content"
	LicenceClassAnalysisResource = "analysis-resource"

	RestrictionNone         = "none"
	RestrictionAttribution  = "attribution"
	RestrictionShareAlike   = "share-alike"
	RestrictionNC           = "nc"
	RestrictionResearchOnly = "research-only"
	RestrictionUnclear      = "unclear"
)

// SourcePin identifies one exact upstream file. Size and SHA-256 are the
// reviewed commitment the generator and the preparation path check.
type SourcePin struct {
	Layer    string `json:"layer"`
	FileName string `json:"fileName"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
	Format   string `json:"format"`
	// Members pins the archive members an ola-conllu source reads: the CoNLL-U
	// first, then the PAULA Citation layer. FileName, Size and SHA256 repeat the
	// CoNLL-U member so the stored single-file pin columns identify it.
	Members []ArchiveMember `json:"members,omitempty"`
}

// ArchiveMember pins one member of a zip archive for a Range fetch: where its
// local header starts, how many bytes it occupies compressed and
// uncompressed, and the checksums of its uncompressed bytes.
type ArchiveMember struct {
	Name           string `json:"name"`
	HeaderOffset   int64  `json:"headerOffset"`
	CompressedSize int64  `json:"compressedSize"`
	Size           int64  `json:"size"`
	// CRC32 is the IEEE CRC-32 of the uncompressed bytes, eight lowercase hex
	// digits.
	CRC32  string `json:"crc32"`
	SHA256 string `json:"sha256"`
}

// Validate reports whether an archive member pin is complete.
func (m ArchiveMember) Validate() error {
	switch {
	case strings.TrimSpace(m.Name) == "":
		return errors.New("classical: archive member name is required")
	case m.HeaderOffset < 0:
		return errors.New("classical: archive member header offset must not be negative")
	case m.CompressedSize <= 0 || m.Size <= 0:
		return errors.New("classical: archive member sizes must be positive")
	case !isLowerHex(m.CRC32, 8):
		return errors.New("classical: archive member CRC-32 must be 8 lowercase hex characters")
	case !isLowerHexSHA256(m.SHA256):
		return errors.New("classical: archive member SHA-256 must be 64 lowercase hex characters")
	}
	return nil
}

// validateMembers checks that the source pin's members agree with its format:
// an ola-conllu source pins exactly its CoNLL-U and PAULA members, and no other
// format pins members at all.
func (p SourcePin) validateMembers() error {
	if p.Format != SourceFormatOLACoNLLU {
		if len(p.Members) > 0 {
			return fmt.Errorf("classical: source format %q does not pin archive members", p.Format)
		}
		return nil
	}
	if len(p.Members) != 2 {
		return errors.New("classical: an ola-conllu source pins exactly two archive members")
	}
	for _, member := range p.Members {
		if err := member.Validate(); err != nil {
			return err
		}
	}
	conllu := p.Members[0]
	if p.FileName != conllu.Name || p.Size != conllu.Size || p.SHA256 != conllu.SHA256 {
		return errors.New("classical: an ola-conllu source's file pin must repeat its CoNLL-U member")
	}
	return nil
}

// TranslationPin identifies one exact reference-translation file and the
// citation range it aligns over.
type TranslationPin struct {
	Layer         string `json:"layer"`
	FileName      string `json:"fileName"`
	Size          int64  `json:"size"`
	SHA256        string `json:"sha256"`
	CitationStart string `json:"citationStart"`
	CitationEnd   string `json:"citationEnd"`
}

// ConstituentBookSpan is one constituent book inside a whole-work Reading
// unit: its number, citation range and word count. It is read-only catalog
// information; a constituent book is never a Reading unit of its own.
type ConstituentBookSpan struct {
	Book          int    `json:"book"`
	CitationStart string `json:"citationStart"`
	CitationEnd   string `json:"citationEnd"`
	WordCount     int    `json:"wordCount"`
}

// Definition is one Reading-unit definition in the generated manifest. Work
// and Edition keys are separate from the Reading-unit Key so all Books of a
// Work share one Work and one Edition.
type Definition struct {
	Key           string `json:"key"`
	WorkKey       string `json:"workKey"`
	WorkTitle     string `json:"workTitle"`
	WorkAuthor    string `json:"workAuthor"`
	OriginalTitle string `json:"originalTitle,omitempty"`
	EditionKey    string `json:"editionKey"`
	EditionLabel  string `json:"editionLabel"`
	Language      string `json:"language"`
	// ConstituentBook is absent for a whole-work unit. Only a withdrawn
	// per-book definition from before ADR 0128 still carries it.
	ConstituentBook *int `json:"constituentBook,omitempty"`
	// ConstituentBooks is how many books the Work's citation scheme names;
	// it is one for a Work without books.
	ConstituentBooks int    `json:"constituentBooks"`
	CitationStart    string `json:"citationStart"`
	CitationEnd      string `json:"citationEnd"`
	WordCount        int    `json:"wordCount"`
	// Contents lists each constituent book of a whole-work unit whose Work
	// has more than one. It is empty otherwise.
	Contents []ConstituentBookSpan `json:"contents,omitempty"`
	// Opening is the first sentence that is not a speaker label, exactly as
	// the Edition prints it. It is derived from the source pin, so changing it
	// alone does not bump the revision. A withdrawn tombstone may omit it.
	Opening     string          `json:"opening,omitempty"`
	Source      SourcePin       `json:"source"`
	Translation *TranslationPin `json:"translation,omitempty"`
	// InterpretationVersion names the versioned interpretation recipe.
	InterpretationVersion string `json:"interpretationVersion"`
	// Revision increases monotonically whenever a content-bearing field
	// changes.
	Revision int `json:"revision"`
	// Withdrawn marks a removed entry whose Books must keep their association.
	Withdrawn bool `json:"withdrawn,omitempty"`
	// Note is the reviewer's sign-off for a curatorial departure.
	Note string `json:"note,omitempty"`
}

// Validate reports whether a definition is complete and canonical.
func (d Definition) Validate() error {
	switch {
	case strings.TrimSpace(d.Key) == "":
		return errors.New("classical: definition key is required")
	case strings.TrimSpace(d.WorkKey) == "":
		return errors.New("classical: definition Work key is required")
	case strings.TrimSpace(d.WorkTitle) == "":
		return errors.New("classical: definition Work title is required")
	case strings.TrimSpace(d.WorkAuthor) == "":
		return errors.New("classical: definition Work author is required")
	case strings.TrimSpace(d.EditionKey) == "":
		return errors.New("classical: definition Edition key is required")
	case strings.TrimSpace(d.EditionLabel) == "":
		return errors.New("classical: definition Edition label is required")
	case !IsLanguage(d.Language):
		return fmt.Errorf("classical: definition language %q is not a classical language", d.Language)
	case d.ConstituentBooks <= 0:
		return errors.New("classical: definition constituent book count must be positive")
	case d.ConstituentBook != nil && (*d.ConstituentBook < 1 || *d.ConstituentBook > d.ConstituentBooks):
		return errors.New("classical: definition constituent book number is outside its Work")
	case !isCitation(d.CitationStart):
		return errors.New("classical: definition citation start is not a citation")
	case !isCitation(d.CitationEnd):
		return errors.New("classical: definition citation end is not a citation")
	case d.WordCount <= 0:
		return errors.New("classical: definition word count must be positive")
	case d.Source.Layer == "":
		return errors.New("classical: definition source layer is required")
	case d.Source.FileName == "":
		return errors.New("classical: definition source file name is required")
	case d.Source.Size <= 0:
		return errors.New("classical: definition source file size must be positive")
	case !isLowerHexSHA256(d.Source.SHA256):
		return errors.New("classical: definition source SHA-256 must be 64 lowercase hex characters")
	case d.Source.Format == "":
		return errors.New("classical: definition source format is required")
	case d.InterpretationVersion == "":
		return errors.New("classical: definition interpretation version is required")
	case d.Revision < 1:
		return errors.New("classical: definition revision must be positive")
	}
	if err := d.Source.validateMembers(); err != nil {
		return err
	}
	if err := d.validateContents(); err != nil {
		return err
	}
	if d.Translation != nil {
		if err := d.Translation.Validate(); err != nil {
			return err
		}
	}
	if d.ConstituentBook != nil && !d.Withdrawn {
		return errors.New("classical: only a withdrawn definition may be a constituent book")
	}
	return nil
}

// validateContents checks that a whole-work definition's contents list agrees
// with its constituent-book count, range and word count.
func (d Definition) validateContents() error {
	if d.ConstituentBook != nil || d.ConstituentBooks == 1 {
		if len(d.Contents) > 0 {
			return errors.New("classical: only a whole-work definition of a multi-book Work lists contents")
		}
		return nil
	}
	if len(d.Contents) != d.ConstituentBooks {
		return errors.New("classical: a whole-work definition lists one content entry per constituent book")
	}
	words, previousBook := 0, 0
	for _, book := range d.Contents {
		switch {
		case book.Book <= previousBook:
			return errors.New("classical: definition contents must ascend by book number")
		case !isCitation(book.CitationStart) || !isCitation(book.CitationEnd):
			return errors.New("classical: definition contents citation is not a citation")
		case book.WordCount <= 0:
			return errors.New("classical: definition contents word count must be positive")
		}
		previousBook = book.Book
		words += book.WordCount
	}
	switch {
	case d.Contents[0].CitationStart != d.CitationStart || d.Contents[len(d.Contents)-1].CitationEnd != d.CitationEnd:
		return errors.New("classical: definition contents must cover the definition citation range")
	case words != d.WordCount:
		return errors.New("classical: definition contents word counts must sum to the definition word count")
	}
	return nil
}

// Validate reports whether a translation pin is complete.
func (t TranslationPin) Validate() error {
	switch {
	case t.Layer == "":
		return errors.New("classical: translation layer is required")
	case t.FileName == "":
		return errors.New("classical: translation file name is required")
	case t.Size <= 0:
		return errors.New("classical: translation file size must be positive")
	case !isLowerHexSHA256(t.SHA256):
		return errors.New("classical: translation SHA-256 must be 64 lowercase hex characters")
	case !isCitation(t.CitationStart):
		return errors.New("classical: translation citation start is not a citation")
	case !isCitation(t.CitationEnd):
		return errors.New("classical: translation citation end is not a citation")
	}
	return nil
}

// Layer pins one upstream layer under ADR 0111.
type Layer struct {
	Key     string `json:"key"`
	Release string `json:"release,omitempty"`
	Commit  string `json:"commit,omitempty"`
	BaseURL string `json:"baseURL,omitempty"`
	// FetchKind is empty for a layer of whole files under BaseURL, or
	// FetchKindArchiveRange for a layer of members of one pinned archive.
	FetchKind string `json:"fetchKind,omitempty"`
	// ArchiveURL, ArchiveSize and Record describe the pinned archive of an
	// archive-range layer: the URL its members are requested from, its total
	// size in bytes and the Zenodo record it comes from.
	ArchiveURL          string      `json:"archiveURL,omitempty"`
	ArchiveSize         int64       `json:"archiveSize,omitempty"`
	Record              string      `json:"record,omitempty"`
	LicenceClass        string      `json:"licenceClass"`
	Restriction         string      `json:"restriction"`
	DeployerRegisterRow string      `json:"deployerRegisterRow"`
	Files               []LayerFile `json:"files,omitempty"`
}

// LayerFile records upstream metadata for one file in a layer. The curatorial
// fields tell whether an override departs from what the upstream file states.
// When Size and SHA256 are present they are the reviewed byte pin the
// generator verifies. A reference translation also carries the credit an
// exported card and deck show for its passage context.
type LayerFile struct {
	Name          string `json:"name"`
	WorkTitle     string `json:"workTitle,omitempty"`
	WorkAuthor    string `json:"workAuthor,omitempty"`
	OriginalTitle string `json:"originalTitle,omitempty"`
	EditionLabel  string `json:"editionLabel,omitempty"`
	Size          int64  `json:"size,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
	// PassageLabel names the translation and work on a card, such as
	// "Brownson (1922), Xen. Anab.". The card appends the cited sections.
	PassageLabel string `json:"passageLabel,omitempty"`
	// DeckNotice is the attribution and licence notice an exported deck
	// carries whenever a card froze a passage from this file.
	DeckNotice string `json:"deckNotice,omitempty"`
}

// HasPassageAttribution reports whether the file carries both its card label
// and its deck notice.
func (f LayerFile) HasPassageAttribution() bool {
	return strings.TrimSpace(f.PassageLabel) != "" && strings.TrimSpace(f.DeckNotice) != ""
}

// File returns the upstream metadata registered for one file name.
func (l Layer) File(name string) (LayerFile, bool) {
	for _, file := range l.Files {
		if file.Name == name {
			return file, true
		}
	}
	return LayerFile{}, false
}

// IsSourceContent reports whether the layer is classed as Source content under
// ADR 0111. Only that class must permit redistribution and be no stricter than
// share-alike, so only it may back a Reading unit's source or translation.
func (l Layer) IsSourceContent() bool {
	return l.LicenceClass == LicenceClassSourceContent
}

// Validate reports whether a layer is admissible under ADR 0111. Source
// content must permit redistribution and be no stricter than share-alike;
// analysis resources may be NC, research-only or unclear because the layer is
// fetched or supplied by the deployer and never ships in the release.
func (l Layer) Validate() error {
	if err := l.validateHeader(); err != nil {
		return err
	}
	if err := l.validateLicence(); err != nil {
		return err
	}
	return l.validateFiles()
}

// validateHeader checks the layer's identity and URL.
func (l Layer) validateHeader() error {
	switch {
	case strings.TrimSpace(l.Key) == "":
		return errors.New("classical: layer key is required")
	case l.Release == "" && l.Commit == "":
		return errors.New("classical: layer needs a release or commit")
	case l.DeployerRegisterRow == "":
		return errors.New("classical: layer deployer-register row is required")
	}
	switch l.FetchKind {
	case "":
		if l.BaseURL == "" {
			return errors.New("classical: layer base URL is required")
		}
		if !isAbsoluteHTTPURL(l.BaseURL) {
			return fmt.Errorf("classical: layer %q base URL is not an absolute HTTP URL", l.Key)
		}
	case FetchKindArchiveRange:
		if !isAbsoluteHTTPURL(l.ArchiveURL) {
			return fmt.Errorf("classical: layer %q archive URL is not an absolute HTTP URL", l.Key)
		}
		if l.ArchiveSize <= 0 {
			return fmt.Errorf("classical: layer %q archive size must be positive", l.Key)
		}
		if strings.TrimSpace(l.Record) == "" {
			return fmt.Errorf("classical: layer %q archive record is required", l.Key)
		}
	default:
		return fmt.Errorf("classical: layer %q has unknown fetch kind %q", l.Key, l.FetchKind)
	}
	return nil
}

// isAbsoluteHTTPURL reports whether value is an absolute http or https URL.
func isAbsoluteHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != ""
}

// validateLicence applies the ADR 0111 admissibility rule for the layer's
// class.
func (l Layer) validateLicence() error {
	switch l.LicenceClass {
	case LicenceClassSourceContent:
		switch l.Restriction {
		case RestrictionNone, RestrictionAttribution, RestrictionShareAlike:
			return nil
		default:
			return fmt.Errorf("classical: layer %q Source content is not redistributable (restriction %q)", l.Key, l.Restriction)
		}
	case LicenceClassAnalysisResource:
		switch l.Restriction {
		case RestrictionNone, RestrictionAttribution, RestrictionShareAlike, RestrictionNC, RestrictionResearchOnly, RestrictionUnclear:
			return nil
		default:
			return fmt.Errorf("classical: layer %q has unknown restriction %q", l.Key, l.Restriction)
		}
	default:
		return fmt.Errorf("classical: layer %q has unknown licence class %q", l.Key, l.LicenceClass)
	}
}

// validateFiles checks each file's name and optional byte pin.
func (l Layer) validateFiles() error {
	for _, file := range l.Files {
		if file.Name == "" {
			return fmt.Errorf("classical: layer %q has a file without a name", l.Key)
		}
		if file.Size < 0 || (file.Size > 0) != (file.SHA256 != "") {
			return fmt.Errorf("classical: layer %q file %q needs both size and SHA-256", l.Key, file.Name)
		}
		if file.SHA256 != "" && !isLowerHexSHA256(file.SHA256) {
			return fmt.Errorf("classical: layer %q file %q SHA-256 is not 64 lowercase hex characters", l.Key, file.Name)
		}
		if err := l.validatePassageAttribution(file); err != nil {
			return err
		}
	}
	return nil
}

// validatePassageAttribution requires a passage label and deck notice together,
// only on pinned Source content, since only Source content reaches a deck.
func (l Layer) validatePassageAttribution(file LayerFile) error {
	hasLabel, hasNotice := strings.TrimSpace(file.PassageLabel) != "", strings.TrimSpace(file.DeckNotice) != ""
	if !hasLabel && !hasNotice {
		return nil
	}
	switch {
	case hasLabel != hasNotice:
		return fmt.Errorf("classical: layer %q file %q needs both a passage label and a deck notice", l.Key, file.Name)
	case !l.IsSourceContent():
		return fmt.Errorf("classical: layer %q file %q is not Source content and cannot credit a passage", l.Key, file.Name)
	case file.SHA256 == "":
		return fmt.Errorf("classical: layer %q file %q credits a passage without a SHA-256 pin", l.Key, file.Name)
	}
	return nil
}

// Layers is the hand-maintained layers.json document.
type Layers struct {
	Layers []Layer `json:"layers"`
}

// Validate reports whether every layer is admissible and keys are unique.
func (l Layers) Validate() error {
	if len(l.Layers) == 0 {
		return errors.New("classical: layers.json has no layers")
	}
	seen := map[string]struct{}{}
	for _, layer := range l.Layers {
		if err := layer.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[layer.Key]; duplicate {
			return fmt.Errorf("classical: duplicate layer key %q", layer.Key)
		}
		seen[layer.Key] = struct{}{}
	}
	return nil
}

// Layer returns one layer by key.
func (l Layers) Layer(key string) (Layer, bool) {
	for _, layer := range l.Layers {
		if layer.Key == key {
			return layer, true
		}
	}
	return Layer{}, false
}

// SourceSpec names the upstream file a Work is generated from. Size and
// SHA-256 are derived from the file, never hand-written.
type SourceSpec struct {
	Layer    string `json:"layer"`
	FileName string `json:"fileName"`
	Format   string `json:"format"`
}

// TranslationSpec names the optional upstream translation file for a Work.
type TranslationSpec struct {
	Layer    string `json:"layer"`
	FileName string `json:"fileName"`
}

// UnitSpec is the explicitly reviewed range of a whole-work Reading unit.
type UnitSpec struct {
	CitationStart string `json:"citationStart"`
	CitationEnd   string `json:"citationEnd"`
}

// Override is one line of overrides.jsonl. A work override has WorkKey; a unit
// override has Key. Work-level values apply to every constituent unit.
type Override struct {
	WorkKey       string `json:"workKey,omitempty"`
	Key           string `json:"key,omitempty"`
	WorkTitle     string `json:"workTitle,omitempty"`
	WorkAuthor    string `json:"workAuthor,omitempty"`
	OriginalTitle string `json:"originalTitle,omitempty"`
	// Language is the Work's classical language; it defaults to Ancient Greek.
	Language     string           `json:"language,omitempty"`
	EditionKey   string           `json:"editionKey,omitempty"`
	EditionLabel string           `json:"editionLabel,omitempty"`
	Note         string           `json:"note,omitempty"`
	Source       *SourceSpec      `json:"source,omitempty"`
	Translation  *TranslationSpec `json:"translation,omitempty"`
	Split        string           `json:"split,omitempty"`
	// ContentsBooks is the authority that the whole-work unit records its
	// constituent books; the source must have a multi-book citation scheme.
	Contents string     `json:"contents,omitempty"`
	Units    []UnitSpec `json:"units,omitempty"`
	Revision *int       `json:"revision,omitempty"`
}

// IsWork reports whether the override applies to a Work.
func (o Override) IsWork() bool { return o.WorkKey != "" }

// Validate reports whether an override is structurally complete.
func (o Override) Validate() error {
	switch {
	case o.WorkKey != "" && o.Key != "":
		return errors.New("classical: an override has either workKey or key, not both")
	case o.WorkKey == "" && o.Key == "":
		return errors.New("classical: an override needs workKey or key")
	case o.Revision != nil && *o.Revision < 1:
		return errors.New("classical: override revision must be positive")
	case o.Language != "" && !IsLanguage(o.Language):
		return fmt.Errorf("classical: override language %q is not a classical language", o.Language)
	}
	if o.Key != "" {
		return o.validateUnit()
	}
	return o.validateWork()
}

// validateUnit restricts a unit override to curatorial fields, a note and a
// revision.
func (o Override) validateUnit() error {
	if o.Language != "" || o.Source != nil || o.Translation != nil || o.Split != "" || o.Contents != "" || o.Units != nil || o.EditionKey != "" {
		return fmt.Errorf("classical: unit override %q may only change curatorial fields, a note or a revision", o.Key)
	}
	return nil
}

// validateWork checks a Work override's source, split and selected units.
func (o Override) validateWork() error {
	if o.Source == nil {
		return fmt.Errorf("classical: work override %q needs a source", o.WorkKey)
	}
	if o.Source.Layer == "" || o.Source.FileName == "" || o.Source.Format == "" {
		return fmt.Errorf("classical: work override %q has an incomplete source", o.WorkKey)
	}
	if o.Split != SplitWholeWork {
		return fmt.Errorf("classical: work override %q has unknown split %q", o.WorkKey, o.Split)
	}
	switch {
	case o.Contents != "" && o.Contents != ContentsBooks:
		return fmt.Errorf("classical: work override %q has unknown contents %q", o.WorkKey, o.Contents)
	case o.Contents != "" && len(o.Units) > 0:
		return fmt.Errorf("classical: work override %q cannot list book contents for a hand-reviewed range", o.WorkKey)
	case len(o.Units) > 1:
		return fmt.Errorf("classical: whole-work override %q needs one unit", o.WorkKey)
	}
	if o.Translation != nil && (o.Translation.Layer == "" || o.Translation.FileName == "") {
		return fmt.Errorf("classical: work override %q has an incomplete translation", o.WorkKey)
	}
	if o.EditionKey == "" {
		return fmt.Errorf("classical: work override %q needs an Edition key", o.WorkKey)
	}
	for _, unit := range o.Units {
		if !isCitation(unit.CitationStart) || !isCitation(unit.CitationEnd) {
			return fmt.Errorf("classical: work override %q has an invalid unit range", o.WorkKey)
		}
	}
	return nil
}

// SortOverrides orders unit overrides by key and work overrides by Work key.
func SortOverrides(overrides []Override) {
	sort.SliceStable(overrides, func(i, j int) bool {
		return OverrideSortKey(overrides[i]) < OverrideSortKey(overrides[j])
	})
}

// OverrideSortKey is the stable order key of one override.
func OverrideSortKey(o Override) string {
	if o.IsWork() {
		return "work:" + o.WorkKey
	}
	return "unit:" + o.Key
}

// isCitation reports whether value is a valid Citation (see ParseCitation).
func isCitation(value string) bool { return IsCitation(value) }

// isLowerHexSHA256 reports whether value is exactly 64 lowercase hexadecimal
// characters.
func isLowerHexSHA256(value string) bool { return isLowerHex(value, 64) }

// isLowerHex reports whether value is exactly length lowercase hexadecimal
// characters.
func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
