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

	bookRows, err := store.ListBookOccurrencesByLemma(ctx, alice.ID, bookA.ID, "de-DE", "haus", "NOUN")
	require.NoError(t, err)
	require.Len(t, bookRows, 1)
	assertOccurrence(t, bookRows[0], bookA.ID, "Book A", sourceA.ID, "epub-unit-v1:1:chapter-a", "Chapter 1", "Das Haus", 4, 8, 6, 10, 13, 17, 1, 1, "obj", 0, "Das")

	surfaceRows, err := store.ListBookOccurrencesBySurface(ctx, alice.ID, bookA.ID, "de", "Haus")
	require.NoError(t, err)
	assert.Equal(t, bookRows, surfaceRows)

	bookRoleRows, err := store.ListBookOccurrencesByLemmaAndDependency(ctx, alice.ID, bookA.ID, "de-DE", "haus", "NOUN", "obj")
	require.NoError(t, err)
	assert.Equal(t, bookRows, bookRoleRows)

	bookSurfaceRoleRows, err := store.ListBookOccurrencesBySurfaceAndDependency(ctx, alice.ID, bookA.ID, "de", "Haus", "obj")
	require.NoError(t, err)
	assert.Equal(t, bookRoleRows, bookSurfaceRoleRows)

	bookRootRows, err := store.ListBookOccurrencesByLemmaAndDependency(ctx, alice.ID, bookA.ID, "de", "haus", "NOUN", "root")
	require.NoError(t, err)
	assert.Empty(t, bookRootRows)

	languageRows, err := store.ListStudyLanguageOccurrencesByLemma(ctx, alice.ID, "de", "haus", "NOUN")
	require.NoError(t, err)
	require.Len(t, languageRows, 2)
	assertOccurrence(t, languageRows[0], bookA.ID, "Book A", sourceA.ID, "epub-unit-v1:1:chapter-a", "Chapter 1", "Das Haus", 4, 8, 6, 10, 13, 17, 1, 1, "obj", 0, "Das")
	assertOccurrence(t, languageRows[1], bookB.ID, "Book B", sourceB.ID, "epub-unit-v1:0:chapter-b", "Chapter 1", "Haus", 0, 4, 0, 4, 0, 4, 0, 2, "root", 0, "Haus")

	languageSurfaceRows, err := store.ListStudyLanguageOccurrencesBySurface(ctx, alice.ID, "de-DE", "Haus")
	require.NoError(t, err)
	assert.Equal(t, languageRows, languageSurfaceRows)

	languageRoleRows, err := store.ListStudyLanguageOccurrencesByLemmaAndDependency(ctx, alice.ID, "de", "haus", "NOUN", "obj")
	require.NoError(t, err)
	require.Len(t, languageRoleRows, 1)
	assert.Equal(t, languageRows[0], languageRoleRows[0])

	languageSurfaceRoleRows, err := store.ListStudyLanguageOccurrencesBySurfaceAndDependency(ctx, alice.ID, "de-DE", "Haus", "root")
	require.NoError(t, err)
	require.Len(t, languageSurfaceRoleRows, 1)
	assert.Equal(t, languageRows[1], languageSurfaceRoleRows[0])

	bookDependents, err := store.ListBookDependentsByGovernorLemma(ctx, alice.ID, bookA.ID, "de-DE", "das", "DET", "obj")
	require.NoError(t, err)
	require.Len(t, bookDependents, 1)
	assert.Equal(t, bookRows[0], bookDependents[0])

	languageDependents, err := store.ListStudyLanguageDependentsByGovernorLemma(ctx, alice.ID, "de", "das", "DET", "obj")
	require.NoError(t, err)
	require.Len(t, languageDependents, 1)

	ownRelation, err := store.ListVocabularyConcordance(ctx, alice.ID, "de", domain.ConcordanceLookup{
		Mode: "effective", Term: "haus", UPOS: "NOUN", BookIDs: []string{bookA.ID, bookB.ID},
		GrammarDirection: "own", Relation: "obj", Page: 1,
	})
	require.NoError(t, err)
	require.Len(t, ownRelation.Occurrences, 1)
	assert.Equal(t, bookA.ID, ownRelation.Occurrences[0].BookID)

	governorDependents, err := store.ListVocabularyConcordance(ctx, alice.ID, "de", domain.ConcordanceLookup{
		Mode: "effective", Term: "das", UPOS: "DET", BookIDs: []string{bookA.ID},
		GrammarDirection: "governor", Relation: "obj", Page: 1,
	})
	require.NoError(t, err)
	require.Len(t, governorDependents.Occurrences, 1)
	assert.Equal(t, "Haus", governorDependents.Occurrences[0].Surface)
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version) VALUES($1,$2,$3,$4,$5,2,5,'der','de','1')`,
		alice.ID, bookA.ID, governorDependents.Occurrences[0].CorpusID,
		governorDependents.Occurrences[0].AnalysisRunID, governorDependents.Occurrences[0].UnitID)
	require.NoError(t, err)
	correctedGovernor, err := store.ListVocabularyConcordance(ctx, alice.ID, "de", domain.ConcordanceLookup{
		Mode: "effective", Term: "der", UPOS: "DET", BookIDs: []string{bookA.ID},
		GrammarDirection: "governor", Relation: "obj", Page: 1,
	})
	require.NoError(t, err)
	require.Len(t, correctedGovernor.Occurrences, 1)
	uncorrectedGovernor, err := store.ListVocabularyConcordance(ctx, alice.ID, "de", domain.ConcordanceLookup{
		Mode: "effective", Term: "das", UPOS: "DET", BookIDs: []string{bookA.ID},
		GrammarDirection: "governor", Relation: "obj", Page: 1,
	})
	require.NoError(t, err)
	assert.Empty(t, uncorrectedGovernor.Occurrences)

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
	assert.Equal(t, bookRows[0], languageDependents[0])

	bobRows, err := store.ListStudyLanguageOccurrencesBySurface(ctx, bob.ID, "de", "Haus")
	require.NoError(t, err)
	require.Len(t, bobRows, 1)
	assert.Equal(t, bookBob.ID, bobRows[0].BookID, "owner scoping")

	bobRoleRows, err := store.ListStudyLanguageOccurrencesBySurfaceAndDependency(ctx, bob.ID, "de", "Haus", "obj")
	require.NoError(t, err)
	assert.Empty(t, bobRoleRows, "role filter must remain owner-scoped")

	bobDependents, err := store.ListStudyLanguageDependentsByGovernorLemma(ctx, bob.ID, "de", "das", "DET", "obj")
	require.NoError(t, err)
	assert.Empty(t, bobDependents, "dependents query must remain owner-scoped")
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
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, aliceBook, domain.BookDispositionSetAside))
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
	goal, err := store.CreatePrimaryGoal(ctx, alice.ID, "de", aliceToReadBook.ID)
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
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, staleBook.ID, domain.BookDispositionSetAside))
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
	assert.Equal(t, domain.BookDispositionSetAside, bookDetail.Disposition, "Browse and analysis promotion must preserve disposition")

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
	})

	got, err := store.ListCorpusSentences(ctx, owner.ID, corpusIDForSource(t, ctx, store, owner.ID, source.ID), []int64{4, 99, 4})
	require.NoError(t, err)
	require.Len(t, got, 1)
	sentence, ok := got[4]
	require.True(t, ok)
	require.Len(t, sentence.Tokens, 3)
	assert.Equal(t, "Das Haus steht.", sentence.Text)
	assert.Equal(t, "Haus", sentence.Tokens[1].Surface)
	assert.Equal(t, "haus", sentence.Tokens[1].RawLemma)
	assert.Equal(t, "haus", sentence.Tokens[1].CanonicalLemma)
	assert.Equal(t, "NOUN", sentence.Tokens[1].UPOS)
	assert.Equal(t, "nsubj", sentence.Tokens[1].Dependency)
	assert.Equal(t, uint32(2), sentence.Tokens[1].Head)
	assert.Equal(t, map[string]string{"Case": "Nom", "Number": "Sing"}, sentence.Tokens[1].Morphology)
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
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: units})
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

func assertOccurrence(t *testing.T, occurrence domain.ConcordanceOccurrence, bookID, bookTitle, sourceID, unitID, chapterTitle, sentenceText string, sentenceStart, sentenceEnd, unitStart, unitEnd, bookStart, bookEnd, unitOrder int64, _ int, dependency string, headOrdinal int64, headSurface string) {
	t.Helper()
	assert.Equal(t, bookID, occurrence.BookID)
	assert.Equal(t, bookTitle, occurrence.BookTitle)
	assert.Equal(t, sourceID, occurrence.SourceMaterialID)
	assert.Equal(t, unitID, occurrence.UnitID)
	assert.Equal(t, chapterTitle, occurrence.ChapterTitle)
	assert.Equal(t, sentenceText, occurrence.SentenceText)
	assert.Equal(t, "Haus", occurrence.Surface)
	assert.Equal(t, "haus", occurrence.CanonicalLemma)
	assert.Equal(t, "NOUN", occurrence.UPOS)
	assert.Equal(t, dependency, occurrence.Dependency)
	assert.Equal(t, headOrdinal, occurrence.HeadOrdinal)
	assert.Equal(t, headSurface, occurrence.HeadSurface)
	assert.Equal(t, sentenceStart, occurrence.SentenceStartOffset)
	assert.Equal(t, sentenceEnd, occurrence.SentenceEndOffset)
	assert.Equal(t, unitStart, occurrence.UnitStartOffset)
	assert.Equal(t, unitEnd, occurrence.UnitEndOffset)
	assert.Equal(t, bookStart, occurrence.BookStartOffset)
	assert.Equal(t, bookEnd, occurrence.BookEndOffset)
	assert.Equal(t, unitOrder, occurrence.UnitOrder)
}
