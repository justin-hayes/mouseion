package storecontract

import (
	"sort"
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
// spelling and part of speech and must stay separate snapshot members.
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
	return h.Seeds().SeedBook(t, h.Seeds().Owner(), BookSeed{Language: GreekLanguage, Imported: true, Vocabulary: vocabulary, ToRead: true})
}

// startGreekReading seeds an eligible Greek Book with vocabulary and starts it.
func startGreekReading(t *testing.T, h Harness, vocabulary []domain.SnapshotIdentity) (string, domain.CurrentReading) {
	t.Helper()
	bookID := seedGreekToRead(t, h, vocabulary)
	reading, err := h.Store().StartCurrentReading(h.Seeds().Owner(), GreekLanguage, bookID)
	require.NoError(t, err)
	return bookID, reading
}

// reservedDiscriminators renders the active reservation's discriminators in a
// language, sorted, so a homograph pair is asserted without depending on order.
func reservedDiscriminators(t *testing.T, h Harness, language string) []string {
	t.Helper()
	reserved, err := h.Store().ListReservedVocabulary(h.Seeds().Owner(), language)
	require.NoError(t, err)
	keys := make([]string, 0, len(reserved))
	for _, item := range reserved {
		keys = append(keys, item.LexicalDiscriminator)
	}
	sort.Strings(keys)
	return keys
}

// requireGreekHeadwords asserts that every active Greek reservation carries the
// citation headword the imported source froze.
func requireGreekHeadwords(t *testing.T, h Harness) {
	t.Helper()
	reserved, err := h.Store().ListReservedVocabulary(h.Seeds().Owner(), GreekLanguage)
	require.NoError(t, err)
	require.NotEmpty(t, reserved)
	for _, item := range reserved {
		assert.NotEmpty(t, item.Headword, "every imported reservation carries its citation headword")
	}
}

