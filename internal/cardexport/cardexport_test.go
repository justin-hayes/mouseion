package cardexport

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type memoryStore struct {
	entries      []Entry
	candidates   []domain.SelectionCandidate
	known        []domain.KnownVocabulary
	history      []domain.GeneratedVocabulary
	active       []domain.CampaignVocabulary
	generated    []Note
	bookID       string
	historyCalls map[string]int
}

type scopedMemoryStore struct {
	*memoryStore
	corpusID string
}

func (s *scopedMemoryStore) GetSourceMaterial(context.Context, string, string) (domain.SourceMaterial, error) {
	return domain.SourceMaterial{ID: s.bookID, Title: "Scoped Book"}, nil
}

func (s *scopedMemoryStore) GetCorpusForAnalysis(context.Context, string, string) (domain.Corpus, error) {
	return domain.Corpus{ID: s.corpusID, SourceMaterialID: s.bookID, AnalysisRunID: "run-1", Status: "complete"}, nil
}

func (s *scopedMemoryStore) ListSelectionCandidatesForCorpus(ctx context.Context, owner, corpusID string) ([]domain.SelectionCandidate, error) {
	var result []domain.SelectionCandidate
	for _, candidate := range s.candidates {
		if candidate.OwnerID == "" || candidate.OwnerID == owner {
			if candidate.CorpusID == corpusID {
				result = append(result, candidate)
			}
		}
	}
	return result, nil
}

func (s *scopedMemoryStore) GetCoverageEntryForCorpus(ctx context.Context, owner, _ string, candidate domain.SelectionCandidate) (Entry, error) {
	return s.GetCoverageEntryForBook(ctx, owner, s.bookID, candidate)
}

func TestDownloadFilenameAndDeckName(t *testing.T) {
	if got := DeckName("de", "Das archaische Griechenland"); got != "Mouseion::de::Das archaische Griechenland" {
		t.Fatalf("deck name = %q", got)
	}
	if got := DownloadFilename(`  Über/../Buch:*?  `); got != "Über_.._Buch___.apkg" {
		t.Fatalf("filename = %q", got)
	}
	if a, b := DownloadFilename("../"), DownloadFilename("../"); a != b || !strings.HasPrefix(a, "mouseion-deck-") || !strings.HasSuffix(a, ".apkg") {
		t.Fatalf("unsafe fallback = %q, %q", a, b)
	}
}

func TestClozeEscapesHTMLAndClozeSyntax(t *testing.T) {
	got, err := Cloze(`<b>Das {{falsche}} Haus & mehr.</b>`, "Haus", `h}}int`)
	if err != nil {
		t.Fatal(err)
	}
	want := `&lt;b&gt;Das &#123;&#123;falsche&#125;&#125; {{c1::Haus::h&#125;&#125;int}} &amp; mehr.&lt;/b&gt;`
	if got != want {
		t.Fatalf("cloze = %q, want %q", got, want)
	}
}

