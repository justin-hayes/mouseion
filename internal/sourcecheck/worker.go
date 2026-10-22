package sourcecheck

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/riverqueue/river"
)

// CheckArgs is the exact key of one extraction-only structure check. Every
// field is part of the River uniqueness key and of the durable check record, so
// retries, repeated startup scans and double evidence changes converge on the
// same work.
type CheckArgs struct {
	OwnerID               string `json:"owner_id" river:"unique"`
	SourceMaterialID      string `json:"source_material_id" river:"unique"`
	ContentRevisionID     string `json:"content_revision_id" river:"unique"`
	SnapshotID            string `json:"snapshot_id" river:"unique"`
	TargetContractVersion int    `json:"target_contract_version" river:"unique"`
	ConventionSet         string `json:"structure_convention_set" river:"unique"`
}

func (CheckArgs) Kind() string { return "check_source_structure" }

// Key returns the durable check identity this job records.
func (a CheckArgs) Key() domain.SourceStructureCheckKey {
	return domain.SourceStructureCheckKey{
		OwnerID: a.OwnerID, SourceMaterialID: a.SourceMaterialID,
		ContentRevisionID: a.ContentRevisionID, SnapshotID: a.SnapshotID,
		TargetContractVersion: a.TargetContractVersion, ConventionSet: a.ConventionSet,
	}
}

// Target returns the contract and convention set this job compares against.
func (a CheckArgs) Target() Target {
	return Target{ContractVersion: a.TargetContractVersion, ConventionSet: a.ConventionSet}
}

func (a CheckArgs) validate() error {
	if strings.TrimSpace(a.OwnerID) == "" || strings.TrimSpace(a.SourceMaterialID) == "" ||
		strings.TrimSpace(a.ContentRevisionID) == "" || strings.TrimSpace(a.SnapshotID) == "" {
		return errors.New("sourcecheck: check identity is incomplete")
	}
	return a.Target().Validate()
}

// Worker runs one extraction-only source structure check. It reads the exact
// retained bytes, compares an in-memory extraction with the current snapshot and
// records only a successful outcome. It never writes a snapshot or changes the
// current pointer.
type Worker struct {
	river.WorkerDefaults[CheckArgs]
	Store Store
	// Extract is an optional seam for deterministic tests. Production leaves it
	// nil and uses the v2 extractor.
	Extract func([]byte) (epub.Extraction, error)
}

func (w *Worker) Work(ctx context.Context, job *river.Job[CheckArgs]) (err error) {
	if w == nil || w.Store == nil {
		return errors.New("sourcecheck: worker store is required")
	}
	args := job.Args
	if !args.Target().Active() {
		return nil // inert until the configured target moves past v1
	}
	if err := args.validate(); err != nil {
		return err
	}
	key := args.Key()
	_, found, err := w.Store.GetSourceStructureCheck(ctx, key)
	if err != nil {
		return fmt.Errorf("load source structure check: %w", err)
	}
	if found {
		return nil // exact-key reuse: a successful check is never re-run
	}
	source, current, ok, err := w.loadCurrentEvidence(ctx, args)
	if err != nil || !ok {
		return err
	}

	outcome, identity, err := w.evaluate(source, current, args, key)
	if err != nil {
		return err
	}
	return w.Store.RecordSourceStructureCheck(ctx, domain.SourceStructureCheck{
		OwnerID: args.OwnerID, SourceMaterialID: args.SourceMaterialID,
		ContentRevisionID: args.ContentRevisionID, SnapshotID: args.SnapshotID,
		TargetContractVersion: args.TargetContractVersion, ConventionSet: args.ConventionSet,
		Outcome: outcome, ResultIdentity: identity,
	})
}

// loadCurrentEvidence verifies that the job still names the source's current
// EPUB evidence. An obsolete job returns ok=false and records nothing; the
// newer evidence owns a different exact key.
func (w *Worker) loadCurrentEvidence(ctx context.Context, args CheckArgs) (source domain.SourceMaterial, current domain.ExtractedSnapshot, ok bool, err error) {
	source, err = w.Store.GetSourceMaterial(ctx, args.OwnerID, args.SourceMaterialID)
	if errors.Is(err, domain.ErrNotFound) {
		return source, current, false, nil
	}
	if err != nil {
		return source, current, false, fmt.Errorf("load source material for structure check: %w", err)
	}
	if source.MediaType != epub.MediaType() {
		return source, current, false, nil // imported classical analyses are excluded
	}
	if source.ContentRevisionID != args.ContentRevisionID {
		return source, current, false, nil
	}
	current, err = w.Store.GetCurrentExtractedSnapshot(ctx, args.OwnerID, args.SourceMaterialID)
	if errors.Is(err, domain.ErrExtractedUnitsUnavailable) {
		return source, current, false, nil
	}
	if err != nil {
		return source, current, false, fmt.Errorf("load current snapshot for structure check: %w", err)
	}
	if current.ContentRevisionID != args.ContentRevisionID || current.SnapshotID != args.SnapshotID {
		return source, current, false, nil
	}
	return source, current, needsCheck(current, args.Target()), nil
}

// evaluate extracts the retained bytes in memory and classifies the result. A
// missing retained original is an explicit unsuccessful-adoption outcome, never
// a failed extraction and never an unchanged result.
func (w *Worker) evaluate(source domain.SourceMaterial, current domain.ExtractedSnapshot, args CheckArgs, key domain.SourceStructureCheckKey) (domain.SourceStructureOutcome, string, error) {
	if len(source.Content) == 0 {
		return domain.SourceStructureOriginalUnavailable, UnavailableIdentity(key, source.ContentDigest), nil
	}
	extract := w.Extract
	if extract == nil {
		extract = epub.ExtractV2
	}
	extraction, err := extract(source.Content)
	if err != nil {
		return "", "", fmt.Errorf("extract retained source for structure check: %w", err)
	}
	target := domain.ExtractedSnapshot{
		ContentRevisionID: args.ContentRevisionID,
		Units:             extraction.Book.ExtractedUnits,
		Structure:         extraction.Structure,
	}
	if target.Units.SchemaVersion != args.TargetContractVersion {
		return "", "", fmt.Errorf("sourcecheck: extractor produced contract %d, want %d", target.Units.SchemaVersion, args.TargetContractVersion)
	}
	if target.Structure.ConventionSet != args.ConventionSet {
		return "", "", fmt.Errorf("sourcecheck: extractor produced convention set %q, want %q", target.Structure.ConventionSet, args.ConventionSet)
	}
	return classifyCheck(current, target), ResultIdentity(target), nil
}
