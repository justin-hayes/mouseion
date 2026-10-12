package deploysecret

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSecret(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		want   error
	}{
		{name: "empty is required", secret: "", want: ErrSecretRequired},
		{name: "whitespace only is required", secret: " \t\n ", want: ErrSecretRequired},
		{name: "31 bytes is weak", secret: strings.Repeat("a", 31), want: ErrSecretWeak},
		{name: "32 bytes is accepted", secret: strings.Repeat("a", 32)},
		// Byte length is what counts, not rune count: 16 two-byte runes are 32
		// bytes, while 15 runes plus one ASCII byte reaches only 31.
		{name: "16 multi-byte runes are 32 bytes", secret: strings.Repeat("é", 16)},
		{name: "multi-byte secret under 32 bytes is weak", secret: strings.Repeat("é", 15) + "a", want: ErrSecretWeak},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecret(tt.secret)
			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.want)
		})
	}
}
