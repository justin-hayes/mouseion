//go:build integration

package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// preUpgradeAnabasis is the seeded state conversion must preserve.
type preUpgradeAnabasis struct {
	ownerID, bookID, sourceID, runID, corpusID, snapshotID string
	oldRevisionID, oldSnapshotID                           string
	definition                                             classical.Definition
	deckID, deckRunID                                      string
	passageContext, passageCitation, brownsonPin           string
}

func preUpgradeDefinition(t *testing.T, data []byte) classical.Definition {
	t.Helper()
	sum := sha256.Sum256(data)
	book := 1
	return classical.Definition{
		Key: "xenophon-anabasis-1", WorkKey: "xenophon-anabasis",
		WorkTitle: "Anabasis", WorkAuthor: "Xenophon", GreekTitle: "Ἀνάβασις",
		EditionKey: "xenophon-anabasis-marchant-1904", EditionLabel: "Marchant 1904",
		Language: classical.Language, ConstituentBook: &book, ConstituentBooks: 7,
		CitationStart: "1.1.1", CitationEnd: "1.10.19", WordCount: 7,
		Source: classical.SourcePin{
			Layer: "oga-src-v0.2.0-text", FileName: "tlg0032.tlg006.perseus-grc2_oga020_src.conllu",
			Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Format: classical.SourceFormatOGACoNLLU,
		},
		InterpretationVersion: "oga-agdt-upos/5", Revision: 1,
	}
}

