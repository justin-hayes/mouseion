package enrichment

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	greekCitationFormVersion = "translation-v20-evidence-assisted-greek-entries-citation-form"
	latinCitationFormVersion = "translation-v21-evidence-assisted-latin-citation-form"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// citationFormGlossRules are the Gloss rules of the modern contextual-Gloss
// contract that the Greek stacked-homograph and Latin pass contracts adopt
// (#2779): the lemma's meaning in citation form, never the sentence's
// inflection, number or arguments, one to three words.
var citationFormGlossRules = []string{
	"Gloss is the English equivalent of the supplied lemma in the sense the representative sentence uses, in citation form: verbs as \"to …\", nouns singular without an article, adjectives in base form.",
	`never carry the sentence's inflection, number or arguments`,
	`Keep the Gloss to one to three words; close comma-separated synonyms are allowed only when one word would mislead.`,
}

// citationFormAlignmentRules are the tightened alignment rule: the highlight
// covers only the English words that render the target itself.
var citationFormAlignmentRules = []string{
	`only the English words that render the target itself, never its subject, object, complements or modifiers`,
	`Where the English renders the target with a function word`,
	`only that word`,
}

func TestCitationFormPassPromptVersionsArePinned(t *testing.T) {
	assert.Equal(t, greekCitationFormVersion, greekEntryEvidenceAssistedPassPromptVersion)
	assert.Equal(t, latinCitationFormVersion, latinEvidenceAssistedPassPromptVersion)
	codec := passCodec(t)
	assert.Equal(t, greekCitationFormVersion, codec.GreekEntryEvidenceAssistedPassPromptContract())
	assert.Equal(t, latinCitationFormVersion, codec.LatinEvidenceAssistedPassPromptContract())
	assert.Equal(t, "gpt-test/"+greekCitationFormVersion, codec.GreekEntryEvidenceAssistedPassProviderVersion())
	assert.Equal(t, "gpt-test/"+latinCitationFormVersion, codec.LatinEvidenceAssistedPassProviderVersion())
}

func TestCitationFormPassPromptsStateTheGlossAndAlignmentRules(t *testing.T) {
	prompts := map[string]string{
		"Greek": greekEntryEvidenceAssistedPassSystemPrompt,
		"Latin": latinEvidenceAssistedPassSystemPrompt,
	}
	for name, prompt := range prompts {
		t.Run(name, func(t *testing.T) {
			for _, rule := range append(append([]string{}, citationFormGlossRules...), citationFormAlignmentRules...) {
				assert.Contains(t, prompt, rule)
			}
			assert.NotContains(t, prompt, "gloss must be one brief English cue", "the old Gloss rule is replaced")
			assert.NotContains(t, prompt, "identify only the smallest meaning-bearing words or phrases corresponding to the supplied target", "the old alignment rule is replaced")
		})
	}
	assert.Contains(t, greekEntryEvidenceAssistedPassSystemPrompt, "ἔχων", "Greek's example names a Greek function-word rendering")
}

// TestCitationFormPassFieldSetsAreUnchanged proves the new versions change the
// prompt only, never the payload.
func TestCitationFormPassFieldSetsAreUnchanged(t *testing.T) {
	want := []string{"item_id", "language", "target_language", "canonical_lemma", "upos", "target_word", "example_sentence", "passage_context", "candidate_senses"}
	assert.Equal(t, want, GreekEntryEvidenceAssistedPassPayloadFields())
	assert.Equal(t, want, LatinEvidenceAssistedPassPayloadFields())
}

