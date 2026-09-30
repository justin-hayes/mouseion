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
	return index, nil
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
	rows, err := i.db.QueryContext(ctx, `SELECT upos, senses_json, gender, article, plural, ipa, principal_parts FROM entries WHERE language = ? AND lemma = ? AND (upos = ? OR upos = '') ORDER BY CASE WHEN upos = ? THEN 0 ELSE 1 END`, language, lemma, upos, upos)
	if err != nil {
		return enrichment.LexicalEntry{}, false, fmt.Errorf("dictionary lookup: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Rows.Err below reports actionable lookup failures.
	var exactSenses, weakSenses []enrichment.LexicalSense
	var gender, article, plural, ipa, principalParts string
	foundExact := false
	for rows.Next() {
		var entryUPOS, sensesJSON, rowGender, rowArticle, rowPlural, rowIPA, rowPrincipalParts string
		if err = rows.Scan(&entryUPOS, &sensesJSON, &rowGender, &rowArticle, &rowPlural, &rowIPA, &rowPrincipalParts); err != nil {
			return enrichment.LexicalEntry{}, false, fmt.Errorf("dictionary lookup row: %w", err)
		}
		var senses []enrichment.LexicalSense
		if err = json.Unmarshal([]byte(sensesJSON), &senses); err != nil {
			return enrichment.LexicalEntry{}, false, fmt.Errorf("dictionary senses: %w", err)
		}
		exactMatch := entryUPOS == upos && upos != ""
		if exactMatch {
			foundExact = true
			gender, article, plural, ipa, principalParts = rowGender, rowArticle, rowPlural, rowIPA, rowPrincipalParts
		} else {
			for senseIndex := range senses {
				senses[senseIndex].MatchStrength = "lemma_only_missing_pos"
			}
		}
		if exactMatch {
			exactSenses = append(exactSenses, senses...)
		} else {
			weakSenses = append(weakSenses, senses...)
		}
	}
	if err = rows.Err(); err != nil {
		return enrichment.LexicalEntry{}, false, fmt.Errorf("dictionary lookup rows: %w", err)
	}
	if !foundExact && len(weakSenses) == 0 {
		return enrichment.LexicalEntry{}, false, nil
	}
	senses := append(exactSenses, weakSenses...)
	seenEvidenceIDs := make(map[string]struct{}, len(senses))
	for index := 0; index < len(senses); {
		sense := &senses[index]
		if sense.EvidenceID == "" {
			evidenceUPOS := upos
			if sense.MatchStrength == "lemma_only_missing_pos" {
				evidenceUPOS = ""
			}
			identity, marshalErr := json.Marshal(evidenceIdentity{Language: language, Lemma: lemma, UPOS: evidenceUPOS, Gloss: sense.Gloss, Phrase: sense.Phrase, Examples: sense.Examples, Topics: sense.Topics, Tags: sense.Tags})
			if marshalErr != nil {
				return enrichment.LexicalEntry{}, false, fmt.Errorf("dictionary evidence identity: %w", marshalErr)
			}
			digest := sha256.Sum256(identity)
			sense.EvidenceID = "wiktionary:" + hex.EncodeToString(digest[:])
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
			sense.Version = i.version
		}
		if sense.MatchStrength == "" {
			sense.MatchStrength = "exact_lemma_pos"
		}
		if _, duplicate := seenEvidenceIDs[sense.EvidenceID]; duplicate {
			senses = append(senses[:index], senses[index+1:]...)
			continue
		}
		seenEvidenceIDs[sense.EvidenceID] = struct{}{}
		index++
	}
	var orderedExactSenses, orderedWeakSenses []enrichment.LexicalSense
	for _, sense := range senses {
		if sense.MatchStrength == "lemma_only_missing_pos" {
			orderedWeakSenses = append(orderedWeakSenses, sense)
		} else {
			orderedExactSenses = append(orderedExactSenses, sense)
		}
	}
	orderedExact := enrichment.OrderSenses(request, orderedExactSenses)
	orderedWeak := enrichment.OrderSenses(request, orderedWeakSenses)
	ordered := append(orderedExact, orderedWeak...)
	result := enrichment.LexicalEntry{Senses: ordered, CandidateSenses: append([]enrichment.LexicalSense(nil), senses...), IPA: ipa, PrincipalParts: principalParts}
	if language == "el" {
		result.Gender = gender
		result.Article = article
		result.Plural = plural
	} else if len(ordered) > 0 {
		result.Gender = ordered[0].Gender
		result.Article = ordered[0].Article
		result.Plural = ordered[0].Plural
	}
	return result, true, nil
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
