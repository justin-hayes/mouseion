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
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAncientGreekReadNextEndAndFinishBook1Reading is the acceptance seam for
// Reading an imported Ancient Greek Book: Read next makes it the grc reading
// with no snapshot, replaces it with a second Greek Book, and End and Finish
// need no commitment. Finish records a completion and accepts no Known
// vocabulary. Greek never shares state with the modern-language reading.
func TestAncientGreekReadNextEndAndFinishBook1Reading(t *testing.T) {
	ctx, store, _, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-reading-learner", "learner-password")
	first, err := registerGreekDefinition(t, ctx, store, learner.ID, greekTestDefinition())
	require.NoError(t, err)
	secondDefinition := greekTestDefinition()
	secondDefinition.Key = "xenophon-anabasis-1-second"
	secondDefinition.SourceFileName = "second.conllu"
	second, err := registerGreekDefinition(t, ctx, store, learner.ID, secondDefinition)
	require.NoError(t, err)
	require.True(t, second.Created)
	require.NotEqual(t, first.BookID, second.BookID)

	importGreekFixtureWithDefinition(t, ctx, store, learner.ID, first.BookID, "oga-src-reading.conllu", greekTestDefinition())
	importGreekFixtureWithDefinition(t, ctx, store, learner.ID, second.BookID, "oga-src-reading-b.conllu", secondDefinition)
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, first.BookID, domain.BookDispositionToRead))
	require.NoError(t, store.SetBookDisposition(ctx, learner.ID, second.BookID, domain.BookDispositionToRead))
	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))

	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	form := url.Values{"csrf_token": {csrf}}
	started := perform(t, h, http.MethodPost, "/reading/books/"+first.BookID+"/start", form, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code, "start body=%s", started.Body.String())
	reading, err := store.GetCurrentReading(ctx, learner.ID, "grc")
	require.NoError(t, err)
	require.Equal(t, first.BookID, reading.BookID)

	// A Greek Reading never becomes the modern-language reading, so the two
	// study languages stay independent.
	modern, err := store.GetCurrentReading(ctx, learner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, modern.BookID)

	switched := perform(t, h, http.MethodPost, "/reading/books/"+second.BookID+"/switch", form, cookies)
	require.Equal(t, http.StatusSeeOther, switched.Code, "switch body=%s", switched.Body.String())
	afterSwitch, err := store.GetCurrentReading(ctx, learner.ID, "grc")
	require.NoError(t, err)
	require.Equal(t, second.BookID, afterSwitch.BookID)

	ended := perform(t, h, http.MethodPost, "/reading/end", form, cookies)
	require.Equal(t, http.StatusSeeOther, ended.Code, "end body=%s", ended.Body.String())
	after, err := store.GetCurrentReading(ctx, learner.ID, "grc")
	require.NoError(t, err)
	assert.Empty(t, after.BookID)

	restarted := perform(t, h, http.MethodPost, "/reading/books/"+first.BookID+"/start", form, cookies)
	require.Equal(t, http.StatusSeeOther, restarted.Code, "restart body=%s", restarted.Body.String())
	finished := perform(t, h, http.MethodPost, "/reading/finish", form, cookies)
	require.Equal(t, http.StatusOK, finished.Code, "finish body=%s", finished.Body.String())
	known, err := store.ListKnownVocabulary(ctx, learner.ID, "grc")
	require.NoError(t, err)
	assert.Empty(t, known, "Finish accepts no Known vocabulary")
	afterFinish, err := store.GetCurrentReading(ctx, learner.ID, "grc")
	require.NoError(t, err)
	assert.Empty(t, afterFinish.BookID)
	detail, err := store.GetBookDetail(ctx, learner.ID, first.BookID)
	require.NoError(t, err)
	assert.Equal(t, 1, detail.CompletionCount)
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
