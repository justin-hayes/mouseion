//go:build integration

package cardexport_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
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
	aliceBookA := seedBook(alice, "export-a", "Book A", fixtureCandidate{"Haus", "Das Haus ist groß.", 1, 10})
	aliceBookB := seedBook(alice, "export-b", "Book B",
		fixtureCandidate{"Haus", "Dieses Haus ist alt.", 100, 10},
		fixtureCandidate{"Welt", "Die Welt ist groß.", 100, 20},
		fixtureCandidate{"Baum", "Der Baum ist grün.", 96, 40},
		fixtureCandidate{"Weg", "Der Weg ist lang.", 4, 30},
	)
	bobBook := seedBook(bob, "export-bob", "Bob's Book", fixtureCandidate{"Haus", "Bobs Haus ist neu.", 1, 10})
	if _, err = store.PutKnownVocabulary(ctx, alice.ID, "de", "Welt", "NOUN"); err != nil {
		t.Fatal(err)
	}

	artifact, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, aliceBookA)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Count != 1 || !contains(artifact.TSV, "Haus") || contains(artifact.TSV, "Baum") {
		t.Fatalf("artifact=%+v", artifact)
	}
	var cards, decks, audits, generated, known int
	var state string
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, alice.ID).Scan(&cards); err != nil {
		t.Fatal(err)
	}
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM decks WHERE owner_id=$1`, alice.ID).Scan(&decks)
	_ = pool.QueryRow(ctx, `SELECT state FROM vocabulary_states WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&state)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='vocabulary.transition' AND details->>'to'='generated'`, alice.ID).Scan(&audits)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus' AND first_source_material_id=$2`, alice.ID, aliceBookA).Scan(&generated)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, alice.ID).Scan(&known)
	if cards != 1 || decks != 1 || state != "generated" || audits != 1 || generated != 1 || known != 1 {
		t.Fatalf("cards=%d decks=%d state=%s audits=%d generated=%d known=%d", cards, decks, state, audits, generated, known)
	}
	var firstDeck, firstGeneratedAt string
	if err = pool.QueryRow(ctx, `SELECT first_deck_id::text,first_generated_at::text FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&firstDeck, &firstGeneratedAt); err != nil {
		t.Fatal(err)
	}
	again, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, aliceBookA)
	if err != nil || again.Count != 1 {
		t.Fatalf("again=%+v err=%v", again, err)
	}
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, alice.ID).Scan(&cards)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='vocabulary.transition' AND details->>'to'='generated'`, alice.ID).Scan(&audits)
	if cards != 1 || audits != 1 {
		t.Fatalf("idempotence cards=%d audits=%d", cards, audits)
	}
	var stableDeck, stableGeneratedAt string
	_ = pool.QueryRow(ctx, `SELECT first_deck_id::text,first_generated_at::text FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&stableDeck, &stableGeneratedAt)
	if stableDeck != firstDeck || stableGeneratedAt != firstGeneratedAt {
		t.Fatalf("generated provenance changed: deck %s -> %s, time %s -> %s", firstDeck, stableDeck, firstGeneratedAt, stableGeneratedAt)
	}

	secondBook, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, aliceBookB)
	if err != nil {
		t.Fatal(err)
	}
	if secondBook.Count != 2 || contains(secondBook.TSV, "Haus") || contains(secondBook.TSV, "Welt") || !contains(secondBook.TSV, "Baum") || !contains(secondBook.TSV, "Weg") {
		t.Fatalf("book B exclusions or 97%% selection incorrect: %+v", secondBook)
	}
	if strings.Index(secondBook.TSV, "Weg") > strings.Index(secondBook.TSV, "Baum") {
		t.Fatalf("book B TSV is not in first-encounter order: %s", secondBook.TSV)
	}
	var bookBGenerated, aliceKnown int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND first_source_material_id=$2 AND canonical_lemma IN ('Baum','Weg')`, alice.ID, aliceBookB).Scan(&bookBGenerated)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Welt'`, alice.ID).Scan(&aliceKnown)
	if bookBGenerated != 2 || aliceKnown != 0 {
		t.Fatalf("book B provenance=%d generated-known=%d", bookBGenerated, aliceKnown)
	}

	bobArtifact, err := cardexport.NewService(store).ExportCoverage(ctx, bob.ID, bobBook)
	if err != nil {
		t.Fatal(err)
	}
	if bobArtifact.Count != 1 || !contains(bobArtifact.TSV, "Haus") {
		t.Fatalf("Alice's generated history affected Bob: %+v", bobArtifact)
	}
	var bobGenerated int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus' AND first_source_material_id=$2`, bob.ID, bobBook).Scan(&bobGenerated)
	if bobGenerated != 1 {
		t.Fatalf("Bob generated provenance=%d", bobGenerated)
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
