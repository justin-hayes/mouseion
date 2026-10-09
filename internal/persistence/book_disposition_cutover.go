package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// The Set Aside retirement (ADR 0086) is a stopped-writer, data-only
// conversion that follows the structural migration 000033. It maps only
// still-legacy 'set_aside' dispositions to Inbox and initializes only missing
// visibility as visible. Stages: Plan (read-only manifest), Apply (one bounded
// transaction guarded by the reviewed manifest), Verify (read-only), and an
// optional guarded forward Correct after the application reopens.

const (
	legacySetAsideDisposition = "set_aside"
	dispositionCutoverVersion = 1
	// dispositionCutoverMaxChanges and ...MaxBooks bound the single
	// transaction. Larger inventories stop for operator review instead of being
	// converted with an unreviewed locking and volume impact.
	dispositionCutoverMaxChanges = 10_000
	dispositionCutoverMaxBooks   = 200_000
	dispositionCutoverLockWait   = "5s"
)

// Errors that stop the conversion for operator review.
var (
	ErrDispositionCutoverBlocked          = errors.New("book disposition cutover: inventory needs review")
	ErrDispositionCutoverManifestMismatch = errors.New("book disposition cutover: current state does not match the reviewed manifest")
	ErrDispositionCutoverStateChanged     = errors.New("book disposition cutover: row changed since cutover; refusing to overwrite a later learner choice")
)

// dispositionCutoverPreservedTables are compared by count and content hash
// before and after the conversion; the conversion must not change any of them.
var dispositionCutoverPreservedTables = []string{
	"books", "book_aliases", "book_membership", "book_covers", "book_current_analyses",
	"source_materials", "primary_goals", "primary_goal_snapshots", "primary_goal_snapshot_vocabulary",
	"reading_history", "known_vocabulary", "deck_preparations", "custom_vocabulary_deck_preparations",
}

// DispositionCutoverOwner is one learner's inventory. Nothing assumes a single
// learner; the operator reviews every owner.
type DispositionCutoverOwner struct {
	OwnerID            string `json:"owner_id"`
	Books              int64  `json:"books"`
	Inbox              int64  `json:"inbox"`
	ToRead             int64  `json:"to_read"`
	SetAside           int64  `json:"set_aside"`
	WithoutDisposition int64  `json:"without_disposition"`
	UnexpectedValues   int64  `json:"unexpected_values"`
	MissingVisibility  int64  `json:"missing_visibility"`
	Hidden             int64  `json:"hidden"`
	ActiveCommitments  int64  `json:"active_commitments"`
}

// DispositionCutoverChange records the original values of one row the
// conversion will change.
type DispositionCutoverChange struct {
	OwnerID          string    `json:"owner_id"`
	BookID           string    `json:"book_id"`
	Disposition      string    `json:"disposition"`
	Revision         int64     `json:"revision"`
	UpdatedAt        time.Time `json:"updated_at"`
	HistoryCategory  string    `json:"history_category"`
	ProjectedBucket  string    `json:"projected_bucket"`
	ConvertedTo      string    `json:"converted_to"`
	ActiveCommitment bool      `json:"active_commitment"`
}

// DispositionCutoverCommitment is an active Current reading that must survive
// the conversion untouched.
type DispositionCutoverCommitment struct {
	OwnerID     string  `json:"owner_id"`
	Language    string  `json:"language"`
	BookID      string  `json:"book_id"`
	SnapshotID  *string `json:"snapshot_id,omitempty"`
	Disposition string  `json:"disposition"`
}

// DispositionCutoverPreservation is the count and content hash of a table (or
// table subset) that the conversion must leave unchanged.
type DispositionCutoverPreservation struct {
	Name        string `json:"name"`
	Rows        int64  `json:"rows"`
	Fingerprint string `json:"fingerprint"`
}

// DispositionCutoverTotals are the expected affected counts.
type DispositionCutoverTotals struct {
	Owners                int64 `json:"owners"`
	Books                 int64 `json:"books"`
	ConvertedDispositions int64 `json:"converted_dispositions"`
	InitializedVisibility int64 `json:"initialized_visibility"`
	ProjectedInbox        int64 `json:"projected_inbox"`
	ProjectedRead         int64 `json:"projected_read"`
}

