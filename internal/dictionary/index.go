package dictionary

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/lemmarisk"
	_ "modernc.org/sqlite"
)

var ErrInvalidIndex = errors.New("dictionary: invalid index")

// Index is a read-only SQLite-backed lexical provider. It is safe for
// concurrent lookups and should be closed with the server.
type Index struct {
	db      *sql.DB
	name    string
	version string

	// greekEntries maps each Ancient Greek entry's ancient-greek/1 key to the
	// source entries that share it. The key is computed here rather than stored
	// in the index, and the source entries are never merged: two homographs
	// that share a key and part of speech both survive.
	greekEntries map[string][]greekEntry
	// greekAvailable is true when the index format carries greek_entries, so an
	// older index reports grc as unavailable instead of merely empty.
	greekAvailable bool
	// skippedGreekHeadwords counts source headwords whose ancient-greek/1 key
	// was not admissible and were therefore not indexed.
	skippedGreekHeadwords int

	// lsjEntries maps each LSJ main entry's ancient-greek/1 key to the source
	// entries that share it. A key with exactly one entry is an unambiguous
	// match; a key with none or several is never matched.
	lsjEntries map[string][]enrichment.AncientGreekLSJEntry
	// lsjAvailable is true when the index format carries the LSJ table, so an
	// older index reports no LSJ entry rather than an empty dictionary.
	lsjAvailable bool
	// skippedLSJHeadwords counts LSJ source headwords whose ancient-greek/1 key
	// was not admissible and were therefore not indexed.
	skippedLSJHeadwords int

	// hStatusAvailable is true when the index carries the French h_status
	// column, so a lookup can report aspirated-h evidence rather than silently
	// dropping it.
	hStatusAvailable bool
}

type evidenceIdentity struct {
	Language string   `json:"language"`
	Lemma    string   `json:"lemma"`
	UPOS     string   `json:"upos"`
	Gloss    string   `json:"gloss"`
	Phrase   string   `json:"phrase"`
	Examples []string `json:"examples"`
	Topics   []string `json:"topics"`
	Tags     []string `json:"tags"`
}

var _ enrichment.LexicalProvider = (*Index)(nil)
var _ enrichment.AncientGreekLSJProvider = (*Index)(nil)

var germanEvidenceStopwords = map[string]bool{"aber": true, "alle": true, "allem": true, "allen": true, "aller": true, "alles": true, "als": true, "also": true, "am": true, "an": true, "and": true, "auf": true, "aus": true, "bei": true, "bin": true, "bis": true, "bist": true, "das": true, "dass": true, "dein": true, "dem": true, "den": true, "der": true, "des": true, "die": true, "dies": true, "diese": true, "dieser": true, "dieses": true, "doch": true, "du": true, "durch": true, "ein": true, "eine": true, "einem": true, "einen": true, "einer": true, "eines": true, "er": true, "es": true, "für": true, "hat": true, "hatte": true, "ich": true, "im": true, "in": true, "ist": true, "mit": true, "nach": true, "nicht": true, "oder": true, "sein": true, "seine": true, "seinem": true, "seinen": true, "seiner": true, "sich": true, "sie": true, "sind": true, "so": true, "und": true, "vom": true, "von": true, "vor": true, "war": true, "waren": true, "was": true, "wie": true, "wird": true, "zu": true, "zum": true, "zur": true}

