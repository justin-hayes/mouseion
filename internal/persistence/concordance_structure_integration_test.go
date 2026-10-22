//go:build integration

package persistence

import (
	"context"
	"testing"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcordanceAndSentenceStudyApplySpeakerLabelRestriction holds ADR 0103's
// Study-context rule through the real reads: a speaker-label occurrence stays
// searchable and is labelled, its whole sentence offers no Study, a standalone
// direction sentence remains available, and a v1 snapshot derives no
// restriction because structure was never recorded.
func TestConcordanceAndSentenceStudyApplySpeakerLabelRestriction(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "speaker-label-study", false)
	require.NoError(t, err)

	const text = "Valentin fällt, das Buch liegt hier. Das Buch liegt hier."
	unit := concordanceUnit(0, "chapter", text, 0, uint64(len([]rune(text))))
	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Poetry", "speaker-label", false, []domain.ExtractedUnit{unit})
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{
		{UnitID: unit.ID, Ordinal: 0, Text: "Valentin fällt, das Buch liegt hier.", Start: 0, End: 36, Tokens: []concordanceToken{
			{Surface: "Valentin", Lemma: "Valentin", Upos: "PROPN", Start: 0, End: 8},
			{Surface: "fällt", Lemma: "fallen", Upos: "VERB", Start: 9, End: 14},
			{Surface: "das", Lemma: "der", Upos: "DET", Start: 16, End: 19},
			{Surface: "Buch", Lemma: "Buch", Upos: "NOUN", Start: 20, End: 24},
			{Surface: "liegt", Lemma: "liegen", Upos: "VERB", Start: 25, End: 30},
		}},
		{UnitID: unit.ID, Ordinal: 1, Text: "Das Buch liegt hier.", Start: 37, End: 57, Tokens: []concordanceToken{
			{Surface: "Das", Lemma: "der", Upos: "DET", Start: 37, End: 40},
			{Surface: "Buch", Lemma: "Buch", Upos: "NOUN", Start: 41, End: 45},
			{Surface: "liegt", Lemma: "liegen", Upos: "VERB", Start: 46, End: 51},
		}},
	})
	var runID, corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text, corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID, &corpusID))

	// A v1 snapshot has no recorded structure. The same evidence remains
	// studyable exactly as before.
	v1Lookup, err := store.ListVocabularyConcordance(ctx, owner.ID, "de", domain.ConcordanceLookup{Term: "fällt", Match: domain.ConcordanceMatchForm, Page: 1})
	require.NoError(t, err)
	require.Len(t, v1Lookup.Occurrences, 1)
	assert.False(t, v1Lookup.Occurrences[0].SpeakerLabel)
	assert.False(t, v1Lookup.Occurrences[0].StudyUnavailable, "v1 records structure not recorded, so no restriction is derived")
	v1Study, err := store.GetVocabularySentenceStudy(ctx, owner.ID, book.ID, runID, corpusID, unit.ID, 0, 1, "fällt")
	require.NoError(t, err)
	assert.False(t, v1Study.SpeakerLabelOverlap)
	assert.Empty(t, v1Study.Attribution, "v1 records no structure, so there is no attribution to show")

	// Switch the same revision to a v2 snapshot that records a speaker label
	// over the first sentence and a standalone direction over the second.
	snapshotID := insertStructureSnapshot(t, ctx, store.Pool(), owner.ID, source.ID, source.ContentRevisionID, domain.StructureConventionSet, []domain.ExtractedUnit{unit})
	require.NoError(t, insertRoleSpan(t, ctx, store.Pool(), owner.ID, source.ID, snapshotID, unit.ID, "speaker", 0, 15))
	require.NoError(t, insertRoleSpan(t, ctx, store.Pool(), owner.ID, source.ID, snapshotID, unit.ID, "stage_direction", 37, 57))
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET snapshot_id=$3 WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID, snapshotID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE source_materials SET current_snapshot_id=$3 WHERE owner_id=$1 AND id=$2`, owner.ID, source.ID, snapshotID)
	require.NoError(t, err)

	// The speaker label is still found and is labelled as evidence.
	labeled, err := store.ListVocabularyConcordance(ctx, owner.ID, "de", domain.ConcordanceLookup{Term: "fällt", Match: domain.ConcordanceMatchForm, Page: 1})
	require.NoError(t, err)
	require.Len(t, labeled.Occurrences, 1)
	assert.True(t, labeled.Occurrences[0].SpeakerLabel)
	assert.True(t, labeled.Occurrences[0].StudyUnavailable)
	assert.Equal(t, "Valentin fällt, das Buch liegt hier.", labeled.Occurrences[0].SentenceText, "the complete sentence is kept unchanged")

	// A sentence overlapping a label supplies no Study even when the searched
	// occurrence is an eligible direction word outside the label itself.
	books, err := store.ListVocabularyConcordance(ctx, owner.ID, "de", domain.ConcordanceLookup{Term: "Buch", Match: domain.ConcordanceMatchForm, Page: 1})
	require.NoError(t, err)
	require.Len(t, books.Occurrences, 2)
	bySentence := map[int64]domain.ConcordanceResultOccurrence{}
	for _, occurrence := range books.Occurrences {
		bySentence[occurrence.SentenceOrdinal] = occurrence
	}
	assert.True(t, bySentence[0].StudyUnavailable)
	assert.False(t, bySentence[0].SpeakerLabel, "the direction word itself is not the speaker label")
	assert.False(t, bySentence[1].StudyUnavailable, "a standalone direction sentence remains study context")
	assert.False(t, bySentence[1].SpeakerLabel)
	assert.Equal(t, domain.SourceAttribution{Kind: domain.AttributionStageDirection}, bySentence[1].Attribution,
		"the expanded context names a direction sentence")

	labeledStudy, err := store.GetVocabularySentenceStudy(ctx, owner.ID, book.ID, runID, corpusID, unit.ID, 0, 1, "fällt")
	require.NoError(t, err)
	assert.True(t, labeledStudy.SpeakerLabelOverlap)
	directionStudy, err := store.GetVocabularySentenceStudy(ctx, owner.ID, book.ID, runID, corpusID, unit.ID, 1, 1, "Buch")
	require.NoError(t, err)
	assert.False(t, directionStudy.SpeakerLabelOverlap, "the standalone direction sentence is still studyable")
	assert.Equal(t, domain.SourceAttribution{Kind: domain.AttributionStageDirection}, directionStudy.Attribution)
}

// TestSentenceStudyAttributionShowsOnlyTheSpeakerName derives the quiet
// speaker metadata from the exact snapshot: the nearest preceding label wins,
// a nested direction is omitted, and the slot is absent when no label applies.
func TestSentenceStudyAttributionShowsOnlyTheSpeakerName(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "sentence-attribution", false)
	require.NoError(t, err)

	const text = "Mephistopheles für sich. Das Buch liegt hier."
	unit := concordanceUnit(0, "chapter", text, 0, uint64(len([]rune(text))))
	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Poetry", "sentence-attribution", false, []domain.ExtractedUnit{unit})
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{{
		UnitID: unit.ID, Ordinal: 0, Text: "Das Buch liegt hier.", Start: 25, End: 45,
		Tokens: []concordanceToken{
			{Surface: "Das", Lemma: "der", Upos: "DET", Start: 25, End: 28},
			{Surface: "Buch", Lemma: "Buch", Upos: "NOUN", Start: 29, End: 33},
			{Surface: "liegt", Lemma: "liegen", Upos: "VERB", Start: 34, End: 39},
			{Surface: "hier", Lemma: "hier", Upos: "ADV", Start: 40, End: 44},
			{Surface: ".", Lemma: ".", Upos: "PUNCT", Start: 44, End: 45},
		},
	}})
	var runID, corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text, corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID, &corpusID))

	snapshotID := insertStructureSnapshot(t, ctx, store.Pool(), owner.ID, source.ID, source.ContentRevisionID, domain.StructureConventionSet, []domain.ExtractedUnit{unit})
	require.NoError(t, insertRoleSpan(t, ctx, store.Pool(), owner.ID, source.ID, snapshotID, unit.ID, "speaker", 0, 24))
	require.NoError(t, insertRoleSpan(t, ctx, store.Pool(), owner.ID, source.ID, snapshotID, unit.ID, "stage_direction", 15, 23))
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET snapshot_id=$3 WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID, snapshotID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE source_materials SET current_snapshot_id=$3 WHERE owner_id=$1 AND id=$2`, owner.ID, source.ID, snapshotID)
	require.NoError(t, err)

	study, err := store.GetVocabularySentenceStudy(ctx, owner.ID, book.ID, runID, corpusID, unit.ID, 0, 1, "Buch")
	require.NoError(t, err)
	assert.Equal(t, domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Mephistopheles"}, study.Attribution,
		"the nested direction is omitted and the terminal period is trimmed")

	occurrences, err := store.ListVocabularyConcordance(ctx, owner.ID, "de", domain.ConcordanceLookup{Term: "Buch", Match: domain.ConcordanceMatchForm, Page: 1})
	require.NoError(t, err)
	require.Len(t, occurrences.Occurrences, 1)
	assert.Equal(t, domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Mephistopheles"}, occurrences.Occurrences[0].Attribution,
		"the expanded context carries the same quiet attribution")
}

// TestStructureLoadersReportMissingSnapshotAndRun keeps the new loaders'
// failure paths honest: a missing exact snapshot is unavailable, not "no
// labels", and a missing run is an error rather than an empty snapshot.
func TestStructureLoadersReportMissingSnapshotAndRun(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "structure-loader-missing", false)
	require.NoError(t, err)
	source := putStructureSource(t, ctx, store, owner.ID, "loader-missing")
	const missing = "00000000-0000-0000-0000-000000000000"

	_, err = LoadSnapshotStructure(ctx, store.Pool(), owner.ID, source.ID, missing)
	require.ErrorIs(t, err, domain.ErrExtractedUnitsUnavailable)

	_, err = loadConcordanceStructures(ctx, store.Pool(), owner.ID, []sqlcgen.ListVocabularyConcordanceRow{{
		SourceMaterialID: source.ID, SnapshotID: missing,
	}})
	require.ErrorIs(t, err, domain.ErrExtractedUnitsUnavailable)

	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback(ctx)) }()
	_, err = analysisRunSnapshotID(ctx, tx, owner.ID, missing)
	require.ErrorContains(t, err, "load analysis run snapshot")
}
