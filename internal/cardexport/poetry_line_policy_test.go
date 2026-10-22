package cardexport_test

import (
	"encoding/json"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/require"
)

const directorSentence = "Ihr beyden die ihr mir so oft,\nIn Noth und Trübsal, beygestanden,\nSagt was ihr wohl, in deutschen Landen,\nVon unsrer Unternehmung hofft?" //nolint:misspell // Goethe source text

func poetryProjection(sentence, lemma, target string) cardexport.CandidateProjection {
	refs, err := json.Marshal([]struct {
		SentenceIndex int              `json:"sentence_index"`
		Text          string           `json:"text"`
		Location      map[string]int64 `json:"location"`
	}{{SentenceIndex: 0, Text: sentence, Location: map[string]int64{"start_offset": 42}}})
	if err != nil {
		panic("poetry fixture references are not JSON encodable: " + err.Error())
	}
	return cardexport.CandidateProjection{
		OwnerID: "owner-1", DeckName: "Faust. Eine Tragödie",
		Candidate: domain.SelectionCandidate{
			OwnerID: "owner-1", CorpusID: "corpus-faust", Language: "de", CanonicalLemma: lemma, UPOS: "NOUN",
			ObservedForms: []byte(`["` + target + `"]`), SentenceReferences: refs,
		},
		Entry:    cardexport.Entry{OwnerID: "owner-1", Language: "de", CanonicalLemma: lemma, UPOS: "NOUN", SourceDocument: "Faust. Eine Tragödie"},
		Provider: "llm", ProviderVersion: "prompt-v2", TargetLanguage: "en",
	}
}

func poetryRunFacts() cardexport.RunFacts {
	return cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "standard", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v2"}
}

// TestPoetryQualityGateAdmitsMultilineAndOmitsDeliberateFragments covers the
// two new deck-preparation outcomes: a genuine four-source-line sentence is
// accepted and carries exact LF context, while the measured short and
// interrupted excerpts become quality omissions with no provider identity.
func TestPoetryQualityGateAdmitsMultilineAndOmitsDeliberateFragments(t *testing.T) {
	t.Run("multiline sentence is accepted with exact LF context", func(t *testing.T) {
		deck, diagnostics, err := cardexport.NewPresentation(nil).Freeze(t.Context(), "owner-1", "Faust. Eine Tragödie", []cardexport.CandidateProjection{
			poetryProjection(directorSentence, "unternehmung", "Unternehmung"),
		})
		require.NoError(t, err)
		require.Empty(t, diagnostics.QualityOmissions)
		require.Equal(t, 1, deck.Summary().Accepted)
		work := deck.WorkProjection()
		require.Len(t, work, 1)

		t.Run("keeps the exact newline-bearing sentence and cache identity", func(t *testing.T) {
			require.Equal(t, directorSentence, work[0].Request.ExampleSentence)
			require.Equal(t, enrichment.SentenceHash(directorSentence), work[0].CacheKey.SentenceHash)
		})
		t.Run("keeps the tested target only", func(t *testing.T) {
			require.Equal(t, "Unternehmung", work[0].Request.TargetWord)
		})
		t.Run("adds no attribution, title or role marker", func(t *testing.T) {
			payload, err := json.Marshal(work[0].Request) //nolint:musttag // the whole request is serialized to check for leaked context
			require.NoError(t, err)
			for _, forbidden := range []string{"Director", "Vorspiel", "Theaterdichter"} {
				require.NotContains(t, string(payload), forbidden)
			}
		})
	})

	for _, test := range []struct{ name, sentence, lemma, target string }{
		{"4:368 short exclamation", "Ach!", "ach", "Ach"},
		{"4:580 short exclamation", "O weh!", "weh", "weh"},
		{"0:599-602 interrupted Geist speech", "Du hast mich mächtig angezogen,\nAn meiner Sphäre lang’ gesogen,\nUnd nun –", "mächtig", "mächtig"},
	} {
		t.Run(test.name, func(t *testing.T) {
			deck, diagnostics, err := cardexport.NewPresentation(nil).Freeze(t.Context(), "owner-1", "Faust. Eine Tragödie", []cardexport.CandidateProjection{
				poetryProjection(test.sentence, test.lemma, test.target),
			})
			require.NoError(t, err)
			require.Len(t, diagnostics.QualityOmissions, 1)

			t.Run("omitted card gets no provider identity", func(t *testing.T) {
				require.Empty(t, deck.WorkProjection())
				items := deck.StorageProjection().Items
				require.Len(t, items, 1)
				require.Equal(t, cardexport.ManifestQualityOmitted, items[0].Disposition)
				require.Nil(t, items[0].CacheKey)
			})
			t.Run("omitted card produces no generated provenance", func(t *testing.T) {
				artifact, _, err := cardexport.NewPresentation(nil).Finalize(t.Context(), deck, nil, poetryRunFacts())
				require.NoError(t, err)
				require.Zero(t, artifact.Count)
				require.Empty(t, artifact.Generated)
			})
		})
	}
}

// TestPresentationOnlyRestoreKeepsFrozenQualityOmission pins that restoring a
// frozen deck never reruns the gate: a quality omission recorded under the
// previous newline policy stays omitted even though the current gate would
// accept the same sentence.
func TestPresentationOnlyRestoreKeepsFrozenQualityOmission(t *testing.T) {
	snapshot := cardexport.ManifestSnapshot{
		SchemaVersion: cardexport.ManifestSchemaVersion, Owner: "owner-1", DeckName: "Faust. Eine Tragödie",
		Filename: cardexport.DownloadFilename("Faust. Eine Tragödie"),
		Items: []cardexport.ManifestItem{{
			Ordinal: 0, Disposition: cardexport.ManifestQualityOmitted,
			Entry:   cardexport.Entry{Language: "de", CanonicalLemma: "unternehmung", UPOS: "NOUN", Sentence: directorSentence, TargetWord: "Unternehmung", SourceDocument: "Faust. Eine Tragödie"},
			Quality: cardexport.SentenceQuality{Accepted: false, Reasons: []string{"structural noise or boilerplate"}},
		}},
	}
	deck, err := cardexport.NewPresentation(nil).Restore(snapshot)
	require.NoError(t, err)

	t.Run("restore keeps the frozen omission", func(t *testing.T) {
		require.Zero(t, deck.Summary().Accepted)
		require.Empty(t, deck.WorkProjection())
		require.Equal(t, cardexport.ManifestQualityOmitted, deck.StorageProjection().Items[0].Disposition)
	})
	t.Run("finalize generates nothing", func(t *testing.T) {
		artifact, _, err := cardexport.NewPresentation(nil).Finalize(t.Context(), deck, nil, poetryRunFacts())
		require.NoError(t, err)
		require.Zero(t, artifact.Count)
		require.Empty(t, artifact.Generated)
	})
}