func OpenIndex(ctx context.Context, path string) (*Index, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%w: path is empty", ErrInvalidIndex)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve path: %w", ErrInvalidIndex, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("%w: stat index: %w", ErrInvalidIndex, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: %s is a directory, not a SQLite file (a missing Docker bind-mount source is created as a directory)", ErrInvalidIndex, absolute)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(absolute)+"?mode=ro&_query_only=1")
	if err != nil {
		return nil, fmt.Errorf("%w: open: %w", ErrInvalidIndex, err)
	}
	db.SetMaxOpenConns(8)
	index := &Index{db: db, name: "kaikki", version: ""}
	if err = db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("%w: ping: %w", ErrInvalidIndex, errors.Join(err, db.Close()))
	}
	if err = db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'provider_version'`).Scan(&index.version); err != nil || strings.TrimSpace(index.version) == "" {
		if err == nil {
			err = errors.New("provider_version is empty")
		}
		err = errors.Join(err, db.Close())
		return nil, fmt.Errorf("%w: metadata: %w", ErrInvalidIndex, err)
	}
	if _, err = db.ExecContext(ctx, `SELECT language, lemma, upos, senses_json, gender, article, plural, ipa, principal_parts FROM entries LIMIT 0`); err != nil {
		return nil, fmt.Errorf("%w: schema: %w", ErrInvalidIndex, errors.Join(err, db.Close()))
	}
	if err = index.loadHStatusAvailability(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if err = index.loadGreekEntries(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if err = index.loadAncientGreekLSJ(ctx); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return index, nil
}

// indexFormatWithFrenchHStatus is the index format that first carried the
// French aspirated-h evidence column. A lower or missing format reports no
// h-status, so an older index falls back to the conservative unknown-h rule
// (ADR 0102) instead of failing to open.
const indexFormatWithFrenchHStatus = 3

// loadHStatusAvailability records whether the index carries the French
// aspirated-h evidence column, so a lookup selects it only when the format
// provides it and otherwise reports unknown h.
func (i *Index) loadHStatusAvailability(ctx context.Context) error {
	version, ok, err := i.indexFormat(ctx)
	if err != nil {
		return err
	}
	i.hStatusAvailable = ok && version >= indexFormatWithFrenchHStatus
	return nil
}

func (i *Index) Name() string {
	if i == nil || i.name == "" {
		return "kaikki"
	}
	return i.name
}

func (i *Index) Version() string {
	if i == nil {
		return ""
	}
	return i.version
}

func (i *Index) Close() error {
	if i == nil || i.db == nil {
		return nil
	}
	return i.db.Close()
}

func (i *Index) Lookup(ctx context.Context, request enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	if i == nil || i.db == nil {
		return enrichment.LexicalEntry{}, false, nil
	}
	language := canonicalization.NormalizeLanguage(request.Language)
	lemma := normalizeLemma(language, request.CanonicalLemma)
	upos := strings.ToUpper(strings.TrimSpace(request.UPOS))
	if language == "" || lemma == "" {
		return enrichment.LexicalEntry{}, false, nil
	}
	exactSenses, weakSenses, morphology, foundExact, err := i.matchEntries(ctx, language, lemma, upos)
	if err != nil {
		return enrichment.LexicalEntry{}, false, err
	}
	if !foundExact && len(weakSenses) == 0 {
		return enrichment.LexicalEntry{}, false, nil
	}
	senses, err := finalizeLookupSenses(combineSenses(exactSenses, weakSenses), language, lemma, upos, i.version)
	if err != nil {
		return enrichment.LexicalEntry{}, false, err
	}
	orderedExact, orderedWeak := splitSensesByMatchStrength(senses)
	ordered := combineSenses(enrichment.OrderSenses(request, orderedExact), enrichment.OrderSenses(request, orderedWeak))
	result := enrichment.LexicalEntry{Senses: ordered, CandidateSenses: append([]enrichment.LexicalSense(nil), senses...), IPA: morphology.ipa, PrincipalParts: morphology.principalParts, HStatus: morphology.hStatus}
	if language == "el" {
		result.Gender, result.Article, result.Plural = morphology.gender, morphology.article, morphology.plural
	} else if len(ordered) > 0 {
		result.Gender, result.Article, result.Plural = ordered[0].Gender, ordered[0].Article, ordered[0].Plural
	}
	return result, true, nil
}

// matchEntries resolves the candidate rows for one lookup. Ancient Greek uses
// the in-memory key index; every other language uses the stored-normalized
// entries query, unchanged.
func (i *Index) matchEntries(ctx context.Context, language, lemma, upos string) (exactSenses, weakSenses []enrichment.LexicalSense, morphology lookupMorphology, foundExact bool, err error) {
	if language == ancientGreekLanguage {
		if !i.greekAvailable {
			return nil, nil, lookupMorphology{}, false, nil
		}
		exact, weak, found := i.ancientGreekRows(lemma, upos)
		return exact, weak, lookupMorphology{}, found, nil
	}
	return lookupEntryRows(ctx, i.db, language, lemma, upos, i.hStatusAvailable)
}

// lookupMorphology carries the morphology columns of an exact-identity row.
type lookupMorphology struct {
	gender, article, plural, ipa, principalParts, hStatus string
}

// lookupEntryRows runs the candidate query and separates rows that match the
// requested part of speech from lemma-only fallbacks, keeping the exact row's
// morphology. withHStatus selects the French aspirated-h evidence column only
// when the index format carries it.
func lookupEntryRows(ctx context.Context, db *sql.DB, language, lemma, upos string, withHStatus bool) (exactSenses, weakSenses []enrichment.LexicalSense, morphology lookupMorphology, foundExact bool, err error) {
	// Two constant queries rather than one assembled column list: the h_status
	// column exists only in the newer index format, and a literal statement
	// keeps the lookup free of any dynamically built SQL.
	var rows *sql.Rows
	if withHStatus {
		rows, err = db.QueryContext(ctx, `SELECT upos, senses_json, gender, article, plural, ipa, principal_parts, h_status FROM entries WHERE language = ? AND lemma = ? AND (upos = ? OR upos = '') ORDER BY CASE WHEN upos = ? THEN 0 ELSE 1 END`, language, lemma, upos, upos)
	} else {
		rows, err = db.QueryContext(ctx, `SELECT upos, senses_json, gender, article, plural, ipa, principal_parts FROM entries WHERE language = ? AND lemma = ? AND (upos = ? OR upos = '') ORDER BY CASE WHEN upos = ? THEN 0 ELSE 1 END`, language, lemma, upos, upos)
	}
	if err != nil {
		return nil, nil, lookupMorphology{}, false, fmt.Errorf("dictionary lookup: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Rows.Err below reports actionable lookup failures.
	exactSenses, weakSenses, morphology, foundExact, err = scanEntryRows(rows, upos, withHStatus)
	if err != nil {
		return nil, nil, lookupMorphology{}, false, err
	}
	return exactSenses, weakSenses, morphology, foundExact, nil
}

// scanEntryRows reads the candidate rows and separates exact part-of-speech
// matches from lemma-only fallbacks, keeping the exact row's morphology.
func scanEntryRows(rows *sql.Rows, upos string, withHStatus bool) (exactSenses, weakSenses []enrichment.LexicalSense, morphology lookupMorphology, foundExact bool, err error) {
	for rows.Next() {
		var entryUPOS, sensesJSON, rowGender, rowArticle, rowPlural, rowIPA, rowPrincipalParts, rowHStatus string
		destinations := []any{&entryUPOS, &sensesJSON, &rowGender, &rowArticle, &rowPlural, &rowIPA, &rowPrincipalParts}
		if withHStatus {
			destinations = append(destinations, &rowHStatus)
		}
		if err = rows.Scan(destinations...); err != nil {
			return nil, nil, lookupMorphology{}, false, fmt.Errorf("dictionary lookup row: %w", err)
		}
		var senses []enrichment.LexicalSense
		if err = json.Unmarshal([]byte(sensesJSON), &senses); err != nil {
			return nil, nil, lookupMorphology{}, false, fmt.Errorf("dictionary senses: %w", err)
		}
		if entryUPOS == upos && upos != "" {
			foundExact = true
			morphology = lookupMorphology{gender: rowGender, article: rowArticle, plural: rowPlural, ipa: rowIPA, principalParts: rowPrincipalParts, hStatus: rowHStatus}
			exactSenses = append(exactSenses, senses...)
			continue
		}
		for senseIndex := range senses {
			senses[senseIndex].MatchStrength = "lemma_only_missing_pos"
		}
		weakSenses = append(weakSenses, senses...)
	}
	if err = rows.Err(); err != nil {
		return nil, nil, lookupMorphology{}, false, fmt.Errorf("dictionary lookup rows: %w", err)
	}
	return exactSenses, weakSenses, morphology, foundExact, nil
}

// finalizeLookupSenses fills each sense's evidence identity and provider
// defaults, then drops senses that collapse onto one evidence identity.
func finalizeLookupSenses(senses []enrichment.LexicalSense, language, lemma, upos, version string) ([]enrichment.LexicalSense, error) {
	seenEvidenceIDs := make(map[string]struct{}, len(senses))
	for index := 0; index < len(senses); {
		sense := &senses[index]
		if err := finalizeSense(sense, language, lemma, upos, version); err != nil {
			return nil, err
		}
		if _, duplicate := seenEvidenceIDs[sense.EvidenceID]; duplicate {
			senses = append(senses[:index], senses[index+1:]...)
			continue
		}
		seenEvidenceIDs[sense.EvidenceID] = struct{}{}
		index++
	}
	return senses, nil
}

// finalizeSense computes a sense's stable evidence identity when it has none
// and fills the provider defaults, leaving explicit values untouched.
func finalizeSense(sense *enrichment.LexicalSense, language, lemma, upos, version string) error {
	if sense.EvidenceID == "" {
		evidenceID, err := lookupEvidenceID(sense, language, lemma, upos)
		if err != nil {
			return err
		}
		sense.EvidenceID = evidenceID
	}
	if sense.Source == "" {
		sense.Source = "wiktionary"
	}
	if sense.Kind == "" {
		sense.Kind = "meaning"
	}
	if sense.Origin == "" {
		sense.Origin = "Kaikki.org Wiktextract enwiktionary"
	}
	if sense.Version == "" {
		sense.Version = version
	}
	if sense.MatchStrength == "" {
		sense.MatchStrength = "exact_lemma_pos"
	}
	return nil
}

// lookupEvidenceID derives the evidence identity for a sense without one. A
// lemma-only sense names no part of speech in its evidence.
func lookupEvidenceID(sense *enrichment.LexicalSense, language, lemma, upos string) (string, error) {
	evidenceUPOS := upos
	if sense.MatchStrength == "lemma_only_missing_pos" {
		evidenceUPOS = ""
	}
	identity, err := json.Marshal(evidenceIdentity{Language: language, Lemma: lemma, UPOS: evidenceUPOS, Gloss: sense.Gloss, Phrase: sense.Phrase, Examples: sense.Examples, Topics: sense.Topics, Tags: sense.Tags})
	if err != nil {
		return "", fmt.Errorf("dictionary evidence identity: %w", err)
	}
	digest := sha256.Sum256(identity)
	return "wiktionary:" + hex.EncodeToString(digest[:]), nil
}

// splitSensesByMatchStrength separates senses that match the requested identity
// from lemma-only fallbacks so each group can be ordered independently.
func splitSensesByMatchStrength(senses []enrichment.LexicalSense) (exact, weak []enrichment.LexicalSense) {
	for _, sense := range senses {
		if sense.MatchStrength == "lemma_only_missing_pos" {
			weak = append(weak, sense)
		} else {
			exact = append(exact, sense)
		}
	}
	return exact, weak
}

// combineSenses concatenates two sense lists without writing into first's
// backing array: the capacity cap forces append to copy whenever second is
// non-empty, and an empty second returns first unchanged as before.
func combineSenses(first, second []enrichment.LexicalSense) []enrichment.LexicalSense {
	return append(slices.Clip(first), second...)
}

func (i *Index) LemmaExists(ctx context.Context, language, lemma, upos string) (bool, error) {
	if i == nil || i.db == nil {
		return false, nil
	}
	language = canonicalization.NormalizeLanguage(language)
	lemma = normalizeLemma(language, lemma)
	upos = strings.ToUpper(strings.TrimSpace(upos))
	if language == "" || lemma == "" || upos == "" {
		return false, nil
	}
	if language == ancientGreekLanguage {
		if !i.greekAvailable {
			return false, nil
		}
		for _, candidate := range i.greekEntries[lemma] {
			if candidate.upos == upos {
				return true, nil
			}
		}
		return false, nil
	}
	var exists bool
	err := i.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM entries WHERE language=? AND lemma=? AND upos=?)`, language, lemma, upos).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("dictionary lemma existence: %w", err)
	}
	return exists, nil
}

