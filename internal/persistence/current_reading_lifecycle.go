package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

func currentReadingFromValues(ownerID, language, bookID, snapshotID, sourceMaterialID, analysisRunID, contentRevisionID, contentSnapshotID, corpusID string, snapshotSize int, createdAt, updatedAt time.Time) domain.CurrentReading {
	return domain.CurrentReading{
		OwnerID: ownerID, Language: language, BookID: bookID, SnapshotID: snapshotID,
		SourceMaterialID: sourceMaterialID, AnalysisRunID: analysisRunID,
		ContentRevisionID: contentRevisionID, ContentSnapshotID: contentSnapshotID,
		CorpusID: corpusID, SnapshotSize: snapshotSize,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	}
}

func currentReadingFromRow(row sqlcgen.GetCurrentReadingRow, snapshotSize int) domain.CurrentReading {
	return currentReadingFromValues(row.GOwnerID, row.Language, row.GBookID, row.SnapshotID, row.SourceMaterialID, row.AnalysisRunID, row.ContentRevisionID, row.ContentSnapshotID, row.CorpusID, snapshotSize, row.CreatedAt, row.UpdatedAt)
}

func listCurrentReadingSnapshotVocabulary(ctx context.Context, q *sqlcgen.Queries, owner, snapshotID string) ([]domain.SelectionCandidate, error) {
	rows, err := q.ListCurrentReadingSnapshotVocabulary(ctx, sqlcgen.ListCurrentReadingSnapshotVocabularyParams{Owner: owner, Snapshot: snapshotID})
	if err != nil {
		return nil, err
	}
	result := make([]domain.SelectionCandidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, snapshotCandidateFromFields(row.OwnerID, row.CorpusID, row.Language, row.CanonicalLemma, row.Upos, row.LexicalDiscriminator, row.Headword, row.OccurrenceCount, row.ObservedForms, row.EligibleSentenceRefs, row.Provenance, row.SelectedAt, row.FirstEncounter))
	}
	return result, nil
}

func createCurrentReadingSnapshot(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, language, bookID string, identity sqlcgen.GetCurrentReadingCandidateIdentityRow) (sqlcgen.CreateCurrentReadingSnapshotRow, []domain.SelectionCandidate, error) {
	snapshot, err := q.CreateCurrentReadingSnapshot(ctx, sqlcgen.CreateCurrentReadingSnapshotParams{
		Owner: owner, Language: language, Book: bookID, SourceMaterial: identity.CaSourceMaterialID,
		AnalysisRun: identity.CaAnalysisRunID, ContentRevision: identity.CaContentRevisionID,
		ContentSnapshot: identity.CaSnapshotID, Corpus: identity.CaCorpusID,
	})
	if err != nil {
		return sqlcgen.CreateCurrentReadingSnapshotRow{}, nil, err
	}
	// The freeze always reads the analysis projection, the same tokens,
	// occurrence decisions and discriminated identities Browse reads. The
	// retiring Browse-selection store is not a source: it was written once at
	// analysis time with that moment's Known and Reserved identities removed,
	// so it silently omits identities that are no longer excluded (#2217).
	candidates, err := projectedAnalysisCurrentReadingCandidates(ctx, tx, q, owner, language, identity)
	if err != nil {
		return sqlcgen.CreateCurrentReadingSnapshotRow{}, nil, err
	}
	candidates, err = eligibleCurrentReadingCandidates(ctx, tx, q, owner, language, candidates)
	if err != nil {
		return sqlcgen.CreateCurrentReadingSnapshotRow{}, nil, err
	}
	for _, candidate := range candidates {
		if err := q.InsertCurrentReadingSnapshotVocabulary(ctx, sqlcgen.InsertCurrentReadingSnapshotVocabularyParams{
			Owner: owner, Snapshot: snapshot.ID, Corpus: candidate.CorpusID, Language: candidate.Language,
			CanonicalLemma: candidate.CanonicalLemma, Upos: candidate.UPOS,
			LexicalDiscriminator: candidate.LexicalDiscriminator, Headword: candidate.Headword,
			OccurrenceCount: candidate.OccurrenceCount,
			ObservedForms:   candidate.ObservedForms, EligibleSentenceRefs: candidate.SentenceReferences,
			Provenance: candidate.Provenance, FirstEncounter: candidate.FirstEncounter, SelectedAt: candidate.SelectedAt,
		}); err != nil {
			return sqlcgen.CreateCurrentReadingSnapshotRow{}, nil, err
		}
	}
	return snapshot, candidates, nil
}

type currentReadingVocabularyIdentity struct {
	lemma         string
	upos          string
	discriminator string
}

