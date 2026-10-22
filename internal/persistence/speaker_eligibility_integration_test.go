//go:build integration

package persistence

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// speakerTestToken describes one surface word of a handwritten test sentence.
type speakerTestToken struct {
	surface, lemma, upos, dependency string
}

// speakerTestSentence lays out words in source order and records their
// unit-relative offsets. A token whose surface repeats an earlier word is
// matched after the previous token, so offsets always advance.
func speakerTestSentence(t *testing.T, text string, start int64, words []speakerTestToken) concordanceSentence {
	t.Helper()
	tokens := make([]concordanceToken, 0, len(words))
	searchFrom := 0
	for _, word := range words {
		index := strings.Index(text[searchFrom:], word.surface)
		require.GreaterOrEqual(t, index, 0, "word %q does not follow its predecessor in %q", word.surface, text)
		byteStart := searchFrom + index
		tokenStart := start + int64(utf8.RuneCountInString(text[:byteStart]))
		tokenEnd := tokenStart + int64(utf8.RuneCountInString(word.surface))
		tokens = append(tokens, concordanceToken{Surface: word.surface, Lemma: word.lemma, Upos: word.upos, Dependency: word.dependency, Start: tokenStart, End: tokenEnd})
		searchFrom = byteStart + len(word.surface)
	}
	return concordanceSentence{Text: text, Start: start, End: start + int64(utf8.RuneCountInString(text)), Tokens: tokens}
}

// seededSpeakerSentence converts one shared gate-passing sentence into a corpus
// sentence whose token offsets are absolute in the unit.
func seededSpeakerSentence(t *testing.T, start int64, seeded testutil.SeededSentence) concordanceSentence {
	t.Helper()
	tokens := make([]concordanceToken, 0, len(seeded.Tokens))
	for _, token := range seeded.Tokens {
		tokenStart, err := checked.Int64FromUint64(token.Location.StartOffset)
		require.NoError(t, err)
		tokenEnd, err := checked.Int64FromUint64(token.Location.EndOffset)
		require.NoError(t, err)
		tokens = append(tokens, concordanceToken{
			Surface: token.Surface, Lemma: token.CanonicalLemma, Upos: token.UPOS, Dependency: token.Dependency,
			Head: int64(token.Head), Morphology: token.Morphology,
			Start: start + tokenStart, End: start + tokenEnd,
		})
	}
	return concordanceSentence{Text: seeded.Text, Start: start, End: start + int64(utf8.RuneCountInString(seeded.Text)), Tokens: tokens}
}

// speakerAnalysisFacts returns the persisted analysis projection and the exact
// snapshot structure the production consumers pass to selection. It also returns
// the coverage statistics selection derives from those facts.
func speakerAnalysisFacts(t *testing.T, ctx context.Context, store *PostgresStore, owner, sourceID, runID, corpusID string) (domain.SnapshotStructure, domain.AnalysisStatistics) {
	t.Helper()
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, txcleanup.Rollback(ctx, tx)) }()
	analysis, _, err := loadAnalysisProjectionFactsTx(ctx, tx, sqlcgen.New(tx), domain.DeckPreparation{OwnerID: owner, SourceMaterialID: sourceID, AnalysisRunID: runID}, corpusID)
	require.NoError(t, err)
	snapshotID, err := analysisRunSnapshotID(ctx, tx, owner, runID)
	require.NoError(t, err)
	structure, err := LoadSnapshotStructure(ctx, tx, owner, sourceID, snapshotID)
	require.NoError(t, err)
	cfg := selection.DefaultConfig(corpusID)
	cfg.Structure = structure
	return structure, selection.AnalyzableStatistics(analysis, cfg)
}

