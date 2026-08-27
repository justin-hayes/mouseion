//go:build integration

package persistence_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestGetCoverageEntryForBookEncodesFirstEncounterAsBigint(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, err := store.CreateUser(ctx, "coverage-entry-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	artifact := domain.NormalizedArtifact{ContentHash: "coverage-entry-hash", Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
	if err = store.PutArtifact(ctx, artifact, []domain.SharedLemma{
		{CanonicalLemma: "Haus", UPOS: "NOUN", Morphology: []byte(`{"Gender":"Neut","Number":"Sing"}`), Frequency: 2},
		{CanonicalLemma: "Haus", UPOS: "NOUN", Morphology: []byte(`{"Gender":"Neut","Number":"Plur"}`), Frequency: 1},
	}); err != nil {
		t.Fatal(err)
	}
	book, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "coverage-entry-book", Title: "Coverage Entry Book", MediaType: "text/plain", ContentHash: artifact.ContentHash, Content: []byte("Haus"), FullText: "Haus"})
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := store.PutCorpus(ctx, owner.ID, book.ID, artifact.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	candidate := domain.SelectionCandidate{OwnerID: owner.ID, CorpusID: corpus.ID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 1, ObservedForms: []byte(`["Haus"]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`), FirstEncounter: 59}
	if _, err = store.PutSelectionCandidate(ctx, candidate); err != nil {
		t.Fatal(err)
	}

	candidates, err := store.ListSelectionCandidatesForCorpus(ctx, owner.ID, corpus.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].OwnerID != owner.ID {
		t.Fatalf("scoped candidates = %#v, want one candidate owned by %s", candidates, owner.ID)
	}

	entry, err := store.GetCoverageEntryForBook(ctx, owner.ID, book.ID, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if entry.FirstEncounter != candidate.FirstEncounter {
		t.Fatalf("first encounter = %d, want %d", entry.FirstEncounter, candidate.FirstEncounter)
	}
	var morphologies []map[string]string
	if err = json.Unmarshal([]byte(entry.Morphology), &morphologies); err != nil || len(morphologies) != 2 || morphologies[0]["Number"] != "Sing" || morphologies[1]["Number"] != "Plur" {
		t.Fatalf("deterministic morphology variants = %s, %v", entry.Morphology, err)
	}

	const exactSentence = "Sie nennt dieses Haus seit vielen Jahren ihr Zuhause."
	const otherSentence = "Das Haus veröffentlicht heute ein neues Buch für Kinder."
	candidate.SentenceReferences, _ = json.Marshal([]map[string]any{{
		"text": exactSentence, "location": map[string]any{"start_offset": 59},
	}})
	candidate.ObservedForms = []byte(`["Haus"]`)
	when := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	if _, err = store.Pool().Exec(ctx, `INSERT INTO enrichment_cache(language,canonical_lemma,upos,provider,provider_version,sentence_hash,translation,gloss,sentence_translation,context_sentence,cached_at) VALUES
		('de','Haus','NOUN','test','1',$1,'home','dwelling','She has called this house her home for many years.','',$3),
		('de','Haus','NOUN','test','1',$2,'publisher','publishing house','The publisher is releasing a new children''s book today.','',$4)`, enrichment.SentenceHash(exactSentence), enrichment.SentenceHash(otherSentence), when, when.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	entry, err = store.GetCoverageEntryForBook(ctx, owner.ID, book.ID, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Sentence != exactSentence || entry.Translation != "home" || entry.SentenceTranslation != "She has called this house her home for many years." {
		t.Fatalf("sentence-aligned enrichment = %+v", entry)
	}
}
