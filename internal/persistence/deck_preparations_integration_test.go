//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeckPreparationPersistence(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))

	alice, err := store.CreateUser(ctx, "prep-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "prep-bob", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "prep-book", Title: "Book", MediaType: "text/plain", ContentHash: "prep-hash", Content: []byte("Buch"), FullText: "Buch"})
	require.NoError(t, err)

	created, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: alice.ID, SourceMaterialID: source.ID, Filename: "book.apkg", DeckName: "Mouseion::de::Book", ContentHash: source.ContentHash})
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationQueued, created.State, "create")
	repeated, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: alice.ID, SourceMaterialID: source.ID, Filename: "changed.apkg", DeckName: "changed", ContentHash: source.ContentHash})
	require.NoError(t, err)
	assert.Equal(t, created.ID, repeated.ID, "idempotent create")
	assert.Equal(t, created.Filename, repeated.Filename, "idempotent create")
	_, err = store.GetDeckPreparation(ctx, bob.ID, created.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Owner isolation is checked independently from the transition cases below.
	_, err = store.DownloadDeckPreparation(ctx, alice.ID, created.ID)
	assert.ErrorIs(t, err, ErrInvalidTransition) //nolint:testifylint // Invalid download and owner-isolation cases are independent.
	_, err = store.DownloadDeckPreparation(ctx, bob.ID, created.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Owner isolation is an independent transition boundary check.

	claimed, err := store.ClaimDeckPreparation(ctx, alice.ID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationPreparing, claimed.State, "claim")
	assert.NotNil(t, claimed.StartedAt, "claim")
	_, err = store.ClaimDeckPreparation(ctx, alice.ID, created.ID)
	assert.ErrorIs(t, err, ErrInvalidTransition) //nolint:testifylint // Repeated claim is an independent lifecycle rejection case.
	readyInput := domain.DeckPreparation{Artifact: []byte("apkg"), Filename: "book.apkg", DeckName: "Mouseion::de::Book", TotalCards: 4, CardsWithEnglish: 3, CardsWithContextualSentenceTranslations: 2, QualityOmissions: 1}
	ready, err := store.CompleteDeckPreparation(ctx, alice.ID, created.ID, readyInput)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, ready.State, "complete")
	assert.NotNil(t, ready.CompletedAt, "complete")
	assert.Equal(t, cardexport.RenderInputVersion, ready.RenderInputVersion, "complete")
	assert.Equal(t, cardexport.PresentationVersion, ready.PresentationVersion, "complete")
	downloaded, err := store.DownloadDeckPreparation(ctx, alice.ID, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "apkg", string(downloaded.Artifact), "download")
	assert.Equal(t, 4, downloaded.TotalCards, "download")
	assert.Equal(t, cardexport.RenderInputVersion, downloaded.RenderInputVersion, "download")
	assert.Equal(t, cardexport.PresentationVersion, downloaded.PresentationVersion, "download")
	_, err = store.CompleteDeckPreparation(ctx, alice.ID, created.ID, readyInput)
	require.NoError(t, err, "idempotent complete")
	changed := readyInput
	changed.Artifact = []byte("different")
	_, err = store.CompleteDeckPreparation(ctx, alice.ID, created.ID, changed)
	assert.ErrorIs(t, err, ErrImmutable) //nolint:testifylint // Immutable update rejection and invalid retry are independent lifecycle cases.
	_, err = store.RetryDeckPreparation(ctx, alice.ID, created.ID)
	assert.ErrorIs(t, err, ErrInvalidTransition) //nolint:testifylint // Immutable update rejection and invalid retry are independent lifecycle cases.

	failed := createPreparation(t, ctx, store, alice.ID, source.ID, "failed-hash")
	_, err = store.ClaimDeckPreparation(ctx, alice.ID, failed.ID)
	require.NoError(t, err)
	failed, err = store.FailDeckPreparation(ctx, alice.ID, failed.ID, "provider unavailable")
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationFailed, failed.State, "fail")
	assert.NotEmpty(t, failed.Error, "fail")
	retried, err := store.RetryDeckPreparation(ctx, alice.ID, failed.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationQueued, retried.State, "retry")
	assert.Empty(t, retried.Error, "retry")
	assert.Nil(t, retried.StartedAt, "retry")
	assert.Nil(t, retried.CompletedAt, "retry")
	cancelled, err := store.CancelDeckPreparation(ctx, alice.ID, retried.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationCancelled, cancelled.State, "cancel")
	_, err = store.RetryDeckPreparation(ctx, alice.ID, cancelled.ID)
	require.NoError(t, err, "retry cancelled")

	var generated int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1`, alice.ID).Scan(&generated)
	require.NoError(t, err)
	assert.Zero(t, generated, "terminal preparations created exclusions")
}

func TestCompletePreparedDeckAtomicallyPersistsArtifactAndProvenance(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "atomic-prep", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "atomic-book", Title: "Atomic Book", MediaType: "text/plain", ContentHash: "atomic-hash", Content: []byte("Haus"), FullText: "Haus"})
	require.NoError(t, err)
	p := createPreparation(t, ctx, store, owner.ID, source.ID, source.ContentHash)
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, p.ID)
	require.NoError(t, err)
	record := func(lemma string) cardexport.GeneratedRecord {
		return cardexport.GeneratedRecord{Input: cardexport.RenderInput{Language: "de", CanonicalLemma: lemma, UPOS: "NOUN"}, Note: cardexport.Note{Key: lemma, Text: lemma + " front", BackExtra: lemma + " back", BookTitle: "Atomic Book"}}
	}
	invalid := record("Invalid")
	invalid.Note.BookTitle = "\x00"
	bad := cardexport.Artifact{APKG: []byte("bad"), Filename: "bad.apkg", DeckName: "Mouseion::de::Atomic Book", Generated: []cardexport.GeneratedRecord{record("Haus"), invalid}, Completeness: cardexport.Completeness{TotalCards: 2}}
	_, err = store.CompletePreparedDeck(ctx, owner.ID, p.ID, bad)
	assert.Error(t, err, "expected rollback error") //nolint:testifylint // The following queries independently verify rollback completeness.
	var cards, generated int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, owner.ID).Scan(&cards)
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&generated)
	require.NoError(t, err)
	assert.Zero(t, cards, "partial provenance after failure")
	assert.Zero(t, generated, "partial provenance after failure")
	artifact := cardexport.Artifact{APKG: []byte("apkg"), Filename: "atomic.apkg", DeckName: "Mouseion::de::Atomic Book", Generated: []cardexport.GeneratedRecord{record("Haus"), record("Baum")}, Completeness: cardexport.Completeness{TotalCards: 2, CardsWithEnglish: 2, CardsWithEnglishSentence: 1}}
	ready, err := store.CompletePreparedDeck(ctx, owner.ID, p.ID, artifact)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, ready.State, "complete")
	assert.Equal(t, 2, ready.TotalCards, "complete")
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, owner.ID).Scan(&cards)
	require.NoError(t, err)
	assert.Equal(t, 2, cards)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND first_source_material_id=$2`, owner.ID, source.ID).Scan(&generated)
	require.NoError(t, err)
	assert.Equal(t, 2, generated)
	_, err = store.CompletePreparedDeck(ctx, owner.ID, p.ID, artifact)
	require.NoError(t, err, "idempotent completion")
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, owner.ID).Scan(&cards)
	require.NoError(t, err)
	assert.Equal(t, 2, cards, "duplicate cards")
	var assignments int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM deck_preparation_vocabulary WHERE owner_id=$1 AND deck_preparation_id=$2`, owner.ID, p.ID).Scan(&assignments)
	require.NoError(t, err)
	assert.Equal(t, 2, assignments, "duplicate provenance assignments")
}

func TestDeckPreparationReanalysisRetiresPreviousBookDeck(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "current-deck-owner", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Reanalyzed Book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "current-deck-book", Title: book.Title, MediaType: "application/epub+zip", Content: []byte("eins"), FullText: "eins"}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "unit-1"), Order: 0, SpineIndex: 0, ManifestID: "unit-1", Text: "eins", StartOffset: 0, EndOffset: 4}}})
	require.NoError(t, err)
	err = store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner.ID, source.ID).Scan(&source.ContentSnapshotID)
	require.NoError(t, err)
	err = store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID)
	require.NoError(t, err)
	var firstRun, secondRun string
	for i, runID := range []*string{&firstRun, &secondRun} {
		err = store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'test','1',$5,'completed',now()) RETURNING id::text`, owner.ID, source.ID, source.ContentRevisionID, source.ContentSnapshotID, string(rune('a'+i))).Scan(runID)
		require.NoError(t, err)
	}
	var deckID string
	err = store.Pool().QueryRow(ctx, `INSERT INTO decks(owner_id,language,name) VALUES($1,'de','current-deck-test') RETURNING id`, owner.ID).Scan(&deckID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_source_material_id) VALUES($1,'de','eins','NUM',$2,$3)`, owner.ID, deckID, source.ID)
	require.NoError(t, err)
	first, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: firstRun, Filename: "one.apkg", DeckName: "One", ContentHash: source.ContentHash})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_vocabulary(owner_id,deck_preparation_id,language,canonical_lemma,upos,generated_at) VALUES($1,$2,'de','eins','NUM',now())`, owner.ID, first.ID)
	require.NoError(t, err)
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, first.ID)
	require.NoError(t, err)
	_, err = store.CompleteDeckPreparation(ctx, owner.ID, first.ID, domain.DeckPreparation{Artifact: []byte("one-apkg"), Filename: "one.apkg", DeckName: "One", TotalCards: 1})
	require.NoError(t, err)
	second, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: secondRun, Filename: "two.apkg", DeckName: "Two", ContentHash: source.ContentHash})
	require.NoError(t, err)
	first, err = store.GetDeckPreparation(ctx, owner.ID, first.ID)
	require.NoError(t, err)
	assert.NotEqual(t, second.ID, first.ID, "current deck transition")
	assert.Nil(t, second.RetiredAt, "current deck transition")
	assert.NotNil(t, first.RetiredAt, "current deck transition")
	var current, history int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FILTER (WHERE retired_at IS NULL), count(*) FROM deck_preparations WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&current, &history)
	require.NoError(t, err)
	assert.Equal(t, 1, current, "one current deck")
	assert.Equal(t, 2, history, "two historical rows")
	retry, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: secondRun, Filename: "changed.apkg", DeckName: "Changed", ContentHash: "changed"})
	require.NoError(t, err)
	assert.Equal(t, second.ID, retry.ID, "analysis retry")
}

