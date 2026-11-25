package canonicalization

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatinSpellingVariantsShareAKey(t *testing.T) {
	profile := Latin()
	key := profile.Canonical("video")
	assert.Equal(t, "uideo", key)
	assert.Equal(t, key, profile.Canonical("uideo"))
	assert.Equal(t, key, profile.Canonical("Video"))
	assert.Equal(t, profile.Canonical("iustus"), profile.Canonical("Justus"))
}

func TestLatinNeverFoldsDigraphs(t *testing.T) {
	profile := Latin()
	assert.NotEqual(t, profile.Canonical("quae"), profile.Canonical("que"))
	assert.NotEqual(t, profile.Canonical("caedo"), profile.Canonical("cedo"))
	assert.NotEqual(t, profile.Canonical("poena"), profile.Canonical("pena"))
	assert.Equal(t, "quae", profile.Canonical("quae"))
}

func TestLatinStripsMacronsAndIsStable(t *testing.T) {
	profile := Latin()
	assert.Equal(t, "amaui", profile.Canonical("amāvī"))
	assert.Equal(t, "amaui", profile.Canonical("amāvī"), "decomposed input")
	assert.Equal(t, "amaui", profile.Canonical("ĂMĀVĪ"))
	key := profile.Canonical("Vīdī")
	assert.Equal(t, key, profile.Canonical(key))
}

func TestLatinIsRegisteredAsADedicatedProfile(t *testing.T) {
	profile, err := For("la")
	require.NoError(t, err)
	assert.Equal(t, "latin", profile.Name())
	assert.Equal(t, "1", profile.Version())
	looked, err := Lookup("la", "1")
	require.NoError(t, err)
	assert.Equal(t, "latin", looked.Name())
	assert.True(t, DedicatedProfileLanguage("la"))
	_, err = NewRegistry().For("la")
	require.ErrorIs(t, err, ErrUnsupportedLanguage)
}

func TestLatinLeavesOtherLanguagesUnchanged(t *testing.T) {
	assert.Equal(t, "ἀνήρ", AncientGreek().Canonical("ἀνήρ"))
	de, err := For("de")
	require.NoError(t, err)
	assert.Equal(t, "vater", de.Canonical("Vater"))
}
