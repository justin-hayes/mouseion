package analysis

import (
	"context"
	"fmt"
	"strconv"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// Classical Analysis updates compare a Book's prepared-definition record with
// its reconciled Reading unit. The target identity is the current definition
// revision and source pin, not an NLP pipeline identity (ADR 0116).

const (
	classicalUpdateAnalyzerName      = "classical-preparation"
	classicalUpdateNormalizationName = "classical-source"
)

// classicalUpdateTarget builds the stable target a confirmation carries back.
func classicalUpdateTarget(interpretationVersion, sourceFileSHA256 string, revision int) domain.AnalysisUpdateTarget {
	return domain.AnalysisUpdateTarget{
		AnalyzerName:         classicalUpdateAnalyzerName,
		AnalyzerVersion:      interpretationVersion,
		NormalizationProfile: classicalUpdateNormalizationName,
		NormalizationVersion: sourceFileSHA256 + ":" + strconv.Itoa(revision),
	}
}

// classicalUpdateCandidates returns, by source ID, imported classical Books
// whose prepared definition differs from the current Reading unit. A missing
// record is an update; so is a changed source pin, interpretation version or
// definition revision.
func (s *Service) classicalUpdateCandidates(ctx context.Context, owner string, sourceIDs []string) (map[string]domain.AnalysisUpdateCandidate, error) {
	candidates := map[string]domain.AnalysisUpdateCandidate{}
	if s == nil || s.pool == nil || len(sourceIDs) == 0 {
		return candidates, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT cai.source_material_id::text, cai.analysis_run_id::text,
		       ru.interpretation_version, ru.source_file_sha256, ru.revision,
		       p.definition_revision, p.source_file_sha256, p.interpretation_version,
		       (SELECT count(*) FROM occurrence_lemma_corrections o
		         WHERE o.owner_id=cai.owner_id AND o.book_id=cai.book_id
		           AND o.analysis_run_id=cai.analysis_run_id AND NOT o.excluded),
		       (SELECT count(*) FROM occurrence_lemma_corrections o
		         WHERE o.owner_id=cai.owner_id AND o.book_id=cai.book_id
		           AND o.analysis_run_id=cai.analysis_run_id AND o.excluded)
		FROM current_analysis_identity cai
		JOIN source_materials s ON s.owner_id=cai.owner_id AND s.id=cai.source_material_id
		JOIN book_reading_units bru ON bru.owner_id=cai.owner_id AND bru.book_id=cai.book_id
		JOIN reading_units ru ON ru.id=bru.reading_unit_id
		LEFT JOIN classical_prepared_definitions p
		  ON p.owner_id=cai.owner_id AND p.book_id=cai.book_id
		WHERE cai.owner_id=$1 AND cai.source_material_id=ANY($2::uuid[])
		  AND s.media_type=$3 AND ru.withdrawn=false`, owner, sourceIDs, domain.ImportedSourceMediaType)
	if err != nil {
		return nil, fmt.Errorf("load classical analysis update identities: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sourceID, runID, interpretation, sha string
		var revision int
		var preparedRevision *int
		var preparedSHA, preparedInterpretation *string
		var corrections, exclusions int
		if err := rows.Scan(&sourceID, &runID, &interpretation, &sha, &revision,
			&preparedRevision, &preparedSHA, &preparedInterpretation, &corrections, &exclusions); err != nil {
			return nil, fmt.Errorf("read classical analysis update identity: %w", err)
		}
		if preparedRevision != nil && *preparedRevision == revision && preparedSHA != nil &&
			*preparedSHA == sha && preparedInterpretation != nil && *preparedInterpretation == interpretation {
			continue
		}
		candidates[sourceID] = domain.AnalysisUpdateCandidate{
			PublishedRunID:       runID,
			Target:               classicalUpdateTarget(interpretation, sha, revision),
			LemmaCorrections:     corrections,
			OccurrenceExclusions: exclusions,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read classical analysis update identities: %w", err)
	}
	return candidates, nil
}

// submitClassicalAnalysisUpdate revalidates one offered classical update and
// enqueues the ordinary ensure-once preparation job. It reports handled=false
// when the source is not an imported classical source.
func (s *Service) submitClassicalAnalysisUpdate(ctx context.Context, owner, bookID, sourceID, expectedRunID, expectedTarget string) (result Handle, handled bool, err error) {
	candidates, err := s.classicalUpdateCandidates(ctx, owner, []string{sourceID})
	if err != nil {
		return Handle{}, false, err
	}
	candidate, ok := candidates[sourceID]
	if !ok {
		// A current classical Book has no offer; an EPUB source is handled by
		// the existing pipeline path below.
		return Handle{}, false, nil
	}
	if candidate.PublishedRunID != expectedRunID || candidate.Target.Key() != expectedTarget {
		return Handle{}, true, ErrAnalysisUpdateChanged
	}
	var disposition string
	var current bool
	err = s.pool.QueryRow(ctx, `
		SELECT d.disposition, EXISTS(SELECT 1 FROM primary_goals g WHERE g.owner_id=b.owner_id AND g.book_id=b.id)
		FROM books b JOIN book_dispositions d ON d.owner_id=b.owner_id AND d.book_id=b.id
		WHERE b.owner_id=$1 AND b.id=$2`, owner, bookID).Scan(&disposition, &current)
	if err != nil {
		return Handle{}, true, ErrAnalysisUpdateChanged
	}
	if current {
		return Handle{}, true, ErrCurrentBook
	}
	if disposition != "to_read" {
		return Handle{}, true, ErrNotFound
	}
	handle, err := s.SubmitClassicalPreparation(ctx, owner, bookID)
	if err != nil {
		return Handle{}, true, err
	}
	return Handle{ID: handle.JobID, DisplayNumber: handle.JobID, JobID: handle.JobID}, true, nil
}
