package storecontract

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// greekIdentity is one imported classical identity: a canonical lemma, part of
// speech, scheme-qualified discriminator, and the citation headword the source
// froze. The headword is display-only evidence and never part of the identity.
func greekIdentity(lemma, upos, discriminator, headword string) domain.SnapshotIdentity {
	return domain.SnapshotIdentity{Language: GreekLanguage, CanonicalLemma: lemma, UPOS: upos, LexicalDiscriminator: discriminator, Headword: headword}
}

// greekVocabulary is a small imported analysis whose δέω homographs share a
// spelling and part of speech.
var greekVocabulary = []domain.SnapshotIdentity{
	greekIdentity("δέω", "VERB", "morpheus:1", "δέω"),
	greekIdentity("δέω", "VERB", "morpheus:2", "δέω"),
}

// greekOtherVocabulary is the analysis of a second Greek Book.
var greekOtherVocabulary = []domain.SnapshotIdentity{
	greekIdentity("λέγω", "VERB", "", "λέγω"),
	greekIdentity("λόγος", "NOUN", "", "λόγος"),
}

// seedGreekToRead seeds an analyzed To Read Greek Book for the learner under test.
func seedGreekToRead(t *testing.T, h Harness, vocabulary []domain.SnapshotIdentity) string {
	t.Helper()
	return h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{Language: GreekLanguage, Imported: true, Vocabulary: vocabulary})
}

// startGreekReading seeds an eligible Greek Book with vocabulary and starts it.
func startGreekReading(t *testing.T, h Harness, vocabulary []domain.SnapshotIdentity) (string, domain.CurrentReading) {
	t.Helper()
	bookID := seedGreekToRead(t, h, vocabulary)
	reading, err := h.Store().StartCurrentReading(h.Seeds().Owner(), GreekLanguage, bookID)
	require.NoError(t, err)
	return bookID, reading
}

// greekScenarios are the Ancient Greek variants of the Current reading
// lifecycle. Readings are scoped to their study language: Greek never shares
// state with the modern-language scenarios or with Modern Greek.
func greekScenarios() []Scenario {
	return []Scenario{{
		Name: "grc read next makes a Greek Book the grc reading only",
		Run: func(t *testing.T, h Harness) {
			bookID, reading := startGreekReading(t, h, greekVocabulary)
			assert.Equal(t, bookID, reading.BookID)
			current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), GreekLanguage)
			require.NoError(t, err)
			assert.Equal(t, bookID, current.BookID)

			// A Greek reading never appears in the modern-language study language.
			modern, err := h.Store().GetCurrentReading(h.Seeds().Owner(), Language)
			require.NoError(t, err)
			assert.Empty(t, modern.BookID, "a grc reading does not become the de reading")
		},
	}, {
		Name: "grc end clears the reading",
		Run: func(t *testing.T, h Harness) {
			startGreekReading(t, h, greekVocabulary)
			require.NoError(t, h.Store().EndCurrentReading(h.Seeds().Owner(), GreekLanguage))
			current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), GreekLanguage)
			require.NoError(t, err)
			assert.Empty(t, current.BookID)
		},
	}, {
		Name: "grc read next moves between two Greek Books",
		Run: func(t *testing.T, h Harness) {
			startGreekReading(t, h, greekVocabulary)
			second := seedGreekToRead(t, h, greekOtherVocabulary)
			switched, err := h.Store().StartCurrentReading(h.Seeds().Owner(), GreekLanguage, second)
			require.NoError(t, err)
			assert.Equal(t, second, switched.BookID)
			current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), GreekLanguage)
			require.NoError(t, err)
			assert.Equal(t, second, current.BookID)
		},
	}, {
		Name: "grc and el readings are independent",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			greekBook := seedGreekToRead(t, h, greekVocabulary)
			elBook := h.Seeds().SeedBook(t, owner, BookSeed{Language: "el", Vocabulary: []domain.SnapshotIdentity{{Language: "el", CanonicalLemma: "σπίτι", UPOS: "NOUN"}}})

			greekReading, err := h.Store().StartCurrentReading(owner, GreekLanguage, greekBook)
			require.NoError(t, err)
			elReading, err := h.Store().StartCurrentReading(owner, "el", elBook)
			require.NoError(t, err)
			assert.Equal(t, greekBook, greekReading.BookID)
			assert.Equal(t, elBook, elReading.BookID)

			// Ending Greek leaves the Modern Greek reading intact.
			require.NoError(t, h.Store().EndCurrentReading(owner, GreekLanguage))
			stillEl, err := h.Store().GetCurrentReading(owner, "el")
			require.NoError(t, err)
			assert.Equal(t, elBook, stillEl.BookID)
		},
	}, {
		Name: "grc finish records a completion and clears only the grc reading",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID, _ := startGreekReading(t, h, greekVocabulary)
			result, err := h.Store().FinishCurrentReading(owner, GreekLanguage)
			require.NoError(t, err)
			assert.Equal(t, bookID, result.Completion.BookID)
			assert.Equal(t, GreekLanguage, result.Completion.Language)
			current, err := h.Store().GetCurrentReading(owner, GreekLanguage)
			require.NoError(t, err)
			assert.Empty(t, current.BookID)
		},
	}}
}
