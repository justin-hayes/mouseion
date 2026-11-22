package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	report    domain.MyBooksBackfillReport
	err       error
	closeErr  error
	closeCall bool
}

func (f *fakeStore) BackfillChosenBooks(context.Context) (domain.MyBooksBackfillReport, error) {
	return f.report, f.err
}

func (f *fakeStore) Close() error {
	f.closeCall = true
	return f.closeErr
}

func TestRunRequiresADatabaseURL(t *testing.T) {
	err := run(context.Background(), func(string) string { return "" }, &bytes.Buffer{}, nil)
	require.EqualError(t, err, "MOUSEION_DATABASE_URL is required")
}

func TestRunReportsTheBackfillAndClosesTheStore(t *testing.T) {
	store := &fakeStore{report: domain.MyBooksBackfillReport{
		Removed: 2,
		LanguageLessWithHistory: []domain.Book{
			{ID: "book-1", Title: "Language-less"},
		},
	}}
	open := func(context.Context, string) (backfillStore, error) { return store, nil }
	var output bytes.Buffer
	require.NoError(t, run(context.Background(), func(string) string { return "postgres://example" }, &output, open))
	assert.Contains(t, output.String(), "removed 2 unchosen Books")
	assert.Contains(t, output.String(), "removed book-1")
	assert.Contains(t, output.String(), "reading history is kept")
	assert.True(t, store.closeCall)
}

func TestRunRefusesBeforeEveryCatalogSync(t *testing.T) {
	store := &fakeStore{err: persistence.ErrMyBooksBackfillBlocked}
	open := func(context.Context, string) (backfillStore, error) { return store, nil }
	err := run(context.Background(), func(string) string { return "postgres://example" }, &bytes.Buffer{}, open)
	require.EqualError(t, err, "refusing to run: every catalog connection must complete a sync first")
}

func TestRunPropagatesOpenAndBackfillFailures(t *testing.T) {
	openErr := errors.New("open failed")
	err := run(context.Background(), func(string) string { return "postgres://example" }, &bytes.Buffer{}, func(context.Context, string) (backfillStore, error) {
		return nil, openErr
	})
	require.ErrorIs(t, err, openErr)

	store := &fakeStore{err: errors.New("write failed")}
	err = run(context.Background(), func(string) string { return "postgres://example" }, &bytes.Buffer{}, func(context.Context, string) (backfillStore, error) {
		return store, nil
	})
	require.ErrorContains(t, err, "backfill chosen books: write failed")
}

func TestRunReportsAWriteFailure(t *testing.T) {
	store := &fakeStore{report: domain.MyBooksBackfillReport{Removed: 1}}
	open := func(context.Context, string) (backfillStore, error) { return store, nil }
	err := run(context.Background(), func(string) string { return "postgres://example" }, failingWriter{}, open)
	require.Error(t, err)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
