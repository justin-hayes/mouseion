package testwrite

import (
	"encoding/json"
	"fmt"
	"io"
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
