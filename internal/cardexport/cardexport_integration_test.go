//go:build integration

package cardexport_test

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestExportPersistsOwnerScopedCardsAndGeneratedStateIdempotently(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, _ := store.CreateUser(ctx, "export-alice", false)
	bob, _ := store.CreateUser(ctx, "export-bob", false)
	seed := func(owner domain.User, hash, lemma, sentence string) string {
		t.Helper()
		artifact := domain.NormalizedArtifact{ContentHash: hash, Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
		if err := store.PutArtifact(ctx, artifact, []domain.SharedLemma{{CanonicalLemma: lemma, UPOS: "NOUN", Morphology: []byte(`{"Gender":"Neut"}`), Frequency: 1}}); err != nil {
			t.Fatal(err)
		}
		source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: hash, Title: "Test Book", MediaType: "text/plain", ContentHash: hash, Content: []byte(sentence), FullText: sentence})
		if err != nil {
			t.Fatal(err)
		}
		corpus, err := store.PutCorpus(ctx, owner.ID, source.ID, hash)
		if err != nil {
			t.Fatal(err)
		}
		if err = store.ReplaceSelectedSentences(ctx, owner.ID, corpus.ID, "de", lemma, "NOUN", []domain.ExampleSentence{{SentenceKey: "s1", Text: sentence, SourceLocation: []byte(`{"start_offset":0}`), SelectionReasons: []byte(`[]`), SelectionRank: 1, Chosen: true}}); err != nil {
			t.Fatal(err)
		}
		if _, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{OwnerID: owner.ID, CorpusID: corpus.ID, Language: "de", CanonicalLemma: lemma, UPOS: "NOUN", OccurrenceCount: 1, ObservedForms: []byte(`["` + lemma + `"]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{"min_occurrences":1,"occurrence_count":1}`)}); err != nil {
			t.Fatal(err)
		}
		return source.ID
	}
	aliceBook := seed(alice, "export-a", "Haus", "Das Haus ist groß.")
	seed(bob, "export-b", "Baum", "Der Baum ist groß.")
	artifact, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, aliceBook)
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
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus' AND first_source_material_id=$2`, alice.ID, aliceBook).Scan(&generated)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, alice.ID).Scan(&known)
	if cards != 1 || decks != 1 || state != "generated" || audits != 1 || generated != 1 || known != 0 {
		t.Fatalf("cards=%d decks=%d state=%s audits=%d generated=%d known=%d", cards, decks, state, audits, generated, known)
	}
	again, err := cardexport.NewService(store).ExportCoverage(ctx, alice.ID, aliceBook)
	if err != nil || again.Count != 1 {
		t.Fatalf("again=%+v err=%v", again, err)
	}
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, alice.ID).Scan(&cards)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='vocabulary.transition' AND details->>'to'='generated'`, alice.ID).Scan(&audits)
	if cards != 1 || audits != 1 {
		t.Fatalf("idempotence cards=%d audits=%d", cards, audits)
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
