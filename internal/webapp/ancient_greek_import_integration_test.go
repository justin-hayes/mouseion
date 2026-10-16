//go:build integration

package webapp

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAncientGreekImportPreparesBook is the acceptance seam for the second
// Ancient Greek slice: importing the pinned oga_src dialect makes My Books show
// the Book as prepared with a word count and coverage, with no NLP service and
// no EPUB analysis.
func TestAncientGreekImportPreparesBook(t *testing.T) {
	ctx, store, pool, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-import-learner", "learner-password")
	registration, err := ancientgreek.NewService(store).Register(ctx, learner.Username, ancientgreek.AnabasisBook1)
	require.NoError(t, err)

	definition, sourcePath := fixtureImport(t)
	report, err := ancientgreek.Import(ctx, ancientgreek.ImportRequest{
		OwnerID:    learner.ID,
		BookID:     registration.BookID,
		SourcePath: sourcePath,
		Definition: definition,
	}, analysis.NewGreekImportWriter(store))
	require.NoError(t, err)
	assert.Equal(t, 2, report.Sentences)
	assert.Equal(t, 10, report.Tokens)
	assert.Equal(t, 1, report.EllipsisTokensSkipped)
	assert.Equal(t, 2, report.ProperNamesDerived)
	assert.Equal(t, 2, report.NumberedLemmaTypes)
	assert.Equal(t, 1, report.BareLemmasWithNumberedSibling)
	assert.False(t, report.Reimported)

	// The imported tokens persisted with their headwords and discriminators.
	var tokens, analyses int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM corpus_tokens WHERE owner_id=$1 AND language='grc'`, learner.ID).Scan(&tokens))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1 AND syntax_state='absent'`, learner.ID).Scan(&analyses))
	assert.Equal(t, 10, tokens)
	assert.Equal(t, 1, analyses)

	// My Books word count is the analysis's analyzable-token count.
	var analyzable int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT c.analyzable_token_count FROM corpora c JOIN book_current_analyses p ON p.owner_id=c.owner_id AND p.analysis_run_id=c.analysis_run_id WHERE p.owner_id=$1 AND p.book_id=$2`, learner.ID, registration.BookID).Scan(&analyzable))
	assert.Equal(t, int64(5), analyzable)

	// The effective vocabulary keeps the homographs distinct, and no proper
	// name is offered as vocabulary.
	rows, err := pool.Query(ctx, `SELECT canonical_lemma, lexical_discriminator, upos FROM vocabulary_browse_counts WHERE owner_id=$1`, learner.ID)
	require.NoError(t, err)
	defer rows.Close()
	identities := map[string]string{}
	for rows.Next() {
		var lemma, discriminator, upos string
		require.NoError(t, rows.Scan(&lemma, &discriminator, &upos))
		identities[discriminator] = upos
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, "VERB", identities[""])
	assert.Equal(t, "VERB", identities["morpheus:1"])
	assert.Equal(t, "VERB", identities["morpheus:2"])
	assert.NotContains(t, identities, "PROPN")

	// My Books shows the Book as prepared with a coverage figure.
	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	library := perform(t, h, "GET", "/library", nil, cookies)
	require.Equal(t, http.StatusOK, library.Code, "library body=%s", library.Body.String())
	assert.NotContains(t, library.Body.String(), notPreparedYetNote)
	assert.Contains(t, library.Body.String(), "0.0% of running words Known")

	// To Read changes the disposition and never submits EPUB analysis.
	detail, err := store.GetBookDetail(ctx, learner.ID, registration.BookID)
	require.NoError(t, err)
	moved := perform(t, h, "POST", "/library/books/"+registration.BookID+"/to-read", url.Values{
		"csrf_token":        {csrf},
		"expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code)
	after, err := store.GetBookDetail(ctx, learner.ID, registration.BookID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, after.Disposition)
	var jobs int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id=$1`, learner.ID).Scan(&jobs))
	assert.Zero(t, jobs, "To Read on a prepared imported Book submits no analysis job")

	// Re-import with the same pin and version changes nothing.
	again, err := ancientgreek.Import(ctx, ancientgreek.ImportRequest{
		OwnerID:    learner.ID,
		BookID:     registration.BookID,
		SourcePath: sourcePath,
		Definition: definition,
	}, analysis.NewGreekImportWriter(store))
	require.NoError(t, err)
	assert.True(t, again.Reimported)
	var runs, corpora int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1`, learner.ID).Scan(&runs))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM corpora WHERE owner_id=$1`, learner.ID).Scan(&corpora))
	assert.Equal(t, 1, runs)
	assert.Equal(t, 1, corpora)

	// A new interpretation version gives a new run for the same pin.
	versioned := definition
	versioned.InterpretationVersion = "oga-agdt-upos/5"
	fresh, err := ancientgreek.Import(ctx, ancientgreek.ImportRequest{
		OwnerID:    learner.ID,
		BookID:     registration.BookID,
		SourcePath: sourcePath,
		Definition: versioned,
	}, analysis.NewGreekImportWriter(store))
	require.NoError(t, err)
	assert.False(t, fresh.Reimported)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1`, learner.ID).Scan(&runs))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM corpora WHERE owner_id=$1`, learner.ID).Scan(&corpora))
	assert.Equal(t, 2, runs)
	assert.Equal(t, 2, corpora)
	var current string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, learner.ID, registration.BookID).Scan(&current))
	assert.Equal(t, fresh.RunID, current, "the newest run is the current analysis")
}

// TestAncientGreekImportRefusesAnUnregisteredBook proves an import into a Book
// that is not the definition's Reading unit writes no analysis.
func TestAncientGreekImportRefusesAnUnregisteredBook(t *testing.T) {
	ctx, store, pool, _ := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-import-unregistered", "learner-password")
	definition, sourcePath := fixtureImport(t)

	_, err := ancientgreek.Import(ctx, ancientgreek.ImportRequest{
		OwnerID:    learner.ID,
		BookID:     uuid.NewString(),
		SourcePath: sourcePath,
		Definition: definition,
	}, analysis.NewGreekImportWriter(store))
	require.Error(t, err)
	var runs int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1`, learner.ID).Scan(&runs))
	assert.Zero(t, runs)
}

// fixtureImport pins the synthetic oga_src fixture as the import definition and
// writes it to a temporary path.
func fixtureImport(t *testing.T) (domain.ReadingUnitDefinition, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	definition := ancientgreek.AnabasisBook1
	definition.SourceFileSize = int64(len(data))
	sum := sha256.Sum256(data)
	definition.SourceFileSHA256 = hex.EncodeToString(sum[:])
	path := filepath.Join(t.TempDir(), "source.conllu")
	require.NoError(t, os.WriteFile(path, data, 0o600)) //nolint:gosec // path is inside t.TempDir().
	return definition, path
}