func TestAnkiPackageContractAndStableIDs(t *testing.T) {
	note := Note{Key: DedupKey("de", "haus", "NOUN", "alice"), Text: "Das {{c1::Haus}} ist heute sehr ruhig.", Lemma: "Haus", POS: "NOUN", Morph: `{"Case":"Nom"}`, English: "house", EnglishSentence: "The house is very quiet today.", BookTitle: "Das archaische Griechenland", SourceSentence: "Das Haus ist heute sehr ruhig.", Tags: []string{"Mouseion", "lang::de", "pos::NOUN", "source::Das_archaische_Griechenland"}}
	missingSentenceTranslation := note
	missingSentenceTranslation.Key = DedupKey("de", "baum", "NOUN", "alice")
	missingSentenceTranslation.Lemma = "Baum"
	missingSentenceTranslation.English = "tree"
	missingSentenceTranslation.EnglishSentence = ""
	deckName := DeckName("de", note.BookTitle)
	notes := []Note{note, missingSentenceTranslation}
	a, err := renderAPKG(deckName, notes)
	if err != nil {
		t.Fatal(err)
	}
	b, err := renderAPKG(deckName, notes)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("package bytes are not deterministic")
	}
	zr, err := zip.NewReader(bytes.NewReader(a), int64(len(a)))
	if err != nil {
		t.Fatal(err)
	}
	members := map[string]*zip.File{}
	for _, f := range zr.File {
		members[f.Name] = f
	}
	if members["collection.anki2"] == nil || members["media"] == nil || len(members) != 2 {
		t.Fatalf("zip members = %#v", members)
	}
	media, _ := members["media"].Open()
	var mediaMap map[string]string
	if err = json.NewDecoder(media).Decode(&mediaMap); err != nil || len(mediaMap) != 0 {
		t.Fatalf("media manifest = %#v, %v", mediaMap, err)
	}
	dbReader, _ := members["collection.anki2"].Open()
	dbBytes, _ := io.ReadAll(dbReader)
	dbPath := t.TempDir() + "/collection.anki2"
	if err = os.WriteFile(dbPath, dbBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var modelsJSON, decksJSON, dconfJSON string
	if err = db.QueryRow(`SELECT models,decks,dconf FROM col`).Scan(&modelsJSON, &decksJSON, &dconfJSON); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(modelsJSON, `"name":"Mouseion Vocab Cloze"`) || !strings.Contains(modelsJSON, `"qfmt":"{{cloze:Text}}"`) || !strings.Contains(decksJSON, deckName) {
		t.Fatalf("models=%s decks=%s", modelsJSON, decksJSON)
	}
	var models map[string]struct {
		Fields []struct {
			Name string `json:"name"`
		} `json:"flds"`
	}
	if err = json.Unmarshal([]byte(modelsJSON), &models); err != nil {
		t.Fatal(err)
	}
	gotNames := make([]string, 0, len(fieldNames))
	for _, model := range models {
		for _, field := range model.Fields {
			gotNames = append(gotNames, field.Name)
		}
	}
	if strings.Join(gotNames, ",") != strings.Join(fieldNames, ",") {
		t.Fatalf("field names = %v", gotNames)
	}
	assertLegacyCollectionContract(t, modelsJSON, decksJSON, dconfJSON, deckName)
	var noteID, cardCount int64
	var fields, tags string
	if err = db.QueryRow(`SELECT id,flds,tags FROM notes WHERE guid=?`, note.Key[:20]).Scan(&noteID, &fields, &tags); err != nil {
		t.Fatal(err)
	}
	serializedFields := strings.Split(fields, "\x1f")
	if noteID != stableID("note|"+note.Key) || len(serializedFields) != 8 || serializedFields[4] != note.English || serializedFields[5] != note.EnglishSentence || !strings.Contains(tags, " Mouseion ") || strings.Contains(strings.ToLower(tags), "leech") {
		t.Fatalf("note id=%d fields=%q tags=%q", noteID, fields, tags)
	}
	if err = db.QueryRow(`SELECT flds FROM notes WHERE guid=?`, missingSentenceTranslation.Key[:20]).Scan(&fields); err != nil {
		t.Fatal(err)
	}
	serializedFields = strings.Split(fields, "\x1f")
	if len(serializedFields) != 8 || serializedFields[5] != "" {
		t.Fatalf("missing sentence translation fields=%q", fields)
	}
	if err = db.QueryRow(`SELECT count(*) FROM cards`).Scan(&cardCount); err != nil || cardCount != 2 {
		t.Fatalf("cards=%d err=%v", cardCount, err)
	}
}