// DispositionCutoverManifest is the operator-reviewed description of exactly
// what the conversion will change and what it must preserve. Its fingerprint
// binds Apply to the reviewed state.
type DispositionCutoverManifest struct {
	Version      int                              `json:"version"`
	Owners       []DispositionCutoverOwner        `json:"owners"`
	Commitments  []DispositionCutoverCommitment   `json:"active_commitments"`
	Changes      []DispositionCutoverChange       `json:"changes"`
	Totals       DispositionCutoverTotals         `json:"totals"`
	Preservation []DispositionCutoverPreservation `json:"preservation"`
	Blockers     []string                         `json:"blockers"`
	Fingerprint  string                           `json:"fingerprint"`
}

// DispositionCutoverReport is the outcome of Apply.
type DispositionCutoverReport struct {
	CutoverID             string `json:"cutover_id,omitempty"`
	ManifestFingerprint   string `json:"manifest_fingerprint"`
	AlreadyApplied        bool   `json:"already_applied"`
	NothingToConvert      bool   `json:"nothing_to_convert"`
	ConvertedDispositions int    `json:"converted_dispositions"`
	InitializedVisibility int    `json:"initialized_visibility"`
}

// DispositionCutoverVerification is the read-only pre-reopen validation. Failures
// is empty exactly when every check passes.
type DispositionCutoverVerification struct {
	LegacyDispositions   int64    `json:"legacy_dispositions"`
	ConstraintValidated  bool     `json:"constraint_validated"`
	MissingVisibility    int64    `json:"missing_visibility"`
	Cutovers             int64    `json:"cutovers"`
	AuditedChanges       int64    `json:"audited_changes"`
	CheckpointedChanges  int64    `json:"checkpointed_changes"`
	ChangedSinceCutover  int64    `json:"changed_since_cutover"`
	ProjectionMismatches int64    `json:"projection_mismatches"`
	Failures             []string `json:"failures"`
}

type cutoverQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// PlanBookDispositionCutover inventories the database and returns the
// manifest. It is read-only and takes a consistent snapshot. A manifest with
// blockers must be reviewed and cannot be applied.
func (s *PostgresStore) PlanBookDispositionCutover(ctx context.Context) (manifest DispositionCutoverManifest, err error) {
	if s == nil || s.pool == nil {
		return manifest, errors.New("book disposition cutover: store is unavailable")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return manifest, fmt.Errorf("begin disposition cutover plan: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	return buildDispositionCutoverManifest(ctx, tx)
}

// ApplyBookDispositionCutover converts the still-legacy dispositions in one
// bounded transaction, provided the live database still matches the reviewed
// manifest exactly. Old application writers and affected workers must already
// be stopped. Rerunning after a commit (including a lost acknowledgement)
// returns the recorded outcome and changes nothing.
func (s *PostgresStore) ApplyBookDispositionCutover(ctx context.Context, reviewed DispositionCutoverManifest) (report DispositionCutoverReport, err error) {
	report.ManifestFingerprint = reviewed.Fingerprint
	if s == nil || s.pool == nil {
		return report, errors.New("book disposition cutover: store is unavailable")
	}
	if want, fpErr := dispositionManifestFingerprint(reviewed); fpErr != nil || want != reviewed.Fingerprint {
		return report, errors.Join(ErrDispositionCutoverManifestMismatch, errors.New("manifest was edited or is not a planned manifest"))
	}
	if len(reviewed.Blockers) > 0 {
		return report, fmt.Errorf("%w: %v", ErrDispositionCutoverBlocked, reviewed.Blockers)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return report, fmt.Errorf("begin disposition cutover: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout = '`+dispositionCutoverLockWait+`'`); err != nil {
		return report, fmt.Errorf("bound disposition cutover locking: %w", err)
	}
	lockTables := append([]string{"book_dispositions", "book_visibility", "book_disposition_cutovers"}, dispositionCutoverPreservedTables...)
	if _, err = tx.Exec(ctx, `LOCK TABLE `+strings.Join(lockTables, ", ")+` IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return report, fmt.Errorf("lock tables for disposition cutover (are writers stopped?): %w", err)
	}

	// A committed run with this exact manifest means a previous attempt succeeded
	// and only its acknowledgement was lost. The audit row is written in the same
	// transaction as the data, so its presence proves the data committed.
	var prior DispositionCutoverReport
	switch err = tx.QueryRow(ctx, `SELECT id::text, converted_dispositions, initialized_visibility
		FROM book_disposition_cutovers WHERE manifest_sha256 = $1`, reviewed.Fingerprint).
		Scan(&prior.CutoverID, &prior.ConvertedDispositions, &prior.InitializedVisibility); {
	case err == nil:
		prior.ManifestFingerprint = reviewed.Fingerprint
		prior.AlreadyApplied = true
		return prior, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return report, fmt.Errorf("read disposition cutover checkpoint: %w", err)
	}

	current, err := buildDispositionCutoverManifest(ctx, tx)
	if err != nil {
		return report, err
	}
	if current.Fingerprint != reviewed.Fingerprint {
		return report, ErrDispositionCutoverManifestMismatch
	}
	if len(current.Blockers) > 0 {
		return report, fmt.Errorf("%w: %v", ErrDispositionCutoverBlocked, current.Blockers)
	}

	if current.Totals.ConvertedDispositions == 0 && current.Totals.InitializedVisibility == 0 {
		// Nothing to map or initialize (fresh install, or a state already
		// converted by an earlier run). Only prove the constraint, no audit row.
		if _, err = tx.Exec(ctx, `ALTER TABLE book_dispositions VALIDATE CONSTRAINT book_dispositions_disposition_check`); err != nil {
			return report, fmt.Errorf("validate disposition constraint: %w", err)
		}
		if err = tx.Commit(ctx); err != nil {
			return report, fmt.Errorf("commit disposition cutover: %w", err)
		}
		report.NothingToConvert = true
		return report, nil
	}

	var cutoverID string
	manifestJSON, err := json.Marshal(current)
	if err != nil {
		return report, fmt.Errorf("encode disposition cutover manifest: %w", err)
	}
	if err = tx.QueryRow(ctx, `INSERT INTO book_disposition_cutovers(manifest_sha256, manifest, converted_dispositions, initialized_visibility)
		VALUES ($1, $2::jsonb, $3, $4) RETURNING id::text`,
		current.Fingerprint, string(manifestJSON), current.Totals.ConvertedDispositions, current.Totals.InitializedVisibility).Scan(&cutoverID); err != nil {
		return report, fmt.Errorf("record disposition cutover checkpoint: %w", err)
	}

	if len(current.Changes) > 0 {
		owners, books, revisions := make([]string, len(current.Changes)), make([]string, len(current.Changes)), make([]int64, len(current.Changes))
		for i, change := range current.Changes {
			owners[i], books[i], revisions[i] = change.OwnerID, change.BookID, change.Revision
		}
		// Guard original identity and revision: a row that is no longer exactly
		// the inventoried legacy row is left untouched and fails the count check.
		rows, queryErr := tx.Query(ctx, `UPDATE book_dispositions d
			SET disposition = 'inbox', revision = d.revision + 1, updated_at = now()
			FROM unnest($1::uuid[], $2::uuid[], $3::bigint[]) AS m(owner_id, book_id, revision)
			WHERE d.owner_id = m.owner_id AND d.book_id = m.book_id
			  AND d.disposition = 'set_aside' AND d.revision = m.revision
			RETURNING d.owner_id::text, d.book_id::text, d.revision`, owners, books, revisions)
		if queryErr != nil {
			return report, fmt.Errorf("convert legacy dispositions: %w", queryErr)
		}
		converted := make(map[[2]string]int64, len(current.Changes))
		for rows.Next() {
			var owner, book string
			var revision int64
			if err = rows.Scan(&owner, &book, &revision); err != nil {
				rows.Close()
				return report, fmt.Errorf("read converted dispositions: %w", err)
			}
			converted[[2]string{owner, book}] = revision
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return report, fmt.Errorf("read converted dispositions: %w", err)
		}
		rows.Close()
		if len(converted) != len(current.Changes) {
			return report, fmt.Errorf("convert legacy dispositions: converted %d of %d inventoried rows", len(converted), len(current.Changes))
		}
		for _, change := range current.Changes {
			if _, err = tx.Exec(ctx, `INSERT INTO book_disposition_cutover_changes(
					cutover_id, owner_id, book_id, original_disposition, original_revision, original_updated_at,
					converted_disposition, converted_revision, history_category)
				VALUES ($1, $2, $3, $4, $5, $6, 'inbox', $7, $8)`,
				cutoverID, change.OwnerID, change.BookID, change.Disposition, change.Revision, change.UpdatedAt,
				converted[[2]string{change.OwnerID, change.BookID}], change.HistoryCategory); err != nil {
				return report, fmt.Errorf("record original disposition values: %w", err)
			}
		}
	}

	// Every preserved table must be byte-for-byte unchanged by the mapping above.
	// Visibility is compared before initialization, and its Hidden rows after.
	after, err := dispositionCutoverPreservation(ctx, tx, cutoverID)
	if err != nil {
		return report, err
	}
	if err = comparePreservation(current.Preservation, after, dispositionPostMappingPreservation); err != nil {
		return report, err
	}

	tag, err := tx.Exec(ctx, `INSERT INTO book_visibility(owner_id, book_id, hidden, revision)
		SELECT b.owner_id, b.id, false, 1 FROM books b
		WHERE NOT EXISTS (SELECT 1 FROM book_visibility v WHERE v.owner_id = b.owner_id AND v.book_id = b.id)
		ON CONFLICT (owner_id, book_id) DO NOTHING`)
	if err != nil {
		return report, fmt.Errorf("initialize missing visibility: %w", err)
	}
	if tag.RowsAffected() != current.Totals.InitializedVisibility {
		return report, fmt.Errorf("initialize missing visibility: initialized %d of %d inventoried Books", tag.RowsAffected(), current.Totals.InitializedVisibility)
	}
	var hiddenFingerprint string
	var hiddenRows int64
	if err = tx.QueryRow(ctx, hiddenVisibilityFingerprintSQL).Scan(&hiddenRows, &hiddenFingerprint); err != nil {
		return report, fmt.Errorf("verify existing Hidden visibility: %w", err)
	}
	for _, before := range current.Preservation {
		if before.Name == "book_visibility.hidden" && (before.Rows != hiddenRows || before.Fingerprint != hiddenFingerprint) {
			return report, errors.New("verify existing Hidden visibility: Hidden rows changed during initialization")
		}
	}

	var remaining, missing int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM book_dispositions WHERE disposition NOT IN ('inbox','to_read')`).Scan(&remaining); err != nil {
		return report, fmt.Errorf("verify no legacy dispositions remain: %w", err)
	}
	if remaining != 0 {
		return report, fmt.Errorf("verify no legacy dispositions remain: %d rows remain", remaining)
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM books b WHERE NOT EXISTS (SELECT 1 FROM book_visibility v WHERE v.owner_id = b.owner_id AND v.book_id = b.id)`).Scan(&missing); err != nil {
		return report, fmt.Errorf("verify visibility is initialized: %w", err)
	}
	if missing != 0 {
		return report, fmt.Errorf("verify visibility is initialized: %d Books lack visibility", missing)
	}
	if _, err = tx.Exec(ctx, `ALTER TABLE book_dispositions VALIDATE CONSTRAINT book_dispositions_disposition_check`); err != nil {
		return report, fmt.Errorf("validate disposition constraint: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return report, fmt.Errorf("commit disposition cutover: %w", err)
	}
	report.CutoverID = cutoverID
	report.ConvertedDispositions = int(current.Totals.ConvertedDispositions)
	report.InitializedVisibility = int(current.Totals.InitializedVisibility)
	return report, nil
}

// VerifyBookDispositionCutover is the read-only validation that must pass
// before the application is reopened. It never repairs anything.
func (s *PostgresStore) VerifyBookDispositionCutover(ctx context.Context) (v DispositionCutoverVerification, err error) {
	v.Failures = make([]string, 0)
	if s == nil || s.pool == nil {
		return v, errors.New("book disposition cutover: store is unavailable")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return v, fmt.Errorf("begin disposition cutover verification: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	scan := func(what, query string, dest ...any) error {
		if scanErr := tx.QueryRow(ctx, query).Scan(dest...); scanErr != nil {
			return fmt.Errorf("verify %s: %w", what, scanErr)
		}
		return nil
	}
	if err = scan("legacy dispositions", `SELECT count(*) FROM book_dispositions WHERE disposition NOT IN ('inbox','to_read')`, &v.LegacyDispositions); err != nil {
		return v, err
	}
	if err = scan("disposition constraint", `SELECT COALESCE(bool_and(convalidated AND pg_get_constraintdef(oid) NOT LIKE '%set_aside%'), false)
		FROM pg_constraint WHERE conrelid = 'public.book_dispositions'::regclass AND conname = 'book_dispositions_disposition_check'`, &v.ConstraintValidated); err != nil {
		return v, err
	}
	if err = scan("visibility initialization", `SELECT count(*) FROM books b WHERE NOT EXISTS (SELECT 1 FROM book_visibility x WHERE x.owner_id = b.owner_id AND x.book_id = b.id)`, &v.MissingVisibility); err != nil {
		return v, err
	}
	if err = scan("checkpoint", `SELECT count(*), COALESCE(sum(converted_dispositions), 0)::bigint FROM book_disposition_cutovers`, &v.Cutovers, &v.CheckpointedChanges); err != nil {
		return v, err
	}
	if err = scan("audit", `SELECT count(*) FROM book_disposition_cutover_changes`, &v.AuditedChanges); err != nil {
		return v, err
	}
	// A converted row is still "as converted" while its revision is unchanged.
	// Later learner edits are expected and reported, not failures. An unedited
	// row must still be Inbox, and its history category must not have shrunk.
	if err = scan("audited rows", `SELECT
			count(*) FILTER (WHERE d.book_id IS NULL OR (d.revision <> c.converted_revision
				AND NOT (c.corrected_at IS NOT NULL AND d.revision = c.converted_revision + 1))),
			count(*) FILTER (WHERE d.revision = c.converted_revision AND (d.disposition <> c.converted_disposition
				OR (c.history_category <> 'unread' AND NOT EXISTS (
					SELECT 1 FROM reading_history h WHERE h.owner_id = c.owner_id AND h.book_id = c.book_id))))
		FROM book_disposition_cutover_changes c
		LEFT JOIN book_dispositions d ON d.owner_id = c.owner_id AND d.book_id = c.book_id`, &v.ChangedSinceCutover, &v.ProjectionMismatches); err != nil {
		return v, err
	}
	if v.LegacyDispositions != 0 {
		v.Failures = append(v.Failures, fmt.Sprintf("%d legacy dispositions remain", v.LegacyDispositions))
	}
	if !v.ConstraintValidated {
		v.Failures = append(v.Failures, "disposition constraint is missing, unvalidated, or still allows set_aside")
	}
	if v.MissingVisibility != 0 {
		v.Failures = append(v.Failures, fmt.Sprintf("%d Books lack visibility", v.MissingVisibility))
	}
	if v.AuditedChanges != v.CheckpointedChanges {
		v.Failures = append(v.Failures, fmt.Sprintf("checkpoint records %d conversions but audit holds %d", v.CheckpointedChanges, v.AuditedChanges))
	}
	if v.ProjectionMismatches != 0 {
		v.Failures = append(v.Failures, fmt.Sprintf("%d converted Books no longer project their recorded bucket", v.ProjectionMismatches))
	}
	return v, nil
}

// CorrectBookDispositionCutover is guarded forward correction after the
// application has reopened. It may re-decide a converted Book (Inbox or To
// Read) only while the disposition revision and visibility are exactly as the
// cutover left them and the Book's history category is unchanged; any later
// learner choice makes it refuse. A repeat of the same correction is a no-op.
func (s *PostgresStore) CorrectBookDispositionCutover(ctx context.Context, cutoverID, owner, bookID string, desired domain.BookDisposition) (applied bool, err error) {
	if err = desired.Validate(); err != nil {
		return false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	var bookLocked bool
	if err = tx.QueryRow(ctx, `SELECT true FROM books WHERE owner_id = $1 AND id = $2 FOR UPDATE`, owner, bookID).Scan(&bookLocked); errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	} else if err != nil {
		return false, err
	}
	var convertedRevision int64
	var category string
	var correctedTo *string
	if err = tx.QueryRow(ctx, `SELECT converted_revision, history_category, corrected_disposition
		FROM book_disposition_cutover_changes WHERE cutover_id = $1 AND owner_id = $2 AND book_id = $3 FOR UPDATE`,
		cutoverID, owner, bookID).Scan(&convertedRevision, &category, &correctedTo); errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	} else if err != nil {
		return false, err
	}
	var disposition string
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT disposition, revision FROM book_dispositions WHERE owner_id = $1 AND book_id = $2 FOR UPDATE`, owner, bookID).Scan(&disposition, &revision); err != nil {
		return false, err
	}
	if correctedTo != nil && *correctedTo == string(desired) && disposition == string(desired) && revision == convertedRevision+1 {
		return false, tx.Commit(ctx)
	}
	var visibilityRevision int64
	var hidden bool
	if err = tx.QueryRow(ctx, `SELECT revision, hidden FROM book_visibility WHERE owner_id = $1 AND book_id = $2 FOR UPDATE`, owner, bookID).Scan(&visibilityRevision, &hidden); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	var historyRows int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id = $1 AND book_id = $2`, owner, bookID).Scan(&historyRows); err != nil {
		return false, err
	}
	var current int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM primary_goals WHERE owner_id = $1 AND book_id = $2`, owner, bookID).Scan(&current); err != nil {
		return false, err
	}
	unchanged := correctedTo == nil && disposition == "inbox" && revision == convertedRevision &&
		(visibilityRevision == 0 || visibilityRevision == 1) && !hidden && current == 0 &&
		(historyRows > 0) == (category != "unread")
	if !unchanged {
		return false, ErrDispositionCutoverStateChanged
	}
	tag, err := tx.Exec(ctx, `UPDATE book_dispositions SET disposition = $3, revision = revision + 1, updated_at = now()
		WHERE owner_id = $1 AND book_id = $2 AND disposition = 'inbox' AND revision = $4`, owner, bookID, string(desired), convertedRevision)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		return false, ErrDispositionCutoverStateChanged
	}
	if _, err = tx.Exec(ctx, `UPDATE book_disposition_cutover_changes SET corrected_disposition = $4, corrected_at = now()
		WHERE cutover_id = $1 AND owner_id = $2 AND book_id = $3`, cutoverID, owner, bookID, string(desired)); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

const hiddenVisibilityFingerprintSQL = `SELECT count(*)::bigint, COALESCE(md5(string_agg(h, '' ORDER BY h)), '')
	FROM (SELECT md5(t::text) AS h FROM book_visibility t WHERE t.hidden) s`

func dispositionManifestFingerprint(m DispositionCutoverManifest) (string, error) {
	m.Fingerprint = ""
	raw, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// dispositionPostMappingPreservation names the preserved facts that must be
// identical after the disposition mapping but before visibility initialization.
func dispositionPostMappingPreservation(name string) bool {
	return name != "book_visibility.hidden"
}

func comparePreservation(before, after []DispositionCutoverPreservation, applies func(string) bool) error {
	afterByName := make(map[string]DispositionCutoverPreservation, len(after))
	for _, item := range after {
		afterByName[item.Name] = item
	}
	for _, item := range before {
		if !applies(item.Name) {
			continue
		}
		got, ok := afterByName[item.Name]
		if !ok || got.Rows != item.Rows || got.Fingerprint != item.Fingerprint {
			return fmt.Errorf("verify preservation: %s changed during the disposition conversion", item.Name)
		}
	}
	return nil
}

// dispositionCutoverPreservation fingerprints everything the conversion must
// leave unchanged. After the mapping, excludeCutover names the audit run whose
// converted rows are removed from the "unchanged" comparison.
func dispositionCutoverPreservation(ctx context.Context, q cutoverQuerier, excludeCutover string) ([]DispositionCutoverPreservation, error) {
	checks := make([]DispositionCutoverPreservation, 0, len(dispositionCutoverPreservedTables)+4)
	hash := func(name, query string, args ...any) error {
		item := DispositionCutoverPreservation{Name: name}
		if err := q.QueryRow(ctx, query, args...).Scan(&item.Rows, &item.Fingerprint); err != nil {
			return fmt.Errorf("fingerprint %s: %w", name, err)
		}
		checks = append(checks, item)
		return nil
	}
	rowHash := func(source string) string {
		return `SELECT count(*)::bigint, COALESCE(md5(string_agg(h, '' ORDER BY h)), '') FROM (SELECT md5(t::text) AS h FROM ` + source + ` t) s`
	}
	for _, table := range dispositionCutoverPreservedTables {
		if err := hash(table, rowHash(table)); err != nil {
			return nil, err
		}
	}
	// Dispositions: every row's identity and creation time, plus the full
	// content of rows the conversion must not touch.
	if err := hash("book_dispositions.identity", rowHash(`(SELECT owner_id, book_id, created_at FROM book_dispositions)`)); err != nil {
		return nil, err
	}
	unchanged := `(SELECT * FROM book_dispositions WHERE disposition <> 'set_aside' AND $1::text = '')`
	if excludeCutover != "" {
		unchanged = `(SELECT d.* FROM book_dispositions d WHERE $1::text <> '' AND NOT EXISTS (
			SELECT 1 FROM book_disposition_cutover_changes c
			WHERE c.cutover_id = $1::uuid AND c.owner_id = d.owner_id AND c.book_id = d.book_id))`
	}
	if err := hash("book_dispositions.unchanged", rowHash(unchanged), excludeCutover); err != nil {
		return nil, err
	}
	if err := hash("book_visibility", rowHash(`book_visibility`)); err != nil {
		return nil, err
	}
	if err := hash("book_visibility.hidden", hiddenVisibilityFingerprintSQL); err != nil {
		return nil, err
	}
	return checks, nil
}

func buildDispositionCutoverManifest(ctx context.Context, q cutoverQuerier) (DispositionCutoverManifest, error) {
	m := DispositionCutoverManifest{Version: dispositionCutoverVersion}
	m.Owners, m.Commitments, m.Changes, m.Blockers = []DispositionCutoverOwner{}, []DispositionCutoverCommitment{}, []DispositionCutoverChange{}, []string{}

	rows, err := q.Query(ctx, `SELECT b.owner_id::text, count(*)::bigint,
			count(*) FILTER (WHERE d.disposition = 'inbox')::bigint,
			count(*) FILTER (WHERE d.disposition = 'to_read')::bigint,
			count(*) FILTER (WHERE d.disposition = 'set_aside')::bigint,
			count(*) FILTER (WHERE d.book_id IS NULL)::bigint,
			count(*) FILTER (WHERE d.book_id IS NOT NULL AND d.disposition NOT IN ('inbox','to_read','set_aside'))::bigint,
			count(*) FILTER (WHERE v.book_id IS NULL)::bigint,
			count(*) FILTER (WHERE v.hidden)::bigint,
			(SELECT count(*) FROM primary_goals g WHERE g.owner_id = b.owner_id)::bigint
		FROM books b
		LEFT JOIN book_dispositions d ON d.owner_id = b.owner_id AND d.book_id = b.id
		LEFT JOIN book_visibility v ON v.owner_id = b.owner_id AND v.book_id = b.id
		GROUP BY b.owner_id ORDER BY b.owner_id`)
	if err != nil {
		return m, fmt.Errorf("inventory owners: %w", err)
	}
	for rows.Next() {
		var o DispositionCutoverOwner
		if err = rows.Scan(&o.OwnerID, &o.Books, &o.Inbox, &o.ToRead, &o.SetAside, &o.WithoutDisposition, &o.UnexpectedValues, &o.MissingVisibility, &o.Hidden, &o.ActiveCommitments); err != nil {
			rows.Close()
			return m, fmt.Errorf("read owner inventory: %w", err)
		}
		m.Owners = append(m.Owners, o)
		m.Totals.Books += o.Books
		m.Totals.InitializedVisibility += o.MissingVisibility
		if o.UnexpectedValues > 0 {
			m.Blockers = append(m.Blockers, fmt.Sprintf("owner %s has %d dispositions outside inbox/to_read/set_aside", o.OwnerID, o.UnexpectedValues))
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return m, fmt.Errorf("read owner inventory: %w", err)
	}
	rows.Close()
	m.Totals.Owners = int64(len(m.Owners))

	rows, err = q.Query(ctx, `SELECT g.owner_id::text, g.language, g.book_id::text, g.snapshot_id::text, COALESCE(d.disposition, '')
		FROM primary_goals g
		LEFT JOIN book_dispositions d ON d.owner_id = g.owner_id AND d.book_id = g.book_id
		ORDER BY g.owner_id, g.language, g.book_id`)
	if err != nil {
		return m, fmt.Errorf("inventory active commitments: %w", err)
	}
	for rows.Next() {
		var c DispositionCutoverCommitment
		if err = rows.Scan(&c.OwnerID, &c.Language, &c.BookID, &c.SnapshotID, &c.Disposition); err != nil {
			rows.Close()
			return m, fmt.Errorf("read active commitment: %w", err)
		}
		m.Commitments = append(m.Commitments, c)
		if c.Disposition != string(domain.BookDispositionToRead) {
			m.Blockers = append(m.Blockers, fmt.Sprintf("active commitment %s/%s/%s is %q, not to_read", c.OwnerID, c.Language, c.BookID, c.Disposition))
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return m, fmt.Errorf("read active commitments: %w", err)
	}
	rows.Close()

	rows, err = q.Query(ctx, `SELECT d.owner_id::text, d.book_id::text, d.disposition, d.revision, d.updated_at,
			CASE WHEN h.completion > 0 AND h.assertion > 0 THEN 'mixed'
			     WHEN h.completion > 0 THEN 'completion'
			     WHEN h.assertion > 0 THEN 'assertion'
			     ELSE 'unread' END,
			EXISTS (SELECT 1 FROM primary_goals g WHERE g.owner_id = d.owner_id AND g.book_id = d.book_id)
		FROM book_dispositions d
		CROSS JOIN LATERAL (
			SELECT count(*) FILTER (WHERE rh.completion_source = 'primary_goal') AS completion,
			       count(*) FILTER (WHERE rh.completion_source = 'previously_read_import') AS assertion
			FROM reading_history rh WHERE rh.owner_id = d.owner_id AND rh.book_id = d.book_id) h
		WHERE d.disposition = 'set_aside'
		ORDER BY d.owner_id, d.book_id`)
	if err != nil {
		return m, fmt.Errorf("inventory legacy dispositions: %w", err)
	}
	for rows.Next() {
		var c DispositionCutoverChange
		if err = rows.Scan(&c.OwnerID, &c.BookID, &c.Disposition, &c.Revision, &c.UpdatedAt, &c.HistoryCategory, &c.ActiveCommitment); err != nil {
			rows.Close()
			return m, fmt.Errorf("read legacy disposition: %w", err)
		}
		c.UpdatedAt = c.UpdatedAt.UTC()
		c.ConvertedTo = string(domain.BookDispositionInbox)
		c.ProjectedBucket = "inbox"
		if c.HistoryCategory != "unread" {
			c.ProjectedBucket = "read"
			m.Totals.ProjectedRead++
		} else {
			m.Totals.ProjectedInbox++
		}
		if c.ActiveCommitment {
			m.Blockers = append(m.Blockers, fmt.Sprintf("legacy Set Aside Book %s/%s is an active commitment", c.OwnerID, c.BookID))
		}
		m.Changes = append(m.Changes, c)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return m, fmt.Errorf("read legacy dispositions: %w", err)
	}
	rows.Close()
	m.Totals.ConvertedDispositions = int64(len(m.Changes))

	if m.Totals.ConvertedDispositions > dispositionCutoverMaxChanges {
		m.Blockers = append(m.Blockers, fmt.Sprintf("%d legacy dispositions exceed the bounded transaction limit of %d", m.Totals.ConvertedDispositions, dispositionCutoverMaxChanges))
	}
	if m.Totals.Books > dispositionCutoverMaxBooks {
		m.Blockers = append(m.Blockers, fmt.Sprintf("%d Books exceed the bounded transaction limit of %d", m.Totals.Books, dispositionCutoverMaxBooks))
	}

	if m.Preservation, err = dispositionCutoverPreservation(ctx, q, ""); err != nil {
		return m, err
	}
	if m.Fingerprint, err = dispositionManifestFingerprint(m); err != nil {
		return m, fmt.Errorf("fingerprint disposition cutover manifest: %w", err)
	}
	return m, nil
}

// ErrLegacyDispositionsRemain is returned at startup while a database still
// holds a retired disposition; the application must stay closed until the
// stopped-writer conversion has run.
var ErrLegacyDispositionsRemain = errors.New("legacy Book dispositions remain; run the Set Aside retirement cutover (doc/cutovers/2026-10-set-aside-retirement.md) before starting the application")

// RequireNoLegacyDispositions refuses to open the application over unconverted
// data, so a reopened server can never read or write a retired value.
func (s *PostgresStore) RequireNoLegacyDispositions(ctx context.Context) error {
	var remaining int64
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM book_dispositions WHERE disposition NOT IN ('inbox','to_read')`).Scan(&remaining); err != nil {
		return fmt.Errorf("check legacy dispositions: %w", err)
	}
	if remaining > 0 {
		return fmt.Errorf("%w (%d rows)", ErrLegacyDispositionsRemain, remaining)
	}
	return nil
}