// greekScenarios are the Ancient Greek variants of the Current reading
// lifecycle. They exercise discriminated identities: homographs that share a
// canonical lemma and part of speech stay separate members throughout Start,
// Reserved, End, Switch and Finish, and Greek never shares state with the
// modern-language scenarios.
func greekScenarios() []Scenario {
	return []Scenario{{
		Name: "grc start freezes discriminated homographs as separate members",
		Run: func(t *testing.T, h Harness) {
			bookID, reading := startGreekReading(t, h, greekVocabulary)
			assert.Equal(t, bookID, reading.BookID)
			assert.NotEmpty(t, reading.SnapshotID)
			assert.Equal(t, 2, reading.SnapshotSize)
			current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), GreekLanguage)
			require.NoError(t, err)
			assert.Equal(t, bookID, current.BookID)
			assert.Equal(t, reading.SnapshotID, current.SnapshotID)
			requireGreekHeadwords(t, h)
			assert.Equal(t, []string{"morpheus:1", "morpheus:2"}, reservedDiscriminators(t, h, GreekLanguage))

			// A Greek reading never appears in the modern-language study language.
			modern, err := h.Store().GetCurrentReading(h.Seeds().Owner(), Language)
			require.NoError(t, err)
			assert.Empty(t, modern.BookID, "a grc reading does not become the de reading")
		},
	}, {
		Name: "grc end clears the exact commitment",
		Run: func(t *testing.T, h Harness) {
			bookID, reading := startGreekReading(t, h, greekVocabulary)
			require.NoError(t, h.Store().EndCurrentReading(h.Seeds().Owner(), GreekLanguage, bookID, reading.SnapshotID))
			current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), GreekLanguage)
			require.NoError(t, err)
			assert.Empty(t, current.BookID)
			assert.Empty(t, reservedDiscriminators(t, h, GreekLanguage))
		},
	}, {
		Name: "grc switch moves between two Greek Books",
		Run: func(t *testing.T, h Harness) {
			first, reading := startGreekReading(t, h, greekVocabulary)
			second := seedGreekToRead(t, h, greekOtherVocabulary)
			switched, err := h.Store().SwitchCurrentReading(h.Seeds().Owner(), GreekLanguage, second, first, reading.SnapshotID)
			require.NoError(t, err)
			assert.Equal(t, second, switched.BookID)
			assert.NotEqual(t, reading.SnapshotID, switched.SnapshotID)
			assert.Equal(t, 2, switched.SnapshotSize)
			current, err := h.Store().GetCurrentReading(h.Seeds().Owner(), GreekLanguage)
			require.NoError(t, err)
			assert.Equal(t, second, current.BookID)
			assert.Equal(t, []string{"", ""}, reservedDiscriminators(t, h, GreekLanguage))
		},
	}, {
		Name: "grc and el readings reserve and count independently",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			greekBook := seedGreekToRead(t, h, greekVocabulary)
			elBook := h.Seeds().SeedBook(t, owner, BookSeed{Language: "el", Vocabulary: []domain.SnapshotIdentity{{Language: "el", CanonicalLemma: "σπίτι", UPOS: "NOUN"}}, ToRead: true})

			greekReading, err := h.Store().StartCurrentReading(owner, GreekLanguage, greekBook)
			require.NoError(t, err)
			elReading, err := h.Store().StartCurrentReading(owner, "el", elBook)
			require.NoError(t, err)
			assert.Equal(t, greekBook, greekReading.BookID)
			assert.Equal(t, elBook, elReading.BookID)
			assert.Equal(t, 2, greekReading.SnapshotSize)
			assert.Equal(t, 1, elReading.SnapshotSize)
			assert.Equal(t, []string{"morpheus:1", "morpheus:2"}, reservedDiscriminators(t, h, GreekLanguage))
			assert.Equal(t, []string{""}, reservedDiscriminators(t, h, "el"))

			// Ending Greek leaves the Modern Greek reading and reservation intact.
			require.NoError(t, h.Store().EndCurrentReading(owner, GreekLanguage, greekBook, greekReading.SnapshotID))
			assert.Empty(t, reservedDiscriminators(t, h, GreekLanguage))
			assert.Equal(t, []string{""}, reservedDiscriminators(t, h, "el"))
			stillEl, err := h.Store().GetCurrentReading(owner, "el")
			require.NoError(t, err)
			assert.Equal(t, elBook, stillEl.BookID)
		},
	}, {
		Name: "grc finish accepts discriminated homographs separately",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID, reading := startGreekReading(t, h, greekVocabulary)
			// A bare Known identity must never cover a numbered homograph.
			h.Seeds().SeedKnownVocabulary(t, owner, []domain.SnapshotIdentity{{Language: GreekLanguage, CanonicalLemma: "δέω", UPOS: "VERB"}})

			result, err := h.Store().FinishCurrentReading(owner, GreekLanguage, bookID, reading.SnapshotID)
			require.NoError(t, err)
			assert.Equal(t, 2, result.Completion.SnapshotVocabularyCount)
			assert.Equal(t, 2, result.Completion.EligibleVocabularyCount)
			assert.Equal(t, 2, result.Completion.GraduatedVocabularyCount)
			assert.Equal(t, 0, result.Completion.AlreadyKnownVocabularyCount)

			// Each accepted identity is Known with the citation headword its
			// snapshot showed; the pre-existing bare identity keeps its own.
			known, err := h.Store().ListKnownVocabulary(owner, GreekLanguage)
			require.NoError(t, err)
			identities := make([]string, 0, len(known))
			for _, item := range known {
				identities = append(identities, knownHeadwordKey(item))
			}
			sort.Strings(identities)
			assert.Equal(t, []string{"δέω:=", "δέω:morpheus:1=δέω", "δέω:morpheus:2=δέω"}, identities)
		},
	}}
}

// knownHeadwordKey renders one Known row as "lemma:discriminator=headword", so a
// scenario asserts the accepted headword without depending on row order.
func knownHeadwordKey(item domain.KnownVocabulary) string {
	return item.CanonicalLemma + ":" + item.LexicalDiscriminator + "=" + item.Headword
}
