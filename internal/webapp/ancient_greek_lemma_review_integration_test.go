//go:build integration

package webapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAncientGreekLemmaReviewCorrectsAndExcludesHomographs is the acceptance
// seam for the seventh Ancient Greek slice. A learner previews and confirms a
// correction of one δέω homograph and an exclusion of the other,
// the Browse projection updates in the same transaction with the correct
// discriminator, and re-importing the same pinned file under a new
// interpretation version carries none of those decisions onto the new run.
func TestAncientGreekLemmaReviewCorrectsAndExcludesHomographs(t *testing.T) {
	ctx, store, pool, h := ancientGreekHarness(t)
	learner := createAccount(t, ctx, store, "greek-lemma-learner", "learner-password")
	registration, err := registerGreekDefinition(t, ctx, store, learner.ID, greekTestDefinition())
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

	require.NoError(t, store.SetActiveStudyLanguage(ctx, learner.ID, "grc"))

	occurrences, err := store.ListLemmaReviewOccurrences(ctx, learner.ID, registration.BookID, "δέω")
	require.NoError(t, err)
	require.Len(t, occurrences, 2)
	require.Equal(t, "morpheus:1", occurrences[0].LexicalDiscriminator)
	require.Equal(t, "morpheus:2", occurrences[1].LexicalDiscriminator, "the review carries each homograph's discriminator")

	// Correct the first homograph to λύω; the recorded ancient-greek/1 profile
	// normalizes it and the correction carries no discriminator.
	confirmLemmaDecision(t, h, learner.ID, registration.BookID, "correct", "λύω", occurrences[0].ID())

	// Re-entering the same correction keeps the original decision.
	confirmLemmaDecision(t, h, learner.ID, registration.BookID, "correct", "λύω", occurrences[0].ID())

	// Exclude the second homograph.
	confirmLemmaDecision(t, h, learner.ID, registration.BookID, "exclude", "", occurrences[1].ID())

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
	decided := map[string]int64{
		"λύω|VERB|":             1,
		"λέγω|VERB|":            1,
		"λόγοσ|NOUN|":           1,
		"λόγοσ|NOUN|morpheus:1": 1,
	}
	assert.Equal(t, decided, greekBrowseCounts(t, ctx, pool, learner.ID, registration.BookID))

	// Re-importing the same pin under a new interpretation version is a new
	// analysis run. Its tokens are identical, so both decisions carry onto it
	// (ADR 0126).
	definition, sourcePath := browseFixtureImport(t)
	definition.InterpretationVersion = "oga-agdt-upos/6"
	_, err = importGreekSource(ctx, greekImportRequest{
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
	assert.Equal(t, []string{"λύω", ""}, corrections, "the correction carries onto the identical token")
	assert.Equal(t, []bool{false, true}, exclusions, "and so does the exclusion")
	assert.Equal(t, decided, greekBrowseCounts(t, ctx, pool, learner.ID, registration.BookID),
		"the new run's projection already reflects the carried decisions")
}

// browseFixtureImport pins the synthetic browse fixture as the import definition
// and writes it to a temporary path, for a re-import under a new version.
func browseFixtureImport(t *testing.T) (domain.ReadingUnitDefinition, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-browse.conllu"))
	require.NoError(t, err)
	definition := greekTestDefinition()
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
