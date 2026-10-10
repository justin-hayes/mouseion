package persistence

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// PreviewLemmaDecisionCounts projects, without writing, the effective counts of
// every identity a proposed decision set touches. Before and After come from the
// same subset projection the decision recompute uses; OtherBooks is the
// identity's projected count in the learner's other currently analyzed Books.
// It reports the pending or unavailable errors of the snapshot freeze while the
// Book or any other analyzed Book lacks a ready projection.
func (s *PostgresStore) PreviewLemmaDecisionCounts(ctx context.Context, owner, bookID string, decisions []domain.LemmaReviewDecision) (counts []domain.LemmaDecisionCounts, err error) {
	if len(decisions) == 0 {
		return nil, nil
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin lemma decision preview: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); err == nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = rollbackErr
		}
	}()
	scope, err := browseCountReadinessForDecision(ctx, tx, owner, bookID)
	if err != nil {
		return nil, err
	}
	if !scope.ready {
		return nil, ErrVocabularyBrowseCountsPending
	}
	affected := make(map[browseCountIdentity]struct{}, len(decisions)*2)
	lemmaSet := make(map[string]struct{}, len(decisions)*2)
	touch := func(lemma, upos string) {
		identity := normalizedBrowseCountIdentity(lemma, upos)
		if identity.lemma == "" {
			return
		}
		affected[identity] = struct{}{}
		lemmaSet[identity.lemma] = struct{}{}
	}
	for _, decision := range decisions {
		o := decision.Occurrence
		current := o.CorrectedLemma
		if current == "" {
			current = o.CanonicalLemma
		}
		touch(current, o.UPOS)
		touch(decision.CanonicalLemma, o.UPOS)
	}
	lemmas := make([]string, 0, len(lemmaSet))
	for lemma := range lemmaSet {
		lemmas = append(lemmas, lemma)
	}
	before, err := projectBrowseCounts(ctx, tx, scope, lemmas, nil)
	if err != nil {
		return nil, err
	}
	after, err := projectBrowseCounts(ctx, tx, scope, lemmas, decisions)
	if err != nil {
		return nil, err
	}
	identities := make([]currentReadingVocabularyIdentity, 0, len(affected))
	for identity := range affected {
		identities = append(identities, currentReadingVocabularyIdentity{lemma: identity.lemma, upos: identity.upos})
	}
	others, err := currentAcrossBookCounts(ctx, tx, owner, scope.language, bookID, identities)
	if err != nil {
		return nil, err
	}
	counts = make([]domain.LemmaDecisionCounts, 0, len(affected))
	for identity := range affected {
		counts = append(counts, domain.LemmaDecisionCounts{
			Language: scope.language, CanonicalLemma: identity.lemma, UPOS: identity.upos,
			Before: before[identity].occurrences, After: after[identity].occurrences,
			OtherBooks: others[currentReadingVocabularyIdentity{lemma: identity.lemma, upos: identity.upos}],
		})
	}
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].CanonicalLemma != counts[j].CanonicalLemma {
			return counts[i].CanonicalLemma < counts[j].CanonicalLemma
		}
		return counts[i].UPOS < counts[j].UPOS
	})
	return counts, nil
}
