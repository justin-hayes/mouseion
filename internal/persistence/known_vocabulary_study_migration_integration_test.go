//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runKnownVocabularyBackfill executes migration 81's up SQL, as the migrator
// does. The test database is already at the latest version, so the migration
// ran once on empty tables; running the file again on seeded rows is the
// supported retry.
func runKnownVocabularyBackfill(t *testing.T, ctx context.Context, store *PostgresStore) {
	t.Helper()
	script, err := migrations.FS.ReadFile("000081_known_vocabulary_to_study.up.sql")
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, string(script))
	require.NoError(t, err)
}

func backfillSnapshot(t *testing.T, ctx context.Context, store *PostgresStore) (progress, material, events []string) {
	t.Helper()
	collect := func(query string) []string {
		rows, err := store.Pool().Query(ctx, query)
		require.NoError(t, err)
		defer rows.Close()
		var out []string
		for rows.Next() {
			var row string
			require.NoError(t, rows.Scan(&row))
			out = append(out, row)
		}
		require.NoError(t, rows.Err())
		return out
	}
	return collect(`SELECT p::text FROM study_progress p ORDER BY p.id`),
		collect(`SELECT m::text FROM study_material m ORDER BY m.id`),
		collect(`SELECT e::text FROM study_already_known_event e ORDER BY e.id`)
}

