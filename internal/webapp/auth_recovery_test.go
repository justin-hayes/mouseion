package webapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnauthenticatedEnhancedMutationStaysUnauthorized(t *testing.T) {
	h, _, _, _ := goalFixtureSession(t)
	for _, enhanced := range []bool{false, true} {
		name := "ordinary form"
		if enhanced {
			name = "enhanced request"
		}
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/reading/finish", nil)
			r.Header.Set("Accept", "text/html")
			if enhanced {
				r.Header.Set("Hx-Request-Type", "partial")
			}
			w := httptest.NewRecorder()

			h.ServeHTTP(w, r)

			require.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Empty(t, w.Header().Get("Location"))
			assert.Equal(t, "HX-Request-Type", w.Header().Get("Vary"))
			assert.Contains(t, w.Body.String(), "authentication required")
		})
	}
}
