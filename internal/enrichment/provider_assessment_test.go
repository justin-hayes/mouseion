package enrichment

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func passCodec(t *testing.T) *TranslationCodec {
	t.Helper()
	codec, err := NewTranslationCodec(LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	return codec
}

// TestIdiomCardAssessmentIsRecordedForItalianOnly proves the passing Italian
// review records the idiom payload for Italian and no other language.
func TestIdiomCardAssessmentIsRecordedForItalianOnly(t *testing.T) {
	assertRecordedAssessment(t, "#2389", IdiomCardPayload, "it",
		(*TranslationCodec).IdiomCardPromptContract, IdiomCardPayloadFields)

	t.Run("another language is refused", func(t *testing.T) {
		require.ErrorIs(t, ProviderAssessments().MaySend(IdiomCardPayload, "de", passCodec(t)), ErrProviderAssessmentNotRecorded, "a language without an idiom assessment is not rolled out by implication")
	})
}

// assertRecordedAssessment proves one compiled assessment is recorded, names
// its payload kind, and matches the codec's current prompt contract and field
// list, so the compiled set allows its payload.
func assertRecordedAssessment(t *testing.T, ticket string, kind ProviderPayloadKind, language string, promptContract func(*TranslationCodec) string, payloadFields func() []string) {
	t.Helper()
	assessment := assessmentByTicketAndKind(t, ticket, kind)
	codec := passCodec(t)

	t.Run("the assessment is recorded", func(t *testing.T) {
		require.True(t, assessment.Recorded)
	})
	t.Run("the record names the payload kind it covers", func(t *testing.T) {
		require.Equal(t, kind, assessment.Kind)
	})
	t.Run("the recorded prompt contract is the one the codec sends", func(t *testing.T) {
		require.Equal(t, promptContract(codec), assessment.PromptVersion, "a new prompt needs a new record")
	})
	t.Run("the recorded field set is the one the codec sends", func(t *testing.T) {
		require.Equal(t, payloadFields(), assessment.Fields, "a changed payload needs a new record")
	})
	t.Run("the compiled set allows the payload", func(t *testing.T) {
		require.NoError(t, ProviderAssessments().MaySend(kind, language, codec))
	})
}

// TestProviderAssessmentSetMaySend proves the one question, for every payload
// kind, allows only an exact match against the current codec and refuses every
// other case.
func TestProviderAssessmentSetMaySend(t *testing.T) {
	codec := passCodec(t)
	kinds := []struct {
		name     string
		kind     ProviderPayloadKind
		language string
		provider string
		prompt   string
		fields   []string
	}{
		{
			name: "Greek evidence-assisted pass", kind: GreekEntryEvidenceAssistedPassPayload, language: "grc",
			provider: codec.ProviderName(), prompt: codec.GreekEntryEvidenceAssistedPassPromptContract(), fields: GreekEntryEvidenceAssistedPassPayloadFields(),
		},
		{
			name: "Latin evidence-assisted pass", kind: LatinEvidenceAssistedPassPayload, language: "la",
			provider: codec.ProviderName(), prompt: codec.LatinEvidenceAssistedPassPromptContract(), fields: LatinEvidenceAssistedPassPayloadFields(),
		},
		{
			name: "idiom card", kind: IdiomCardPayload, language: "it",
			provider: codec.ProviderName(), prompt: codec.IdiomCardPromptContract(), fields: IdiomCardPayloadFields(),
		},
		{
			name: "fluency agreement", kind: FluencyAgreementPayload, language: "de",
			provider: codec.ProviderName(), prompt: codec.FluencyAgreementPromptContract(), fields: FluencyAgreementPayloadFields(),
		},
	}
	for _, kind := range kinds {
		t.Run(kind.name, func(t *testing.T) {
			recorded := ProviderAssessment{
				Kind: kind.kind, Language: kind.language, Provider: kind.provider,
				PromptVersion: kind.prompt, Fields: kind.fields,
				Disposition: "recorded for this test", Ticket: "#2463", Recorded: true,
			}

			t.Run("an exact match allows", func(t *testing.T) {
				require.NoError(t, ProviderAssessmentSet{recorded}.MaySend(kind.kind, kind.language, codec))
			})
			t.Run("an unrecorded assessment refuses", func(t *testing.T) {
				unrecorded := recorded
				unrecorded.Recorded = false
				require.ErrorIs(t, ProviderAssessmentSet{unrecorded}.MaySend(kind.kind, kind.language, codec), ErrProviderAssessmentNotRecorded)
			})
			t.Run("a drifted provider refuses", func(t *testing.T) {
				drifted := recorded
				drifted.Provider = "other-provider"
				require.ErrorIs(t, ProviderAssessmentSet{drifted}.MaySend(kind.kind, kind.language, codec), ErrProviderAssessmentNotRecorded)
			})
			t.Run("a drifted prompt version refuses", func(t *testing.T) {
				drifted := recorded
				drifted.PromptVersion = "an-older-prompt"
				require.ErrorIs(t, ProviderAssessmentSet{drifted}.MaySend(kind.kind, kind.language, codec), ErrProviderAssessmentNotRecorded)
			})
			t.Run("a drifted field list refuses", func(t *testing.T) {
				drifted := recorded
				drifted.Fields = append(append([]string(nil), recorded.Fields...), "book_title")
				require.ErrorIs(t, ProviderAssessmentSet{drifted}.MaySend(kind.kind, kind.language, codec), ErrProviderAssessmentNotRecorded)
			})
			t.Run("a language with no record refuses", func(t *testing.T) {
				unrecordedLanguage := "fr"
				require.ErrorIs(t, ProviderAssessmentSet{recorded}.MaySend(kind.kind, unrecordedLanguage, codec), ErrProviderAssessmentNotRecorded)
			})
			t.Run("a record for another kind refuses", func(t *testing.T) {
				otherKind := GreekEntryEvidenceAssistedPassPayload
				if kind.kind == otherKind {
					otherKind = IdiomCardPayload
				}
				require.ErrorIs(t, ProviderAssessmentSet{recorded}.MaySend(otherKind, kind.language, codec), ErrProviderAssessmentNotRecorded)
			})
			t.Run("a missing codec refuses", func(t *testing.T) {
				require.ErrorIs(t, ProviderAssessmentSet{recorded}.MaySend(kind.kind, kind.language, nil), ErrProviderAssessmentNotRecorded)
			})
		})
	}
}

// TestProviderAssessmentLanguageWithoutARecordRefuses proves an assessed payload
// for a language the installation records no assessment for fails closed.
func TestProviderAssessmentLanguageWithoutARecordRefuses(t *testing.T) {
	codec := passCodec(t)
	require.ErrorIs(t, ProviderAssessments().MaySend(GreekEntryEvidenceAssistedPassPayload, "de", codec), ErrProviderAssessmentNotRecorded)
}

// TestProviderAssessmentRecordCoversOnePayloadKindOnly proves a record for one
// payload kind never allows a different kind for the same language.
func TestProviderAssessmentRecordCoversOnePayloadKindOnly(t *testing.T) {
	codec := passCodec(t)
	pass := assessmentByTicketAndKind(t, "#2781", GreekEntryEvidenceAssistedPassPayload)

	t.Run("the matching kind is allowed", func(t *testing.T) {
		require.NoError(t, ProviderAssessmentSet{pass}.MaySend(GreekEntryEvidenceAssistedPassPayload, "grc", codec))
	})
	t.Run("a different kind is refused", func(t *testing.T) {
		require.ErrorIs(t, ProviderAssessmentSet{pass}.MaySend(FluencyAgreementSourceEvidencePayload, "grc", codec), ErrProviderAssessmentNotRecorded, "a pass record never allows the Fluency agreement")
	})
}

// TestProviderAssessmentRefusesAnUnknownPayloadKind proves a kind with no
// current codec contract fails closed.
func TestProviderAssessmentRefusesAnUnknownPayloadKind(t *testing.T) {
	unknown := ProviderPayloadKind("unknown-payload")
	assessment := ProviderAssessment{Kind: unknown, Language: "grc", Recorded: true}
	require.ErrorIs(t, ProviderAssessmentSet{assessment}.MaySend(unknown, "grc", passCodec(t)), ErrProviderAssessmentNotRecorded)
}

// TestEvidenceAssistedPassUnrecordedAssessmentFailsClosed proves an unrecorded
// assessment allows no pass call.
func TestEvidenceAssistedPassUnrecordedAssessmentFailsClosed(t *testing.T) {
	unrecorded := ProviderAssessments()
	for i := range unrecorded {
		if unrecorded[i].Kind == GreekEntryEvidenceAssistedPassPayload {
			unrecorded[i].Recorded = false
		}
	}
	require.ErrorIs(t, unrecorded.MaySend(GreekEntryEvidenceAssistedPassPayload, "grc", passCodec(t)), ErrProviderAssessmentNotRecorded)
}

// assessmentByTicketAndKind returns the compiled assessment recorded for one
// ticket and payload kind; one ticket can record several kinds.
func assessmentByTicketAndKind(t *testing.T, ticket string, kind ProviderPayloadKind) ProviderAssessment {
	t.Helper()
	for _, assessment := range ProviderAssessments() {
		if assessment.Ticket == ticket && assessment.Kind == kind {
			return assessment
		}
	}
	t.Fatalf("no %s provider assessment recorded for %s", kind, ticket)
	return ProviderAssessment{}
}
