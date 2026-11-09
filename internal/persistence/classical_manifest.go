package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/justin-hayes/mouseion/internal/classical"
)

// classicalManifestLockSalt namespaces the installation-wide advisory lock
// taken while reconciling the embedded manifest, so a multi-process rollout
// serializes startup reconciliation.
const classicalManifestLockSalt = 470

// storedClassicalUnit is the previously reconciled state a startup comparison
// needs. TranslationLayer is recorded only once reconciliation has seen the
// release that carries it, so a legacy row's absent pin is not a change.
type storedClassicalUnit struct {
	EditionID             string
	Revision              int
	Withdrawn             bool
	SourceLayer           string
	SourceFileName        string
	SourceFileSize        int64
	SourceFileSHA256      string
	SourceFormat          string
	InterpretationVersion string
	Translation           *classical.TranslationPin
}

// ReconcileClassicalManifest writes one release's Reading-unit definitions into
// the shared Work, Edition and Reading-unit registry in one idempotent
// transaction. It inserts new keys, corrects curatorial fields in place, applies
// content only under a revision bump and withdraws entries the manifest no
// longer carries. It never creates a learner Book or touches learner state.
func (s *PostgresStore) ReconcileClassicalManifest(ctx context.Context, definitions []classical.Definition) error {
	if err := classical.ValidateManifest(definitions); err != nil {
		return err
	}
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		return reconcileClassicalManifestTx(ctx, tx, definitions)
	})
}

// reconcileClassicalManifestTx is the one-transaction reconciliation. It
// serializes across processes, refuses a broken revision before writing, then
// corrects and inserts every definition before withdrawing absent ones.
func reconcileClassicalManifestTx(ctx context.Context, tx pgx.Tx, definitions []classical.Definition) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, classicalManifestLockSalt); err != nil {
		return err
	}
	stored, err := loadStoredClassicalUnits(ctx, tx)
	if err != nil {
		return err
	}
	if err := validateStoredRevisions(definitions, stored); err != nil {
		return err
	}
	if err := upsertClassicalDefinitions(ctx, tx, definitions); err != nil {
		return err
	}
	if err := applyCurrentClassicalMetadata(ctx, tx, definitions); err != nil {
		return err
	}
	if err := withdrawAbsentClassicalUnits(ctx, tx, definitions); err != nil {
		return err
	}
	return pruneMergedClassicalRows(ctx, tx, definitions)
}

// upsertClassicalDefinitions writes each Work, Edition and Reading unit.
func upsertClassicalDefinitions(ctx context.Context, tx pgx.Tx, definitions []classical.Definition) error {
	for _, definition := range definitions {
		workID, err := reconcileClassicalWork(ctx, tx, definition)
		if err != nil {
			return err
		}
		editionID, err := reconcileClassicalEdition(ctx, tx, definition, workID)
		if err != nil {
			return err
		}
		if err := upsertClassicalReadingUnit(ctx, tx, definition, editionID); err != nil {
			return err
		}
	}
	return nil
}

