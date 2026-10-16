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
	"regexp"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAncientGreekLemmaReviewCorrectsAndExcludesHomographs is the acceptance
// seam for the seventh Ancient Greek slice. A learner previews and confirms a
// correction of one δέω homograph and an exclusion of the other through HTTP,
// the Browse projection updates in the same transaction with the correct
// discriminator, and re-importing the same pinned file under a new
// interpretation version carries none of those decisions onto the new run.
func TestAncientGreekLemmaReviewCorrectsAndExcludesHomographs(t *testing.T) {
	ctx, store, pool, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-lemma-learner", "learner-password")
	registration, err := ancientgreek.NewService(store).Register(ctx, learner.Username, ancientgreek.AnabasisBook1)
	require.NoError(t, err)
	importGreekFixture(t, ctx, store, learner.ID, registration.BookID, "oga-src-browse.conllu")

	// The imported Book's projection keeps the two δέω homographs apart.
	initial := greekBrowseCounts(t, ctx, pool, learner.ID, registration.BookID)
	require.Equal(t, map[string]int64{
		"δέω|VERB|morpheus:1":   1,
		"δέω|VERB|morpheus:2":   1,
		"λέγω|VERB|":            1,
		"λόγοσ|NOUN|":           1,
		"λόγοσ|NOUN|morpheus:1": 1,
	}, initial)

	cookies, csrf := loginCookies(t, h, learner.Username, "learner-password")
	chosen := perform(t, h, "POST", "/active-study-language", url.Values{"csrf_token": {csrf}, "language": {"grc"}}, cookies)
	require.Equal(t, http.StatusSeeOther, chosen.Code, "choose Ancient Greek as the study language: %s", chosen.Body.String())

	occurrences, err := store.ListLemmaReviewOccurrences(ctx, learner.ID, registration.BookID, "δέω")
	require.NoError(t, err)
	require.Len(t, occurrences, 2)
	require.Equal(t, "morpheus:1", occurrences[0].LexicalDiscriminator)
	require.Equal(t, "morpheus:2", occurrences[1].LexicalDiscriminator, "the review carries each homograph's discriminator")

	reviewPath := "/reading/books/" + registration.BookID + "/lemma-review"
	page := perform(t, h, "GET", reviewPath+"?form="+url.QueryEscape("δέω"), nil, cookies)
	require.Equal(t, http.StatusOK, page.Code, "review body=%s", page.Body.String())

	// Correct the first homograph to λύω; the recorded ancient-greek/1 profile
	// normalizes it and the correction carries no discriminator.
	confirmLemmaDecision(t, h, reviewPath, cookies, csrf, "correct", "λύω", occurrences[0].ID())

	// Re-entering the same correction keeps the original decision.
	preview := perform(t, h, "POST", reviewPath, url.Values{
		"csrf_token": {csrf}, "stage": {"preview"}, "form": {"δέω"}, "target": {occurrences[0].ID()},
		"decision": {"correct"}, "lemma": {"λύω"},
	}, cookies)
	require.Equal(t, http.StatusOK, preview.Code, "re-preview body=%s", preview.Body.String())
	repeated := perform(t, h, "POST", reviewPath, url.Values{
		"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"δέω"}, "decision": {"correct"}, "lemma": {"λύω"},
		"fingerprint": {previewFingerprint(t, preview.Body.String())}, "selected": {occurrences[0].ID()},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, repeated.Code, "re-confirm body=%s", repeated.Body.String())

	// Exclude the second homograph.
	confirmLemmaDecision(t, h, reviewPath, cookies, csrf, "exclude", "", occurrences[1].ID())

	// The decisions persist for exactly their occurrences and record the
	// analysis's normalization profile by its exact version.
	after, err := store.ListLemmaReviewOccurrences(ctx, learner.ID, registration.BookID, "δέω")
	require.NoError(t, err)
	require.Len(t, after, 2)
	assert.Equal(t, "λύω", after[0].CorrectedLemma)
	assert.False(t, after[0].Excluded)
	assert.True(t, after[1].Excluded)
	var profile, version string
	require.NoError(t, pool.QueryRow(ctx, `SELECT normalization_profile,normalization_version FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2`, learner.ID, registration.BookID).Scan(&profile, &version))
	assert.Equal(t, "ancient-greek", profile)
	assert.Equal(t, "1", version)

	// Browse updates in the same transaction: the corrected homograph's old
	// discriminated identity is gone and the new bare λύω identity appears,
	// while every other identity keeps its exact count.
	assert.Equal(t, map[string]int64{
		"λύω|VERB|":             1,
		"λέγω|VERB|":            1,
		"λόγοσ|NOUN|":           1,
		"λόγοσ|NOUN|morpheus:1": 1,
	}, greekBrowseCounts(t, ctx, pool, learner.ID, registration.BookID))

	// Re-importing the same pin under a new interpretation version is a new
	// analysis run; none of the earlier decisions apply to it.
	definition, sourcePath := browseFixtureImport(t)
	definition.InterpretationVersion = "oga-agdt-upos/5"
	_, err = ancientgreek.Import(ctx, ancientgreek.ImportRequest{
		OwnerID: learner.ID, BookID: registration.BookID, SourcePath: sourcePath, Definition: definition,
	}, analysis.NewGreekImportWriter(store))
	require.NoError(t, err)

	fresh, err := store.ListLemmaReviewOccurrences(ctx, learner.ID, registration.BookID, "δέω")
	require.NoError(t, err)
	require.Len(t, fresh, 2)
	corrections := make([]string, 0, len(fresh))
	exclusions := make([]bool, 0, len(fresh))
	for _, occurrence := range fresh {
		corrections = append(corrections, occurrence.CorrectedLemma)
		exclusions = append(exclusions, occurrence.Excluded)
	}
	assert.Equal(t, []string{"", ""}, corrections, "a new analysis run carries no correction from the old run")
	assert.Equal(t, []bool{false, false}, exclusions, "a new analysis run carries no exclusion from the old run")
	assert.Equal(t, initial, greekBrowseCounts(t, ctx, pool, learner.ID, registration.BookID),
		"the new run's projection holds no decision from the old run")
}

// confirmLemmaDecision previews and confirms one decision through the review
// form, using the fingerprint the preview returned.
func confirmLemmaDecision(t *testing.T, h http.Handler, reviewPath string, cookies []*http.Cookie, csrf, decision, lemma, occurrenceID string) {
	t.Helper()
	preview := perform(t, h, "POST", reviewPath, url.Values{
		"csrf_token": {csrf}, "stage": {"preview"}, "form": {"δέω"}, "target": {occurrenceID},
		"decision": {decision}, "lemma": {lemma},
	}, cookies)
	require.Equal(t, http.StatusOK, preview.Code, "preview %s body=%s", decision, preview.Body.String())
	confirm := perform(t, h, "POST", reviewPath, url.Values{
		"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"δέω"}, "decision": {decision}, "lemma": {lemma},
		"fingerprint": {previewFingerprint(t, preview.Body.String())}, "selected": {occurrenceID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, confirm.Code, "confirm %s body=%s", decision, confirm.Body.String())
}

// previewFingerprint reads the stale-state fingerprint a preview page carries.
func previewFingerprint(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`name="fingerprint" value="([a-f0-9]+)"`).FindStringSubmatch(body)
	require.Len(t, match, 2, "preview body has no fingerprint: %s", body)
	return match[1]
}

// browseFixtureImport pins the synthetic browse fixture as the import definition
// and writes it to a temporary path, for a re-import under a new version.
func browseFixtureImport(t *testing.T) (domain.ReadingUnitDefinition, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-browse.conllu"))
	require.NoError(t, err)
	definition := ancientgreek.AnabasisBook1
	definition.SourceFileSize = int64(len(data))
	sum := sha256.Sum256(data)
	definition.SourceFileSHA256 = hex.EncodeToString(sum[:])
	path := filepath.Join(t.TempDir(), "browse.conllu")
	require.NoError(t, os.WriteFile(path, data, 0o600)) //nolint:gosec // path is inside t.TempDir().
	return definition, path
}

// greekBrowseCounts reads the Book's current persisted Browse projection keyed
// by canonical lemma, part of speech and discriminator.
func greekBrowseCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, bookID string) map[string]int64 {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT c.canonical_lemma,c.upos,c.lexical_discriminator,c.occurrence_count
		FROM vocabulary_browse_counts c
		JOIN book_current_analyses cai ON cai.owner_id=c.owner_id AND cai.book_id=$2
			AND cai.analysis_run_id=c.analysis_run_id
		WHERE c.owner_id=$1`, owner, bookID)
	require.NoError(t, err)
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var lemma, upos, discriminator string
		var count int64
		require.NoError(t, rows.Scan(&lemma, &upos, &discriminator, &count))
		counts[lemma+"|"+upos+"|"+discriminator] = count
	}
	require.NoError(t, rows.Err())
	return counts
}
