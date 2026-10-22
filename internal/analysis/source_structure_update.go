package analysis

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/sourcecheck"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// ErrSourceStructureUpdateChanged reports that the checked source structure
// offer, the expected source evidence, or the running extraction contract
// changed after the learner was shown it. Nothing is adopted or queued.
var ErrSourceStructureUpdateChanged = errors.New("source structure update offer changed")

const sourceStructureUpdateOffersSelect = `
SELECT structure_check.source_material_id::text, structure_check.content_revision_id::text, structure_check.snapshot_id::text,
       structure_check.target_contract_version, structure_check.structure_convention_set, structure_check.result_identity
FROM source_structure_checks structure_check
JOIN source_materials source
  ON source.owner_id = structure_check.owner_id AND source.id = structure_check.source_material_id
WHERE structure_check.owner_id = $1
  AND structure_check.source_material_id = ANY($2::uuid[])
  AND structure_check.outcome = 'update_available'
  AND source.current_content_revision_id = structure_check.content_revision_id
  AND source.current_snapshot_id = structure_check.snapshot_id
  AND structure_check.target_contract_version = $3
  AND structure_check.structure_convention_set = $4`

const sourceStructureDecisionCountsSelect = `
SELECT cai.source_material_id::text,
       (SELECT count(*) FROM occurrence_lemma_corrections o
         WHERE o.owner_id = cai.owner_id AND o.book_id = cai.book_id
           AND o.analysis_run_id = cai.analysis_run_id AND NOT o.excluded),
       (SELECT count(*) FROM occurrence_lemma_corrections o
         WHERE o.owner_id = cai.owner_id AND o.book_id = cai.book_id
           AND o.analysis_run_id = cai.analysis_run_id AND o.excluded)
FROM current_analysis_identity cai
WHERE cai.owner_id = $1 AND cai.source_material_id = ANY($2::uuid[])`

