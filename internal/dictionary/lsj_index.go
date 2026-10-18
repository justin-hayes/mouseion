package dictionary

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// indexFormatWithAncientGreekLSJ is the index metadata format that carries the
// LSJ (Perseus) table alongside the Kaikki Ancient Greek entries, with each
// entry's sub-entry classification. A lower format has no LSJ entries, so an
// Ancient Greek card shows no dictionary entry rather than a guess.
const indexFormatWithAncientGreekLSJ = 5

// loadAncientGreekLSJ loads the derived LSJ entries and the pinned revision
// they came from. It reports the LSJ path unavailable for an index older than
// the LSJ format, and treats a missing table as an invalid index.
func (i *Index) loadAncientGreekLSJ(ctx context.Context) error {
	version, ok, err := i.indexFormat(ctx)
	if err != nil {
		return err
	}
	if !ok || version < indexFormatWithAncientGreekLSJ {
		return nil
	}
	pin, err := i.metadataValue(ctx, "lsj_pin")
	if err != nil {
		return err
	}
	rows, err := i.db.QueryContext(ctx, `SELECT entry_id, headword, senses_json, subordinate FROM lsj_entries`)
	if err != nil {
		return fmt.Errorf("%w: lsj schema: %w", ErrInvalidIndex, err)
	}
	defer rows.Close() //nolint:errcheck // Rows.Err below reports actionable load failures.
	entries := make(map[string][]lsjEntry)
	for rows.Next() {
		var entryID, headword, sensesJSON string
		var subordinate int
		if err = rows.Scan(&entryID, &headword, &sensesJSON, &subordinate); err != nil {
			return fmt.Errorf("%w: lsj row: %w", ErrInvalidIndex, err)
		}
		key, admitted := ancientGreekKey(headword)
		if !admitted {
			i.skippedLSJHeadwords++
			continue
		}
		var senses []enrichment.AncientGreekLSJSense
		if err = json.Unmarshal([]byte(sensesJSON), &senses); err != nil {
			return fmt.Errorf("%w: lsj senses: %w", ErrInvalidIndex, err)
		}
		entries[key] = append(entries[key], lsjEntry{
			entry:       enrichment.AncientGreekLSJEntry{EntryID: entryID, Headword: headword, Pin: pin, Senses: senses},
			subordinate: subordinate != 0,
		})
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("%w: lsj rows: %w", ErrInvalidIndex, err)
	}
	i.lsjEntries = entries
	i.lsjAvailable = true
	return nil
}

// indexFormat reads the index metadata format version. ok=false means the
// metadata row is absent.
func (i *Index) indexFormat(ctx context.Context) (version int, ok bool, err error) {
	var format string
	if err = i.db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'index_format'`).Scan(&format); errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("%w: index format: %w", ErrInvalidIndex, err)
	}
	if parsed, convErr := strconv.Atoi(strings.TrimSpace(format)); convErr == nil {
		return parsed, true, nil
	}
	return 0, false, nil
}

// metadataValue reads one metadata value, treating an absent key as empty.
func (i *Index) metadataValue(ctx context.Context, key string) (string, error) {
	var value string
	err := i.db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("%w: metadata %s: %w", ErrInvalidIndex, key, err)
	}
	return strings.TrimSpace(value), nil
}

// AncientGreekLSJAvailable reports whether the index carries LSJ entries. An
// older index reports the LSJ path unavailable rather than an empty dictionary.
func (i *Index) AncientGreekLSJAvailable() bool {
	return i != nil && i.lsjAvailable
}

// SkippedLSJHeadwords reports how many LSJ source headwords were not indexed
// because their ancient-greek/1 key was not admissible.
func (i *Index) SkippedLSJHeadwords() int {
	if i == nil {
		return 0
	}
	return i.skippedLSJHeadwords
}

// LookupAncientGreekLSJ resolves an Ancient Greek headword to its matched LSJ
// entry. A match is returned only when the canonical headword key selects
// exactly one competing entry: an unresolved homograph or a missing entry yields
// no entry, never a guess. A phrase or redirect sub-entry filed under the bare
// word's key never competes, and a capitalised proper-name entry does not make a
// lowercase common word ambiguous. The identity's lexical discriminator is never
// used, because an OGA morpheus:N number is not LSJ's numbering. When a headword
// has exactly one competing entry, that entry is the match for every identity
// that shares the headword: there is no second entry to borrow, and LSJ's
// hierarchy is shown whole rather than truncated per homograph.
func (i *Index) LookupAncientGreekLSJ(_ context.Context, lemma string) (enrichment.AncientGreekLSJEntry, bool) {
	if i == nil || !i.lsjAvailable {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	key, admitted := ancientGreekKey(lemma)
	if !admitted {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	candidates := competingLSJEntries(i.lsjEntries[key], lemma)
	if len(candidates) != 1 {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	return candidates[0], true
}

// lsjEntry is one loaded LSJ source entry with the derivation's sub-entry
// classification. A subordinate entry is a phrase or a redirect stub LSJ filed
// under the bare word's key; it never competes as a homograph.
type lsjEntry struct {
	entry       enrichment.AncientGreekLSJEntry
	subordinate bool
}

// competingLSJEntries returns the entries that can be the bare lemma's own
// entry. A subordinate sub-entry never competes. A capitalised proper-name entry
// does not compete with a lowercase lemma while a lowercase entry exists, but
// entries of the other capitalisation are still the lemma's evidence when they
// are the only ones (for example the common adjective Ἑλληνικός).
func competingLSJEntries(entries []lsjEntry, lemma string) []enrichment.AncientGreekLSJEntry {
	capitalised := headwordIsCapitalised(lemma)
	sameCase := make([]enrichment.AncientGreekLSJEntry, 0, len(entries))
	otherCase := make([]enrichment.AncientGreekLSJEntry, 0, len(entries))
	for _, candidate := range entries {
		if candidate.subordinate {
			continue
		}
		if headwordIsCapitalised(candidate.entry.Headword) == capitalised {
			sameCase = append(sameCase, candidate.entry)
		} else {
			otherCase = append(otherCase, candidate.entry)
		}
	}
	if len(sameCase) > 0 {
		return sameCase
	}
	return otherCase
}

// headwordIsCapitalised reports whether a headword's first letter is uppercase.
func headwordIsCapitalised(headword string) bool {
	for _, r := range headword {
		if unicode.IsLetter(r) {
			return unicode.IsUpper(r)
		}
	}
	return false
}
