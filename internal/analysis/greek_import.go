package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// GreekImportWriter persists an imported classical analysis as an ordinary
// analysis run and publishes it. It never calls the NLP service and never
// enqueues a River job.
type GreekImportWriter struct {
	store *persistence.PostgresStore
}

func NewGreekImportWriter(store *persistence.PostgresStore) *GreekImportWriter {
	return &GreekImportWriter{store: store}
}

// WriteImportedAnalysis stores the imported source material and its analysis and
// publishes it. A repeat with the same pin and interpretation version finds the
// existing run and changes nothing.
func (w *GreekImportWriter) WriteImportedAnalysis(ctx context.Context, in ancientgreek.ImportWriteRequest) (result ancientgreek.ImportWriteResult, err error) {
	if w == nil || w.store == nil {
		return result, errors.New("analysis: Greek import writer store is required")
	}
	stored, err := w.store.PutImportedSourceMaterial(ctx, importedSourceMaterial(in), importedUnits(in), in.BookID)
	if err != nil {
		return result, fmt.Errorf("store imported source material: %w", err)
	}
	var snapshotID string
	if err := w.store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, in.OwnerID, stored.ID).Scan(&snapshotID); err != nil {
		return result, fmt.Errorf("load imported source snapshot: %w", err)
	}

	tx, err := w.store.Pool().Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()

	runID, reimported, err := persistImportedAnalysis(ctx, tx, in, stored, snapshotID)
	if err != nil {
		return result, err
	}
	if reimported {
		return ancientgreek.ImportWriteResult{RunID: runID, Reimported: true}, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	return ancientgreek.ImportWriteResult{RunID: runID}, nil
}

// persistImportedAnalysis writes one import in the caller's transaction. It
// reports reimported when a completed run for the same pin and interpretation
// version already exists, in which case it writes nothing.
func persistImportedAnalysis(ctx context.Context, tx pgx.Tx, in ancientgreek.ImportWriteRequest, stored domain.SourceMaterial, snapshotID string) (string, bool, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 191))`, in.OwnerID); err != nil {
		return "", false, fmt.Errorf("lock imported analysis: %w", err)
	}
	bookID, err := lockImportedBook(ctx, tx, in)
	if err != nil {
		return "", false, err
	}
	runID, reimported, err := findOrCreateImportedRun(ctx, tx, in, stored.ID, stored.ContentRevisionID, snapshotID)
	if err != nil || reimported {
		return runID, reimported, err
	}
	if err := publishImportedRun(ctx, tx, in, stored, runID, bookID); err != nil {
		return "", false, err
	}
	return runID, false, nil
}

// findOrCreateImportedRun returns the existing completed run or records a new
// one.
func findOrCreateImportedRun(ctx context.Context, tx pgx.Tx, in ancientgreek.ImportWriteRequest, sourceID, revisionID, snapshotID string) (string, bool, error) {
	existing, found, err := existingImportedRun(ctx, tx, in, revisionID)
	if err != nil {
		return "", false, err
	}
	if found {
		return existing, true, nil
	}
	runID, err := insertImportedRun(ctx, tx, in, sourceID, revisionID, snapshotID)
	if err != nil {
		return "", false, err
	}
	return runID, false, nil
}

// publishImportedRun writes the corpus, its sentences and tokens, completes the
// run and publishes it.
func publishImportedRun(ctx context.Context, tx pgx.Tx, in ancientgreek.ImportWriteRequest, stored domain.SourceMaterial, runID, bookID string) error {
	args := JobArgs{OwnerID: in.OwnerID, SourceMaterialID: stored.ID, RunID: runID, Language: in.Definition.Language}
	analyzed, err := importedAnalysis(in, args)
	if err != nil {
		return err
	}
	if err := insertCorpusArtifact(ctx, tx, analyzed.artifact); err != nil {
		return err
	}
	if err := insertSharedLemmas(ctx, tx, analyzed.lemmas); err != nil {
		return err
	}
	corpusID, err := insertCompletedCorpus(ctx, tx, args, analyzed)
	if err != nil {
		return err
	}
	if err := persistNormalizedCorpus(ctx, tx, in.OwnerID, analyzed.result.Language, runID, corpusID, analyzed.result); err != nil {
		return err
	}
	if err := completeImportedRun(ctx, tx, args, corpusID); err != nil {
		return err
	}
	return publishEligibleAnalysis(ctx, tx, args, bookID)
}

// importedSourceMaterial builds the source-material row for an import.
func importedSourceMaterial(in ancientgreek.ImportWriteRequest) domain.SourceMaterial {
	return domain.SourceMaterial{
		OwnerID:          in.OwnerID,
		Language:         in.Definition.Language,
		SourceIdentifier: in.Definition.SourceFileName,
		Title:            in.Definition.WorkTitle,
		MediaType:        ancientgreek.SourceMediaType,
		Content:          in.SourceBytes,
		FullText:         in.FullText,
	}
}

// importedUnits builds the single unit that carries the whole imported Reading
// unit. Its identity is the classical unit id the corpus sentences reference.
func importedUnits(in ancientgreek.ImportWriteRequest) domain.ExtractedUnits {
	return domain.ExtractedUnits{
		SchemaVersion: domain.ExtractedUnitsSchemaVersion,
		Units: []domain.ExtractedUnit{{
			ID:               in.UnitID,
			Order:            0,
			SpineIndex:       0,
			Title:            in.UnitTitle,
			TitleSource:      domain.UnitTitleManifestID,
			Text:             in.FullText,
			StartOffset:      0,
			EndOffset:        uint64(len([]rune(in.FullText))),
			PackagePath:      in.Definition.SourceFileName,
			ManifestID:       in.Definition.Key,
			SourceHref:       in.Definition.SourceFileName,
			ResolvedHref:     in.Definition.SourceFileName,
			MediaType:        ancientgreek.SourceMediaType,
			Linear:           true,
			Properties:       []string{},
			NavigationLabels: []string{},
			LandmarkTypes:    []string{},
		}},
	}
}

// lockImportedBook locks the Book and refuses an import whose Book is not the
// definition's Reading unit.
func lockImportedBook(ctx context.Context, tx pgx.Tx, in ancientgreek.ImportWriteRequest) (string, error) {
	var bookID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM books WHERE owner_id=$1 AND id=$2 FOR UPDATE`, in.OwnerID, in.BookID).Scan(&bookID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("analysis: imported Book was not found")
		}
		return "", fmt.Errorf("lock imported Book: %w", err)
	}
	var associated bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM book_reading_units bru
		JOIN reading_units ru ON ru.id = bru.reading_unit_id
		WHERE bru.owner_id=$1 AND bru.book_id=$2 AND ru.definition_key=$3)`,
		in.OwnerID, in.BookID, in.Definition.Key).Scan(&associated); err != nil {
		return "", fmt.Errorf("check imported Book Reading unit: %w", err)
	}
	if !associated {
		return "", errors.New("analysis: imported Book is not the definition's Reading unit")
	}
	return bookID, nil
}

// existingImportedRun reports a completed run for the same pin and
// interpretation version, which makes the import a no-op.
func existingImportedRun(ctx context.Context, tx pgx.Tx, in ancientgreek.ImportWriteRequest, revisionID string) (string, bool, error) {
	var runID, state string
	err := tx.QueryRow(ctx, `SELECT id::text, state FROM analysis_runs
		WHERE owner_id=$1 AND content_revision_id=$2 AND analyzer_name=$3 AND analyzer_version=$4 AND config_identity=$5
		FOR UPDATE`, in.OwnerID, revisionID, in.AnalyzerName, in.AnalyzerVersion, in.ConfigIdentity).Scan(&runID, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("check existing imported analysis: %w", err)
	}
	if state != "completed" {
		return "", false, fmt.Errorf("analysis: existing imported run is %s", state)
	}
	return runID, true, nil
}

// insertImportedRun records the completed run.
func insertImportedRun(ctx context.Context, tx pgx.Tx, in ancientgreek.ImportWriteRequest, sourceID, revisionID, snapshotID string) (string, error) {
	var runID string
	err := tx.QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,attempt_count,syntax_state)
		VALUES($1,$2,$3,$4,$5,$6,$7,'completed',1,$8) RETURNING id::text`,
		in.OwnerID, sourceID, revisionID, snapshotID, in.AnalyzerName, in.AnalyzerVersion, in.ConfigIdentity, string(in.SyntaxState)).Scan(&runID)
	if err != nil {
		return "", fmt.Errorf("insert imported analysis run: %w", err)
	}
	return runID, nil
}