// seedPreUpgradeAnabasis writes the old operator registration shape: a Book
// linked to the Reading unit, a whole-file source, a published unit analysis,
// and the learner state that must survive conversion.
func seedPreUpgradeAnabasis(t *testing.T, ctx context.Context, store *persistence.PostgresStore, pool *pgxpool.Pool) preUpgradeAnabasis {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "ancientgreek", "testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	definition := preUpgradeDefinition(t, data)
	owner, err := store.CreateUser(ctx, "conversion-owner", false)
	require.NoError(t, err)
	registration, err := store.RegisterReadingUnitBook(ctx, owner.ID, domain.ReadingUnitDefinition{
		Key:                   definition.Key,
		Language:              definition.Language,
		WorkTitle:             definition.WorkTitle,
		WorkAuthor:            definition.WorkAuthor,
		EditionLabel:          definition.EditionLabel,
		CitationStart:         definition.CitationStart,
		CitationEnd:           definition.CitationEnd,
		SourceFileName:        definition.Source.FileName,
		SourceFileSize:        definition.Source.Size,
		SourceFileSHA256:      definition.Source.SHA256,
		InterpretationVersion: definition.InterpretationVersion,
	})
	require.NoError(t, err)

	parsed, err := ancientgreek.ParseOgaSource(data)
	require.NoError(t, err)
	unit, err := parsed.CutUnit(domain.ReadingUnitDefinition{
		Key: definition.Key, Language: definition.Language, WorkTitle: definition.WorkTitle,
		WorkAuthor: definition.WorkAuthor, EditionLabel: definition.EditionLabel,
		CitationStart: definition.CitationStart, CitationEnd: definition.CitationEnd,
		SourceFileName: definition.Source.FileName, SourceFileSize: definition.Source.Size,
		SourceFileSHA256: definition.Source.SHA256, InterpretationVersion: definition.InterpretationVersion,
	})
	require.NoError(t, err)
	written, err := NewGreekImportWriter(store).WriteImportedAnalysis(ctx, ancientgreek.ImportWriteRequest{
		OwnerID: owner.ID, BookID: registration.BookID, Definition: domain.ReadingUnitDefinition{
			Key: definition.Key, Language: definition.Language, WorkTitle: definition.WorkTitle,
			WorkAuthor: definition.WorkAuthor, EditionLabel: definition.EditionLabel,
			CitationStart: definition.CitationStart, CitationEnd: definition.CitationEnd,
			SourceFileName: definition.Source.FileName, SourceFileSize: definition.Source.Size,
			SourceFileSHA256: definition.Source.SHA256, InterpretationVersion: definition.InterpretationVersion,
		},
		SourceBytes: data, FullText: unit.FullText, UnitID: unit.UnitID, UnitTitle: definition.WorkTitle,
		Result: unit.Result, SyntaxState: unit.SyntaxState, AnalyzerName: unit.AnalyzerName,
		AnalyzerVersion: unit.AnalyzerVersion, ConfigIdentity: unit.ConfigIdentity,
	})
	require.NoError(t, err)

	seed := preUpgradeAnabasis{
		ownerID: owner.ID, bookID: registration.BookID, runID: written.RunID,
		definition: definition,
	}
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT sm.id::text, c.id::text, sm.current_content_revision_id::text, sm.current_snapshot_id::text
		FROM source_materials sm
		JOIN book_current_analyses p ON p.owner_id=sm.owner_id AND p.source_material_id=sm.id
		JOIN analysis_runs r ON r.owner_id=p.owner_id AND r.id=p.analysis_run_id
		JOIN corpora c ON c.owner_id=r.owner_id AND c.id=r.corpus_id
		WHERE sm.owner_id=$1 AND sm.book_id=$2`, owner.ID, registration.BookID).
		Scan(&seed.sourceID, &seed.corpusID, &seed.oldRevisionID, &seed.oldSnapshotID))
	// Rewrite the source identity to the pre-upgrade whole-file shape.
	_, err = pool.Exec(ctx, `UPDATE source_materials SET source_identifier=$3 WHERE owner_id=$1 AND id=$2`,
		owner.ID, seed.sourceID, definition.Source.FileName)
	require.NoError(t, err)

	// A confirmed Lemma correction and one Occurrence exclusion on the run.
	var firstStart, firstEnd, secondStart, secondEnd int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT start_offset, end_offset FROM corpus_tokens
		WHERE owner_id=$1 AND analysis_run_id=$2 AND sentence_ordinal=0 AND token_ordinal=0`,
		owner.ID, written.RunID).Scan(&firstStart, &firstEnd))
	require.NoError(t, pool.QueryRow(ctx, `SELECT start_offset, end_offset FROM corpus_tokens
		WHERE owner_id=$1 AND analysis_run_id=$2 AND sentence_ordinal=0 AND token_ordinal=1`,
		owner.ID, written.RunID).Scan(&secondStart, &secondEnd))
	_, err = pool.Exec(ctx, `INSERT INTO occurrence_lemma_corrections(
		owner_id, book_id, corpus_id, analysis_run_id, source_document_id,
		start_offset, end_offset, canonical_lemma, normalization_profile, normalization_version, excluded)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'λέγω','ancient-greek','1',false),
		       ($1,$2,$3,$4,$5,$8,$9,NULL,NULL,NULL,true)`,
		owner.ID, registration.BookID, seed.corpusID, written.RunID, unit.UnitID,
		firstStart, firstEnd, secondStart, secondEnd)
	require.NoError(t, err)

	_, err = store.PutKnownVocabularyIdentity(ctx, owner.ID, classical.Language, "λέγω", "VERB", "morpheus:1")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO reading_history(owner_id, language, book_id, completed_at)
		VALUES($1,$2,$3,now())`, owner.ID, classical.Language, registration.BookID)
	require.NoError(t, err)

	require.NoError(t, store.SetActiveStudyLanguage(ctx, owner.ID, classical.Language))
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, registration.BookID, domain.BookDispositionToRead))
	started, err := store.StartCurrentReadingResult(ctx, owner.ID, classical.Language, registration.BookID)
	require.NoError(t, err)
	seed.snapshotID = started.Reading.SnapshotID
	require.NotEmpty(t, seed.snapshotID)

	// One installation-wide translation row the conversion never touches.
	seed.brownsonPin = "f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7"
	_, err = pool.Exec(ctx, `INSERT INTO reference_translations(translation_pin_sha256, language, citation, translation)
		VALUES($1,'en','1.1.1','Cyrus speaks.')`, seed.brownsonPin)
	require.NoError(t, err)

	// A prepared deck whose frozen passage evidence must survive untouched.
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: seed.sourceID, AnalysisRunID: written.RunID,
		SnapshotID: seed.snapshotID, Filename: "anabasis.apkg", DeckName: "Mouseion::grc::Anabasis",
		ContentHash: "sha256:pre-upgrade",
	})
	require.NoError(t, err)
	seed.deckID = preparation.ID
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO deck_preparation_runs(
			owner_id, preparation_id, run_number, state, translation_state,
			external_translation_consent, external_translation_configured,
			manifest_schema_version, retry_policy_version, max_provider_attempts,
			max_batch_generations, batch_max_requests, batch_max_bytes, candidate_count,
			completed_count, completed_at, translation_completed_at)
		VALUES($1,$2,1,'completed','completed',false,false,1,1,1,1,1,1,1,1,now(),now())
		RETURNING id::text`, owner.ID, preparation.ID).Scan(&seed.deckRunID))
	seed.passageContext = "Cyrus speaks."
	seed.passageCitation = "1.1.1"
	_, err = pool.Exec(ctx, `
		INSERT INTO deck_preparation_manifests(
			owner_id, preparation_id, run_id, schema_version, manifest_digest,
			deck_name, filename, selected_count, accepted_count, omitted_count)
		VALUES($1,$2,$3,1,$4,'Mouseion::grc::Anabasis','anabasis.apkg',1,1,0)`,
		owner.ID, preparation.ID, seed.deckRunID, strings.Repeat("b", 64))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO deck_preparation_manifest_items(
			owner_id, preparation_id, run_id, ordinal, disposition, language, canonical_lemma, upos,
			source_sentence, tested_target, first_encounter, quality_score, quality_reasons,
			render_payload, candidate_digest, target_language, quality_gdex_score,
			passage_context, passage_citation, brownson_pin)
		VALUES($1,$2,$3,0,'accepted','grc','λόγοσ','NOUN','λέγει Κῦρος·','λόγος',1,50,'{}','{}',
		       $4,'en',0,$5,$6,$7)`,
		owner.ID, preparation.ID, seed.deckRunID, strings.Repeat("a", 64),
		seed.passageContext, seed.passageCitation, seed.brownsonPin)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE deck_preparations SET state='ready', artifact=decode('00','hex'),
		total_cards=1, completed_at=now(), current_run_id=$3 WHERE owner_id=$1 AND id=$2`,
		owner.ID, preparation.ID, seed.deckRunID)
	require.NoError(t, err)

	return seed
}

