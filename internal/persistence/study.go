package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListStudyCandidates returns the effective vocabulary identities Study could
// offer as new words, unordered: the domain owns the stream order. Known and
// Reserved vocabulary are not consulted.
func (s *PostgresStore) ListStudyCandidates(ctx context.Context, owner, language string) ([]domain.StudyCandidate, error) {
	language = canonicalization.NormalizeLanguage(language)
	rows, err := sqlcgen.New(s.pool).ListStudyCandidates(ctx, sqlcgen.ListStudyCandidatesParams{Owner: owner, Language: language})
	if err != nil {
		return nil, fmt.Errorf("list study candidates: %w", err)
	}
	candidates := make([]domain.StudyCandidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, domain.StudyCandidate{
			Language: language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, LexicalDiscriminator: row.LexicalDiscriminator,
			InCurrentReading:    row.InCurrentReading,
			PersonalOccurrences: row.PersonalOccurrences,
		})
	}
	return candidates, nil
}

// ListStudyCandidatesAmong is ListStudyCandidates for the given identities
// only. It reads just their rows, so its cost follows the identities asked for
// and not the library. An identity that is not a candidate is absent.
func (s *PostgresStore) ListStudyCandidatesAmong(ctx context.Context, owner, language string, identities []domain.StudyIdentity) ([]domain.StudyCandidate, error) {
	language = canonicalization.NormalizeLanguage(language)
	wanted := make(map[domain.StudyIdentity]bool, len(identities))
	lemmas := make([]string, 0, len(identities))
	for _, identity := range identities {
		identity.Language = language
		if !wanted[identity] {
			wanted[identity] = true
			lemmas = append(lemmas, identity.CanonicalLemma)
		}
	}
	if len(lemmas) == 0 {
		return nil, nil
	}
	rows, err := sqlcgen.New(s.pool).ListStudyCandidatesAmong(ctx, sqlcgen.ListStudyCandidatesAmongParams{Owner: owner, Language: language, Lemmas: lemmas})
	if err != nil {
		return nil, fmt.Errorf("list study candidates among: %w", err)
	}
	candidates := make([]domain.StudyCandidate, 0, len(rows))
	for _, row := range rows {
		identity := domain.StudyIdentity{Language: language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, LexicalDiscriminator: row.LexicalDiscriminator}
		if wanted[identity] {
			candidates = append(candidates, domain.StudyCandidate{StudyIdentity: identity, InCurrentReading: row.InCurrentReading, PersonalOccurrences: row.PersonalOccurrences})
		}
	}
	return candidates, nil
}

// ListCurrentStudyMaterial returns every word's current Study material in the
// study language.
func (s *PostgresStore) ListCurrentStudyMaterial(ctx context.Context, owner, language string) ([]domain.StudyMaterial, error) {
	rows, err := sqlcgen.New(s.pool).ListCurrentStudyMaterial(ctx, sqlcgen.ListCurrentStudyMaterialParams{Owner: owner, Language: canonicalization.NormalizeLanguage(language)})
	if err != nil {
		return nil, fmt.Errorf("list study material: %w", err)
	}
	material := make([]domain.StudyMaterial, 0, len(rows))
	for _, row := range rows {
		item, err := studyMaterialFromRow(row.ID, row.Language, row.CanonicalLemma, row.Upos, row.LexicalDiscriminator, row.State, row.OmissionReason,
			row.BookID, row.AnalysisRunID, row.CorpusID, row.SentenceOrdinal, row.TokenOrdinal, row.Sentence, row.TargetWord, row.Gloss, row.SentenceTranslation, row.SentenceTranslationTargets)
		if err != nil {
			return nil, err
		}
		item.EvidenceFingerprint = row.EvidenceFingerprint
		item.OwnerID = owner
		item.CreatedAt, item.UpdatedAt = row.CreatedAt, row.UpdatedAt
		material = append(material, item)
	}
	return material, nil
}

