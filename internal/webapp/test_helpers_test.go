package webapp

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireHandler(t *testing.T, h http.Handler) *Handler {
	t.Helper()
	result, ok := h.(*Handler)
	require.True(t, ok, "webapp test handler has unexpected type %T", h)
	return result
}
