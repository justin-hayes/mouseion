package canonicalization

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"
)

// ancientGreekFixture is one reviewed ancient-greek-normalization/1 row. Strings
// are code-point arrays so no editor or terminal can normalize them.
type ancientGreekFixture struct {
	Format           string                   `json:"format"`
	Case             string                   `json:"case"`
	Profile          string                   `json:"profile"`
	Unicode          string                   `json:"unicode"`
	Input            []string                 `json:"input"`
	Expect           ancientGreekExpect       `json:"expect"`
	EqualToCase      string                   `json:"equal_to_case"`
	DistinctFromCase string                   `json:"distinct_from_case"`
	Call             *ancientGreekFixtureCall `json:"call"`
	Note             string                   `json:"note"`
}

type ancientGreekExpect struct {
	Key      []string `json:"key"`
	Profile  string   `json:"profile"`
	Rejected string   `json:"rejected"`
}

type ancientGreekFixtureCall struct {
	Fn       string `json:"fn"`
	Language string `json:"language"`
	Version  string `json:"version"`
}

func TestAncientGreekNormalizationFixture(t *testing.T) {
	fixtures := loadAncientGreekFixtures(t)
	profile := AncientGreek()
	outcomes := make(map[string]string, len(fixtures))
	for _, fixture := range fixtures {
		outcomes[fixture.Case] = exerciseAncientGreekFixture(t, profile, fixture)
	}
	for _, fixture := range fixtures {
		if fixture.EqualToCase != "" {
			assert.Equal(t, outcomes[fixture.EqualToCase], outcomes[fixture.Case], "case %q must equal %q", fixture.Case, fixture.EqualToCase)
		}
		if fixture.DistinctFromCase != "" {
			assert.NotEqual(t, outcomes[fixture.DistinctFromCase], outcomes[fixture.Case], "case %q must differ from %q", fixture.Case, fixture.DistinctFromCase)
		}
	}
}

func TestAncientGreekUnicodePin(t *testing.T) {
	assert.Equal(t, "17.0.0", AncientGreekUnicodeVersion)
	assert.Equal(t, AncientGreekUnicodeVersion, unicode.Version, "Go case-mapping tables")
	assert.Equal(t, AncientGreekUnicodeVersion, norm.Version, "normalization tables")
}

func TestAncientGreekNeverCasefolds(t *testing.T) {
	ancient, neutral := AncientGreek(), LanguageNeutral()
	for _, name := range []string{"iota subscript", "capital ypogegrammeni", "capital I with dot"} {
		var lemma string
		switch name {
		case "iota subscript":
			lemma = "\u1FC4"
		case "capital ypogegrammeni":
			lemma = "\u1FBC"
		default:
			lemma = "\u0130"
		}
		assert.NotEqual(t, neutral.Canonical(lemma), ancient.Canonical(lemma), name)
	}
}

func TestGrcAndElResolveToDifferentProfiles(t *testing.T) {
	grc, err := For("grc")
	require.NoError(t, err)
	el, err := For("el")
	require.NoError(t, err)
	assert.Equal(t, AncientGreekProfileName, grc.Name())
	assert.Equal(t, "grc", grc.Language())
	assert.Equal(t, ModernGreekProfileName, el.Name())
	assert.NotEqual(t, grc.Name()+"/"+grc.Version(), el.Name()+"/"+el.Version())
}

func TestDedicatedProfileLanguageNeverFallsBack(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.For("grc")
	require.ErrorIs(t, err, ErrUnsupportedLanguage, "an unregistered dedicated language is unsupported")
	_, err = registry.Lookup("grc", UnicodeCasefoldVersion)
	require.ErrorIs(t, err, ErrProfileNotFound, "version lookup for grc never returns the fallback")
	fallback, err := registry.For("it")
	require.NoError(t, err)
	assert.Equal(t, UnicodeCasefoldProfile, fallback.Name(), "other unregistered languages keep the fallback")

	require.NoError(t, registry.Register(AncientGreek(), true))
	grc, err := registry.For("grc")
	require.NoError(t, err)
	assert.Equal(t, AncientGreekProfileName, grc.Name())
	assert.Equal(t, AncientGreekProfileVersion, grc.Version())
}

// exerciseAncientGreekFixture applies one fixture row and returns the observable
// outcome (a key, a resolved profile name/version, or a rejection) for relation
// checks.
func exerciseAncientGreekFixture(t *testing.T, profile Profile, fixture ancientGreekFixture) string {
	t.Helper()
	assert.Equal(t, "ancient-greek-normalization/1", fixture.Format, "case %q format", fixture.Case)
	if fixture.Call != nil {
		name, rejected := resolveAncientGreekCall(t, *fixture.Call)
		if fixture.Expect.Rejected != "" {
			assert.Equal(t, fixture.Expect.Rejected, rejected, "case %q rejection", fixture.Case)
			assert.Empty(t, name, "case %q profile", fixture.Case)
			return "rejected:" + rejected
		}
		assert.Empty(t, rejected, "case %q rejection", fixture.Case)
		assert.Equal(t, fixture.Expect.Profile, name, "case %q profile", fixture.Case)
		return name
	}
	require.Equal(t, profile.Name()+"/"+profile.Version(), fixture.Profile, "case %q profile", fixture.Case)
	key := profile.Canonical(decodeAncientGreekCodePoints(t, fixture.Input))
	assert.Equal(t, fixture.Expect.Key, encodeAncientGreekCodePoints(key), "case %q key", fixture.Case)
	assert.Equal(t, key, profile.Canonical(key), "case %q is idempotent", fixture.Case)
	return key
}

func resolveAncientGreekCall(t *testing.T, call ancientGreekFixtureCall) (name, rejected string) {
	t.Helper()
	var profile Profile
	var err error
	switch call.Fn {
	case "For":
		profile, err = For(call.Language)
	case "Lookup":
		profile, err = Lookup(call.Language, call.Version)
	default:
		require.FailNow(t, "unknown fixture call", call.Fn)
	}
	return ancientGreekOutcome(t, profile, err)
}

func ancientGreekOutcome(t *testing.T, profile Profile, err error) (name, rejected string) {
	t.Helper()
	if err == nil {
		require.NotNil(t, profile)
		return profile.Name() + "/" + profile.Version(), ""
	}
	switch {
	case errors.Is(err, ErrUnsupportedLanguage):
		rejected = "unsupported_language"
	case errors.Is(err, ErrProfileNotFound):
		rejected = "profile_not_found"
	default:
		require.FailNow(t, "unexpected resolution error", err)
	}
	return "", rejected
}

func loadAncientGreekFixtures(t *testing.T) []ancientGreekFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/ancient-greek-normalization-v1.jsonl")
	require.NoError(t, err)
	var fixtures []ancientGreekFixture
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var fixture ancientGreekFixture
		require.NoError(t, json.Unmarshal([]byte(line), &fixture), line)
		fixtures = append(fixtures, fixture)
	}
	require.NotEmpty(t, fixtures)
	return fixtures
}

func decodeAncientGreekCodePoints(t *testing.T, points []string) string {
	t.Helper()
	var builder strings.Builder
	for _, point := range points {
		value, err := strconv.ParseInt(strings.TrimPrefix(point, "U+"), 16, 32)
		require.NoError(t, err, point)
		builder.WriteRune(rune(value))
	}
	return builder.String()
}

func encodeAncientGreekCodePoints(value string) []string {
	points := make([]string, 0, len(value))
	for _, r := range value {
		points = append(points, fmt.Sprintf("U+%04X", r))
	}
	return points
}
