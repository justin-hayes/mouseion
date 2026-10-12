package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

const germanVocabularyBackfillLockSalt int64 = 849

// GermanVocabularyBackfillReport describes one operator run. Conflicted owners
// are rolled back independently so the operator can resolve them and retry.
type GermanVocabularyBackfillReport struct {
	Owners    int
	Examined  int
	Updated   int
	Merged    int
	Conflicts []GermanVocabularyBackfillConflict
}

type GermanVocabularyBackfillConflict struct {
	OwnerID        string
	Table          string
	CanonicalLemma string
	UPOS           string
	Detail         string
}

type backfillIdentity struct{ lemma, upos string }

func (s *PostgresStore) BackfillGermanVocabulary(ctx context.Context) (GermanVocabularyBackfillReport, error) {
	var report GermanVocabularyBackfillReport
	rows, err := s.pool.Query(ctx, `SELECT id::text FROM users ORDER BY id`)
	if err != nil {
		return report, err
	}
	var owners []string
	for rows.Next() {
		var owner string
		if err = rows.Scan(&owner); err != nil {
			rows.Close()
			return report, err
		}
		owners = append(owners, owner)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return report, err
	}
	rows.Close()

	profile := canonicalization.GermanPost1996()
	report.Owners = len(owners)
	for _, owner := range owners {
		ownerReport, conflict, err := s.backfillGermanOwner(ctx, owner, profile)
		if err != nil {
			return report, fmt.Errorf("backfill German vocabulary for owner %s: %w", owner, err)
		}
		report.Examined += ownerReport.examined
		report.Updated += ownerReport.updated
		report.Merged += ownerReport.merged
		if conflict != nil {
			report.Conflicts = append(report.Conflicts, *conflict)
		}
	}
	return report, nil
}

type germanOwnerBackfillReport struct{ examined, updated, merged int }