// loadStoredClassicalUnits reads every reconciled Reading unit with the fields
// that decide a revision refusal.
func loadStoredClassicalUnits(ctx context.Context, tx pgx.Tx) (map[string]storedClassicalUnit, error) {
	rows, err := tx.Query(ctx, `
		SELECT definition_key, edition_id::text, revision, withdrawn,
		       source_layer, source_file_name, source_file_size, source_file_sha256, source_format, interpretation_version,
		       translation_layer, translation_file_name, translation_file_size,
		       translation_file_sha256, translation_citation_start, translation_citation_end
		FROM reading_units
		FOR UPDATE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stored := map[string]storedClassicalUnit{}
	for rows.Next() {
		var key string
		var unit storedClassicalUnit
		var layer, fileName, sha256, citationStart, citationEnd pgtype.Text
		var size pgtype.Int8
		if err := rows.Scan(&key, &unit.EditionID, &unit.Revision, &unit.Withdrawn,
			&unit.SourceLayer, &unit.SourceFileName, &unit.SourceFileSize, &unit.SourceFileSHA256, &unit.SourceFormat, &unit.InterpretationVersion,
			&layer, &fileName, &size, &sha256, &citationStart, &citationEnd); err != nil {
			return nil, err
		}
		unit.Translation = classicalTranslationPin(layer, fileName, size, sha256, citationStart, citationEnd)
		stored[key] = unit
	}
	return stored, rows.Err()
}

// validateStoredRevisions refuses a broken build before it writes anything. A
// reduced revision or a content-bearing change without a bump can only be a
// manifest that disagrees with what this installation already reconciled.
func validateStoredRevisions(definitions []classical.Definition, stored map[string]storedClassicalUnit) error {
	for _, definition := range definitions {
		previous, ok := stored[definition.Key]
		if !ok {
			continue
		}
		if previous.Revision > definition.Revision {
			return fmt.Errorf("classical: definition %q would reduce revision from %d to %d", definition.Key, previous.Revision, definition.Revision)
		}
		if previous.Revision == definition.Revision && storedContentChanged(previous, definition) {
			return fmt.Errorf("classical: definition %q changed content without a revision bump", definition.Key)
		}
	}
	return nil
}

// storedContentChanged reports whether the manifest changes a source pin,
// translation pin or interpretation version compared with the stored row. An
// unrecorded translation pin on a legacy row is not a change.
func storedContentChanged(previous storedClassicalUnit, next classical.Definition) bool {
	if previous.SourceLayer != "" {
		if previous.SourceLayer != next.Source.Layer || previous.SourceFormat != next.Source.Format {
			return true
		}
		return previous.SourceFileName != next.Source.FileName ||
			previous.SourceFileSize != next.Source.Size ||
			previous.SourceFileSHA256 != next.Source.SHA256 ||
			previous.InterpretationVersion != next.InterpretationVersion ||
			!sameClassicalTranslation(previous.Translation, next.Translation)
	}
	// A row that predates reconciliation never recorded its layer, format or
	// translation pin, so only the pins it did store can be compared.
	return previous.SourceFileName != next.Source.FileName ||
		previous.SourceFileSize != next.Source.Size ||
		previous.SourceFileSHA256 != next.Source.SHA256 ||
		previous.InterpretationVersion != next.InterpretationVersion
}

// sameClassicalTranslation compares two optional translation pins, including
// their aligned citation range.
func sameClassicalTranslation(a, b *classical.TranslationPin) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// reconcileClassicalWork finds or inserts the Work row and returns its ID. An
// old one-key-per-row Work whose reading unit carries this definition is
// corrected to the shared Work key in place, so the Reading unit's link does
// not change.
func reconcileClassicalWork(ctx context.Context, tx pgx.Tx, definition classical.Definition) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM works WHERE definition_key = $1`, definition.WorkKey).Scan(&id)
	switch {
	case err == nil:
		_, err = tx.Exec(ctx, `UPDATE works SET title = $2, author = $3, greek_title = $4 WHERE id = $1`,
			id, definition.WorkTitle, definition.WorkAuthor, definition.GreekTitle)
		return id, err
	case !errors.Is(err, pgx.ErrNoRows):
		return "", err
	}
	err = tx.QueryRow(ctx, `
		SELECT w.id::text
		FROM reading_units ru
		JOIN editions e ON e.id = ru.edition_id
		JOIN works w ON w.id = e.work_id
		WHERE ru.definition_key = $1`, definition.Key).Scan(&id)
	switch {
	case err == nil:
		_, err = tx.Exec(ctx, `UPDATE works SET definition_key = $2, title = $3, author = $4, greek_title = $5 WHERE id = $1`,
			id, definition.WorkKey, definition.WorkTitle, definition.WorkAuthor, definition.GreekTitle)
		return id, err
	case !errors.Is(err, pgx.ErrNoRows):
		return "", err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO works(definition_key, title, author, greek_title)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text`,
		definition.WorkKey, definition.WorkTitle, definition.WorkAuthor, definition.GreekTitle).Scan(&id)
	return id, err
}

// reconcileClassicalEdition finds or inserts the Edition row under its shared
// key. When the reading unit still points at an old one-key Edition, that row is
// renamed and re-pointed at the shared Work instead of creating a duplicate.
func reconcileClassicalEdition(ctx context.Context, tx pgx.Tx, definition classical.Definition, workID string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id::text FROM editions WHERE definition_key = $1`, definition.EditionKey).Scan(&id)
	switch {
	case err == nil:
		_, err = tx.Exec(ctx, `UPDATE editions SET work_id = $2, label = $3 WHERE id = $1`, id, workID, definition.EditionLabel)
		return id, err
	case !errors.Is(err, pgx.ErrNoRows):
		return "", err
	}
	err = tx.QueryRow(ctx, `SELECT edition_id::text FROM reading_units WHERE definition_key = $1`, definition.Key).Scan(&id)
	switch {
	case err == nil:
		_, err = tx.Exec(ctx, `UPDATE editions SET definition_key = $2, work_id = $3, label = $4 WHERE id = $1`,
			id, definition.EditionKey, workID, definition.EditionLabel)
		return id, err
	case !errors.Is(err, pgx.ErrNoRows):
		return "", err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO editions(work_id, definition_key, label)
		VALUES ($1, $2, $3)
		RETURNING id::text`,
		workID, definition.EditionKey, definition.EditionLabel).Scan(&id)
	return id, err
}

// upsertClassicalReadingUnit writes the definition's Reading unit under its
// permanent key, including the revision the next startup compares against.
func upsertClassicalReadingUnit(ctx context.Context, tx pgx.Tx, definition classical.Definition, editionID string) error {
	var constituentBook pgtype.Int4
	if definition.ConstituentBook != nil {
		//nolint:gosec // a manifest constituent-book number is a small validated positive value; the column is int4.
		constituentBook = pgtype.Int4{Int32: int32(*definition.ConstituentBook), Valid: true}
	}
	var translationLayer, translationFileName, translationSHA256, translationStart, translationEnd pgtype.Text
	var translationSize pgtype.Int8
	if definition.Translation != nil {
		translationLayer = textArg(definition.Translation.Layer)
		translationFileName = textArg(definition.Translation.FileName)
		translationSize = pgtype.Int8{Int64: definition.Translation.Size, Valid: true}
		translationSHA256 = textArg(definition.Translation.SHA256)
		translationStart = textArg(definition.Translation.CitationStart)
		translationEnd = textArg(definition.Translation.CitationEnd)
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO reading_units(
			edition_id, definition_key, citation_start, citation_end,
			source_layer, source_file_name, source_file_size, source_file_sha256, source_format, interpretation_version,
			constituent_book, constituent_books, word_count, revision, withdrawn,
			translation_layer, translation_file_name, translation_file_size,
			translation_file_sha256, translation_citation_start, translation_citation_end)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21)
		ON CONFLICT (definition_key) DO UPDATE SET
			edition_id = EXCLUDED.edition_id,
			citation_start = EXCLUDED.citation_start,
			citation_end = EXCLUDED.citation_end,
			source_layer = EXCLUDED.source_layer,
			source_file_name = EXCLUDED.source_file_name,
			source_file_size = EXCLUDED.source_file_size,
			source_file_sha256 = EXCLUDED.source_file_sha256,
			source_format = EXCLUDED.source_format,
			interpretation_version = EXCLUDED.interpretation_version,
			constituent_book = EXCLUDED.constituent_book,
			constituent_books = EXCLUDED.constituent_books,
			word_count = EXCLUDED.word_count,
			revision = EXCLUDED.revision,
			withdrawn = EXCLUDED.withdrawn,
			translation_layer = EXCLUDED.translation_layer,
			translation_file_name = EXCLUDED.translation_file_name,
			translation_file_size = EXCLUDED.translation_file_size,
			translation_file_sha256 = EXCLUDED.translation_file_sha256,
			translation_citation_start = EXCLUDED.translation_citation_start,
			translation_citation_end = EXCLUDED.translation_citation_end`,
		editionID, definition.Key, definition.CitationStart, definition.CitationEnd,
		definition.Source.Layer, definition.Source.FileName, definition.Source.Size, definition.Source.SHA256, definition.Source.Format, definition.InterpretationVersion,
		constituentBook, definition.ConstituentBooks, definition.WordCount, definition.Revision, definition.Withdrawn,
		translationLayer, translationFileName, translationSize, translationSHA256, translationStart, translationEnd)
	return err
}

