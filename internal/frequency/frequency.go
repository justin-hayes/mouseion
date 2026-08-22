// Package frequency imports and serves versioned, language-scoped frequency resources.
package frequency

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const (
	DWDSName        = "DWDS Lemmadatenbank"
	DWDSSourceURL   = "https://www.dwds.de/lemma/csv"
	DWDSLicense     = "CC BY-SA 4.0"
	DWDSAttribution = "Digitales Wörterbuch der deutschen Sprache (DWDS)"
)

var ErrEmptyDataset = errors.New("frequency: dataset contains no frequency entries")

type RowIssue struct {
	Row     int
	Message string
}

type Duplicate struct {
	CanonicalLemma string
	UPOS           string
	KeptRow        int
	DiscardedRow   int
}

type ParseResult struct {
	Entries          []domain.FrequencyEntry
	MissingFrequency []RowIssue
	// MissingWordClass reports otherwise usable rows skipped because DWDS did
	// not provide a wortklasse/upos value.
	MissingWordClass []RowIssue
	Duplicates       []Duplicate
}

type rawEntry struct {
	lemma, upos string
	class       int
	row         int
}

// Percentiles derives midpoint percentile ranks from the snapshot's seven
// logarithmic frequency-class buckets. Ties receive the same stable value.
func Percentiles(histogram [7]int) ([7]float64, error) {
	var out [7]float64
	total := 0
	for _, count := range histogram {
		if count < 0 {
			return out, errors.New("frequency: histogram counts cannot be negative")
		}
		total += count
	}
	if total == 0 {
		return out, ErrEmptyDataset
	}
	below := 0
	for class, count := range histogram {
		out[class] = (float64(below) + float64(count)/2) / float64(total)
		below += count
	}
	return out, nil
}

