//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
)

// TestVocabularyBrowseShowsTheLowercaseSpellingWhenLatinLowercasingMergesTwins
// pins the display choice of ADR 0132: when lowercasing merges a capitalised and
// a lowercase source spelling of one lemma into one identity, Browse shows the
// lowercase one, within a Book and across Books, while Greek keeps its
// byte-order minimum.
func TestVocabularyBrowseShowsTheLowercaseSpellingWhenLatinLowercasingMergesTwins(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "browse-latin-headword-owner", false)
	require.NoError(t, err)
	twins := `
		SELECT id, lemma, discriminator, headword, upos, occurrences FROM (VALUES
			(0, 'roma', '', 'Roma', 'NOUN', 2),
			(1, 'roma', '', 'roma', 'NOUN', 0),
			(2, 'urbs', '', 'Urbs', 'NOUN', 1)
		) AS i(id, lemma, discriminator, headword, upos, occurrences)`
	seedBrowseBook(t, ctx, store, owner.ID, "la", "De Roma 1", twins)
	seedBrowseBook(t, ctx, store, owner.ID, "la", "De Roma 2", `
		SELECT id, lemma, discriminator, headword, upos, occurrences FROM (VALUES
			(0, 'roma', '', 'Roma', 'NOUN', 3)
		) AS i(id, lemma, discriminator, headword, upos, occurrences)`)
	seedBrowseBook(t, ctx, store, owner.ID, "grc", "Greek twins", `
		SELECT id, lemma, discriminator, headword, upos, occurrences FROM (VALUES
			(0, 'λογοσ', '', 'Λόγος', 'NOUN', 2),
			(1, 'λογοσ', '', 'λόγος', 'NOUN', 0)
		) AS i(id, lemma, discriminator, headword, upos, occurrences)`)

	headwords := func(language string) map[string]string {
		page, err := store.ListVocabularyBrowsePage(ctx, owner.ID, language, domain.VocabularyBrowseQuery{Page: 1})
		require.NoError(t, err)
		out := map[string]string{}
		for _, row := range page.Rows {
			out[row.CanonicalLemma] = row.Headword
		}
		return out
	}
	require.Equal(t, map[string]string{"roma": "roma", "urbs": "Urbs"}, headwords("la"),
		"the lowercase twin wins across both Books; a lone capitalised spelling is shown as given")
	require.Equal(t, map[string]string{"λογοσ": "Λόγος"}, headwords("grc"), "Greek is unchanged")
}