// completeImportedRun records the completion history and attaches the corpus.
func completeImportedRun(ctx context.Context, tx pgx.Tx, args JobArgs, corpusID string) error {
	if _, err := tx.Exec(ctx, `INSERT INTO processing_history(owner_id,corpus_id,operation,status,details,completed_at) VALUES($1,$2,'analysis','complete',$3,now())`, args.OwnerID, corpusID, importedHistoryDetails()); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE analysis_runs SET corpus_id=$3::uuid,updated_at=now(),completed_at=now() WHERE owner_id=$1 AND id=$2`, args.OwnerID, args.RunID, corpusID)
	return err
}

// importedHistoryDetails is the ASCII-only completion detail of an imported run.
func importedHistoryDetails() []byte {
	details, err := json.Marshal(map[string]any{"source": "classical-import"})
	if err != nil {
		return []byte(`{}`)
	}
	return details
}

// importedAnalysis derives the artifact identity, shared lemmas and statistics
// from the source-neutral result.
func importedAnalysis(in ancientgreek.ImportWriteRequest, args JobArgs) (snapshotAnalysis, error) {
	artifactHash := normalizedArtifactHash(importedContentHash(in), "", "", args.AnalyzerName, args.AnalyzerVersion, in.ConfigIdentity, in.Result)
	lemmas, err := aggregateLemmas(artifactHash, in.Result)
	if err != nil {
		return snapshotAnalysis{}, fmt.Errorf("aggregate imported lemmas: %w", err)
	}
	return snapshotAnalysis{
		result:     in.Result,
		lemmas:     lemmas,
		statistics: selection.AnalyzableStatistics(in.Result, selection.DefaultConfig("")),
		artifact: domain.NormalizedArtifact{
			ContentHash:          artifactHash,
			Language:             in.Result.Language,
			SchemaVersion:        in.Result.SchemaVersion,
			NormalizationProfile: in.Result.NormalizationProfile.Name,
			NormalizationVersion: in.Result.NormalizationProfile.Version,
			AnalyzerName:         in.Result.Analysis.AnalyzerName,
			AnalyzerVersion:      in.Result.Analysis.AnalyzerVersion,
		},
	}, nil
}

// importedContentHash is the source bytes' digest, the artifact identity's
// content component.
func importedContentHash(in ancientgreek.ImportWriteRequest) string {
	sum := sha256.Sum256(in.SourceBytes)
	return "sha256:" + hex.EncodeToString(sum[:])
}