// TestKnownVocabularyBackfillBecomesAlreadyKnownStudyProgress holds ticket
// #2738: every Known identity without Study progress becomes Mature
// Already-known progress with its discriminator, first reviews spread evenly
// over 42 days, existing progress untouched, a word without a Corpus context
// withdrawn, and a second run changing nothing.
func TestKnownVocabularyBackfillBecomesAlreadyKnownStudyProgress(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	alice, err := store.CreateUser(ctx, "backfill-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "backfill-bob", false)
	require.NoError(t, err)

	// Alice's Corpus: one analysed German Book whose Browse counts evidence
	// gehen and the first homograph "bank 1".
	books := seedScaleLibrary(t, ctx, store, alice.ID, scaleLibrary{books: 1, units: 1, sentencesPerUnit: 1})
	var run, corpus string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text, corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, alice.ID, books[0]).Scan(&run, &corpus))
	for _, word := range [][3]string{{"gehen", "VERB", ""}, {"bank", "NOUN", "1"}} {
		_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,lexical_discriminator,occurrence_count) VALUES($1,$2,'de',$3,$4,$5,$6,$7,3)`,
			alice.ID, books[0], run, corpus, word[0], word[1], word[2])
		require.NoError(t, err)
	}

	// Known vocabulary: homographs, a context-less word, a lemma-wide row that
	// names no word, another language, another owner, and "haus", which is
	// already in study.
	_, err = store.Pool().Exec(ctx, `INSERT INTO known_vocabulary(owner_id, language, canonical_lemma, upos, lexical_discriminator) VALUES
		($1,'de','gehen','VERB',''), ($1,'de','bank','NOUN','1'), ($1,'de','bank','NOUN','2'), ($1,'de','kuchen','NOUN',''),
		($1,'de','ding','',''), ($1,'it','casa','NOUN',''), ($1,'de','haus','NOUN',''), ($2,'de','gehen','VERB','')`, alice.ID, bob.ID)
	require.NoError(t, err)
	var existingMaterial string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO study_material(owner_id, language, canonical_lemma, upos, state, omission_reason) VALUES($1,'de','haus','NOUN','unpreparable','test') RETURNING id::text`, alice.ID).Scan(&existingMaterial))
	existingDue := time.Date(2031, 1, 2, 3, 4, 5, 0, time.UTC)
	_, err = store.Pool().Exec(ctx, `INSERT INTO study_progress(owner_id, language, canonical_lemma, upos, material_id, introduced_at, learned_meaning, state, due, reps, scheduled_days)
		VALUES($1,'de','haus','NOUN',$2,'2030-01-01','house','learning',$3,2,1)`, alice.ID, existingMaterial, existingDue)
	require.NoError(t, err)
	var existingBefore string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT p::text FROM study_progress p WHERE canonical_lemma='haus'`).Scan(&existingBefore))

	runKnownVocabularyBackfill(t, ctx, store)

	progress, err := store.ListStudyProgress(ctx, alice.ID, "de")
	require.NoError(t, err)
	byWord := map[string]domain.StudyProgress{}
	for _, p := range progress {
		byWord[p.CanonicalLemma+"|"+p.LexicalDiscriminator] = p
	}
	require.Len(t, byWord, 5, "haus plus the four Known words with a word identity")
	assert.NotContains(t, byWord, "ding|", "a lemma-wide Known row names no word")

	var existingAfter string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT p::text FROM study_progress p WHERE canonical_lemma='haus'`).Scan(&existingAfter))
	assert.Equal(t, existingBefore, existingAfter, "existing progress is never touched")

	var at time.Time
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT min(answered_at) FROM study_already_known_event`).Scan(&at))
	// Alice's four German words, ordered by identity, are due i/4 of 42 days
	// after the backfill time.
	wantUPOS := map[string]string{"bank|1": "NOUN", "bank|2": "NOUN", "gehen|": "VERB", "kuchen|": "NOUN"}
	for index, key := range []string{"bank|1", "bank|2", "gehen|", "kuchen|"} {
		p, ok := byWord[key]
		require.True(t, ok, key)
		assert.Equal(t, domain.StudyStateReview, p.Memory.State, key)
		assert.EqualValues(t, domain.StudyAlreadyKnownDays, p.Memory.ScheduledDays, key)
		assert.EqualValues(t, 1, p.Memory.Reps, key)
		assert.Equal(t, wantUPOS[key], p.UPOS, key)
		assert.Equal(t, p.CanonicalLemma, p.LearnedMeaning, key)
		wantOffset := time.Duration(float64(42*24*time.Hour) * float64(index) / 4)
		assert.InDelta(t, wantOffset.Seconds(), p.Memory.Due.Sub(at).Seconds(), 1, "%s is due %v after the backfill", key, wantOffset)
		assert.InDelta(t, 21*24*time.Hour.Seconds(), p.Memory.Due.Sub(p.Memory.LastReview).Seconds(), 1, key)
	}
	assert.True(t, domain.IsStudyMature(byWord["gehen|"]), "a word with a Corpus context is Mature at once")
	assert.True(t, domain.IsStudyMature(byWord["bank|1"]))
	assert.False(t, byWord["gehen|"].Withdrawn)
	assert.False(t, byWord["bank|1"].Withdrawn)
	assert.True(t, byWord["bank|1"].Reintroducing, "the placeholder material is replaced from a context")
	for _, key := range []string{"bank|2", "kuchen|"} {
		assert.True(t, byWord[key].Withdrawn, "%s has no Corpus context, so it is withdrawn", key)
		assert.False(t, domain.IsStudyMature(byWord[key]), key)
	}

	events, err := store.ListStudyAlreadyKnownEvents(ctx, alice.ID, "de")
	require.NoError(t, err)
	require.Len(t, events, 4)
	for _, event := range events {
		assert.Equal(t, domain.StudyAlreadyKnownBackfill, event.Source)
		assert.NotEmpty(t, event.ProgressID)
	}
	var reviewLog int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM study_review_log`).Scan(&reviewLog))
	assert.Zero(t, reviewLog, "an Already-known backfill is never a grade")

	italian, err := store.ListStudyProgress(ctx, alice.ID, "it")
	require.NoError(t, err)
	require.Len(t, italian, 1)
	assert.True(t, italian[0].Withdrawn, "a word of a language with no Corpus is withdrawn")
	bobs, err := store.ListStudyProgress(ctx, bob.ID, "de")
	require.NoError(t, err)
	require.Len(t, bobs, 1)
	assert.Zero(t, bobs[0].Memory.Due.Sub(at).Round(time.Second), "an owner's first word is due at the backfill time")
	assert.True(t, bobs[0].Withdrawn, "another owner's Corpus does not evidence the word")

	// A second run changes nothing.
	progressBefore, materialBefore, eventsBefore := backfillSnapshot(t, ctx, store)
	runKnownVocabularyBackfill(t, ctx, store)
	progressAfter, materialAfter, eventsAfter := backfillSnapshot(t, ctx, store)
	assert.Equal(t, progressBefore, progressAfter)
	assert.Equal(t, materialBefore, materialAfter)
	assert.Equal(t, eventsBefore, eventsAfter)

	// The down migration removes exactly what the backfill wrote, and the
	// backfill can then be run again.
	down, err := migrations.FS.ReadFile("000081_known_vocabulary_to_study.down.sql")
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, string(down))
	require.NoError(t, err)
	var remaining, placeholders int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT (SELECT count(*) FROM study_progress), (SELECT count(*) FROM study_material WHERE omission_reason='known_vocabulary_backfill')`).Scan(&remaining, &placeholders))
	assert.Equal(t, 1, remaining, "only haus keeps its progress")
	assert.Zero(t, placeholders)
	runKnownVocabularyBackfill(t, ctx, store)
	progressRerun, _, _ := backfillSnapshot(t, ctx, store)
	assert.Len(t, progressRerun, len(progressAfter))
}
