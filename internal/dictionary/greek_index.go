package dictionary

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// ancientGreekLanguage is the dedicated-profile language whose entries the
// index keys in memory rather than in SQL.
const ancientGreekLanguage = "grc"

// indexFormatWithAncientGreek is the index metadata format that first carried
// the greek_entries table. An index without that format (or with a lower one)
// has no Ancient Greek entries, so grc is reported unavailable.
const indexFormatWithAncientGreek = 2

// greekEntry is one source Ancient Greek dictionary entry, keyed in memory by
// its computed canonical lemma.
type greekEntry struct {
	upos   string
	senses []enrichment.LexicalSense
}

// loadGreekEntries computes each Ancient Greek source entry's ancient-greek/1
// key when the index opens, so lookups match on the key rather than on a stored
// one. A headword whose key is not admissible is skipped and counted. An index
// without the Ancient Greek format leaves grc unavailable rather than merely
// empty.
func (i *Index) loadGreekEntries(ctx context.Context) error {
	version, ok, err := i.indexFormat(ctx)
	if err != nil {
		return err
	}
	if !ok || version < indexFormatWithAncientGreek {
		return nil
	}
	rows, err := i.db.QueryContext(ctx, `SELECT headword, upos, senses_json FROM greek_entries`)
	if err != nil {
		return fmt.Errorf("%w: ancient greek schema: %w", ErrInvalidIndex, err)
	}
	defer rows.Close() //nolint:errcheck // Rows.Err below reports actionable load failures.
	entries := make(map[string][]greekEntry)
	for rows.Next() {
		var headword, upos, sensesJSON string
		if err = rows.Scan(&headword, &upos, &sensesJSON); err != nil {
			return fmt.Errorf("%w: ancient greek row: %w", ErrInvalidIndex, err)
		}
		key, admitted := ancientGreekKey(headword)
		if !admitted {
			i.skippedGreekHeadwords++
			continue
		}
		var senses []enrichment.LexicalSense
		if err = json.Unmarshal([]byte(sensesJSON), &senses); err != nil {
			return fmt.Errorf("%w: ancient greek senses: %w", ErrInvalidIndex, err)
		}
		entries[key] = append(entries[key], greekEntry{upos: strings.ToUpper(strings.TrimSpace(upos)), senses: senses})
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("%w: ancient greek rows: %w", ErrInvalidIndex, err)
	}
	i.greekEntries = entries
	i.greekAvailable = true
	return nil
}

// ancientGreekKey derives the ancient-greek/1 canonical key from a source
// headword. A headword with no admissible key (empty, or carrying no letter)
// is not indexed.
func ancientGreekKey(headword string) (string, bool) {
	key := normalizeLemma(ancientGreekLanguage, headword)
	if key == "" {
		return "", false
	}
	for _, r := range key {
		if unicode.IsLetter(r) {
			return key, true
		}
	}
	return "", false
}

// AncientGreekAvailable reports whether the index carries Ancient Greek
// entries. An older index reports grc as unavailable rather than as an
// unindexed language.
func (i *Index) AncientGreekAvailable() bool {
	return i != nil && i.greekAvailable
}

// SkippedGreekHeadwords reports how many Ancient Greek source headwords were
// not indexed because their ancient-greek/1 key was not admissible.
func (i *Index) SkippedGreekHeadwords() int {
	if i == nil {
		return 0
	}
	return i.skippedGreekHeadwords
}

// ancientGreekRows selects every source entry whose key and part of speech
// match. The lexical discriminator is never used: a request without one still
// gets every matching entry. A source entry with no mapped part of speech is
// weaker lemma-only evidence, as for the modern languages.
func (i *Index) ancientGreekRows(lemma, upos string) (exactSenses, weakSenses []enrichment.LexicalSense, foundExact bool) {
	for _, candidate := range i.greekEntries[lemma] {
		switch {
		case upos != "" && candidate.upos == upos:
			foundExact = true
			exactSenses = append(exactSenses, enrichment.CloneLexicalSenses(candidate.senses)...)
		case candidate.upos == "":
			weak := enrichment.CloneLexicalSenses(candidate.senses)
			for index := range weak {
				weak[index].MatchStrength = "lemma_only_missing_pos"
			}
			weakSenses = append(weakSenses, weak...)
		}
	}
	return exactSenses, weakSenses, foundExact
}
