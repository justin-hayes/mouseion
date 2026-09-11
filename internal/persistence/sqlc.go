package persistence

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// queries returns the generated sqlc query layer bound to the pool.
func (s *PostgresStore) queries() *sqlcgen.Queries { return sqlcgen.New(s.pool) }

// withTx folds Begin/defer Rollback/Commit for the callers that need to run
// generated queries against an in-flight transaction. The domain rules around
// the transaction (fences, guards, bounded errors) stay in the callers.
func withTx(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context, pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// uuidArg converts a canonical UUID string into the pgtype form sqlc binds for
// uuid columns.
func uuidArg(value string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(value)
	return id
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func pgText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func pgTime(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func pgTimePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func pgInt8(value pgtype.Int8) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

// textArg converts a string into the pgtype form sqlc binds for nullable text
// columns that are guaranteed non-null by the caller's domain rules.
func textArg(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func myBookFromEvidence(e sqlcgen.MyBooksEvidence) domain.MyBook {
	item := domain.MyBook{
		Book: domain.Book{
			ID:                 e.BookID,
			OwnerID:            e.BookOwnerID,
			Title:              e.BookTitle,
			MetadataProvenance: e.BookMetadataProvenance,
			LanguageState:      e.BookLanguageState,
			LanguageTag:        e.BookLanguageTag,
			CreatedAt:          pgTime(e.BookCreatedAt),
			UpdatedAt:          pgTime(e.BookUpdatedAt),
		},
	}
	if e.Acquired {
		item.Acquired = &domain.SourceMaterialSummary{
			Source: domain.SourceMaterial{
				ID:                   e.SourceID,
				OwnerID:              e.SourceOwnerID,
				Language:             e.SourceLanguage,
				SourceIdentifier:     e.SourceIdentifier,
				Title:                e.SourceTitle,
				MediaType:            e.SourceMediaType,
				ContentHash:          e.SourceContentHash,
				ContentDigest:        e.SourceContentDigest,
				ContentRevisionID:    e.SourceContentRevisionID,
				ContentSnapshotID:    e.SourceContentSnapshotID,
				ContentDigestVersion: int(e.SourceDigestVersion),
				CreatedAt:            pgTime(e.SourceCreatedAt),
			},
			BookTitle:      e.BookTitle,
			BookID:         e.BookID,
			AnalysisStatus: e.AnalysisStatus,
			AnalysisState:  e.AnalysisState,
			AnalysisRunID:  e.AnalysisRunID,
			CorpusID:       e.CorpusID,
			AnalysisJobID:  e.AnalysisJobID,
		}
	}
	return item
}

func sourceMaterialSummaryFromRow(row sqlcgen.ListSourceMaterialsRow) domain.SourceMaterialSummary {
	return domain.SourceMaterialSummary{
		Source: domain.SourceMaterial{
			ID:                   row.SID,
			OwnerID:              row.SOwnerID,
			Language:             row.Language,
			SourceIdentifier:     row.SourceIdentifier,
			Title:                row.Title,
			MediaType:            row.MediaType,
			ContentHash:          row.ContentHash,
			ContentDigest:        row.ContentDigest,
			ContentRevisionID:    row.ContentRevisionID,
			ContentSnapshotID:    row.ContentSnapshotID,
			ContentDigestVersion: int(row.DigestVersion),
			CreatedAt:            pgTime(row.CreatedAt),
		},
		BookID:         row.BookID,
		BookTitle:      row.BookTitle,
		AnalysisStatus: row.AnalysisStatus,
		AnalysisState:  row.AnalysisState,
		AnalysisRunID:  row.AnalysisRunID,
		CorpusID:       row.CorpusID,
		AnalysisJobID:  row.AnalysisJobID,
	}
}

func pgInt4(value pgtype.Int4) int {
	if !value.Valid {
		return 0
	}
	return int(value.Int32)
}

// exampleSentenceFromFields maps the example_sentences row columns onto the
// domain ExampleSentence. The sqlc-generated row structs for the several
// sentence queries are distinct named types with identical fields, so the
// mapping takes the fields directly.
func exampleSentenceFromFields(
	id, ownerID, corpusID pgtype.UUID,
	sentenceKey, sentenceText string,
	sourceLocation []byte,
	language, canonicalLemma, upos pgtype.Text,
	selectionRank, selectionScore pgtype.Int4,
	selectionReasons []byte,
	isChosen bool,
	createdAt pgtype.Timestamptz,
) domain.ExampleSentence {
	return domain.ExampleSentence{
		ID:               uuidString(id),
		OwnerID:          uuidString(ownerID),
		CorpusID:         uuidString(corpusID),
		SentenceKey:      sentenceKey,
		Text:             sentenceText,
		Language:         pgText(language),
		CanonicalLemma:   pgText(canonicalLemma),
		UPOS:             pgText(upos),
		SourceLocation:   sourceLocation,
		SelectionReasons: selectionReasons,
		SelectionRank:    pgInt4(selectionRank),
		SelectionScore:   pgInt4(selectionScore),
		Chosen:           isChosen,
		CreatedAt:        pgTime(createdAt),
	}
}

func preparedDeckOutcomeFromModel(m sqlcgen.DeckPreparationTranslationOutcome) domain.PreparedDeckTranslationOutcome {
	return domain.PreparedDeckTranslationOutcome{
		OwnerID:              uuidString(m.OwnerID),
		PreparationID:        uuidString(m.PreparationID),
		RunID:                uuidString(m.RunID),
		Ordinal:              int(m.Ordinal),
		State:                domain.PreparedDeckOutcomeState(m.State),
		DispatchCount:        int(m.DispatchCount),
		ProviderAttemptCount: int(m.ProviderAttemptCount),
		MaxProviderAttempts:  int(m.MaxProviderAttempts),
		NextAttemptAt:        pgTime(m.NextAttemptAt),
		DispatchGeneration:   int(m.DispatchGeneration),
		RiverJobID:           pgInt8(m.RiverJobID),
		ClaimToken:           uuidString(m.ClaimToken),
		ClaimedAt:            pgTimePtr(m.ClaimedAt),
		LeaseExpiresAt:       pgTimePtr(m.LeaseExpiresAt),
		TerminalAt:           pgTimePtr(m.TerminalAt),
		ErrorClass:           m.ErrorClass,
		ErrorCode:            m.ErrorCode,
		CacheHitCount:        int(m.CacheHitCount),
		ProviderCallCount:    int(m.ProviderCallCount),
		CacheLatency:         time.Duration(m.CacheLatencyMs) * time.Millisecond,
		ProviderLatency:      time.Duration(m.ProviderLatencyMs) * time.Millisecond,
		UpdatedAt:            pgTime(m.UpdatedAt),
	}
}

// preparedDeckRunFromFields maps the deck_preparation_runs row columns onto
// the domain PreparedDeckRun. The two sqlc-generated transition row structs
// have identical fields; the mapping takes the fields directly.
func preparedDeckRunFromFields(
	id, ownerID, preparationID pgtype.UUID,
	runNumber int32,
	state, translationState, executionMode, targetLanguage string,
	externalTranslationConsent, externalTranslationConfigured bool,
	contextMode, provider, providerVersion, endpoint, model pgtype.Text,
	manifestSchemaVersion, retryPolicyVersion, maxProviderAttempts, maxBatchGenerations, batchMaxRequests int32,
	batchMaxBytes int64,
	candidateCount, completedCount, failedCount, finalizationDispatchGeneration, finalizationDispatchCount int32,
	finalizationJobID pgtype.Int8,
	finalizationClaimToken pgtype.UUID,
	errorClass, errorCode string,
	createdAt, updatedAt pgtype.Timestamptz,
	finalizationClaimedAt, finalizationLeaseExpiresAt, translationCompletedAt, completedAt pgtype.Timestamptz,
) domain.PreparedDeckRun {
	return domain.PreparedDeckRun{
		ID:                             uuidString(id),
		OwnerID:                        uuidString(ownerID),
		PreparationID:                  uuidString(preparationID),
		RunNumber:                      int(runNumber),
		State:                          domain.PreparedDeckRunState(state),
		TranslationState:               domain.PreparedDeckTranslationState(translationState),
		ExecutionMode:                  domain.PreparedDeckExecutionMode(executionMode),
		TargetLanguage:                 targetLanguage,
		ExternalTranslationConsent:     externalTranslationConsent,
		ExternalTranslationConfigured:  externalTranslationConfigured,
		ContextMode:                    pgText(contextMode),
		Provider:                       pgText(provider),
		ProviderVersion:                pgText(providerVersion),
		Endpoint:                       pgText(endpoint),
		Model:                          pgText(model),
		ManifestSchemaVersion:          int(manifestSchemaVersion),
		RetryPolicyVersion:             int(retryPolicyVersion),
		MaxProviderAttempts:            int(maxProviderAttempts),
		MaxBatchGenerations:            int(maxBatchGenerations),
		BatchMaxRequests:               int(batchMaxRequests),
		BatchMaxBytes:                  batchMaxBytes,
		CandidateCount:                 int(candidateCount),
		CompletedCount:                 int(completedCount),
		FailedCount:                    int(failedCount),
		FinalizationDispatchGeneration: int(finalizationDispatchGeneration),
		FinalizationDispatchCount:      int(finalizationDispatchCount),
		FinalizationJobID:              pgInt8(finalizationJobID),
		FinalizationClaimToken:         uuidString(finalizationClaimToken),
		ErrorClass:                     errorClass,
		ErrorCode:                      errorCode,
		CreatedAt:                      pgTime(createdAt),
		UpdatedAt:                      pgTime(updatedAt),
		FinalizationClaimedAt:          pgTimePtr(finalizationClaimedAt),
		FinalizationLeaseExpiresAt:     pgTimePtr(finalizationLeaseExpiresAt),
		TranslationCompletedAt:         pgTimePtr(translationCompletedAt),
		CompletedAt:                    pgTimePtr(completedAt),
	}
}
