package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
)

// A query interrupted before its first row must report the database error,
// not try to decode a Book-status payload that was never returned.
func TestReadVocabularyBrowseRowsInterruptedBeforeSummary(t *testing.T) {
	_, err := readVocabularyBrowseRows(interruptedBrowseRows{err: context.DeadlineExceeded}, domain.VocabularyBrowseQuery{Page: 1})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded), "got %v", err)
}

func TestReadVocabularyBrowseRowsMissingSummary(t *testing.T) {
	_, err := readVocabularyBrowseRows(interruptedBrowseRows{}, domain.VocabularyBrowseQuery{Page: 1})
	require.ErrorContains(t, err, "missing summary row")
}

type interruptedBrowseRows struct {
	pgx.Rows
	err error
}

func (r interruptedBrowseRows) Next() bool { return false }
func (r interruptedBrowseRows) Err() error { return r.err }
