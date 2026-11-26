package dictionary

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"unicode"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// latinLanguage is the base language tag whose dedicated `latin` profile keys
// the Lewis & Short table.
const latinLanguage = "la"

// indexFormatWithLSHomograph is the index format whose ls_entries rows carry
// the source's homograph number.
const indexFormatWithLSHomograph = 8

// loadLatinLS loads the derived Lewis & Short entries and their pinned
// revision, keyed by the latin profile. Without an ls_pin the table was never
// derived and Latin reports unavailable; a pin with a missing table is an
// invalid index.
func (i *Index) loadLatinLS(ctx context.Context) error {
	pin, err := i.metadataValue(ctx, "ls_pin")
	if err != nil {
		return err
	}
	if pin == "" {
		return nil
	}
	// Format 8 added the source's homograph number; an older index has none.
	query := `SELECT entry_id, headword, orth, senses_json, 0 FROM ls_entries`
	if version, ok, formatErr := i.indexFormat(ctx); formatErr != nil {
		return formatErr
	} else if ok && version >= indexFormatWithLSHomograph {
		query = `SELECT entry_id, headword, orth, senses_json, homograph FROM ls_entries`
	}
	rows, err := i.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("%w: ls schema: %w", ErrInvalidIndex, err)
	}
	defer rows.Close() //nolint:errcheck // Rows.Err below reports actionable load failures.
	entries := make(map[string][]enrichment.LatinLSEntry)
	for rows.Next() {
		var entryID, headword, orth, sensesJSON string
		var homograph int
		if err = rows.Scan(&entryID, &headword, &orth, &sensesJSON, &homograph); err != nil {
			return fmt.Errorf("%w: ls row: %w", ErrInvalidIndex, err)
		}
		key, admitted := latinKey(headword)
		if !admitted {
			i.skippedLSHeadwords++
			continue
		}
		var senses []enrichment.LatinLSSense
		if err = json.Unmarshal([]byte(sensesJSON), &senses); err != nil {
			return fmt.Errorf("%w: ls senses: %w", ErrInvalidIndex, err)
		}
		entries[key] = append(entries[key], enrichment.LatinLSEntry{EntryID: entryID, Headword: headword, Orth: orth, Homograph: homograph, Pin: pin, Senses: senses})
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("%w: ls rows: %w", ErrInvalidIndex, err)
	}
	for _, group := range entries {
		slices.SortStableFunc(group, func(a, b enrichment.LatinLSEntry) int { return compareEntryID(a.EntryID, b.EntryID) })
	}
	i.lsEntries = entries
	i.lsAvailable = true
	i.lsPin = pin
	return nil
}

// LatinLSPin is the pinned Lewis & Short revision the index was derived from, or
// empty when the index carries no Lewis & Short entries.
func (i *Index) LatinLSPin() string {
	if i == nil {
		return ""
	}
	return i.lsPin
}

// latinKey derives the latin/1 key of a headword. A headword with no letter is
// not indexed.
func latinKey(headword string) (string, bool) {
	key := normalizeLemma(latinLanguage, headword)
	for _, r := range key {
		if unicode.IsLetter(r) {
			return key, true
		}
	}
	return "", false
}

var entryIDDigits = regexp.MustCompile(`\d+`)

// compareEntryID orders source entry ids by their numeric parts, so n9 precedes
// n10; ids equal in number fall back to text order.
func compareEntryID(a, b string) int {
	numbers := func(id string) []int {
		var out []int
		for _, digits := range entryIDDigits.FindAllString(id, -1) {
			if value, err := strconv.Atoi(digits); err == nil {
				out = append(out, value)
			}
		}
		return out
	}
	if c := slices.Compare(numbers(a), numbers(b)); c != 0 {
		return c
	}
	return slices.Compare([]byte(a), []byte(b))
}

// LatinLSAvailable reports whether the index carries Lewis & Short entries. An
// index derived without them reports the Latin path unavailable rather than an
// empty dictionary.
func (i *Index) LatinLSAvailable() bool {
	return i != nil && i.lsAvailable
}

// SkippedLSHeadwords reports how many Lewis & Short headwords were not indexed
// because their latin key was not admissible.
func (i *Index) SkippedLSHeadwords() int {
	if i == nil {
		return 0
	}
	return i.skippedLSHeadwords
}

// LookupLatinLS returns every Lewis & Short entry under the lemma's latin key,
// in entry id order. Homographs are returned together: nothing is chosen for
// the sentence, and Greek's exactly-one-candidate rule does not apply.
func (i *Index) LookupLatinLS(_ context.Context, lemma string) ([]enrichment.LatinLSEntry, bool) {
	if i == nil || !i.lsAvailable {
		return nil, false
	}
	key, admitted := latinKey(lemma)
	if !admitted {
		return nil, false
	}
	entries := i.lsEntries[key]
	if len(entries) == 0 {
		return nil, false
	}
	return slices.Clone(entries), true
}
