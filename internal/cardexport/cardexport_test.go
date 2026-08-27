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
	note := Note{Key: DedupKey("de", "haus", "NOUN", "alice"), Identity: strings.Repeat("a", 64), Text: "Das <b>Haus</b> ist heute sehr ruhig.", Article: "das", Lemma: "Haus", English: "house", EnglishSentence: "The house is very quiet today.", BookTitle: "Das archaische Griechenland", SourceSentence: "Das Haus ist heute sehr ruhig.", Tags: []string{"Mouseion", "lang::de", "pos::NOUN", "source::Das_archaische_Griechenland"}}
	missingSentenceTranslation := note
	missingSentenceTranslation.Key = DedupKey("de", "baum", "NOUN", "alice")
	missingSentenceTranslation.Identity = strings.Repeat("b", 64)
	missingSentenceTranslation.Article = "der"
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
	if !strings.Contains(modelsJSON, `"name":"Mouseion Vocab Recognition"`) || !strings.Contains(modelsJSON, `"qfmt":"{{Front}}"`) || !strings.Contains(modelsJSON, `{{#Article}}{{Article}} {{/Article}}{{Lemma}}`) || !strings.Contains(modelsJSON, `{{SourceSentence}}`) || strings.Contains(modelsJSON, `{{Identity}}`) || strings.Contains(strings.ToLower(modelsJSON), "cloze") || strings.Contains(modelsJSON, `"name":"Morph"`) || !strings.Contains(decksJSON, deckName) {
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
	var noteID, cardCount, checksum int64
	var fields, tags, sortField string
	if err = db.QueryRow(`SELECT id,flds,tags,sfld,csum FROM notes WHERE guid=?`, note.Key[:20]).Scan(&noteID, &fields, &tags, &sortField, &checksum); err != nil {
		t.Fatal(err)
	}
	serializedFields := strings.Split(fields, "\x1f")
	if noteID != stableID("note|"+note.Key) || len(serializedFields) != 8 || serializedFields[0] != note.Identity || serializedFields[2] != note.Article || serializedFields[3] != note.Lemma || serializedFields[4] != note.English || serializedFields[5] != note.EnglishSentence || sortField != note.Identity || checksum != fieldChecksum(note.Identity) || !strings.Contains(tags, " Mouseion ") || strings.Contains(strings.ToLower(tags), "leech") {
		t.Fatalf("note id=%d fields=%q sfld=%q checksum=%d tags=%q", noteID, fields, sortField, checksum, tags)
	}
	if err = db.QueryRow(`SELECT flds FROM notes WHERE guid=?`, missingSentenceTranslation.Key[:20]).Scan(&fields); err != nil {
		t.Fatal(err)
	}
	serializedFields = strings.Split(fields, "\x1f")
	if len(serializedFields) != 8 || serializedFields[5] != "" {
		t.Fatalf("missing sentence translation fields=%q", fields)
	}
	var distinctSortFields int64
	if err = db.QueryRow(`SELECT count(DISTINCT sfld) FROM notes`).Scan(&distinctSortFields); err != nil || distinctSortFields != 2 {
		t.Fatalf("distinct identity sort fields=%d err=%v", distinctSortFields, err)
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

func TestCardIdentityIsDeterministicCardSpecificAndOwnerScoped(t *testing.T) {
	entry := Entry{Language: "de", CanonicalLemma: "gut", UPOS: "ADJ", TargetWord: "die Besten›", Sentence: "Heute sah sie ‹die Besten› und lächelte freundlich.", SourceDocument: "Book", FirstEncounter: 42}
	identity := CardIdentity("alice", entry)
	cleanEntry := entry
	cleanEntry.TargetWord = "die Besten"
	if len(identity) != 64 || identity != CardIdentity("alice", entry) || identity != CardIdentity("alice", cleanEntry) {
		t.Fatalf("identity is unstable across equivalent legacy target forms: %q", identity)
	}
	variants := []struct {
		owner string
		entry Entry
	}{
		{owner: "bob", entry: entry},
		{owner: "alice", entry: func() Entry {
			value := entry
			value.CanonicalLemma = "die"
			value.UPOS = "DET"
			value.TargetWord = "‹die"
			return value
		}()},
		{owner: "alice", entry: func() Entry { value := entry; value.FirstEncounter++; return value }()},
	}
	for _, variant := range variants {
		if got := CardIdentity(variant.owner, variant.entry); got == identity {
			t.Fatalf("distinct card inputs collided: %+v", variant)
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

func TestBoldTargetUsesOriginalUnicodeByteOffsets(t *testing.T) {
	for _, test := range []struct {
		sentence, target, want string
	}{
		{sentence: "ẞ Haus ist heute sehr groß.", target: "Haus", want: "ẞ <b>Haus</b> ist heute sehr groß."},
		{sentence: "Das ẞ steht heute dort ruhig.", target: "ß", want: "Das <b>ẞ</b> steht heute dort ruhig."},
		{sentence: "K Haus ist heute sehr groß.", target: "Haus", want: "K <b>Haus</b> ist heute sehr groß."},
		{sentence: "Heute blieb die Souveränität› vollständig erhalten.", target: "Souveränität›", want: "Heute blieb die <b>Souveränität</b>› vollständig erhalten."},
		{sentence: "Heute sah sie ‹die Besten› dort.", target: "die Besten›", want: "Heute sah sie ‹<b>die Besten</b>› dort."},
		{sentence: "Heute sah sie ‹die Besten› dort.", target: "‹die", want: "Heute sah sie ‹<b>die</b> Besten› dort."},
		{sentence: "Oggi O'Neill-like arriva insieme agli amici.", target: "O'Neill-like", want: "Oggi <b>O&#39;Neill-like</b> arriva insieme agli amici."},
	} {
		got, err := BoldTarget(test.sentence, test.target)
		if err != nil || got != test.want {
			t.Fatalf("BoldTarget(%q, %q) = %q, %v; want %q", test.sentence, test.target, got, err, test.want)
		}
	}
}

func TestRenderTSVEscapesAndOrdersFields(t *testing.T) {
	n := Note{Key: "key", Identity: "identity", Text: "Grüße\t{{c1::Welt}}", Article: "die", Lemma: "Welt", English: "world", EnglishSentence: "Hello world.", BookTitle: "My Book", SourceSentence: "Grüße Welt", Tags: []string{"Mouseion", "lang::de", "source::My_Book"}}
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
	if len(rows) != 1 || len(rows[0]) != 9 || rows[0][0] != n.Identity || rows[0][1] != n.Text || rows[0][2] != "die" || rows[0][3] != "Welt" || rows[0][7] != "Grüße Welt" || rows[0][8] != "Mouseion lang::de source::My_Book" {
		t.Fatalf("rows=%#v", rows)
	}
}

func TestEscapeFieldCannotInjectAnkiFieldSeparator(t *testing.T) {
	got := escapeField("safe\x1finjected")
	if strings.ContainsRune(got, '\x1f') || got != "safe&#31;injected" {
		t.Fatalf("escaped field = %q", got)
	}
}

func TestMakeNoteFormatsLemmaWithoutChangingTargetOrIdentity(t *testing.T) {
	entry := Entry{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		Sentence: "Die Häuser sind alt.", TargetWord: "Häuser", Morphology: `{"Gender":"Neut","Number":"Plur"}`,
	}
	note, err := makeNote("alice", entry)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(note.Text, "<b>Häuser</b>") || note.Article != "das" || note.Lemma != "Haus" || !strings.HasPrefix(note.BackExtra, "das Haus\n") {
		t.Fatalf("note = %#v", note)
	}
	if want := DedupKey("de", "haus", "NOUN", "alice"); note.Key != want {
		t.Fatalf("note key = %q, want canonical identity key %q", note.Key, want)
	}
}

func TestMakeNoteDerivesGermanArticleFromMorphology(t *testing.T) {
	for _, test := range []struct {
		name, language, lemma, sentence, target, morphology, wantArticle, wantLemma string
	}{
		{name: "feminine ignores declined context", language: "de", sentence: "Bei der Iteration wurde das Ergebnis erneut geprüft.", target: "Iteration", morphology: `{"Case":"Dat","Gender":"Fem","Number":"Sing"}`, wantArticle: "die", wantLemma: "Iteration"},
		{name: "neuter without context article", language: "de", sentence: "Dieses Ruderblatt wurde gestern sorgfältig ausgetauscht.", target: "Ruderblatt", morphology: `{"Gender":"Neut","Number":"Sing"}`, wantArticle: "das", wantLemma: "Ruderblatt"},
		{name: "masculine singular", language: "de", sentence: "Dort steht ein Baum seit vielen Jahren.", target: "Baum", morphology: `{"Gender":"Masc","Number":"Sing"}`, wantArticle: "der", wantLemma: "Baum"},
		{name: "plural inflection uses lemma gender", language: "de", lemma: "haus", sentence: "Viele Häuser stehen seit Jahren am See.", target: "Häuser", morphology: `{"Gender":"Neut","Number":"Plur"}`, wantArticle: "das", wantLemma: "Haus"},
		{name: "plural only without gender", language: "de", lemma: "eltern", sentence: "Meine Eltern wohnen seit Jahren am See.", target: "Eltern", morphology: `{"Number":"Plur"}`, wantArticle: "die", wantLemma: "Eltern"},
		{name: "gender sufficient when number missing", language: "de", sentence: "Dieses Haus steht seit Jahren am See.", target: "Haus", morphology: `{"Gender":"Neut"}`, wantArticle: "das", wantLemma: "Haus"},
		{name: "missing gender and singular number", language: "de", sentence: "Dieses Haus steht seit Jahren am See.", target: "Haus", morphology: `{"Number":"Sing"}`, wantLemma: "Haus"},
		{name: "consistent gender across number variants", language: "de", sentence: "Dieses Haus steht seit Jahren am See.", target: "Haus", morphology: `[{"Gender":"Neut","Number":"Sing"},{"Gender":"Neut","Number":"Plur"}]`, wantArticle: "das", wantLemma: "Haus"},
		{name: "ambiguous gender variants", language: "de", sentence: "Dieses Haus steht seit Jahren am See.", target: "Haus", morphology: `[{"Gender":"Neut","Number":"Sing"},{"Gender":"Masc","Number":"Sing"}]`, wantLemma: "Haus"},
		{name: "malformed morphology", language: "de", sentence: "Dieses Haus steht seit Jahren am See.", target: "Haus", morphology: `{`, wantLemma: "Haus"},
		{name: "non German", language: "it", sentence: "La casa è ancora molto grande oggi.", target: "casa", morphology: `{"Gender":"Fem","Number":"Sing"}`, wantLemma: "casa"},
	} {
		t.Run(test.name, func(t *testing.T) {
			lemma := test.lemma
			if lemma == "" {
				lemma = strings.ToLower(test.target)
			}
			note, err := makeNote("alice", Entry{Language: test.language, CanonicalLemma: lemma, UPOS: "NOUN", Sentence: test.sentence, TargetWord: test.target, Morphology: test.morphology})
			if err != nil {
				t.Fatal(err)
			}
			if note.Article != test.wantArticle || note.Lemma != test.wantLemma {
				t.Fatalf("article=%q lemma=%q, want article=%q lemma=%q", note.Article, note.Lemma, test.wantArticle, test.wantLemma)
			}
		})
	}
}

func TestBestSentenceEvidencePreservesCompleteSourceText(t *testing.T) {
	const source = "  Vor dem alten Haus spielen heute mehrere fröhliche Kinder.  "
	candidate := domain.SelectionCandidate{
		CanonicalLemma:     "haus",
		ObservedForms:      []byte(`["Haus"]`),
		SentenceReferences: []byte(`[{"text":"  Vor dem alten Haus spielen heute mehrere fröhliche Kinder.  ","location":{"start_offset":10}}]`),
	}
	evidence, ok := BestSentenceEvidence(candidate)
	if !ok || evidence.Sentence != source {
		t.Fatalf("evidence sentence = %q, ok=%v", evidence.Sentence, ok)
	}
}

func TestLongContextUsesValidatedShortFrontAndFullSourceOnBack(t *testing.T) {
	longSentence := "Das Haus steht am Rand, während die Kinder im großen Garten spielen und ihre Eltern " + strings.Repeat("das Abendessen vorbereiten und über den nächsten Tag sprechen ", 8) + "weitergehen."
	shortContext := "Das Haus steht am Rand"
	entry := Entry{
		OwnerID: "alice", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: longSentence,
		TargetWord: "Haus", ContextSentence: shortContext, Translation: "house",
		SentenceTranslation: "The house stands at the edge while the children play.", SourceDocument: "Book",
		FirstEncounter: 12,
	}
	store := &memoryStore{bookID: "book", candidates: []domain.SelectionCandidate{{
		OwnerID: "alice", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1,
		ObservedForms: []byte(`["Haus"]`), SentenceReferences: []byte(`[{"text":` + jsonString(longSentence) + `,"location":{"start_offset":12}}]`),
	}}, entries: []Entry{entry}}

	store.entries[0].ContextSentence = ""
	withoutContext, err := NewService(store).BuildCoverage(context.Background(), "alice", "book")
	if err != nil {
		t.Fatal(err)
	}
	if withoutContext.Count != 0 || withoutContext.Completeness.QualityOmitted != 1 || len(withoutContext.Omitted) != 1 || !contains(withoutContext.Omitted[0].Reasons, "too long") || len(withoutContext.EnrichmentCandidates) != 1 {
		t.Fatalf("without context artifact=%+v", withoutContext)
	}
	store.entries[0].ContextSentence = shortContext
	artifact, err := NewService(store).BuildCoverage(context.Background(), "alice", "book")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Count != 1 || artifact.Completeness.QualityOmitted != 0 || len(artifact.Omitted) != 0 || len(artifact.Generated) != 1 {
		t.Fatalf("artifact=%+v", artifact)
	}
	note := artifact.Generated[0].Note
	if note.Text != "Das <b>Haus</b> steht am Rand" || note.SourceSentence != longSentence || !strings.Contains(note.BackExtra, note.SourceSentence) || !strings.Contains(artifact.TSV, longSentence) {
		t.Fatalf("note=%+v TSV=%q", note, artifact.TSV)
	}
}

func TestPreparedArtifactCoversRecognitionContractAcrossAPKGAndTSV(t *testing.T) {
	const owner, bookID, sourceDocument = "alice", "book", "German rollout book"
	longSentence := "Das Haus steht am Rand," + strings.Repeat(" während die Kinder im großen Garten spielen", 12) + "."
	shortContext := "Das Haus steht am Rand"
	punctuationSentence := "Heute sah sie ‹die Besten› und lächelte freundlich."
	fixtures := []struct {
		lemma, upos, target, sentence, morphology, translation, sentenceTranslation string
		context                                                                     string
		firstEncounter                                                              int64
	}{
		{lemma: "die", upos: "DET", target: "‹die", sentence: punctuationSentence, translation: "the", sentenceTranslation: "Today she saw the best and smiled kindly.", firstEncounter: 10},
		{lemma: "gut", upos: "ADJ", target: "die Besten›", sentence: punctuationSentence, translation: "good", sentenceTranslation: "Today she saw the best and smiled kindly.", firstEncounter: 20},
		{lemma: "souveränität", upos: "NOUN", target: "Souveränität›", sentence: "Über Souveränität› sprach die Professorin gestern sehr ausführlich.", morphology: `{"Gender":"Fem","Number":"Sing"}`, translation: "sovereignty", sentenceTranslation: "The professor spoke about sovereignty in detail yesterday.", firstEncounter: 25},
		{lemma: "geleiten", upos: "VERB", target: "geleitet", sentence: "Nausikaa hat ihn geleitet und danach den Weg beschrieben.", translation: "to guide", sentenceTranslation: "Nausikaa guided him and then described the path.", firstEncounter: 30},
		{lemma: "buch", upos: "NOUN", target: "Buch", sentence: "Ich lese heute das Buch und lerne daraus viel.", morphology: `{"Gender":"Neut","Number":"Sing"}`, translation: "book", sentenceTranslation: "Today I read the book and learn a lot from it.", firstEncounter: 40},
		{lemma: "iteration", upos: "NOUN", target: "Iteration", sentence: "Bei der Iteration wurde das Ergebnis erneut sorgfältig geprüft.", morphology: `{"Case":"Dat","Gender":"Fem","Number":"Sing"}`, translation: "iteration", sentenceTranslation: "The result was carefully checked again during the iteration.", firstEncounter: 42},
		{lemma: "ruderblatt", upos: "NOUN", target: "Ruderblatt", sentence: "Dieses Ruderblatt wurde gestern in der Werkstatt sorgfältig ausgetauscht.", morphology: `{"Gender":"Neut","Number":"Sing"}`, translation: "rudder blade", sentenceTranslation: "This rudder blade was carefully replaced in the workshop yesterday.", firstEncounter: 44},
		{lemma: "besuchen", upos: "VERB", target: "besucht", sentence: "Morgen besucht Anna ihre Klasse im Museum und lernt viel.", translation: "to visit", sentenceTranslation: "Tomorrow Anna visits her class at the museum and learns a lot.", firstEncounter: 50},
		{lemma: "haus", upos: "NOUN", target: "Haus", sentence: longSentence, context: shortContext, morphology: `{"Gender":"Neut","Number":"Sing"}`, translation: "house", sentenceTranslation: "The house stands at the edge while the children play in the garden.", firstEncounter: 60},
	}
	store := &memoryStore{bookID: bookID}
	for _, fixture := range fixtures {
		store.candidates = append(store.candidates, domain.SelectionCandidate{
			OwnerID: owner, Language: "de", CanonicalLemma: fixture.lemma, UPOS: fixture.upos, OccurrenceCount: 1,
			ObservedForms:      []byte("[" + jsonString(fixture.target) + "]"),
			SentenceReferences: []byte("[{\"sentence_index\":0,\"text\":" + jsonString(fixture.sentence) + ",\"location\":{\"start_offset\":" + fmt.Sprint(fixture.firstEncounter) + "}}]"),
		})
		store.entries = append(store.entries, Entry{
			OwnerID: owner, Language: "de", CanonicalLemma: fixture.lemma, UPOS: fixture.upos, Sentence: fixture.sentence,
			TargetWord: fixture.target, ContextSentence: fixture.context, Translation: fixture.translation,
			SentenceTranslation: fixture.sentenceTranslation, Morphology: fixture.morphology, SourceDocument: sourceDocument, FirstEncounter: fixture.firstEncounter,
		})
	}

	service := NewService(store)
	first, err := service.BuildCoverage(context.Background(), owner, bookID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.BuildCoverage(context.Background(), owner, bookID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.APKG, second.APKG) || first.TSV != second.TSV {
		t.Fatal("prepared artifact is not deterministic")
	}
	if first.Count != len(fixtures) || len(first.Generated) != len(fixtures) || len(first.Omitted) != 0 || first.Completeness != (Completeness{TotalCards: len(fixtures), CardsWithEnglish: len(fixtures), CardsWithEnglishSentence: len(fixtures)}) {
		t.Fatalf("artifact=%+v", first)
	}

	notesByLemma := make(map[string]Note, len(first.Generated))
	for _, generated := range first.Generated {
		notesByLemma[generated.Entry.CanonicalLemma] = generated.Note
		for _, value := range []string{generated.Note.Identity, generated.Note.Text, generated.Note.Article, generated.Note.Lemma, generated.Note.English, generated.Note.EnglishSentence, generated.Note.BookTitle, generated.Note.SourceSentence, generated.Note.BackExtra} {
			if strings.Contains(value, "{{c1::") || strings.Contains(value, "geleiten|leiten") || strings.Contains(value, `"Case"`) {
				t.Fatalf("legacy or raw analyzer content in note %q: %+v", value, generated.Note)
			}
		}
	}
	if got := notesByLemma["die"].Text; !strings.Contains(got, "‹<b>die</b>") || notesByLemma["die"].SourceSentence != punctuationSentence {
		t.Fatalf("punctuation-bearing front=%q source=%q", got, notesByLemma["die"].SourceSentence)
	}
	if got := notesByLemma["gut"].Text; !strings.Contains(got, "‹<b>die Besten</b>›") || notesByLemma["gut"].SourceSentence != punctuationSentence {
		t.Fatalf("trailing punctuation front=%q source=%q", got, notesByLemma["gut"].SourceSentence)
	}
	if got := notesByLemma["souveränität"].Text; !strings.Contains(got, "<b>Souveränität</b>›") || notesByLemma["souveränität"].SourceSentence != fixtures[2].sentence {
		t.Fatalf("legacy sovereignty front=%q source=%q", got, notesByLemma["souveränität"].SourceSentence)
	}
	if notesByLemma["geleiten"].Lemma != "geleiten" {
		t.Fatalf("pipe lemma leaked or was not normalized: %q", notesByLemma["geleiten"].Lemma)
	}
	if notesByLemma["buch"].Article != "das" || notesByLemma["buch"].Lemma != "Buch" || notesByLemma["buch"].Key != DedupKey("de", "buch", "NOUN", owner) {
		t.Fatalf("article display changed identity: %+v", notesByLemma["buch"])
	}
	if note := notesByLemma["iteration"]; note.Article != "die" || note.Lemma != "Iteration" || !strings.HasPrefix(note.BackExtra, "die Iteration\n") {
		t.Fatalf("declined context article leaked into card: %+v", note)
	}
	if note := notesByLemma["ruderblatt"]; note.Article != "das" || note.Lemma != "Ruderblatt" || !strings.HasPrefix(note.BackExtra, "das Ruderblatt\n") {
		t.Fatalf("article without sentence determiner = %+v", note)
	}
	if notesByLemma["die"].Identity == notesByLemma["gut"].Identity {
		t.Fatal("different targets in the same sentence share an Identity")
	}
	for _, candidate := range first.EnrichmentCandidates {
		if candidate.TargetWord == "‹die" || candidate.TargetWord == "die Besten›" {
			t.Fatalf("legacy punctuation leaked into enrichment candidate: %+v", candidate)
		}
	}
	longNote := notesByLemma["haus"]
	if longNote.Text != "Das <b>Haus</b> steht am Rand" || longNote.SourceSentence != longSentence || !strings.Contains(longNote.BackExtra, longSentence) || !strings.Contains(longNote.BackExtra, fixtures[len(fixtures)-1].sentenceTranslation) {
		t.Fatalf("long-context note=%+v", longNote)
	}

	rows, err := readTSV(first.TSV)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(fixtures) || len(rows[0]) != 9 || !strings.Contains(first.TSV, longSentence) || !strings.Contains(first.TSV, "\tdas\tBuch\t") || strings.Contains(first.TSV, "Morph") || strings.Contains(first.TSV, "{{c1::") {
		t.Fatalf("TSV rows=%#v TSV=%q", rows, first.TSV)
	}
	for i, generated := range first.Generated {
		if strings.Join(rows[i][:8], "\x1f") != strings.Join(noteFields(generated.Note), "\x1f") {
			t.Fatalf("TSV row %d=%#v note=%#v", i, rows[i], generated.Note)
		}
	}
	modelsJSON, apkgRows := readAPKGNotes(t, first.APKG)
	if len(apkgRows) != len(fixtures) || strings.Contains(strings.ToLower(modelsJSON), "cloze") || strings.Contains(modelsJSON, `"name":"Morph"`) {
		t.Fatalf("APKG model=%s rows=%#v", modelsJSON, apkgRows)
	}
	apkgByLemma := make(map[string][]string, len(apkgRows))
	for _, row := range apkgRows {
		if len(row) != 8 || strings.Contains(row[1], "{{c1::") || strings.Contains(row[3], "geleiten|leiten") || strings.Contains(row[3], `"Case"`) {
			t.Fatalf("APKG row=%#v", row)
		}
		apkgByLemma[row[3]] = row
	}
	for _, generated := range first.Generated {
		row, ok := apkgByLemma[generated.Note.Lemma]
		if !ok || strings.Join(row, "\x1f") != strings.Join(noteFields(generated.Note), "\x1f") {
			t.Fatalf("APKG row=%#v note=%#v", row, generated.Note)
		}
	}
}

func readTSV(value string) ([][]string, error) {
	reader := csv.NewReader(strings.NewReader(value))
	reader.Comma = '\t'
	return reader.ReadAll()
}

func readAPKGNotes(t *testing.T, payload []byte) (string, [][]string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	var collection []byte
	for _, member := range zr.File {
		if member.Name != "collection.anki2" {
			continue
		}
		reader, openErr := member.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		collection, err = io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
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
	var modelsJSON string
	if err = db.QueryRow(`SELECT models FROM col`).Scan(&modelsJSON); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT flds FROM notes`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var fields [][]string
	for rows.Next() {
		var serialized string
		if err = rows.Scan(&serialized); err != nil {
			t.Fatal(err)
		}
		fields = append(fields, strings.Split(serialized, "\x1f"))
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return modelsJSON, fields
}

func jsonString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
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
	if !strings.Contains(note.Text, "<b>porterà</b>") || note.Article != "" || note.Lemma != "portare" || note.SourceSentence != "Domani Lucia porterà finalmente il pane fresco alla sua famiglia." || !strings.Contains(artifact.TSV, "lang::it") {
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
