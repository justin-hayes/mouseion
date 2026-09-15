//go:build integration

package cardexport_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportCoverageGeneratedAndKnownExclusionsEndToEnd(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()
	alice, err := store.CreateUser(ctx, "export-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "export-bob", false)
	require.NoError(t, err)
	type fixtureCandidate struct {
		lemma, sentence string
		occurrences     int
		firstEncounter  int
	}
	seedBook := func(owner domain.User, hash, title string, candidates ...fixtureCandidate) string {
		t.Helper()
		artifact := domain.NormalizedArtifact{ContentHash: hash, Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
		lemmas := make([]domain.SharedLemma, 0, len(candidates))
		for _, candidate := range candidates {
			lemmas = append(lemmas, domain.SharedLemma{CanonicalLemma: candidate.lemma, UPOS: "NOUN", Morphology: []byte(`{"Gender":"Neut"}`), Frequency: int64(candidate.occurrences)})
		}
		err := store.PutArtifact(ctx, artifact, lemmas)
		require.NoError(t, err)
		source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: hash, Title: title, MediaType: "text/plain", ContentHash: hash, Content: []byte(title), FullText: title})
		require.NoError(t, err)
		corpus, err := store.PutCorpus(ctx, owner.ID, source.ID, hash)
		require.NoError(t, err)
		for index, candidate := range candidates {
			location := []byte(fmt.Sprintf(`{"start_offset":%d}`, candidate.firstEncounter))
			err = store.ReplaceSelectedSentences(ctx, owner.ID, corpus.ID, "de", candidate.lemma, "NOUN", []domain.ExampleSentence{{SentenceKey: fmt.Sprintf("s%d", index), Text: candidate.sentence, SourceLocation: location, SelectionReasons: []byte(`[]`), SelectionRank: 1, Chosen: true}})
			require.NoError(t, err)
			references := []byte(fmt.Sprintf(`[{"location":{"start_offset":%d},"text":%q}]`, candidate.firstEncounter, candidate.sentence))
			_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{OwnerID: owner.ID, CorpusID: corpus.ID, Language: "de", CanonicalLemma: candidate.lemma, UPOS: "NOUN", OccurrenceCount: candidate.occurrences, ObservedForms: []byte(`["` + candidate.lemma + `"]`), SentenceReferences: references, Provenance: []byte(fmt.Sprintf(`{"min_occurrences":1,"occurrence_count":%d}`, candidate.occurrences))})
			require.NoError(t, err)
		}
		return source.ID
	}
	aliceBookA := seedBook(alice, "export-a", "Book A", fixtureCandidate{"Haus", "Das alte Haus ist überraschend groß.", 3, 10})
	_, err = pool.Exec(ctx, `UPDATE selection_candidates SET eligible_sentence_refs='[{"location":{"start_offset":1},"text":"Inhaltsverzeichnis: Das Haus und seine Geschichte ..... 12."},{"location":{"start_offset":10},"text":"Das alte Haus ist überraschend groß."}]' WHERE owner_id=$1 AND canonical_lemma='Haus' AND corpus_id=(SELECT id::text FROM corpora WHERE owner_id=$1 AND source_material_id=$2)`, alice.ID, aliceBookA)
	require.NoError(t, err)
	aliceBookB := seedBook(alice, "export-b", "Book B",
		fixtureCandidate{"Haus", "Dieses alte Haus steht noch am Stadtrand.", 100, 10},
		fixtureCandidate{"Welt", "Die ganze Welt ist wirklich sehr groß.", 100, 20},
		fixtureCandidate{"Baum", "Der alte Baum trägt viele grüne Blätter.", 96, 40},
		fixtureCandidate{"Weg", "Der schmale Weg führt durch den Wald.", 4, 30},
		fixtureCandidate{"Himmel", "Der blaue Himmel leuchtet über dem Dorf.", 3, 50},
		fixtureCandidate{"Karte", "Sie zeichnet eine Karte für die Reise.", 2, 60},
		fixtureCandidate{"Rand", "Am Rand des Waldes beginnt ein Feld.", 1, 70},
		fixtureCandidate{"Stern", "Ein heller Stern steht über dem Dorf.", 3, 80},
	)
	reservationPreparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: alice.ID, SourceMaterialID: aliceBookB, Filename: "reservation.apkg", DeckName: "Reservation", ContentHash: "reservation"})
	require.NoError(t, err)
	_, err = store.ClaimDeckPreparation(ctx, alice.ID, reservationPreparation.ID)
	require.NoError(t, err)
	_, err = store.CompleteDeckPreparation(ctx, alice.ID, reservationPreparation.ID, domain.DeckPreparation{Artifact: []byte("reservation-artifact"), Filename: reservationPreparation.Filename, DeckName: reservationPreparation.DeckName, TotalCards: 1})
	require.NoError(t, err)
	reservationDeck, err := store.PutDeck(ctx, alice.ID, "de", "Reservation")
	require.NoError(t, err)
	_, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: "Himmel", UPOS: "NOUN", FirstDeckID: reservationDeck.ID, FirstSourceMaterialID: &aliceBookB})
	require.NoError(t, err)
	_, err = store.StartDeckVocabularyStudy(ctx, alice.ID, reservationPreparation.ID)
	require.NoError(t, err)
	reservedVocabulary, err := store.ListReservedVocabulary(ctx, alice.ID, "de")
	require.NoError(t, err)
	require.Len(t, reservedVocabulary, 1)
	assert.Equal(t, "Himmel", reservedVocabulary[0].CanonicalLemma)
	bobBook := seedBook(bob, "export-bob", "Bob's Book", fixtureCandidate{"Haus", "Bobs neues Haus steht nah am Fluss.", 1, 10})
	_, err = store.PutKnownVocabulary(ctx, alice.ID, "de", "Welt", "NOUN")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO enrichment_cache(language,canonical_lemma,upos,provider,provider_version,sentence_hash,translation,fallback_gloss,sentence_translation) VALUES('de','Haus','NOUN','test','1',$1,'house','a dwelling','This translation belongs to another sentence.')`, enrichment.SentenceHash("Dieses alte Haus steht noch am Stadtrand."))
	require.NoError(t, err)

	artifact, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, aliceBookA)
	require.NoError(t, err)
	assert.Equal(t, 1, artifact.Count)
	assert.Equal(t, 0, artifact.Completeness.CardsWithEnglishSentence)
	assert.False(t, contains(artifact.TSV, "This translation belongs to another sentence."))
	assert.True(t, contains(artifact.TSV, "Haus"))
	assert.False(t, contains(artifact.TSV, "Baum"))
	assert.False(t, contains(artifact.TSV, "Inhaltsverzeichnis"), "lower-quality first reference was selected")
	_, err = pool.Exec(ctx, `INSERT INTO enrichment_cache(language,canonical_lemma,upos,provider,provider_version,sentence_hash,translation,fallback_gloss,sentence_translation,sentence_translation_target) VALUES('de','Haus','NOUN','test','1',$1,'house','a dwelling','The old house is surprisingly large.','old house')`, enrichment.SentenceHash("Das alte Haus ist überraschend groß."))
	require.NoError(t, err)
	var cards, decks, audits, generated, known int
	var state string
	err = pool.QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, alice.ID).Scan(&cards)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM decks WHERE owner_id=$1`, alice.ID).Scan(&decks)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT state FROM vocabulary_states WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&state)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='vocabulary.transition' AND details->>'to'='generated'`, alice.ID).Scan(&audits)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus' AND first_source_material_id=$2`, alice.ID, aliceBookA).Scan(&generated)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, alice.ID).Scan(&known)
	require.NoError(t, err)
	assert.Equal(t, 1, cards)
	assert.Equal(t, 2, decks)
	assert.Equal(t, "generated", state)
	assert.Equal(t, 1, audits)
	assert.Equal(t, 1, generated)
	assert.Equal(t, 1, known)
	var firstDeck, firstGeneratedAt string
	err = pool.QueryRow(ctx, `SELECT first_deck_id::text,first_generated_at::text FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&firstDeck, &firstGeneratedAt)
	require.NoError(t, err)
	again, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, aliceBookA)
	require.NoError(t, err)
	assert.Equal(t, 1, again.Count)
	assert.Equal(t, cardexport.Completeness{TotalCards: 1, CardsWithEnglish: 1, CardsWithEnglishSentence: 1}, again.Completeness)
	assert.True(t, contains(again.TSV, "The <b>old house</b> is surprisingly large."))
	var generatedRows int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, alice.ID).Scan(&cards)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='vocabulary.transition' AND details->>'to'='generated'`, alice.ID).Scan(&audits)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus' AND first_source_material_id=$2`, alice.ID, aliceBookA).Scan(&generatedRows)
	require.NoError(t, err)
	assert.Equal(t, 1, cards)
	assert.Equal(t, 1, audits)
	assert.Equal(t, 1, generatedRows)
	var stableDeck, stableGeneratedAt string
	err = pool.QueryRow(ctx, `SELECT first_deck_id::text,first_generated_at::text FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&stableDeck, &stableGeneratedAt)
	require.NoError(t, err)
	assert.Equal(t, firstDeck, stableDeck, "generated provenance changed")
	assert.Equal(t, firstGeneratedAt, stableGeneratedAt, "generated provenance changed")

	secondBook, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, aliceBookB)
	require.NoError(t, err)
	assert.Equal(t, 3, secondBook.Count)
	assert.False(t, contains(secondBook.TSV, "Haus"))
	assert.False(t, contains(secondBook.TSV, "Welt"))
	assert.True(t, contains(secondBook.TSV, "Baum"))
	assert.True(t, contains(secondBook.TSV, "Weg"))
	assert.False(t, contains(secondBook.TSV, "Himmel"))
	assert.False(t, contains(secondBook.TSV, "Karte"))
	assert.False(t, contains(secondBook.TSV, "Rand"))
	assert.True(t, contains(secondBook.TSV, "Stern"))
	assert.Less(t, strings.Index(secondBook.TSV, "Weg"), strings.Index(secondBook.TSV, "Baum"), "book B TSV is not in first-encounter order")
	assert.Less(t, strings.Index(secondBook.TSV, "Baum"), strings.Index(secondBook.TSV, "Stern"), "book B TSV is not in first-encounter order")
	var bookBGenerated, aliceKnown int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND first_source_material_id=$2 AND canonical_lemma IN ('Baum','Weg','Stern')`, alice.ID, aliceBookB).Scan(&bookBGenerated)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Welt'`, alice.ID).Scan(&aliceKnown)
	require.NoError(t, err)
	assert.Equal(t, 3, bookBGenerated)
	assert.Equal(t, 0, aliceKnown)
	var bookBDeckCards int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM cards c JOIN decks d ON d.owner_id=c.owner_id AND d.id=c.deck_id WHERE d.owner_id=$1 AND d.name='Book B'`, alice.ID).Scan(&bookBDeckCards)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM decks WHERE owner_id=$1`, alice.ID).Scan(&decks)
	require.NoError(t, err)
	assert.Equal(t, 3, bookBDeckCards)
	assert.Equal(t, 3, decks)

	bobArtifact, err := cardexport.NewService(store).ExportCoverage(ctx, bob.ID, bobBook)
	require.NoError(t, err)
	assert.Equal(t, 0, bobArtifact.Count)
	assert.False(t, contains(bobArtifact.TSV, "Haus"))
	var bobGenerated int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus' AND first_source_material_id=$2`, bob.ID, bobBook).Scan(&bobGenerated)
	require.NoError(t, err)
	assert.Equal(t, 0, bobGenerated)

	badBook := seedBook(alice, "export-quality", "Quality Book", fixtureCandidate{"Fragment", "Fragment.", 3, 10})
	omitted, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, badBook)
	require.NoError(t, err)
	var omittedGenerated, omittedCards int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Fragment'`, alice.ID).Scan(&omittedGenerated)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1 AND canonical_lemma='Fragment'`, alice.ID).Scan(&omittedCards)
	require.NoError(t, err)
	assert.Equal(t, 0, omitted.Count)
	assert.Len(t, omitted.Omitted, 1)
	assert.Equal(t, 0, omittedGenerated)
	assert.Equal(t, 0, omittedCards)
	var qualityCorpus string
	err = pool.QueryRow(ctx, `SELECT id::text FROM corpora WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, badBook).Scan(&qualityCorpus)
	require.NoError(t, err)
	err = store.ReplaceSelectedSentences(ctx, alice.ID, qualityCorpus, "de", "Fragment", "NOUN", []domain.ExampleSentence{{SentenceKey: "quality-improved", Text: "Dieses Fragment enthält jetzt genügend hilfreichen Kontext.", SourceLocation: []byte(`{"start_offset":10}`), SelectionReasons: []byte(`[]`), SelectionRank: 1, Chosen: true}})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE selection_candidates SET eligible_sentence_refs='[{"location":{"start_offset":10},"text":"Dieses Fragment enthält jetzt genügend hilfreichen Kontext."}]' WHERE owner_id=$1 AND corpus_id=$2 AND canonical_lemma='Fragment'`, alice.ID, qualityCorpus)
	require.NoError(t, err)
	improved, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, badBook)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Fragment'`, alice.ID).Scan(&omittedGenerated)
	require.NoError(t, err)
	assert.Equal(t, 1, improved.Count)
	assert.Empty(t, improved.Omitted)
	assert.Equal(t, 1, omittedGenerated)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
