package analysis

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// ErrAnalysisUpdateChanged reports that the published analysis or the current
// Analysis pipeline identity changed after the learner was offered an Analysis
// update. Nothing is submitted; the learner reviews the refreshed offer.
var ErrAnalysisUpdateChanged = errors.New("analysis update offer changed")

// WithCapabilities lets the service read the current Analysis pipeline
// identity from the running analysis service. Without it every Book is
// treated as current and no Analysis update is offered (ADR 0107).
func (s *Service) WithCapabilities(provider analyzer.CapabilityProvider) *Service {
	s.capabilities = provider
	return s
}

// currentTargets reads the running service's capabilities once. It reports
// false when the identity is unknown: no provider, an unreachable service, or a
// cached answer served because the service is unreachable.
func (s *Service) currentTargets(ctx context.Context) (analyzer.Capabilities, bool) {
	if s.capabilities == nil {
		return analyzer.Capabilities{}, false
	}
	value, err := s.capabilities.GetCapabilities(ctx)
	if err != nil || value.Degraded {
		return analyzer.Capabilities{}, false
	}
	return value, true
}

// currentTarget is the current identity for one language. A language that is
// not reported, not ready, or reported without a normalization identity is
// unknown, so its Books are treated as current.
func currentTarget(value analyzer.Capabilities, language string) (domain.AnalysisUpdateTarget, bool) {
	capability, ok := value.Language(language)
	if !ok || !capability.Ready || strings.TrimSpace(capability.NormalizationProfile) == "" || strings.TrimSpace(capability.NormalizationVersion) == "" {
		return domain.AnalysisUpdateTarget{}, false
	}
	return domain.AnalysisUpdateTarget{
		AnalyzerName: snapshotAnalyzerName, AnalyzerVersion: snapshotAnalyzerVersion,
		NormalizationProfile: capability.NormalizationProfile, NormalizationVersion: capability.NormalizationVersion,
	}, true
}

// pipelineOutdated reports whether a published identity differs from the
// current one. The selection configuration is derived from the unchanged
// snapshot, so a Book is outdated when the snapshot no longer produces the
// identity its run recorded (ADR 0107, amended by ADR 0108).
func pipelineOutdated(published domain.AnalysisPipelineIdentity, target domain.AnalysisUpdateTarget, derivedSelection string) bool {
	return published.AnalyzerName != target.AnalyzerName ||
		published.AnalyzerVersion != target.AnalyzerVersion ||
		published.ConfigIdentity != derivedSelection ||
		published.NormalizationProfile != target.NormalizationProfile ||
		published.NormalizationVersion != target.NormalizationVersion
}

// publishedAnalysis is a Book's published EPUB analysis with the identity its
// artifact recorded and the learner decisions made on it.
type publishedAnalysis struct {
	sourceID, language, runID     string
	snapshotID, contentRevisionID string
	identity                      domain.AnalysisPipelineIdentity
	corrections, exclusions       int
}

// publishedAnalysisSelect reads the published analysis of EPUB sources. The
// normalization identity comes from the artifact, so runs created before
// ADR 0107 are judged without a backfill.
const publishedAnalysisSelect = `SELECT cai.source_material_id::text,s.language,cai.analysis_run_id::text,
	r.analyzer_name,r.analyzer_version,r.config_identity,r.snapshot_id::text,r.content_revision_id::text,
	a.normalization_profile,a.normalization_version,
	(SELECT count(*) FROM occurrence_lemma_corrections o WHERE o.owner_id=cai.owner_id AND o.book_id=cai.book_id AND o.analysis_run_id=cai.analysis_run_id AND NOT o.excluded),
	(SELECT count(*) FROM occurrence_lemma_corrections o WHERE o.owner_id=cai.owner_id AND o.book_id=cai.book_id AND o.analysis_run_id=cai.analysis_run_id AND o.excluded)
	FROM current_analysis_identity cai
	JOIN source_materials s ON s.owner_id=cai.owner_id AND s.id=cai.source_material_id
	JOIN analysis_runs r ON r.owner_id=cai.owner_id AND r.id=cai.analysis_run_id
	JOIN corpora c ON c.owner_id=cai.owner_id AND c.id=cai.corpus_id
	JOIN normalized_corpus_artifacts a ON a.content_hash=c.artifact_hash
	WHERE cai.owner_id=$1 AND s.media_type='application/epub+zip'`

