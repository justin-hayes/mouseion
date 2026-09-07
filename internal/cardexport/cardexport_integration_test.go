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
	"github.com/justin-hayes/mouseion/migrations"
)

func TestExportCoverageGeneratedAndKnownExclusionsEndToEnd(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, _ := store.CreateUser(ctx, "export-alice", false)
	bob, _ := store.CreateUser(ctx, "export-bob", false)
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
		if err := store.PutArtifact(ctx, artifact, lemmas); err != nil {
			t.Fatal(err)
		}
		source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: hash, Title: title, MediaType: "text/plain", ContentHash: hash, Content: []byte(title), FullText: title})
		if err != nil {
			t.Fatal(err)
		}
		corpus, err := store.PutCorpus(ctx, owner.ID, source.ID, hash)
		if err != nil {
			t.Fatal(err)
		}
		for index, candidate := range candidates {
			location := []byte(fmt.Sprintf(`{"start_offset":%d}`, candidate.firstEncounter))
			if err = store.ReplaceSelectedSentences(ctx, owner.ID, corpus.ID, "de", candidate.lemma, "NOUN", []domain.ExampleSentence{{SentenceKey: fmt.Sprintf("s%d", index), Text: candidate.sentence, SourceLocation: location, SelectionReasons: []byte(`[]`), SelectionRank: 1, Chosen: true}}); err != nil {
				t.Fatal(err)
			}
			references := []byte(fmt.Sprintf(`[{"location":{"start_offset":%d},"text":%q}]`, candidate.firstEncounter, candidate.sentence))
			if _, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{OwnerID: owner.ID, CorpusID: corpus.ID, Language: "de", CanonicalLemma: candidate.lemma, UPOS: "NOUN", OccurrenceCount: candidate.occurrences, ObservedForms: []byte(`["` + candidate.lemma + `"]`), SentenceReferences: references, Provenance: []byte(fmt.Sprintf(`{"min_occurrences":1,"occurrence_count":%d}`, candidate.occurrences))}); err != nil {
				t.Fatal(err)
			}
		}
		return source.ID
	}
	aliceBookA := seedBook(alice, "export-a", "Book A", fixtureCandidate{"Haus", "Das alte Haus ist überraschend groß.", 3, 10})
	if _, err = pool.Exec(ctx, `UPDATE selection_candidates SET eligible_sentence_refs='[{"location":{"start_offset":1},"text":"Inhaltsverzeichnis: Das Haus und seine Geschichte ..... 12."},{"location":{"start_offset":10},"text":"Das alte Haus ist überraschend groß."}]' WHERE owner_id=$1 AND canonical_lemma='Haus' AND corpus_id=(SELECT id::text FROM corpora WHERE owner_id=$1 AND source_material_id=$2)`, alice.ID, aliceBookA); err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ClaimDeckPreparation(ctx, alice.ID, reservationPreparation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompleteDeckPreparation(ctx, alice.ID, reservationPreparation.ID, domain.DeckPreparation{Artifact: []byte("reservation-artifact"), Filename: reservationPreparation.Filename, DeckName: reservationPreparation.DeckName}); err != nil {
		t.Fatal(err)
	}
	reservationDeck, err := store.PutDeck(ctx, alice.ID, "de", "Reservation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: "Himmel", UPOS: "NOUN", FirstDeckID: reservationDeck.ID, FirstSourceMaterialID: &aliceBookB}); err != nil {
		t.Fatal(err)
	}
	reservationCampaign, err := store.CreateLearningCampaign(ctx, alice.ID, aliceBookB, reservationPreparation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateLearningCampaignProgress(ctx, alice.ID, reservationCampaign.ID, persistence.LearningCampaignExpectedState{Status: reservationCampaign.Status, BookProgress: reservationCampaign.BookProgress, DeckProgress: reservationCampaign.DeckProgress}, domain.BookReading, domain.DeckStudying); err != nil {
		t.Fatal(err)
	}
	activeVocabulary, err := store.ListActiveLearningCampaignVocabulary(ctx, alice.ID, "de")
	if err != nil || len(activeVocabulary) != 1 || activeVocabulary[0].CanonicalLemma != "Himmel" {
		t.Fatalf("active reservation vocabulary=%+v err=%v", activeVocabulary, err)
	}
	bobBook := seedBook(bob, "export-bob", "Bob's Book", fixtureCandidate{"Haus", "Bobs neues Haus steht nah am Fluss.", 1, 10})
	if _, err = store.PutKnownVocabulary(ctx, alice.ID, "de", "Welt", "NOUN"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO enrichment_cache(language,canonical_lemma,upos,provider,provider_version,sentence_hash,translation,gloss,sentence_translation) VALUES('de','Haus','NOUN','test','1',$1,'house','a dwelling','This translation belongs to another sentence.')`, enrichment.SentenceHash("Dieses alte Haus steht noch am Stadtrand.")); err != nil {
		t.Fatal(err)
	}

	artifact, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, aliceBookA)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Count != 1 || artifact.Completeness.CardsWithEnglishSentence != 0 || contains(artifact.TSV, "This translation belongs to another sentence.") || !contains(artifact.TSV, "Haus") || contains(artifact.TSV, "Baum") {
		t.Fatalf("artifact=%+v", artifact)
	}
	if contains(artifact.TSV, "Inhaltsverzeichnis") {
		t.Fatalf("lower-quality first reference was selected: %s", artifact.TSV)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO enrichment_cache(language,canonical_lemma,upos,provider,provider_version,sentence_hash,translation,gloss,sentence_translation,sentence_translation_target) VALUES('de','Haus','NOUN','test','1',$1,'house','a dwelling','The old house is surprisingly large.','old house')`, enrichment.SentenceHash("Das alte Haus ist überraschend groß.")); err != nil {
		t.Fatal(err)
	}
	var cards, decks, audits, generated, known int
	var state string
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, alice.ID).Scan(&cards); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM decks WHERE owner_id=$1`, alice.ID).Scan(&decks); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT state FROM vocabulary_states WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='vocabulary.transition' AND details->>'to'='generated'`, alice.ID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus' AND first_source_material_id=$2`, alice.ID, aliceBookA).Scan(&generated); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, alice.ID).Scan(&known); err != nil {
		t.Fatal(err)
	}
	if cards != 1 || decks != 2 || state != "generated" || audits != 1 || generated != 1 || known != 1 {
		t.Fatalf("cards=%d decks=%d state=%s audits=%d generated=%d known=%d", cards, decks, state, audits, generated, known)
	}
	var firstDeck, firstGeneratedAt string
	if err = pool.QueryRow(ctx, `SELECT first_deck_id::text,first_generated_at::text FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&firstDeck, &firstGeneratedAt); err != nil {
		t.Fatal(err)
	}
	again, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, aliceBookA)
	if err != nil || again.Count != 1 || again.Completeness != (cardexport.Completeness{TotalCards: 1, CardsWithEnglish: 1, CardsWithEnglishSentence: 1}) || !contains(again.TSV, "The <b>old house</b> is surprisingly large.") {
		t.Fatalf("again=%+v err=%v", again, err)
	}
	var generatedRows int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, alice.ID).Scan(&cards); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='vocabulary.transition' AND details->>'to'='generated'`, alice.ID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus' AND first_source_material_id=$2`, alice.ID, aliceBookA).Scan(&generatedRows); err != nil {
		t.Fatal(err)
	}
	if cards != 1 || audits != 1 || generatedRows != 1 {
		t.Fatalf("idempotence cards=%d audits=%d generated=%d", cards, audits, generatedRows)
	}
	var stableDeck, stableGeneratedAt string
	if err = pool.QueryRow(ctx, `SELECT first_deck_id::text,first_generated_at::text FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&stableDeck, &stableGeneratedAt); err != nil {
		t.Fatal(err)
	}
	if stableDeck != firstDeck || stableGeneratedAt != firstGeneratedAt {
		t.Fatalf("generated provenance changed: deck %s -> %s, time %s -> %s", firstDeck, stableDeck, firstGeneratedAt, stableGeneratedAt)
	}

	secondBook, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, aliceBookB)
	if err != nil {
		t.Fatal(err)
	}
	if secondBook.Count != 3 || contains(secondBook.TSV, "Haus") || contains(secondBook.TSV, "Welt") || !contains(secondBook.TSV, "Baum") || !contains(secondBook.TSV, "Weg") || contains(secondBook.TSV, "Himmel") || contains(secondBook.TSV, "Karte") || contains(secondBook.TSV, "Rand") || !contains(secondBook.TSV, "Stern") {
		t.Fatalf("book B exclusions or frequency-floor selection incorrect: %+v", secondBook)
	}
	if strings.Index(secondBook.TSV, "Weg") > strings.Index(secondBook.TSV, "Baum") || strings.Index(secondBook.TSV, "Baum") > strings.Index(secondBook.TSV, "Stern") {
		t.Fatalf("book B TSV is not in first-encounter order: %s", secondBook.TSV)
	}
	var bookBGenerated, aliceKnown int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND first_source_material_id=$2 AND canonical_lemma IN ('Baum','Weg','Stern')`, alice.ID, aliceBookB).Scan(&bookBGenerated); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Welt'`, alice.ID).Scan(&aliceKnown); err != nil {
		t.Fatal(err)
	}
	if bookBGenerated != 3 || aliceKnown != 0 {
		t.Fatalf("book B provenance=%d generated-known=%d", bookBGenerated, aliceKnown)
	}
	var bookBDeckCards int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM cards c JOIN decks d ON d.owner_id=c.owner_id AND d.id=c.deck_id WHERE d.owner_id=$1 AND d.name='Book B'`, alice.ID).Scan(&bookBDeckCards); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM decks WHERE owner_id=$1`, alice.ID).Scan(&decks); err != nil {
		t.Fatal(err)
	}
	if bookBDeckCards != 3 || decks != 3 {
		t.Fatalf("book B persisted cards=%d owner decks=%d", bookBDeckCards, decks)
	}

	bobArtifact, err := cardexport.NewService(store).ExportCoverage(ctx, bob.ID, bobBook)
	if err != nil {
		t.Fatal(err)
	}
	if bobArtifact.Count != 0 || contains(bobArtifact.TSV, "Haus") {
		t.Fatalf("Alice's generated history affected Bob: %+v", bobArtifact)
	}
	var bobGenerated int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus' AND first_source_material_id=$2`, bob.ID, bobBook).Scan(&bobGenerated); err != nil {
		t.Fatal(err)
	}
	if bobGenerated != 0 {
		t.Fatalf("Bob generated provenance=%d", bobGenerated)
	}

	badBook := seedBook(alice, "export-quality", "Quality Book", fixtureCandidate{"Fragment", "Fragment.", 3, 10})
	omitted, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, badBook)
	if err != nil {
		t.Fatal(err)
	}
	var omittedGenerated, omittedCards int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Fragment'`, alice.ID).Scan(&omittedGenerated); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1 AND canonical_lemma='Fragment'`, alice.ID).Scan(&omittedCards); err != nil {
		t.Fatal(err)
	}
	if omitted.Count != 0 || len(omitted.Omitted) != 1 || omittedGenerated != 0 || omittedCards != 0 {
		t.Fatalf("omitted=%+v generated=%d cards=%d", omitted, omittedGenerated, omittedCards)
	}
	var qualityCorpus string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM corpora WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, badBook).Scan(&qualityCorpus); err != nil {
		t.Fatal(err)
	}
	if err = store.ReplaceSelectedSentences(ctx, alice.ID, qualityCorpus, "de", "Fragment", "NOUN", []domain.ExampleSentence{{SentenceKey: "quality-improved", Text: "Dieses Fragment enthält jetzt genügend hilfreichen Kontext.", SourceLocation: []byte(`{"start_offset":10}`), SelectionReasons: []byte(`[]`), SelectionRank: 1, Chosen: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE selection_candidates SET eligible_sentence_refs='[{"location":{"start_offset":10},"text":"Dieses Fragment enthält jetzt genügend hilfreichen Kontext."}]' WHERE owner_id=$1 AND corpus_id=$2 AND canonical_lemma='Fragment'`, alice.ID, qualityCorpus); err != nil {
		t.Fatal(err)
	}
	improved, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, badBook)
	if err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Fragment'`, alice.ID).Scan(&omittedGenerated); err != nil {
		t.Fatal(err)
	}
	if improved.Count != 1 || len(improved.Omitted) != 0 || omittedGenerated != 1 {
		t.Fatalf("improved=%+v generated=%d", improved, omittedGenerated)
	}
}

func TestGeneratedVocabularyBackfillPreservesFirstDeckAndUnknownSource(t *testing.T) {
	ctx := context.Background()
	_, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	var owner, firstDeck, secondDeck string
	if err := pool.QueryRow(ctx, `INSERT INTO users(username) VALUES('backfill-owner') RETURNING id::text`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO decks(owner_id,language,name,created_at) VALUES($1,'de','first','2024-01-01') RETURNING id::text`, owner).Scan(&firstDeck); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO decks(owner_id,language,name,created_at) VALUES($1,'de','second','2024-02-01') RETURNING id::text`, owner).Scan(&secondDeck); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO cards(owner_id,deck_id,dedup_key,canonical_lemma,upos,front,back,created_at) VALUES($1,$2,'older','Haus','NOUN','a','a','2024-01-02'),($1,$3,'newer','Haus','NOUN','b','b','2024-02-02')`, owner, firstDeck, secondDeck); err != nil {
		t.Fatal(err)
	}
	migration, err := migrations.FS.ReadFile("000013_backfill_generated_vocabulary.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	var gotDeck string
	var sourceIsNull bool
	if err = pool.QueryRow(ctx, `SELECT first_deck_id::text, first_source_material_id IS NULL FROM generated_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma='Haus' AND upos='NOUN'`, owner).Scan(&gotDeck, &sourceIsNull); err != nil {
		t.Fatal(err)
	}
	if gotDeck != firstDeck || !sourceIsNull {
		t.Fatalf("first deck=%s want=%s source null=%t", gotDeck, firstDeck, sourceIsNull)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