func TestConvertLegacyAnabasisBookPreservesEveryPieceOfState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	seed := seedPreUpgradeAnabasis(t, ctx, store, pool)

	// Startup order: reconcile the manifest first, then convert the Book.
	require.NoError(t, store.ReconcileClassicalManifest(ctx, []classical.Definition{seed.definition}))
	report, err := ConvertLegacyClassicalBooks(ctx, store, []classical.Definition{seed.definition})
	require.NoError(t, err)
	assert.Equal(t, 1, report.Converted)
	assert.Zero(t, report.EvidenceDiffers)

	// The source is now keyed by the Reading unit and its current content is
	// only Book 1.
	source, err := store.GetSourceMaterial(ctx, seed.ownerID, seed.sourceID)
	require.NoError(t, err)
	assert.Equal(t, seed.definition.Key, source.SourceIdentifier, "the source identifier is the Reading-unit key")
	assert.NotEqual(t, seed.oldRevisionID, source.ContentRevisionID)
	assert.Equal(t, source.FullText, string(source.Content))
	assert.Contains(t, source.FullText, "λέγει")
	assert.NotContains(t, source.FullText, "Ἀρταξέρξης")
	convertedAgain, err := store.ConvertLegacyClassicalSource(ctx, domain.ClassicalSourceConversion{
		OwnerID: seed.ownerID, BookID: seed.bookID, SourceMaterialID: seed.sourceID,
		ReadingUnitKey: seed.definition.Key, UnitID: "classical-unit-v1:" + seed.definition.Key,
		UnitText: source.FullText, SourceFileName: seed.definition.Source.FileName,
		DefinitionRevision: seed.definition.Revision, SourceFileSHA256: seed.definition.Source.SHA256,
		InterpretationVersion: seed.definition.InterpretationVersion, AnalysisRunID: seed.runID,
		ExpectedContentRevisionID: seed.oldRevisionID, ExpectedSnapshotID: seed.oldSnapshotID,
	})
	require.NoError(t, err)
	assert.False(t, convertedAgain, "a converted source is a no-op")

	// The completed run is re-pointed at the unit revision and snapshot, and
	// its corpus and evidence are the same rows.
	var runRevision, runSnapshot, corpusID, preparedRun string
	require.NoError(t, pool.QueryRow(ctx, `SELECT content_revision_id::text, snapshot_id::text, corpus_id::text
		FROM analysis_runs WHERE owner_id=$1 AND id=$2`, seed.ownerID, seed.runID).Scan(&runRevision, &runSnapshot, &corpusID))
	assert.Equal(t, source.ContentRevisionID, runRevision)
	assert.NotEqual(t, seed.oldRevisionID, runRevision)
	assert.NotEqual(t, seed.oldSnapshotID, runSnapshot)
	assert.Equal(t, seed.corpusID, corpusID, "the corpus is unchanged")

	// The prepared-definition record is current, and no second run exists.
	var revision, preparedCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT definition_revision, analysis_run_id::text
		FROM classical_prepared_definitions WHERE owner_id=$1 AND book_id=$2`,
		seed.ownerID, seed.bookID).Scan(&revision, &preparedRun))
	assert.Equal(t, seed.definition.Revision, revision)
	assert.Equal(t, seed.runID, preparedRun)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1`, seed.ownerID).Scan(&preparedCount))
	assert.Equal(t, 1, preparedCount, "conversion adds no analysis run")

	// Learner decisions, vocabulary, completion and the current reading's
	// frozen evidence survive.
	var corrections, exclusions, known, completed int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE NOT excluded), count(*) FILTER (WHERE excluded)
		FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2`, seed.ownerID, seed.bookID).Scan(&corrections, &exclusions))
	assert.Equal(t, 1, corrections)
	assert.Equal(t, 1, exclusions)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, seed.ownerID).Scan(&known))
	assert.Equal(t, 1, known)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, seed.ownerID, seed.bookID).Scan(&completed))
	assert.Equal(t, 1, completed)
	current, err := store.GetCurrentReading(ctx, seed.ownerID, classical.Language)
	require.NoError(t, err)
	assert.Equal(t, seed.bookID, current.BookID)
	assert.Equal(t, seed.snapshotID, current.SnapshotID, "the current reading keeps its frozen snapshot")
	_, err = store.ListCurrentReadingSnapshotVocabulary(ctx, seed.ownerID, seed.snapshotID)
	require.NoError(t, err, "the frozen snapshot remains readable")

	// The deck's frozen passage text, translation pin and evidence are exactly
	// what they were before conversion.
	var passageContext, passageCitation, brownsonPin, sourceSentence string
	require.NoError(t, pool.QueryRow(ctx, `SELECT passage_context, passage_citation, brownson_pin, source_sentence
		FROM deck_preparation_manifest_items WHERE owner_id=$1 AND preparation_id=$2`,
		seed.ownerID, seed.deckID).Scan(&passageContext, &passageCitation, &brownsonPin, &sourceSentence))
	assert.Equal(t, seed.passageContext, passageContext)
	assert.Equal(t, seed.passageCitation, passageCitation)
	assert.Equal(t, seed.brownsonPin, brownsonPin)
	assert.Equal(t, "λέγει Κῦρος·", sourceSentence)
	var translation string
	require.NoError(t, pool.QueryRow(ctx, `SELECT translation FROM reference_translations WHERE translation_pin_sha256=$1 AND citation='1.1.1'`, seed.brownsonPin).Scan(&translation))
	assert.Equal(t, "Cyrus speaks.", translation)

	// A second startup changes nothing.
	var beforeRevisions, beforeSnapshots, beforeRuns, beforePrepared int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, seed.ownerID, seed.sourceID).Scan(&beforeRevisions))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, seed.ownerID, seed.sourceID).Scan(&beforeSnapshots))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1`, seed.ownerID).Scan(&beforeRuns))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM classical_prepared_definitions WHERE owner_id=$1`, seed.ownerID).Scan(&beforePrepared))
	again, err := ConvertLegacyClassicalBooks(ctx, store, []classical.Definition{seed.definition})
	require.NoError(t, err)
	assert.Zero(t, again.Converted)
	var afterRevisions, afterSnapshots, afterRuns, afterPrepared int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, seed.ownerID, seed.sourceID).Scan(&afterRevisions))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM source_material_unit_snapshots WHERE owner_id=$1 AND source_material_id=$2`, seed.ownerID, seed.sourceID).Scan(&afterSnapshots))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1`, seed.ownerID).Scan(&afterRuns))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM classical_prepared_definitions WHERE owner_id=$1`, seed.ownerID).Scan(&afterPrepared))
	assert.Equal(t, beforeRevisions, afterRevisions)
	assert.Equal(t, beforeSnapshots, afterSnapshots)
	assert.Equal(t, beforeRuns, afterRuns)
	assert.Equal(t, beforePrepared, afterPrepared)
	legacy, err := store.ListLegacyLanguageRows(ctx, classical.Language)
	require.NoError(t, err)
	assert.Empty(t, legacy, "a converted Book no longer blocks the catalog")
}

