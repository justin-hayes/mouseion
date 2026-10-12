package fixtures

import (
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
)

var contractStatusHeader = regexp.MustCompile(`(?m)^// Contract status: (contractual|illustrative)\.`)

// Every non-test source file must declare whether it is contractual under
// ADR 0088 or illustrative canned state.
func TestSourceFilesDeclareContractStatus(t *testing.T) {
	root, err := os.OpenRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	testutil.Cleanup(t, "source root", root.Close)
	paths, err := fs.Glob(root.FS(), "*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, err := root.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !contractStatusHeader.Match(data) {
			t.Errorf("%s: missing \"// Contract status: contractual.\" or \"// Contract status: illustrative.\" header", path)
		}
	}
}