// TestCitationFormPassContractsAreRecordedExactly proves the compiled
// assessments (#2781) allow exactly the new prompt versions for Greek and Latin
// and refuse the superseded ones.
func TestCitationFormPassContractsAreRecordedExactly(t *testing.T) {
	codec := passCodec(t)
	require.NoError(t, ProviderAssessments().MaySend(GreekEntryEvidenceAssistedPassPayload, "grc", codec))
	require.NoError(t, ProviderAssessments().MaySend(LatinEvidenceAssistedPassPayload, "la", codec))
	assert.Equal(t, greekCitationFormVersion, greekEntryEvidenceAssistedPassAssessment.PromptVersion)
	assert.Equal(t, latinCitationFormVersion, latinEvidenceAssistedPassAssessment.PromptVersion)
	assert.Equal(t, "#2781", greekEntryEvidenceAssistedPassAssessment.Ticket)
	assert.Equal(t, "#2781", latinEvidenceAssistedPassAssessment.Ticket)

	t.Run("the superseded versions are refused", func(t *testing.T) {
		greek, latin := greekEntryEvidenceAssistedPassAssessment, latinEvidenceAssistedPassAssessment
		greek.PromptVersion, latin.PromptVersion = "translation-v19-evidence-assisted-greek-entries", "translation-v18-evidence-assisted-latin"
		require.ErrorIs(t, ProviderAssessmentSet{greek}.MaySend(GreekEntryEvidenceAssistedPassPayload, "grc", codec), ErrProviderAssessmentNotRecorded)
		require.ErrorIs(t, ProviderAssessmentSet{latin}.MaySend(LatinEvidenceAssistedPassPayload, "la", codec), ErrProviderAssessmentNotRecorded)
	})
	t.Run("the records answer only for their own language", func(t *testing.T) {
		require.ErrorIs(t, ProviderAssessments().MaySend(GreekEntryEvidenceAssistedPassPayload, "la", codec), ErrProviderAssessmentNotRecorded)
		require.ErrorIs(t, ProviderAssessments().MaySend(LatinEvidenceAssistedPassPayload, "grc", codec), ErrProviderAssessmentNotRecorded)
	})
}

// TestCitationFormChangeLeavesOtherContractsByteForByte pins the contracts the
// change must not touch by the SHA-256 of their system prompts and their
// prompt versions: the modern contextual-Gloss contract, German's Fluency
// agreement contract (and the source-evidence variant) and the #2171 Greek pass
// contract.
func TestCitationFormChangeLeavesOtherContractsByteForByte(t *testing.T) {
	tests := []struct {
		name, prompt, hash string
	}{
		{"modern contextual Gloss", contextualGlossSystemPrompt, "8da909d12ca1801c8cf69d3433e2833e9143c46db4082a755c93e82f6f022dfe"},
		{"Greek deck pass (#2171)", evidenceAssistedPassSystemPrompt, "655c1c9e2cbbd9eccc5b019baf2fd7d24f892e3382f1bef8b96c45036642b915"},
		{"Fluency agreement", fluencyAgreementSystemPrompt, "250b76d75a294df28c6c612b7519414527676d975f5c27cdb8d2b35e49be3560"},
		{"Fluency agreement, source evidence", fluencyAgreementSourceEvidenceSystemPrompt, "27979bd063f2d59e78557fb160203a5fdcba836dcf5f7371a1a7cbee94619825"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.hash, sha256Hex(tc.prompt))
		})
	}
	assert.Equal(t, "translation-v17-clause-lemma-mismatch", contextualGlossPromptVersion)
	assert.Equal(t, "translation-v16-evidence-assisted-greek", evidenceAssistedPassPromptVersion)
	assert.Equal(t, "fluency-agreement-v2", fluencyAgreementPromptVersion)
	assert.Equal(t, "fluency-agreement-source-evidence-v2", fluencyAgreementSourceEvidencePromptVersion)
}

// TestCheckPassAssessmentAllowsFrozenWorkUnderTheRecordedContracts proves the
// last check before a pass is encoded allows Latin and Greek work under the
// recorded contracts, and that the client no longer refuses on the assessment.
func TestCheckPassAssessmentAllowsFrozenWorkUnderTheRecordedContracts(t *testing.T) {
	codec := passCodec(t)
	latin := latinPassRequest()
	latin.RequireEvidenceAssistedPass = true
	greekStudy := greekEntryPassRequest()
	greekStudy.RequireEvidenceAssistedPass = true
	greekDeck := greekEntryPassRequest()
	greekDeck.RequireEvidenceAssistedPass, greekDeck.StudyPass = true, false

	require.NoError(t, CheckPassAssessment(codec, latin))
	require.NoError(t, CheckPassAssessment(codec, greekStudy))
	require.NoError(t, CheckPassAssessment(codec, greekDeck))
	require.NoError(t, CheckPassAssessment(codec, TranslationRequest{Language: "la"}), "a request that is not a pass is never refused")
}