func (s *PostgresStore) backfillGermanOwner(ctx context.Context, owner string, profile canonicalization.Profile) (report germanOwnerBackfillReport, conflict *GermanVocabularyBackfillConflict, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return report, nil, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, $2))`, owner, germanVocabularyBackfillLockSalt); err != nil {
		return report, nil, err
	}
	report, conflict, err = backfillGermanOwnerTx(ctx, tx, owner, profile)
	if err != nil {
		return report, nil, err
	}
	if conflict != nil {
		return report, conflict, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return report, nil, err
	}
	report.examined = report.updated + report.merged
	return report, nil, nil
}

// backfillGermanOwnerTx refuses the owner when the backfill would collide with
// curated sentences, and otherwise folds every record kind onto its canonical
// lemma.
func backfillGermanOwnerTx(ctx context.Context, tx pgx.Tx, owner string, profile canonicalization.Profile) (report germanOwnerBackfillReport, conflict *GermanVocabularyBackfillConflict, err error) {
	curated, found, err := germanCuratedConflict(ctx, tx, owner, profile)
	if err != nil {
		return report, nil, err
	}
	if found {
		curated.OwnerID = owner
		return report, &curated, nil
	}
	report, err = backfillGermanOwnerRecords(ctx, tx, owner, profile)
	return report, nil, err
}

// backfillGermanOwnerRecords merges every record kind in turn, accumulating the
// updated and merged counts. Curated sentences run last and report one count
// because they are retargeted in place rather than merged.
func backfillGermanOwnerRecords(ctx context.Context, tx pgx.Tx, owner string, profile canonicalization.Profile) (report germanOwnerBackfillReport, err error) {
	merges := []func(context.Context, pgx.Tx, string, canonicalization.Profile) (int, int, error){
		backfillKnownVocabulary,
		backfillVocabularyStates,
		backfillGeneratedVocabulary,
		backfillSelectionCandidates,
		backfillExampleSentences,
	}
	for _, merge := range merges {
		updated, merged, err := merge(ctx, tx, owner, profile)
		if err != nil {
			return report, err
		}
		report.updated += updated
		report.merged += merged
	}
	updated, err := backfillCuratedSentences(ctx, tx, owner, profile)
	if err != nil {
		return report, err
	}
	report.updated += updated
	return report, nil
}

func germanAffected[T any](rows []T, current func(T) string, profile canonicalization.Profile) map[string]bool {
	affected := make(map[string]bool)
	for _, row := range rows {
		value := current(row)
		if normalized := profile.Canonical(value); normalized != value {
			affected[normalized] = true
		}
	}
	return affected
}

type knownVocabularyRow struct {
	id, lemma, upos string
	createdAt       time.Time
}

func backfillKnownVocabulary(ctx context.Context, tx pgx.Tx, owner string, profile canonicalization.Profile) (int, int, error) {
	rows, err := queryKnownVocabulary(ctx, tx, owner)
	if err != nil {
		return 0, 0, err
	}
	affected := germanAffected(rows, func(row knownVocabularyRow) string { return row.lemma }, profile)
	var updated, merged int
	for target := range affected {
		for _, upos := range distinctUPOS(rows, target, profile) {
			group := knownVocabularyGroup(rows, target, upos, profile)
			if len(group) == 0 {
				continue
			}
			groupUpdated, groupMerged, err := mergeKnownVocabularyGroup(ctx, tx, owner, target, group)
			if err != nil {
				return updated, merged, err
			}
			updated += groupUpdated
			merged += groupMerged
		}
	}
	return updated, merged, nil
}

// knownVocabularyGroup selects the historical facts that canonicalize to target
// at upos. distinctUPOS only yields upos with a match, so the result is never
// empty.
func knownVocabularyGroup(rows []knownVocabularyRow, target, upos string, profile canonicalization.Profile) []knownVocabularyRow {
	var group []knownVocabularyRow
	for _, row := range rows {
		if row.upos == upos && (row.lemma == target || profile.Canonical(row.lemma) == target) {
			group = append(group, row)
		}
	}
	return group
}

// mergeKnownVocabularyGroup folds one identity group of known vocabulary facts
// onto its canonical lemma. The earliest created row wins, ties broken by id;
// the other rows are merged away and the winner is retargeted when its lemma is
// not already canonical.
func mergeKnownVocabularyGroup(ctx context.Context, tx pgx.Tx, owner, target string, group []knownVocabularyRow) (int, int, error) {
	winner := earliestKnownVocabulary(group)
	var merged int
	for _, row := range group[1:] {
		if _, err := tx.Exec(ctx, `DELETE FROM known_vocabulary WHERE owner_id=$1 AND id=$2`, owner, row.id); err != nil {
			return 0, merged, err
		}
		merged++
	}
	if winner.lemma != target {
		if _, err := tx.Exec(ctx, `UPDATE known_vocabulary SET canonical_lemma=$3 WHERE owner_id=$1 AND id=$2`, owner, winner.id, target); err != nil {
			return 0, merged, err
		}
		return 1, merged, nil
	}
	return 0, merged, nil
}

// earliestKnownVocabulary sorts group by creation time, ties broken by id, and
// returns its winner.
func earliestKnownVocabulary(group []knownVocabularyRow) knownVocabularyRow {
	sort.Slice(group, func(i, j int) bool {
		if !group[i].createdAt.Equal(group[j].createdAt) {
			return group[i].createdAt.Before(group[j].createdAt)
		}
		return group[i].id < group[j].id
	})
	return group[0]
}

func queryKnownVocabulary(ctx context.Context, tx pgx.Tx, owner string) ([]knownVocabularyRow, error) {
	rows, err := tx.Query(ctx, `SELECT id::text, canonical_lemma, upos, created_at FROM known_vocabulary WHERE owner_id=$1 AND language='de'`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []knownVocabularyRow
	for rows.Next() {
		var row knownVocabularyRow
		if err := rows.Scan(&row.id, &row.lemma, &row.upos, &row.createdAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func distinctUPOS(rows []knownVocabularyRow, target string, profile canonicalization.Profile) []string {
	seen := map[string]bool{}
	for _, row := range rows {
		if row.lemma == target || profile.Canonical(row.lemma) == target {
			seen[row.upos] = true
		}
	}
	result := make([]string, 0, len(seen))
	for upos := range seen {
		result = append(result, upos)
	}
	sort.Strings(result)
	return result
}

type vocabularyStateRow struct {
	id, lemma, upos, state string
	updatedAt              time.Time
}

func backfillVocabularyStates(ctx context.Context, tx pgx.Tx, owner string, profile canonicalization.Profile) (int, int, error) {
	rows, err := queryVocabularyStates(ctx, tx, owner)
	if err != nil {
		return 0, 0, err
	}
	affected := germanAffected(rows, func(row vocabularyStateRow) string { return row.lemma }, profile)
	var updated, merged int
	for target := range affected {
		for _, upos := range distinctStateUPOS(rows, target, profile) {
			group := vocabularyStateGroup(rows, target, upos, profile)
			groupUpdated, groupMerged, err := mergeVocabularyStateGroup(ctx, tx, owner, target, group)
			if err != nil {
				return updated, merged, err
			}
			updated += groupUpdated
			merged += groupMerged
		}
	}
	return updated, merged, nil
}

// vocabularyStateGroup selects the historical states that canonicalize to
// target at upos. distinctStateUPOS only yields upos with a match, so the
// result is never empty.
func vocabularyStateGroup(rows []vocabularyStateRow, target, upos string, profile canonicalization.Profile) []vocabularyStateRow {
	var group []vocabularyStateRow
	for _, row := range rows {
		if row.upos == upos && (row.lemma == target || profile.Canonical(row.lemma) == target) {
			group = append(group, row)
		}
	}
	return group
}

// mergeVocabularyStateGroup folds one identity group of vocabulary states onto
// its canonical lemma. The highest-precedence state wins (known over candidate),
// ties broken by most recent update and then id; the other rows are merged away
// and the winner is retargeted when its lemma is not already canonical.
func mergeVocabularyStateGroup(ctx context.Context, tx pgx.Tx, owner, target string, group []vocabularyStateRow) (int, int, error) {
	winner := preferredVocabularyState(group)
	var merged int
	for _, row := range group[1:] {
		if _, err := tx.Exec(ctx, `DELETE FROM vocabulary_states WHERE owner_id=$1 AND id=$2`, owner, row.id); err != nil {
			return 0, merged, err
		}
		merged++
	}
	if winner.lemma != target {
		if _, err := tx.Exec(ctx, `UPDATE vocabulary_states SET canonical_lemma=$3, state=$4, updated_at=$5 WHERE owner_id=$1 AND id=$2`, owner, winner.id, target, winner.state, winner.updatedAt); err != nil {
			return 0, merged, err
		}
		return 1, merged, nil
	}
	return 0, merged, nil
}

// preferredVocabularyState sorts group by state precedence, then most recent
// update, ties broken by id, and returns its winner.
func preferredVocabularyState(group []vocabularyStateRow) vocabularyStateRow {
	priority := map[string]int{"candidate": 1, "ignored": 2, "accepted": 3, "generated": 4, "known": 5}
	sort.Slice(group, func(i, j int) bool {
		if priority[group[i].state] != priority[group[j].state] {
			return priority[group[i].state] > priority[group[j].state]
		}
		if !group[i].updatedAt.Equal(group[j].updatedAt) {
			return group[i].updatedAt.After(group[j].updatedAt)
		}
		return group[i].id < group[j].id
	})
	return group[0]
}

func queryVocabularyStates(ctx context.Context, tx pgx.Tx, owner string) ([]vocabularyStateRow, error) {
	rows, err := tx.Query(ctx, `SELECT id::text, canonical_lemma, upos, state, updated_at FROM vocabulary_states WHERE owner_id=$1 AND language='de'`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []vocabularyStateRow
	for rows.Next() {
		var row vocabularyStateRow
		if err := rows.Scan(&row.id, &row.lemma, &row.upos, &row.state, &row.updatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func distinctStateUPOS(rows []vocabularyStateRow, target string, profile canonicalization.Profile) []string {
	seen := map[string]bool{}
	for _, row := range rows {
		if row.lemma == target || profile.Canonical(row.lemma) == target {
			seen[row.upos] = true
		}
	}
	result := make([]string, 0, len(seen))
	for upos := range seen {
		result = append(result, upos)
	}
	sort.Strings(result)
	return result
}

type generatedVocabularyRow struct {
	lemma, upos, firstDeckID, firstSourceID string
	firstGeneratedAt                        time.Time
}

type deckVocabularyRow struct {
	preparationID string
	generatedAt   time.Time
	graduatedAt   *time.Time
}

func backfillGeneratedVocabulary(ctx context.Context, tx pgx.Tx, owner string, profile canonicalization.Profile) (int, int, error) {
	rows, err := queryGeneratedVocabulary(ctx, tx, owner)
	if err != nil {
		return 0, 0, err
	}
	affected := germanAffected(rows, func(row generatedVocabularyRow) string { return row.lemma }, profile)
	var updated, merged int
	for target := range affected {
		for _, upos := range distinctGeneratedUPOS(rows, target, profile) {
			group := generatedVocabularyGroup(rows, target, upos, profile)
			groupUpdated, groupMerged, err := mergeGeneratedVocabularyGroup(ctx, tx, owner, target, upos, group)
			if err != nil {
				return updated, merged, err
			}
			updated += groupUpdated
			merged += groupMerged
		}
	}
	return updated, merged, nil
}

// generatedVocabularyGroup selects the historical facts that canonicalize to
// target at upos. distinctGeneratedUPOS only yields upos with a match, so the
// result is never empty.
func generatedVocabularyGroup(rows []generatedVocabularyRow, target, upos string, profile canonicalization.Profile) []generatedVocabularyRow {
	var group []generatedVocabularyRow
	for _, row := range rows {
		if row.upos == upos && (row.lemma == target || profile.Canonical(row.lemma) == target) {
			group = append(group, row)
		}
	}
	return group
}

// mergeGeneratedVocabularyGroup folds one identity group of generated
// vocabulary facts onto its canonical lemma. The generated_vocabulary identity
// and its deck_preparation_vocabulary children are separate record kinds with
// separate conflicts: several historical spellings collapse to one identity,
// and several children per deck preparation collapse to one row. Because the
// children reference the identity by foreign key, the merge stages the
// canonical identity under a temporary lemma while the children are
// reconciled, then installs the canonical lemma.
func mergeGeneratedVocabularyGroup(ctx context.Context, tx pgx.Tx, owner, target, upos string, group []generatedVocabularyRow) (int, int, error) {
	winner := earliestGeneratedVocabulary(group)
	temporary := "__mouseion_german_v6_" + uuid.NewString()
	if err := insertGeneratedVocabulary(ctx, tx, owner, temporary, upos, winner); err != nil {
		return 0, 0, err
	}
	eliminated, err := mergeDeckPreparationVocabulary(ctx, tx, owner, group, upos, temporary)
	if err != nil {
		return 0, 0, err
	}
	if err := installGeneratedVocabulary(ctx, tx, owner, group, target, upos, winner, temporary); err != nil {
		return 0, 0, err
	}
	return 1, len(group) - 1 + eliminated, nil
}

// earliestGeneratedVocabulary sorts group by first appearance in place and
// returns its winner.
func earliestGeneratedVocabulary(group []generatedVocabularyRow) generatedVocabularyRow {
	sort.Slice(group, func(i, j int) bool {
		if !group[i].firstGeneratedAt.Equal(group[j].firstGeneratedAt) {
			return group[i].firstGeneratedAt.Before(group[j].firstGeneratedAt)
		}
		if group[i].firstDeckID != group[j].firstDeckID {
			return group[i].firstDeckID < group[j].firstDeckID
		}
		return group[i].firstSourceID < group[j].firstSourceID
	})
	return group[0]
}

// mergeDeckPreparationVocabulary reconciles the deck_preparation_vocabulary
// children of one identity group. They conflict when several historical
// spellings contribute a row for the same deck preparation, so it collapses
// each preparation onto one child and stages them under the temporary
// identity, reporting how many children the merge eliminated.
func mergeDeckPreparationVocabulary(ctx context.Context, tx pgx.Tx, owner string, group []generatedVocabularyRow, upos, temporary string) (int, error) {
	children, err := queryDeckVocabulary(ctx, tx, owner, group, upos)
	if err != nil {
		return 0, err
	}
	mergedChildren := mergeDeckVocabulary(children)
	if _, err := tx.Exec(ctx, `DELETE FROM deck_preparation_vocabulary WHERE owner_id=$1 AND language='de' AND upos=$3 AND canonical_lemma=ANY($2::text[])`, owner, generatedVocabularyLemmas(group), upos); err != nil {
		return 0, err
	}
	for _, child := range mergedChildren {
		if _, err := tx.Exec(ctx, `INSERT INTO deck_preparation_vocabulary(owner_id, deck_preparation_id, language, canonical_lemma, upos, generated_at, graduated_at) VALUES($1,$2,'de',$3,$4,$5,$6)`, owner, child.preparationID, temporary, upos, child.generatedAt, child.graduatedAt); err != nil {
			return 0, err
		}
	}
	return len(children) - len(mergedChildren), nil
}

// installGeneratedVocabulary retires the historical generated_vocabulary
// identities that conflict with target, installs the canonical identity, and
// repoints the staged children at it.
func installGeneratedVocabulary(ctx context.Context, tx pgx.Tx, owner string, group []generatedVocabularyRow, target, upos string, winner generatedVocabularyRow, temporary string) error {
	for _, row := range group {
		if _, err := tx.Exec(ctx, `DELETE FROM generated_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma=$2 AND upos=$3`, owner, row.lemma, upos); err != nil {
			return err
		}
	}
	if err := insertGeneratedVocabulary(ctx, tx, owner, target, upos, winner); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE deck_preparation_vocabulary SET canonical_lemma=$3 WHERE owner_id=$1 AND language='de' AND canonical_lemma=$2 AND upos=$4`, owner, temporary, target, upos); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM generated_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma=$2 AND upos=$3`, owner, temporary, upos); err != nil {
		return err
	}
	return nil
}

func insertGeneratedVocabulary(ctx context.Context, tx pgx.Tx, owner, lemma, upos string, winner generatedVocabularyRow) error {
	_, err := tx.Exec(ctx, `INSERT INTO generated_vocabulary(owner_id, language, canonical_lemma, upos, first_deck_id, first_source_material_id, first_generated_at) VALUES($1,'de',$2,$3,$4,NULLIF($5,'')::uuid,$6)`, owner, lemma, upos, winner.firstDeckID, winner.firstSourceID, winner.firstGeneratedAt)
	return err
}

func generatedVocabularyLemmas(group []generatedVocabularyRow) []string {
	lemmas := make([]string, 0, len(group))
	for _, row := range group {
		lemmas = append(lemmas, row.lemma)
	}
	return lemmas
}

func queryGeneratedVocabulary(ctx context.Context, tx pgx.Tx, owner string) ([]generatedVocabularyRow, error) {
	rows, err := tx.Query(ctx, `SELECT canonical_lemma, upos, first_deck_id::text, COALESCE(first_source_material_id::text,''), first_generated_at FROM generated_vocabulary WHERE owner_id=$1 AND language='de'`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []generatedVocabularyRow
	for rows.Next() {
		var row generatedVocabularyRow
		if err := rows.Scan(&row.lemma, &row.upos, &row.firstDeckID, &row.firstSourceID, &row.firstGeneratedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func distinctGeneratedUPOS(rows []generatedVocabularyRow, target string, profile canonicalization.Profile) []string {
	seen := map[string]bool{}
	for _, row := range rows {
		if row.lemma == target || profile.Canonical(row.lemma) == target {
			seen[row.upos] = true
		}
	}
	result := make([]string, 0, len(seen))
	for upos := range seen {
		result = append(result, upos)
	}
	sort.Strings(result)
	return result
}

func queryDeckVocabulary(ctx context.Context, tx pgx.Tx, owner string, group []generatedVocabularyRow, upos string) ([]deckVocabularyRow, error) {
	rows, err := tx.Query(ctx, `SELECT deck_preparation_id::text, generated_at, graduated_at FROM deck_preparation_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma=ANY($2::text[]) AND upos=$3 ORDER BY deck_preparation_id, generated_at, graduated_at NULLS LAST`, owner, generatedVocabularyLemmas(group), upos)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []deckVocabularyRow
	for rows.Next() {
		var row deckVocabularyRow
		if err := rows.Scan(&row.preparationID, &row.generatedAt, &row.graduatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func mergeDeckVocabulary(rows []deckVocabularyRow) []deckVocabularyRow {
	byPreparation := make(map[string]deckVocabularyRow)
	for _, row := range rows {
		current, ok := byPreparation[row.preparationID]
		if !ok {
			byPreparation[row.preparationID] = row
			continue
		}
		previousGraduatedAt := current.graduatedAt
		if row.generatedAt.Before(current.generatedAt) {
			current = row
		}
		graduatedAt := previousGraduatedAt
		if row.graduatedAt != nil && (graduatedAt == nil || row.graduatedAt.Before(*graduatedAt)) {
			graduatedAt = row.graduatedAt
		}
		if graduatedAt != nil {
			value := *graduatedAt
			current.graduatedAt = &value
		}
		byPreparation[row.preparationID] = current
	}
	result := make([]deckVocabularyRow, 0, len(byPreparation))
	for _, row := range byPreparation {
		result = append(result, row)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].preparationID < result[j].preparationID })
	return result
}

type selectionCandidateRow struct {
	corpusID, lemma, upos                         string
	occurrenceCount                               int
	observedForms, sentenceReferences, provenance []byte
	selectedAt                                    time.Time
}

func backfillSelectionCandidates(ctx context.Context, tx pgx.Tx, owner string, profile canonicalization.Profile) (int, int, error) {
	rows, err := querySelectionCandidates(ctx, tx, owner)
	if err != nil {
		return 0, 0, err
	}
	affected := germanAffected(rows, func(row selectionCandidateRow) string { return row.lemma }, profile)
	var updated, merged int
	for target := range affected {
		for upos := range selectionCandidateUPOS(rows, target, profile) {
			for corpusID, group := range selectionCandidateGroups(rows, target, upos, profile) {
				groupUpdated, groupMerged, err := mergeSelectionCandidateGroup(ctx, tx, owner, target, upos, corpusID, group)
				if err != nil {
					return updated, merged, err
				}
				updated += groupUpdated
				merged += groupMerged
			}
		}
	}
	return updated, merged, nil
}

// selectionCandidateUPOS collects the upos that carry a fact canonicalizing to
// target.
func selectionCandidateUPOS(rows []selectionCandidateRow, target string, profile canonicalization.Profile) map[string]bool {
	uposSet := map[string]bool{}
	for _, row := range rows {
		if row.lemma == target || profile.Canonical(row.lemma) == target {
			uposSet[row.upos] = true
		}
	}
	return uposSet
}

// selectionCandidateGroups collects the facts that canonicalize to target at
// upos, keyed by corpus: each corpus owns an independent conflict.
func selectionCandidateGroups(rows []selectionCandidateRow, target, upos string, profile canonicalization.Profile) map[string][]selectionCandidateRow {
	forCorpus := map[string][]selectionCandidateRow{}
	for _, row := range rows {
		if row.upos == upos && (row.lemma == target || profile.Canonical(row.lemma) == target) {
			forCorpus[row.corpusID] = append(forCorpus[row.corpusID], row)
		}
	}
	return forCorpus
}

// mergeSelectionCandidateGroup folds one corpus's candidate facts for one
// identity group onto its canonical lemma. It reports one updated identity and
// the number of rows the merge eliminated.
func mergeSelectionCandidateGroup(ctx context.Context, tx pgx.Tx, owner, target, upos, corpusID string, group []selectionCandidateRow) (int, int, error) {
	mergedRow, err := mergeSelectionCandidates(group)
	if err != nil {
		return 0, 0, fmt.Errorf("merge selection candidate %s/%s: %w", target, upos, err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM selection_candidates WHERE owner_id=$1 AND corpus_id=$2 AND language='de' AND canonical_lemma=ANY($3::text[]) AND upos=$4`, owner, corpusID, candidateLemmas(group), upos); err != nil {
		return 0, 0, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO selection_candidates(owner_id, corpus_id, language, canonical_lemma, upos, occurrence_count, observed_forms, eligible_sentence_refs, provenance, selected_at) VALUES($1,$2,'de',$3,$4,$5,$6,$7,$8,$9)`, owner, corpusID, target, upos, mergedRow.occurrenceCount, mergedRow.observedForms, mergedRow.sentenceReferences, mergedRow.provenance, mergedRow.selectedAt); err != nil {
		return 0, 0, err
	}
	return 1, len(group) - 1, nil
}

func querySelectionCandidates(ctx context.Context, tx pgx.Tx, owner string) ([]selectionCandidateRow, error) {
	rows, err := tx.Query(ctx, `SELECT corpus_id, canonical_lemma, upos, occurrence_count, observed_forms, eligible_sentence_refs, provenance, selected_at FROM selection_candidates WHERE owner_id=$1 AND language='de' ORDER BY corpus_id, canonical_lemma, upos`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []selectionCandidateRow
	for rows.Next() {
		var row selectionCandidateRow
		if err := rows.Scan(&row.corpusID, &row.lemma, &row.upos, &row.occurrenceCount, &row.observedForms, &row.sentenceReferences, &row.provenance, &row.selectedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func candidateLemmas(rows []selectionCandidateRow) []string {
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.lemma] = true
	}
	result := make([]string, 0, len(seen))
	for lemma := range seen {
		result = append(result, lemma)
	}
	sort.Strings(result)
	return result
}

// mergeSelectionCandidates folds one corpus's candidate facts onto a single row,
// unioning their observations and sentence references, summing their
// occurrences, and keeping the smallest min_occurrences any fact declares.
func mergeSelectionCandidates(rows []selectionCandidateRow) (selectionCandidateRow, error) {
	if len(rows) == 0 {
		return selectionCandidateRow{}, errors.New("empty candidate group")
	}
	result, minOccurrences, err := mergeSelectionCandidateRows(rows)
	if err != nil {
		return selectionCandidateRow{}, err
	}
	provenance := map[string]any{"min_occurrences": minOccurrences, "occurrence_count": result.occurrenceCount}
	result.provenance, err = json.Marshal(provenance)
	return result, err
}

// mergeSelectionCandidateRows starts from the first fact and folds every fact
// in, returning the merged row and the smallest declared min_occurrences.
func mergeSelectionCandidateRows(rows []selectionCandidateRow) (selectionCandidateRow, int, error) {
	result := rows[0]
	var err error
	if result.observedForms, err = mergeJSONArray(result.observedForms, nil); err != nil {
		return selectionCandidateRow{}, 0, err
	}
	if result.sentenceReferences, err = mergeJSONArray(result.sentenceReferences, nil); err != nil {
		return selectionCandidateRow{}, 0, err
	}
	result.occurrenceCount = 0
	minOccurrences := 0
	for _, row := range rows {
		if err := mergeSelectionCandidateRow(&result, row); err != nil {
			return selectionCandidateRow{}, 0, err
		}
		converted, declared, err := minimumSelectionCandidateOccurrences(row.provenance)
		if err != nil {
			return selectionCandidateRow{}, 0, err
		}
		if declared && (minOccurrences == 0 || converted < minOccurrences) {
			minOccurrences = converted
		}
	}
	return result, minOccurrences, nil
}

// mergeSelectionCandidateRow folds one fact's observations, sentence
// references, occurrence count and selection time into result.
func mergeSelectionCandidateRow(result *selectionCandidateRow, row selectionCandidateRow) error {
	result.occurrenceCount += row.occurrenceCount
	var err error
	if result.observedForms, err = mergeJSONArray(result.observedForms, row.observedForms); err != nil {
		return err
	}
	if result.sentenceReferences, err = mergeJSONArray(result.sentenceReferences, row.sentenceReferences); err != nil {
		return err
	}
	if row.selectedAt.After(result.selectedAt) {
		result.selectedAt = row.selectedAt
	}
	return nil
}

// minimumSelectionCandidateOccurrences reads min_occurrences from one fact's
// provenance, reporting whether the fact declares it.
func minimumSelectionCandidateOccurrences(provenance []byte) (int, bool, error) {
	var fields map[string]any
	if err := json.Unmarshal(provenance, &fields); err != nil {
		return 0, false, err
	}
	value, ok := fields["min_occurrences"].(float64)
	if !ok {
		return 0, false, nil
	}
	converted, err := checked.IntFromFloat64(value)
	if err != nil {
		return 0, false, fmt.Errorf("invalid min_occurrences: %w", err)
	}
	if converted < 0 {
		return 0, false, errors.New("invalid min_occurrences: value must not be negative")
	}
	return converted, true, nil
}

func mergeJSONArray(left, right []byte) ([]byte, error) {
	var values []json.RawMessage
	for _, input := range [][]byte{left, right} {
		if len(input) == 0 {
			continue
		}
		var part []json.RawMessage
		if err := json.Unmarshal(input, &part); err != nil {
			return nil, err
		}
		values = append(values, part...)
	}
	seen := map[string]bool{}
	result := make([]json.RawMessage, 0, len(values))
	for _, value := range values {
		compact, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		key := string(compact)
		if !seen[key] {
			seen[key] = true
			result = append(result, compact)
		}
	}
	return json.Marshal(result)
}

type selectedSentenceRow struct {
	id, corpusID, lemma, upos string
	selectionReasons          []byte
	selectionRank             int
	selectionScore            int
	isChosen                  bool
	createdAt                 time.Time
}

type selectedSentenceGroup struct {
	corpusID, canonicalLemma, upos string
	rows                           []selectedSentenceRow
}

type selectedSentenceGroupKey struct{ corpusID, canonicalLemma, upos string }

// germanCuratedConflict reports whether applying the backfill to the owner's
// curated sentences would collide. found is false when the backfill is safe to
// apply; conflict is only meaningful when found is true.
func germanCuratedConflict(ctx context.Context, tx pgx.Tx, owner string, profile canonicalization.Profile) (conflict GermanVocabularyBackfillConflict, found bool, err error) {
	values, err := loadCuratedSentenceIdentities(ctx, tx, owner)
	if err != nil {
		return GermanVocabularyBackfillConflict{}, false, err
	}
	historical := curatedHistoricalIdentityCounts(values, profile)
	for _, row := range values {
		target := profile.Canonical(row.lemma)
		if target != row.lemma && historical[backfillIdentity{lemma: target, upos: row.upos}] > 1 {
			return GermanVocabularyBackfillConflict{Table: "curated_sentences", CanonicalLemma: target, UPOS: row.upos, Detail: "multiple historical curated facts converge on one identity"}, true, nil
		}
		if target == row.lemma {
			continue
		}
		exists, err := curatedModernIdentityExists(ctx, tx, owner, target, row.upos)
		if err != nil {
			return GermanVocabularyBackfillConflict{}, false, err
		}
		if exists {
			return GermanVocabularyBackfillConflict{Table: "curated_sentences", CanonicalLemma: target, UPOS: row.upos, Detail: "both historical and modern curated facts exist"}, true, nil
		}
	}
	return GermanVocabularyBackfillConflict{}, false, nil
}

// loadCuratedSentenceIdentities reads the identities of the owner's curated
// sentences.
func loadCuratedSentenceIdentities(ctx context.Context, tx pgx.Tx, owner string) ([]backfillIdentity, error) {
	rows, err := tx.Query(ctx, `SELECT canonical_lemma, upos FROM curated_sentences WHERE owner_id=$1 AND language='de'`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []backfillIdentity
	for rows.Next() {
		var row backfillIdentity
		if err := rows.Scan(&row.lemma, &row.upos); err != nil {
			return nil, err
		}
		values = append(values, row)
	}
	return values, rows.Err()
}

// curatedHistoricalIdentityCounts counts how many non-canonical curated
// identities each target identity absorbs.
func curatedHistoricalIdentityCounts(values []backfillIdentity, profile canonicalization.Profile) map[backfillIdentity]int {
	counts := map[backfillIdentity]int{}
	for _, row := range values {
		target := backfillIdentity{lemma: profile.Canonical(row.lemma), upos: row.upos}
		if target.lemma != row.lemma {
			counts[target]++
		}
	}
	return counts
}

// curatedModernIdentityExists reports whether the canonical identity already
// exists for owner.
func curatedModernIdentityExists(ctx context.Context, tx pgx.Tx, owner, lemma, upos string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM curated_sentences WHERE owner_id=$1 AND language='de' AND canonical_lemma=$2 AND upos=$3)`, owner, lemma, upos).Scan(&exists)
	return exists, err
}

