package testwrite

import (
	"net/http"
	"testing"
)

type plainWriter struct{ http.ResponseWriter }

type fakeTestingT struct {
	failed bool
}

func (f *fakeTestingT) Helper() {}

func (f *fakeTestingT) Fatalf(string, ...any) { f.failed = true; panic("fatal") }

func TestFlushRefusesAWriterWithoutFlushing(t *testing.T) {
	fake := &fakeTestingT{}
	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("Flush did not refuse the writer")
			}
		}()
		Flush(fake, plainWriter{})
	}()
	if !fake.failed {
		t.Fatal("Flush accepted a response writer without flushing")
	}
}
