// Package canonicalization derives stable, language-specific vocabulary
// identities from raw analyzer lemmas.
package canonicalization

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Profile is a pure, deterministic set of lemma normalization rules.
// Name and Version are persisted with the canonical lemma. Selecting a newer
// active profile affects new items only; migration of stored items is explicit.
type Profile interface {
	Name() string
	Version() string
	Language() string
	Canonical(lemma string) string
}

// NormalizedLemma keeps the analyzer lemma intact and records exactly which
// rules derived its canonical value. Persist these fields together.
type NormalizedLemma struct {
	RawLemma       string
	CanonicalLemma string
	ProfileName    string
	ProfileVersion string
}

var (
	ErrUnsupportedLanguage = errors.New("canonicalization: unsupported language")
	ErrProfileNotFound     = errors.New("canonicalization: profile not found")
	ErrDuplicateProfile    = errors.New("canonicalization: duplicate profile")
)

type profileKey struct{ language, version string }

// Registry stores immutable profiles and active versions. It is concurrency-safe.
type Registry struct {
	mu       sync.RWMutex
	profiles map[profileKey]Profile
	active   map[string]string
}

func NewRegistry() *Registry {
	return &Registry{profiles: make(map[profileKey]Profile), active: make(map[string]string)}
}

// Register adds a profile. If active, future normalization uses this version;
// older versions remain addressable through Lookup for reproducibility.
func (r *Registry) Register(profile Profile, active bool) error {
	if profile == nil || strings.TrimSpace(profile.Name()) == "" || strings.TrimSpace(profile.Version()) == "" {
		return fmt.Errorf("canonicalization: profile name and version are required")
	}
	language := normalizeLanguage(profile.Language())
	if language == "" {
		return fmt.Errorf("canonicalization: profile language is required")
	}
	key := profileKey{language, profile.Version()}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.profiles[key]; exists {
		return fmt.Errorf("%w: %s version %s", ErrDuplicateProfile, language, profile.Version())
	}
	r.profiles[key] = profile
	if active {
		r.active[language] = profile.Version()
	}
	return nil
}

// For returns the active profile. Locales first use an exact registration and
// then fall back to their base language (for example, de-DE to de).
func (r *Registry) For(language string) (Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, candidate := range languageCandidates(language) {
		if version, ok := r.active[candidate]; ok {
			return r.profiles[profileKey{candidate, version}], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrUnsupportedLanguage, language)
}

// Lookup returns a specifically versioned profile.
func (r *Registry) Lookup(language, version string) (Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, candidate := range languageCandidates(language) {
		if profile, ok := r.profiles[profileKey{candidate, version}]; ok {
			return profile, nil
		}
	}
	return nil, fmt.Errorf("%w: %s version %s", ErrProfileNotFound, language, version)
}

var defaultRegistry = func() *Registry {
	r := NewRegistry()
	if err := r.Register(GermanPost1996Profile{version: "2"}, false); err != nil {
		panic(err)
	}
	if err := r.Register(GermanPost1996Profile{version: "3"}, false); err != nil {
		panic(err)
	}
	if err := r.Register(GermanPost1996(), true); err != nil {
		panic(err)
	}
	return r
}()

func For(language string) (Profile, error) { return defaultRegistry.For(language) }
func Lookup(language, version string) (Profile, error) {
	return defaultRegistry.Lookup(language, version)
}
func Register(profile Profile, active bool) error { return defaultRegistry.Register(profile, active) }

// Normalize derives a canonical lemma using the active language profile while
// preserving the raw analyzer value and applied profile metadata.
func Normalize(language, rawLemma string) (NormalizedLemma, error) {
	profile, err := For(language)
	if err != nil {
		return NormalizedLemma{}, err
	}
	return NormalizeWith(profile, rawLemma), nil
}

// NormalizeWith applies an explicitly selected profile. It is useful for
// reproducible imports and explicit re-normalization migrations.
func NormalizeWith(profile Profile, rawLemma string) NormalizedLemma {
	return NormalizedLemma{
		RawLemma:       rawLemma,
		CanonicalLemma: profile.Canonical(rawLemma),
		ProfileName:    profile.Name(),
		ProfileVersion: profile.Version(),
	}
}

// Lemma applies the baseline language-neutral normalization. It remains for
// backward compatibility; language-aware callers should select a Profile.
func Lemma(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func normalizeLanguage(language string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
}

func languageCandidates(language string) []string {
	language = normalizeLanguage(language)
	if language == "" {
		return nil
	}
	if base, _, found := strings.Cut(language, "-"); found {
		return []string{language, base}
	}
	return []string{language}
}
