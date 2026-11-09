package persistence

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// classicalAddLockSalt namespaces the per-owner/Reading-unit advisory lock that
// Add shares with the retired operator registration. Using the same key lets an
// Add and a registration of one unit serialize rather than racing the unique
// constraint.
const classicalAddLockSalt = 469

// AddClassicalReadingUnits gives the learner one Inbox Book per named current
// Reading unit, linked immutably to that unit and chosen for Ancient Greek. It
// downloads nothing, starts no job and never changes the active study language.
// Re-adding returns the existing Book, restoring an active membership and
// clearing Hidden, so the operation is idempotent.
func (s *PostgresStore) AddClassicalReadingUnits(ctx context.Context, owner string, readingUnitKeys []string) (classical.AddReport, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return classical.AddReport{}, errors.New("persistence: classical Add owner is required")
	}
	keys, err := normalizeClassicalAddKeys(readingUnitKeys)
	if err != nil {
		return classical.AddReport{}, err
	}
	legacy, err := s.ListLegacyLanguageRows(ctx, classical.Language)
	if err != nil {
		return classical.AddReport{}, fmt.Errorf("persistence: inspect %s rows: %w", classical.Language, err)
	}
	if len(legacy) > 0 {
		return classical.AddReport{}, classical.ErrLegacyRows
	}
	report := classical.AddReport{Books: make([]classical.AddedBook, 0, len(keys))}
	err = withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		if err := lockClassicalAddKeys(ctx, tx, owner, keys); err != nil {
			return err
		}
		if _, err := q.PutSupportedLanguage(ctx, sqlcgen.PutSupportedLanguageParams{
			Language: classical.Language, DisplayName: classical.LanguageDisplayName,
		}); err != nil {
			return err
		}
		for _, key := range keys {
			added, err := addClassicalReadingUnitTx(ctx, q, owner, key)
			if err != nil {
				return err
			}
			report.Books = append(report.Books, added)
		}
		return nil
	})
	if err != nil {
		return classical.AddReport{}, err
	}
	return report, nil
}

// normalizeClassicalAddKeys trims, rejects an empty part and removes duplicates
// while keeping constituent-book order, so a repeated key never adds twice.
func normalizeClassicalAddKeys(keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, errors.New("persistence: classical Add needs at least one Reading unit")
	}
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, errors.New("persistence: classical Add Reading unit key is required")
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out, nil
}

// lockClassicalAddKeys serializes one owner's Add against another Add or a
// registration of the same units. The sorted order makes concurrent Adds take
// locks consistently.
func lockClassicalAddKeys(ctx context.Context, tx pgx.Tx, owner string, keys []string) error {
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	for _, key := range sorted {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, $2))`, owner+":"+key, classicalAddLockSalt); err != nil {
			return err
		}
	}
	return nil
}

// addClassicalReadingUnitTx creates or reopens one Book for a current unit.
func addClassicalReadingUnitTx(ctx context.Context, q *sqlcgen.Queries, owner, key string) (classical.AddedBook, error) {
	unit, err := q.GetCurrentClassicalUnitByKey(ctx, key)
	err = missing(err)
	if errors.Is(err, ErrNotFound) {
		return classical.AddedBook{}, fmt.Errorf("classical: Reading unit %q is not current", key)
	}
	if err != nil {
		return classical.AddedBook{}, err
	}
	added := classical.AddedBook{ReadingUnitKey: unit.UnitKey, WorkKey: unit.WorkKey}
	existing, err := q.GetRegisteredBookID(ctx, sqlcgen.GetRegisteredBookIDParams{OwnerID: owner, ReadingUnitID: unit.ID})
	err = missing(err)
	switch {
	case err == nil:
		if err := reopenClassicalBookTx(ctx, q, owner, existing); err != nil {
			return classical.AddedBook{}, err
		}
		added.BookID = existing
		return added, nil
	case !errors.Is(err, ErrNotFound):
		return classical.AddedBook{}, err
	}
	book, err := q.InsertBook(ctx, sqlcgen.InsertBookParams{
		OwnerID:            owner,
		Title:              unit.WorkTitle,
		Author:             unit.WorkAuthor,
		MetadataProvenance: domain.MetadataProvenanceOperatorRegistration,
		LanguageState:      domain.LanguageChosen,
		LanguageTag:        textArg(classical.Language),
	})
	if err != nil {
		return classical.AddedBook{}, err
	}
	if err := q.InsertBookMembership(ctx, sqlcgen.InsertBookMembershipParams{OwnerID: owner, BookID: book.ID}); err != nil {
		return classical.AddedBook{}, err
	}
	if err := q.InsertInboxBookDisposition(ctx, sqlcgen.InsertInboxBookDispositionParams{OwnerID: owner, BookID: book.ID}); err != nil {
		return classical.AddedBook{}, err
	}
	if err := q.InsertBookReadingUnit(ctx, sqlcgen.InsertBookReadingUnitParams{OwnerID: owner, BookID: book.ID, ReadingUnitID: unit.ID}); err != nil {
		return classical.AddedBook{}, err
	}
	added.BookID = book.ID
	added.Created = true
	return added, nil
}

// reopenClassicalBookTx makes an existing Book reachable again without
// changing its disposition, Reading history or analysis. A removed membership
// returns to active and a Hidden Book becomes visible at the next revision.
func reopenClassicalBookTx(ctx context.Context, q *sqlcgen.Queries, owner, bookID string) error {
	visibility, err := q.GetBookVisibilityForUpdate(ctx, sqlcgen.GetBookVisibilityForUpdateParams{Owner: owner, Book: bookID})
	if err != nil {
		return missing(err)
	}
	if visibility.Hidden {
		if err := q.WriteBookVisibility(ctx, sqlcgen.WriteBookVisibilityParams{
			Owner: owner, Book: bookID, Hidden: false, Revision: visibility.Revision + 1,
		}); err != nil {
			return err
		}
	}
	return q.ActivateBookMembership(ctx, sqlcgen.ActivateBookMembershipParams{OwnerID: owner, BookID: bookID})
}

// ListOwnedClassicalReadingUnits returns the learner's active Books associated
// with a Reading unit. The catalog shows "In My Books" from this without
// exposing preparation state or coverage.
func (s *PostgresStore) ListOwnedClassicalReadingUnits(ctx context.Context, owner string) ([]classical.OwnedReadingUnit, error) {
	rows, err := s.queries().ListOwnedClassicalReadingUnits(ctx, owner)
	if err != nil {
		return nil, err
	}
	owned := make([]classical.OwnedReadingUnit, 0, len(rows))
	for _, row := range rows {
		owned = append(owned, classical.OwnedReadingUnit{
			ReadingUnitKey: row.UnitKey, WorkKey: row.WorkKey, BookID: row.BookID,
		})
	}
	return owned, nil
}