// applyCurrentClassicalMetadata rewrites shared curatorial fields from the
// current definitions after all rows exist, so a withdrawn tombstone's older
// title never overwrites a Work that still has a current Reading unit.
func applyCurrentClassicalMetadata(ctx context.Context, tx pgx.Tx, definitions []classical.Definition) error {
	works := map[string]classical.Definition{}
	editions := map[string]classical.Definition{}
	for _, definition := range definitions {
		if definition.Withdrawn {
			continue
		}
		if _, ok := works[definition.WorkKey]; !ok {
			works[definition.WorkKey] = definition
		}
		if _, ok := editions[definition.EditionKey]; !ok {
			editions[definition.EditionKey] = definition
		}
	}
	for key, definition := range works {
		if _, err := tx.Exec(ctx, `UPDATE works SET title = $2, author = $3, greek_title = $4 WHERE definition_key = $1`,
			key, definition.WorkTitle, definition.WorkAuthor, definition.GreekTitle); err != nil {
			return err
		}
	}
	for key, definition := range editions {
		if _, err := tx.Exec(ctx, `UPDATE editions SET label = $2 WHERE definition_key = $1`, key, definition.EditionLabel); err != nil {
			return err
		}
	}
	return nil
}

// withdrawAbsentClassicalUnits tombstones every reconciled Reading unit the
// manifest no longer carries. The rows stay so existing Books keep their
// association; the catalog hides them.
func withdrawAbsentClassicalUnits(ctx context.Context, tx pgx.Tx, definitions []classical.Definition) error {
	keys := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		keys = append(keys, definition.Key)
	}
	_, err := tx.Exec(ctx, `UPDATE reading_units SET withdrawn = true WHERE NOT (definition_key = ANY($1::text[])) AND withdrawn = false`, keys)
	return err
}

