//go:build integration

package persistence

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcordanceOccurrencesAreCurrentOwnerScopedAndDeterministic(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))

	alice, err := store.CreateUser(ctx, "concordance-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "concordance-bob", false)
	require.NoError(t, err)

	bookA, sourceA := createConcordanceBook(t, ctx, store, alice.ID, "Book A", "a", true, []domain.ExtractedUnit{
		concordanceUnit(0, "intro", "Intro", 0, 5),
		concordanceUnit(1, "chapter-a", "xxDas Haus", 7, 17),
	})
	insertConcordanceAnalysis(t, ctx, store, sourceA, true, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:intro", Ordinal: 0, Text: "Intro", Start: 0, End: 5},
		{UnitID: "epub-unit-v1:1:chapter-a", Ordinal: 1, Text: "Das Haus", Start: 2, End: 10, Tokens: []concordanceToken{
			{Surface: "Das", Lemma: "das", Upos: "DET", Dependency: "det", Head: 1, Start: 2, End: 5},
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Dependency: "obj", Head: 0, Start: 6, End: 10},
		}},
	})

	bookB, sourceB := createConcordanceBook(t, ctx, store, alice.ID, "Book B", "b", true, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter-b", "Haus", 0, 4),
	})
	insertConcordanceAnalysis(t, ctx, store, sourceB, true, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter-b", Ordinal: 0, Text: "Haus", Start: 0, End: 4, Tokens: []concordanceToken{{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4}}},
	})

	// This is a complete analysis artifact, but it is not the Book's current
	// analysis, so it must not contribute to study-language results.
	_, sourceNotCurrent := createConcordanceBook(t, ctx, store, alice.ID, "Book Not Current", "not-current", false, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter-old", "Haus", 0, 4),
	})
	insertConcordanceAnalysis(t, ctx, store, sourceNotCurrent, false, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter-old", Ordinal: 0, Text: "Haus", Start: 0, End: 4, Tokens: []concordanceToken{{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4}}},
	})

	bookBob, sourceBob := createConcordanceBook(t, ctx, store, bob.ID, "Bob's Book", "bob", false, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter-bob", "Haus", 0, 4),
	})
	insertConcordanceAnalysis(t, ctx, store, sourceBob, true, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter-bob", Ordinal: 0, Text: "Haus", Start: 0, End: 4, Tokens: []concordanceToken{{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4}}},
	})

	var dependency, headSurface string
	var headOrdinal int64
	err = store.Pool().QueryRow(ctx, `
		SELECT dependency,head_ordinal,head_surface
		FROM concordance_occurrences
		WHERE owner_id=$1 AND book_id=$2 AND language='de' AND canonical_lemma='haus' AND upos='NOUN'`, alice.ID, bookA.ID).
		Scan(&dependency, &headOrdinal, &headSurface)
	require.NoError(t, err)
	assert.Equal(t, "obj", dependency)
	assert.Equal(t, int64(0), headOrdinal)
	assert.Equal(t, "Das", headSurface)

	lemmaLookup, err := store.ListVocabularyConcordance(ctx, alice.ID, "de", domain.ConcordanceLookup{Term: "Haus", Page: 1})
	require.NoError(t, err)
	assert.Equal(t, domain.ConcordanceMatchLemma, lemmaLookup.Match)
	require.Len(t, lemmaLookup.Occurrences, 2)
	assert.Equal(t, bookA.ID, lemmaLookup.Occurrences[0].BookID)
	assert.Equal(t, bookB.ID, lemmaLookup.Occurrences[1].BookID)
	dasLookup, err := store.ListVocabularyConcordance(ctx, alice.ID, "de", domain.ConcordanceLookup{Term: "Das", Page: 1})
	require.NoError(t, err)
	assert.Equal(t, domain.ConcordanceMatchLemma, dasLookup.Match, "das is an evidenced lemma")
	targetOccurrence := lemmaLookup.Occurrences[0]
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version) VALUES($1,$2,$3,$4,$5,2,5,'der','de','1')`,
		alice.ID, bookA.ID, targetOccurrence.CorpusID, targetOccurrence.AnalysisRunID, targetOccurrence.UnitID)
	require.NoError(t, err)
	correctedLemma, err := store.ListVocabularyConcordance(ctx, alice.ID, "de", domain.ConcordanceLookup{Term: "der", Page: 1})
	require.NoError(t, err)
	assert.Equal(t, domain.ConcordanceMatchLemma, correctedLemma.Match)
	require.Len(t, correctedLemma.Occurrences, 1)
	assert.Equal(t, "Das", correctedLemma.Occurrences[0].Surface, "the correction attributes the occurrence to the corrected lemma")
	uncorrectedForm, err := store.ListVocabularyConcordance(ctx, alice.ID, "de", domain.ConcordanceLookup{Term: "das", Match: domain.ConcordanceMatchForm, Page: 1})
	require.NoError(t, err)
	require.Len(t, uncorrectedForm.Occurrences, 1, "surface lookup retains unchanged source matching after correction")

	governorDependents := lemmaLookup
	study, err := store.GetVocabularySentenceStudy(ctx, alice.ID, bookA.ID,
		governorDependents.Occurrences[0].AnalysisRunID, governorDependents.Occurrences[0].CorpusID,
		governorDependents.Occurrences[0].UnitID, governorDependents.Occurrences[0].SentenceOrdinal,
		governorDependents.Occurrences[0].TokenOrdinal, governorDependents.Occurrences[0].Surface)
	require.NoError(t, err)
	assert.Equal(t, "Das Haus", study.SentenceText)
	assert.Equal(t, int64(1), study.TargetOrdinal)
	assert.Len(t, study.Tokens, 2)
	assert.True(t, study.Tokens[0].Corrected)
	assert.Equal(t, "der", study.Tokens[0].EffectiveLemma)
	assert.Equal(t, "obj", study.Tokens[1].Dependency)
	_, err = store.Pool().Exec(ctx, `DELETE FROM corpus_tokens WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3 AND sentence_ordinal=1`, alice.ID, governorDependents.Occurrences[0].CorpusID, governorDependents.Occurrences[0].AnalysisRunID)
	// Missing parse rows do not invalidate a complete sentence source.
	require.NoError(t, err)
	parseGap, err := store.GetVocabularySentenceStudy(ctx, alice.ID, bookA.ID,
		governorDependents.Occurrences[0].AnalysisRunID, governorDependents.Occurrences[0].CorpusID,
		governorDependents.Occurrences[0].UnitID, 1, 0, "Haus")
	require.NoError(t, err)
	assert.Equal(t, "Das Haus", parseGap.SentenceText)
	assert.Empty(t, parseGap.Tokens)
	assert.Equal(t, "Haus", parseGap.TargetSurface)
	require.Len(t, parseGap.UnresolvedDecisions, 1, "a decision with no matching analyzed word is unresolved")
	assert.Equal(t, int64(0), parseGap.UnresolvedDecisions[0].AddressedWords)
	assert.Equal(t, "der", parseGap.UnresolvedDecisions[0].CanonicalLemma)

	bobLookup, err := store.ListVocabularyConcordance(ctx, bob.ID, "de", domain.ConcordanceLookup{Term: "Haus", Page: 1})
	require.NoError(t, err)
	require.Len(t, bobLookup.Occurrences, 1)
	assert.Equal(t, bookBob.ID, bobLookup.Occurrences[0].BookID, "owner scoping")
}

// TestGetVocabularySentenceStudyReadsStoredMorphology holds the Sentence study
// read path: every analyzed word carries its stored morphology unfiltered, an
// empty feature map stays empty, and a complete sentence with no persisted
// parse tokens still returns the sentence and its target surface.
func TestGetVocabularySentenceStudyReadsStoredMorphology(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "sentence-study-morphology", false)
	require.NoError(t, err)

	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Morphology Book", "study-morphology", false, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter", "Das Haus steht.", 0, 15),
	})
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter", Ordinal: 0, Text: "Das Haus steht.", Start: 0, End: 15, Tokens: []concordanceToken{
			{Surface: "Das", Lemma: "das", Upos: "DET", Dependency: "det", Head: 1, Morphology: map[string]string{"Case": "Nom", "Number": "Sing"}, Start: 0, End: 3},
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Dependency: "nsubj", Head: 2, Morphology: map[string]string{}, Start: 4, End: 8},
			{Surface: "steht", Lemma: "stehen", Upos: "VERB", Dependency: "root", Head: 2, Morphology: map[string]string{"Mood": "Ind", "VerbForm": "Fin"}, Start: 9, End: 14},
		}},
	})
	corpusID := corpusIDForSource(t, ctx, store, owner.ID, source.ID)
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, owner.ID, corpusID).Scan(&runID))

	study, err := store.GetVocabularySentenceStudy(ctx, owner.ID, book.ID, runID, corpusID, "epub-unit-v1:0:chapter", 0, 1, "Haus")
	require.NoError(t, err)
	require.Len(t, study.Tokens, 3)
	assert.Equal(t, map[string]string{"Case": "Nom", "Number": "Sing"}, study.Tokens[0].Morphology)
	assert.Empty(t, study.Tokens[1].Morphology, "a word with no stored features keeps empty morphology")
	assert.Equal(t, map[string]string{"Mood": "Ind", "VerbForm": "Fin"}, study.Tokens[2].Morphology)
	// The reader receives the authoritative source slice and each stored span,
	// so alignment can validate the sentence against the immutable unit text.
	assert.Equal(t, "Das Haus steht.", study.UnitText)
	assert.Equal(t, int64(0), study.SentenceStartOffset)
	assert.Equal(t, int64(15), study.SentenceEndOffset)
	assert.Equal(t, [][2]int64{{0, 3}, {4, 8}, {9, 14}}, [][2]int64{
		{study.Tokens[0].StartOffset, study.Tokens[0].EndOffset},
		{study.Tokens[1].StartOffset, study.Tokens[1].EndOffset},
		{study.Tokens[2].StartOffset, study.Tokens[2].EndOffset},
	})

	// A stored value the string-map model cannot represent contributes no
	// features rather than failing the whole sentence-study read.
	_, err = store.Pool().Exec(ctx, `UPDATE corpus_tokens SET morphology='{"VerbForm": 5}'::jsonb WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3 AND sentence_ordinal=0 AND token_ordinal=2`, owner.ID, corpusID, runID)
	require.NoError(t, err)
	degraded, err := store.GetVocabularySentenceStudy(ctx, owner.ID, book.ID, runID, corpusID, "epub-unit-v1:0:chapter", 0, 1, "Haus")
	require.NoError(t, err)
	require.Len(t, degraded.Tokens, 3)
	assert.Empty(t, degraded.Tokens[2].Morphology, "an undecodable stored value contributes no features")

	_, err = store.Pool().Exec(ctx, `DELETE FROM corpus_tokens WHERE owner_id=$1 AND corpus_id=$2 AND analysis_run_id=$3 AND sentence_ordinal=0`, owner.ID, corpusID, runID)
	require.NoError(t, err)
	noTokens, err := store.GetVocabularySentenceStudy(ctx, owner.ID, book.ID, runID, corpusID, "epub-unit-v1:0:chapter", 0, 1, "Haus")
	require.NoError(t, err)
	assert.Equal(t, "Das Haus steht.", noTokens.SentenceText)
	assert.Empty(t, noTokens.Tokens, "a source sentence with no parse tokens has no token evidence")
	assert.Equal(t, "Haus", noTokens.TargetSurface)
}

// TestGetVocabularySentenceStudyAttributesOnlyUniqueDecisions holds the
// correction/exclusion gate: a decision is attributed only when its exact
// owner/Book/analysis/source-document/span addresses exactly one persisted
// analyzed word. A span shared by siblings stays unresolved and is applied to
// no word, while an attributed exclusion keeps every grammar feature.
func TestGetVocabularySentenceStudyAttributesOnlyUniqueDecisions(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "sentence-study-decisions", false)
	require.NoError(t, err)

	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Decisions Book", "study-decisions", false, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter", "zum Haus steht", 0, 14),
	})
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter", Ordinal: 0, Text: "zum Haus steht", Start: 0, End: 14, Tokens: []concordanceToken{
			{Surface: "zu", Lemma: "zu", Upos: "ADP", Dependency: "case", Head: 2, Morphology: map[string]string{"Case": "Dat"}, Start: 0, End: 3},
			{Surface: "dem", Lemma: "der", Upos: "DET", Dependency: "det", Head: 2, Morphology: map[string]string{"Case": "Dat", "Gender": "Neut"}, Start: 0, End: 3},
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Dependency: "root", Head: 2, Morphology: map[string]string{"Case": "Dat", "Number": "Sing"}, Start: 4, End: 8},
			{Surface: "steht", Lemma: "stehen", Upos: "VERB", Dependency: "root", Head: 3, Morphology: map[string]string{"Mood": "Ind", "VerbForm": "Fin"}, Start: 9, End: 14},
		}},
	})
	corpus, run := currentSentenceStudyAnalysis(t, ctx, store, owner.ID, book.ID)

	// A decision whose span is shared by the two words of the contraction
	// addresses two analyzed words, so it is unresolved and applied to neither.
	insertOccurrenceDecision(t, ctx, store, owner.ID, book.ID, corpus, run, "epub-unit-v1:0:chapter", 0, 3, "zu", false)
	// A decision whose span addresses exactly one word is attributed to it.
	insertOccurrenceDecision(t, ctx, store, owner.ID, book.ID, corpus, run, "epub-unit-v1:0:chapter", 4, 8, "heim", false)
	// An exclusion that addresses exactly one word is attributed to it.
	insertOccurrenceDecision(t, ctx, store, owner.ID, book.ID, corpus, run, "epub-unit-v1:0:chapter", 9, 14, "", true)

	study, err := store.GetVocabularySentenceStudy(ctx, owner.ID, book.ID, run, corpus, "epub-unit-v1:0:chapter", 0, 2, "Haus")
	require.NoError(t, err)
	require.Len(t, study.Tokens, 4)

	t.Run("shared-span siblings carry no decision", func(t *testing.T) {
		require.False(t, study.Tokens[0].Corrected, "a shared-span sibling is never corrected")
		require.Equal(t, "zu", study.Tokens[0].EffectiveLemma)
		require.False(t, study.Tokens[1].Corrected, "the other shared-span sibling is never corrected")
		require.Equal(t, "der", study.Tokens[1].EffectiveLemma)
	})

	t.Run("a unique correction is attributed without changing grammar evidence", func(t *testing.T) {
		require.True(t, study.Tokens[2].Corrected)
		require.Equal(t, "heim", study.Tokens[2].EffectiveLemma)
		require.Equal(t, map[string]string{"Case": "Dat", "Number": "Sing"}, study.Tokens[2].Morphology,
			"a lemma correction changes no morphology")
		require.Equal(t, "root", study.Tokens[2].Dependency, "a lemma correction changes no dependency")
	})

	t.Run("a unique exclusion keeps every grammar feature", func(t *testing.T) {
		require.True(t, study.Tokens[3].Excluded)
		require.False(t, study.Tokens[3].Corrected)
		require.Equal(t, map[string]string{"Mood": "Ind", "VerbForm": "Fin"}, study.Tokens[3].Morphology,
			"an exclusion keeps every grammar feature")
		require.Equal(t, "root", study.Tokens[3].Dependency, "an exclusion keeps its dependency")
	})

	t.Run("the shared-span decision is unresolved", func(t *testing.T) {
		require.Len(t, study.UnresolvedDecisions, 1)
		require.Equal(t, int64(0), study.UnresolvedDecisions[0].StartOffset)
		require.Equal(t, int64(3), study.UnresolvedDecisions[0].EndOffset)
		require.Equal(t, "zu", study.UnresolvedDecisions[0].CanonicalLemma)
		require.Equal(t, int64(2), study.UnresolvedDecisions[0].AddressedWords)
	})
}

// TestGetVocabularySentenceStudyResolvesUniquenessBeforeSentenceFiltering holds
// that uniqueness is measured over every persisted word the decision addresses,
// not just the selected sentence: a sibling the sentence subset filters away
// still makes the decision unresolved.
func TestGetVocabularySentenceStudyResolvesUniquenessBeforeSentenceFiltering(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "sentence-study-filtered-sibling", false)
	require.NoError(t, err)

	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Filtered Sibling Book", "study-filtered-sibling", false, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter", "Haus Weg", 0, 8),
	})
	// Two persisted words share the decision's span, but only the first is part
	// of the selected sentence. The decision still addresses two words.
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter", Ordinal: 0, Text: "Haus", Start: 0, End: 4, Tokens: []concordanceToken{
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Dependency: "root", Head: 0, Start: 0, End: 4},
		}},
		{UnitID: "epub-unit-v1:0:chapter", Ordinal: 1, Text: "Weg", Start: 5, End: 8, Tokens: []concordanceToken{
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Dependency: "root", Head: 0, Start: 0, End: 4},
		}},
	})
	corpus, run := currentSentenceStudyAnalysis(t, ctx, store, owner.ID, book.ID)
	insertOccurrenceDecision(t, ctx, store, owner.ID, book.ID, corpus, run, "epub-unit-v1:0:chapter", 0, 4, "heim", false)

	study, err := store.GetVocabularySentenceStudy(ctx, owner.ID, book.ID, run, corpus, "epub-unit-v1:0:chapter", 0, 0, "Haus")
	require.NoError(t, err)
	require.Len(t, study.Tokens, 1)
	assert.False(t, study.Tokens[0].Corrected, "a filtered-away sibling makes the decision unresolved")
	assert.Equal(t, "haus", study.Tokens[0].EffectiveLemma)
	require.Len(t, study.UnresolvedDecisions, 1)
	assert.Equal(t, int64(2), study.UnresolvedDecisions[0].AddressedWords)
}

// TestGetVocabularySentenceStudyIsolatesDecisionsBySourceAndAnalysis holds that
// a decision bound to another source document or another analysis run never
// attaches to this sentence's words.
func TestGetVocabularySentenceStudyIsolatesDecisionsBySourceAndAnalysis(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "sentence-study-isolation", false)
	require.NoError(t, err)

	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Isolation Book", "study-isolation", false, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter", "Haus", 0, 4),
	})
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter", Ordinal: 0, Text: "Haus", Start: 0, End: 4, Tokens: []concordanceToken{
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Dependency: "root", Head: 0, Start: 0, End: 4},
		}},
	})
	corpus, run := currentSentenceStudyAnalysis(t, ctx, store, owner.ID, book.ID)

	// A decision for the same span in another source document does not attach.
	insertOccurrenceDecision(t, ctx, store, owner.ID, book.ID, corpus, run, "epub-unit-v1:9:elsewhere", 0, 4, "heim", false)
	isolated, err := store.GetVocabularySentenceStudy(ctx, owner.ID, book.ID, run, corpus, "epub-unit-v1:0:chapter", 0, 0, "Haus")
	require.NoError(t, err)
	require.Len(t, isolated.Tokens, 1)
	assert.False(t, isolated.Tokens[0].Corrected)
	assert.Empty(t, isolated.UnresolvedDecisions)

	// A decision for the same source document in another analysis run does not
	// attach to the current run either.
	_, otherSource := createConcordanceBook(t, ctx, store, owner.ID, "Isolation Other", "study-isolation-other", false, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter", "Haus", 0, 4),
	})
	insertConcordanceAnalysis(t, ctx, store, otherSource, false, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter", Ordinal: 0, Text: "Haus", Start: 0, End: 4, Tokens: []concordanceToken{
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Dependency: "root", Head: 0, Start: 0, End: 4},
		}},
	})
	otherCorpus, otherRun := sourceAnalysis(t, ctx, store, owner.ID, otherSource.ID)
	insertOccurrenceDecision(t, ctx, store, owner.ID, book.ID, otherCorpus, otherRun, "epub-unit-v1:0:chapter", 0, 4, "heim", false)
	isolated, err = store.GetVocabularySentenceStudy(ctx, owner.ID, book.ID, run, corpus, "epub-unit-v1:0:chapter", 0, 0, "Haus")
	require.NoError(t, err)
	assert.False(t, isolated.Tokens[0].Corrected, "a decision from another analysis run never attaches")
	assert.Empty(t, isolated.UnresolvedDecisions)

	// The decision bound to this exact analysis and source document attaches.
	insertOccurrenceDecision(t, ctx, store, owner.ID, book.ID, corpus, run, "epub-unit-v1:0:chapter", 0, 4, "heim", false)
	attributed, err := store.GetVocabularySentenceStudy(ctx, owner.ID, book.ID, run, corpus, "epub-unit-v1:0:chapter", 0, 0, "Haus")
	require.NoError(t, err)
	assert.True(t, attributed.Tokens[0].Corrected)
	assert.Equal(t, "heim", attributed.Tokens[0].EffectiveLemma)
	assert.Empty(t, attributed.UnresolvedDecisions)
}

func currentSentenceStudyAnalysis(t *testing.T, ctx context.Context, store *PostgresStore, owner, book string) (corpus, run string) {
	t.Helper()
	err := store.Pool().QueryRow(ctx, `SELECT corpus_id::text, analysis_run_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner, book).Scan(&corpus, &run)
	require.NoError(t, err)
	return corpus, run
}

