package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListStudyOccurrences returns the occurrences that resolve to the word's
// identity under current evidence: confirmed Lemma corrections, Occurrence
// exclusions and the published analyses of the learner's Books. It reads the
// same concordance Study material is prepared from, so a sentence Study could
// show is exactly an occurrence listed here. An excluded occurrence, one in
// speaker-label text, and one whose effective lexical discriminator is another
// word's are not the identity's.
func (s *PostgresStore) ListStudyOccurrences(ctx context.Context, owner string, identity domain.StudyIdentity) ([]domain.StudyOccurrenceRef, error) {
	identity = normalizedStudyIdentity(identity)
	var refs []domain.StudyOccurrenceRef
	for page := 1; ; page++ {
		result, err := s.ListVocabularyConcordance(ctx, owner, identity.Language, domain.ConcordanceLookup{
			Language: identity.Language, Term: identity.CanonicalLemma, Match: domain.ConcordanceMatchLemma,
			UPOS: identity.UPOS, Priority: domain.ConcordancePriorityNone, Page: page,
		})
		if err != nil {
			return nil, err
		}
		found, err := s.studyOccurrencesOnPage(ctx, owner, identity, result.Occurrences)
		if err != nil {
			return nil, err
		}
		refs = append(refs, found...)
		if !result.HasNext {
			return refs, nil
		}
	}
}

// studyOccurrencesOnPage keeps the page's occurrences that resolve to the
// identity.
func (s *PostgresStore) studyOccurrencesOnPage(ctx context.Context, owner string, identity domain.StudyIdentity, occurrences []domain.ConcordanceResultOccurrence) ([]domain.StudyOccurrenceRef, error) {
	usable := make([]domain.ConcordanceResultOccurrence, 0, len(occurrences))
	ordinals := map[string][]int64{}
	for _, occurrence := range occurrences {
		if occurrence.Excluded || occurrence.StudyUnavailable || occurrence.SpeakerLabel {
			continue
		}
		usable = append(usable, occurrence)
		ordinals[occurrence.CorpusID] = append(ordinals[occurrence.CorpusID], occurrence.SentenceOrdinal)
	}
	sentences := make(map[string]map[int64]analyzer.Sentence, len(ordinals))
	for corpusID, list := range ordinals {
		loaded, err := s.ListCorpusSentences(ctx, owner, corpusID, list)
		if err != nil {
			return nil, err
		}
		sentences[corpusID] = loaded
	}
	var refs []domain.StudyOccurrenceRef
	for _, occurrence := range usable {
		sentence, ok := sentences[occurrence.CorpusID][occurrence.SentenceOrdinal]
		if !ok || effectiveStudyDiscriminator(occurrence, sentence) != identity.LexicalDiscriminator {
			continue
		}
		refs = append(refs, domain.StudyOccurrenceRef{
			BookID: occurrence.BookID, AnalysisRunID: occurrence.AnalysisRunID, CorpusID: occurrence.CorpusID,
			SentenceOrdinal: occurrence.SentenceOrdinal, TokenOrdinal: occurrence.TokenOrdinal,
		})
	}
	return refs, nil
}

// effectiveStudyDiscriminator is the lexical discriminator an occurrence
// counts under: none after a Lemma correction, otherwise its token's.
func effectiveStudyDiscriminator(occurrence domain.ConcordanceResultOccurrence, sentence analyzer.Sentence) string {
	if occurrence.Corrected || occurrence.TokenOrdinal < 0 || occurrence.TokenOrdinal >= int64(len(sentence.Tokens)) {
		return ""
	}
	return sentence.Tokens[occurrence.TokenOrdinal].LexicalDiscriminator
}

// SupersedeStudyMaterial replaces a word's current Study material after a
// correction: the record is kept for the review log, and the word has no
// current material until new material is prepared. A record that is missing,
// foreign or already superseded is ErrNotFound.
func (s *PostgresStore) SupersedeStudyMaterial(ctx context.Context, owner, materialID string) error {
	if !validUUID(materialID) {
		return ErrNotFound
	}
	updated, err := sqlcgen.New(s.pool).SupersedeStudyMaterial(ctx, sqlcgen.SupersedeStudyMaterialParams{Owner: owner, ID: materialID})
	if err != nil {
		return fmt.Errorf("supersede study material: %w", err)
	}
	if updated == 0 {
		return ErrNotFound
	}
	return nil
}

// SetStudyWordWithdrawn withdraws a word by a correction at now, or returns it
// when withdrawn is false. Its progress is kept either way. A word already in
// that state is left as it is; a missing or foreign word is ErrNotFound.
func (s *PostgresStore) SetStudyWordWithdrawn(ctx context.Context, owner, progressID string, withdrawn bool, now time.Time) error {
	if !validUUID(progressID) {
		return ErrNotFound
	}
	q := sqlcgen.New(s.pool)
	if _, err := q.LockStudyProgress(ctx, sqlcgen.LockStudyProgressParams{Owner: owner, ID: progressID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("get study progress: %w", err)
	}
	stamp := pgtype.Timestamptz{Time: now.UTC(), Valid: withdrawn}
	if _, err := q.SetStudyProgressWithdrawn(ctx, sqlcgen.SetStudyProgressWithdrawnParams{Owner: owner, ID: progressID, WithdrawnAt: stamp}); err != nil {
		return fmt.Errorf("set study word withdrawn: %w", err)
	}
	return nil
}

// CompleteStudyReintroductions moves each word whose replacement Study
// material is ready onto it: the new Gloss becomes the learned meaning and the
// word is marked corrected at now, with its FSRS state untouched. A word whose
// replacement material cannot be prepared has no usable occurrence left and is
// withdrawn.
func (s *PostgresStore) CompleteStudyReintroductions(ctx context.Context, owner, language string, now time.Time) error {
	language = canonicalization.NormalizeLanguage(language)
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		if _, err := q.CompleteStudyReintroductions(ctx, sqlcgen.CompleteStudyReintroductionsParams{Owner: owner, Language: language, Now: now.UTC()}); err != nil {
			return fmt.Errorf("complete study reintroductions: %w", err)
		}
		if _, err := q.WithdrawUnpreparableStudyReintroductions(ctx, sqlcgen.WithdrawUnpreparableStudyReintroductionsParams{Owner: owner, Language: language, Now: now.UTC()}); err != nil {
			return fmt.Errorf("withdraw unpreparable study reintroductions: %w", err)
		}
		return nil
	})
}