func scanPublishedAnalysis(row pgx.Row) (publishedAnalysis, error) {
	var p publishedAnalysis
	err := row.Scan(&p.sourceID, &p.language, &p.runID, &p.identity.AnalyzerName, &p.identity.AnalyzerVersion, &p.identity.ConfigIdentity, &p.snapshotID, &p.contentRevisionID, &p.identity.NormalizationProfile, &p.identity.NormalizationVersion, &p.corrections, &p.exclusions)
	return p, err
}

func (p publishedAnalysis) candidate(target domain.AnalysisUpdateTarget) domain.AnalysisUpdateCandidate {
	return domain.AnalysisUpdateCandidate{PublishedRunID: p.runID, Target: target, LemmaCorrections: p.corrections, OccurrenceExclusions: p.exclusions}
}

// AnalysisUpdateCandidates returns, by source ID, the sources whose published
// analysis was produced under an Analysis pipeline identity other than the
// current one. An unknown current identity offers nothing. Candidates are only
// advisory: callers decide eligibility through the evidence classification,
// and SubmitAnalysisUpdate revalidates under lock.
func (s *Service) AnalysisUpdateCandidates(ctx context.Context, owner string, sourceIDs []string) (map[string]domain.AnalysisUpdateCandidate, error) {
	candidates := map[string]domain.AnalysisUpdateCandidate{}
	ids := make([]string, 0, len(sourceIDs))
	for _, id := range sourceIDs {
		if _, err := uuid.Parse(strings.TrimSpace(id)); err == nil {
			ids = append(ids, id)
		}
	}
	if s == nil || s.pool == nil || strings.TrimSpace(owner) == "" || len(ids) == 0 {
		return candidates, nil
	}
	capabilities, known := s.currentTargets(ctx)
	if !known {
		return candidates, nil
	}
	rows, err := s.pool.Query(ctx, publishedAnalysisSelect+` AND cai.source_material_id=ANY($2::uuid[])`, owner, ids)
	if err != nil {
		return nil, fmt.Errorf("load published analysis identities: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		published, err := scanPublishedAnalysis(rows)
		if err != nil {
			return nil, fmt.Errorf("read published analysis identity: %w", err)
		}
		candidate, outdated, err := s.outdatedCandidate(ctx, owner, published, capabilities)
		if err != nil {
			return nil, err
		}
		if outdated {
			candidates[published.sourceID] = candidate
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read published analysis identities: %w", err)
	}
	return candidates, nil
}

// outdatedCandidate reports whether one published analysis differs from the
// identity its unchanged snapshot and the running service produce now.
func (s *Service) outdatedCandidate(ctx context.Context, owner string, published publishedAnalysis, capabilities analyzer.Capabilities) (domain.AnalysisUpdateCandidate, bool, error) {
	target, ok := currentTarget(capabilities, published.language)
	if !ok {
		return domain.AnalysisUpdateCandidate{}, false, nil
	}
	derivedSelection, err := s.selectionIdentityForSnapshot(ctx, owner, published)
	if err != nil {
		return domain.AnalysisUpdateCandidate{}, false, err
	}
	if !pipelineOutdated(published.identity, target, derivedSelection) {
		return domain.AnalysisUpdateCandidate{}, false, nil
	}
	return published.candidate(target), true, nil
}

// selectionIdentityForSnapshot derives the selection identity the published
// run's unchanged snapshot produces now. Analysis runs always carry a snapshot
// and content revision (both columns are NOT NULL).
func (s *Service) selectionIdentityForSnapshot(ctx context.Context, owner string, published publishedAnalysis) (string, error) {
	units, err := loadSnapshotUnitsForSelection(ctx, s.pool, owner, published.sourceID, published.contentRevisionID, published.snapshotID)
	if err != nil {
		return "", fmt.Errorf("derive analysis selection identity: %w", err)
	}
	return snapshotConfigIdentityFor(identifyMainText(units)), nil
}

// SubmitAnalysisUpdate queues the learner-confirmed Analysis update of a To
// Read Book. The confirmation carries the published run and the target it was
// offered; both are revalidated under the owner and Book locks, and
// ErrAnalysisUpdateChanged is returned without submitting anything when either
// changed. A Book whose published analysis is already current returns its
// existing run.
func (s *Service) SubmitAnalysisUpdate(ctx context.Context, owner, bookID, sourceID, expectedRunID, expectedTarget string) (result Handle, err error) {
	if s == nil || s.pool == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" || strings.TrimSpace(sourceID) == "" {
		return Handle{}, errors.New("analysis owner, Book and source are required")
	}
	// The capability lookup is a network call, so it happens before the locks;
	// the comparison against the confirmed target happens under them.
	capabilities, known := s.currentTargets(ctx)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, fmt.Errorf("begin analysis update: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 191))`, owner); err != nil {
		return Handle{}, fmt.Errorf("lock analysis update: %w", err)
	}
	if err := admitToReadBookAnalysis(ctx, tx, owner, bookID); err != nil {
		return Handle{}, err
	}
	args, err := loadSubmissionInputs(ctx, tx, owner, bookID, sourceID)
	if err != nil {
		return Handle{}, err
	}
	published, err := scanPublishedAnalysis(tx.QueryRow(ctx, publishedAnalysisSelect+` AND cai.book_id=$2 AND cai.source_material_id=$3`, owner, bookID, sourceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Handle{}, ErrAnalysisUpdateChanged
	}
	if err != nil {
		return Handle{}, fmt.Errorf("load published analysis for update: %w", err)
	}
	target, ok := currentTarget(capabilities, args.Language)
	if !known || !ok || published.runID != expectedRunID || target.Key() != expectedTarget {
		return Handle{}, ErrAnalysisUpdateChanged
	}
	identity := domain.AnalysisPipelineIdentity{AnalyzerName: args.AnalyzerName, AnalyzerVersion: args.AnalyzerVersion, ConfigIdentity: args.ConfigIdentity, NormalizationProfile: target.NormalizationProfile, NormalizationVersion: target.NormalizationVersion}
	result, handled, err := s.submitExistingRun(ctx, tx, owner, args, &identity)
	if err != nil {
		return Handle{}, err
	}
	if !handled {
		if result, err = s.createAnalysisRun(ctx, tx, owner, args); err != nil {
			return Handle{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return result, nil
}

// completedRunMatches reports whether a completed run's artifact records the
// target normalization identity. The Go-side parts are already in the run key.
func completedRunMatches(ctx context.Context, tx pgx.Tx, owner, runID string, target domain.AnalysisPipelineIdentity) (bool, error) {
	var matches bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM analysis_runs r
		JOIN corpora c ON c.owner_id=r.owner_id AND c.id=r.corpus_id AND c.analysis_run_id=r.id
		JOIN normalized_corpus_artifacts a ON a.content_hash=c.artifact_hash
		WHERE r.owner_id=$1 AND r.id=$2 AND a.normalization_profile=$3 AND a.normalization_version=$4
	)`, owner, runID, target.NormalizationProfile, target.NormalizationVersion).Scan(&matches)
	if err != nil {
		return false, fmt.Errorf("compare completed analysis identity: %w", err)
	}
	return matches, nil
}
