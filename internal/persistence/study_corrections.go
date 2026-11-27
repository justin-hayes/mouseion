package persistence

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
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
func (s *PostgresStore) ListStudyOccurrences(ctx context.Context, owner string, identity domain.StudyItem) ([]domain.StudyOccurrenceRef, error) {
	if identity.IsIdiom() {
		return nil, nil // an Idiom's occurrences are not words'; defined with Idiom material
	}
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

// FindStudyOccurrences returns what ReviewStudyEvidence needs of the word's
// occurrences without listing them all: the introducing occurrence in the
// Book's current analysis when it still resolves to the identity, found by key,
// else the first occurrence of the identity that does. Its cost does not grow
// with how often the word occurs (#2705). source should carry its locator, so
// an occurrence an Analysis update re-published is found in its new run.
func (s *PostgresStore) FindStudyOccurrences(ctx context.Context, owner string, identity domain.StudyItem, source domain.StudyOccurrenceRef) ([]domain.StudyOccurrenceRef, error) {
	if identity.IsIdiom() {
		return nil, nil
	}
	identity = normalizedStudyIdentity(identity)
	if refs, err := s.studySourceOccurrence(ctx, owner, identity, source); err != nil || len(refs) > 0 {
		return refs, err
	}
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
		if len(found) > 0 {
			return found[:1], nil
		}
		if !result.HasNext {
			return nil, nil
		}
	}
}

// studySourceOccurrence reads the introducing occurrence's token by key, in its
// own run or at its locator in the Book's current run, and keeps it when it
// resolves to the identity under the same rules the concordance applies.
func (s *PostgresStore) studySourceOccurrence(ctx context.Context, owner string, identity domain.StudyItem, source domain.StudyOccurrenceRef) ([]domain.StudyOccurrenceRef, error) {
	if !validUUID(source.BookID) || !validUUID(source.AnalysisRunID) {
		return nil, nil
	}
	term, _, err := concordanceTerms(identity.Language, identity.CanonicalLemma)
	if err != nil {
		return nil, err
	}
	rows, err := sqlcgen.New(s.pool).ListStudySourceTokens(ctx, sqlcgen.ListStudySourceTokensParams{
		Owner: owner, Language: identity.Language, Book: source.BookID, AnalysisRun: source.AnalysisRunID,
		SentenceOrdinal: source.SentenceOrdinal, TokenOrdinal: source.TokenOrdinal,
		UnitID: source.UnitID, StartOffset: source.StartOffset, EndOffset: source.EndOffset,
	})
	if err != nil {
		return nil, fmt.Errorf("list study source tokens: %w", err)
	}
	// The introducing occurrence itself outranks a token found by its locator.
	slices.SortStableFunc(rows, func(a, b sqlcgen.ListStudySourceTokensRow) int {
		return cmp.Or(cmp.Compare(studySourceRank(a, source), studySourceRank(b, source)),
			cmp.Compare(a.SentenceOrdinal, b.SentenceOrdinal), cmp.Compare(a.TokenOrdinal, b.TokenOrdinal))
	})
	for _, row := range rows {
		if !studySourceTokenResolves(row, identity, term) {
			continue
		}
		occurrence, err := s.studySourceOccurrenceOf(ctx, owner, row)
		if err != nil {
			return nil, err
		}
		refs, err := s.studyOccurrencesOnPage(ctx, owner, identity, []domain.ConcordanceResultOccurrence{occurrence})
		if err != nil || len(refs) > 0 {
			return refs, err
		}
	}
	return nil, nil
}

// studySourceRank is 0 for the source's own token and 1 for one found by locator.
func studySourceRank(row sqlcgen.ListStudySourceTokensRow, source domain.StudyOccurrenceRef) int {
	if row.AnalysisRunID == source.AnalysisRunID && row.SentenceOrdinal == source.SentenceOrdinal && row.TokenOrdinal == source.TokenOrdinal {
		return 0
	}
	return 1
}

// studySourceTokenResolves applies the concordance's lemma match to a token: it
// is not excluded, its effective lemma is the term and its part of speech is
// the identity's.
func studySourceTokenResolves(row sqlcgen.ListStudySourceTokensRow, identity domain.StudyItem, term string) bool {
	effective := row.CanonicalLemma
	if row.DecisionLemma.Valid {
		effective = row.DecisionLemma.String
	}
	return !row.DecisionExcluded && effective == term && (identity.UPOS == "" || row.Upos == identity.UPOS)
}

// studySourceOccurrenceOf projects a token row the way the concordance does,
// with the speaker-label restrictions of its snapshot.
func (s *PostgresStore) studySourceOccurrenceOf(ctx context.Context, owner string, row sqlcgen.ListStudySourceTokensRow) (domain.ConcordanceResultOccurrence, error) {
	structure, err := LoadSnapshotStructure(ctx, s.pool, owner, row.SourceMaterialID, row.SnapshotID)
	if err != nil {
		return domain.ConcordanceResultOccurrence{}, err
	}
	return domain.ConcordanceResultOccurrence{
		BookID: row.BookID, AnalysisRunID: row.AnalysisRunID, CorpusID: row.CorpusID, UnitID: row.UnitID,
		SentenceOrdinal: row.SentenceOrdinal, TokenOrdinal: row.TokenOrdinal,
		UnitStartOffset: row.UnitStartOffset, UnitEndOffset: row.UnitEndOffset,
		Corrected:        row.DecisionLemma.Valid && !row.DecisionExcluded,
		SpeakerLabel:     speakerLabelOverlap(structure, row.UnitID, row.UnitStartOffset, row.UnitEndOffset),
		StudyUnavailable: speakerLabelOverlap(structure, row.UnitID, row.SentenceUnitStartOffset, row.SentenceUnitEndOffset),
	}, nil
}