// ParseDWDS accepts the official comma-separated snapshot, normalized TSV,
// and gzip-compressed variants (detected by the gzip magic bytes).
func ParseDWDS(input io.Reader, language, sourceVersion string) (ParseResult, error) {
	if strings.TrimSpace(sourceVersion) == "" {
		return ParseResult{}, errors.New("frequency: DWDS snapshot Stand/version is required")
	}
	buffered, err := io.ReadAll(input)
	if err != nil {
		return ParseResult{}, fmt.Errorf("frequency: read snapshot: %w", err)
	}
	if len(buffered) >= 2 && buffered[0] == 0x1f && buffered[1] == 0x8b {
		gz, err := gzip.NewReader(bytes.NewReader(buffered))
		if err != nil {
			return ParseResult{}, fmt.Errorf("frequency: open gzip snapshot: %w", err)
		}
		buffered, err = io.ReadAll(gz)
		closeErr := gz.Close()
		if err != nil {
			return ParseResult{}, fmt.Errorf("frequency: decompress snapshot: %w", err)
		}
		if closeErr != nil {
			return ParseResult{}, fmt.Errorf("frequency: close gzip snapshot: %w", closeErr)
		}
	}
	if !utf8.Valid(buffered) {
		return ParseResult{}, errors.New("frequency: snapshot is not valid UTF-8")
	}
	comma := ','
	if firstLine, _, _ := strings.Cut(string(buffered), "\n"); strings.Count(firstLine, "\t") > strings.Count(firstLine, ",") {
		comma = '\t'
	}
	r := csv.NewReader(strings.NewReader(string(buffered)))
	r.Comma = comma
	r.FieldsPerRecord = -1
	r.ReuseRecord = false
	header, err := r.Read()
	if err != nil {
		return ParseResult{}, fmt.Errorf("frequency: read header: %w", err)
	}
	columns := make(map[string]int, len(header))
	for i, value := range header {
		columns[normalizeHeader(value)] = i
	}
	lemmaCol, okLemma := findColumn(columns, "lemma")
	uposCol, okUPOS := findColumn(columns, "upos", "wortklasse")
	classCol, okClass := findColumn(columns, "haeufigkeitsklasse", "frequenzklasse")
	if !okLemma || !okUPOS || !okClass {
		return ParseResult{}, fmt.Errorf("frequency: required columns missing (need lemma, upos/wortklasse, haeufigkeitsklasse/frequenzklasse); got %q", header)
	}
	versionCol, hasVersion := findColumn(columns, "source_version")
	maxCol := max(lemmaCol, uposCol, classCol)
	if hasVersion {
		maxCol = max(maxCol, versionCol)
	}
	var result ParseResult
	var malformed []RowIssue
	byIdentity := make(map[string]rawEntry)
	for rowNum := 2; ; rowNum++ {
		record, readErr := r.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			malformed = append(malformed, RowIssue{rowNum, readErr.Error()})
			continue
		}
		if len(record) <= maxCol {
			malformed = append(malformed, RowIssue{rowNum, fmt.Sprintf("expected column %d, got %d fields", maxCol+1, len(record))})
			continue
		}
		lemma := strings.TrimSpace(record[lemmaCol])
		wordClass := strings.TrimSpace(record[uposCol])
		classText := strings.TrimSpace(record[classCol])
		if lemma == "" {
			malformed = append(malformed, RowIssue{rowNum, "lemma must be non-empty"})
			continue
		}
		if classText == "" || strings.EqualFold(classText, "n/a") {
			result.MissingFrequency = append(result.MissingFrequency, RowIssue{rowNum, "frequency class is unavailable"})
			continue
		}
		class, parseErr := strconv.Atoi(classText)
		if parseErr != nil || class < 0 || class > 6 {
			malformed = append(malformed, RowIssue{rowNum, fmt.Sprintf("invalid frequency class %q (want integer 0-6)", classText)})
			continue
		}
		if wordClass == "" {
			result.MissingWordClass = append(result.MissingWordClass, RowIssue{rowNum, "word class is unavailable"})
			continue
		}
		if hasVersion && strings.TrimSpace(record[versionCol]) != sourceVersion {
			malformed = append(malformed, RowIssue{rowNum, fmt.Sprintf("source_version %q does not match snapshot version %q", record[versionCol], sourceVersion)})
			continue
		}
		normalized, normalizeErr := canonicalization.Normalize(language, lemma)
		if normalizeErr != nil {
			return ParseResult{}, fmt.Errorf("frequency: row %d normalize lemma: %w", rowNum, normalizeErr)
		}
		upos := mapUPOS(wordClass)
		key := normalized.CanonicalLemma + "\x00" + upos
		candidate := rawEntry{normalized.CanonicalLemma, upos, class, rowNum}
		if existing, duplicate := byIdentity[key]; duplicate {
			kept, discarded := existing, candidate
			if candidate.class > existing.class || (candidate.class == existing.class && candidate.row < existing.row) {
				kept, discarded = candidate, existing
				byIdentity[key] = candidate
			}
			result.Duplicates = append(result.Duplicates, Duplicate{candidate.lemma, candidate.upos, kept.row, discarded.row})
			continue
		}
		byIdentity[key] = candidate
	}
	if len(malformed) > 0 {
		return ParseResult{}, malformedError(malformed)
	}
	if len(byIdentity) == 0 {
		return ParseResult{}, ErrEmptyDataset
	}
	var histogram [7]int
	raw := make([]rawEntry, 0, len(byIdentity))
	for _, entry := range byIdentity {
		histogram[entry.class]++
		raw = append(raw, entry)
	}
	percentiles, err := Percentiles(histogram)
	if err != nil {
		return ParseResult{}, err
	}
	sort.Slice(raw, func(i, j int) bool {
		if raw[i].class != raw[j].class {
			return raw[i].class > raw[j].class
		}
		if raw[i].lemma != raw[j].lemma {
			return raw[i].lemma < raw[j].lemma
		}
		return raw[i].upos < raw[j].upos
	})
	result.Entries = make([]domain.FrequencyEntry, len(raw))
	for i, entry := range raw {
		result.Entries[i] = domain.FrequencyEntry{Language: language, CanonicalLemma: entry.lemma, UPOS: entry.upos, Rank: int64(i + 1), FrequencyClass: entry.class, Percentile: percentiles[entry.class]}
	}
	return result, nil
}

func normalizeHeader(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "\ufeff"))), "ä", "ae")
}

func findColumn(columns map[string]int, names ...string) (int, bool) {
	for _, name := range names {
		if value, ok := columns[name]; ok {
			return value, true
		}
	}
	return 0, false
}

func malformedError(issues []RowIssue) error {
	const shown = 8
	parts := make([]string, 0, min(len(issues), shown))
	for _, issue := range issues[:min(len(issues), shown)] {
		parts = append(parts, fmt.Sprintf("row %d: %s", issue.Row, issue.Message))
	}
	if len(issues) > shown {
		parts = append(parts, fmt.Sprintf("and %d more malformed rows", len(issues)-shown))
	}
	return fmt.Errorf("frequency: rejected snapshot: %s", strings.Join(parts, "; "))
}

