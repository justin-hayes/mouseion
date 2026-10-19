package cardexport

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The French article rule (ADR 0102) reads one supported gender and the
// headword's initial letter, and shows a gender label only where the article
// cannot reveal the gender. Every row of the acceptance table is pinned here.
func TestMakeNoteRendersFrenchArticleTable(t *testing.T) {
	for _, test := range []struct {
		name, lemma, sentence, target, morphology string
		wantArticle, wantGender, wantHead         string
	}{
		{name: "consonant initial masculine", lemma: "livre", sentence: "Le livre est sur la table.", target: "livre", morphology: `{"Gender":"Masc","Number":"Sing"}`, wantArticle: "le", wantGender: "", wantHead: "le livre"},
		{name: "consonant initial feminine", lemma: "maison", sentence: "La maison est grande.", target: "maison", morphology: `{"Gender":"Fem","Number":"Sing"}`, wantArticle: "la", wantGender: "", wantHead: "la maison"},
		{name: "vowel initial masculine keeps a gender label", lemma: "ami", sentence: "Un ami arrive ce soir.", target: "ami", morphology: `{"Gender":"Masc","Number":"Sing"}`, wantArticle: "l'", wantGender: "m.", wantHead: "l'ami (m.)"},
		{name: "vowel initial feminine keeps a gender label", lemma: "école", sentence: "Une école ouvre demain.", target: "école", morphology: `{"Gender":"Fem","Number":"Sing"}`, wantArticle: "l'", wantGender: "f.", wantHead: "l'école (f.)"},
		{name: "accented initial vowel still elides", lemma: "île", sentence: "Une île apparaît au loin.", target: "île", morphology: `{"Gender":"Fem","Number":"Sing"}`, wantArticle: "l'", wantGender: "f.", wantHead: "l'île (f.)"},
		{name: "diphthong ligature initial still elides", lemma: "œuf", sentence: "Un œuf est frais.", target: "œuf", morphology: `{"Gender":"Masc","Number":"Sing"}`, wantArticle: "l'", wantGender: "m.", wantHead: "l'œuf (m.)"},
		{name: "aspirated h takes the definite article", lemma: "hache", sentence: "La hache est lourde.", target: "hache", morphology: `{"Gender":"Fem","Number":"Sing","HStatus":"aspirated"}`, wantArticle: "la", wantGender: "", wantHead: "la hache"},
		{name: "non-aspirated h appears bare with gender", lemma: "héros", sentence: "Le héros revient au village.", target: "héros", morphology: `{"Gender":"Masc","Number":"Sing"}`, wantArticle: "", wantGender: "m.", wantHead: "héros (m.)"},
		{name: "unknown h status is never treated as mute", lemma: "hôtel", sentence: "Cet hôtel ouvre en mai.", target: "hôtel", morphology: `{"Gender":"Masc","Number":"Sing"}`, wantArticle: "", wantGender: "m.", wantHead: "hôtel (m.)"},
		{name: "uncertain y initial appears bare with gender", lemma: "yaourt", sentence: "Le yaourt est sucré.", target: "yaourt", morphology: `{"Gender":"Masc","Number":"Sing"}`, wantArticle: "", wantGender: "m.", wantHead: "yaourt (m.)"},
		{name: "conflicting gender is bare without a label", lemma: "élève", sentence: "Un élève travaille ici.", target: "élève", morphology: `[{"Gender":"Masc","Number":"Sing"},{"Gender":"Fem","Number":"Sing"}]`, wantArticle: "", wantGender: "", wantHead: "élève"},
		{name: "missing gender is bare without a label", lemma: "espèce", sentence: "Cette espèce est rare.", target: "espèce", morphology: `{"Number":"Sing"}`, wantArticle: "", wantGender: "", wantHead: "espèce"},
		{name: "plural-only gender stays bare without a label", lemma: "gens", sentence: "Les gens arrivent ce soir.", target: "gens", morphology: `{"Gender":"Masc","Number":"Plur"}`, wantArticle: "", wantGender: "", wantHead: "gens"},
	} {
		t.Run(test.name, func(t *testing.T) {
			note, err := makeNote("alice", renderInputFromEntry(Entry{
				Language: "fr", CanonicalLemma: test.lemma, UPOS: "NOUN",
				Sentence: test.sentence, TargetWord: test.target, Morphology: test.morphology,
			}))
			require.NoError(t, err)
			assert.Equal(t, strings.ReplaceAll(test.wantArticle, "'", "&#39;"), note.Article)
			assert.Equal(t, test.wantGender, note.Gender)
			assert.Equal(t, test.lemma, note.Lemma, "lemma capitalisation display stays German-only")
			assert.True(t, strings.HasPrefix(note.BackExtra, strings.ReplaceAll(test.wantHead, "'", "&#39;")+"\n"), "back = %q", note.BackExtra)
			if test.wantGender == "" {
				assert.NotContains(t, note.BackExtra, "(m.)")
				assert.NotContains(t, note.BackExtra, "(f.)")
			}
		})
	}
}

