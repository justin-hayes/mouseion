//go:build integration

package webapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAncientGreekBrowseListsBook1Vocabulary is the acceptance seam for the
// third Ancient Greek slice: the Browse projection lists the imported Book's
// vocabulary with citation headwords, keeps its homographs apart, labels a bare
// lemma whose source gives no homograph number for it, and offers no proper
// name. It reads the real PostgreSQL projection and renders it through the
// Reading Browse component a learner sees.
//
// Starting a Reading of Book 1 is the next slice (#2106), so this one drives
// Browse through the projection and the component rather than through
// /reading.
func TestAncientGreekBrowseListsBook1Vocabulary(t *testing.T) {
	ctx, store, _, _ := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-browse-learner", "learner-password")
	registration, err := ancientgreek.NewService(store).Register(ctx, learner.Username, ancientgreek.AnabasisBook1)
	require.NoError(t, err)
	importGreekFixture(t, ctx, store, learner.ID, registration.BookID, "oga-src-browse.conllu")

	// The projection keeps δέω 1 and δέω 2 apart, carries the citation headword
	// rather than the stored key, and flags the bare λόγος whose work also
	// numbers it. No proper name reaches the eligible set.
	page, err := store.ListVocabularyBrowsePage(ctx, learner.ID, "grc", domain.VocabularyBrowseQuery{CurrentBookID: registration.BookID, IncludeAll: true, Page: 1})
	require.NoError(t, err)
	require.Equal(t, int64(5), page.Total)
	require.Len(t, page.Rows, 5)

	rows := make(map[string]domain.VocabularyBrowseRow, len(page.Rows))
	for _, row := range page.Rows {
		rows[row.CanonicalLemma+"|"+row.UPOS+"|"+row.LexicalDiscriminator] = row
		assert.NotEqual(t, "PROPN", row.UPOS, "no proper name is offered as vocabulary")
	}
	assertIdentity := func(key, headword string, bare bool) {
		t.Helper()
		row, ok := rows[key]
		require.True(t, ok, "expected identity %q, got %v", key, sortedKeys(rows))
		assert.Equal(t, headword, row.Headword)
		assert.Equal(t, bare, row.BareLemmaWithNumberedSibling)
		assert.Equal(t, int64(1), row.OccurrenceCount)
		assert.Equal(t, int64(1), row.AcrossBooksOccurrenceCount)
	}
	assertIdentity("δέω|VERB|morpheus:1", "δέω", false)
	assertIdentity("δέω|VERB|morpheus:2", "δέω", false)
	assertIdentity("λέγω|VERB|", "λέγω", false)
	assertIdentity("λόγοσ|NOUN|", "λόγος", true)
	assertIdentity("λόγοσ|NOUN|morpheus:1", "λόγος", false)

	// The same facts render: the citation headword (with final ς) instead of the
	// stored key, two δέω rows, and exactly one bare-lemma note.
	page.CurrentBookID = registration.BookID
	page.ReadingBookID = registration.BookID
	html := renderReadingBrowse(t, "grc", page, "")
	assert.Contains(t, html, ">λόγος</a>", "the noun shows its citation headword")
	assert.NotContains(t, html, ">λογοσ</a>", "the stored canonical key is never shown")
	assert.Equal(t, 2, strings.Count(html, ">δέω</a>"), "both homographs are separate rows")
	assert.Equal(t, 1, strings.Count(html, "the source gives no homograph number"), "only the bare lemma with a numbered sibling is labelled")
	assert.NotContains(t, html, "κῦρος", "a proper name is not offered as vocabulary")
}

func sortedKeys(rows map[string]domain.VocabularyBrowseRow) []string {
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// importGreekFixture imports one synthetic oga_src fixture under a definition
// whose size and SHA-256 pin match the fixture bytes.
func importGreekFixture(t *testing.T, ctx context.Context, store *persistence.PostgresStore, ownerID, bookID, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", name)) //nolint:gosec // the fixture name is a test constant.
	require.NoError(t, err)
	definition := ancientgreek.AnabasisBook1
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
