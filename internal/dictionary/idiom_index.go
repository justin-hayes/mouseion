package dictionary

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
)

// indexFormatWithIdiomEntries is the first non-LSJ index format carrying the
// word-to-multi-word idiom lookup (ticket #2361). LSJ indexes start at format
// 7. The table probe, not the version alone, is authoritative: a historical
// format-6 LSJ index has a number at or above the threshold but no tables.
const indexFormatWithIdiomEntries = 5

// loadIdiomAvailability records whether the index carries both idiom lookup
// tables. A missing table or an older format reports no idiom candidates
// rather than failing the whole index.
func (i *Index) loadIdiomAvailability(ctx context.Context) error {
	version, ok, err := i.indexFormat(ctx)
	if err != nil {
		return err
	}
	if !ok || version < indexFormatWithIdiomEntries {
		return nil
	}
	var available bool
	err = i.db.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'idiom_entries')
		AND EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'idiom_words')`).Scan(&available)
	if err != nil {
		return fmt.Errorf("%w: idiom schema: %w", ErrInvalidIndex, err)
	}
	i.idiomAvailable = available
	return nil
}

// IdiomEntries returns the multi-word entries containing word for a language.
// The word is normalized with the language's canonicalization profile, so a
// surface form and its lemma reach the same entries. Each entry carries its
// component words, senses, evidence source and version. An index without the
// idiom lookup returns no entries.
func (i *Index) IdiomEntries(ctx context.Context, language, word string) ([]idiom.Entry, error) {
	if i == nil || i.db == nil || !i.idiomAvailable {
		return nil, nil
	}
	language = canonicalization.NormalizeLanguage(language)
	word = normalizeLemma(language, word)
	if language == "" || word == "" {
		return nil, nil
	}
	rows, err := i.db.QueryContext(ctx, `SELECT e.headword, e.upos, e.words_json, e.senses_json, e.source, e.version
		FROM idiom_words AS w
		JOIN idiom_entries AS e
		  ON e.language = w.language
		 AND e.headword = w.headword
		 AND e.upos = w.upos
		 AND e.source = w.source
		WHERE w.language = ? AND w.word = ?
		ORDER BY e.headword, e.upos, e.source`, language, word)
	if err != nil {
		return nil, fmt.Errorf("dictionary idiom lookup: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Rows.Err below reports actionable lookup failures.
	entries := make([]idiom.Entry, 0, 2)
	for rows.Next() {
		var headword, upos, wordsJSON, sensesJSON, source, version string
		if err = rows.Scan(&headword, &upos, &wordsJSON, &sensesJSON, &source, &version); err != nil {
			return nil, fmt.Errorf("dictionary idiom row: %w", err)
		}
		var words []string
		if err = json.Unmarshal([]byte(wordsJSON), &words); err != nil {
			return nil, fmt.Errorf("dictionary idiom words: %w", err)
		}
		var senses []enrichment.LexicalSense
		if err = json.Unmarshal([]byte(sensesJSON), &senses); err != nil {
			return nil, fmt.Errorf("dictionary idiom senses: %w", err)
		}
		entries = append(entries, idiom.Entry{
			Language: language,
			Headword: headword,
			UPOS:     upos,
			Words:    words,
			Senses:   senses,
			Source:   source,
			Version:  version,
		})
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("dictionary idiom rows: %w", err)
	}
	return entries, nil
}
