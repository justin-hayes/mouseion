//go:build integration

package prepareddeck

import (
	"encoding/json"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/justin-hayes/mouseion/internal/storecontract"
	"github.com/stretchr/testify/require"
)

// TestStoreContractSeedsPassTheSharedGate proves the PostgreSQL store-contract
// harness seeds every vocabulary identity through a sentence the shared gate
// accepts, and seeds declared-unusable identities as fragments it rejects. The
// freeze applies this same gate.
func TestStoreContractSeedsPassTheSharedGate(t *testing.T) {
	h, ok := newPostgresHarness(t).(*postgresHarness)
	require.True(t, ok)

	usable := domain.SnapshotIdentity{Language: storecontract.Language, CanonicalLemma: "haus", UPOS: "NOUN"}
	unusable := domain.SnapshotIdentity{Language: storecontract.Language, CanonicalLemma: "garten", UPOS: "NOUN"}
	bookID := h.SeedBook(t, h.Owner(), storecontract.BookSeed{
		Vocabulary: []domain.SnapshotIdentity{usable, unusable},
		Unusable:   []domain.SnapshotIdentity{unusable},
		ToRead:     true,
	})

	greekUsable := domain.SnapshotIdentity{Language: storecontract.GreekLanguage, CanonicalLemma: "λέγω", UPOS: "VERB"}
	greekUnusable := domain.SnapshotIdentity{Language: storecontract.GreekLanguage, CanonicalLemma: "λόγος", UPOS: "NOUN"}
	greekBook := h.SeedBook(t, h.Owner(), storecontract.BookSeed{
		Language: storecontract.GreekLanguage, Imported: true,
		Vocabulary: []domain.SnapshotIdentity{greekUsable, greekUnusable},
		Unusable:   []domain.SnapshotIdentity{greekUnusable},
		ToRead:     true,
	})

	t.Run("a default modern seed passes", func(t *testing.T) {
		require.True(t, seedPassesGate(t, h, h.Owner(), bookID, usable))
	})
	t.Run("a declared-unusable modern seed fails", func(t *testing.T) {
		require.False(t, seedPassesGate(t, h, h.Owner(), bookID, unusable))
	})
	t.Run("a default imported Greek seed passes", func(t *testing.T) {
		require.True(t, seedPassesGate(t, h, h.Owner(), greekBook, greekUsable))
	})
	t.Run("a declared-unusable imported Greek seed fails", func(t *testing.T) {
		require.False(t, seedPassesGate(t, h, h.Owner(), greekBook, greekUnusable))
	})
}

// seedPassesGate resolves one seeded identity's persisted sentences and reports
// whether the shared scorer's "any reference passes" admits the identity.
func seedPassesGate(t *testing.T, h *postgresHarness, owner, bookID string, identity domain.SnapshotIdentity) bool {
	t.Helper()
	var corpusID string
	require.NoError(t, h.store.Pool().QueryRow(h.ctx, `SELECT corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner, bookID).Scan(&corpusID))

	rows, err := h.store.Pool().Query(h.ctx, `SELECT DISTINCT sentence_ordinal FROM corpus_tokens WHERE owner_id=$1 AND corpus_id=$2 AND canonical_lemma=$3 AND upos=$4 ORDER BY sentence_ordinal`, owner, corpusID, identity.CanonicalLemma, identity.UPOS)
	require.NoError(t, err)
	t.Cleanup(rows.Close)
	ordinals := make([]int64, 0, 5)
	for rows.Next() {
		var ordinal int64
		require.NoError(t, rows.Scan(&ordinal))
		ordinals = append(ordinals, ordinal)
	}
	require.NoError(t, rows.Err())
	require.NotEmpty(t, ordinals, "the seed contributes at least one sentence for the identity")

	sentences, err := h.store.ListCorpusSentences(h.ctx, owner, corpusID, ordinals)
	require.NoError(t, err)
	references := make([]selection.SentenceReference, 0, len(ordinals))
	for _, ordinal := range ordinals {
		sentence, ok := sentences[ordinal]
		require.True(t, ok, "the persisted corpus holds sentence %d", ordinal)
		references = append(references, selection.SentenceReference{SentenceIndex: int(ordinal), Text: sentence.Text, Location: sentence.Location})
	}
	referencesJSON, err := json.Marshal(references)
	require.NoError(t, err)
	formsJSON, err := json.Marshal([]string{identity.CanonicalLemma})
	require.NoError(t, err)
	candidate := domain.SelectionCandidate{
		Language: identity.Language, CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS,
		ObservedForms: formsJSON, SentenceReferences: referencesJSON,
	}
	return sentencequality.AnyReferencePasses(candidate, sentences)
}
