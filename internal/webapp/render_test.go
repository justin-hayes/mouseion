package webapp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type recordingComponent struct {
	renders int
}

func (c *recordingComponent) Render(context.Context, io.Writer) error {
	c.renders++
	return nil
}

type failingComponent struct {
	renders int
}

func (c *failingComponent) Render(_ context.Context, w io.Writer) error {
	c.renders++
	if _, err := io.WriteString(w, "partial page"); err != nil {
		return err
	}
	return errors.New("render failed")
}

type countingResponseWriter struct {
	header      http.Header
	statusCalls int
	status      int
	body        []byte
}

func (w *countingResponseWriter) Header() http.Header { return w.header }

func (w *countingResponseWriter) WriteHeader(status int) {
	w.statusCalls++
	if w.status == 0 {
		w.status = status
	}
}

func (w *countingResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.body = append(w.body, body...)
	return len(body), nil
}

func TestRenderStopsAfterCSRFEstablishmentFailure(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), csrfFailureContextKey{}, errors.New("random source failed")))
	w := httptest.NewRecorder()
	component := &recordingComponent{}

	render(w, r, component)

	if component.renders != 0 {
		t.Fatalf("component rendered %d times after CSRF establishment failed", component.renders)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want untouched response status %d", w.Code, http.StatusOK)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty response body", w.Body.String())
	}
}

func TestRenderStatusStopsAfterCSRFEstablishmentFailure(t *testing.T) {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), csrfFailureContextKey{}, errors.New("random source failed")))
	w := &countingResponseWriter{header: make(http.Header)}
	w.WriteHeader(http.StatusInternalServerError)
	component := &recordingComponent{}

	renderStatus(w, r, http.StatusConflict, component)

	if component.renders != 0 {
		t.Fatalf("component rendered %d times after CSRF establishment failed", component.renders)
	}
	if w.statusCalls != 1 {
		t.Fatalf("WriteHeader called %d times, want 1", w.statusCalls)
	}
	if w.status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.status, http.StatusInternalServerError)
	}
}

func TestRenderAndRenderStatusPreserveSuccessfulResponses(t *testing.T) {
	tests := []struct {
		name   string
		render func(http.ResponseWriter, *http.Request, *recordingComponent)
		status int
	}{
		{name: "normal", render: func(w http.ResponseWriter, r *http.Request, c *recordingComponent) {
			render(w, r, c)
		}, status: http.StatusOK},
		{name: "explicit status", render: func(w http.ResponseWriter, r *http.Request, c *recordingComponent) {
			renderStatus(w, r, http.StatusAccepted, c)
		}, status: http.StatusAccepted},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			component := &recordingComponent{}

			test.render(w, r, component)

			if w.Code != test.status {
				t.Fatalf("status = %d, want %d", w.Code, test.status)
			}
			if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
				t.Fatalf("Content-Type = %q, want HTML content type", got)
			}
			if component.renders != 1 {
				t.Fatalf("component rendered %d times, want 1", component.renders)
			}
		})
	}
}

func TestRenderAndRenderStatusDiscardPartialContentOnFailure(t *testing.T) {
	tests := []struct {
		name   string
		render func(http.ResponseWriter, *http.Request, *failingComponent)
	}{
		{name: "normal", render: func(w http.ResponseWriter, r *http.Request, c *failingComponent) {
			render(w, r, c)
		}},
		{name: "explicit status", render: func(w http.ResponseWriter, r *http.Request, c *failingComponent) {
			renderStatus(w, r, http.StatusAccepted, c)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			component := &failingComponent{}

			test.render(w, r, component)

			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
			}
			if got := w.Body.String(); got != "unable to render page\n" {
				t.Fatalf("body = %q, want clean server error", got)
			}
			if component.renders != 1 {
				t.Fatalf("component rendered %d times, want 1", component.renders)
			}
		})
	}
}
