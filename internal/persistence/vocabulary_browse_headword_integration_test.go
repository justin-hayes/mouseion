//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// browseHeadwordRow is the part of a Browse row the citation headword and the
// bare-lemma note decide, beside the counts that must not change with them.
type browseHeadwordRow struct {
	headword                        string
	bare                            bool
	occurrences, acrossBooks, books int64
}

// TestVocabularyBrowseCarriesCitationHeadwordsAndBareLemmaNotesPerScope pins
// the Ancient Greek Browse output across two Books. A headword comes from the
// in-scope Books' analysis tokens; a bare lemma is labelled when an in-scope
// Book also holds a numbered form with the same part of speech; and a token
// with a headword but no count adds no row.
func TestVocabularyBrowseCarriesCitationHeadwordsAndBareLemmaNotesPerScope(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "browse-headword-owner", false)
	require.NoError(t, err)
	first := seedBrowseBook(t, ctx, store, owner.ID, "grc", "Anabasis 1", `
		SELECT id, lemma, discriminator, headword, upos, occurrences FROM (VALUES
			(0, 'δεω', 'morpheus:1', 'δέω', 'VERB', 2),
			(1, 'δεω', 'morpheus:2', 'δέω', 'VERB', 1),
			(2, 'λογοσ', '', 'λόγος', 'NOUN', 3),
			(3, 'λογοσ', 'morpheus:1', 'λόγος', 'NOUN', 1),
			(4, 'λεγω', '', '', 'VERB', 4),
			(5, 'ο', '', 'ὁ', 'DET', 0)
		) AS i(id, lemma, discriminator, headword, upos, occurrences)`)
	second := seedBrowseBook(t, ctx, store, owner.ID, "grc", "Anabasis 2", `
		SELECT id, lemma, discriminator, headword, upos, occurrences FROM (VALUES
			(0, 'λογοσ', '', 'λόγος', 'NOUN', 5),
			(1, 'λεγω', '', 'λέγω', 'VERB', 6),
			(2, 'φιλοσ', '', 'φίλος', 'ADJ', 1),
			(3, 'φιλοσ', 'morpheus:1', 'φίλος', 'NOUN', 1)
		) AS i(id, lemma, discriminator, headword, upos, occurrences)`)

	browse := func(query domain.VocabularyBrowseQuery) (domain.VocabularyBrowsePage, map[string]browseHeadwordRow) {
		t.Helper()
		query.Page = 1
		page, err := store.ListVocabularyBrowsePage(ctx, owner.ID, "grc", query)
		require.NoError(t, err)
		rows := make(map[string]browseHeadwordRow, len(page.Rows))
		for _, row := range page.Rows {
			rows[row.CanonicalLemma+"|"+row.UPOS+"|"+row.LexicalDiscriminator] = browseHeadwordRow{
				headword: row.Headword, bare: row.BareLemmaWithNumberedSibling,
				occurrences: row.OccurrenceCount, acrossBooks: row.AcrossBooksOccurrenceCount, books: row.BookCount,
			}
		}
		return page, rows
	}

	t.Run("library", func(t *testing.T) {
		page, rows := browse(domain.VocabularyBrowseQuery{})
		assert.Equal(t, int64(7), page.Total)
		assert.Equal(t, int64(7), page.InventoryTotal)
		assert.Equal(t, map[string]browseHeadwordRow{
			"δεω|VERB|morpheus:1":   {"δέω", false, 2, 2, 1},
			"δεω|VERB|morpheus:2":   {"δέω", false, 1, 1, 1},
			"λογοσ|NOUN|":           {"λόγος", true, 8, 8, 2},
			"λογοσ|NOUN|morpheus:1": {"λόγος", false, 1, 1, 1},
			"λεγω|VERB|":            {"λέγω", false, 10, 10, 2},
			"φιλοσ|ADJ|":            {"φίλος", false, 1, 1, 1},
			"φιλοσ|NOUN|morpheus:1": {"φίλος", false, 1, 1, 1},
		}, rows)
	})

	t.Run("one Book without the numbered sibling", func(t *testing.T) {
		page, rows := browse(domain.VocabularyBrowseQuery{BookIDs: []string{second}})
		assert.Equal(t, int64(4), page.Total)
		assert.Equal(t, map[string]browseHeadwordRow{
			"λογοσ|NOUN|":           {"λόγος", false, 5, 8, 1},
			"λεγω|VERB|":            {"λέγω", false, 6, 10, 1},
			"φιλοσ|ADJ|":            {"φίλος", false, 1, 1, 1},
			"φιλοσ|NOUN|morpheus:1": {"φίλος", false, 1, 1, 1},
		}, rows)
	})

	t.Run("current reading without a headword", func(t *testing.T) {
		page, rows := browse(domain.VocabularyBrowseQuery{CurrentBookID: first, IncludeAll: true})
		assert.Equal(t, int64(5), page.Total)
		assert.Equal(t, int64(5), page.InventoryTotal)
		assert.Equal(t, map[string]browseHeadwordRow{
			"δεω|VERB|morpheus:1":   {"δέω", false, 2, 2, 1},
			"δεω|VERB|morpheus:2":   {"δέω", false, 1, 1, 1},
			"λογοσ|NOUN|":           {"λόγος", true, 3, 8, 1},
			"λογοσ|NOUN|morpheus:1": {"λόγος", false, 1, 1, 1},
			"λεγω|VERB|":            {"", false, 4, 10, 1},
		}, rows)
	})
}