func TestConversionEvidenceDiffersLeavesAPublishedAnalysisAndAnUpdateOffer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	seed := seedPreUpgradeAnabasis(t, ctx, store, pool)
	require.NoError(t, store.ReconcileClassicalManifest(ctx, []classical.Definition{seed.definition}))

	// Change one published sentence, so the current interpretation reproduces
	// different evidence.
	_, err = pool.Exec(ctx, `UPDATE corpus_sentences SET sentence_text='different' WHERE owner_id=$1 AND analysis_run_id=$2 AND sentence_ordinal=0`,
		seed.ownerID, seed.runID)
	require.NoError(t, err)

	report, err := ConvertLegacyClassicalBooks(ctx, store, []classical.Definition{seed.definition})
	require.NoError(t, err)
	assert.Zero(t, report.Converted)
	assert.Equal(t, 1, report.EvidenceDiffers)

	// The published analysis and whole-file source are untouched.
	source, err := store.GetSourceMaterial(ctx, seed.ownerID, seed.sourceID)
	require.NoError(t, err)
	assert.Equal(t, seed.definition.Source.FileName, source.SourceIdentifier)
	assert.Equal(t, seed.oldRevisionID, source.ContentRevisionID)
	current, err := store.GetCurrentReading(ctx, seed.ownerID, classical.Language)
	require.NoError(t, err)
	assert.Equal(t, seed.runID, current.AnalysisRunID)

	// The persistence conversion refuses to touch evidence the comparison did
	// not name, and an already-converted source is a no-op.
	conversion := domain.ClassicalSourceConversion{
		OwnerID: seed.ownerID, BookID: seed.bookID, SourceMaterialID: seed.sourceID,
		ReadingUnitKey: seed.definition.Key, UnitID: "classical-unit-v1:" + seed.definition.Key,
		UnitText: "text", SourceFileName: seed.definition.Source.FileName,
		DefinitionRevision: seed.definition.Revision, SourceFileSHA256: seed.definition.Source.SHA256,
		InterpretationVersion: seed.definition.InterpretationVersion, AnalysisRunID: seed.runID,
		ExpectedContentRevisionID: "different", ExpectedSnapshotID: seed.oldSnapshotID,
	}
	_, err = store.ConvertLegacyClassicalSource(ctx, conversion)
	require.ErrorIs(t, err, persistence.ErrClassicalConversionChanged)
	conversion.ExpectedContentRevisionID = seed.oldRevisionID
	conversion.BookID = "00000000-0000-0000-0000-000000000000"
	_, err = store.ConvertLegacyClassicalSource(ctx, conversion)
	require.ErrorIs(t, err, persistence.ErrClassicalConversionChanged)

	// The Book keeps the ordinary Analysis update offer through its missing
	// prepared definition.
	service := NewService(store.Pool(), nil)
	candidates, err := service.AnalysisUpdateCandidates(ctx, seed.ownerID, []string{seed.sourceID})
	require.NoError(t, err)
	candidate, ok := candidates[seed.sourceID]
	require.True(t, ok, "a differing conversion offers an Analysis update")
	assert.Equal(t, seed.runID, candidate.PublishedRunID)
	assert.NotEmpty(t, candidate.Target.Key())

	// The catalog stays fail-closed while the conversion is incomplete, and
	// the Add path names the stable code.
	legacy, err := store.ListLegacyLanguageRows(ctx, classical.Language)
	require.NoError(t, err)
	require.NotEmpty(t, legacy)
	_, err = store.AddClassicalReadingUnits(ctx, seed.ownerID, []string{seed.definition.Key})
	require.ErrorIs(t, err, classical.ErrLegacyRows)

	// Preparation is fail-closed for the same reason. Complete the conversion's
	// source-identity rewrite so the update path can be exercised; the
	// differing evidence itself stays as it was.
	_, err = pool.Exec(ctx, `UPDATE source_materials sm
		SET source_identifier = ru.definition_key
		FROM book_reading_units bru
		JOIN reading_units ru ON ru.id = bru.reading_unit_id
		WHERE sm.owner_id = bru.owner_id AND sm.book_id = bru.book_id
		  AND sm.owner_id = $1 AND sm.id = $2`, seed.ownerID, seed.sourceID)
	require.NoError(t, err)
	legacy, err = store.ListLegacyLanguageRows(ctx, classical.Language)
	require.NoError(t, err)
	require.Empty(t, legacy)

	// Confirming the offer revalidates it and queues the dedicated preparation
	// job once the Book is not the Current reading.
	require.NoError(t, MigrateRiver(ctx, pool))
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "classical update client", client.Stop)
	updates := NewService(store.Pool(), client).WithClassicalPreparationStore(store)
	_, err = updates.SubmitAnalysisUpdate(ctx, seed.ownerID, seed.bookID, seed.sourceID, candidate.PublishedRunID, candidate.Target.Key())
	require.ErrorIs(t, err, ErrCurrentBook)

	require.NoError(t, store.EndCurrentReading(ctx, seed.ownerID, classical.Language, seed.bookID, current.SnapshotID))
	_, err = updates.SubmitAnalysisUpdate(ctx, seed.ownerID, seed.bookID, seed.sourceID, candidate.PublishedRunID, "stale-target")
	require.ErrorIs(t, err, ErrAnalysisUpdateChanged)

	require.NoError(t, store.SetBookDisposition(ctx, seed.ownerID, seed.bookID, domain.BookDispositionInbox))
	_, err = updates.SubmitAnalysisUpdate(ctx, seed.ownerID, seed.bookID, seed.sourceID, candidate.PublishedRunID, candidate.Target.Key())
	require.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, store.SetBookDisposition(ctx, seed.ownerID, seed.bookID, domain.BookDispositionToRead))
	handle, err := updates.SubmitAnalysisUpdate(ctx, seed.ownerID, seed.bookID, seed.sourceID, candidate.PublishedRunID, candidate.Target.Key())
	require.NoError(t, err)
	assert.NotZero(t, handle.JobID, "a confirmed classical update queues preparation")

	// A candidate-load failure is reported to the caller so startup can keep
	// the catalog fail-closed rather than opening on unverified evidence.
	_, err = pool.Exec(ctx, `DROP TABLE source_materials CASCADE`)
	require.NoError(t, err)
	_, err = ConvertLegacyClassicalBooks(ctx, store, []classical.Definition{seed.definition})
	require.Error(t, err)
	_, err = updates.AnalysisUpdateCandidates(ctx, seed.ownerID, []string{seed.sourceID})
	require.Error(t, err)
}