// eligibleCurrentReadingCandidates applies the common learner-state exclusions
// and uses only ready current per-Book projections for the across-Books
// route. Readiness and totals share one SQL statement snapshot.
func eligibleCurrentReadingCandidates(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, language string, candidates []domain.SelectionCandidate) ([]domain.SelectionCandidate, error) {
	known, err := listKnownVocabulary(ctx, tx, owner, language)
	if err != nil {
		return nil, err
	}
	reserved, err := listReservedVocabulary(ctx, tx, owner, language)
	if err != nil {
		return nil, err
	}
	eligibility := selection.NewEligibility(known, reserved)
	acrossBooks, err := currentAcrossBookCounts(ctx, tx, owner, language, "", requestedCurrentReadingIdentities(candidates, eligibility))
	if err != nil {
		return nil, err
	}
	return filterCurrentReadingCandidates(ctx, tx, q, owner, language, candidates, eligibility, acrossBooks)
}

// requestedCurrentReadingIdentities collects the distinct below-the-floor
// identities that need a current across-Books total, skipping candidates the
// learner state already excludes.
func requestedCurrentReadingIdentities(candidates []domain.SelectionCandidate, eligibility selection.Eligibility) []currentReadingVocabularyIdentity {
	identities := make([]currentReadingVocabularyIdentity, 0)
	seen := make(map[currentReadingVocabularyIdentity]struct{})
	for _, candidate := range candidates {
		if candidate.OccurrenceCount >= selection.DefaultRecurringMinOccurrences || eligibility.ExcludesLearnerState(candidate) {
			continue
		}
		identity := currentReadingVocabularyIdentity{lemma: candidate.CanonicalLemma, upos: candidate.UPOS, discriminator: candidate.LexicalDiscriminator}
		if _, ok := seen[identity]; !ok {
			seen[identity] = struct{}{}
			identities = append(identities, identity)
		}
	}
	return identities
}

// filterCurrentReadingCandidates keeps the candidates that meet the frequency
// rule and are not held by an active snapshot, then applies the sentence-quality
// gate as the last step.
func filterCurrentReadingCandidates(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, language string, candidates []domain.SelectionCandidate, eligibility selection.Eligibility, acrossBooks map[currentReadingVocabularyIdentity]int64) ([]domain.SelectionCandidate, error) {
	selected := make([]domain.SelectionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !eligibility.AllowsBookDeckCandidate(candidate, currentReadingCandidateFrequency(candidate, acrossBooks)) {
			continue
		}
		reserved, err := currentReadingCandidateReserved(ctx, q, owner, language, candidate)
		if err != nil {
			return nil, err
		}
		if reserved {
			continue
		}
		selected = append(selected, candidate)
	}
	return gatedCurrentReadingCandidates(ctx, tx, owner, selected)
}

// gatedCurrentReadingCandidates applies ADR 0106 after the frequency rule: a
// candidate is frozen only when at least one of its sentence references in this
// Book passes the shared sentence-quality gate. It resolves references exactly
// as the deck does, by persisted sentence ordinal with each sentence's start
// offset, so the two can never disagree about a sentence.
func gatedCurrentReadingCandidates(ctx context.Context, tx pgx.Tx, owner string, candidates []domain.SelectionCandidate) ([]domain.SelectionCandidate, error) {
	ordinalsByCorpus := candidateSentenceOrdinalsByCorpus(candidates)
	sentencesByCorpus := make(map[string]map[int64]analyzer.Sentence, len(ordinalsByCorpus))
	for corpusID, ordinalSet := range ordinalsByCorpus {
		sentences, err := loadCorpusSentenceOrdinals(ctx, tx, owner, corpusID, ordinalSet)
		if err != nil {
			return nil, err
		}
		sentencesByCorpus[corpusID] = sentences
	}
	frozen := make([]domain.SelectionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !sentencequality.AnyReferencePasses(candidate, sentencesByCorpus[candidate.CorpusID]) {
			continue
		}
		frozen = append(frozen, candidate)
	}
	return frozen, nil
}

// currentReadingCandidateFrequency is the across-Books total that can qualify a
// candidate below the in-Book floor.
func currentReadingCandidateFrequency(candidate domain.SelectionCandidate, acrossBooks map[currentReadingVocabularyIdentity]int64) int64 {
	if candidate.OccurrenceCount < selection.DefaultRecurringMinOccurrences {
		return acrossBooks[currentReadingVocabularyIdentity{lemma: candidate.CanonicalLemma, upos: candidate.UPOS, discriminator: candidate.LexicalDiscriminator}]
	}
	return 0
}

// currentReadingCandidateReserved reports whether an active snapshot already
// holds the candidate's identity.
func currentReadingCandidateReserved(ctx context.Context, q *sqlcgen.Queries, owner, language string, candidate domain.SelectionCandidate) (bool, error) {
	return q.IsCurrentReadingVocabularyReserved(ctx, sqlcgen.IsCurrentReadingVocabularyReservedParams{
		Owner: owner, Language: language, CanonicalLemma: candidate.CanonicalLemma, Upos: candidate.UPOS,
		LexicalDiscriminator: candidate.LexicalDiscriminator,
	})
}

