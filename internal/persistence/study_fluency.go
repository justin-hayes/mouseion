package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// GetStudyProgress returns one word's Study progress, ErrNotFound when the
// owner has none with that ID.
func (s *PostgresStore) GetStudyProgress(ctx context.Context, owner, id string) (domain.StudyProgress, error) {
	if !validUUID(id) {
		return domain.StudyProgress{}, ErrNotFound
	}
	row, err := sqlcgen.New(s.pool).GetStudyProgress(ctx, sqlcgen.GetStudyProgressParams{Owner: owner, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StudyProgress{}, ErrNotFound
	}
	if err != nil {
		return domain.StudyProgress{}, fmt.Errorf("get study progress: %w", err)
	}
	return studyProgressFromRow(row), nil
}

// ListStudyFluencyContexts returns every Fluency context of the learner's
// words in the study language, with when each was last answered. Contexts that
// disagree with the learned meaning are included; the domain shows only the
// agreeing ones.
func (s *PostgresStore) ListStudyFluencyContexts(ctx context.Context, owner, language string) ([]domain.FluencyContext, error) {
	contexts, err := s.allStudyFluencyContexts(ctx, owner, language)
	if err != nil {
		return nil, err
	}
	return s.withoutRetiredFluencyContexts(ctx, owner, contexts)
}

// allStudyFluencyContexts lists every stored context, retired ones included.
func (s *PostgresStore) allStudyFluencyContexts(ctx context.Context, owner, language string) ([]domain.FluencyContext, error) {
	rows, err := sqlcgen.New(s.pool).ListStudyFluencyContexts(ctx, sqlcgen.ListStudyFluencyContextsParams{Owner: owner, Language: canonicalization.NormalizeLanguage(language)})
	if err != nil {
		return nil, fmt.Errorf("list study fluency contexts: %w", err)
	}
	answers, err := sqlcgen.New(s.pool).ListStudyFluencyContextResponses(ctx, sqlcgen.ListStudyFluencyContextResponsesParams{Owner: owner, Language: canonicalization.NormalizeLanguage(language)})
	if err != nil {
		return nil, fmt.Errorf("list study fluency context responses: %w", err)
	}
	responses := map[string][]domain.FluencyResponseEvent{}
	for _, answer := range answers {
		responses[answer.ContextID] = append(responses[answer.ContextID], domain.FluencyResponseEvent{Response: domain.FluencyResponse(answer.Response), At: answer.RespondedAt})
	}
	contexts := make([]domain.FluencyContext, 0, len(rows))
	for _, row := range rows {
		item := domain.FluencyContext{
			ID: row.ID, OwnerID: owner, ProgressID: row.ProgressID, ItemKind: studyItemKind(row.ItemKind), Agreement: domain.FluencyAgreement(row.Agreement), Reason: domain.FluencyReason(row.Reason),
			BookID: row.BookID, AnalysisRunID: row.AnalysisRunID, CorpusID: row.CorpusID,
			SentenceOrdinal: row.SentenceOrdinal, TokenOrdinal: row.TokenOrdinal,
			Sentence: row.Sentence, TargetWord: row.TargetWord, Gloss: row.Gloss, SentenceTranslation: row.SentenceTranslation,
			Form:      domain.FluencyForm{Surface: row.FormSurface, Features: row.FormFeatures, Split: row.FormSplit},
			Cited:     domain.StudyCitations{SenseIDs: row.CitedSenseIds, EntryIDs: row.CitedEntryIds, DictionaryPin: row.DictionaryPin},
			CreatedAt: row.CreatedAt, Responses: responses[row.ID],
		}
		if err := json.Unmarshal(row.SentenceTranslationTargets, &item.SentenceTranslationTargets); err != nil {
			return nil, fmt.Errorf("decode fluency context targets: %w", err)
		}
		if row.LastResponseAt.Unix() != 0 {
			item.LastResponseAt = row.LastResponseAt
		}
		contexts = append(contexts, item)
	}
	return contexts, nil
}

// withoutRetiredFluencyContexts leaves out the contexts in a sentence the
// learner retired as the word's context (ADR 0135 §3), so a reported sentence
// is never asked about again. The contexts stay stored. Only the retirements of
// the words that have contexts are read.
func (s *PostgresStore) withoutRetiredFluencyContexts(ctx context.Context, owner string, contexts []domain.FluencyContext) ([]domain.FluencyContext, error) {
	var words []string
	seen := map[string]bool{}
	for _, item := range contexts {
		if !seen[item.ProgressID] {
			seen[item.ProgressID] = true
			words = append(words, item.ProgressID)
		}
	}
	if len(words) == 0 {
		return contexts, nil
	}
	rows, err := sqlcgen.New(s.pool).ListRetiredStudyReportsOfWords(ctx, sqlcgen.ListRetiredStudyReportsOfWordsParams{Owner: owner, Progress: words})
	if err != nil || len(rows) == 0 {
		return contexts, wrapRetiredReports(err)
	}
	byWord := map[string][]domain.StudyReport{}
	for _, report := range studyReportsFromRows(rows) {
		byWord[report.ProgressID] = append(byWord[report.ProgressID], report)
	}
	retired := map[string][]domain.StudyOccurrenceRef{}
	for progress, list := range byWord {
		retired[progress] = s.locatedReportSources(ctx, owner, list)
	}
	kept := contexts[:0:0]
	for _, item := range contexts {
		if !domain.RetiredStudyOccurrence(retired[item.ProgressID], fluencyContextRef(item.BookID, item.AnalysisRunID, item.CorpusID, item.SentenceOrdinal, item.TokenOrdinal)) {
			kept = append(kept, item)
		}
	}
	return kept, nil
}

func wrapRetiredReports(err error) error {
	if err != nil {
		return fmt.Errorf("list retired study reports: %w", err)
	}
	return nil
}

func fluencyContextRef(book, run, corpus string, sentence, token int64) domain.StudyOccurrenceRef {
	return domain.StudyOccurrenceRef{BookID: book, AnalysisRunID: run, CorpusID: corpus, SentenceOrdinal: sentence, TokenOrdinal: token}
}

// hiddenAgreeingFluencyContexts counts the word's agreeing contexts that a
// retired sentence hides. They do not count towards the word's limit, so the
// slot can be refilled. It reads only the word's own contexts and retirements.
func (s *PostgresStore) hiddenAgreeingFluencyContexts(ctx context.Context, owner, progressID string) (int, error) {
	retired, err := s.ListRetiredStudyOccurrences(ctx, owner, progressID)
	if err != nil || len(retired) == 0 {
		return 0, err
	}
	rows, err := sqlcgen.New(s.pool).ListAgreeingStudyFluencyContextSourcesOfWord(ctx, sqlcgen.ListAgreeingStudyFluencyContextSourcesOfWordParams{Owner: owner, Progress: progressID})
	if err != nil {
		return 0, fmt.Errorf("list agreeing study fluency contexts: %w", err)
	}
	hidden := 0
	for _, row := range rows {
		if domain.RetiredStudyOccurrence(retired, fluencyContextRef(row.BookID, row.AnalysisRunID, row.CorpusID, row.SentenceOrdinal, row.TokenOrdinal)) {
			hidden++
		}
	}
	return hidden, nil
}

// RecordStudyFluencyContext stores one assessed occurrence of a mature word. It
// returns domain.ErrFluencyContextNotStored when the word already has its
// domain.FluencyContextsPerWord agreeing contexts, or the occurrence was
// already assessed, and ErrNotFound when the word is not the learner's.
func (s *PostgresStore) RecordStudyFluencyContext(ctx context.Context, owner string, record domain.FluencyContextRecord) error {
	if !validUUID(record.ProgressID) || !validUUID(record.BookID) || !validUUID(record.AnalysisRunID) || !validUUID(record.CorpusID) {
		return ErrNotFound
	}
	if err := checkFluencyReason(record); err != nil {
		return err
	}
	targets := record.SentenceTranslationTargets
	if targets == nil {
		targets = []string{}
	}
	encoded, err := json.Marshal(targets)
	if err != nil {
		return fmt.Errorf("encode fluency context targets: %w", err)
	}
	q := sqlcgen.New(s.pool)
	if _, err := q.GetStudyProgress(ctx, sqlcgen.GetStudyProgressParams{Owner: owner, ID: record.ProgressID}); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("get study progress: %w", err)
	}
	hidden, err := s.hiddenAgreeingFluencyContexts(ctx, owner, record.ProgressID)
	if err != nil {
		return err
	}
	_, err = q.InsertStudyFluencyContext(ctx, sqlcgen.InsertStudyFluencyContextParams{
		Owner: owner, Progress: record.ProgressID, Agreement: string(record.Agreement), Reason: string(record.Reason),
		Book: record.BookID, AnalysisRun: record.AnalysisRunID, Corpus: record.CorpusID,
		SentenceOrdinal: record.SentenceOrdinal, TokenOrdinal: record.TokenOrdinal,
		Sentence: record.Sentence, TargetWord: record.TargetWord, Gloss: record.Gloss, SentenceTranslation: record.SentenceTranslation,
		SentenceTranslationTargets: encoded,
		FormSurface:                record.Form.Surface, FormFeatures: record.Form.Features, FormSplit: record.Form.Split,
		CitedSenseIds: nonNilStrings(record.Cited.SenseIDs), CitedEntryIds: nonNilStrings(record.Cited.EntryIDs), DictionaryPin: record.Cited.DictionaryPin,
		PerWord: domain.FluencyContextsPerWord + hidden,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrFluencyContextNotStored
	}
	if err != nil {
		return fmt.Errorf("record study fluency context: %w", err)
	}
	return nil
}

// RecordStudyFluencyResponse appends the learner's answer to one of their
// agreeing Fluency contexts. It writes nothing else: Study progress and the
// review log are untouched. ErrNotFound means the context is missing, foreign
// or does not agree with the learned meaning.
func (s *PostgresStore) RecordStudyFluencyResponse(ctx context.Context, owner string, record domain.FluencyResponseRecord) error {
	if !validUUID(record.ContextID) {
		return ErrNotFound
	}
	if _, ok := domain.ParseFluencyResponse(string(record.Response)); !ok {
		return fmt.Errorf("record study fluency response: unknown response %q", record.Response)
	}
	_, err := sqlcgen.New(s.pool).AppendStudyFluencyResponse(ctx, sqlcgen.AppendStudyFluencyResponseParams{
		Owner: owner, Context: record.ContextID, Response: string(record.Response), RespondedAt: record.RespondedAt.UTC(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("record study fluency response: %w", err)
	}
	return nil
}

// StudyFluencyResponse is one stored answer, for tests and evidence readers.
type StudyFluencyResponse struct {
	ID, ContextID string
	Response      domain.FluencyResponse
	RespondedAt   time.Time
}

// ListStudyFluencyResponses returns a context's answers, oldest first.
func (s *PostgresStore) ListStudyFluencyResponses(ctx context.Context, owner, contextID string) ([]StudyFluencyResponse, error) {
	if !validUUID(contextID) {
		return nil, nil
	}
	rows, err := sqlcgen.New(s.pool).ListStudyFluencyResponses(ctx, sqlcgen.ListStudyFluencyResponsesParams{Owner: owner, Context: contextID})
	if err != nil {
		return nil, fmt.Errorf("list study fluency responses: %w", err)
	}
	responses := make([]StudyFluencyResponse, 0, len(rows))
	for _, row := range rows {
		responses = append(responses, StudyFluencyResponse{ID: row.ID, ContextID: row.ContextID, Response: domain.FluencyResponse(row.Response), RespondedAt: row.RespondedAt})
	}
	return responses, nil
}

// checkFluencyReason refuses a record whose reason is unknown or does not match
// its agreement.
func checkFluencyReason(record domain.FluencyContextRecord) error {
	reason, ok := domain.ParseFluencyReason(string(record.Reason))
	if !ok || reason.Agreement() != record.Agreement {
		return fmt.Errorf("record study fluency context: reason %q does not give agreement %q", record.Reason, record.Agreement)
	}
	return nil
}

// StudyBookInCorpus reports whether the Book is an active member of the
// learner's My Books, so it is in the Corpus new Fluency contexts draw from.
func (s *PostgresStore) StudyBookInCorpus(ctx context.Context, owner, bookID string) (bool, error) {
	if !validUUID(bookID) {
		return false, nil
	}
	inCorpus, err := sqlcgen.New(s.pool).StudyBookInLibrary(ctx, sqlcgen.StudyBookInLibraryParams{Owner: owner, Book: bookID})
	if err != nil {
		return false, fmt.Errorf("study book in corpus: %w", err)
	}
	return inCorpus, nil
}

// RecordStudyFluencyExhausted records that a scan of the word, begun at
// scannedAt, found no unassessed candidate in the Corpus (#2817). It returns
// ErrNotFound when the word is not the learner's.
func (s *PostgresStore) RecordStudyFluencyExhausted(ctx context.Context, owner, progressID string, scannedAt time.Time) error {
	if !validUUID(progressID) {
		return ErrNotFound
	}
	q := sqlcgen.New(s.pool)
	if _, err := q.GetStudyProgress(ctx, sqlcgen.GetStudyProgressParams{Owner: owner, ID: progressID}); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("get study progress: %w", err)
	}
	if err := q.UpsertStudyFluencyScan(ctx, sqlcgen.UpsertStudyFluencyScanParams{Owner: owner, Progress: progressID, ExhaustedAt: scannedAt}); err != nil {
		return fmt.Errorf("record study fluency exhausted: %w", err)
	}
	return nil
}

// ListStudyFluencyScans returns the recorded scans of the learner's words in
// the study language, each with the word's last lapse (the zero time when it
// never lapsed).
func (s *PostgresStore) ListStudyFluencyScans(ctx context.Context, owner, language string) ([]domain.FluencyScan, error) {
	rows, err := sqlcgen.New(s.pool).ListStudyFluencyScans(ctx, sqlcgen.ListStudyFluencyScansParams{Owner: owner, Language: canonicalization.NormalizeLanguage(language)})
	if err != nil {
		return nil, fmt.Errorf("list study fluency scans: %w", err)
	}
	scans := make([]domain.FluencyScan, 0, len(rows))
	for _, row := range rows {
		scans = append(scans, domain.FluencyScan{ProgressID: row.ProgressID, ExhaustedAt: row.ExhaustedAt, LastLapseAt: epochAsZero(row.LastLapseAt)})
	}
	return scans, nil
}

// StudyCorpusChangedAt is when the Corpus of the study language last changed: a
// Book of the language added to or removed from My Books, or one of its
// analysis runs completed. The zero time means it never has.
func (s *PostgresStore) StudyCorpusChangedAt(ctx context.Context, owner, language string) (time.Time, error) {
	changed, err := sqlcgen.New(s.pool).GetStudyCorpusChangedAt(ctx, sqlcgen.GetStudyCorpusChangedAtParams{Owner: owner, Language: canonicalization.NormalizeLanguage(language)})
	if err != nil {
		return time.Time{}, fmt.Errorf("get study corpus changed at: %w", err)
	}
	return epochAsZero(changed), nil
}

// epochAsZero reads the Unix epoch a query returns for "never" as the zero time.
func epochAsZero(t time.Time) time.Time {
	if t.Unix() == 0 {
		return time.Time{}
	}
	return t
}
