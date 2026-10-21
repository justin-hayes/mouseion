package dictionary

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/lemmarisk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

// frenchLemmaRiskFixtureJSON is the committed acceptance fixture for French
// lemma-risk detection. It is embedded so the test reads a fixed file rather
// than a path.
//
//go:embed testdata/french_lemma_risk_fixture.json
var frenchLemmaRiskFixtureJSON []byte

// frenchLemmaRiskFixture is the acceptance fixture for French lemma-risk
// detection (issue #2244). It distils the reviewed cases from the #2166
// dictionary-coverage run into an index plus analyzed occurrences: every
// expected flag names the dictionary-backed alternative the learner should
// review, and every unflagged case is a deliberate precision guard. Zero
// incorrect flags is the acceptance bar.
type frenchLemmaRiskFixture struct {
	Entries []struct {
		Lemma  string           `json:"lemma"`
		UPOS   string           `json:"upos"`
		Senses []map[string]any `json:"senses"`
	} `json:"entries"`
	Headwords []struct {
		Lemma string `json:"lemma"`
		UPOS  string `json:"upos"`
	} `json:"headwords"`
	Forms []struct {
		Form       string `json:"form"`
		UPOS       string `json:"upos"`
		Target     string `json:"target"`
		Participle bool   `json:"participle"`
		EvidenceID string `json:"evidence_id"`
	} `json:"forms"`
	Occurrences []struct {
		ID       string `json:"id"`
		Surface  string `json:"surface"`
		Lemma    string `json:"lemma"`
		UPOS     string `json:"upos"`
		Sentence string `json:"sentence"`
	} `json:"occurrences"`
	ExpectedFlags map[string]string `json:"expected_flags"`
}

func TestFrenchLemmaRiskFixtureHasNoIncorrectFlags(t *testing.T) {
	var fixture frenchLemmaRiskFixture
	require.NoError(t, json.Unmarshal(frenchLemmaRiskFixtureJSON, &fixture))
	index := newFrenchLemmaRiskIndex(t, fixture)
	occurrences := make([]lemmarisk.Occurrence, 0, len(fixture.Occurrences))
	uposByID := make(map[string]string, len(fixture.Occurrences))
	for _, occurrence := range fixture.Occurrences {
		occurrences = append(occurrences, lemmarisk.Occurrence{
			ID: occurrence.ID, Language: "fr", Surface: occurrence.Surface,
			Lemma: occurrence.Lemma, UPOS: occurrence.UPOS, Sentence: occurrence.Sentence,
		})
		uposByID[occurrence.ID] = occurrence.UPOS
	}

	flags, assessed, err := lemmarisk.Detect(t.Context(), "fr", occurrences, index)

	require.NoError(t, err)
	require.True(t, assessed, "a French index that answers is an assessment")
	require.Len(t, flags, len(fixture.ExpectedFlags), "flag count must match the reviewed fixture exactly")
	for _, flag := range flags {
		assert.Equal(t, fixture.ExpectedFlags[flag.OccurrenceID], flag.Alternative.Lemma, flag.OccurrenceID)
		assert.Equal(t, uposByID[flag.OccurrenceID], flag.Alternative.UPOS, "an alternative keeps the occurrence's UPOS")
		assert.Equal(t, "wiktionary", flag.Alternative.Source)
		assert.Equal(t, "fixture-v1", flag.Alternative.Version)
		assert.NotEmpty(t, flag.Alternative.EvidenceID)
		assert.True(t, flag.Alternative.ContextRelevant)
		assert.Contains(t, flag.Reason, "dictionary links this surface")
	}
}

