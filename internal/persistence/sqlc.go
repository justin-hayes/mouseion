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

type rowScanner interface {
	Scan(...any) error
}

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

// nullableUUIDArg converts a UUID string into the pgtype form for nullable
// uuid columns; the empty string is written as NULL.
func nullableUUIDArg(value string) pgtype.UUID {
	if value == "" {
		return pgtype.UUID{}
	}
	return uuidArg(value)
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func uuidStringPtr(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	s := uuid.UUID(value.Bytes).String()
	return &s
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

func generatedVocabularyFromFields(ownerID, language, lemma, upos, firstDeckID, firstSourceMaterialID string, firstGeneratedAt pgtype.Timestamptz) domain.GeneratedVocabulary {
	var sourceMaterialID *string
	if firstSourceMaterialID != "" {
		sourceMaterialID = &firstSourceMaterialID
	}
	return domain.GeneratedVocabulary{
		OwnerID: ownerID, Language: language, CanonicalLemma: lemma, UPOS: upos,
		FirstDeckID: firstDeckID, FirstSourceMaterialID: sourceMaterialID,
		FirstGeneratedAt: pgTime(firstGeneratedAt),
	}
}

func bookFromFields(id, ownerID, title, metadataProvenance, languageState, languageTag string, createdAt, updatedAt pgtype.Timestamptz) domain.Book {
	return domain.Book{ID: id, OwnerID: ownerID, Title: title, MetadataProvenance: metadataProvenance, LanguageState: languageState, LanguageTag: languageTag, CreatedAt: pgTime(createdAt), UpdatedAt: pgTime(updatedAt)}
}

func bookAliasFromFields(id, ownerID, bookID, connectionID, aliasType, namespace, value string, createdAt pgtype.Timestamptz) domain.BookAlias {
	return domain.BookAlias{ID: id, OwnerID: ownerID, BookID: bookID, ConnectionID: connectionID, AliasType: aliasType, Namespace: namespace, Value: value, CreatedAt: pgTime(createdAt)}
}

// textArg converts a string into the pgtype form sqlc binds for nullable text
// columns that are guaranteed non-null by the caller's domain rules.
func textArg(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

// nullableTextArg converts a string into a nullable pgtype.Text: the empty
// string is written as NULL, matching the domain's nullable-text conventions.
func nullableTextArg(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

// intArg converts an int into the pgtype form sqlc binds for nullable integer
// columns that are guaranteed non-null by the caller's domain rules.
func intArg(value int) pgtype.Int4 {
	return pgtype.Int4{Int32: int32(value), Valid: true}
}

func pgTimeArgPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func sourceMaterialFromFields(id, ownerID, language, sourceIdentifier, title, mediaType, contentHash, contentDigest, contentRevisionID string, content []byte, fullText string, digestVersion int32, createdAt pgtype.Timestamptz) domain.SourceMaterial {
	return domain.SourceMaterial{
		ID: id, OwnerID: ownerID, Language: language, SourceIdentifier: sourceIdentifier,
		Title: title, MediaType: mediaType, ContentHash: contentHash, ContentDigest: contentDigest,
		ContentRevisionID: contentRevisionID, Content: content, FullText: fullText,
		ContentDigestVersion: int(digestVersion), CreatedAt: pgTime(createdAt),
	}
}

func corpusFromRow(row sqlcgen.GetCorpusRow) domain.Corpus {
	v := domain.Corpus{
		ID: row.ID, OwnerID: row.OwnerID, SourceMaterialID: row.SourceMaterialID,
		ArtifactHash: row.ArtifactHash, AnalysisRunID: row.AnalysisRunID, Status: row.Status,
		CreatedAt: pgTime(row.CreatedAt),
	}
	if row.AnalyzableTokenCount.Valid && row.DistinctLemmaCount.Valid {
		v.Statistics = &domain.AnalysisStatistics{
			AnalyzableTokenCount: row.AnalyzableTokenCount.Int64,
			DistinctLemmaCount:   row.DistinctLemmaCount.Int64,
		}
		if row.SentenceCount.Valid && row.NormalizedTokenCount.Valid && row.EmptySentenceCount.Valid && row.MedianSentenceTokenCount.Valid && row.P90SentenceTokenCount.Valid && row.LongSentenceCount.Valid {
			v.Statistics.TextProfile = &domain.TextProfile{
				SentenceCount:            row.SentenceCount.Int64,
				NormalizedTokenCount:     row.NormalizedTokenCount.Int64,
				EmptySentenceCount:       row.EmptySentenceCount.Int64,
				MedianSentenceTokenCount: row.MedianSentenceTokenCount.Float64,
				P90SentenceTokenCount:    row.P90SentenceTokenCount.Int64,
				LongSentenceCount:        row.LongSentenceCount.Int64,
			}
		}
	}
	return v
}

func analysisJobFromRow(row sqlcgen.ListAnalysisJobsRow) domain.AnalysisJob {
	return domain.AnalysisJob{
		ID: row.RiverJobID, DisplayNumber: row.DisplayNumber, OwnerID: row.JOwnerID,
		SourceMaterialID: row.JSourceMaterialID, ContentHash: row.ContentHash,
		CorpusID: row.CorpusID, AnalysisRunID: row.AnalysisRunID, AnalysisState: row.AnalysisState,
		Progress: int(row.Progress), Error: row.Error,
		CreatedAt: pgTime(row.CreatedAt), UpdatedAt: pgTime(row.UpdatedAt),
	}
}

func catalogueSyncStatusFromRow(ownerID, connectionID, state string, lastSyncedAt pgtype.Timestamptz, lastUpsertedCount int32, lastError string, updatedAt pgtype.Timestamptz) domain.CatalogueSyncStatus {
	status := domain.CatalogueSyncStatus{
		OwnerID: ownerID, ConnectionID: connectionID, State: domain.CatalogueSyncState(state),
		LastUpsertedCount: int(lastUpsertedCount), LastError: lastError,
		LastSyncedAt: pgTimePtr(lastSyncedAt), UpdatedAt: pgTime(updatedAt),
	}
	return status
}

func knownVocabularyFromFields(id, ownerID, language, canonicalLemma, upos string, createdAt pgtype.Timestamptz) domain.KnownVocabulary {
	return domain.KnownVocabulary{
		ID: id, OwnerID: ownerID, Language: language, CanonicalLemma: canonicalLemma,
		UPOS: upos, CreatedAt: pgTime(createdAt),
	}
}

func vocabularyStateFromFields(id, ownerID, language, canonicalLemma, upos, state string, updatedAt pgtype.Timestamptz) domain.VocabularyState {
	return domain.VocabularyState{
		ID: id, OwnerID: ownerID, Language: language, CanonicalLemma: canonicalLemma,
		UPOS: upos, State: state, UpdatedAt: pgTime(updatedAt),
	}
}

func deckPreparationFromModel(row sqlcgen.DeckPreparation) domain.DeckPreparation {
	return domain.DeckPreparation{
		ID: uuidString(row.ID), OwnerID: uuidString(row.OwnerID), SourceMaterialID: uuidString(row.SourceMaterialID),
		State: domain.DeckPreparationState(row.State), Artifact: row.Artifact, Filename: row.Filename,
		DeckName: row.DeckName, ContentHash: row.ContentHash, TotalCards: int(row.TotalCards),
		CardsWithEnglish: int(row.CardsWithEnglish), CardsWithContextualSentenceTranslations: int(row.CardsWithContextualSentenceTranslations),
		QualityOmissions: int(row.QualityOmissions), Error: row.Error, CreatedAt: pgTime(row.CreatedAt),
		UpdatedAt: pgTime(row.UpdatedAt), StartedAt: pgTimePtr(row.StartedAt), CompletedAt: pgTimePtr(row.CompletedAt),
		AnalysisRunID: uuidString(row.AnalysisRunID), CurrentRunID: uuidString(row.CurrentRunID),
		StudyingAt: pgTimePtr(row.StudyingAt), ReviewedAt: pgTimePtr(row.ReviewedAt),
		GraduatedAt: pgTimePtr(row.GraduatedAt), ReleasedAt: pgTimePtr(row.ReleasedAt),
		RetiredAt: pgTimePtr(row.RetiredAt),
	}
}

func deckPreparationVocabularyFromModel(row sqlcgen.DeckPreparationVocabulary) domain.DeckPreparationVocabulary {
	return domain.DeckPreparationVocabulary{
		OwnerID: uuidString(row.OwnerID), DeckPreparationID: uuidString(row.DeckPreparationID),
		Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos,
		GeneratedAt: pgTime(row.GeneratedAt), GraduatedAt: pgTimePtr(row.GraduatedAt),
	}
}

func selectionCandidateFromFields(ownerID, corpusID, language, canonicalLemma, upos string, occurrenceCount int32, observedForms, sentenceReferences, provenance []byte, selectedAt pgtype.Timestamptz, firstEncounter int64) domain.SelectionCandidate {
	return domain.SelectionCandidate{
		OwnerID: ownerID, CorpusID: corpusID, Language: language, CanonicalLemma: canonicalLemma, UPOS: upos,
		OccurrenceCount: int(occurrenceCount), ObservedForms: observedForms, SentenceReferences: sentenceReferences,
		Provenance: provenance, SelectedAt: pgTime(selectedAt), FirstEncounter: firstEncounter,
	}
}

func curatedSentenceFromFields(id, ownerID, exampleSentenceID, language, canonicalLemma, upos, notes string, createdAt pgtype.Timestamptz) domain.CuratedSentence {
	return domain.CuratedSentence{
		ID: id, OwnerID: ownerID, ExampleSentenceID: exampleSentenceID, Language: language,
		CanonicalLemma: canonicalLemma, UPOS: upos, Notes: notes, CreatedAt: pgTime(createdAt),
	}
}

func deckFromFields(id, ownerID, language, name string, createdAt pgtype.Timestamptz) domain.Deck {
	return domain.Deck{ID: id, OwnerID: ownerID, Language: language, Name: name, CreatedAt: pgTime(createdAt)}
}

func cardFromFields(id, ownerID, deckID, dedupKey, canonicalLemma, upos, front, back string, createdAt pgtype.Timestamptz) domain.Card {
	return domain.Card{
		ID: id, OwnerID: ownerID, DeckID: deckID, DedupKey: dedupKey,
		CanonicalLemma: canonicalLemma, UPOS: upos, Front: front, Back: back, CreatedAt: pgTime(createdAt),
	}
}

// opdsFromFields decrypts the stored credential. The language column is a
// legacy artifact not exposed by the domain type.
func opdsFromFields(id, ownerID, name, url, username string, encrypted []byte, language string, createdAt, updatedAt pgtype.Timestamptz) (domain.OpdsConnection, error) {
	password, err := decryptCredential(encrypted)
	if err != nil {
		return domain.OpdsConnection{}, err
	}
	return domain.OpdsConnection{
		ID: id, OwnerID: ownerID, Name: name, URL: url, Username: username, Password: password,
		CreatedAt: pgTime(createdAt), UpdatedAt: pgTime(updatedAt),
	}, nil
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
			ID:                   row.SourceID,
			OwnerID:              row.SourceOwnerID,
			Language:             row.SourceLanguage,
			SourceIdentifier:     row.SourceIdentifier,
			Title:                row.SourceTitle,
			MediaType:            row.SourceMediaType,
			ContentHash:          row.ContentHash,
			ContentDigest:        row.ContentDigest,
			ContentRevisionID:    row.ContentRevisionID,
			ContentSnapshotID:    row.ContentSnapshotID,
			ContentDigestVersion: int(row.DigestVersion),
			CreatedAt:            pgTime(row.SourceCreatedAt),
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

func preparedDeckRunFromModel(m sqlcgen.DeckPreparationRun) domain.PreparedDeckRun {
	return preparedDeckRunFromFields(
		m.ID, m.OwnerID, m.PreparationID, m.RunNumber, m.State, m.TranslationState,
		m.ExecutionMode, m.TargetLanguage, m.ExternalTranslationConsent, m.ExternalTranslationConfigured,
		m.ContextMode, m.Provider, m.ProviderVersion, m.Endpoint, m.Model,
		m.ManifestSchemaVersion, m.RetryPolicyVersion, m.MaxProviderAttempts, m.MaxBatchGenerations,
		m.BatchMaxRequests, m.BatchMaxBytes, m.CandidateCount, m.CompletedCount, m.FailedCount,
		m.FinalizationDispatchGeneration, m.FinalizationDispatchCount, m.FinalizationJobID,
		m.FinalizationClaimToken, m.ErrorClass, m.ErrorCode, m.CreatedAt, m.UpdatedAt,
		m.FinalizationClaimedAt, m.FinalizationLeaseExpiresAt, m.TranslationCompletedAt, m.CompletedAt,
	)
}

func preparedDeckBatchChunkFromModel(m sqlcgen.DeckPreparationBatchChunk) domain.PreparedDeckBatchChunk {
	return domain.PreparedDeckBatchChunk{
		ID: uuidString(m.ID), OwnerID: uuidString(m.OwnerID), PreparationID: uuidString(m.PreparationID), RunID: uuidString(m.RunID),
		ChunkIndex: int(m.ChunkIndex), Generation: int(m.Generation), State: domain.PreparedDeckBatchChunkState(m.State),
		ProviderStatus: pgText(m.ProviderStatus), Model: m.Model, Endpoint: m.Endpoint, SplitReason: m.SplitReason,
		FirstOrdinal: int(m.FirstOrdinal), LastOrdinal: int(m.LastOrdinal), InputDigest: m.InputDigest,
		RequestCount: int(m.RequestCount), InputBytes: m.InputBytes, EstimatedPromptTokens: m.EstimatedPromptTokens,
		CompletedCount: int(m.CompletedCount), FailedCount: int(m.FailedCount), ExpiredCount: int(m.ExpiredCount),
		InputFileID: pgText(m.InputFileID), BatchID: pgText(m.BatchID), OutputFileID: pgText(m.OutputFileID), ErrorFileID: pgText(m.ErrorFileID),
		SubmissionJobID: pgInt8(m.SubmissionJobID), SubmissionGeneration: int(m.SubmissionGeneration), SubmissionClaimToken: uuidString(m.SubmissionClaimToken),
		SubmissionClaimedAt: pgTimePtr(m.SubmissionClaimedAt), SubmissionLeaseExpiresAt: pgTimePtr(m.SubmissionLeaseExpiresAt),
		ReconciliationJobID: pgInt8(m.ReconciliationJobID), ReconciliationGeneration: int(m.ReconciliationGeneration), ReconciliationClaimToken: uuidString(m.ReconciliationClaimToken),
		ReconciliationClaimedAt: pgTimePtr(m.ReconciliationClaimedAt), ReconciliationLeaseExpiresAt: pgTimePtr(m.ReconciliationLeaseExpiresAt),
		ErrorClass: m.ErrorClass, ErrorCode: m.ErrorCode, InputTokens: m.InputTokens, OutputTokens: m.OutputTokens, TotalTokens: m.TotalTokens,
		CreatedAt: pgTime(m.CreatedAt), UpdatedAt: pgTime(m.UpdatedAt), SubmittedAt: pgTimePtr(m.SubmittedAt), LastPolledAt: pgTimePtr(m.LastPolledAt),
		ProviderCompletedAt: pgTimePtr(m.ProviderCompletedAt), ReconciledAt: pgTimePtr(m.ReconciledAt),
		InputFileCleanupState: m.InputFileCleanupState, OutputFileCleanupState: m.OutputFileCleanupState, ErrorFileCleanupState: m.ErrorFileCleanupState,
		InputFileCleanupAttempts: int(m.InputFileCleanupAttempts), OutputFileCleanupAttempts: int(m.OutputFileCleanupAttempts), ErrorFileCleanupAttempts: int(m.ErrorFileCleanupAttempts),
		CleanupErrorClass: m.CleanupErrorClass, CleanupErrorCode: m.CleanupErrorCode, CleanupClaimToken: uuidString(m.CleanupClaimToken),
		CleanupClaimedAt: pgTimePtr(m.CleanupClaimedAt), CleanupLeaseExpiresAt: pgTimePtr(m.CleanupLeaseExpiresAt), CleanupCompletedAt: pgTimePtr(m.CleanupCompletedAt),
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