func backfillCuratedSentences(ctx context.Context, tx pgx.Tx, owner string, profile canonicalization.Profile) (int, error) {
	rows, err := tx.Query(ctx, `SELECT id::text, canonical_lemma, upos FROM curated_sentences WHERE owner_id=$1 AND language='de'`, owner)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var updated int
	for rows.Next() {
		var id, lemma, upos string
		if err := rows.Scan(&id, &lemma, &upos); err != nil {
			return updated, err
		}
		target := profile.Canonical(lemma)
		if target == lemma {
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE curated_sentences SET canonical_lemma=$3 WHERE owner_id=$1 AND id=$2`, owner, id, target); err != nil {
			return updated, err
		}
		updated++
	}
	return updated, rows.Err()
}

func backfillExampleSentences(ctx context.Context, tx pgx.Tx, owner string, profile canonicalization.Profile) (int, int, error) {
	rows, err := querySelectedSentences(ctx, tx, owner)
	if err != nil {
		return 0, 0, err
	}
	groups := canonicalizeSelectedSentenceGroups(rows, profile)
	var updated, merged int
	for _, group := range groups {
		reconciled := reconcileSelectedSentenceGroup(group)
		groupUpdated, groupMerged, err := persistSelectedSentenceGroup(ctx, tx, owner, reconciled)
		if err != nil {
			return updated, merged, err
		}
		updated += groupUpdated
		merged += groupMerged
	}
	return updated, merged, nil
}

func canonicalizeSelectedSentenceGroups(rows []selectedSentenceRow, profile canonicalization.Profile) []selectedSentenceGroup {
	affected := make(map[backfillIdentity]bool)
	for _, row := range rows {
		canonicalLemma := profile.Canonical(row.lemma)
		if canonicalLemma != row.lemma {
			affected[backfillIdentity{lemma: canonicalLemma, upos: row.upos}] = true
		}
	}

	groups := make(map[selectedSentenceGroupKey]*selectedSentenceGroup)
	for _, row := range rows {
		canonicalLemma := profile.Canonical(row.lemma)
		key := backfillIdentity{lemma: canonicalLemma, upos: row.upos}
		if !affected[key] {
			continue
		}
		groupKey := selectedSentenceGroupKey{corpusID: row.corpusID, canonicalLemma: canonicalLemma, upos: row.upos}
		group, ok := groups[groupKey]
		if !ok {
			group = &selectedSentenceGroup{corpusID: row.corpusID, canonicalLemma: canonicalLemma, upos: row.upos}
			groups[groupKey] = group
		}
		group.rows = append(group.rows, row)
	}

	result := make([]selectedSentenceGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, *group)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].canonicalLemma != result[j].canonicalLemma {
			return result[i].canonicalLemma < result[j].canonicalLemma
		}
		if result[i].upos != result[j].upos {
			return result[i].upos < result[j].upos
		}
		return result[i].corpusID < result[j].corpusID
	})
	return result
}

func reconcileSelectedSentenceGroup(group selectedSentenceGroup) selectedSentenceGroup {
	sort.Slice(group.rows, func(i, j int) bool {
		if group.rows[i].selectionRank != group.rows[j].selectionRank {
			return group.rows[i].selectionRank < group.rows[j].selectionRank
		}
		if !group.rows[i].createdAt.Equal(group.rows[j].createdAt) {
			return group.rows[i].createdAt.Before(group.rows[j].createdAt)
		}
		return group.rows[i].id < group.rows[j].id
	})

	chosen := -1
	for i, row := range group.rows {
		if row.isChosen {
			chosen = i
			break
		}
	}
	for i := range group.rows {
		group.rows[i].selectionRank = i + 1
		group.rows[i].isChosen = chosen == i
	}
	return group
}

func persistSelectedSentenceGroup(ctx context.Context, tx pgx.Tx, owner string, group selectedSentenceGroup) (int, int, error) {
	for _, row := range group.rows {
		if _, err := tx.Exec(ctx, `UPDATE example_sentences SET language=NULL, canonical_lemma=NULL, upos=NULL, selection_rank=NULL, selection_score=NULL, selection_reasons=NULL, is_chosen=false WHERE owner_id=$1 AND id=$2`, owner, row.id); err != nil {
			return 0, 0, err
		}
	}

	updated := 0
	for _, row := range group.rows {
		if _, err := tx.Exec(ctx, `UPDATE example_sentences SET language='de', canonical_lemma=$3, upos=$4, selection_rank=$5, selection_score=$6, selection_reasons=$7, is_chosen=$8 WHERE owner_id=$1 AND id=$2`, owner, row.id, group.canonicalLemma, group.upos, row.selectionRank, row.selectionScore, row.selectionReasons, row.isChosen); err != nil {
			return updated, 0, err
		}
		if row.lemma != group.canonicalLemma {
			updated++
		}
	}
	return updated, len(group.rows) - 1, nil
}

func querySelectedSentences(ctx context.Context, tx pgx.Tx, owner string) ([]selectedSentenceRow, error) {
	rows, err := tx.Query(ctx, `SELECT id::text, corpus_id::text, canonical_lemma, upos, selection_rank, selection_score, selection_reasons, is_chosen, created_at FROM example_sentences WHERE owner_id=$1 AND language='de'`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []selectedSentenceRow
	for rows.Next() {
		var row selectedSentenceRow
		if err := rows.Scan(&row.id, &row.corpusID, &row.lemma, &row.upos, &row.selectionRank, &row.selectionScore, &row.selectionReasons, &row.isChosen, &row.createdAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