// newFrenchLemmaRiskIndex opens a format-4 index seeded from the French fixture.
func newFrenchLemmaRiskIndex(t *testing.T, fixture frenchLemmaRiskFixture) *Index {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", databasePath)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), dictionaryFrenchFormsFixtureSchema)
	require.NoError(t, err)
	for _, entry := range fixture.Entries {
		sensesJSON, err := json.Marshal(entry.Senses)
		require.NoError(t, err)
		_, err = db.ExecContext(t.Context(),
			`INSERT INTO entries VALUES ('fr', ?, ?, ?, '', '', '', '', '', '')`,
			entry.Lemma, entry.UPOS, string(sensesJSON))
		require.NoError(t, err)
	}
	for _, headword := range fixture.Headwords {
		_, err = db.ExecContext(t.Context(), `INSERT INTO french_headwords VALUES (?, ?)`, headword.Lemma, headword.UPOS)
		require.NoError(t, err)
	}
	for _, form := range fixture.Forms {
		_, err = db.ExecContext(t.Context(),
			`INSERT INTO french_forms VALUES (?, ?, ?, ?, ?)`,
			form.Form, form.UPOS, form.Target, form.Participle, form.EvidenceID)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
	index, err := OpenIndex(t.Context(), databasePath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, index.Close()) })
	return index
}

func TestFrenchLemmaRiskUnavailableWithOldIndexOrNoIndex(t *testing.T) {
	t.Run("no index", func(t *testing.T) {
		flags, assessed, err := lemmarisk.Detect(context.Background(), "fr", []lemmarisk.Occurrence{{
			ID: "fraca", Language: "fr", Surface: "fracas", Lemma: "fraca", UPOS: "NOUN", Sentence: "Le fracas est fort.",
		}}, nil)
		require.NoError(t, err)
		assert.False(t, assessed)
		assert.Empty(t, flags)
	})
	t.Run("an index older than the French form-of format", func(t *testing.T) {
		index := newFrenchTestIndex(t,
			`INSERT INTO entries VALUES ('fr', 'fracas', 'NOUN', '[{"Gloss":"fracas"}]', '', '', '', '', '', '')`,
		)
		flags, assessed, err := lemmarisk.Detect(context.Background(), "fr", []lemmarisk.Occurrence{{
			ID: "fraca", Language: "fr", Surface: "fracas", Lemma: "fraca", UPOS: "NOUN", Sentence: "Le fracas est fort.",
		}}, index)
		require.NoError(t, err)
		assert.False(t, assessed, "an older index is unavailable, never a clean verdict")
		assert.Empty(t, flags)

		_, err = index.AlternativesForLemma(context.Background(), "fr", "fracas", "fraca", "NOUN", "Le fracas est fort.")
		require.ErrorIs(t, err, ErrFrenchFormsUnavailable)
	})
	t.Run("a Book whose occurrences are all reviewed", func(t *testing.T) {
		index := newFrenchTestIndex(t)
		flags, assessed, err := lemmarisk.Detect(context.Background(), "fr", []lemmarisk.Occurrence{{
			ID: "reviewed", Language: "fr", Surface: "fracas", Lemma: "fraca", UPOS: "NOUN", Sentence: "Le fracas est fort.", Reviewed: true,
		}}, index)
		require.NoError(t, err)
		assert.False(t, assessed, "an unavailable index is not assessed even when there is nothing left to inspect")
		assert.Empty(t, flags)
	})
}

func TestFrenchAlternativesIgnoreEmptyIdentityAndBlankSurface(t *testing.T) {
	index := newFrenchFormsTestIndex(t)

	alternatives, err := index.AlternativesForLemma(t.Context(), "fr", "fracas", "fraca", "", "Le fracas est fort.")
	require.NoError(t, err)
	assert.Empty(t, alternatives, "an occurrence without a part of speech has no alternative")

	alternatives, err = index.AlternativesForLemma(t.Context(), "fr", "   ", "fraca", "NOUN", "Le fracas est fort.")
	require.NoError(t, err)
	assert.Empty(t, alternatives, "a blank surface has no alternative")
}

func TestGermanAlternativesIgnoreNonNouns(t *testing.T) {
	index := newTestIndex(t,
		`INSERT INTO entries VALUES ('de', 'haus', 'NOUN', '[{"Gloss":"house"}]', '', '', '', '', '')`,
	)

	alternatives, err := index.Alternatives(t.Context(), "de", "Haus", "VERB", "Das Haus ist groß.")

	require.NoError(t, err)
	assert.Empty(t, alternatives)
}

