//go:build integration

package webapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAncientGreekStartReservedEndSwitchBook1Reading is the acceptance seam for
// the fifth Ancient Greek slice: starting a Reading of the imported Book 1
// freezes a snapshot under the 3/15 rule with the source's discriminated
// identities as separate members, reserves them, moves the reading to a second
// Greek Book, and ends on the exact commitment. δέω 1 and δέω 2 share a spelling
// and part of speech and must stay apart throughout, and Greek never shares
// state with the modern-language reading.
func TestAncientGreekStartReservedEndSwitchBook1Reading(t *testing.T) {
	ctx, store, _, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-reading-learner", "learner-password")
	service := ancientgreek.NewService(store)
	first, err := service.Register(ctx, learner.Username, ancientgreek.AnabasisBook1)
	require.NoError(t, err)
	secondDefinition := ancientgreek.AnabasisBook1
	secondDefinition.Key = "xenophon-anabasis-1-second"
	secondDefinition.SourceFileName = "second.conllu"
	second, err := service.Register(ctx, learner.Username, secondDefinition)
	require.NoError(t, err)
	require.True(t, second.Created)
	require.NotEqual(t, first.BookID, second.BookID)

	importGreekFixtureWithDefinition(t, ctx, store, learner.ID, first.BookID, "oga-src-reading.conllu", ancientgreek.AnabasisBook1)
	importGreekFixtureWithDefinition(t, ctx, store, learner.ID, second.BookID, "oga-src-reading-b.conllu", secondDefinition)
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, first.BookID, domain.BookDispositionToRead))
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, second.BookID, domain.BookDispositionToRead))
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))

	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	started := perform(t, h, http.MethodPost, "/reading/books/"+first.BookID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code, "start body=%s", started.Body.String())

	reading, err := store.GetCurrentReading(ctx, learner.ID, "grc")
	require.NoError(t, err)
	require.Equal(t, first.BookID, reading.BookID)
	require.NotEmpty(t, reading.SnapshotID)
	require.Equal(t, 3, reading.SnapshotSize)

	// The frozen snapshot keeps δέω 1 and δέω 2 as separate members and carries
	// each identity's citation headword.
	snapshot, err := store.ListCurrentReadingSnapshotVocabulary(ctx, learner.ID, reading.SnapshotID)
	require.NoError(t, err)
	members := make(map[string]string, len(snapshot))
	for _, row := range snapshot {
		members[row.CanonicalLemma+"|"+row.UPOS+"|"+row.LexicalDiscriminator] = row.Headword
	}
	assert.Equal(t, map[string]string{
		"δέω|VERB|morpheus:1": "δέω",
		"δέω|VERB|morpheus:2": "δέω",
		"λόγοσ|NOUN|":         "λόγος",
	}, members)
	// ἵππος occurs once in the Book and nowhere else, so the 3/15 rule keeps it
	// out of the snapshot.
	assert.NotContains(t, members, "ἵπποσ|NOUN|", "an identity below the in-Book floor is not frozen")

	// Reserved includes both homographs while the Reading is active.
	reserved, err := store.ListReservedVocabulary(ctx, learner.ID, "grc")
	require.NoError(t, err)
	for _, item := range reserved {
		assert.NotEmpty(t, item.Headword, "every imported reservation carries its citation headword")
	}
	assert.Equal(t, []string{"", "morpheus:1", "morpheus:2"}, reservedDiscriminatorKeys(reserved))

	// A Greek Reading never becomes the modern-language reading, so the two
	// study languages stay independent.
	modern, err := store.GetCurrentReading(ctx, learner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, modern.BookID)

	// Switch moves the reading to the second Greek Book, which freezes the
	// second Book's identities as its own snapshot.
	switched := perform(t, h, http.MethodPost, "/reading/books/"+second.BookID+"/switch", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_book_id":     {reading.BookID},
		"expected_current_snapshot_id": {reading.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, switched.Code, "switch body=%s", switched.Body.String())
	afterSwitch, err := store.GetCurrentReading(ctx, learner.ID, "grc")
	require.NoError(t, err)
	require.Equal(t, second.BookID, afterSwitch.BookID)
	require.NotEqual(t, reading.SnapshotID, afterSwitch.SnapshotID)
	require.Equal(t, 3, afterSwitch.SnapshotSize)
	reservedAfterSwitch, err := store.ListReservedVocabulary(ctx, learner.ID, "grc")
	require.NoError(t, err)
	assert.Equal(t, []string{"", "morpheus:1", "morpheus:2"}, reservedDiscriminatorKeys(reservedAfterSwitch))

	// End clears exactly the commitment the second reading named.
	ended := perform(t, h, http.MethodPost, "/reading/end", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_book_id":     {afterSwitch.BookID},
		"expected_current_snapshot_id": {afterSwitch.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, ended.Code, "end body=%s", ended.Body.String())
	after, err := store.GetCurrentReading(ctx, learner.ID, "grc")
	require.NoError(t, err)
	assert.Empty(t, after.BookID)
	reservedAfter, err := store.ListReservedVocabulary(ctx, learner.ID, "grc")
	require.NoError(t, err)
	assert.Empty(t, reservedAfter)
}

// reservedDiscriminatorKeys renders a reservation's discriminators in sorted
// order, so a test asserts membership without depending on row order.
func reservedDiscriminatorKeys(reserved []domain.DeckPreparationVocabulary) []string {
	keys := make([]string, 0, len(reserved))
	for _, item := range reserved {
		keys = append(keys, item.LexicalDiscriminator)
	}
	sort.Strings(keys)
	return keys
}

// importGreekFixtureWithDefinition imports one synthetic oga_src fixture under
// the definition, pinned to the fixture's size and SHA-256.
func importGreekFixtureWithDefinition(t *testing.T, ctx context.Context, store *persistence.PostgresStore, ownerID, bookID, name string, definition domain.ReadingUnitDefinition) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", name)) //nolint:gosec // the fixture name is a test constant.
	require.NoError(t, err)
	definition.SourceFileSize = int64(len(data))
	sum := sha256.Sum256(data)
	definition.SourceFileSHA256 = hex.EncodeToString(sum[:])
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, data, 0o600)) //nolint:gosec // path is inside t.TempDir().
	_, err = ancientgreek.Import(ctx, ancientgreek.ImportRequest{
		OwnerID: ownerID, BookID: bookID, SourcePath: path, Definition: definition,
	}, analysis.NewGreekImportWriter(store))
	require.NoError(t, err)
}
