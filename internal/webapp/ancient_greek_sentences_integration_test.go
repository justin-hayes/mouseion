//go:build integration

package webapp

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"
)

// TestAncientGreekConcordanceAndSentenceStudy is the acceptance seam for the
// fourth Ancient Greek slice: Concordance finds Greek lemmas and forms across
// NFC/NFD, case and final-sigma variants without merging accented and
// unaccented words, labels a proper name, and Sentence study shows the
// reconstructed sentence with the target highlighted, citation headwords, the
// mapped morphology line, and the syntax-unavailable state instead of a
// fabricated relation and head.
func TestAncientGreekConcordanceAndSentenceStudy(t *testing.T) {
	ctx, store, pool, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-concordance", "learner-password")
	registration, err := ancientgreek.NewService(store).Register(ctx, learner.Username, ancientgreek.AnabasisBook1)
	require.NoError(t, err)

	definition, sourcePath := fixtureImportFrom(t, "oga-src-sentences.conllu")
	report, err := ancientgreek.Import(ctx, ancientgreek.ImportRequest{
		OwnerID:    learner.ID,
		BookID:     registration.BookID,
		SourcePath: sourcePath,
		Definition: definition,
	}, analysis.NewGreekImportWriter(store))
	require.NoError(t, err)
	require.Equal(t, 2, report.Sentences)
	require.Equal(t, 9, report.Tokens)

	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))
	cookies, _ := loginCookies(t, h, learner.Username, "learner-password")

	lemmaQuery := func(term string) url.Values {
		return url.Values{"language": {"grc"}, "term": {term}, "as": {domain.ConcordanceMatchLemma}}
	}
	formQuery := func(term string) url.Values {
		return url.Values{"language": {"grc"}, "term": {term}, "as": {domain.ConcordanceMatchForm}}
	}
	find := func(t *testing.T, query url.Values) string {
		t.Helper()
		response := perform(t, h, http.MethodGet, "/vocabulary/concordance?"+query.Encode(), nil, cookies)
		require.Equal(t, http.StatusOK, response.Code, "body=%s", response.Body.String())
		return response.Body.String()
	}

	t.Run("lemma search variants unify", func(t *testing.T) {
		nfc := find(t, lemmaQuery("λόγος"))
		require.NotEmpty(t, concordanceOccurrenceIDs(t, nfc))
		require.Contains(t, nfc, `<strong class="concordance-observed-target">λόγος</strong>`, "the nominative form is a result")
		require.Contains(t, nfc, `>λόγον<`, "lemma search expands every observed form")
		for _, variant := range []string{norm.NFD.String("λόγος"), "ΛΌΓΟΣ", "λόγοσ"} {
			t.Run(variant, func(t *testing.T) {
				require.Equal(t, concordanceOccurrenceIDs(t, nfc), concordanceOccurrenceIDs(t, find(t, lemmaQuery(variant))))
			})
		}
	})

	t.Run("form search variants unify", func(t *testing.T) {
		final := find(t, formQuery("λόγος"))
		require.NotEmpty(t, concordanceOccurrenceIDs(t, final))
		require.Contains(t, final, `<strong class="concordance-observed-target">λόγος</strong>`)
		for _, variant := range []string{"λόγοσ", norm.NFD.String("λόγος"), "ΛΌΓΟΣ"} {
			t.Run(variant, func(t *testing.T) {
				require.Equal(t, concordanceOccurrenceIDs(t, final), concordanceOccurrenceIDs(t, find(t, formQuery(variant))))
			})
		}
	})

	t.Run("an unaccented query does not match an accented form", func(t *testing.T) {
		for name, query := range map[string]url.Values{"lemma": lemmaQuery("λογος"), "form": formQuery("λογος")} {
			t.Run(name, func(t *testing.T) {
				body := find(t, query)
				require.Contains(t, body, "No analyzed occurrences")
				require.Empty(t, concordanceOccurrenceIDs(t, body))
			})
		}
	})

	t.Run("a proper name is findable and labelled", func(t *testing.T) {
		properName := find(t, lemmaQuery("Κῦρος"))
		require.Contains(t, properName, `<strong class="concordance-observed-target">Κῦρος</strong>`)
		require.Contains(t, properName, "Forms of κῦρος · proper noun")
	})

	var run, corpus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text, corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, learner.ID, registration.BookID).Scan(&run, &corpus))
	unit := domain.ClassicalUnitID(definition.Key)
	studyOf := func(t *testing.T, sentence, target int64) string {
		t.Helper()
		query := url.Values{
			"book": {registration.BookID}, "run": {run}, "corpus": {corpus}, "unit": {unit},
			"sentence": {strconv.FormatInt(sentence, 10)}, "target": {strconv.FormatInt(target, 10)},
		}
		response := perform(t, h, http.MethodGet, "/vocabulary/concordance/sentence?"+query.Encode(), nil, cookies)
		require.Equal(t, http.StatusOK, response.Code, "body=%s", response.Body.String())
		return response.Body.String()
	}

	t.Run("sentence study shows the sentence, target and morphology line", func(t *testing.T) {
		sentence := studyOf(t, 0, 1)
		require.Contains(t, sentence, "ὁ λόγος Κῦρος λέγει.", "the reconstructed sentence is shown")
		require.Contains(t, sentence, `<strong class="sentence-study-word">λόγος</strong>`, "the target is highlighted at its span")
		require.Contains(t, sentence, "SG.M.NOM", "the target's mapped morphology is on the line")
		require.Contains(t, sentence, "3SG.PRS.IND.active", "the verb's mapped morphology is on the line")
	})

	t.Run("sentence study reports syntax unavailable", func(t *testing.T) {
		sentence := studyOf(t, 0, 1)
		require.Contains(t, sentence, "Syntax unavailable")
		require.NotContains(t, sentence, "dependency <code>dep</code>", "the stored non-committal relation is never shown")
		require.NotContains(t, sentence, "head λέγει")
	})

	t.Run("a homograph shows its citation headword", func(t *testing.T) {
		homograph := studyOf(t, 1, 3)
		require.Contains(t, homograph, "λέγει τὸν λόγον δέω")
		require.Contains(t, homograph, `<strong class="sentence-study-word">δέω</strong>`)
		require.Contains(t, homograph, "citation headword <code>δέω</code>")
		require.NotContains(t, homograph, "effective lemma <code>δεω</code>")
		require.NotContains(t, homograph, "<code>δεω</code>")
		require.Contains(t, homograph, "1SG.PRS.IND.active")
		require.Contains(t, homograph, "Syntax unavailable")
		require.NotContains(t, homograph, "dependency <code>dep</code>")
	})
}

// concordanceOccurrenceIDs lists the rendered Concordance occurrence identity
// attributes in source order, so two lookups can be compared without depending
// on any other page content.
func concordanceOccurrenceIDs(t *testing.T, body string) []string {
	t.Helper()
	matches := concordanceOccurrenceIDPattern.FindAllStringSubmatch(body, -1)
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match[1])
	}
	return ids
}

var concordanceOccurrenceIDPattern = regexp.MustCompile(`id="(occurrence-[^"]+)"`)

// fixtureImportFrom pins one synthetic oga_src fixture as the import definition
// and writes it to a temporary path.
func fixtureImportFrom(t *testing.T, name string) (domain.ReadingUnitDefinition, string) {
	t.Helper()
	//nolint:gosec // the caller passes a fixture name compiled into the test.
	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", name))
	require.NoError(t, err)
	definition := ancientgreek.AnabasisBook1
	definition.SourceFileSize = int64(len(data))
	sum := sha256.Sum256(data)
	definition.SourceFileSHA256 = hex.EncodeToString(sum[:])
	path := filepath.Join(t.TempDir(), "source.conllu")
	require.NoError(t, os.WriteFile(path, data, 0o600)) //nolint:gosec // path is inside t.TempDir().
	return definition, path
}
