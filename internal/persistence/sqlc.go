package persistence

import (
	"context"
	"errors"
	"strings"
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

// validUUID reports whether Postgres would read value as a uuid. The pgtype
// scan accepts the same spellings the uuid type does.
func validUUID(value string) bool {
	var id pgtype.UUID
	return id.Scan(value) == nil
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
		CorpusID: row.CorpusID, AnalysisRunID: row.AnalysisRunID,
		BookID:      row.BookID,
		BookTitle:   row.BookTitle,
		Lifecycle:   domain.JobLifecycleForRiverState(row.AnalysisState),
		Stage:       domain.BookPreparationStage(row.AnalysisStage),
		FailureCode: domain.BookPreparationCode(row.FailureCode),
		Progress:    row.Progress,
		CreatedAt:   row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func catalogueSyncStatusFromRow(ownerID, connectionID, state string, lastSyncedAt pgtype.Timestamptz, lastUpsertedCount int, lastError string, updatedAt time.Time) domain.CatalogueSyncStatus {
	return domain.CatalogueSyncStatus{
		OwnerID: ownerID, ConnectionID: connectionID, State: domain.CatalogueSyncState(state),
		LastUpsertedCount: lastUpsertedCount, LastError: lastError,
		LastSyncedAt: pgTimePtr(lastSyncedAt), UpdatedAt: updatedAt,
	}
}

func knownVocabularyFromFields(id, ownerID, language, canonicalLemma, upos, discriminator, headword string, createdAt time.Time) domain.KnownVocabulary {
	return domain.KnownVocabulary{
		ID: id, OwnerID: ownerID, Language: language, CanonicalLemma: canonicalLemma,
		UPOS: upos, LexicalDiscriminator: discriminator, Headword: headword, CreatedAt: createdAt,
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

// opdsFromFields decrypts the stored credential.
func opdsFromFields(id, ownerID, name, url, username string, encrypted []byte, createdAt, updatedAt time.Time) (domain.OpdsConnection, error) {
	password, err := decryptCredential(encrypted)
	if err != nil {
		return domain.OpdsConnection{}, err
	}
	return domain.OpdsConnection{
		ID: id, OwnerID: ownerID, Name: name, URL: url, Username: username, Password: password,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}, nil
}

func myBookFromEvidence(e sqlcgen.MyBooksEvidence, failureCode string) domain.MyBook {
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
			BookTitle:     e.BookTitle,
			BookAuthor:    e.BookAuthor,
			BookID:        e.BookID,
			Signals:       analysisSignalsFromView(e.SourceID, e.SourceMediaType, e.SourceContentRevisionID, e.SourceContentSnapshotID, e.AnalysisStatus, e.AnalysisState, e.AnalysisRunID, failureCode),
			AnalysisRunID: e.AnalysisRunID,
			CorpusID:      e.CorpusID,
			AnalysisJobID: e.AnalysisJobID,
		}
	}
	return item
}

// analysisSignalsFromView maps source_material_evidence columns to the typed
// analysis signals. It is the only place that reads the view's status and state
// strings; the domain classifier never sees them.
func analysisSignalsFromView(sourceID, mediaType, revisionID, snapshotID, status, state, runID, failureCode string) domain.AnalysisSignals {
	return domain.AnalysisSignals{
		Content:     contentSignalFromView(sourceID, mediaType, revisionID, snapshotID),
		Published:   publishedSignalFromView(status, runID),
		LatestRun:   latestRunSignalFromView(status, state),
		FailureCode: domain.BookPreparationCode(failureCode),
	}
}

func contentSignalFromView(sourceID, mediaType, revisionID, snapshotID string) domain.ContentSignal {
	mediaType = strings.TrimSpace(mediaType)
	switch {
	case sourceID == "":
		return domain.ContentNotAcquired
	case revisionID == "" || snapshotID == "":
		return domain.ContentNoCurrentRevision
	case strings.EqualFold(mediaType, "application/epub+zip"):
		return domain.ContentCurrentEPUB
	case strings.EqualFold(mediaType, domain.ImportedSourceMediaType):
		return domain.ContentCurrentImported
	default:
		return domain.ContentNonEPUB
	}
}

// publishedSignalFromView reads the current-analysis identity: a run identifier
// means a published analysis exists; the view's stale status marks a publication
// that no longer matches the content.
func publishedSignalFromView(status, runID string) domain.PublishedAnalysisSignal {
	switch {
	case runID != "":
		return domain.PublishedCurrent
	case status == "stale":
		return domain.PublishedStale
	default:
		return domain.PublishedNone
	}
}

func latestRunSignalFromView(status, state string) domain.LatestRunSignal {
	switch state {
	case "queued":
		return domain.RunQueued
	case "running":
		return domain.RunRunning
	case "failed":
		return domain.RunFailed
	case "cancelled":
		return domain.RunCancelled
	case "completed":
		switch status {
		case "analyzed":
			return domain.RunCompleted
		case "not analyzed":
			return domain.RunPublicationFailed
		default:
			return domain.RunPublicationPending
		}
	default:
		return domain.RunNone
	}
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
	}, row.AnalysisFailureCode)
	if row.ReadingUnitKey != "" || row.WorkKey != "" {
		book.ReadingUnit = &domain.BookReadingUnit{
			OwnerID:          row.BookOwnerID,
			BookID:           row.BookID,
			ReadingUnitKey:   row.ReadingUnitKey,
			WorkKey:          row.WorkKey,
			WorkTitle:        row.WorkTitle,
			WorkAuthor:       row.WorkAuthor,
			OriginalTitle:    row.GreekTitle,
			EditionLabel:     row.EditionLabel,
			ConstituentBook:  row.ConstituentBook,
			ConstituentBooks: row.ConstituentBooks,
			Revision:         row.Revision,
			Withdrawn:        row.Withdrawn,
		}
	}
	book.IsCurrentReading = row.IsCurrentReading
	book.EverCurrentReading = row.EverCurrentReading
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
		BookID:        row.BookID,
		BookTitle:     row.BookTitle,
		BookAuthor:    row.BookAuthor,
		Signals:       analysisSignalsFromView(row.SourceID, row.SourceMediaType, row.ContentRevisionID, row.ContentSnapshotID, row.AnalysisStatus, row.AnalysisState, row.AnalysisRunID, row.AnalysisFailureCode),
		AnalysisRunID: row.AnalysisRunID,
		CorpusID:      row.CorpusID,
		AnalysisJobID: row.AnalysisJobID,
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
