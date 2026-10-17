package canonicalization

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrenchProfileKeepsAccentsSignificant(t *testing.T) {
	profile := French()
	assert.NotEqual(t, profile.Canonical("où"), profile.Canonical("ou"), "où and ou stay distinct")
	assert.NotEqual(t, profile.Canonical("sûr"), profile.Canonical("sur"), "sûr and sur stay distinct")
	assert.Equal(t, profile.Canonical("aujourd\u2019hui"), profile.Canonical("aujourd'hui"), "both apostrophes share one identity")
	assert.Equal(t, profile.Canonical("où"), profile.Canonical(profile.Canonical("où")), "canonical result is idempotent")
}

func TestFrenchProfileIdentity(t *testing.T) {
	profile, err := For("fr")
	require.NoError(t, err)
	assert.Equal(t, FrenchProfileName, profile.Name())
	assert.Equal(t, FrenchProfileVersion, profile.Version())
	assert.Equal(t, frenchLanguage, profile.Language())

	// A French locale resolves to the French profile through its base language,
	// and the versioned lookup is addressable for reproducible normalization.
	locale, err := For("fr-FR")
	require.NoError(t, err)
	assert.Equal(t, FrenchProfileName, locale.Name())
	versioned, err := Lookup("fr", FrenchProfileVersion)
	require.NoError(t, err)
	assert.Equal(t, FrenchProfileName, versioned.Name())
}

func TestFrenchProfileNormalizePreservesRawLemma(t *testing.T) {
	normalized, err := Normalize("fr", "aujourd\u2019hui")
	require.NoError(t, err)
	assert.Equal(t, "aujourd'hui", normalized.CanonicalLemma)
	assert.Equal(t, "aujourd\u2019hui", normalized.RawLemma, "the raw lemma is untouched")
	assert.Equal(t, FrenchProfileName, normalized.ProfileName)
	assert.Equal(t, FrenchProfileVersion, normalized.ProfileVersion)
}