// studyOccurrencesOnPage keeps the page's occurrences that resolve to the
// identity.
func (s *PostgresStore) studyOccurrencesOnPage(ctx context.Context, owner string, identity domain.StudyItem, occurrences []domain.ConcordanceResultOccurrence) ([]domain.StudyOccurrenceRef, error) {
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
			UnitID: occurrence.UnitID, StartOffset: occurrence.UnitStartOffset, EndOffset: occurrence.UnitEndOffset,
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

// RestoreStudyMaterial returns a word whose material was replaced to that
// material, ending any replacement it has: the word is reviewed in the record
// it was introduced in again. Its progress and learned meaning were never
// touched, so nothing else changes. A material that is current already is
// left as it is; a missing or foreign one is ErrNotFound.
func (s *PostgresStore) RestoreStudyMaterial(ctx context.Context, owner string, word domain.StudyProgress) error {
	if !validUUID(word.MaterialID) {
		return ErrNotFound
	}
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		if _, err := q.SupersedeOtherCurrentStudyMaterialOfWord(ctx, sqlcgen.SupersedeOtherCurrentStudyMaterialOfWordParams{
			Owner: owner, ItemKind: word.ItemKind.String(), Language: word.Language, CanonicalLemma: word.CanonicalLemma, Upos: word.UPOS,
			LexicalDiscriminator: word.LexicalDiscriminator, Keep: word.MaterialID,
		}); err != nil {
			return fmt.Errorf("supersede replacement material: %w", err)
		}
		if _, err := q.UnsupersedeStudyMaterial(ctx, sqlcgen.UnsupersedeStudyMaterialParams{Owner: owner, ID: word.MaterialID}); err != nil {
			return fmt.Errorf("restore study material: %w", err)
		}
		return nil
	})
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

// StudyOccurrenceLocator completes an occurrence with its locator: the unit and
// token span an Analysis update carries decisions across runs by. A token that
// no longer exists is ErrNotFound.
func (s *PostgresStore) StudyOccurrenceLocator(ctx context.Context, owner string, ref domain.StudyOccurrenceRef) (domain.StudyOccurrenceRef, error) {
	if !validUUID(ref.AnalysisRunID) || !validUUID(ref.CorpusID) {
		return domain.StudyOccurrenceRef{}, ErrNotFound
	}
	row, err := sqlcgen.New(s.pool).GetStudyOccurrenceLocator(ctx, sqlcgen.GetStudyOccurrenceLocatorParams{
		Owner: owner, AnalysisRun: ref.AnalysisRunID, Corpus: ref.CorpusID, SentenceOrdinal: ref.SentenceOrdinal, TokenOrdinal: ref.TokenOrdinal,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StudyOccurrenceRef{}, ErrNotFound
	}
	if err != nil {
		return domain.StudyOccurrenceRef{}, fmt.Errorf("get study occurrence locator: %w", err)
	}
	ref.UnitID, ref.StartOffset, ref.EndOffset = row.UnitID, row.StartOffset, row.EndOffset
	return ref, nil
}

// RelocateStudyMaterial names the occurrence a word's ready, current material
// now comes from, after an Analysis update re-published it in a new run. The
// sentence, Gloss and translation are kept.
func (s *PostgresStore) RelocateStudyMaterial(ctx context.Context, owner, materialID string, ref domain.StudyOccurrenceRef) error {
	if !validUUID(materialID) || !validUUID(ref.AnalysisRunID) || !validUUID(ref.CorpusID) {
		return ErrNotFound
	}
	updated, err := sqlcgen.New(s.pool).RelocateStudyMaterial(ctx, sqlcgen.RelocateStudyMaterialParams{
		Owner: owner, ID: materialID, AnalysisRun: uuidArg(ref.AnalysisRunID), Corpus: uuidArg(ref.CorpusID),
		SentenceOrdinal: pgtype.Int8{Int64: ref.SentenceOrdinal, Valid: true}, TokenOrdinal: pgtype.Int8{Int64: ref.TokenOrdinal, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("relocate study material: %w", err)
	}
	if updated == 0 {
		return ErrNotFound
	}
	return nil
}

// SetStudyMaterialEvidence records the occurrences a preparation attempt saw
// on a pending record, before it is found unpreparable. A record that is
// missing or no longer pending is ErrNotFound.
func (s *PostgresStore) SetStudyMaterialEvidence(ctx context.Context, owner, materialID, evidence string) error {
	if !validUUID(materialID) {
		return ErrNotFound
	}
	updated, err := sqlcgen.New(s.pool).SetStudyMaterialEvidence(ctx, sqlcgen.SetStudyMaterialEvidenceParams{Owner: owner, ID: materialID, Evidence: evidence})
	if err != nil {
		return fmt.Errorf("set study material evidence: %w", err)
	}
	if updated == 0 {
		return ErrNotFound
	}
	return nil
}
