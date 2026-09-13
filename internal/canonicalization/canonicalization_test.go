package canonicalization

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testProfile struct{ version, suffix string }

func (p testProfile) Name() string                  { return "test-german" }
func (p testProfile) Version() string               { return p.version }
func (p testProfile) Language() string              { return "de" }
func (p testProfile) Canonical(lemma string) string { return Lemma(lemma) + p.suffix }

func TestLemma(t *testing.T) {
	assert.Equal(t, "haus", Lemma("  Haus "))
}

func TestNormalizeLanguageUsesCanonicalBaseForm(t *testing.T) {
	tests := map[string]string{"de_DE": "de", "de-de": "de", "de": "de", "  PT  ": "pt"}
	for raw, want := range tests {
		assert.Equal(t, want, NormalizeLanguage(raw), raw)
	}
}

func TestGermanPost1996Fixtures(t *testing.T) {
	tests := []struct{ name, raw, want string }{
		{"historical dass", "daß", "dass"}, {"historical muss casing", "  MUẞ  ", "muss"},
		{"historical th", "Thür", "tür"}, {"modern spelling", "Schluss", "schluss"},
		{"diacritic retained", "Grüßen", "grüßen"}, {"modern sharp s retained", "Straße", "straße"},
		{"ambiguous form", "Maße", "maße"}, {"regional form unchanged", "Bub", "bub"},
		{"attached closing quote", "Souveränität›", "souveränität"}, {"leading slash", "/oder", "oder"},
		{"internal punctuation", "O'Neill-like", "o'neill-like"},
	}
	profile := GermanPost1996()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, profile.Canonical(tt.raw), tt.raw)
		})
	}
}

func TestGermanPost1996DoesNotCollapseDistinctLexemes(t *testing.T) {
	profile := GermanPost1996()
	for _, pair := range [][2]string{{"Maße", "Masse"}, {"Bus", "Buß"}, {"weisen", "weißen"}} {
		left, right := profile.Canonical(pair[0]), profile.Canonical(pair[1])
		assert.NotEqual(t, left, right, "distinct lemmas %q and %q", pair[0], pair[1])
	}
}

func TestRegistryActiveAndVersionedLookup(t *testing.T) {
	registry := NewRegistry()
	require.NoError(t, registry.Register(GermanPost1996(), true))
	got, err := registry.For("de-DE")
	require.NoError(t, err)
	assert.Equal(t, "german-standard-post-1996", got.Name())
	assert.Equal(t, "5", got.Version())
	assert.Equal(t, "de", got.Language())
	versioned, err := registry.Lookup("de_DE", "5")
	require.NoError(t, err)
	assert.Equal(t, got, versioned)
}

func TestNormalizePreservesRawLemmaAndRecordsProfile(t *testing.T) {
	got, err := Normalize("de", "  Daß  ")
	require.NoError(t, err)
	assert.Equal(t, "  Daß  ", got.RawLemma)
	assert.Equal(t, "dass", got.CanonicalLemma)
	assert.Equal(t, "german-standard-post-1996", got.ProfileName)
	assert.Equal(t, "5", got.ProfileVersion)
}

func TestGermanNormalizationLeavesFullLexemesUnchanged(t *testing.T) {
	for _, raw := range []string{"stehen", "aufstehen", "wiederherstellen"} {
		got, err := Normalize("de", raw)
		require.NoError(t, err)
		assert.Equal(t, raw, got.RawLemma)
		assert.Equal(t, raw, got.CanonicalLemma)
		assert.Equal(t, "german-standard-post-1996", got.ProfileName)
		assert.Equal(t, "5", got.ProfileVersion)
	}
}

func TestGermanV5SelectsFirstUsablePipeLemmaAndRetainsPriorVersions(t *testing.T) {
	got, err := Normalize("de", "  | geleiten | leiten ")
	require.NoError(t, err)
	assert.Equal(t, "  | geleiten | leiten ", got.RawLemma)
	assert.Equal(t, "geleiten", got.CanonicalLemma)
	assert.Equal(t, "5", got.ProfileVersion)
	historical, err := Lookup("de", "2")
	require.NoError(t, err)
	assert.Equal(t, "geleiten|leiten", historical.Canonical("geleiten|leiten"))
	historical, err = Lookup("de", "3")
	require.NoError(t, err)
	assert.Equal(t, "geleiten", historical.Canonical("geleiten|leiten"))
	assert.Equal(t, "souveränität›", historical.Canonical("Souveränität›"))
	historical, err = Lookup("de", "4")
	require.NoError(t, err)
	assert.Equal(t, "souveränität", historical.Canonical("Souveränität›"))
}

func TestActivatingNewVersionDoesNotMutatePriorResult(t *testing.T) {
	registry := NewRegistry()
	v1 := testProfile{version: "1", suffix: "-v1"}
	v2 := testProfile{version: "2", suffix: "-v2"}
	require.NoError(t, registry.Register(v1, true))
	persisted := NormalizeWith(v1, "Haus")
	require.NoError(t, registry.Register(v2, true))
	active, err := registry.For("de")
	require.NoError(t, err)
	newItem := NormalizeWith(active, "Haus")
	assert.Equal(t, "haus-v1", persisted.CanonicalLemma)
	assert.Equal(t, "1", persisted.ProfileVersion)
	assert.Equal(t, "haus-v2", newItem.CanonicalLemma)
	assert.Equal(t, "2", newItem.ProfileVersion)
}

func TestRegistryErrors(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.For("it")
	assert.ErrorIs(t, err, ErrUnsupportedLanguage)
	_, err = registry.Lookup("de", "99")
	assert.ErrorIs(t, err, ErrProfileNotFound)
	require.NoError(t, registry.Register(GermanPost1996(), true))
	assert.ErrorIs(t, registry.Register(GermanPost1996(), false), ErrDuplicateProfile)
}
