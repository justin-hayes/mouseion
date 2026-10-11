package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
)

// ReadLemmaReviewProposal previews a proposal in one repeatable-read transaction.
// For every identity the proposal moves, it reads the effective count in this
// Book now and after the proposal, the count across the learner's other
// currently analyzed Books, and Known and Reserved state. The fingerprint binds a
// later confirm to exactly that state. It reports the pending or unavailable
// errors of the snapshot freeze while the Book or any other analyzed Book lacks a
// ready projection. Previewing writes nothing.
func (s *PostgresStore) ReadLemmaReviewProposal(ctx context.Context, proposal domain.LemmaReviewProposal) (preview domain.LemmaReviewPreview, err error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return domain.LemmaReviewPreview{}, fmt.Errorf("begin lemma review preview: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); err == nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = rollbackErr
		}
	}()
	return readLemmaReviewProposalTx(ctx, tx, proposal)
}

func readLemmaReviewProposalTx(ctx context.Context, tx pgx.Tx, proposal domain.LemmaReviewProposal) (domain.LemmaReviewPreview, error) {
	occurrences, err := listLemmaReviewOccurrences(ctx, sqlcgen.New(tx), proposal.OwnerID, proposal.BookID, proposal.Surface)
	if err != nil {
		return domain.LemmaReviewPreview{}, err
	}
	scope, err := browseCountReadinessForDecision(ctx, tx, proposal.OwnerID, proposal.BookID)
	if err != nil {
		return domain.LemmaReviewPreview{}, err
	}
	if !scope.ready {
		return domain.LemmaReviewPreview{}, ErrVocabularyBrowseCountsPending
	}
	states, err := lemmaReviewIdentityStates(ctx, tx, scope, proposal, selection.ProposalIdentities(proposal))
	if err != nil {
		return domain.LemmaReviewPreview{}, err
	}
	return domain.LemmaReviewPreview{States: states, Fingerprint: domain.LemmaReviewFingerprint(proposal, occurrences, states)}, nil
}

// lemmaReviewIdentityStates reads the state of only the affected identities, so
// its work is proportional to them and not to the Book.
func lemmaReviewIdentityStates(ctx context.Context, tx pgx.Tx, scope browseCountReadiness, proposal domain.LemmaReviewProposal, affected []domain.LemmaReviewIdentity) ([]domain.LemmaReviewIdentityState, error) {
	if len(affected) == 0 {
		return nil, nil
	}
	lemmaSet := make(map[string]struct{}, len(affected))
	identities := make([]currentReadingVocabularyIdentity, 0, len(affected))
	for _, identity := range affected {
		normalized := normalizedBrowseCountIdentity(identity.CanonicalLemma, identity.UPOS)
		lemmaSet[normalized.lemma] = struct{}{}
		identities = append(identities, currentReadingVocabularyIdentity{lemma: normalized.lemma, upos: normalized.upos})
	}
	lemmas := make([]string, 0, len(lemmaSet))
	for lemma := range lemmaSet {
		lemmas = append(lemmas, lemma)
	}
	before, err := projectBrowseCounts(ctx, tx, scope, lemmas, nil)
	if err != nil {
		return nil, err
	}
	after, err := projectBrowseCounts(ctx, tx, scope, lemmas, proposal.Decisions())
	if err != nil {
		return nil, err
	}
	others, err := currentAcrossBookCounts(ctx, tx, proposal.OwnerID, scope.language, proposal.BookID, identities)
	if err != nil {
		return nil, err
	}
	known, err := knownLemmaReviewIdentities(ctx, tx, proposal.OwnerID, proposal.Language, lemmas)
	if err != nil {
		return nil, err
	}
	q := sqlcgen.New(tx)
	states := make([]domain.LemmaReviewIdentityState, 0, len(affected))
	for _, identity := range affected {
		normalized := normalizedBrowseCountIdentity(identity.CanonicalLemma, identity.UPOS)
		reserved, err := q.ReservedVocabularyExists(ctx, sqlcgen.ReservedVocabularyExistsParams{
			OwnerID: proposal.OwnerID, Language: identity.Language, CanonicalLemma: normalized.lemma, Upos: normalized.upos,
		})
		if err != nil {
			return nil, err
		}
		states = append(states, domain.LemmaReviewIdentityState{
			Identity: identity, InBook: before[normalized].occurrences, AfterInBook: after[normalized].occurrences,
			OtherBooks: others[currentReadingVocabularyIdentity{lemma: normalized.lemma, upos: normalized.upos}],
			Known:      knownLemmaReviewContains(known, normalized), Reserved: reserved,
		})
	}
	return states, nil
}

// knownLemmaReviewIdentities returns the Known identities of the given lemmas.
func knownLemmaReviewIdentities(ctx context.Context, tx pgx.Tx, owner, language string, lemmas []string) (map[browseCountIdentity]bool, error) {
	rows, err := tx.Query(ctx, `SELECT canonical_lemma,upos FROM known_vocabulary WHERE owner_id=$1 AND language=$2 AND canonical_lemma=ANY($3)`,
		owner, canonicalization.NormalizeLanguage(language), lemmas)
	if err != nil {
		return nil, fmt.Errorf("read Known state for lemma review preview: %w", err)
	}
	defer rows.Close()
	known := make(map[browseCountIdentity]bool)
	for rows.Next() {
		var lemma, upos string
		if err := rows.Scan(&lemma, &upos); err != nil {
			return nil, err
		}
		known[browseCountIdentity{lemma: lemma, upos: upos}] = true
	}
	return known, rows.Err()
}

// knownLemmaReviewContains applies selection's Known rule, where an empty
// stored UPOS is the lemma wildcard.
func knownLemmaReviewContains(known map[browseCountIdentity]bool, identity browseCountIdentity) bool {
	return known[identity] || known[browseCountIdentity{lemma: identity.lemma}]
}