func sourceAnalysis(t *testing.T, ctx context.Context, store *PostgresStore, owner, sourceID string) (corpus, run string) {
	t.Helper()
	err := store.Pool().QueryRow(ctx, `SELECT id::text, analysis_run_id::text FROM corpora WHERE owner_id=$1 AND source_material_id=$2`, owner, sourceID).Scan(&corpus, &run)
	require.NoError(t, err)
	return corpus, run
}

func insertOccurrenceDecision(t *testing.T, ctx context.Context, store *PostgresStore, owner, book, corpus, run, unit string, start, end int64, lemma string, excluded bool) {
	t.Helper()
	var canonical, profile, version any
	if !excluded {
		canonical, profile, version = lemma, "de", "1"
	}
	_, err := store.Pool().Exec(ctx, `
		INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		owner, book, corpus, run, unit, start, end, canonical, profile, version, excluded)
	require.NoError(t, err)
}

func TestVocabularyBrowseUsesCurrentOwnerScopedEvidence(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	alice, err := store.CreateUser(ctx, "browse-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "browse-bob", false)
	require.NoError(t, err)
	var aliceBook string
	var aliceOtherBook string
	for owner, text := range map[string]string{alice.ID: "alice", bob.ID: "bob"} {
		textLen := int64(len(text))
		book, source := createConcordanceBook(t, ctx, store, owner, "Browse "+text, text, false, []domain.ExtractedUnit{
			concordanceUnit(0, "browse", text+" Haus", 0, uint64(textLen+5)),
		})
		insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{{
			UnitID: "epub-unit-v1:0:browse", Ordinal: 0, Text: text + " Haus", Start: 0, End: textLen + 5,
			Tokens: []concordanceToken{{Surface: text, Lemma: text, Upos: "NOUN", Start: 0, End: textLen}, {Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: textLen + 1, End: textLen + 5}},
		}})
		if owner == alice.ID {
			aliceBook = book.ID
			_, err := store.PutKnownVocabulary(ctx, alice.ID, "de", "haus", "NOUN")
			require.NoError(t, err)
			_, err = store.PutKnownVocabulary(ctx, alice.ID, "de", "ghost", "NOUN")
			require.NoError(t, err)
		}
	}
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, aliceBook, domain.BookDispositionInbox))
	aliceToReadBook, aliceToReadSource := createConcordanceBook(t, ctx, store, alice.ID, "Browse To Read", "browse-to-read", true, []domain.ExtractedUnit{
		concordanceUnit(0, "browse-to-read", "Haus", 0, 4),
	})
	aliceOtherBook = aliceToReadBook.ID
	insertConcordanceAnalysis(t, ctx, store, aliceToReadSource, true, []concordanceSentence{{
		UnitID: "epub-unit-v1:0:browse-to-read", Ordinal: 0, Text: "Haus", Start: 0, End: 4,
		Tokens: []concordanceToken{{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4}},
	}})
	var toReadCorpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, alice.ID, aliceToReadBook.ID).Scan(&toReadCorpusID))
	goal, err := store.StartCurrentReading(ctx, alice.ID, "de", aliceToReadBook.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO primary_goal_snapshot_vocabulary(owner_id,snapshot_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,$3,'de','haus','NOUN',1,'[]','[]','{}')`, alice.ID, goal.SnapshotID, toReadCorpusID)
	require.NoError(t, err)
	browseDeck, err := store.PutDeck(ctx, alice.ID, "de", "Browse generated identity")
	require.NoError(t, err)
	_, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{
		OwnerID: alice.ID, Language: "de", CanonicalLemma: "heim", UPOS: "NOUN", FirstDeckID: browseDeck.ID,
		FirstSourceMaterialID: &aliceToReadSource.ID,
	})
	require.NoError(t, err)
	staleBook, staleSource := createConcordanceBook(t, ctx, store, alice.ID, "Browse without current analysis", "browse-old", false, []domain.ExtractedUnit{
		concordanceUnit(0, "browse-old", "Haus", 0, 4),
	})
	insertConcordanceAnalysis(t, ctx, store, staleSource, false, []concordanceSentence{{
		UnitID: "epub-unit-v1:0:browse-old", Ordinal: 0, Text: "Haus", Start: 0, End: 4,
		Tokens: []concordanceToken{{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Start: 0, End: 4}},
	}})
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, staleBook.ID, domain.BookDispositionInbox))
	_, err = store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Italian", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "it"})
	require.NoError(t, err)
	var corpusID, analysisRunID string
	err = store.Pool().QueryRow(ctx, `SELECT corpus_id::text, analysis_run_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, alice.ID, aliceBook).Scan(&corpusID, &analysisRunID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `
		INSERT INTO occurrence_lemma_corrections(owner_id, book_id, corpus_id, analysis_run_id, source_document_id, start_offset, end_offset, canonical_lemma, normalization_profile, normalization_version, excluded)
		VALUES ($1,$2,$3,$4,'epub-unit-v1:0:browse',$5,$6,'heim','test-profile','1',false),
		       ($1,$2,$3,$4,'epub-unit-v1:0:browse',0,$7,NULL,NULL,NULL,true)`,
		alice.ID, aliceBook, corpusID, analysisRunID, int64(len("alice"))+1, int64(len("alice"))+5, int64(len("alice")))
	require.NoError(t, err)
	buildBrowseProjectionsForOwner(t, ctx, store, alice.ID)
	buildBrowseProjectionsForOwner(t, ctx, store, bob.ID)

	page, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{Prefix: "ha", Page: 1})
	require.NoError(t, err)
	require.Len(t, page.Rows, 1)
	assert.Equal(t, "haus", page.Rows[0].CanonicalLemma)
	assert.True(t, page.Rows[0].Known, "Known identities remain visible when current evidence exists")
	assert.True(t, page.Rows[0].Reserved, "Reserved identities remain visible when current evidence exists")
	assert.Equal(t, int64(1), page.Rows[0].BookCount, "the To Read Book contributes independently of Inbox")
	assert.Equal(t, int64(1), page.Rows[0].AcrossBooksOccurrenceCount)
	assert.Equal(t, int64(2), page.AnalyzedBooks)
	assert.Equal(t, int64(1), page.NoncontributingBooks)
	assert.Equal(t, int64(1), page.BooksWithoutCurrentAnalysis)
	assert.Len(t, page.Books, 3)
	assert.NotEmpty(t, aliceOtherBook)
	bookEvidence := make(map[string]domain.VocabularyBrowseBook, len(page.Books))
	for _, book := range page.Books {
		bookEvidence[book.Title] = book
	}
	assert.True(t, bookEvidence["Browse alice"].HasCurrentAnalysis)
	assert.True(t, bookEvidence["Browse alice"].HasVocabularyEvidence)
	assert.True(t, bookEvidence["Browse To Read"].HasVocabularyEvidence)
	assert.False(t, bookEvidence["Browse without current analysis"].HasCurrentAnalysis)

	ghost, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{Prefix: "ghost", Page: 1})
	require.NoError(t, err)
	assert.Empty(t, ghost.Rows, "Known-only identities without current evidence must not be shown")
	assert.Zero(t, ghost.Total)
	assert.Equal(t, int64(2), ghost.InventoryTotal, "prefix filtering must not change the complete inventory count")
	excluded, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{Prefix: "ali", Page: 1})
	require.NoError(t, err)
	assert.Empty(t, excluded.Rows, "an excluded occurrence must contribute no effective identity")
	assert.Zero(t, excluded.Total)
	assert.Equal(t, int64(2), excluded.InventoryTotal)

	page, err = store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{Prefix: "hei", Page: 1})
	require.NoError(t, err)
	require.Len(t, page.Rows, 1)
	assert.Equal(t, "heim", page.Rows[0].CanonicalLemma)
	assert.Equal(t, int64(1), page.Rows[0].OccurrenceCount)
	assert.Equal(t, int64(1), page.Rows[0].BookCount)
	assert.False(t, page.Rows[0].Known)
	assert.True(t, page.Rows[0].Corrected)
	assert.True(t, page.Rows[0].Generated, "Generated does not imply Known")
	assert.Equal(t, int64(1), page.Total)
	assert.Equal(t, int64(2), page.ContributingBooks)
	assert.Equal(t, int64(1), page.NoncontributingBooks)
	assert.Equal(t, int64(1), page.BooksWithoutCurrentAnalysis)
	var promotedRun string
	err = store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, staleSource.ID).Scan(&promotedRun)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, alice.ID, staleBook.ID, staleSource.ID, promotedRun)
	require.NoError(t, err)
	buildBrowseProjectionsForOwner(t, ctx, store, alice.ID)
	promoted, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{Prefix: "ha", Page: 1})
	require.NoError(t, err)
	require.Len(t, promoted.Rows, 1)
	assert.Equal(t, int64(2), promoted.Rows[0].OccurrenceCount)
	assert.Equal(t, int64(2), promoted.Rows[0].BookCount)
	currentBook, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: aliceToReadBook.ID, Prefix: "ha", Sort: "occurrences", IncludeAll: true, Page: 1})
	require.NoError(t, err)
	require.Len(t, currentBook.Rows, 1)
	assert.Equal(t, int64(1), currentBook.Rows[0].OccurrenceCount, "the Browse inventory and local count stay scoped to Current reading")
	assert.Equal(t, int64(2), currentBook.Rows[0].AcrossBooksOccurrenceCount, "a current analysis in another disposition contributes to the cross-Book count")
	assert.Equal(t, int64(1), currentBook.InventoryTotal, "the Current-reading inventory count remains Book-scoped")
	require.Len(t, currentBook.Books, 1)
	assert.Equal(t, "Browse To Read", currentBook.Books[0].Title, "the evidence projection names only the Current-reading Book")
	onlyElsewhere, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{CurrentBookID: aliceToReadBook.ID, Prefix: "alice", Page: 1})
	require.NoError(t, err)
	assert.Empty(t, onlyElsewhere.Rows, "identities found only in another Book do not enter the Current-reading inventory")
	assert.Equal(t, int64(3), promoted.AnalyzedBooks)
	assert.Zero(t, promoted.BooksWithoutCurrentAnalysis)

	notKnownOrReserved, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{KnownFilter: "not-known-or-reserved", Sort: "occurrences", Page: 1})
	require.NoError(t, err)
	require.Len(t, notKnownOrReserved.Rows, 1)
	assert.Equal(t, "heim", notKnownOrReserved.Rows[0].CanonicalLemma, "the Not Known or Reserved view excludes either learner state")
	assert.Equal(t, int64(1), notKnownOrReserved.Rows[0].OccurrenceCount)

	byOccurrences, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{Sort: "occurrences", Page: 1})
	require.NoError(t, err)
	require.Len(t, byOccurrences.Rows, 2)
	assert.Equal(t, "haus", byOccurrences.Rows[0].CanonicalLemma)
	assert.Equal(t, int64(2), byOccurrences.Rows[0].OccurrenceCount)
	assert.Equal(t, "heim", byOccurrences.Rows[1].CanonicalLemma)

	multiBook, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "de", domain.VocabularyBrowseQuery{Prefix: "ha", BookIDs: []string{aliceToReadBook.ID, staleBook.ID}, UPOS: []string{"NOUN"}, Sort: "books", Page: 1})
	require.NoError(t, err)
	require.Len(t, multiBook.Rows, 1)
	assert.Equal(t, "haus", multiBook.Rows[0].CanonicalLemma)
	assert.Equal(t, int64(2), multiBook.Rows[0].OccurrenceCount)
	assert.Equal(t, int64(2), multiBook.Rows[0].BookCount, "the selected multi-Book subset scopes distinct-Book counts")
	assert.Equal(t, int64(1), multiBook.ScopedInventoryTotal)

	bookDetail, err := store.GetBookDetail(ctx, alice.ID, aliceBook)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, bookDetail.Disposition, "Browse and analysis promotion must preserve disposition")

	italian, err := store.ListVocabularyBrowsePage(ctx, alice.ID, "it", domain.VocabularyBrowseQuery{Page: 1})
	require.NoError(t, err)
	assert.Empty(t, italian.Rows, "Books in another study language must not contribute German evidence")
	assert.Equal(t, int64(1), italian.NoncontributingBooks)
	assert.Equal(t, int64(1), italian.BooksWithoutCurrentAnalysis)

	bobPage, err := store.ListVocabularyBrowsePage(ctx, bob.ID, "de", domain.VocabularyBrowseQuery{Prefix: "ha", Page: 1})
	require.NoError(t, err)
	require.Len(t, bobPage.Rows, 1)
	assert.False(t, bobPage.Rows[0].Known, "learner state must not leak across owners")
	assert.Equal(t, int64(1), bobPage.Rows[0].OccurrenceCount)
	assert.Equal(t, int64(1), bobPage.Rows[0].AcrossBooksOccurrenceCount, "Alice's matching German Books must not contribute to Bob's total")
}

func TestCurrentReadingBrowseRanksLocalFrequencyThenAcrossBookTotal(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "browse-rank-owner", false)
	require.NoError(t, err)
	createAnalyzed := func(title, key, text string, tokens []concordanceToken) domain.Book {
		book, source := createConcordanceBook(t, ctx, store, owner.ID, title, key, false, []domain.ExtractedUnit{
			concordanceUnit(0, key, text, 0, uint64(len(text))),
		})
		insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{{
			UnitID: "epub-unit-v1:0:" + key, Ordinal: 0, Text: text, Start: 0, End: int64(len(text)), Tokens: tokens,
		}})
		return book
	}
	current := createAnalyzed("Current", "browse-rank-current", "alpha zeta", []concordanceToken{
		{Surface: "alpha", Lemma: "alpha", Upos: "NOUN", Start: 0, End: 5},
		{Surface: "zeta", Lemma: "zeta", Upos: "NOUN", Start: 6, End: 10},
	})
	createAnalyzed("Other", "browse-rank-other", "zeta zeta zeta alpha ghost", []concordanceToken{
		{Surface: "zeta", Lemma: "zeta", Upos: "NOUN", Start: 0, End: 4},
		{Surface: "zeta", Lemma: "zeta", Upos: "NOUN", Start: 5, End: 9},
		{Surface: "zeta", Lemma: "zeta", Upos: "NOUN", Start: 10, End: 14},
		{Surface: "alpha", Lemma: "alpha", Upos: "NOUN", Start: 15, End: 20},
		{Surface: "ghost", Lemma: "ghost", Upos: "NOUN", Start: 21, End: 26},
	})
	buildBrowseProjectionsForOwner(t, ctx, store, owner.ID)

	page, err := store.ListVocabularyBrowsePage(ctx, owner.ID, "de", domain.VocabularyBrowseQuery{
		CurrentBookID: current.ID, Sort: "occurrences", IncludeAll: true, Page: 1,
	})
	require.NoError(t, err)
	require.Len(t, page.Rows, 2, "an identity found only in another Book cannot broaden Browse")
	assert.Equal(t, "zeta", page.Rows[0].CanonicalLemma, "equal local counts rank by descending across-Book total before canonical lemma")
	assert.Equal(t, int64(1), page.Rows[0].OccurrenceCount)
	assert.Equal(t, int64(4), page.Rows[0].AcrossBooksOccurrenceCount)
	assert.Equal(t, "alpha", page.Rows[1].CanonicalLemma)
	assert.Equal(t, int64(2), page.Rows[1].AcrossBooksOccurrenceCount)
}

func TestListCorpusSentencesReturnsBatchedTokenDependencyData(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))

	owner, err := store.CreateUser(ctx, "cardexport-sentence-owner", false)
	require.NoError(t, err)
	_, source := createConcordanceBook(t, ctx, store, owner.ID, "Export Book", "cardexport-sentence", false, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter", "Das Haus steht.", 0, 15),
	})
	insertConcordanceAnalysis(t, ctx, store, source, false, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter", Ordinal: 4, Text: "Das Haus steht.", Start: 0, End: 15, Tokens: []concordanceToken{
			{Surface: "Das", Lemma: "das", Upos: "DET", Dependency: "det", Head: 1, Start: 0, End: 3},
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Dependency: "nsubj", Head: 2, Morphology: map[string]string{"Case": "Nom", "Number": "Sing"}, Start: 4, End: 8},
			{Surface: "steht", Lemma: "stehen", Upos: "VERB", Dependency: "root", Head: 2, Start: 9, End: 14},
		}},
		{UnitID: "epub-unit-v1:0:chapter", Ordinal: 5, Text: "Der Mann geht.", Start: 40, End: 54, Tokens: []concordanceToken{
			{Surface: "Der", Lemma: "der", Upos: "DET", Dependency: "det", Head: 1, Start: 40, End: 43},
			{Surface: "Mann", Lemma: "mann", Upos: "NOUN", Dependency: "nsubj", Head: 2, Start: 44, End: 48},
			{Surface: "geht", Lemma: "gehen", Upos: "VERB", Dependency: "root", Head: 2, Start: 49, End: 53},
		}},
	})

	got, err := store.ListCorpusSentences(ctx, owner.ID, corpusIDForSource(t, ctx, store, owner.ID, source.ID), []int64{4, 5, 99, 4})
	require.NoError(t, err)
	require.Len(t, got, 2)
	sentence, ok := got[4]
	require.True(t, ok)
	require.Len(t, sentence.Tokens, 3)
	assert.Equal(t, "Das Haus steht.", sentence.Text)
	assert.Equal(t, uint64(0), sentence.Location.StartOffset, "card export anchors targets relative to the persisted sentence start")
	assert.Equal(t, uint64(4), sentence.Tokens[1].Location.StartOffset)
	assert.Equal(t, uint64(8), sentence.Tokens[1].Location.EndOffset)
	assert.Equal(t, "Haus", sentence.Tokens[1].Surface)
	assert.Equal(t, "haus", sentence.Tokens[1].RawLemma)
	assert.Equal(t, "haus", sentence.Tokens[1].CanonicalLemma)
	assert.Equal(t, "NOUN", sentence.Tokens[1].UPOS)
	assert.Equal(t, "nsubj", sentence.Tokens[1].Dependency)
	assert.Equal(t, uint32(2), sentence.Tokens[1].Head)
	assert.Equal(t, map[string]string{"Case": "Nom", "Number": "Sing"}, sentence.Tokens[1].Morphology)

	later, ok := got[5]
	require.True(t, ok)
	assert.Equal(t, uint64(40), later.Location.StartOffset, "a sentence later in the document keeps its document-absolute start")
	require.Len(t, later.Tokens, 3)
	assert.Equal(t, uint64(44), later.Tokens[1].Location.StartOffset)
	assert.Equal(t, uint64(48), later.Tokens[1].Location.EndOffset)
}

func corpusIDForSource(t *testing.T, ctx context.Context, store *PostgresStore, ownerID, sourceID string) string {
	t.Helper()
	var corpusID string
	err := store.Pool().QueryRow(ctx, `SELECT id::text FROM corpora WHERE owner_id=$1 AND source_material_id=$2`, ownerID, sourceID).Scan(&corpusID)
	require.NoError(t, err)
	return corpusID
}

type concordanceSentence struct {
	UnitID              string
	Ordinal, Start, End int64
	Text                string
	Tokens              []concordanceToken
}

type concordanceToken struct {
	Surface, Lemma, Upos, Dependency string
	Head                             int64
	Morphology                       map[string]string
	Start, End                       int64
}

func concordanceUnit(spine uint64, manifest, text string, start, end uint64) domain.ExtractedUnit {
	return domain.ExtractedUnit{
		ID: domain.EPUBUnitID(spine, manifest), Order: spine, SpineIndex: spine,
		Title: "Chapter 1", TitleSource: domain.UnitTitleHeading, Text: text,
		StartOffset: start, EndOffset: end, ManifestID: manifest,
		PackagePath: manifest + ".xhtml", SourceHref: manifest + ".xhtml", ResolvedHref: manifest + ".xhtml",
		MediaType: "application/xhtml+xml", Linear: true,
	}
}

func createConcordanceBook(t *testing.T, ctx context.Context, store *PostgresStore, owner, title, suffix string, toRead bool, units []domain.ExtractedUnit) (domain.Book, domain.SourceMaterial) {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{
		OwnerID: owner, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState: domain.LanguageChosen, LanguageTag: "de",
	})
	require.NoError(t, err)
	var fullTextBuilder strings.Builder
	for i, unit := range units {
		if i > 0 {
			fullTextBuilder.WriteString("\n\n")
		}
		fullTextBuilder.WriteString(unit.Text)
	}
	fullText := fullTextBuilder.String()
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner, Language: "de", SourceIdentifier: "concordance-" + suffix,
		Title: title, MediaType: "application/epub+zip", Content: []byte("epub-" + suffix), FullText: fullText,
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersionV1, Units: units})
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner, book.ID, source.ID))
	if toRead {
		require.NoError(t, store.SetBookDisposition(ctx, owner, book.ID, domain.BookDispositionToRead))
	}
	return book, source
}

func insertConcordanceAnalysis(t *testing.T, ctx context.Context, store *PostgresStore, source domain.SourceMaterial, current bool, sentences []concordanceSentence) {
	t.Helper()
	var revisionID, snapshotID string
	err := store.Pool().QueryRow(ctx, `SELECT current_content_revision_id::text,current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, source.OwnerID, source.ID).Scan(&revisionID, &snapshotID)
	require.NoError(t, err)
	require.NoError(t, store.PutArtifact(ctx, domain.NormalizedArtifact{
		ContentHash: source.ContentHash, Language: "de", SchemaVersion: "1", NormalizationProfile: "de",
		NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1",
	}, nil))
	var runID string
	err = store.Pool().QueryRow(ctx, `
		INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,
			analyzer_name,analyzer_version,config_identity,state,completed_at)
		VALUES($1,$2,$3,$4,'test','1',$5,'completed',now()) RETURNING id::text`,
		source.OwnerID, source.ID, revisionID, snapshotID, source.ID).Scan(&runID)
	require.NoError(t, err)
	corpus, err := store.PutCorpus(ctx, source.OwnerID, source.ID, source.ContentHash)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, runID, source.OwnerID, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, source.OwnerID, runID)
	require.NoError(t, err)
	if current {
		_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) SELECT $1,b.id,$2,$3 FROM books b WHERE b.owner_id=$1 AND b.id=(SELECT book_id FROM source_materials WHERE owner_id=$1 AND id=$2)`, source.OwnerID, source.ID, runID)
		require.NoError(t, err)
	}
	for _, sentence := range sentences {
		_, err = store.Pool().Exec(ctx, `
			INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, source.OwnerID, runID, corpus.ID, sentence.UnitID, sentence.Ordinal, sentence.Text, sentence.Start, sentence.End)
		require.NoError(t, err)
		for tokenOrdinal, token := range sentence.Tokens {
			dependency := token.Dependency
			if dependency == "" {
				dependency = "root"
			}
			morphology, err := json.Marshal(token.Morphology)
			require.NoError(t, err)
			if string(morphology) == "null" {
				morphology = []byte(`{}`)
			}
			_, err = store.Pool().Exec(ctx, `
				INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,
					surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset)
				VALUES($1,'de',$2,$3,$4,$5,$6,$7,$7,$8,$9,$10,$11,$12,$13)`, source.OwnerID, runID, corpus.ID, sentence.Ordinal, tokenOrdinal, token.Surface, token.Lemma, token.Upos, dependency, token.Head, morphology, token.Start, token.End)
			require.NoError(t, err)
		}
	}
}
