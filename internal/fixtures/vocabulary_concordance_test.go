package fixtures

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureConcordanceServesHausAndMultiBookOccurrences(t *testing.T) {
	store := NewStore()

	haus, err := store.ListVocabularyConcordance(t.Context(), OwnerID, "de", domain.ConcordanceLookup{Language: "de", Term: "Haus"})
	require.NoError(t, err)
	require.NotEmpty(t, haus.Occurrences)
	for _, occurrence := range haus.Occurrences {
		assert.Equal(t, "haus", occurrence.CanonicalLemma)
	}

	books, err := store.ListVocabularyConcordance(t.Context(), OwnerID, "de", domain.ConcordanceLookup{Language: "de", Term: "fixture-books"})
	require.NoError(t, err)
	assert.NotEmpty(t, books.Occurrences)
}

// TestFixtureSentenceStudyServesStoredMorphology exercises the fixtureserver's
// Sentence study read path so the browser smoke fixtures carry morphology into
// the text alternative, in source order and with empty features preserved.
func TestFixtureSentenceStudyServesStoredMorphology(t *testing.T) {
	store := NewStore()

	study, err := store.GetVocabularySentenceStudy(t.Context(), OwnerID, BookID, ResultRunID, "fixture-corpus", "fixture-concordance-unit", 2, 1, "Haus")
	require.NoError(t, err)
	require.Len(t, study.Tokens, 6)
	assert.Equal(t, map[string]string{"Case": "Nom", "Gender": "Neut", "Number": "Sing"}, study.Tokens[0].Morphology)
	assert.Equal(t, "Haus", study.Tokens[1].Surface)
	assert.True(t, study.Tokens[1].Corrected)
	assert.Equal(t, map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "3", "Tense": "Pres", "VerbForm": "Fin"}, study.Tokens[2].Morphology)
	assert.Empty(t, study.Tokens[3].Morphology, "a word with no supplied features keeps empty morphology")
	assert.Equal(t, map[string]string{"PartType": "Sep"}, study.Tokens[4].Morphology)
}

// TestFixtureConcordanceReportsStaleErrorsPriorityAndEmptyPages holds the
// Concordance edge responses: a stale revision is flagged without occurrences,
// the canned server-error and timeout terms surface the same errors the
// PostgreSQL adapter returns, a captured priority Book always matches the
// single-Book fixture corpus, and a page past the matches is empty.
func TestFixtureConcordanceReportsStaleErrorsPriorityAndEmptyPages(t *testing.T) {
	store := NewStore()
	ctx := t.Context()

	t.Run("a stale revision is flagged without occurrences", func(t *testing.T) {
		stale, err := store.ListVocabularyConcordance(ctx, OwnerID, "de", domain.ConcordanceLookup{Language: "de", Term: "Haus", Revision: "fixture-stale-revision"})
		require.NoError(t, err)
		assert.True(t, stale.Stale)
		assert.Empty(t, stale.Occurrences)
	})

	t.Run("the canned server error reports an error", func(t *testing.T) {
		_, err := store.ListVocabularyConcordance(ctx, OwnerID, "de", domain.ConcordanceLookup{Language: "de", Term: "fixture-server-error"})
		require.Error(t, err)
	})

	t.Run("the canned timeout reports a deadline", func(t *testing.T) {
		_, err := store.ListVocabularyConcordance(ctx, OwnerID, "de", domain.ConcordanceLookup{Language: "de", Term: "fixture-server-timeout"})
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("a captured priority Book always matches", func(t *testing.T) {
		priority, err := store.ListVocabularyConcordance(ctx, OwnerID, "de", domain.ConcordanceLookup{Language: "de", Term: "Haus", Priority: BookID, Page: 1})
		require.NoError(t, err)
		assert.Equal(t, domain.ConcordancePriorityMatches, priority.PriorityState)
		assert.Equal(t, BookID, priority.PriorityBookID)
	})

	t.Run("a page past the matches is empty", func(t *testing.T) {
		beyond, err := store.ListVocabularyConcordance(ctx, OwnerID, "de", domain.ConcordanceLookup{Language: "de", Term: "Haus", Page: 100})
		require.NoError(t, err)
		assert.Empty(t, beyond.Occurrences)
		assert.False(t, beyond.HasNext)
	})
}

// TestFixtureConcordanceAppliesSuppliedAndFormInterpretations holds how a
// Concordance lookup interprets its term: a supplied interpretation is honored,
// a term that is not an evidenced fixture lemma falls back to a source-form
// match, and an unrecognized interpretation matches nothing.
func TestFixtureConcordanceAppliesSuppliedAndFormInterpretations(t *testing.T) {
	store := NewStore()
	ctx := t.Context()

	t.Run("a supplied form interpretation matches the source form", func(t *testing.T) {
		supplied, err := store.ListVocabularyConcordance(ctx, OwnerID, "de", domain.ConcordanceLookup{Language: "de", Term: "Haus", Match: domain.ConcordanceMatchForm, UPOS: "NOUN", Page: 1})
		require.NoError(t, err)
		assert.Equal(t, domain.ConcordanceMatchForm, supplied.Match)
		require.NotEmpty(t, supplied.Occurrences)
		for _, occurrence := range supplied.Occurrences {
			assert.Equal(t, "NOUN", occurrence.UPOS)
		}
	})

	t.Run("a term that is not an evidenced lemma falls back to form", func(t *testing.T) {
		form, err := store.ListVocabularyConcordance(ctx, OwnerID, "de", domain.ConcordanceLookup{Language: "de", Term: "sehen", Page: 1})
		require.NoError(t, err)
		assert.Equal(t, domain.ConcordanceMatchForm, form.Match)
		assert.Empty(t, form.Occurrences)
	})

	t.Run("an unrecognized interpretation matches nothing", func(t *testing.T) {
		unrecognized, err := store.ListVocabularyConcordance(ctx, OwnerID, "de", domain.ConcordanceLookup{Language: "de", Term: "Haus", Match: "unrecognized", Page: 1})
		require.NoError(t, err)
		assert.Empty(t, unrecognized.Occurrences)
	})
}