// Defensive French article helpers reject an empty headword and an
// unrecognised gender rather than guessing, and a singular unrecognised gender
// is fatal to the evidence fold.
func TestFrenchArticleHelpersRejectUnknownInput(t *testing.T) {
	t.Run("an unrecognised gender yields no article or label", func(t *testing.T) {
		assert.Empty(t, frenchDefiniteArticle("common"))
		assert.Empty(t, frenchGenderLabel("common"))
	})
	t.Run("an empty headword yields no prefix", func(t *testing.T) {
		article, label := frenchNounPresentation("masc", "", "")
		assert.Empty(t, article)
		assert.Empty(t, label)
	})
	t.Run("a singular unrecognised gender is fatal", func(t *testing.T) {
		note, err := makeNote("alice", renderInputFromEntry(Entry{
			Language: "fr", CanonicalLemma: "espèce", UPOS: "NOUN",
			Sentence: "Cette espèce est rare.", TargetWord: "espèce",
			Morphology: `{"Gender":"Common","Number":"Sing"}`,
		}))
		require.NoError(t, err)
		assert.Empty(t, note.Article)
		assert.Empty(t, note.Gender)
	})
}

// Gender appears only on NOUN rows: a French verb carries no article and no
// gender label even when its morphology names a gender.
func TestMakeNoteKeepsFrenchGenderOnNounsOnly(t *testing.T) {
	note, err := makeNote("alice", renderInputFromEntry(Entry{
		Language: "fr", CanonicalLemma: "aller", UPOS: "VERB",
		Sentence: "Il va au marché.", TargetWord: "va",
		Morphology: `{"Gender":"Masc","Number":"Sing"}`,
	}))
	require.NoError(t, err)
	assert.Empty(t, note.Article)
	assert.Empty(t, note.Gender)
	assert.Equal(t, "aller", note.Lemma)
}