// currentAcrossBookCounts totals each identity's projected count over the
// learner's currently analyzed Books in a language, optionally leaving one Book
// out. Counts and readiness share one SQL statement snapshot; the totals are
// withheld with a pending or unavailable error while any such Book lacks a
// ready projection. The identity includes the lexical discriminator, so source
// homographs are totalled separately.
func currentAcrossBookCounts(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, owner, language, excludeBook string, identities []currentReadingVocabularyIdentity) (map[currentReadingVocabularyIdentity]int64, error) {
	acrossBooks := make(map[currentReadingVocabularyIdentity]int64, len(identities))
	if len(identities) == 0 {
		return acrossBooks, nil
	}
	lemmas, upos := make([]string, 0, len(identities)), make([]string, 0, len(identities))
	discriminators := make([]string, 0, len(identities))
	for _, identity := range identities {
		lemmas, upos = append(lemmas, identity.lemma), append(upos, identity.upos)
		discriminators = append(discriminators, identity.discriminator)
	}
	rows, err := q.Query(ctx, `
WITH requested AS (
 SELECT lemma,upos,discriminator FROM unnest($3::text[],$4::text[],$5::text[]) AS r(lemma,upos,discriminator)
), current_books AS MATERIALIZED (
 SELECT cai.owner_id,cai.book_id,cai.analysis_run_id,cai.corpus_id,b.language_tag AS language
 FROM current_analysis_identity cai
 JOIN books b ON b.owner_id=cai.owner_id AND b.id=cai.book_id
 WHERE cai.owner_id=$1 AND b.language_state='chosen' AND b.language_tag=$2
), projection_status AS (
 SELECT NOT EXISTS (
   SELECT 1 FROM current_books cb
   LEFT JOIN vocabulary_browse_count_readiness r ON r.owner_id=cb.owner_id AND r.book_id=cb.book_id
     AND r.analysis_run_id=cb.analysis_run_id AND r.corpus_id=cb.corpus_id
     AND r.language=cb.language AND r.builder_version=3
   WHERE r.owner_id IS NULL
 ) AS ready,
 EXISTS (
   SELECT 1 FROM current_books cb
   LEFT JOIN vocabulary_browse_count_readiness r ON r.owner_id=cb.owner_id AND r.book_id=cb.book_id
     AND r.analysis_run_id=cb.analysis_run_id AND r.corpus_id=cb.corpus_id
     AND r.language=cb.language AND r.builder_version=3
   JOIN river_job failed ON failed.kind='rebuild_vocabulary_browse_counts' AND failed.state='discarded'
     AND failed.args->>'owner_id'=cb.owner_id::text AND failed.args->>'book_id'=cb.book_id::text
     AND failed.args->>'run_id'=cb.analysis_run_id::text
   WHERE r.owner_id IS NULL
     AND NOT EXISTS (SELECT 1 FROM river_job active
       WHERE active.kind='rebuild_vocabulary_browse_counts'
         AND active.state IN ('available','pending','running','retryable','scheduled')
         AND active.args->>'owner_id'=cb.owner_id::text AND active.args->>'book_id'=cb.book_id::text
         AND active.args->>'run_id'=cb.analysis_run_id::text)
 ) AS unavailable
), counts AS MATERIALIZED (
 SELECT c.canonical_lemma,c.upos,c.lexical_discriminator,sum(c.occurrence_count)::bigint AS occurrences
 FROM current_books cb
 JOIN vocabulary_browse_count_readiness r ON r.owner_id=cb.owner_id AND r.book_id=cb.book_id
   AND r.analysis_run_id=cb.analysis_run_id AND r.corpus_id=cb.corpus_id
   AND r.language=cb.language AND r.builder_version=3
 JOIN vocabulary_browse_counts c ON c.owner_id=cb.owner_id AND c.book_id=cb.book_id
   AND c.analysis_run_id=cb.analysis_run_id AND c.corpus_id=cb.corpus_id AND c.language=cb.language
 JOIN requested i ON i.lemma=c.canonical_lemma AND i.upos=c.upos AND i.discriminator=c.lexical_discriminator
 WHERE $6::text='' OR c.book_id::text<>$6
 GROUP BY c.canonical_lemma,c.upos,c.lexical_discriminator
)
SELECT i.lemma,i.upos,i.discriminator,COALESCE(c.occurrences,0)::bigint,s.ready,s.unavailable
FROM requested i CROSS JOIN projection_status s
LEFT JOIN counts c ON c.canonical_lemma=i.lemma AND c.upos=i.upos AND c.lexical_discriminator=i.discriminator`, owner, language, lemmas, upos, discriminators, excludeBook)
	if err != nil {
		return nil, fmt.Errorf("load current across-Book vocabulary counts: %w", err)
	}
	ready, unavailable := true, false
	for rows.Next() {
		var lemma, pos, discriminator string
		var count int64
		if err := rows.Scan(&lemma, &pos, &discriminator, &count, &ready, &unavailable); err != nil {
			rows.Close()
			return nil, err
		}
		acrossBooks[currentReadingVocabularyIdentity{lemma: lemma, upos: pos, discriminator: discriminator}] = count
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if !ready {
		if unavailable {
			return nil, ErrVocabularyBrowseCountsUnavailable
		}
		return nil, ErrVocabularyBrowseCountsPending
	}
	return acrossBooks, nil
}

// projectedAnalysisCurrentReadingCandidates rebuilds the current Book's
// candidate set from immutable analyzer evidence plus exact-occurrence learner
// corrections. It runs in the same transaction as the freeze, after the Book row
// is locked.
func projectedAnalysisCurrentReadingCandidates(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, language string, identity sqlcgen.GetCurrentReadingCandidateIdentityRow) ([]domain.SelectionCandidate, error) {
	analysis, corrections, err := loadAnalysisProjectionFactsTx(ctx, tx, q, domain.DeckPreparation{
		OwnerID: owner, SourceMaterialID: identity.CaSourceMaterialID, AnalysisRunID: identity.CaAnalysisRunID,
	}, identity.CaCorpusID)
	if err != nil {
		return nil, err
	}
	if len(analysis.Sentences) == 0 {
		// An analysis without tokens freezes an empty snapshot.
		return nil, nil
	}
	decisions, err := currentReadingCorrectionDecisions(corrections)
	if err != nil {
		return nil, err
	}
	structure, err := LoadSnapshotStructure(ctx, tx, owner, identity.CaSourceMaterialID, identity.CaSnapshotID)
	if err != nil {
		return nil, err
	}
	config := selection.DefaultConfig(identity.CaCorpusID)
	config.Structure = structure
	projected, err := selection.Project(analysis, config, decisions)
	if err != nil {
		return nil, err
	}
	return projectedCurrentReadingCandidates(owner, language, identity.CaCorpusID, projected, tokenHeadwords(analysis))
}

// tokenHeadwords maps each analyzed token identity to its citation headword,
// the minimum non-empty headword the analysis carries for that identity. The map
// is empty for a modern-language analysis, whose tokens carry no headword.
func tokenHeadwords(analysis analyzer.Result) map[selection.Identity]string {
	headwords := make(map[selection.Identity]string)
	for _, sentence := range analysis.Sentences {
		for _, token := range sentence.Tokens {
			if token.Headword == "" {
				continue
			}
			identity := selection.Identity{Language: analysis.Language, CanonicalLemma: token.CanonicalLemma, UPOS: token.UPOS, LexicalDiscriminator: token.LexicalDiscriminator}
			if headword, ok := headwords[identity]; !ok || token.Headword < headword {
				headwords[identity] = token.Headword
			}
		}
	}
	return headwords
}

// currentReadingCorrectionDecisions translates persisted occurrence corrections
// into the projection's occurrence decisions.
func currentReadingCorrectionDecisions(corrections []domain.OccurrenceLemmaCorrection) ([]selection.OccurrenceDecision, error) {
	decisions := make([]selection.OccurrenceDecision, 0, len(corrections))
	for _, correction := range corrections {
		start, startErr := checked.Uint64FromInt64(correction.StartOffset)
		end, endErr := checked.Uint64FromInt64(correction.EndOffset)
		if startErr != nil || endErr != nil {
			return nil, errors.New("persisted lemma correction contains invalid offsets")
		}
		decisions = append(decisions, selection.OccurrenceDecision{
			Occurrence: selection.OccurrenceIdentity{SourceDocumentID: correction.SourceDocumentID, StartOffset: start, EndOffset: end},
			Lemma:      correction.CanonicalLemma,
			Excluded:   correction.Excluded,
		})
	}
	return decisions, nil
}

// projectedCurrentReadingCandidates persists the projection as frozen selection
// candidates, encoding each fact, its first encounter, its citation headword,
// and the sentence-quality gate version that will admit it (ADR 0106 §3).
func projectedCurrentReadingCandidates(owner, language, corpusID string, projected []selection.Candidate, headwords map[selection.Identity]string) ([]domain.SelectionCandidate, error) {
	candidates := make([]domain.SelectionCandidate, 0, len(projected))
	for _, candidate := range projected {
		candidate.Provenance.GateVersion = sentencequality.GateVersion
		encoded, err := encodeProjectedCurrentReadingCandidate(candidate)
		if err != nil {
			return nil, err
		}
		first, err := firstProjectedEncounter(candidate.SentenceReferences)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, domain.SelectionCandidate{
			OwnerID: owner, CorpusID: corpusID, Language: language,
			CanonicalLemma: candidate.Identity.CanonicalLemma, UPOS: candidate.Identity.UPOS,
			LexicalDiscriminator: candidate.Identity.LexicalDiscriminator, Headword: headwords[candidate.Identity],
			OccurrenceCount: candidate.OccurrenceCount, FirstEncounter: first,
			ObservedForms: encoded.observedForms, SentenceReferences: encoded.sentenceReferences, Provenance: encoded.provenance, SelectedAt: time.Now().UTC(),
		})
	}
	return candidates, nil
}

