package dictionary

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// indexFormatWithAncientGreekLSJ is the index metadata format that first
// carried the LSJ (Perseus) table alongside the Kaikki Ancient Greek entries. An
// index without that format (or with a lower one) carries no LSJ entries, so an
// Ancient Greek card shows no dictionary entry rather than a guess.
const indexFormatWithAncientGreekLSJ = 4

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
	rows, err := i.db.QueryContext(ctx, `SELECT entry_id, headword, senses_json FROM lsj_entries`)
	if err != nil {
		return fmt.Errorf("%w: lsj schema: %w", ErrInvalidIndex, err)
	}
	defer rows.Close() //nolint:errcheck // Rows.Err below reports actionable load failures.
	entries := make(map[string][]enrichment.AncientGreekLSJEntry)
	for rows.Next() {
		var entryID, headword, sensesJSON string
		if err = rows.Scan(&entryID, &headword, &sensesJSON); err != nil {
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
		entries[key] = append(entries[key], enrichment.AncientGreekLSJEntry{EntryID: entryID, Headword: headword, Pin: pin, Senses: senses})
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
// exactly one LSJ main entry: an unresolved homograph or a missing entry yields
// no entry, never a guess. The identity's lexical discriminator is never used,
// because an OGA morpheus:N number is not LSJ's numbering. When a headword has
// exactly one LSJ entry, that entry is the match for every identity that shares
// the headword: there is no second entry to borrow, and LSJ's hierarchy is shown
// whole rather than truncated per homograph.
func (i *Index) LookupAncientGreekLSJ(_ context.Context, lemma string) (enrichment.AncientGreekLSJEntry, bool) {
	if i == nil || !i.lsjAvailable {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	key, admitted := ancientGreekKey(lemma)
	if !admitted {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	candidates := i.lsjEntries[key]
	if len(candidates) != 1 {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	return candidates[0], true
}