func mapUPOS(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(normalized, "substantiv") {
		return "NOUN"
	}
	if strings.Contains(normalized, "eigenname") {
		return "PROPN"
	}
	if strings.Contains(normalized, "adjektiv") || normalized == "komparativ" || normalized == "superlativ" {
		return "ADJ"
	}
	if strings.Contains(normalized, "adverb") {
		return "ADV"
	}
	if normalized == "verb" {
		return "VERB"
	}
	mapping := map[string]string{"präposition": "ADP", "konjunktion": "CCONJ", "interjektion": "INTJ", "artikel": "DET", "bestimmter artikel": "DET", "unbestimmter artikel": "DET", "partikel": "PART", "pronomen": "PRON", "personalpronomen": "PRON", "possessivpronomen": "DET", "demonstrativpronomen": "PRON", "indefinitpronomen": "PRON", "interrogativpronomen": "PRON", "relativpronomen": "PRON", "kardinalzahlwort": "NUM", "ordinalzahlwort": "NUM", "bruchzahlwort": "NUM"}
	if upos, ok := mapping[normalized]; ok {
		return upos
	}
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", "_"))
}

type Store interface {
	GetUserByID(context.Context, string) (domain.User, error)
	CreateFrequencyDataset(context.Context, domain.FrequencyDataset, []domain.FrequencyEntry) (domain.FrequencyDataset, error)
	ActivateFrequencyDataset(context.Context, string) error
	DeactivateFrequencyDataset(context.Context, string) error
	RemoveFrequencyDataset(context.Context, string) error
	GetActiveFrequencyDataset(context.Context, string) (domain.FrequencyDataset, error)
	ListFrequencyDatasets(context.Context, string) ([]domain.FrequencyDataset, error)
	FrequencyPercentile(context.Context, string, string, string) (float64, bool, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Create(ctx context.Context, adminID, language, version string, input io.Reader) (domain.FrequencyDataset, ParseResult, error) {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return domain.FrequencyDataset{}, ParseResult{}, err
	}
	parsed, err := ParseDWDS(input, language, version)
	if err != nil {
		return domain.FrequencyDataset{}, ParseResult{}, err
	}
	dataset := domain.FrequencyDataset{Language: language, Name: DWDSName, Version: version, SourceURL: DWDSSourceURL, License: DWDSLicense, Attribution: DWDSAttribution}
	created, err := s.store.CreateFrequencyDataset(ctx, dataset, parsed.Entries)
	return created, parsed, err
}

// Replace imports a new immutable version and activates it only after the full import commits.
func (s *Service) Replace(ctx context.Context, adminID, language, version string, input io.Reader) (domain.FrequencyDataset, ParseResult, error) {
	dataset, parsed, err := s.Create(ctx, adminID, language, version, input)
	if err != nil {
		return dataset, parsed, err
	}
	if err = s.store.ActivateFrequencyDataset(ctx, dataset.ID); err != nil {
		return dataset, parsed, err
	}
	dataset.Active = true
	return dataset, parsed, nil
}

func (s *Service) Activate(ctx context.Context, adminID, datasetID string) error {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return err
	}
	return s.store.ActivateFrequencyDataset(ctx, datasetID)
}

func (s *Service) Deactivate(ctx context.Context, adminID, datasetID string) error {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return err
	}
	return s.store.DeactivateFrequencyDataset(ctx, datasetID)
}

func (s *Service) Remove(ctx context.Context, adminID, datasetID string) error {
	if err := s.requireAdmin(ctx, adminID); err != nil {
		return err
	}
	return s.store.RemoveFrequencyDataset(ctx, datasetID)
}

func (s *Service) List(ctx context.Context, language string) ([]domain.FrequencyDataset, error) {
	return s.store.ListFrequencyDatasets(ctx, language)
}

func (s *Service) GetActive(ctx context.Context, language string) (domain.FrequencyDataset, error) {
	return s.store.GetActiveFrequencyDataset(ctx, language)
}

func (s *Service) FrequencyPercentile(ctx context.Context, language, lemma, upos string) (float64, bool, error) {
	normalized, err := canonicalization.Normalize(language, lemma)
	if err != nil {
		return 0, false, err
	}
	return s.store.FrequencyPercentile(ctx, language, normalized.CanonicalLemma, strings.ToUpper(strings.TrimSpace(upos)))
}

func (s *Service) IsTopPercentile(ctx context.Context, language, lemma, upos string, cutoff float64) (bool, bool, error) {
	if math.IsNaN(cutoff) || cutoff < 0 || cutoff > 1 {
		return false, false, errors.New("frequency: cutoff must be between 0 and 1")
	}
	percentile, found, err := s.FrequencyPercentile(ctx, language, lemma, upos)
	return found && percentile >= cutoff, found, err
}

func (s *Service) requireAdmin(ctx context.Context, id string) error {
	user, err := s.store.GetUserByID(ctx, id)
	if err != nil || !user.IsAdmin {
		return auth.ErrForbidden
	}
	return nil
}
