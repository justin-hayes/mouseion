package persistence

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
)

// These names remain exported for callers outside the web app, but they are
// aliases of the domain sentinels so errors.Is classifies them identically.
func TestDomainSentinelAliases(t *testing.T) {
	cases := []struct {
		name        string
		persistence error
		domain      error
	}{
		{"invalid transition", ErrInvalidTransition, domain.ErrInvalidTransition},
		{"browse counts pending", ErrVocabularyBrowseCountsPending, domain.ErrVocabularyBrowseCountsPending},
		{"browse counts unavailable", ErrVocabularyBrowseCountsUnavailable, domain.ErrVocabularyBrowseCountsUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorIs(t, tc.persistence, tc.domain)
			require.ErrorIs(t, tc.domain, tc.persistence)
		})
	}
}