// IPA, "invariable" and the optional fields render only from index evidence;
// a missing plural, IPA or conjugation summary is left out rather than invented.
func TestMakeNoteRendersFrenchEvidenceAndOmitsMissingFields(t *testing.T) {
	t.Run("invariable plural and IPA render from index evidence", func(t *testing.T) {
		note, err := makeNote("alice", renderInputFromEntry(Entry{
			Language: "fr", CanonicalLemma: "prix", UPOS: "NOUN",
			Sentence: "Le prix du pain a augmenté.", TargetWord: "prix",
			Morphology: `{"Gender":"Masc","Number":"Sing"}`, Plural: "invariable", IPA: "/pʁi/",
		}))
		require.NoError(t, err)
		assert.Equal(t, "invariable", note.Plural)
		assert.Equal(t, "/pʁi/", note.IPA)
		assert.Contains(t, note.BackExtra, "(Pl. invariable)")
		assert.Contains(t, note.BackExtra, "/pʁi/")
	})

	t.Run("the optional schwa is preserved verbatim", func(t *testing.T) {
		note, err := makeNote("alice", renderInputFromEntry(Entry{
			Language: "fr", CanonicalLemma: "souvenir", UPOS: "NOUN",
			Sentence: "Le souvenir reste vivant.", TargetWord: "souvenir",
			Morphology: `{"Gender":"Masc","Number":"Sing"}`, IPA: "/su.v(ə).niʁ/",
		}))
		require.NoError(t, err)
		assert.Equal(t, "/su.v(ə).niʁ/", note.IPA)
	})

	t.Run("a missing plural, IPA and conjugation summary are omitted", func(t *testing.T) {
		note, err := makeNote("alice", renderInputFromEntry(Entry{
			Language: "fr", CanonicalLemma: "manger", UPOS: "VERB",
			Sentence: "Ils mangent ensemble.", TargetWord: "mangent",
		}))
		require.NoError(t, err)
		assert.Empty(t, note.Plural)
		assert.Empty(t, note.IPA)
		assert.Empty(t, note.PrincipalParts)
		assert.NotContains(t, note.BackExtra, "(Pl.")
		assert.NotContains(t, note.BackExtra, "/")
	})
}

// A French prepared deck uses the Mouseion::fr::<book title> hierarchy and the
// lang::fr tag.
func TestFrenchPreparedDeckUsesLanguageHierarchyAndTag(t *testing.T) {
	note, err := makeNote("alice", renderInputFromEntry(Entry{
		Language: "fr", CanonicalLemma: "maison", UPOS: "NOUN", Sentence: "La maison est grande.",
		TargetWord: "maison", SourceDocument: "Le Petit Prince", Morphology: `{"Gender":"Fem","Number":"Sing"}`,
	}))
	require.NoError(t, err)
	assert.Equal(t, []string{"Mouseion", "lang::fr", "pos::NOUN", "source::Le_Petit_Prince"}, note.Tags)
	deckName := DeckName("fr", "Le Petit Prince")
	assert.Equal(t, "Mouseion::fr::Le Petit Prince", deckName)
	apkg, err := renderAPKG(t.Context(), deckName, []Note{note}, "")
	require.NoError(t, err)
	assert.Contains(t, collectionColumn(t, apkg, "decks"), deckName)
}

// A dash-led French dialogue sentence is an eligible representative sentence:
// the card carries the persisted sentence verbatim, leading dash included, with
// the target anchored.
func TestFrenchDialogueSentenceIsEligibleAndRendersVerbatim(t *testing.T) {
	const sentence = "— Le tigre bondit sur sa proie."
	candidate := domain.SelectionCandidate{
		CorpusID: "french-corpus", Language: "fr", CanonicalLemma: "tigre",
		ObservedForms:      []byte(`["tigre"]`),
		SentenceReferences: []byte(`[{"sentence_index":0,"text":"` + sentence + `","location":{"start_offset":0}}]`),
	}
	sentences := map[int64]analyzer.Sentence{0: {
		Text:     sentence,
		Location: analyzer.SourceLocation{StartOffset: 0},
	}}

	evidence, ok := sentencequality.BestSentenceEvidenceFromCorpus(candidate, sentences)
	require.True(t, ok)
	require.True(t, evidence.Quality.Accepted, "quality=%+v", evidence.Quality)
	assert.Equal(t, sentence, evidence.Sentence, "the representative sentence is persisted verbatim")
	require.Equal(t, "tigre", evidence.Target)

	note, err := makeNote("alice", renderInputFromEntry(Entry{
		Language: "fr", CanonicalLemma: "tigre", UPOS: "NOUN",
		Sentence: evidence.Sentence, TargetWord: evidence.Target,
		FirstEncounter: evidence.FirstEncounter, SentenceTokens: evidence.Tokens,
		SentenceStartOffset: evidence.SentenceStartOffset,
	}))
	require.NoError(t, err)
	assert.Equal(t, "— Le <b>tigre</b> bondit sur sa proie.", note.Text)
}

