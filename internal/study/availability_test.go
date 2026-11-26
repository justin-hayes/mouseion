package study

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAvailableOnlyOnTheModernLanguageRoute(t *testing.T) {
	cases := []struct {
		language string
		want     bool
	}{
		{"de", true},
		{"it", true},
		{"el", true},
		{"la", false},
		{"grc", false},
	}
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			require.Equal(t, tc.want, Available(tc.language))
		})
	}
}