// TestSpeakerLabelsAreStructuralNonVocabularyAcrossCountConsumers proves that
// exact-snapshot speaker spans exclude label tokens from the projection, Browse,
// coverage and the Reading freeze, while standalone direction words still count
// and the raw analyzer evidence remains.
func TestSpeakerLabelsAreStructuralNonVocabularyAcrossCountConsumers(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "speaker-counts", false)
	require.NoError(t, err)

	// One unit carries the Valentin label with a nested direction, an eligible
	// standalone direction sentence, a mixed-role paragraph with an inline
	// speaker label, and three gate-passing sentences so one identity reaches the
	// Reading freeze floor.
	first := speakerTestSentence(t, "Valentin fällt.", 0, []speakerTestToken{
		{surface: "Valentin", lemma: "Valentin", upos: "PROPN", dependency: "nsubj"},
		{surface: "fällt", lemma: "fallen", upos: "VERB", dependency: "root"},
	})
	direction := speakerTestSentence(t, "Er schlägt das Buch auf und lacht.", first.End+1, []speakerTestToken{
		{surface: "Er", lemma: "er", upos: "PRON", dependency: "nsubj"},
		{surface: "schlägt", lemma: "schlagen", upos: "VERB", dependency: "root"},
		{surface: "das", lemma: "der", upos: "DET", dependency: "det"},
		{surface: "Buch", lemma: "Buch", upos: "NOUN", dependency: "obj"},
		{surface: "auf", lemma: "auf", upos: "ADV", dependency: "advmod"},
		{surface: "und", lemma: "und", upos: "CCONJ", dependency: "cc"},
		{surface: "lacht", lemma: "lachen", upos: "VERB", dependency: "conj"},
	})
	mixed := speakerTestSentence(t, "Mephistopheles mit seltsamen Geberden.", direction.End+1, []speakerTestToken{
		{surface: "Mephistopheles", lemma: "Mephistopheles", upos: "PROPN", dependency: "nsubj"},
		{surface: "mit", lemma: "mit", upos: "ADP", dependency: "case"},
		{surface: "seltsamen", lemma: "seltsam", upos: "ADJ", dependency: "amod"},
		{surface: "Geberden", lemma: "Geberde", upos: "NOUN", dependency: "obl"},
	})
	bookWord := testutil.GatePassingSentence("de", testutil.CorpusWord{Lemma: "Buch", UPOS: "NOUN"})
	gateSentences := []concordanceSentence{
		seededSpeakerSentence(t, mixed.End+1, bookWord),
		seededSpeakerSentence(t, mixed.End+1+int64(utf8.RuneCountInString(bookWord.Text))+1, bookWord),
		seededSpeakerSentence(t, mixed.End+1+2*int64(utf8.RuneCountInString(bookWord.Text))+2, bookWord),
	}
	sentences := append([]concordanceSentence{first, direction, mixed}, gateSentences...)
	textParts := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		textParts = append(textParts, sentence.Text)
	}
	text := strings.Join(textParts, "\n")
	textLength, err := checked.Uint64FromInt(utf8.RuneCountInString(text))
	require.NoError(t, err)
	unit := concordanceUnit(0, "speaker-v2", text, 0, textLength)
	for index := range sentences {
		sentences[index].UnitID = unit.ID
		sentences[index].Ordinal = int64(index)
	}
	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Speaker counts", "speaker-v2", true, []domain.ExtractedUnit{unit})

	// The writer still emits v1; this test installs the v2 snapshot and role
	// spans a future writer will create, then points the source at it.
	pool := store.Pool()
	snapshotID := insertStructureSnapshot(t, ctx, pool, owner.ID, source.ID, source.ContentRevisionID, domain.StructureConventionSet, []domain.ExtractedUnit{unit})
	_, err = pool.Exec(ctx, `UPDATE source_materials SET current_snapshot_id=$1 WHERE owner_id=$2 AND id=$3`, snapshotID, owner.ID, source.ID)
	require.NoError(t, err)
	valentinEnd := int(first.End)
	require.NoError(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, snapshotID, unit.ID, string(domain.SourceStructureRoleSpeaker), 0, valentinEnd))
	nestedStart := valentinEnd - utf8.RuneCountInString("fällt.")
	require.NoError(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, snapshotID, unit.ID, string(domain.SourceStructureRoleDirection), nestedStart, valentinEnd))
	mephistophelesStart := int(mixed.Start)
	require.NoError(t, insertRoleSpan(t, ctx, pool, owner.ID, source.ID, snapshotID, unit.ID, string(domain.SourceStructureRoleSpeaker), mephistophelesStart, mephistophelesStart+utf8.RuneCountInString("Mephistopheles")))

	insertConcordanceAnalysis(t, ctx, store, source, true, sentences)
	var runID, corpusID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID, &corpusID))

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, BuildVocabularyBrowseCountsTx(ctx, tx, owner.ID, book.ID, source.ID, runID, corpusID, "de"))
	require.NoError(t, tx.Commit(ctx))

	structure, statistics := speakerAnalysisFacts(t, ctx, store, owner.ID, source.ID, runID, corpusID)
	require.Len(t, structure.RoleSpans, 3, "the fixture records two speaker labels and their nested direction")
	speakerLabels := 0
	for _, span := range structure.RoleSpans {
		if span.Role == domain.SourceStructureRoleSpeaker {
			speakerLabels++
		}
	}
	assert.Equal(t, 2, speakerLabels, "only speaker spans exclude tokens; the nested direction is inside its label")
	spanTx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = LoadSnapshotStructure(ctx, spanTx, owner.ID, source.ID, "00000000-0000-0000-0000-000000000000")
	require.Error(t, err)
	require.NoError(t, spanTx.Rollback(ctx))
	assert.Equal(t, int64(9), statistics.AnalyzableTokenCount, "label tokens are not in the coverage denominator")
	assert.Equal(t, int64(6), statistics.DistinctLemmaCount)
	_, err = pool.Exec(ctx, `UPDATE corpora SET analyzable_token_count=$3, distinct_lemma_count=$4 WHERE owner_id=$1 AND id=$2`, owner.ID, corpusID, statistics.AnalyzableTokenCount, statistics.DistinctLemmaCount)
	require.NoError(t, err)

	want := map[string]int64{
		"Buch/NOUN": 4, "schlagen/VERB": 1, "auf/ADV": 1, "lachen/VERB": 1, "seltsam/ADJ": 1, "Geberde/NOUN": 1,
	}
	assert.Equal(t, want, browseCountRows(t, ctx, store, owner.ID, book.ID), "the projection excludes the label and keeps standalone directions")
	page, err := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: book.ID, IncludeAll: true, Page: 1})
	require.NoError(t, err)
	assert.False(t, page.BrowseCountsUpdating)
	browse := make(map[string]int64, len(page.Rows))
	for _, row := range page.Rows {
		browse[row.CanonicalLemma+"/"+row.UPOS] = row.OccurrenceCount
	}
	assert.Equal(t, want, browse, "Browse reads the same exclusion as the projection")

	coverage, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, corpusID)
	require.NoError(t, err)
	assert.Equal(t, int64(9), coverage.AnalyzableTokenCount, "coverage agrees on the same denominator")
	assert.Equal(t, int64(6), coverage.DistinctLemmaCount)

	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	frozen, err := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	require.Len(t, frozen, 1, "only the identity that reaches the Reading floor freezes")
	assert.Equal(t, "Buch", frozen[0].CanonicalLemma)
	assert.Equal(t, 4, frozen[0].OccurrenceCount)
	assert.NotContains(t, string(frozen[0].SentenceReferences), "Valentin", "the label stays raw corpus evidence, never frozen context")

	// A projection rebuild never mutates the already-frozen Reading evidence.
	tx, err = pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, BuildVocabularyBrowseCountsTx(ctx, tx, owner.ID, book.ID, source.ID, runID, corpusID, "de"))
	require.NoError(t, tx.Commit(ctx))
	reloaded, err := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, frozen, reloaded)
}