// A missing defensible meaning yields the established retryable provider
// omission, and the rest of the French deck still exports.
func TestFrenchDeckExportsRestWhenOneMeaningIsOmitted(t *testing.T) {
	dictionary := lexicalStub{found: true, result: enrichment.LexicalEntry{
		Senses:          []enrichment.LexicalSense{{Gloss: "friend"}},
		CandidateSenses: []enrichment.LexicalSense{{Gloss: "friend", EvidenceID: "wiktionary:fr:ami"}},
	}}
	persisted := map[int64]analyzer.Sentence{
		0: {Text: "Je vois l'homme près du marché.", Location: analyzer.SourceLocation{StartOffset: 0}},
		1: {Text: "Il va au marché avec sa sœur.", Location: analyzer.SourceLocation{StartOffset: 32}},
	}
	startOffsets := map[int]int64{0: 0, 1: 32}
	projection := func(index int, lemma, sentence, target, morphology string) CandidateProjection {
		refs, err := json.Marshal([]struct {
			SentenceIndex int              `json:"sentence_index"`
			Text          string           `json:"text"`
			Location      map[string]int64 `json:"location"`
		}{{SentenceIndex: index, Text: sentence, Location: map[string]int64{"start_offset": startOffsets[index]}}})
		require.NoError(t, err)
		return CandidateProjection{
			OwnerID: "alice", DeckName: "Le Petit Prince",
			Candidate: domain.SelectionCandidate{OwnerID: "alice", CorpusID: "french-corpus", Language: "fr", CanonicalLemma: lemma, UPOS: "NOUN", ObservedForms: []byte(`["` + target + `"]`), SentenceReferences: refs},
			Entry:     Entry{Language: "fr", CanonicalLemma: lemma, UPOS: "NOUN", Sentence: sentence, TargetWord: target, Morphology: morphology},
			Sentences: persisted,
			Provider:  "llm", ProviderVersion: "prompt-fr", TargetLanguage: "en", RequireContextualGloss: true,
		}
	}
	presentation := NewPresentation(dictionary)
	deck, _, err := presentation.Freeze(t.Context(), "alice", "Le Petit Prince", []CandidateProjection{
		projection(0, "marché", "Je vois l'homme près du marché.", "marché", `{"Gender":"Masc","Number":"Sing"}`),
		projection(1, "sœur", "Il va au marché avec sa sœur.", "sœur", `{"Gender":"Fem","Number":"Sing"}`),
	})
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 2)

	var resolved, omitted WorkItem
	for _, item := range work {
		if item.Request.CanonicalLemma == "marché" {
			resolved = item
		} else {
			omitted = item
		}
	}
	artifact, diagnostics, err := NewPresentation(dictionary).Finalize(t.Context(), deck, []StoredResult{
		{CacheKey: resolved.CacheKey, Record: enrichment.CacheEntry{CacheKey: resolved.CacheKey, Translation: "market", FallbackGloss: "market", SentenceTranslation: "He goes to the market with his sister.", SentenceTranslationTargets: []string{"market"}}},
		{CacheKey: omitted.CacheKey, OmissionReason: "No defensible contextual meaning."},
	}, RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-fr"})
	require.NoError(t, err)
	assert.Equal(t, "Mouseion::fr::Le Petit Prince", artifact.DeckName)
	require.Len(t, artifact.Generated, 1)
	assert.Equal(t, "marché", artifact.Generated[0].Note.Lemma)
	assert.Equal(t, []MeaningOmission{{Target: "sœur", Reason: "No defensible contextual meaning."}}, diagnostics.MeaningOmissions)
}