// SourceStructureUpdateCandidates returns, by source ID, the Books whose
// current evidence has a successful update-available structure check for the
// configured target. The counts disclose the confirmed decisions on the
// published analysis that will not carry over. Candidates are advisory: the
// submit path revalidates the check, target and expected source under lock.
func (s *Service) SourceStructureUpdateCandidates(ctx context.Context, owner string, sourceIDs []string) (map[string]domain.SourceStructureUpdateCandidate, error) {
	candidates := map[string]domain.SourceStructureUpdateCandidate{}
	ids := validSourceIDs(sourceIDs)
	target := sourcecheck.ConfiguredTarget()
	if s == nil || s.pool == nil || strings.TrimSpace(owner) == "" || len(ids) == 0 || target.Validate() != nil {
		return candidates, nil
	}
	rows, err := s.pool.Query(ctx, sourceStructureUpdateOffersSelect, owner, ids, target.ContractVersion, target.ConventionSet)
	if err != nil {
		return nil, fmt.Errorf("load source structure update offers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sourceID string
		var candidate domain.SourceStructureUpdateCandidate
		if err := rows.Scan(&sourceID, &candidate.ContentRevisionID, &candidate.SnapshotID, &candidate.TargetContractVersion, &candidate.ConventionSet, &candidate.ResultIdentity); err != nil {
			return nil, fmt.Errorf("read source structure update offer: %w", err)
		}
		candidates[sourceID] = candidate
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read source structure update offers: %w", err)
	}
	if len(candidates) == 0 {
		return candidates, nil
	}
	countIDs := make([]string, 0, len(candidates))
	for id := range candidates {
		countIDs = append(countIDs, id)
	}
	return s.attachStructureDecisionCounts(ctx, owner, candidates, countIDs)
}

// attachStructureDecisionCounts adds the confirmed decisions on each Book's
// published analysis to the offered updates.
func (s *Service) attachStructureDecisionCounts(ctx context.Context, owner string, candidates map[string]domain.SourceStructureUpdateCandidate, sourceIDs []string) (map[string]domain.SourceStructureUpdateCandidate, error) {
	counts, err := s.pool.Query(ctx, sourceStructureDecisionCountsSelect, owner, sourceIDs)
	if err != nil {
		return nil, fmt.Errorf("load source structure decision counts: %w", err)
	}
	defer counts.Close()
	for counts.Next() {
		var sourceID string
		var candidate domain.SourceStructureUpdateCandidate
		if err := counts.Scan(&sourceID, &candidate.LemmaCorrections, &candidate.OccurrenceExclusions); err != nil {
			return nil, fmt.Errorf("read source structure decision counts: %w", err)
		}
		if existing, ok := candidates[sourceID]; ok {
			existing.LemmaCorrections = candidate.LemmaCorrections
			existing.OccurrenceExclusions = candidate.OccurrenceExclusions
			candidates[sourceID] = existing
		}
	}
	if err := counts.Err(); err != nil {
		return nil, fmt.Errorf("read source structure decision counts: %w", err)
	}
	return candidates, nil
}

// validSourceIDs filters to IDs PostgreSQL can compare as uuid[].
func validSourceIDs(sourceIDs []string) []string {
	ids := make([]string, 0, len(sourceIDs))
	seen := make(map[string]struct{}, len(sourceIDs))
	for _, id := range sourceIDs {
		id = strings.TrimSpace(id)
		if _, err := uuid.Parse(id); err != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

// adoptionSource is the exact source evidence a confirmation reads before it
// re-extracts retained bytes. It is read outside the adopting transaction and
// revalidated under the source lock.
type adoptionSource struct {
	source       domain.SourceMaterial
	bookID       string
	revisionID   string
	snapshotID   string
	retainedData []byte
}

const adoptionSourceSelect = `
SELECT s.owner_id::text, s.id::text, s.language, s.source_identifier, s.title, s.media_type,
       COALESCE(s.book_id::text,''), COALESCE(s.current_content_revision_id::text,''),
       COALESCE(s.current_snapshot_id::text,''), COALESCE(r.content, s.content)
FROM source_materials s
LEFT JOIN source_content_revisions r
  ON r.owner_id = s.owner_id AND r.source_material_id = s.id
 AND r.revision_id = s.current_content_revision_id
WHERE s.owner_id = $1 AND s.id = $2`

func (s *Service) loadAdoptionSource(ctx context.Context, owner, sourceID string) (adoptionSource, error) {
	var value adoptionSource
	err := s.pool.QueryRow(ctx, adoptionSourceSelect, owner, sourceID).Scan(
		&value.source.OwnerID, &value.source.ID, &value.source.Language, &value.source.SourceIdentifier,
		&value.source.Title, &value.source.MediaType, &value.bookID, &value.revisionID, &value.snapshotID,
		&value.retainedData,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return adoptionSource{}, ErrNotFound
	}
	if err != nil {
		return adoptionSource{}, fmt.Errorf("load source for structure adoption: %w", err)
	}
	return value, nil
}

const sourceStructureCheckSelect = `
SELECT outcome, result_identity
FROM source_structure_checks
WHERE owner_id = $1 AND source_material_id = $2 AND content_revision_id = $3
  AND snapshot_id = $4 AND target_contract_version = $5
  AND structure_convention_set = $6`

// SubmitSourceStructureUpdate adopts the checked extraction of a Book's
// retained bytes and atomically queues its one snapshot-bound analysis. The
// expected revision, snapshot, target and checked result are revalidated under
// the owner and source locks, so a stale form or concurrent source change
// adopts nothing. An active Reading is allowed: it keeps its frozen evidence.
func (s *Service) SubmitSourceStructureUpdate(ctx context.Context, owner, bookID, sourceID string, expected domain.SourceStructureUpdateExpectation) (result Handle, err error) {
	if s == nil || s.pool == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(sourceID) == "" {
		return Handle{}, errors.New("analysis owner and source are required")
	}
	extracted, err := s.checkedStructureExtraction(ctx, owner, bookID, sourceID, expected)
	if err != nil {
		return Handle{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, fmt.Errorf("begin source structure adoption: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 191))`, owner); err != nil {
		return Handle{}, fmt.Errorf("lock source structure adoption: %w", err)
	}
	result, err = s.adoptAndQueueStructure(ctx, tx, owner, bookID, sourceID, expected, extracted)
	if err != nil {
		return Handle{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return result, nil
}

// checkedStructureExtraction re-reads the retained bytes, verifies the checked
// offer and returns the extraction the transaction may adopt. It performs no
// writes.
func (s *Service) checkedStructureExtraction(ctx context.Context, owner, bookID, sourceID string, expected domain.SourceStructureUpdateExpectation) (domain.ExtractedSnapshot, error) {
	source, err := s.loadCheckedStructureSource(ctx, owner, bookID, sourceID, expected)
	if err != nil {
		return domain.ExtractedSnapshot{}, err
	}
	var outcome domain.SourceStructureOutcome
	var resultIdentity string
	err = s.pool.QueryRow(ctx, sourceStructureCheckSelect, owner, sourceID, expected.ContentRevisionID, expected.SnapshotID, expected.TargetContractVersion, expected.ConventionSet).Scan(&outcome, &resultIdentity)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ExtractedSnapshot{}, ErrSourceStructureUpdateChanged
	}
	if err != nil {
		return domain.ExtractedSnapshot{}, fmt.Errorf("load source structure check: %w", err)
	}
	if outcome != domain.SourceStructureUpdateAvailable || resultIdentity != expected.ResultIdentity {
		return domain.ExtractedSnapshot{}, ErrSourceStructureUpdateChanged
	}
	extraction, err := epub.ExtractV2(source.retainedData)
	if err != nil {
		return domain.ExtractedSnapshot{}, fmt.Errorf("re-extract retained source for structure adoption: %w", err)
	}
	extracted := domain.ExtractedSnapshot{
		ContentRevisionID: expected.ContentRevisionID,
		Units:             extraction.Book.ExtractedUnits,
		Structure:         extraction.Structure,
	}
	if extracted.Units.SchemaVersion != expected.TargetContractVersion || extracted.Structure.ConventionSet != expected.ConventionSet {
		return domain.ExtractedSnapshot{}, ErrSourceStructureUpdateChanged
	}
	if sourcecheck.ResultIdentity(extracted) != expected.ResultIdentity {
		return domain.ExtractedSnapshot{}, ErrSourceStructureUpdateChanged
	}
	return extracted, nil
}

// loadCheckedStructureSource resolves the Book's current EPUB source and
// verifies that it still matches the confirmed revision and target. It never
// extracts or writes.
func (s *Service) loadCheckedStructureSource(ctx context.Context, owner, bookID, sourceID string, expected domain.SourceStructureUpdateExpectation) (adoptionSource, error) {
	target := sourcecheck.Target{ContractVersion: expected.TargetContractVersion, ConventionSet: expected.ConventionSet}
	if err := target.Validate(); err != nil || target != sourcecheck.ConfiguredTarget() {
		return adoptionSource{}, ErrSourceStructureUpdateChanged
	}
	source, err := s.loadAdoptionSource(ctx, owner, sourceID)
	if err != nil {
		return adoptionSource{}, err
	}
	if bookID != "" && source.bookID != bookID {
		return adoptionSource{}, ErrNotFound
	}
	if source.source.MediaType != epub.MediaType() {
		return adoptionSource{}, ErrEPUBRequired
	}
	if source.revisionID != expected.ContentRevisionID || len(source.retainedData) == 0 {
		return adoptionSource{}, ErrSourceStructureUpdateChanged
	}
	return source, nil
}

// adoptAndQueueStructure switches to the exact checked snapshot and queues its
// analysis in the caller's transaction.
func (s *Service) adoptAndQueueStructure(ctx context.Context, tx pgx.Tx, owner, bookID, sourceID string, expected domain.SourceStructureUpdateExpectation, extracted domain.ExtractedSnapshot) (Handle, error) {
	adoptedSnapshotID, err := persistence.AdoptExtractedSnapshot(ctx, tx, persistence.ExtractedSnapshotAdoption{
		OwnerID: owner, SourceMaterialID: sourceID,
		ContentRevisionID: expected.ContentRevisionID, ExpectedSnapshotID: expected.SnapshotID,
		Units: extracted.Units, Structure: extracted.Structure,
	})
	if err != nil {
		if errors.Is(err, persistence.ErrSourceEvidenceChanged) {
			return Handle{}, ErrSourceStructureUpdateChanged
		}
		return Handle{}, err
	}
	args, err := loadSubmissionInputs(ctx, tx, owner, bookID, sourceID)
	if err != nil {
		return Handle{}, err
	}
	if args.SnapshotID != adoptedSnapshotID {
		return Handle{}, fmt.Errorf("adopted snapshot %s does not match analysis submission %s", adoptedSnapshotID, args.SnapshotID)
	}
	result, handled, err := s.submitExistingRun(ctx, tx, owner, args, nil)
	if err != nil {
		return Handle{}, err
	}
	if !handled {
		result, err = s.createAnalysisRun(ctx, tx, owner, args)
		if err != nil {
			return Handle{}, err
		}
	}
	return result, nil
}
