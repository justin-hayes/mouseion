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
			ID: row.ID, OwnerID: owner, ProgressID: row.ProgressID, Agreement: domain.FluencyAgreement(row.Agreement),
			BookID: row.BookID, AnalysisRunID: row.AnalysisRunID, CorpusID: row.CorpusID,
			SentenceOrdinal: row.SentenceOrdinal, TokenOrdinal: row.TokenOrdinal,
			Sentence: row.Sentence, TargetWord: row.TargetWord, Gloss: row.Gloss, SentenceTranslation: row.SentenceTranslation,
			Form:      domain.FluencyForm{Surface: row.FormSurface, Features: row.FormFeatures, Split: row.FormSplit},
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

// RecordStudyFluencyContext stores one assessed occurrence of a mature word. It
// returns domain.ErrFluencyContextNotStored when the word already has its
// domain.FluencyContextsPerWord agreeing contexts, or the occurrence was
// already assessed, and ErrNotFound when the word is not the learner's.
func (s *PostgresStore) RecordStudyFluencyContext(ctx context.Context, owner string, record domain.FluencyContextRecord) error {
	if !validUUID(record.ProgressID) || !validUUID(record.BookID) || !validUUID(record.AnalysisRunID) || !validUUID(record.CorpusID) {
		return ErrNotFound
	}
	if record.Agreement != domain.FluencyAgrees && record.Agreement != domain.FluencyDisagrees {
		return fmt.Errorf("record study fluency context: unknown agreement %q", record.Agreement)
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
	_, err = q.InsertStudyFluencyContext(ctx, sqlcgen.InsertStudyFluencyContextParams{
		Owner: owner, Progress: record.ProgressID, Agreement: string(record.Agreement),
		Book: record.BookID, AnalysisRun: record.AnalysisRunID, Corpus: record.CorpusID,
		SentenceOrdinal: record.SentenceOrdinal, TokenOrdinal: record.TokenOrdinal,
		Sentence: record.Sentence, TargetWord: record.TargetWord, Gloss: record.Gloss, SentenceTranslation: record.SentenceTranslation,
		SentenceTranslationTargets: encoded,
		FormSurface:                record.Form.Surface, FormFeatures: record.Form.Features, FormSplit: record.Form.Split,
		PerWord: domain.FluencyContextsPerWord,
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
