package persistence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Vocabulary eligibility is decided in the selection domain, not in SQL (ADR
// 0087). Concordance lists every occurrence, including proper nouns and
// separable particles, so it is the only file allowed to carry these predicates.
var eligibilityPredicates = []string{"'NOUN'", "compound:prt", "[[:alpha:]]"}

var eligibilityPredicateAllowlist = map[string]bool{
	"concordance.go": true,
}

func TestSQLVocabularyEligibilityPredicatesStayOutOfPersistence(t *testing.T) {
	goFiles, err := filepath.Glob("*.go")
	require.NoError(t, err)
	sqlFiles, err := filepath.Glob(filepath.Join("..", "..", "sqlc", "queries", "*.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, sqlFiles)
	for _, path := range append(goFiles, sqlFiles...) {
		if strings.HasSuffix(path, "_test.go") || eligibilityPredicateAllowlist[filepath.Base(path)] {
			continue
		}
		content, err := os.ReadFile(filepath.Clean(path))
		require.NoError(t, err)
		for _, predicate := range eligibilityPredicates {
			if strings.Contains(string(content), predicate) {
				t.Errorf("%s mentions %s; vocabulary eligibility belongs in the selection domain, not SQL (ADR 0087)", path, predicate)
			}
		}
	}
}
