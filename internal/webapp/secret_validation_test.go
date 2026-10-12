package webapp

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/justin-hayes/mouseion/internal/deploysecret"
)

func TestNewWithErrorRejectsMissingSecret(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "")
	_, err := NewWithError(Services{})
	require.ErrorIs(t, err, deploysecret.ErrSecretRequired)
}

func TestNewWithErrorRejectsWeakSecret(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "too-short")
	_, err := NewWithError(Services{})
	require.ErrorIs(t, err, deploysecret.ErrSecretWeak)
}
