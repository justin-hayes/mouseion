package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// queries returns the generated sqlc query layer bound to the pool.
func (s *PostgresStore) queries() *sqlcgen.Queries { return sqlcgen.New(s.pool) }

// withTx folds Begin/defer Rollback/Commit for the callers that need to run
// generated queries against an in-flight transaction. The domain rules around
// the transaction (fences, guards, bounded errors) stay in the callers.
func withTx(ctx context.Context, pool *pgxpool.Pool, fn func(context.Context, pgx.Tx) error) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if err := fn(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// uuidArg converts a canonical UUID string into the pgtype form sqlc binds for
// nullable uuid columns whose domain rules guarantee a value.
func uuidArg(value string) pgtype.UUID {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		panic("invalid UUID passed to persistence boundary: " + err.Error())
	}
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

func nullableInt8Arg(corpusID string, value int64) pgtype.Int8 {
	if corpusID == "" {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: value, Valid: true}
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

func generatedVocabularyFromFields(ownerID, language, lemma, upos, firstDeckID, firstSourceMaterialID string, firstGeneratedAt time.Time) domain.GeneratedVocabulary {
	var sourceMaterialID *string
	if firstSourceMaterialID != "" {
		sourceMaterialID = &firstSourceMaterialID
	}
	return domain.GeneratedVocabulary{
		OwnerID: ownerID, Language: language, CanonicalLemma: lemma, UPOS: upos,
		FirstDeckID: firstDeckID, FirstSourceMaterialID: sourceMaterialID,
		FirstGeneratedAt: firstGeneratedAt,
	}
}

func sourceMaterialFromFields(id, ownerID, language, sourceIdentifier, title, mediaType, contentHash, contentDigest, contentRevisionID string, content []byte, fullText string, digestVersion int, createdAt time.Time) domain.SourceMaterial {
	return domain.SourceMaterial{
		ID: id, OwnerID: ownerID, Language: language, SourceIdentifier: sourceIdentifier,
		Title: title, MediaType: mediaType, ContentHash: contentHash, ContentDigest: contentDigest,
		ContentRevisionID: contentRevisionID, Content: content, FullText: fullText,
		ContentDigestVersion: digestVersion, CreatedAt: createdAt,
	}
}

func corpusFromRow(row sqlcgen.GetCorpusRow) domain.Corpus {
	v := domain.Corpus{
		ID: row.ID, OwnerID: row.OwnerID, SourceMaterialID: row.SourceMaterialID,
		ArtifactHash: row.ArtifactHash, AnalysisRunID: row.AnalysisRunID, Status: row.Status,
		CreatedAt: row.CreatedAt,
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
		Progress: row.Progress, Error: row.Error,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func catalogueSyncStatusFromRow(ownerID, connectionID, state string, lastSyncedAt pgtype.Timestamptz, lastUpsertedCount int, lastError string, updatedAt time.Time) domain.CatalogueSyncStatus {
	return domain.CatalogueSyncStatus{
		OwnerID: ownerID, ConnectionID: connectionID, State: domain.CatalogueSyncState(state),
		LastUpsertedCount: lastUpsertedCount, LastError: lastError,
		LastSyncedAt: pgTimePtr(lastSyncedAt), UpdatedAt: updatedAt,
	}
}

func knownVocabularyFromFields(id, ownerID, language, canonicalLemma, upos string, createdAt time.Time) domain.KnownVocabulary {
	return domain.KnownVocabulary{
		ID: id, OwnerID: ownerID, Language: language, CanonicalLemma: canonicalLemma,
		UPOS: upos, CreatedAt: createdAt,
	}
}

func deckPreparationFromModel(row sqlcgen.DeckPreparation) domain.DeckPreparation {
	return domain.DeckPreparation{
		ID: row.ID, OwnerID: row.OwnerID, SourceMaterialID: row.SourceMaterialID,
		BookID:         uuidString(row.BookID),
		GoalSnapshotID: uuidString(row.GoalSnapshotID),
		State:          domain.DeckPreparationState(row.State), Artifact: row.Artifact, Filename: row.Filename,
		DeckName: row.DeckName, ContentHash: row.ContentHash, TotalCards: row.TotalCards,
		CardsWithEnglish: row.CardsWithEnglish, CardsWithContextualSentenceTranslations: row.CardsWithContextualSentenceTranslations, CardsWithFallbackGloss: row.CardsWithFallbackGloss,
		QualityOmissions: row.QualityOmissions, Error: row.Error, CreatedAt: row.CreatedAt,
		RenderInputVersion: row.RenderInputVersion, PresentationVersion: row.PresentationVersion,
		DeckRevision: row.DeckRevision,
		UpdatedAt:    row.UpdatedAt, StartedAt: pgTimePtr(row.StartedAt), CompletedAt: pgTimePtr(row.CompletedAt),
		AnalysisRunID: uuidString(row.AnalysisRunID), CurrentRunID: uuidString(row.CurrentRunID),
		StudyingAt: pgTimePtr(row.StudyingAt), ReviewedAt: pgTimePtr(row.ReviewedAt),
		GraduatedAt: pgTimePtr(row.GraduatedAt), ReleasedAt: pgTimePtr(row.ReleasedAt),
		RetiredAt: pgTimePtr(row.RetiredAt),
	}
}

func selectionCandidateFromFields(ownerID, corpusID, language, canonicalLemma, upos string, occurrenceCount int, observedForms, sentenceReferences, provenance []byte, selectedAt time.Time, firstEncounter int64) domain.SelectionCandidate {
	return domain.SelectionCandidate{
		OwnerID: ownerID, CorpusID: corpusID, Language: language, CanonicalLemma: canonicalLemma, UPOS: upos,
		OccurrenceCount: occurrenceCount, ObservedForms: observedForms, SentenceReferences: sentenceReferences,
		Provenance: provenance, SelectedAt: selectedAt, FirstEncounter: firstEncounter,
	}
}

func curatedSentenceFromFields(id, ownerID, exampleSentenceID, language, canonicalLemma, upos, notes string, createdAt time.Time) domain.CuratedSentence {
	return domain.CuratedSentence{
		ID: id, OwnerID: ownerID, ExampleSentenceID: exampleSentenceID, Language: language,
		CanonicalLemma: canonicalLemma, UPOS: upos, Notes: notes, CreatedAt: createdAt,
	}
}

func cardFromFields(id, ownerID, deckID, dedupKey, canonicalLemma, upos, front, back string, createdAt time.Time) domain.Card {
	return domain.Card{
		ID: id, OwnerID: ownerID, DeckID: deckID, DedupKey: dedupKey,
		CanonicalLemma: canonicalLemma, UPOS: upos, Front: front, Back: back, CreatedAt: createdAt,
	}
}

// opdsFromFields decrypts the stored credential. The language column is a
// legacy artifact not exposed by the domain type.
func opdsFromFields(id, ownerID, name, url, username string, encrypted []byte, language string, createdAt, updatedAt time.Time) (domain.OpdsConnection, error) {
	password, err := decryptCredential(encrypted)
	if err != nil {
		return domain.OpdsConnection{}, err
	}
	return domain.OpdsConnection{
		ID: id, OwnerID: ownerID, Name: name, URL: url, Username: username, Password: password,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}, nil
}

func myBookFromEvidence(e sqlcgen.MyBooksEvidence) domain.MyBook {
	item := domain.MyBook{
		Book: domain.Book{
			ID:                 e.BookID,
			OwnerID:            e.BookOwnerID,
			Title:              e.BookTitle,
			Author:             e.BookAuthor,
			MetadataProvenance: e.BookMetadataProvenance,
			LanguageState:      e.BookLanguageState,
			LanguageTag:        e.BookLanguageTag,
			CreatedAt:          e.BookCreatedAt,
			UpdatedAt:          e.BookUpdatedAt,
		},
		Cover: domain.BookCover{
			State:  e.BookCoverState,
			Width:  e.BookCoverWidth,
			Height: e.BookCoverHeight,
		},
	}
	if e.Acquired {
		createdAt := time.Time{}
		if e.SourceCreatedAt != nil {
			createdAt = *e.SourceCreatedAt
		}
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
				ContentDigestVersion: e.SourceDigestVersion,
				CreatedAt:            createdAt,
			},
			BookTitle:      e.BookTitle,
			BookAuthor:     e.BookAuthor,
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

func myBookFromBrowseRow(row sqlcgen.BrowseMyBooksEvidenceRow) domain.MyBook {
	book := myBookFromEvidence(sqlcgen.MyBooksEvidence{
		BookID: row.BookID, BookOwnerID: row.BookOwnerID, BookTitle: row.BookTitle,
		BookMetadataProvenance: row.BookMetadataProvenance, BookLanguageState: row.BookLanguageState,
		BookLanguageTag: row.BookLanguageTag, BookCreatedAt: row.BookCreatedAt, BookUpdatedAt: row.BookUpdatedAt,
		SourceID: row.SourceID, SourceOwnerID: row.SourceOwnerID, SourceLanguage: row.SourceLanguage,
		SourceIdentifier: row.SourceIdentifier, SourceTitle: row.SourceTitle, SourceMediaType: row.SourceMediaType,
		SourceContentHash: row.SourceContentHash, SourceContentDigest: row.SourceContentDigest,
		SourceContentRevisionID: row.SourceContentRevisionID, SourceContentSnapshotID: row.SourceContentSnapshotID,
		SourceDigestVersion: row.SourceDigestVersion, SourceCreatedAt: row.SourceCreatedAt, Acquired: row.Acquired,
		AnalysisStatus: row.AnalysisStatus, AnalysisState: row.AnalysisState, AnalysisRunID: row.AnalysisRunID,
		CorpusID: row.CorpusID, AnalysisJobID: row.AnalysisJobID, BookAuthor: row.BookAuthor,
		BookCoverState: row.BookCoverState, BookCoverWidth: row.BookCoverWidth, BookCoverHeight: row.BookCoverHeight,
	})
	book.IsCurrentReading = row.IsCurrentReading
	book.CompletionCount = int(row.CompletionCount)
	book.LatestCompletionAt = completionTime(row.LatestCompletedAt)
	book.LatestCompletionSource = domain.ReadingCompletionSource(row.LatestCompletionSource)
	return book
}

func completionTime(value any) *time.Time {
	switch completedAt := value.(type) {
	case time.Time:
		return &completedAt
	case pgtype.Timestamptz:
		if completedAt.Valid {
			return &completedAt.Time
		}
	}
	return nil
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
			ContentDigestVersion: row.DigestVersion,
			CreatedAt:            row.SourceCreatedAt,
		},
		BookID:         row.BookID,
		BookTitle:      row.BookTitle,
		BookAuthor:     row.BookAuthor,
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

// intArg converts an int into the validated pgtype form sqlc binds for integer
// columns.
func intArg(value int) (pgtype.Int4, error) {
	converted, err := checked.Int32FromInt(value)
	if err != nil {
		return pgtype.Int4{}, err
	}
	return pgtype.Int4{Int32: converted, Valid: true}, nil
}

func pgTimeArgPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

// exampleSentenceFromFields maps the example_sentences row columns onto the
// domain ExampleSentence. The sqlc-generated row structs for the several
// sentence queries are distinct named types with identical fields, so the
// mapping takes the fields directly.
func exampleSentenceFromFields(
	id, ownerID, corpusID string,
	sentenceKey, sentenceText string,
	sourceLocation []byte,
	language, canonicalLemma, upos pgtype.Text,
	selectionRank, selectionScore pgtype.Int4,
	selectionReasons []byte,
	isChosen bool,
	createdAt time.Time,
) domain.ExampleSentence {
	return domain.ExampleSentence{
		ID:               id,
		OwnerID:          ownerID,
		CorpusID:         corpusID,
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
		CreatedAt:        createdAt,
	}
}

func preparedDeckOutcomeFromModel(m sqlcgen.DeckPreparationTranslationOutcome) domain.PreparedDeckTranslationOutcome {
	return domain.PreparedDeckTranslationOutcome{
		OwnerID:              m.OwnerID,
		PreparationID:        m.PreparationID,
		RunID:                m.RunID,
		Ordinal:              m.Ordinal,
		State:                domain.PreparedDeckOutcomeState(m.State),
		DispatchCount:        m.DispatchCount,
		ProviderAttemptCount: m.ProviderAttemptCount,
		MaxProviderAttempts:  m.MaxProviderAttempts,
		NextAttemptAt:        m.NextAttemptAt,
		DispatchGeneration:   m.DispatchGeneration,
		RiverJobID:           pgInt8(m.RiverJobID),
		ClaimToken:           uuidString(m.ClaimToken),
		ClaimedAt:            pgTimePtr(m.ClaimedAt),
		LeaseExpiresAt:       pgTimePtr(m.LeaseExpiresAt),
		TerminalAt:           pgTimePtr(m.TerminalAt),
		ErrorClass:           m.ErrorClass,
		ErrorCode:            m.ErrorCode,
		CacheHitCount:        m.CacheHitCount,
		ProviderCallCount:    m.ProviderCallCount,
		CacheLatency:         time.Duration(m.CacheLatencyMs) * time.Millisecond,
		ProviderLatency:      time.Duration(m.ProviderLatencyMs) * time.Millisecond,
		UpdatedAt:            m.UpdatedAt,
	}
}

func preparedDeckRunFromModel(m sqlcgen.DeckPreparationRun) domain.PreparedDeckRun {
	return preparedDeckRunFromFields(
		m.ID, m.OwnerID, m.PreparationID, m.RunNumber, m.State, m.TranslationState,
		m.ExecutionMode, m.TargetLanguage, m.ExternalTranslationConsent, m.ExternalTranslationConfigured,
		m.ContextMode, m.Provider, m.ProviderVersion, m.Endpoint, m.Model,
		m.ManifestSchemaVersion, m.RenderInputVersion, m.PresentationVersion, m.RetryPolicyVersion, m.MaxProviderAttempts, m.MaxBatchGenerations,
		m.BatchMaxRequests, m.BatchMaxBytes, m.CandidateCount, m.CompletedCount, m.FailedCount,
		m.FinalizationDispatchGeneration, m.FinalizationDispatchCount, m.FinalizationJobID,
		m.FinalizationClaimToken, m.ErrorClass, m.ErrorCode, m.CreatedAt, m.UpdatedAt,
		m.FinalizationClaimedAt, m.FinalizationLeaseExpiresAt, m.TranslationCompletedAt, m.CompletedAt,
	)
}

// preparedDeckRunFromFields maps a deck_preparation_runs row onto the domain
// PreparedDeckRun. The run model and the generated finalization row structs
// are distinct named types with identical fields; the mapping takes the fields
// directly.
func preparedDeckRunFromFields(
	id, ownerID, preparationID string,
	runNumber int,
	state, translationState, executionMode, targetLanguage string,
	externalTranslationConsent, externalTranslationConfigured bool,
	contextMode, provider, providerVersion, endpoint, model pgtype.Text,
	manifestSchemaVersion, renderInputVersion, presentationVersion, retryPolicyVersion, maxProviderAttempts, maxBatchGenerations, batchMaxRequests int,
	batchMaxBytes int64,
	candidateCount, completedCount, failedCount, finalizationDispatchGeneration, finalizationDispatchCount int,
	finalizationJobID pgtype.Int8,
	finalizationClaimToken pgtype.UUID,
	errorClass, errorCode string,
	createdAt, updatedAt time.Time,
	finalizationClaimedAt, finalizationLeaseExpiresAt, translationCompletedAt, completedAt pgtype.Timestamptz,
) domain.PreparedDeckRun {
	return domain.PreparedDeckRun{
		ID:                             id,
		OwnerID:                        ownerID,
		PreparationID:                  preparationID,
		RunNumber:                      runNumber,
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
		ManifestSchemaVersion:          manifestSchemaVersion,
		RenderInputVersion:             renderInputVersion,
		PresentationVersion:            presentationVersion,
		RetryPolicyVersion:             retryPolicyVersion,
		MaxProviderAttempts:            maxProviderAttempts,
		MaxBatchGenerations:            maxBatchGenerations,
		BatchMaxRequests:               batchMaxRequests,
		BatchMaxBytes:                  batchMaxBytes,
		CandidateCount:                 candidateCount,
		CompletedCount:                 completedCount,
		FailedCount:                    failedCount,
		FinalizationDispatchGeneration: finalizationDispatchGeneration,
		FinalizationDispatchCount:      finalizationDispatchCount,
		FinalizationJobID:              pgInt8(finalizationJobID),
		FinalizationClaimToken:         uuidString(finalizationClaimToken),
		ErrorClass:                     errorClass,
		ErrorCode:                      errorCode,
		CreatedAt:                      createdAt,
		UpdatedAt:                      updatedAt,
		FinalizationClaimedAt:          pgTimePtr(finalizationClaimedAt),
		FinalizationLeaseExpiresAt:     pgTimePtr(finalizationLeaseExpiresAt),
		TranslationCompletedAt:         pgTimePtr(translationCompletedAt),
		CompletedAt:                    pgTimePtr(completedAt),
	}
}

func preparedDeckBatchChunkFromModel(m sqlcgen.DeckPreparationBatchChunk) domain.PreparedDeckBatchChunk {
	return domain.PreparedDeckBatchChunk{
		ID: m.ID, OwnerID: m.OwnerID, PreparationID: m.PreparationID, RunID: m.RunID,
		ChunkIndex: m.ChunkIndex, Generation: m.Generation, State: domain.PreparedDeckBatchChunkState(m.State),
		ProviderStatus: pgText(m.ProviderStatus), Model: m.Model, Endpoint: m.Endpoint, SplitReason: m.SplitReason,
		FirstOrdinal: m.FirstOrdinal, LastOrdinal: m.LastOrdinal, InputDigest: m.InputDigest,
		RequestCount: m.RequestCount, InputBytes: m.InputBytes, EstimatedPromptTokens: m.EstimatedPromptTokens,
		CompletedCount: m.CompletedCount, FailedCount: m.FailedCount, ExpiredCount: m.ExpiredCount,
		InputFileID: pgText(m.InputFileID), BatchID: pgText(m.BatchID), OutputFileID: pgText(m.OutputFileID), ErrorFileID: pgText(m.ErrorFileID),
		SubmissionJobID: pgInt8(m.SubmissionJobID), SubmissionGeneration: m.SubmissionGeneration, SubmissionClaimToken: uuidString(m.SubmissionClaimToken),
		SubmissionClaimedAt: pgTimePtr(m.SubmissionClaimedAt), SubmissionLeaseExpiresAt: pgTimePtr(m.SubmissionLeaseExpiresAt),
		ReconciliationJobID: pgInt8(m.ReconciliationJobID), ReconciliationGeneration: m.ReconciliationGeneration, ReconciliationClaimToken: uuidString(m.ReconciliationClaimToken),
		ReconciliationClaimedAt: pgTimePtr(m.ReconciliationClaimedAt), ReconciliationLeaseExpiresAt: pgTimePtr(m.ReconciliationLeaseExpiresAt),
		ErrorClass: m.ErrorClass, ErrorCode: m.ErrorCode, InputTokens: m.InputTokens, OutputTokens: m.OutputTokens, TotalTokens: m.TotalTokens,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, SubmittedAt: pgTimePtr(m.SubmittedAt), LastPolledAt: pgTimePtr(m.LastPolledAt),
		ProviderCompletedAt: pgTimePtr(m.ProviderCompletedAt), ReconciledAt: pgTimePtr(m.ReconciledAt),
		InputFileCleanupState: m.InputFileCleanupState, OutputFileCleanupState: m.OutputFileCleanupState, ErrorFileCleanupState: m.ErrorFileCleanupState,
		InputFileCleanupAttempts: m.InputFileCleanupAttempts, OutputFileCleanupAttempts: m.OutputFileCleanupAttempts, ErrorFileCleanupAttempts: m.ErrorFileCleanupAttempts,
		CleanupErrorClass: m.CleanupErrorClass, CleanupErrorCode: m.CleanupErrorCode, CleanupClaimToken: uuidString(m.CleanupClaimToken),
		CleanupClaimedAt: pgTimePtr(m.CleanupClaimedAt), CleanupLeaseExpiresAt: pgTimePtr(m.CleanupLeaseExpiresAt), CleanupCompletedAt: pgTimePtr(m.CleanupCompletedAt),
	}
}