// GetStudyMaterial returns one Study material record, ErrNotFound when the
// owner has none with that ID.
func (s *PostgresStore) GetStudyMaterial(ctx context.Context, owner, id string) (domain.StudyMaterial, error) {
	if !validUUID(id) {
		return domain.StudyMaterial{}, ErrNotFound
	}
	row, err := sqlcgen.New(s.pool).GetStudyMaterial(ctx, sqlcgen.GetStudyMaterialParams{Owner: owner, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StudyMaterial{}, ErrNotFound
	}
	if err != nil {
		return domain.StudyMaterial{}, fmt.Errorf("get study material: %w", err)
	}
	item, err := studyMaterialFromRow(row.ID, row.Language, row.CanonicalLemma, row.Upos, row.LexicalDiscriminator, row.State, row.OmissionReason,
		row.BookID, row.AnalysisRunID, row.CorpusID, row.SentenceOrdinal, row.TokenOrdinal, row.Sentence, row.TargetWord, row.Gloss, row.SentenceTranslation, row.SentenceTranslationTargets)
	if err != nil {
		return domain.StudyMaterial{}, err
	}
	item.EvidenceFingerprint = row.EvidenceFingerprint
	item.OwnerID = owner
	item.CreatedAt, item.UpdatedAt = row.CreatedAt, row.UpdatedAt
	return item, nil
}

func studyMaterialFromRow(id, language, lemma, upos, discriminator, state, reason, bookID, runID, corpusID string, sentenceOrdinal, tokenOrdinal int64, sentence, target, gloss, translation string, targets []byte) (domain.StudyMaterial, error) {
	item := domain.StudyMaterial{
		ID:       id,
		Language: language, CanonicalLemma: lemma, UPOS: upos, LexicalDiscriminator: discriminator,
		State: domain.StudyMaterialState(state), OmissionReason: reason,
		BookID: bookID, AnalysisRunID: runID, CorpusID: corpusID, SentenceOrdinal: sentenceOrdinal, TokenOrdinal: tokenOrdinal,
		Sentence: sentence, TargetWord: target, Gloss: gloss, SentenceTranslation: translation,
	}
	if err := json.Unmarshal(targets, &item.SentenceTranslationTargets); err != nil {
		return domain.StudyMaterial{}, fmt.Errorf("decode study material targets: %w", err)
	}
	return item, nil
}

// CountStudyMaterial reports how many words have ready, pending and
// unpreparable current material.
func (s *PostgresStore) CountStudyMaterial(ctx context.Context, owner, language string) (domain.StudyMaterialCounts, error) {
	rows, err := sqlcgen.New(s.pool).CountCurrentStudyMaterial(ctx, sqlcgen.CountCurrentStudyMaterialParams{Owner: owner, Language: canonicalization.NormalizeLanguage(language)})
	if err != nil {
		return domain.StudyMaterialCounts{}, fmt.Errorf("count study material: %w", err)
	}
	var counts domain.StudyMaterialCounts
	for _, row := range rows {
		switch domain.StudyMaterialState(row.State) {
		case domain.StudyMaterialReady:
			counts.Ready = int(row.Total)
		case domain.StudyMaterialPending:
			counts.Preparing = int(row.Total)
		case domain.StudyMaterialUnpreparable:
			counts.Unpreparable = int(row.Total)
		}
	}
	return counts, nil
}

// ReserveStudyMaterial starts a word's material as pending. It reports false
// with no ID when the word already has current material.
func (s *PostgresStore) ReserveStudyMaterial(ctx context.Context, owner string, identity domain.StudyIdentity) (string, bool, error) {
	return reserveStudyMaterial(ctx, sqlcgen.New(s.pool), owner, identity)
}

// ReserveStudyMaterialTx is ReserveStudyMaterial inside the caller's
// transaction, so the study job package enqueues the preparation job in the
// same transaction as the pending record it prepares (ADR 0010).
func (s *PostgresStore) ReserveStudyMaterialTx(ctx context.Context, tx pgx.Tx, owner string, identity domain.StudyIdentity) (string, bool, error) {
	return reserveStudyMaterial(ctx, sqlcgen.New(tx), owner, identity)
}

func reserveStudyMaterial(ctx context.Context, q *sqlcgen.Queries, owner string, identity domain.StudyIdentity) (string, bool, error) {
	identity = normalizedStudyIdentity(identity)
	if identity.Language == "" || identity.CanonicalLemma == "" || identity.UPOS == "" {
		return "", false, fmt.Errorf("reserve study material: %w", domain.ErrInvalidStudyMaterial)
	}
	id, err := q.ReserveStudyMaterial(ctx, sqlcgen.ReserveStudyMaterialParams{
		Owner: owner, Language: identity.Language, CanonicalLemma: identity.CanonicalLemma, Upos: identity.UPOS, LexicalDiscriminator: identity.LexicalDiscriminator,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reserve study material: %w", err)
	}
	return id, true, nil
}

func normalizedStudyIdentity(identity domain.StudyIdentity) domain.StudyIdentity {
	identity.Language = canonicalization.NormalizeLanguage(identity.Language)
	identity.CanonicalLemma = strings.TrimSpace(identity.CanonicalLemma)
	identity.UPOS = strings.ToUpper(strings.TrimSpace(identity.UPOS))
	identity.LexicalDiscriminator = strings.TrimSpace(identity.LexicalDiscriminator)
	return identity
}

// RecordStudyMaterialReady stores a pending word's prepared material. It
// returns ErrNotFound when the record is missing or no longer pending.
func (s *PostgresStore) RecordStudyMaterialReady(ctx context.Context, owner, id string, content domain.StudyMaterialReadyContent) error {
	if !content.Complete() {
		return fmt.Errorf("record study material: %w", domain.ErrInvalidStudyMaterial)
	}
	if !validUUID(id) || !validUUID(content.BookID) || !validUUID(content.AnalysisRunID) || !validUUID(content.CorpusID) {
		return ErrNotFound
	}
	targets := content.SentenceTranslationTargets
	if targets == nil {
		targets = []string{}
	}
	encoded, err := json.Marshal(targets)
	if err != nil {
		return fmt.Errorf("encode study material targets: %w", err)
	}
	updated, err := sqlcgen.New(s.pool).MarkStudyMaterialReady(ctx, sqlcgen.MarkStudyMaterialReadyParams{
		Owner: owner, ID: id,
		Book: uuidArg(content.BookID), AnalysisRun: uuidArg(content.AnalysisRunID), Corpus: uuidArg(content.CorpusID),
		SentenceOrdinal: pgtype.Int8{Int64: content.SentenceOrdinal, Valid: true}, TokenOrdinal: pgtype.Int8{Int64: content.TokenOrdinal, Valid: true},
		Sentence: content.Sentence, TargetWord: content.TargetWord, Gloss: content.Gloss,
		SentenceTranslation: content.SentenceTranslation, SentenceTranslationTargets: encoded,
	})
	if err != nil {
		return fmt.Errorf("record study material: %w", err)
	}
	if updated == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordStudyMaterialUnpreparable marks a pending word unpreparable with its
// reason. It returns ErrNotFound when the record is missing or no longer
// pending.
func (s *PostgresStore) RecordStudyMaterialUnpreparable(ctx context.Context, owner, id, reason string) error {
	if !validUUID(id) {
		return ErrNotFound
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("record study material omission: %w", domain.ErrInvalidStudyMaterial)
	}
	updated, err := sqlcgen.New(s.pool).MarkStudyMaterialUnpreparable(ctx, sqlcgen.MarkStudyMaterialUnpreparableParams{Owner: owner, ID: id, Reason: strings.TrimSpace(reason)})
	if err != nil {
		return fmt.Errorf("record study material omission: %w", err)
	}
	if updated == 0 {
		return ErrNotFound
	}
	return nil
}