// projectedCurrentReadingEncoding is a projection candidate's JSON facts.
type projectedCurrentReadingEncoding struct {
	observedForms      []byte
	sentenceReferences []byte
	provenance         []byte
}

func encodeProjectedCurrentReadingCandidate(candidate selection.Candidate) (projectedCurrentReadingEncoding, error) {
	var encoded projectedCurrentReadingEncoding
	var err error
	if encoded.observedForms, err = json.Marshal(candidate.ObservedForms); err != nil {
		return encoded, err
	}
	if encoded.sentenceReferences, err = json.Marshal(candidate.SentenceReferences); err != nil {
		return encoded, err
	}
	if encoded.provenance, err = json.Marshal(candidate.Provenance); err != nil {
		return encoded, err
	}
	return encoded, nil
}

// firstProjectedEncounter is the earliest offset a projected candidate occurs
// at, or the maximum int64 when it has no references.
func firstProjectedEncounter(refs []selection.SentenceReference) (int64, error) {
	first := int64(^uint64(0) >> 1)
	for _, ref := range refs {
		encounter, err := checked.Int64FromUint64(ref.Location.StartOffset)
		if err != nil {
			return 0, errors.New("projected occurrence offset exceeds persisted range")
		}
		if encounter < first {
			first = encounter
		}
	}
	return first, nil
}