// Alternatives returns only indexed German noun alternatives licensed by a
// simple observed-form inflection candidate and sentence examples with a
// meaningful lexical overlap. These are review evidence, never replacements.
func (i *Index) Alternatives(ctx context.Context, language, surface, upos, sentence string) ([]lemmarisk.Alternative, error) {
	if canonicalization.NormalizeLanguage(language) != "de" || strings.ToUpper(strings.TrimSpace(upos)) != "NOUN" {
		return nil, nil
	}
	result := make([]lemmarisk.Alternative, 0, 2)
	seen := make(map[string]bool)
	for _, candidate := range germanNounLemmaCandidates(surface) {
		entry, found, err := i.Lookup(ctx, enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: candidate, UPOS: upos})
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		for _, sense := range entry.CandidateSenses {
			if seen[candidate] || !sentenceExampleOverlap(sentence, surface, sense.Examples) {
				continue
			}
			seen[candidate] = true
			result = append(result, lemmarisk.Alternative{Lemma: candidate, UPOS: upos, Source: sense.Source, Version: sense.Version, EvidenceID: sense.EvidenceID, ContextRelevant: true})
			break
		}
	}
	return result, nil
}

func germanNounLemmaCandidates(surface string) []string {
	word := normalizeLemma("de", strings.TrimSpace(surface))
	if word == "" {
		return nil
	}
	candidates := []string{word}
	if strings.HasSuffix(word, "en") && len([]rune(word)) > 4 {
		candidates = append(candidates, strings.TrimSuffix(word, "en"), strings.TrimSuffix(word, "en")+"e")
	}
	if strings.HasSuffix(word, "n") && len([]rune(word)) > 3 {
		candidates = append(candidates, strings.TrimSuffix(word, "n"), strings.TrimSuffix(word, "n")+"e")
	}
	if strings.HasSuffix(word, "e") && len([]rune(word)) > 3 {
		candidates = append(candidates, strings.TrimSuffix(word, "e"))
	}
	unique := candidates[:0]
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		if candidate != "" && !seen[candidate] {
			seen[candidate] = true
			unique = append(unique, candidate)
		}
	}
	return unique
}

func sentenceExampleOverlap(sentence, surface string, examples []string) bool {
	context := evidenceWords(sentence, surface)
	if len(context) == 0 {
		return false
	}
	for _, example := range examples {
		matches := 0
		for word := range evidenceWords(example, surface) {
			if context[word] {
				matches++
			}
		}
		if matches >= 2 {
			return true
		}
	}
	return false
}

func evidenceWords(text, surface string) map[string]bool {
	words := make(map[string]bool)
	surface = strings.ToLower(strings.TrimSpace(surface))
	for _, field := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len([]rune(field)) < 4 || field == surface || germanEvidenceStopwords[field] {
			continue
		}
		words[field] = true
	}
	return words
}

func normalizeLemma(language, lemma string) string {
	normalized, err := canonicalization.Normalize(language, lemma)
	if err != nil {
		return ""
	}
	return normalized.CanonicalLemma
}
