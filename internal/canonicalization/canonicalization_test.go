package canonicalization

import (
	"errors"
	"testing"
)

type testProfile struct{ version, suffix string }

func (p testProfile) Name() string                  { return "test-german" }
func (p testProfile) Version() string               { return p.version }
func (p testProfile) Language() string              { return "de" }
func (p testProfile) Canonical(lemma string) string { return Lemma(lemma) + p.suffix }

func TestLemma(t *testing.T) {
	if got := Lemma("  Haus "); got != "haus" {
		t.Fatalf("Lemma() = %q, want %q", got, "haus")
	}
}

func TestGermanPost1996Fixtures(t *testing.T) {
	tests := []struct{ name, raw, want string }{
		{"historical dass", "daß", "dass"}, {"historical muss casing", "  MUẞ  ", "muss"},
		{"historical th", "Thür", "tür"}, {"modern spelling", "Schluss", "schluss"},
		{"diacritic retained", "Grüßen", "grüßen"}, {"modern sharp s retained", "Straße", "straße"},
		{"ambiguous form", "Maße", "maße"}, {"regional form unchanged", "Bub", "bub"},
	}
	profile := GermanPost1996()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := profile.Canonical(tt.raw); got != tt.want {
				t.Fatalf("Canonical(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestGermanPost1996DoesNotCollapseDistinctLexemes(t *testing.T) {
	profile := GermanPost1996()
	for _, pair := range [][2]string{{"Maße", "Masse"}, {"Bus", "Buß"}, {"weisen", "weißen"}} {
		left, right := profile.Canonical(pair[0]), profile.Canonical(pair[1])
		if left == right {
			t.Fatalf("distinct lemmas %q and %q both canonicalized to %q", pair[0], pair[1], left)
		}
	}
}

func TestRegistryActiveAndVersionedLookup(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(GermanPost1996(), true); err != nil {
		t.Fatal(err)
	}
	got, err := registry.For("de-DE")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name() != "german-standard-post-1996" || got.Version() != "2" || got.Language() != "de" {
		t.Fatalf("unexpected profile: %s version %s (%s)", got.Name(), got.Version(), got.Language())
	}
	if versioned, err := registry.Lookup("de_DE", "2"); err != nil || versioned != got {
		t.Fatalf("Lookup() = (%v, %v), want active profile", versioned, err)
	}
}

func TestNormalizePreservesRawLemmaAndRecordsProfile(t *testing.T) {
	got, err := Normalize("de", "  Daß  ")
	if err != nil {
		t.Fatal(err)
	}
	if got.RawLemma != "  Daß  " || got.CanonicalLemma != "dass" ||
		got.ProfileName != "german-standard-post-1996" || got.ProfileVersion != "2" {
		t.Fatalf("Normalize() = %#v", got)
	}
}

func TestActivatingNewVersionDoesNotMutatePriorResult(t *testing.T) {
	registry := NewRegistry()
	v1 := testProfile{version: "1", suffix: "-v1"}
	v2 := testProfile{version: "2", suffix: "-v2"}
	if err := registry.Register(v1, true); err != nil {
		t.Fatal(err)
	}
	persisted := NormalizeWith(v1, "Haus")
	if err := registry.Register(v2, true); err != nil {
		t.Fatal(err)
	}
	active, err := registry.For("de")
	if err != nil {
		t.Fatal(err)
	}
	newItem := NormalizeWith(active, "Haus")
	if persisted.CanonicalLemma != "haus-v1" || persisted.ProfileVersion != "1" {
		t.Fatalf("prior result changed: %#v", persisted)
	}
	if newItem.CanonicalLemma != "haus-v2" || newItem.ProfileVersion != "2" {
		t.Fatalf("new result did not use active version: %#v", newItem)
	}
}

func TestRegistryErrors(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.For("it"); !errors.Is(err, ErrUnsupportedLanguage) {
		t.Fatalf("For() error = %v, want ErrUnsupportedLanguage", err)
	}
	if _, err := registry.Lookup("de", "99"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("Lookup() error = %v, want ErrProfileNotFound", err)
	}
	if err := registry.Register(GermanPost1996(), true); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(GermanPost1996(), false); !errors.Is(err, ErrDuplicateProfile) {
		t.Fatalf("duplicate Register() error = %v, want ErrDuplicateProfile", err)
	}
}