// pruneMergedClassicalRows removes Edition and Work rows left empty after old
// one-key rows were corrected or merged into shared ones. Reaching rows stay;
// only rows no Reading unit references, and no manifest key names, go.
func pruneMergedClassicalRows(ctx context.Context, tx pgx.Tx, definitions []classical.Definition) error {
	editionKeys := make([]string, 0, len(definitions))
	workKeys := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		editionKeys = append(editionKeys, definition.EditionKey)
		workKeys = append(workKeys, definition.WorkKey)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM editions e
		WHERE NOT EXISTS (SELECT 1 FROM reading_units ru WHERE ru.edition_id = e.id)
		  AND NOT (e.definition_key = ANY($1::text[]))`, editionKeys); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		DELETE FROM works w
		WHERE NOT EXISTS (SELECT 1 FROM editions e WHERE e.work_id = w.id)
		  AND NOT (w.definition_key = ANY($1::text[]))`, workKeys)
	return err
}

// classicalTranslationPin reassembles a recorded translation pin, treating a
// partial or absent pin as unrecorded.
func classicalTranslationPin(layer, fileName pgtype.Text, size pgtype.Int8, sha256, citationStart, citationEnd pgtype.Text) *classical.TranslationPin {
	if !layer.Valid || !fileName.Valid || !size.Valid || !sha256.Valid || !citationStart.Valid || !citationEnd.Valid {
		return nil
	}
	return &classical.TranslationPin{
		Layer:         layer.String,
		FileName:      fileName.String,
		Size:          size.Int64,
		SHA256:        sha256.String,
		CitationStart: citationStart.String,
		CitationEnd:   citationEnd.String,
	}
}