func releaseCurrentReadingSnapshot(ctx context.Context, q *sqlcgen.Queries, owner, snapshotID string) error {
	if snapshotID == "" {
		return nil
	}
	if _, err := q.LockCurrentReadingSnapshot(ctx, sqlcgen.LockCurrentReadingSnapshotParams{Owner: owner, Snapshot: snapshotID}); err != nil {
		return err
	}
	return q.ReleaseCurrentReadingSnapshot(ctx, sqlcgen.ReleaseCurrentReadingSnapshotParams{Owner: owner, Snapshot: snapshotID})
}

func releaseCurrentReadingSnapshots(ctx context.Context, q *sqlcgen.Queries, owner string, snapshotIDs []string) error {
	for _, snapshotID := range snapshotIDs {
		if err := releaseCurrentReadingSnapshot(ctx, q, owner, snapshotID); err != nil {
			return err
		}
	}
	return nil
}

// ImportPreviouslyRead records one owner-and-Book-scoped completion without
// creating vocabulary evidence. The partial unique index makes concurrent
// retries converge on the same imported fact.
func (s *PostgresStore) ImportPreviouslyRead(ctx context.Context, owner, bookID string) (domain.ReadingCompletion, error) {
	row, err := s.queries().InsertPreviouslyReadImport(ctx, sqlcgen.InsertPreviouslyReadImportParams{
		Owner: owner, Book: bookID, CompletedAt: time.Now().UTC(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, getErr := s.queries().GetPreviouslyReadImport(ctx, sqlcgen.GetPreviouslyReadImportParams{Owner: owner, Book: bookID})
		if getErr != nil {
			return domain.ReadingCompletion{}, missing(getErr)
		}
		return importedReadingCompletion(existing.OwnerID, existing.Language, existing.BookID, existing.CompletedAt, existing.GoalSnapshotID, existing.SnapshotVocabularyCount, existing.EligibleVocabularyCount, existing.GraduatedVocabularyCount, existing.AlreadyKnownVocabularyCount), nil
	}
	if err != nil {
		return domain.ReadingCompletion{}, missing(err)
	}
	return importedReadingCompletion(row.OwnerID, row.Language, row.BookID, row.CompletedAt, row.GoalSnapshotID, row.SnapshotVocabularyCount, row.EligibleVocabularyCount, row.GraduatedVocabularyCount, row.AlreadyKnownVocabularyCount), nil
}

func importedReadingCompletion(owner, language, book string, completedAt time.Time, snapshotID string, snapshotCount, eligibleCount, graduatedCount, knownCount int) domain.ReadingCompletion {
	return domain.ReadingCompletion{
		OwnerID: owner, Language: language, BookID: book, CompletedAt: completedAt,
		GoalSnapshotID: snapshotID, SnapshotVocabularyCount: snapshotCount,
		EligibleVocabularyCount: eligibleCount, GraduatedVocabularyCount: graduatedCount,
		AlreadyKnownVocabularyCount: knownCount, Source: domain.ReadingCompletionPreviouslyRead,
	}
}

func getReadingCompletion(ctx context.Context, q *sqlcgen.Queries, owner, language, bookID, snapshotID string) (sqlcgen.GetReadingCompletionRow, error) {
	return q.GetReadingCompletion(ctx, sqlcgen.GetReadingCompletionParams{Owner: owner, Language: language, Book: bookID, Snapshot: snapshotID})
}

// CountCurrentReadingVocabularyToAccept reports the currently eligible frozen
// identities. It is deliberately read-only so the confirmation can state the
// exact modeled consequence before the learner accepts completion.
func (s *PostgresStore) CountCurrentReadingVocabularyToAccept(ctx context.Context, owner, language string) (int, error) {
	goal, err := s.GetCurrentReading(ctx, owner, language)
	if err != nil {
		return 0, err
	}
	if !goal.IsActive() || goal.SnapshotID == "" {
		return 0, nil
	}
	snapshot, known, err := loadCurrentReadingSnapshotFacts(ctx, s.queries(), owner, goal.SnapshotID)
	if err != nil {
		return 0, err
	}
	return domain.PlanSnapshotAcceptance(snapshot, known).EligibleCount, nil
}

// GetCurrentReading returns the owner's current reading in one study language.
// No row is the ordinary absent current-reading state.
func (s *PostgresStore) GetCurrentReading(ctx context.Context, owner, language string) (domain.CurrentReading, error) {
	language = canonicalization.NormalizeLanguage(language)
	if language == "" {
		return domain.CurrentReading{}, nil
	}
	row, err := s.queries().GetCurrentReading(ctx, sqlcgen.GetCurrentReadingParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CurrentReading{}, nil
	}
	if err != nil {
		return domain.CurrentReading{}, err
	}
	snapshotSize, err := snapshotSizeForGoal(ctx, s.queries(), row.SnapshotID, row.GOwnerID)
	if err != nil {
		return domain.CurrentReading{}, err
	}
	return currentReadingFromRow(row, snapshotSize), nil
}

// ListCurrentReadingSnapshotVocabulary reads one immutable snapshot without
// changing any learner state.
func (s *PostgresStore) ListCurrentReadingSnapshotVocabulary(ctx context.Context, owner, snapshotID string) ([]domain.SelectionCandidate, error) {
	if snapshotID == "" {
		return nil, nil
	}
	return listCurrentReadingSnapshotVocabulary(ctx, s.queries(), owner, snapshotID)
}

func snapshotSizeForGoal(ctx context.Context, q *sqlcgen.Queries, snapshotID, owner string) (int, error) {
	if snapshotID == "" {
		return 0, nil
	}
	rows, err := q.ListCurrentReadingSnapshotVocabulary(ctx, sqlcgen.ListCurrentReadingSnapshotVocabularyParams{Owner: owner, Snapshot: snapshotID})
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

// StartResult is domain.StartResult.
type StartResult = domain.StartResult

// StartCurrentReading creates the owner's current reading for an eligible To Read
// Book in language.
func (s *PostgresStore) StartCurrentReading(ctx context.Context, owner, language, bookID string) (domain.CurrentReading, error) {
	start, err := s.StartCurrentReadingResult(ctx, owner, language, bookID)
	return start.Reading, err
}

// StartCurrentReadingResult is StartCurrentReading that also reports whether the
// Start replayed the current reading.
func (s *PostgresStore) StartCurrentReadingResult(ctx context.Context, owner, language, bookID string) (StartResult, error) {
	return s.startCurrentReading(ctx, owner, language, bookID, nil)
}

// StartCurrentReadingWith creates the current reading and runs beforeCommit in
// the same transaction after its immutable snapshot has been populated. The
// callback can atomically attach durable work that depends on that snapshot.
// A replayed Start writes nothing, so beforeCommit does not run for it.
func (s *PostgresStore) StartCurrentReadingWith(ctx context.Context, owner, language, bookID string, beforeCommit func(context.Context, pgx.Tx, domain.CurrentReading) error) (domain.CurrentReading, error) {
	start, err := s.startCurrentReading(ctx, owner, language, bookID, beforeCommit)
	return start.Reading, err
}

func (s *PostgresStore) startCurrentReading(ctx context.Context, owner, language, bookID string, beforeCommit func(context.Context, pgx.Tx, domain.CurrentReading) error) (start StartResult, err error) {
	language = canonicalization.NormalizeLanguage(language)
	if err := validateCurrentReadingStart(owner, language, bookID); err != nil {
		return StartResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return StartResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	if err := lockLemmaReviewLearnerState(ctx, tx, owner); err != nil {
		return StartResult{}, err
	}
	if err := lockBookForUpdate(ctx, q, owner, bookID); err != nil {
		return StartResult{}, err
	}
	facts, identity, err := loadStartFacts(ctx, tx, q, owner, language, bookID)
	if err != nil {
		return StartResult{}, err
	}
	decision := domain.DecideStart(facts)
	if err := domain.StartDecisionError(decision); err != nil {
		return StartResult{}, err
	}
	if decision.Outcome == domain.StartReplayed {
		return replayCurrentReadingStart(ctx, tx, q, owner, language)
	}
	return createStartedCurrentReading(ctx, tx, q, owner, language, bookID, identity, beforeCommit)
}

// validateCurrentReadingStart rejects a Start whose reading identity is
// incomplete and treats a Book identity Postgres cannot read as a uuid as the
// same not-found a well-formed unknown Book gets.
func validateCurrentReadingStart(owner, language, bookID string) error {
	if err := (domain.CurrentReading{OwnerID: owner, Language: language, BookID: bookID}).Validate(); err != nil {
		return err
	}
	if !validUUID(owner) || !validUUID(bookID) {
		return ErrNotFound
	}
	return nil
}

// replayCurrentReadingStart returns the existing reading a replayed Start found
// and commits without writing.
func replayCurrentReadingStart(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, language string) (StartResult, error) {
	current, err := readCurrentReadingLocked(ctx, q, owner, language)
	if err != nil {
		return StartResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StartResult{}, err
	}
	return StartResult{Reading: current, Replayed: true}, nil
}

// createStartedCurrentReading freezes the snapshot, inserts the current reading,
// runs the optional beforeCommit hook, and commits.
func createStartedCurrentReading(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, language, bookID string, identity sqlcgen.GetCurrentReadingCandidateIdentityRow, beforeCommit func(context.Context, pgx.Tx, domain.CurrentReading) error) (StartResult, error) {
	snapshot, candidates, err := createCurrentReadingSnapshot(ctx, tx, q, owner, language, bookID, identity)
	if err != nil {
		return StartResult{}, err
	}
	goal, err := insertCurrentReading(ctx, q, owner, language, bookID, snapshot.ID)
	if err != nil {
		return StartResult{}, err
	}
	goal.SourceMaterialID = identity.CaSourceMaterialID
	goal.AnalysisRunID = identity.CaAnalysisRunID
	goal.ContentRevisionID = identity.CaContentRevisionID
	goal.ContentSnapshotID = identity.CaSnapshotID
	goal.CorpusID = identity.CaCorpusID
	goal.SnapshotSize = len(candidates)
	if beforeCommit != nil {
		if err := beforeCommit(ctx, tx, goal); err != nil {
			return StartResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return StartResult{}, err
	}
	return StartResult{Reading: goal}, nil
}

// readCurrentReadingLocked reads the owner's current reading in language inside
// a transaction whose learner-state lock is already held.
func readCurrentReadingLocked(ctx context.Context, q *sqlcgen.Queries, owner, language string) (domain.CurrentReading, error) {
	row, err := q.GetCurrentReading(ctx, sqlcgen.GetCurrentReadingParams{Owner: owner, Language: language})
	if err != nil {
		return domain.CurrentReading{}, err
	}
	snapshotSize, err := snapshotSizeForGoal(ctx, q, row.SnapshotID, row.GOwnerID)
	if err != nil {
		return domain.CurrentReading{}, err
	}
	return currentReadingFromRow(row, snapshotSize), nil
}

// CurrentReadingIneligibleError is domain.CurrentReadingIneligibleError.
type CurrentReadingIneligibleError = domain.CurrentReadingIneligibleError

// loadStartFacts reads the facts domain.DecideStart decides over, in the order
// the Start rules consult them. The caller holds the learner-state and Book
// locks, so the facts cannot change before the transition commits. The identity
// is only meaningful when the decision accepts.
func loadStartFacts(ctx context.Context, tx pgx.Tx, q *sqlcgen.Queries, owner, language, bookID string) (domain.StartFacts, sqlcgen.GetCurrentReadingCandidateIdentityRow, error) {
	facts := domain.StartFacts{Language: language}
	var identity sqlcgen.GetCurrentReadingCandidateIdentityRow
	current, err := q.GetCurrentReadingForUpdate(ctx, sqlcgen.GetCurrentReadingForUpdateParams{Owner: owner, Language: language})
	switch {
	case err == nil:
		facts.AlreadyCurrent = true
		facts.CurrentIsBook = strings.EqualFold(current.GBookID, bookID)
	case !errors.Is(err, pgx.ErrNoRows):
		return facts, identity, err
	}
	if err := loadStartEvidence(ctx, q, owner, bookID, &facts); err != nil {
		return facts, identity, err
	}
	identity, err = q.GetCurrentReadingCandidateIdentity(ctx, sqlcgen.GetCurrentReadingCandidateIdentityParams{Owner: owner, Book: bookID})
	switch {
	case err == nil:
		facts.IdentityPublished = true
	case !errors.Is(err, pgx.ErrNoRows):
		return facts, identity, err
	}
	if facts.IdentityPublished {
		facts.UnresolvedLemmaReviewFlags, err = unresolvedLemmaReviewFlags(ctx, tx, owner, bookID, identity.CaAnalysisRunID)
		if err != nil {
			return facts, identity, err
		}
	}
	return facts, identity, nil
}

// loadStartEvidence fills the classifier's inputs from the Book's evidence row.
func loadStartEvidence(ctx context.Context, q *sqlcgen.Queries, owner, bookID string, facts *domain.StartFacts) error {
	evidence, err := q.GetCurrentReadingEvidence(ctx, sqlcgen.GetCurrentReadingEvidenceParams{Owner: owner, Book: bookID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// A Book without an evidence row has no acquired content, no analysis, and
	// no disposition, so the zero row reads as exactly that.
	facts.Signals = analysisSignalsFromView(evidence.SourceID, evidence.SourceMediaType, evidence.SourceContentRevisionID, evidence.SourceContentSnapshotID, evidence.AnalysisStatus, evidence.AnalysisState, evidence.AnalysisRunID)
	facts.Disposition = domain.BookDisposition(evidence.Disposition)
	facts.BookLanguage = ""
	if evidence.BookLanguageState == domain.LanguageChosen {
		facts.BookLanguage = evidence.BookLanguageTag
	}
	return nil
}

// currentReadingEligibility asks the domain classifier whether the Book may
// become the owner's current reading in language. The caller holds the
// learner-state and Book locks, so the evidence, disposition, and language it
// reads cannot change before the transition commits.
func currentReadingEligibility(ctx context.Context, q *sqlcgen.Queries, owner, language, bookID string) (domain.CurrentReadingEligibilityReason, error) {
	var facts domain.StartFacts
	if err := loadStartEvidence(ctx, q, owner, bookID, &facts); err != nil {
		return "", err
	}
	classification := domain.ClassifyBookEvidence(facts.Signals, facts.Disposition, facts.BookLanguage)
	return domain.CurrentReadingEligibilityIn(classification, facts.BookLanguage, language), nil
}

// lockBookForUpdate locks the owner's Book row for update and reports a missing
// Book as ErrNotFound. Callers use it to fence the Book before they lock the
// Current reading or preparation rows that hang off it.
func lockBookForUpdate(ctx context.Context, q *sqlcgen.Queries, owner, bookID string) error {
	_, err := q.GetBookForUpdate(ctx, sqlcgen.GetBookForUpdateParams{Owner: owner, ID: bookID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func insertCurrentReading(ctx context.Context, q *sqlcgen.Queries, owner, language, bookID, snapshotID string) (domain.CurrentReading, error) {
	row, err := q.InsertCurrentReading(ctx, sqlcgen.InsertCurrentReadingParams{Owner: owner, Language: language, Book: bookID, Snapshot: uuidArg(snapshotID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CurrentReading{}, ErrCurrentReadingExists
	}
	if err != nil {
		return domain.CurrentReading{}, err
	}
	return currentReadingFromValues(row.OwnerID, row.Language, row.BookID, row.SnapshotID, "", "", "", "", "", 0, row.CreatedAt, row.UpdatedAt), nil
}

// loadSnapshotLifecycle reads the durable lifecycle facts of one snapshot into
// *into, leaving it nil when the snapshot is unnamed or not recorded for the
// owner and language.
func loadSnapshotLifecycle(ctx context.Context, q *sqlcgen.Queries, owner, language, snapshotID string, into **domain.CurrentReadingSnapshotLifecycle) error {
	*into = nil
	if snapshotID == "" {
		return nil
	}
	row, err := q.GetCurrentReadingSnapshotLifecycle(ctx, sqlcgen.GetCurrentReadingSnapshotLifecycleParams{Owner: owner, Language: language, Snapshot: snapshotID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	lifecycle := &domain.CurrentReadingSnapshotLifecycle{BookID: row.BookID, CreatedAt: row.CreatedAt, Completed: row.Completed}
	if row.ReleasedAt.Valid {
		lifecycle.ReleasedAt = row.ReleasedAt.Time
	}
	*into = lifecycle
	return nil
}

// loadCurrentReadingForUpdate locks and reads the owner's current reading in
// language; the zero value means none is current.
func loadCurrentReadingForUpdate(ctx context.Context, q *sqlcgen.Queries, owner, language string) (domain.CurrentReading, error) {
	current, err := q.GetCurrentReadingForUpdate(ctx, sqlcgen.GetCurrentReadingForUpdateParams{Owner: owner, Language: language})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CurrentReading{}, nil
	}
	if err != nil {
		return domain.CurrentReading{}, err
	}
	return currentReadingFromValues(current.GOwnerID, current.Language, current.GBookID, current.SnapshotID, current.SourceMaterialID, current.AnalysisRunID, current.ContentRevisionID, current.ContentSnapshotID, current.CorpusID, 0, current.CreatedAt, current.UpdatedAt), nil
}