func TestFrenchFormOfAlternativesSkipUnusableLinks(t *testing.T) {
	index := newFrenchFormsTestIndex(t,
		`INSERT INTO entries VALUES ('fr', 'voile', 'NOUN', '[{"Gloss":"sail"}]', '', '', '', '', '', '')`,
		`INSERT INTO entries VALUES ('fr', 'voiles', 'NOUN', '[{"Gloss":"plural of voile","Tags":["form-of"]}]', '', '', '', '', '', '')`,
		`INSERT INTO french_headwords VALUES ('voile', 'NOUN')`,
		// An empty evidence id, a duplicate target, a missing target, and a
		// form-of-only target are all skipped before the valid headword link.
		`INSERT INTO french_forms VALUES ('voiles', 'NOUN', 'voile', 0, '')`,
		`INSERT INTO french_forms VALUES ('voiles', 'NOUN', 'voile', 0, 'first')`,
		`INSERT INTO french_forms VALUES ('voiles', 'NOUN', 'absent', 0, 'second')`,
		`INSERT INTO french_forms VALUES ('voiles', 'NOUN', 'voiles', 0, 'third')`,
	)

	alternatives, err := index.AlternativesForLemma(t.Context(), "fr", "voiles", "voiler", "NOUN", "Les voiles claquent.")

	require.NoError(t, err)
	require.Len(t, alternatives, 1)
	assert.Equal(t, "voile", alternatives[0].Lemma)
	assert.Equal(t, "first", alternatives[0].EvidenceID)
}

func TestFrenchSurfaceHeadwordSkipsFormOfSensesAndMissingEntries(t *testing.T) {
	index := newFrenchFormsTestIndex(t,
		`INSERT INTO entries VALUES ('fr', 'fracas', 'NOUN', '[{"Gloss":"plural of fracas","Tags":["form-of"]},{"Gloss":"fracas"}]', '', '', '', '', '', '')`,
		`INSERT INTO entries VALUES ('fr', 'incoherent', 'NOUN', '[{"Gloss":"plural of x","Tags":["form-of"]}]', '', '', '', '', '', '')`,
		`INSERT INTO french_headwords VALUES ('fracas', 'NOUN')`,
		`INSERT INTO french_headwords VALUES ('incoherent', 'NOUN')`,
		`INSERT INTO french_headwords VALUES ('ghost', 'NOUN')`,
	)

	alternatives, err := index.AlternativesForLemma(t.Context(), "fr", "fracas", "fraca", "NOUN", "Le fracas est fort.")
	require.NoError(t, err)
	require.Len(t, alternatives, 1)
	assert.Equal(t, "fracas", alternatives[0].Lemma)
	assert.NotEmpty(t, alternatives[0].EvidenceID, "the ordinary sense supplies the evidence")

	alternatives, err = index.AlternativesForLemma(t.Context(), "fr", "incoherent", "incoherent", "NOUN", "Un incoherent.")
	require.NoError(t, err)
	assert.Empty(t, alternatives, "a headword record with only form-of senses offers no alternative")

	alternatives, err = index.AlternativesForLemma(t.Context(), "fr", "ghost", "ghost", "NOUN", "Un ghost.")
	require.NoError(t, err)
	assert.Empty(t, alternatives, "a headword record with no entry offers no alternative")
}

