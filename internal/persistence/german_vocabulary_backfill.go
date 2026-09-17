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
	if conflict, err := germanCuratedConflict(ctx, tx, owner, profile); err != nil {
		return report, nil, err
	} else if conflict != nil {
		conflict.OwnerID = owner
		return report, conflict, nil
	}

	if n, m, err := backfillKnownVocabulary(ctx, tx, owner, profile); err != nil {
		return report, nil, err
	} else {
		report.updated += n
		report.merged += m
	}
	if n, m, err := backfillVocabularyStates(ctx, tx, owner, profile); err != nil {
		return report, nil, err
	} else {
		report.updated += n
		report.merged += m
	}
	if n, m, err := backfillGeneratedVocabulary(ctx, tx, owner, profile); err != nil {
		return report, nil, err
	} else {
		report.updated += n
		report.merged += m
	}
	if n, m, err := backfillSelectionCandidates(ctx, tx, owner, profile); err != nil {
		return report, nil, err
	} else {
		report.updated += n
		report.merged += m
	}
	if n, m, err := backfillExampleSentences(ctx, tx, owner, profile); err != nil {
		return report, nil, err
	} else {
		report.updated += n
		report.merged += m
	}
	if n, err := backfillCuratedSentences(ctx, tx, owner, profile); err != nil {
		return report, nil, err
	} else {
		report.updated += n
	}
	if err = tx.Commit(ctx); err != nil {
		return report, nil, err
	}
	report.examined = report.updated + report.merged
	return report, nil, nil
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
			var group []knownVocabularyRow
			for _, row := range rows {
				if row.upos == upos && (row.lemma == target || profile.Canonical(row.lemma) == target) {
					group = append(group, row)
				}
			}
			if len(group) == 0 {
				continue
			}
			sort.Slice(group, func(i, j int) bool {
				if !group[i].createdAt.Equal(group[j].createdAt) {
					return group[i].createdAt.Before(group[j].createdAt)
				}
				return group[i].id < group[j].id
			})
			winner := group[0]
			for _, row := range group[1:] {
				if _, err = tx.Exec(ctx, `DELETE FROM known_vocabulary WHERE owner_id=$1 AND id=$2`, owner, row.id); err != nil {
					return updated, merged, err
				}
				merged++
			}
			if winner.lemma != target {
				if _, err = tx.Exec(ctx, `UPDATE known_vocabulary SET canonical_lemma=$3 WHERE owner_id=$1 AND id=$2`, owner, winner.id, target); err != nil {
					return updated, merged, err
				}
				updated++
			}
		}
	}
	return updated, merged, nil
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
		if err = rows.Scan(&row.id, &row.lemma, &row.upos, &row.createdAt); err != nil {
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
	priority := map[string]int{"candidate": 1, "ignored": 2, "accepted": 3, "generated": 4, "known": 5}
	var updated, merged int
	for target := range affected {
		for _, upos := range distinctStateUPOS(rows, target, profile) {
			var group []vocabularyStateRow
			for _, row := range rows {
				if row.upos == upos && (row.lemma == target || profile.Canonical(row.lemma) == target) {
					group = append(group, row)
				}
			}
			sort.Slice(group, func(i, j int) bool {
				if priority[group[i].state] != priority[group[j].state] {
					return priority[group[i].state] > priority[group[j].state]
				}
				if !group[i].updatedAt.Equal(group[j].updatedAt) {
					return group[i].updatedAt.After(group[j].updatedAt)
				}
				return group[i].id < group[j].id
			})
			winner := group[0]
			for _, row := range group[1:] {
				if _, err = tx.Exec(ctx, `DELETE FROM vocabulary_states WHERE owner_id=$1 AND id=$2`, owner, row.id); err != nil {
					return updated, merged, err
				}
				merged++
			}
			if winner.lemma != target {
				if _, err = tx.Exec(ctx, `UPDATE vocabulary_states SET canonical_lemma=$3, state=$4, updated_at=$5 WHERE owner_id=$1 AND id=$2`, owner, winner.id, target, winner.state, winner.updatedAt); err != nil {
					return updated, merged, err
				}
				updated++
			}
		}
	}
	return updated, merged, nil
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
		if err = rows.Scan(&row.id, &row.lemma, &row.upos, &row.state, &row.updatedAt); err != nil {
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
			var group []generatedVocabularyRow
			for _, row := range rows {
				if row.upos == upos && (row.lemma == target || profile.Canonical(row.lemma) == target) {
					group = append(group, row)
				}
			}
			sort.Slice(group, func(i, j int) bool {
				if !group[i].firstGeneratedAt.Equal(group[j].firstGeneratedAt) {
					return group[i].firstGeneratedAt.Before(group[j].firstGeneratedAt)
				}
				if group[i].firstDeckID != group[j].firstDeckID {
					return group[i].firstDeckID < group[j].firstDeckID
				}
				return group[i].firstSourceID < group[j].firstSourceID
			})
			winner := group[0]
			temporary := "__mouseion_german_v6_" + uuid.NewString()
			if _, err = tx.Exec(ctx, `INSERT INTO generated_vocabulary(owner_id, language, canonical_lemma, upos, first_deck_id, first_source_material_id, first_generated_at) VALUES($1,'de',$2,$3,$4,NULLIF($5,'')::uuid,$6)`, owner, temporary, upos, winner.firstDeckID, winner.firstSourceID, winner.firstGeneratedAt); err != nil {
				return updated, merged, err
			}
			children, err := queryDeckVocabulary(ctx, tx, owner, group, upos)
			if err != nil {
				return updated, merged, err
			}
			mergedChildren := mergeDeckVocabulary(children)
			lemmas := make([]string, 0, len(group))
			for _, row := range group {
				lemmas = append(lemmas, row.lemma)
			}
			if _, err = tx.Exec(ctx, `DELETE FROM deck_preparation_vocabulary WHERE owner_id=$1 AND language='de' AND upos=$3 AND canonical_lemma=ANY($2::text[])`, owner, lemmas, upos); err != nil {
				return updated, merged, err
			}
			for _, child := range mergedChildren {
				if _, err = tx.Exec(ctx, `INSERT INTO deck_preparation_vocabulary(owner_id, deck_preparation_id, language, canonical_lemma, upos, generated_at, graduated_at) VALUES($1,$2,'de',$3,$4,$5,$6)`, owner, child.preparationID, temporary, upos, child.generatedAt, child.graduatedAt); err != nil {
					return updated, merged, err
				}
			}
			for _, row := range group {
				if _, err = tx.Exec(ctx, `DELETE FROM generated_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma=$2 AND upos=$3`, owner, row.lemma, upos); err != nil {
					return updated, merged, err
				}
			}
			if _, err = tx.Exec(ctx, `INSERT INTO generated_vocabulary(owner_id, language, canonical_lemma, upos, first_deck_id, first_source_material_id, first_generated_at) VALUES($1,'de',$2,$3,$4,NULLIF($5,'')::uuid,$6)`, owner, target, upos, winner.firstDeckID, winner.firstSourceID, winner.firstGeneratedAt); err != nil {
				return updated, merged, err
			}
			if _, err = tx.Exec(ctx, `UPDATE deck_preparation_vocabulary SET canonical_lemma=$3 WHERE owner_id=$1 AND language='de' AND canonical_lemma=$2 AND upos=$4`, owner, temporary, target, upos); err != nil {
				return updated, merged, err
			}
			if _, err = tx.Exec(ctx, `DELETE FROM generated_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma=$2 AND upos=$3`, owner, temporary, upos); err != nil {
				return updated, merged, err
			}
			updated++
			merged += len(group) - 1
			merged += len(children) - len(mergedChildren)
		}
	}
	return updated, merged, nil
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
		if err = rows.Scan(&row.lemma, &row.upos, &row.firstDeckID, &row.firstSourceID, &row.firstGeneratedAt); err != nil {
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
	lemmas := make([]string, 0, len(group))
	for _, row := range group {
		lemmas = append(lemmas, row.lemma)
	}
	rows, err := tx.Query(ctx, `SELECT deck_preparation_id::text, generated_at, graduated_at FROM deck_preparation_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma=ANY($2::text[]) AND upos=$3 ORDER BY deck_preparation_id, generated_at, graduated_at NULLS LAST`, owner, lemmas, upos)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []deckVocabularyRow
	for rows.Next() {
		var row deckVocabularyRow
		if err = rows.Scan(&row.preparationID, &row.generatedAt, &row.graduatedAt); err != nil {
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
		uposSet := map[string]bool{}
		for _, row := range rows {
			if row.lemma == target || profile.Canonical(row.lemma) == target {
				uposSet[row.upos] = true
			}
		}
		for upos := range uposSet {
			forCorpus := map[string][]selectionCandidateRow{}
			for _, row := range rows {
				if row.upos == upos && (row.lemma == target || profile.Canonical(row.lemma) == target) {
					forCorpus[row.corpusID] = append(forCorpus[row.corpusID], row)
				}
			}
			for corpusID, group := range forCorpus {
				mergedRow, err := mergeSelectionCandidates(group)
				if err != nil {
					return updated, merged, fmt.Errorf("merge selection candidate %s/%s: %w", target, upos, err)
				}
				if _, err = tx.Exec(ctx, `DELETE FROM selection_candidates WHERE owner_id=$1 AND corpus_id=$2 AND language='de' AND canonical_lemma=ANY($3::text[]) AND upos=$4`, owner, corpusID, candidateLemmas(group), upos); err != nil {
					return updated, merged, err
				}
				if _, err = tx.Exec(ctx, `INSERT INTO selection_candidates(owner_id, corpus_id, language, canonical_lemma, upos, occurrence_count, observed_forms, eligible_sentence_refs, provenance, selected_at) VALUES($1,$2,'de',$3,$4,$5,$6,$7,$8,$9)`, owner, corpusID, target, upos, mergedRow.occurrenceCount, mergedRow.observedForms, mergedRow.sentenceReferences, mergedRow.provenance, mergedRow.selectedAt); err != nil {
					return updated, merged, err
				}
				updated++
				merged += len(group) - 1
			}
		}
	}
	return updated, merged, nil
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
		if err = rows.Scan(&row.corpusID, &row.lemma, &row.upos, &row.occurrenceCount, &row.observedForms, &row.sentenceReferences, &row.provenance, &row.selectedAt); err != nil {
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

func mergeSelectionCandidates(rows []selectionCandidateRow) (selectionCandidateRow, error) {
	if len(rows) == 0 {
		return selectionCandidateRow{}, errors.New("empty candidate group")
	}
	result := rows[0]
	var err error
	result.observedForms, err = mergeJSONArray(rows[0].observedForms, nil)
	if err != nil {
		return selectionCandidateRow{}, err
	}
	result.sentenceReferences, err = mergeJSONArray(rows[0].sentenceReferences, nil)
	if err != nil {
		return selectionCandidateRow{}, err
	}
	result.occurrenceCount = 0
	minOccurrences := 0
	for _, row := range rows {
		result.occurrenceCount += row.occurrenceCount
		result.observedForms, err = mergeJSONArray(result.observedForms, row.observedForms)
		if err != nil {
			return selectionCandidateRow{}, err
		}
		result.sentenceReferences, err = mergeJSONArray(result.sentenceReferences, row.sentenceReferences)
		if err != nil {
			return selectionCandidateRow{}, err
		}
		if row.selectedAt.After(result.selectedAt) {
			result.selectedAt = row.selectedAt
		}
		var provenance map[string]any
		if err = json.Unmarshal(row.provenance, &provenance); err != nil {
			return selectionCandidateRow{}, err
		}
		if value, ok := provenance["min_occurrences"].(float64); ok {
			converted, conversionErr := checked.IntFromFloat64(value)
			if conversionErr != nil {
				return selectionCandidateRow{}, fmt.Errorf("invalid min_occurrences: %w", conversionErr)
			}
			if converted < 0 {
				return selectionCandidateRow{}, errors.New("invalid min_occurrences: value must not be negative")
			}
			if minOccurrences == 0 || converted < minOccurrences {
				minOccurrences = converted
			}
		}
	}
	provenance := map[string]any{"min_occurrences": minOccurrences, "occurrence_count": result.occurrenceCount}
	result.provenance, err = json.Marshal(provenance)
	return result, err
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

func germanCuratedConflict(ctx context.Context, tx pgx.Tx, owner string, profile canonicalization.Profile) (*GermanVocabularyBackfillConflict, error) {
	rows, err := tx.Query(ctx, `SELECT canonical_lemma, upos FROM curated_sentences WHERE owner_id=$1 AND language='de'`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []backfillIdentity
	for rows.Next() {
		var row backfillIdentity
		if err = rows.Scan(&row.lemma, &row.upos); err != nil {
			return nil, err
		}
		values = append(values, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	counts := map[backfillIdentity]int{}
	for _, row := range values {
		target := backfillIdentity{lemma: profile.Canonical(row.lemma), upos: row.upos}
		if target.lemma != row.lemma {
			counts[target]++
		}
	}
	for _, row := range values {
		target := profile.Canonical(row.lemma)
		if target == row.lemma {
			continue
		}
		if counts[backfillIdentity{lemma: target, upos: row.upos}] > 1 {
			return &GermanVocabularyBackfillConflict{Table: "curated_sentences", CanonicalLemma: target, UPOS: row.upos, Detail: "multiple historical curated facts converge on one identity"}, nil
		}
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM curated_sentences WHERE owner_id=$1 AND language='de' AND canonical_lemma=$2 AND upos=$3)`, owner, target, row.upos).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			return &GermanVocabularyBackfillConflict{Table: "curated_sentences", CanonicalLemma: target, UPOS: row.upos, Detail: "both historical and modern curated facts exist"}, nil
		}
	}
	// A nil conflict means the backfill is safe to apply.
	//nolint:nilnil // the pointer is the explicit conflict/absence result.
	return nil, nil
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
		if err = rows.Scan(&id, &lemma, &upos); err != nil {
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
	affected := germanAffected(rows, func(row selectedSentenceRow) string { return row.lemma }, profile)
	var updated, merged int
	for target := range affected {
		uposSet := map[string]bool{}
		for _, row := range rows {
			if row.lemma == target || profile.Canonical(row.lemma) == target {
				uposSet[row.upos] = true
			}
		}
		for upos := range uposSet {
			corpora := map[string][]selectedSentenceRow{}
			for _, row := range rows {
				if row.upos == upos && (row.lemma == target || profile.Canonical(row.lemma) == target) {
					corpora[row.corpusID] = append(corpora[row.corpusID], row)
				}
			}
			for _, group := range corpora {
				sort.Slice(group, func(i, j int) bool {
					if group[i].selectionRank != group[j].selectionRank {
						return group[i].selectionRank < group[j].selectionRank
					}
					if !group[i].createdAt.Equal(group[j].createdAt) {
						return group[i].createdAt.Before(group[j].createdAt)
					}
					return group[i].id < group[j].id
				})
				chosen := -1
				for i, row := range group {
					if row.isChosen {
						chosen = i
						break
					}
				}
				for _, row := range group {
					if _, err = tx.Exec(ctx, `UPDATE example_sentences SET language=NULL, canonical_lemma=NULL, upos=NULL, selection_rank=NULL, selection_score=NULL, selection_reasons=NULL, is_chosen=false WHERE owner_id=$1 AND id=$2`, owner, row.id); err != nil {
						return updated, merged, err
					}
				}
				for i, row := range group {
					isChosen := chosen == i
					if _, err = tx.Exec(ctx, `UPDATE example_sentences SET language='de', canonical_lemma=$3, upos=$4, selection_rank=$5, selection_score=$6, selection_reasons=$7, is_chosen=$8 WHERE owner_id=$1 AND id=$2`, owner, row.id, target, upos, i+1, row.selectionScore, row.selectionReasons, isChosen); err != nil {
						return updated, merged, err
					}
					if row.lemma != target {
						updated++
					}
				}
				merged += len(group) - 1
			}
		}
	}
	return updated, merged, nil
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
		if err = rows.Scan(&row.id, &row.corpusID, &row.lemma, &row.upos, &row.selectionRank, &row.selectionScore, &row.selectionReasons, &row.isChosen, &row.createdAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