func TestDeckPreparationRepreparationCreatesCurrentAnalysisGeneration(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "reprepare-owner", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Reprepare Book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "reprepare-book", Title: book.Title, MediaType: "application/epub+zip", Content: []byte("eins"), FullText: "eins"}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "unit-1"), Order: 0, SpineIndex: 0, ManifestID: "unit-1", Text: "eins", StartOffset: 0, EndOffset: 4}}})
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID))
	err = store.Pool().QueryRow(ctx, `SELECT current_content_revision_id::text,current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner.ID, source.ID).Scan(&source.ContentRevisionID, &source.ContentSnapshotID)
	require.NoError(t, err)
	var runID string
	err = store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'test','1','reprepare','completed',now()) RETURNING id::text`, owner.ID, source.ID, source.ContentRevisionID, source.ContentSnapshotID).Scan(&runID)
	require.NoError(t, err)
	old, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: runID, Filename: "old.apkg", DeckName: "Old", ContentHash: source.ContentHash})
	require.NoError(t, err)
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, old.ID)
	require.NoError(t, err)
	_, err = store.CompleteDeckPreparation(ctx, owner.ID, old.ID, domain.DeckPreparation{Artifact: []byte("old-artifact"), Filename: "old.apkg", DeckName: "Old", TotalCards: 1})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET error=$1 WHERE owner_id=$2 AND id=$3`, domain.DeckPreparationRequiresRepreparationError, owner.ID, old.ID)
	require.NoError(t, err)
	oldArtifact, err := store.DownloadDeckPreparation(ctx, owner.ID, old.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("old-artifact"), oldArtifact.Artifact)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET retired_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, old.ID)
	require.NoError(t, err)

	oldAfter, err := store.GetDeckPreparation(ctx, owner.ID, old.ID)
	require.NoError(t, err)
	assert.NotNil(t, oldAfter.RetiredAt, "the old ready row is retained as history after it is superseded")
	current, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: runID, Filename: "new.apkg", DeckName: "New", ContentHash: source.ContentHash})
	require.NoError(t, err)
	assert.NotEqual(t, old.ID, current.ID)
	assert.Equal(t, runID, current.AnalysisRunID)
	assert.Nil(t, current.RetiredAt)

	repeated, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: runID, Filename: "changed.apkg", DeckName: "Changed", ContentHash: "changed"})
	require.NoError(t, err)
	assert.Equal(t, current.ID, repeated.ID, "the current generation remains idempotent")
}

func createPreparation(t *testing.T, ctx context.Context, store *PostgresStore, owner, source, hash string) domain.DeckPreparation {
	t.Helper()
	p, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner, SourceMaterialID: source, Filename: hash + ".apkg", DeckName: hash, ContentHash: hash})
	require.NoError(t, err)
	return p
}