func TestFrenchAlternativesReportUnavailableData(t *testing.T) {
	t.Run("participle lookup error", func(t *testing.T) {
		index := newRawFrenchIndex(t,
			`CREATE TABLE entries (language TEXT, lemma TEXT, upos TEXT)`,
			`INSERT INTO entries VALUES ('fr', 'mouiller', 'VERB')`,
		)

		_, err := index.AlternativesForLemma(t.Context(), "fr", "mouillé", "mouiller", "ADJ", "Le linge est mouillé.")
		require.Error(t, err)
	})
	t.Run("headword lookup error", func(t *testing.T) {
		index := newRawFrenchIndex(t,
			`CREATE TABLE entries (language TEXT, lemma TEXT, upos TEXT)`,
		)

		_, err := index.AlternativesForLemma(t.Context(), "fr", "voile", "voile", "NOUN", "La voile se gonfle.")
		require.Error(t, err)
	})
	t.Run("form-of lookup error", func(t *testing.T) {
		index := newRawFrenchIndex(t,
			`CREATE TABLE entries (language TEXT, lemma TEXT, upos TEXT)`,
			`CREATE TABLE french_headwords (lemma TEXT, upos TEXT)`,
		)

		_, err := index.AlternativesForLemma(t.Context(), "fr", "voiles", "voiler", "NOUN", "Les voiles claquent.")
		require.Error(t, err)
	})
	t.Run("form-of target lookup error", func(t *testing.T) {
		index := newRawFrenchIndex(t,
			`CREATE TABLE entries (lemma TEXT)`,
			`CREATE TABLE french_headwords (lemma TEXT, upos TEXT)`,
			`CREATE TABLE french_forms (form TEXT, upos TEXT, target TEXT, participle INTEGER, evidence_id TEXT)`,
			`INSERT INTO french_forms VALUES ('voiles', 'NOUN', 'voile', 0, 'link')`,
		)

		_, err := index.AlternativesForLemma(t.Context(), "fr", "voiles", "voiler", "NOUN", "Les voiles claquent.")
		require.Error(t, err)
	})
	t.Run("unreadable form-of row", func(t *testing.T) {
		index := newRawFrenchIndex(t,
			`CREATE TABLE entries (language TEXT, lemma TEXT, upos TEXT)`,
			`CREATE TABLE french_headwords (lemma TEXT, upos TEXT)`,
			`CREATE TABLE french_forms (form TEXT, upos TEXT, target TEXT, participle INTEGER, evidence_id TEXT)`,
			`INSERT INTO french_forms VALUES ('voiles', 'NOUN', 'voile', 0, NULL)`,
		)

		_, err := index.AlternativesForLemma(t.Context(), "fr", "voiles", "voiler", "NOUN", "Les voiles claquent.")
		require.Error(t, err)
	})
}

// newRawFrenchIndex opens a writable SQLite handle behind a French-capable
// Index, so a test can reproduce a malformed or unreadable table after open.
func newRawFrenchIndex(t *testing.T, seedStatements ...string) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	for _, statement := range seedStatements {
		_, err = db.ExecContext(t.Context(), statement)
		require.NoError(t, err)
	}
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return &Index{db: db, name: "kaikki", version: "fixture-v1", frenchFormsAvailable: true}
}

func TestAssessmentAvailability(t *testing.T) {
	old := newFrenchTestIndex(t)
	available, err := old.AssessmentAvailable(t.Context(), "de")
	require.NoError(t, err)
	assert.True(t, available, "German assessment predates the French form-of data")
	available, err = old.AssessmentAvailable(t.Context(), "fr")
	require.NoError(t, err)
	assert.False(t, available, "an older index cannot assess French")
	available, err = old.AssessmentAvailable(t.Context(), "it")
	require.NoError(t, err)
	assert.False(t, available, "Italian has no automatic assessment")

	fresh := newFrenchFormsTestIndex(t)
	available, err = fresh.AssessmentAvailable(t.Context(), "fr")
	require.NoError(t, err)
	assert.True(t, available)
}

// newFrenchFormsTestIndex opens a French format-4 index seeded with
// seedStatements.
func newFrenchFormsTestIndex(t *testing.T, seedStatements ...string) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), dictionaryFrenchFormsFixtureSchema)
	require.NoError(t, err)
	for _, statement := range seedStatements {
		_, err = db.ExecContext(t.Context(), statement)
		require.NoError(t, err)
	}
	require.NoError(t, db.Close())
	index, err := OpenIndex(t.Context(), path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, index.Close()) })
	return index
}
