package testwrite

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type testingT interface {
	Helper()
	Fatalf(string, ...any)
}

func String(t testingT, w io.Writer, value string) {
	t.Helper()
	if _, err := io.WriteString(w, value); err != nil {
		t.Fatalf("write test response: %v", err)
	}
}

func Bytes(t testingT, w io.Writer, value []byte) {
	t.Helper()
	if _, err := w.Write(value); err != nil {
		t.Fatalf("write test response: %v", err)
	}
}

func Fprintf(t testingT, w io.Writer, format string, args ...any) {
	t.Helper()
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		t.Fatalf("write test response: %v", err)
	}
}

func JSON(t testingT, w io.Writer, value any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write test response: %v", err)
	}
}

// Flush sends a chunked test response before the handler returns, so a client
// can observe progress (or a stall) while the handler is still running.
func Flush(t testingT, w http.ResponseWriter) {
	t.Helper()
	flusher, ok := w.(http.Flusher)
	if !ok {
		t.Fatalf("response writer does not support flushing")
	}
	flusher.Flush()
}
