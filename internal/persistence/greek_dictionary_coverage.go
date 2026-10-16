package persistence

import (
	"context"
	"fmt"
)

// AncientGreekVocabularyIdentity is one effective Ancient Greek vocabulary
// identity evidenced by the installation's analyses. The lexical discriminator
// keeps homographs apart; the dictionary hit-rate report never uses it to
// match, because the dictionary is keyed by lemma and part of speech.
type AncientGreekVocabularyIdentity struct {
	CanonicalLemma string
	UPOS           string
	Discriminator  string
}

// ListAncientGreekVocabularyIdentities returns the distinct effective Ancient
// Greek vocabulary identities across the installation's analyses. It reads the
// Browse count projection, which holds each Book's current published analysis
// (a newer publication replaces the previous run's rows), so confirmed lemma
// corrections and exclusions are already applied and superseded runs are not
// counted.
func (s *PostgresStore) ListAncientGreekVocabularyIdentities(ctx context.Context) ([]AncientGreekVocabularyIdentity, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT canonical_lemma, upos, lexical_discriminator
		FROM vocabulary_browse_counts
		WHERE language = 'grc'
		GROUP BY canonical_lemma, upos, lexical_discriminator
		ORDER BY canonical_lemma, upos, lexical_discriminator`)
	if err != nil {
		return nil, fmt.Errorf("list ancient greek vocabulary identities: %w", err)
	}
	defer rows.Close()
	identities := make([]AncientGreekVocabularyIdentity, 0)
	for rows.Next() {
		var identity AncientGreekVocabularyIdentity
		if err := rows.Scan(&identity.CanonicalLemma, &identity.UPOS, &identity.Discriminator); err != nil {
			return nil, fmt.Errorf("scan ancient greek vocabulary identity: %w", err)
		}
		identities = append(identities, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ancient greek vocabulary identities: %w", err)
	}
	return identities, nil
}