// TestV1CountsAndFrozenReadingStayPinnedAcrossSpeakerAwareRebuild proves that a
// v1 snapshot (structure not recorded) keeps its ordinary counts and an existing
// frozen Reading when the speaker-aware projection version rebuilds it.
func TestV1CountsAndFrozenReadingStayPinnedAcrossSpeakerAwareRebuild(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "speaker-v1-pinned", false)
	require.NoError(t, err)

	label := speakerTestSentence(t, "Valentin fällt.", 0, []speakerTestToken{
		{surface: "Valentin", lemma: "Valentin", upos: "PROPN", dependency: "nsubj"},
		{surface: "fällt", lemma: "fallen", upos: "VERB", dependency: "root"},
	})
	second := speakerTestSentence(t, "Valentin fällt.", label.End+1, []speakerTestToken{
		{surface: "Valentin", lemma: "Valentin", upos: "PROPN", dependency: "nsubj"},
		{surface: "fällt", lemma: "fallen", upos: "VERB", dependency: "root"},
	})
	third := speakerTestSentence(t, "Valentin fällt.", second.End+1, []speakerTestToken{
		{surface: "Valentin", lemma: "Valentin", upos: "PROPN", dependency: "nsubj"},
		{surface: "fällt", lemma: "fallen", upos: "VERB", dependency: "root"},
	})
	bookWord := testutil.GatePassingSentence("de", testutil.CorpusWord{Lemma: "fallen", UPOS: "VERB"})
	sentences := []concordanceSentence{label, second, third}
	for range 3 {
		start := sentences[len(sentences)-1].End + 1
		sentences = append(sentences, seededSpeakerSentence(t, start, bookWord))
	}
	textParts := make([]string, 0, len(sentences))
	for _, sentence := range sentences {
		textParts = append(textParts, sentence.Text)
	}
	text := strings.Join(textParts, "\n")
	textLength, err := checked.Uint64FromInt(utf8.RuneCountInString(text))
	require.NoError(t, err)
	unit := concordanceUnit(0, "speaker-v1-pinned", text, 0, textLength)
	for index := range sentences {
		sentences[index].UnitID = unit.ID
		sentences[index].Ordinal = int64(index)
	}
	book, source := createConcordanceBook(t, ctx, store, owner.ID, "V1 pinned", "speaker-v1-pinned", true,
		[]domain.ExtractedUnit{unit})
	insertConcordanceAnalysis(t, ctx, store, source, true, sentences)
	var runID, corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID, &corpusID))

	rebuild := func() {
		tx, beginErr := store.Pool().Begin(ctx)
		require.NoError(t, beginErr)
		require.NoError(t, BuildVocabularyBrowseCountsTx(ctx, tx, owner.ID, book.ID, source.ID, runID, corpusID, "de"))
		require.NoError(t, tx.Commit(ctx))
	}
	rebuild()
	want := map[string]int64{"fallen/VERB": 6}
	assert.Equal(t, want, browseCountRows(t, ctx, store, owner.ID, book.ID), "v1 records no structure, so labels contribute ordinary vocabulary")

	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	frozen, err := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	require.Len(t, frozen, 1)
	assert.Equal(t, "fallen", frozen[0].CanonicalLemma)
	assert.Equal(t, 6, frozen[0].OccurrenceCount)

	rebuild()
	assert.Equal(t, want, browseCountRows(t, ctx, store, owner.ID, book.ID), "a rebuild leaves v1 counts identical")
	reloaded, err := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, frozen, reloaded, "the frozen Reading stays pinned across a projection rebuild")
}