func assertLegacyCollectionContract(t *testing.T, modelsJSON, decksJSON, dconfJSON, deckName string) {
	t.Helper()
	fixture, err := os.ReadFile("testdata/legacy_collection_contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract map[string]map[string]string
	if err = json.Unmarshal(fixture, &contract); err != nil {
		t.Fatal(err)
	}

	collections := map[string]string{"models": modelsJSON, "decks": decksJSON, "dconf": dconfJSON}
	for collectionName, encoded := range collections {
		decoder := json.NewDecoder(strings.NewReader(encoded))
		decoder.UseNumber()
		var entries map[string]map[string]any
		if err = decoder.Decode(&entries); err != nil {
			t.Fatalf("decode %s JSON: %v\n%s", collectionName, err, encoded)
		}
		if len(entries) != 1 {
			t.Fatalf("%s entries = %#v", collectionName, entries)
		}
		for key, entry := range entries {
			assertJSONShape(t, collectionName, entry, contract[collectionName])
			if id, ok := entry["id"].(json.Number); ok {
				value, numberErr := id.Int64()
				if numberErr != nil || value > 1<<53-1 {
					t.Fatalf("%s id %q is not JSON-safe: %v", collectionName, id, numberErr)
				}
				if collectionName != "dconf" && key != id.String() {
					t.Fatalf("%s key %q does not match id %q", collectionName, key, id)
				}
			}
			if collectionName == "decks" && entry["name"] != deckName {
				t.Fatalf("deck name = %q, want %q", entry["name"], deckName)
			}
		}
	}
}

func assertJSONShape(t *testing.T, name string, value map[string]any, shape map[string]string) {
	t.Helper()
	for field, wantType := range shape {
		got, ok := value[field]
		if !ok {
			t.Errorf("%s missing required field %q", name, field)
			continue
		}
		valid := false
		switch wantType {
		case "string":
			_, valid = got.(string)
		case "number":
			_, valid = got.(json.Number)
		case "boolean":
			_, valid = got.(bool)
		case "object":
			_, valid = got.(map[string]any)
		case "array":
			_, valid = got.([]any)
		case "number_pair":
			pair, isArray := got.([]any)
			valid = isArray && len(pair) == 2
			for _, item := range pair {
				_, isNumber := item.(json.Number)
				valid = valid && isNumber
			}
		}
		if !valid {
			t.Errorf("%s field %q = %#v, want %s", name, field, got, wantType)
		}
	}
}

func (m *memoryStore) ListGeneratedVocabulary(_ context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
	if m.historyCalls == nil {
		m.historyCalls = make(map[string]int)
	}
	m.historyCalls[language]++
	var out []domain.GeneratedVocabulary
	for _, word := range m.history {
		if word.OwnerID == owner && word.Language == language {
			out = append(out, word)
		}
	}
	return out, nil
}

func (m *memoryStore) ListLegacyGeneratedVocabulary(ctx context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
	return m.ListGeneratedVocabulary(ctx, owner, language)
}

func (m *memoryStore) ListActiveLearningCampaignVocabulary(_ context.Context, owner, language string) ([]domain.CampaignVocabulary, error) {
	var result []domain.CampaignVocabulary
	for _, word := range m.active {
		if word.OwnerID == owner && word.Language == language {
			result = append(result, word)
		}
	}
	return result, nil
}

func (m *memoryStore) ListSelectionCandidatesForBook(_ context.Context, _ string, bookID string) ([]domain.SelectionCandidate, error) {
	if bookID != m.bookID {
		return nil, nil
	}
	return append([]domain.SelectionCandidate(nil), m.candidates...), nil
}
func (m *memoryStore) ListKnownVocabulary(_ context.Context, _ string, language string) ([]domain.KnownVocabulary, error) {
	var out []domain.KnownVocabulary
	for _, word := range m.known {
		if word.Language == language {
			out = append(out, word)
		}
	}
	return out, nil
}
func (m *memoryStore) GetCoverageEntryForBook(_ context.Context, owner, _ string, candidate domain.SelectionCandidate) (Entry, error) {
	for _, entry := range m.entries {
		if entry.OwnerID == owner && entry.Language == candidate.Language && entry.CanonicalLemma == candidate.CanonicalLemma && entry.UPOS == candidate.UPOS {
			return entry, nil
		}
	}
	return Entry{}, errors.New("missing entry")
}

func (m *memoryStore) RecordGeneratedForBook(_ context.Context, owner, bookID, _ string, e Entry, n Note) error {
	if owner != e.OwnerID {
		return ErrInvalidInput
	}
	if bookID != m.bookID {
		return ErrInvalidInput
	}
	m.generated = append(m.generated, n)
	return nil
}

func TestDedupKeyStableAndOwnerScoped(t *testing.T) {
	a := DedupKey("de", "Haus", "NOUN", "alice")
	if a != DedupKey("de", "Haus", "NOUN", "alice") || len(a) != 64 {
		t.Fatalf("unstable key %q", a)
	}
	for _, b := range []string{DedupKey("de", "Haus", "NOUN", "bob"), DedupKey("fr", "Haus", "NOUN", "alice"), DedupKey("de", "haus", "NOUN", "alice"), DedupKey("de", "Haus", "VERB", "alice")} {
		if a == b {
			t.Fatal("distinct identity produced same key")
		}
	}
}

func TestClozeWithAndWithoutHint(t *testing.T) {
	got, err := Cloze("Das Haus ist groß.", "Haus", "house")
	if err != nil || got != "Das {{c1::Haus::house}} ist groß." {
		t.Fatalf("got=%q err=%v", got, err)
	}
	got, err = Cloze("Das Haus ist groß.", "haus", "")
	if err != nil || got != "Das {{c1::Haus}} ist groß." {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if _, err = Cloze("Kein Treffer.", "Haus", ""); err == nil {
		t.Fatal("missing target accepted")
	}
}

func TestRenderTSVEscapesAndOrdersFields(t *testing.T) {
	n := Note{Key: "key", Text: "Grüße\t{{c1::Welt}}", Lemma: "Welt", POS: "NOUN", Morph: "Case=Nom", English: "world", EnglishSentence: "Hello world.", BookTitle: "My Book", SourceSentence: "Grüße Welt", Tags: []string{"Mouseion", "lang::de", "source::My_Book"}}
	got, err := RenderTSV([]Note{n})
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(strings.NewReader(got))
	r.Comma = '\t'
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0]) != 9 || rows[0][0] != n.Text || rows[0][1] != "Welt" || rows[0][7] != "Grüße Welt" || rows[0][8] != "Mouseion lang::de source::My_Book" {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestMakeNoteFormatsLemmaWithoutChangingTargetOrIdentity(t *testing.T) {
	entry := Entry{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		Sentence: "Die Häuser sind alt.", TargetWord: "Häuser",
	}
	note, err := makeNote("alice", entry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(note.Text, "{{c1::Häuser}}") || note.Lemma != "Haus" || note.POS != "NOUN" {
		t.Fatalf("note = %#v", note)
	}
	if want := DedupKey("de", "haus", "NOUN", "alice"); note.Key != want {
		t.Fatalf("note key = %q, want canonical identity key %q", note.Key, want)
	}
}

func TestBuildCoveragePreparesItalianCardWithoutChangingAccents(t *testing.T) {
	store := &memoryStore{bookID: "libro"}
	store.candidates = []domain.SelectionCandidate{{
		Language: "it", CanonicalLemma: "portare", UPOS: "VERB", OccurrenceCount: 2,
		ObservedForms:      []byte(`["porterà"]`),
		SentenceReferences: []byte(`[{"sentence_index":0,"text":"Domani Lucia porterà finalmente il pane fresco alla sua famiglia.","location":{"start_offset":7}}]`),
	}}
	store.entries = []Entry{{OwnerID: "alice", Language: "it", CanonicalLemma: "portare", UPOS: "VERB", Morphology: `{"Mood":"Ind","Tense":"Fut"}`, Translation: "to bring", SentenceTranslation: "Tomorrow Lucia will finally bring fresh bread to her family.", SourceDocument: "Il viaggio"}}

	artifact, err := NewService(store).BuildCoverage(context.Background(), "alice", "libro")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Count != 1 || artifact.DeckName != "Mouseion::it::Il viaggio" || artifact.Filename != "Il viaggio.apkg" || len(artifact.Generated) != 1 {
		t.Fatalf("Italian prepared artifact = %+v", artifact)
	}
	note := artifact.Generated[0].Note
	if !strings.Contains(note.Text, "{{c1::porterà::to bring}}") || note.Lemma != "portare" || note.POS != "VERB" || note.SourceSentence != "Domani Lucia porterà finalmente il pane fresco alla sua famiglia." || !strings.Contains(artifact.TSV, "lang::it") {
		t.Fatalf("Italian prepared note = %+v\nTSV=%q", note, artifact.TSV)
	}
	assertAPKGDeckAndCard(t, artifact.APKG, "Mouseion::it::Il viaggio", "portare", "lang::it")
}

func TestBuildCoverageForAnalysisUsesOnlyItsCorpus(t *testing.T) {
	store := &scopedMemoryStore{memoryStore: &memoryStore{bookID: "libro"}, corpusID: "corpus-scoped"}
	store.candidates = []domain.SelectionCandidate{
		{OwnerID: "alice", CorpusID: "corpus-scoped", Language: "it", CanonicalLemma: "portare", UPOS: "VERB", OccurrenceCount: 2, ObservedForms: []byte(`["porterà"]`), SentenceReferences: []byte(`[{"text":"Domani Lucia porterà finalmente il pane fresco alla sua famiglia.","location":{"start_offset":7}}]`)},
		{OwnerID: "alice", CorpusID: "corpus-other", Language: "it", CanonicalLemma: "sbagliare", UPOS: "VERB", OccurrenceCount: 100, ObservedForms: []byte(`["sbaglia"]`), SentenceReferences: []byte(`[{"text":"Questo candidato appartiene a un altro risultato analizzato.","location":{"start_offset":7}}]`)},
	}
	store.entries = []Entry{{OwnerID: "alice", Language: "it", CanonicalLemma: "portare", UPOS: "VERB", Translation: "to bring", SourceDocument: "Scoped Book"}}

	artifact, err := NewService(store).BuildCoverageForAnalysis(context.Background(), "alice", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Count != 1 || len(artifact.Generated) != 1 || artifact.Generated[0].Entry.CanonicalLemma != "portare" {
		t.Fatalf("scoped prepared artifact = %+v", artifact)
	}
}

func assertAPKGDeckAndCard(t *testing.T, payload []byte, deckName, lemma, tag string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("open APKG: %v", err)
	}
	var collection []byte
	for _, member := range zr.File {
		if member.Name != "collection.anki2" {
			continue
		}
		reader, openErr := member.Open()
		if openErr != nil {
			t.Fatalf("open APKG collection: %v", openErr)
		}
		collection, err = io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatalf("read APKG collection: %v", err)
		}
	}
	if len(collection) == 0 {
		t.Fatal("APKG has no collection.anki2")
	}
	path := t.TempDir() + "/collection.anki2"
	if err = os.WriteFile(path, collection, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var decks, fields, tags string
	if err = db.QueryRow(`SELECT decks FROM col`).Scan(&decks); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT flds,tags FROM notes`).Scan(&fields, &tags); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(decks, `"name":"`+deckName+`"`) || !strings.Contains(fields, "\x1f"+lemma+"\x1f") || !strings.Contains(tags, " "+tag+" ") {
		t.Fatalf("Italian APKG decks=%s fields=%q tags=%q", decks, fields, tags)
	}
}

func TestCoverageCandidatesExcludesKnownAndGeneratedBeforeCutoff(t *testing.T) {
	otherBook := "other-book"
	store := &memoryStore{known: []domain.KnownVocabulary{{Language: "de", CanonicalLemma: "known", UPOS: "NOUN"}}}
	store.history = []domain.GeneratedVocabulary{
		{OwnerID: "alice", Language: "de", CanonicalLemma: "generated", UPOS: "NOUN", FirstSourceMaterialID: &otherBook},
		{OwnerID: "alice", Language: "de", CanonicalLemma: "legacy", UPOS: "NOUN"},
	}
	candidates := []domain.SelectionCandidate{
		{Language: "de", CanonicalLemma: "known", UPOS: "NOUN", OccurrenceCount: 100},
		{Language: "de", CanonicalLemma: "generated", UPOS: "NOUN", OccurrenceCount: 100},
		{Language: "de", CanonicalLemma: "legacy", UPOS: "NOUN", OccurrenceCount: 100},
		{Language: "de", CanonicalLemma: "one", UPOS: "NOUN", OccurrenceCount: 96},
		{Language: "de", CanonicalLemma: "singleton-a", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "singleton-b", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "singleton-c", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "singleton-d", UPOS: "NOUN", OccurrenceCount: 1},
	}
	got, err := NewService(store).coverageCandidates(context.Background(), "alice", "current-book", candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].CanonicalLemma != "one" || got[1].CanonicalLemma != "singleton-a" {
		t.Fatalf("coverage candidates = %#v", got)
	}
	if store.historyCalls["de"] != 1 {
		t.Fatalf("generated vocabulary loaded %d times", store.historyCalls["de"])
	}
}

func TestCoverageCandidatesAllowsSameBookAndIsolatesOwners(t *testing.T) {
	currentBook := "current-book"
	store := &memoryStore{history: []domain.GeneratedVocabulary{
		{OwnerID: "alice", Language: "de", CanonicalLemma: "same", UPOS: "NOUN", FirstSourceMaterialID: &currentBook},
		{OwnerID: "bob", Language: "de", CanonicalLemma: "bob-word", UPOS: "NOUN"},
	}}
	candidates := []domain.SelectionCandidate{
		{Language: "de", CanonicalLemma: "same", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "bob-word", UPOS: "NOUN", OccurrenceCount: 1},
	}
	got, err := NewService(store).coverageCandidates(context.Background(), "alice", currentBook, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("coverage candidates = %#v", got)
	}
}

func TestCoverageCandidatesReservesOnlyActiveCampaignVocabulary(t *testing.T) {
	store := &memoryStore{active: []domain.CampaignVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "reserved", UPOS: "VERB"}}}
	candidates := []domain.SelectionCandidate{
		{Language: "de", CanonicalLemma: "reserved", UPOS: "VERB", OccurrenceCount: 10},
		{Language: "de", CanonicalLemma: "released", UPOS: "ADJ", OccurrenceCount: 10},
	}
	got, err := NewService(store).coverageCandidates(context.Background(), "alice", "future-book", candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].CanonicalLemma != "released" {
		t.Fatalf("active reservation candidates = %+v", got)
	}
	store.active = nil
	got, err = NewService(store).coverageCandidates(context.Background(), "alice", "future-book", candidates)
	if err != nil || len(got) != 2 {
		t.Fatalf("released reservation candidates = %+v, %v", got, err)
	}
}

func TestCoverageCandidatesKnownLemmaWildcardAndSingleton(t *testing.T) {
	store := &memoryStore{known: []domain.KnownVocabulary{{Language: "de", CanonicalLemma: "known"}}}
	candidates := []domain.SelectionCandidate{
		{Language: "de", CanonicalLemma: "known", UPOS: "VERB", OccurrenceCount: 10},
		{Language: "de", CanonicalLemma: "only", UPOS: "NOUN", OccurrenceCount: 1},
	}
	got, err := NewService(store).coverageCandidates(context.Background(), "alice", "book", candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].CanonicalLemma != "only" {
		t.Fatalf("coverage candidates = %#v", got)
	}
}

func TestSelectCoverageCandidatesMetricContract(t *testing.T) {
	candidates := []domain.SelectionCandidate{
		{Language: "de", CanonicalLemma: "eins", UPOS: "NOUN", OccurrenceCount: 94},
		{Language: "de", CanonicalLemma: "zwei", UPOS: "NOUN", OccurrenceCount: 2},
		{Language: "de", CanonicalLemma: "drei", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "beta", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "gamma", UPOS: "NOUN", OccurrenceCount: 1},
	}
	tests := []struct {
		target int
		want   []string
	}{
		{target: 95, want: []string{"eins", "zwei"}},
		{target: 97, want: []string{"eins", "zwei", "alpha"}},
		{target: 99, want: []string{"eins", "zwei", "alpha", "beta", "drei"}},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%d_percent", tc.target), func(t *testing.T) {
			got := selectCoverageCandidates(candidates, tc.target)
			if len(got) != len(tc.want) {
				t.Fatalf("selected %d candidates, want %d: %+v", len(got), len(tc.want), got)
			}
			for i, candidate := range got {
				if candidate.CanonicalLemma != tc.want[i] {
					t.Fatalf("candidate %d = %q, want %q", i, candidate.CanonicalLemma, tc.want[i])
				}
			}
		})
	}
}

func TestSelectCoverageCandidatesStopsAtExactThreshold(t *testing.T) {
	candidates := []domain.SelectionCandidate{
		{Language: "de", CanonicalLemma: "common", UPOS: "NOUN", OccurrenceCount: 97},
		{Language: "de", CanonicalLemma: "rare", UPOS: "NOUN", OccurrenceCount: 3},
	}
	got := selectCoverageCandidates(candidates, 97)
	if len(got) != 1 || got[0].CanonicalLemma != "common" {
		t.Fatalf("selected = %+v, want exact 97%% prefix", got)
	}
}

func TestScoreSentenceQuality(t *testing.T) {
	tests := []struct {
		name, sentence, target, reason string
		location                       int64
		accepted                       bool
	}{
		{"long contextual", "Vor dem alten Haus spielen heute mehrere fröhliche Kinder, während ihre Eltern im sonnigen Garten gemeinsam das Abendessen vorbereiten.", "Haus", "usable length", 42, true},
		{"too short", "Altes Haus.", "Haus", "too short or fragmented", 42, false},
		{"too long", "Das Haus " + strings.Repeat("steht weit außerhalb der alten Stadt ", 9) + "still.", "Haus", "too long", 42, false},
		{"missing target", "Vor dem alten Gebäude spielen heute mehrere fröhliche Kinder.", "Haus", "target not present as a word", 42, false},
		{"fragmented boundary", "Vor dem alten Haus spielen heute mehrere Kinder", "Haus", "incomplete sentence boundaries", 42, false},
		{"contents", "Inhaltsverzeichnis: Das Haus und seine lange Geschichte ..... 12.", "Haus", "structural noise or boilerplate", 42, false},
		{"bibliography", "Bibliography: Das Haus in der europäischen Literatur.", "Haus", "structural noise or boilerplate", 42, false},
		{"numbered list", "12. Das Haus steht oberhalb des alten Dorfes.", "Haus", "structural noise or boilerplate", 42, false},
		{"chapter heading", "Kapitel 3: Das Haus und seine lange Geschichte.", "Haus", "structural noise or boilerplate", 42, false},
		{"citation density", "Das Haus wird bei Müller [1999] und Schmidt [2004] ausführlich beschrieben.", "Haus", "structural noise or boilerplate", 42, false},
		{"extraction anomaly", "Das Haus Haus Haus Haus steht heute am See.", "Haus", "structural noise or boilerplate", 42, false},
		{"boilerplate", "All rights reserved for this edition of Haus und Garten.", "Haus", "structural noise or boilerplate", 42, false},
		{"invalid location", "Vor dem alten Haus spielen heute mehrere fröhliche Kinder.", "Haus", "invalid source location", -1, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ScoreSentenceQuality(tc.sentence, tc.target, tc.location)
			if got.Accepted != tc.accepted || !contains(got.Reasons, tc.reason) {
				t.Fatalf("quality=%+v", got)
			}
		})
	}
}

func TestBestSentenceEvidenceRanksAllReferences(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CanonicalLemma: "haus",
		ObservedForms:  []byte(`["Haus"]`),
		SentenceReferences: []byte(`[
			{"sentence_index":1,"text":"Inhaltsverzeichnis: Das Haus und seine Geschichte ..... 12.","location":{"start_offset":10}},
			{"sentence_index":2,"text":"Vor dem alten Haus spielen heute mehrere fröhliche Kinder im Garten.","location":{"start_offset":80}}
		]`),
	}
	got, ok := BestSentenceEvidence(candidate)
	if !ok || !got.Quality.Accepted || got.FirstEncounter != 80 || got.Target != "Haus" || !strings.HasPrefix(got.Sentence, "Vor dem") {
		t.Fatalf("evidence=%+v ok=%v", got, ok)
	}
}

func TestBestSentenceEvidenceUsesWordTargetsAndStableSourceTie(t *testing.T) {
	candidate := domain.SelectionCandidate{
		CanonicalLemma: "Haus",
		ObservedForms:  []byte(`["Haus"]`),
		SentenceReferences: []byte(`[
			{"sentence_index":9,"text":"Dieses Haus steht seit vielen Jahren ruhig am See.","location":{"start_offset":90}},
			{"sentence_index":2,"text":"Unser Haus steht seit vielen Jahren ruhig am See.","location":{"start_offset":20}},
			{"sentence_index":1,"text":"Das Hausboot liegt seit vielen Jahren ruhig am See.","location":{"start_offset":10}}
		]`),
	}
	first, ok := BestSentenceEvidence(candidate)
	second, okAgain := BestSentenceEvidence(candidate)
	if !ok || !okAgain || first.Sentence != "Dieses Haus steht seit vielen Jahren ruhig am See." || first.Sentence != second.Sentence || first.Target != second.Target || first.FirstEncounter != second.FirstEncounter || first.Quality.Accepted != second.Quality.Accepted || first.Quality.Score != second.Quality.Score || strings.Join(first.Quality.Reasons, "\x00") != strings.Join(second.Quality.Reasons, "\x00") {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

func TestExportCoverageOmitsBadEvidenceAndRecordsOnlyAcceptedNotes(t *testing.T) {
	store := &memoryStore{bookID: "book"}
	store.candidates = []domain.SelectionCandidate{
		{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 1, FirstEncounter: 10, ObservedForms: []byte(`["Haus"]`)},
		{Language: "de", CanonicalLemma: "Baum", UPOS: "NOUN", OccurrenceCount: 1, FirstEncounter: 20, ObservedForms: []byte(`["Baum"]`)},
	}
	store.entries = []Entry{
		{OwnerID: "alice", Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", Sentence: "Haus.", Translation: "house", SourceDocument: "Book", FirstEncounter: 10},
		{OwnerID: "alice", Language: "de", CanonicalLemma: "Baum", UPOS: "NOUN", Sentence: "Unter dem alten Baum warten heute mehrere müde Wanderer.", Translation: "tree", SentenceTranslation: "Several tired hikers are waiting under the old tree today.", SourceDocument: "Book", FirstEncounter: 20},
	}

	artifact, err := NewService(store).ExportCoverage(context.Background(), "alice", "book")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Count != 1 || artifact.Completeness != (Completeness{TotalCards: 1, CardsWithEnglish: 1, CardsWithEnglishSentence: 1, QualityOmitted: 1}) || len(artifact.Omitted) != 1 || artifact.Omitted[0].CanonicalLemma != "Haus" || len(artifact.EnrichmentCandidates) != 1 || artifact.EnrichmentCandidates[0].CanonicalLemma != "Baum" || artifact.EnrichmentCandidates[0].ExampleSentence == "" || len(store.generated) != 1 || !strings.Contains(artifact.TSV, "Baum") || strings.Contains(artifact.TSV, "Haus") {
		t.Fatalf("artifact=%+v generated=%+v", artifact, store.generated)
	}

	// Rejected evidence was not added to generated history, so improved evidence
	// remains eligible on a later export.
	store.entries[0].Sentence = "Vor dem alten Haus spielen heute mehrere fröhliche Kinder."
	artifact, err = NewService(store).ExportCoverage(context.Background(), "alice", "book")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Count != 2 || artifact.Completeness != (Completeness{TotalCards: 2, CardsWithEnglish: 2, CardsWithEnglishSentence: 1}) || len(artifact.Omitted) != 0 || len(store.generated) != 3 || !strings.Contains(artifact.TSV, "Haus") {
		t.Fatalf("later artifact=%+v generated=%+v", artifact, store.generated)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
