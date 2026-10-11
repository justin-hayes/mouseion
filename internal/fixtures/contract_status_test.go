package fixtures

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var contractStatusHeader = regexp.MustCompile(`(?m)^// Contract status: (contractual|illustrative)\.`)

// Every non-test source file must declare whether it is contractual under
// ADR 0088 or illustrative canned state.
func TestSourceFilesDeclareContractStatus(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path) //nolint:gosec // path is a source file from this package's directory glob.
		if err != nil {
			t.Fatal(err)
		}
		if !contractStatusHeader.Match(data) {
			t.Errorf("%s: missing \"// Contract status: contractual.\" or \"// Contract status: illustrative.\" header", path)
		}
	}
}
