package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeIdentityStore struct {
	identities []persistence.AncientGreekVocabularyIdentity
	err        error
	closed     bool
}

func (f *fakeIdentityStore) ListAncientGreekVocabularyIdentities(context.Context) ([]persistence.AncientGreekVocabularyIdentity, error) {
	return f.identities, f.err
}

func (f *fakeIdentityStore) Close() error {
	f.closed = true
	return nil
}

type fakeIndex struct {
	available bool
	entries   map[string]bool
	lookupErr error
}

func (f *fakeIndex) Lookup(_ context.Context, request enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	if f.lookupErr != nil {
		return enrichment.LexicalEntry{}, false, f.lookupErr
	}
	return enrichment.LexicalEntry{}, f.entries[request.CanonicalLemma+"|"+request.UPOS], nil
}

func (f *fakeIndex) AncientGreekAvailable() bool { return f.available }

func storeOpener(store identityStore, err error) identityStoreOpener {
	return func(context.Context, string) (identityStore, error) { return store, err }
}

func indexOpener(index dictionaryIndex, err error) dictionaryIndexOpener {
	return func(context.Context, string) (dictionaryIndex, error) { return index, err }
}

func setEnv(t *testing.T) {
	t.Helper()
	t.Setenv("MOUSEION_DATABASE_URL", "postgres://example")
	t.Setenv("MOUSEION_DICTIONARY_INDEX", "/dictionary.sqlite")
}

func TestRunRejectsArgumentsAndMissingConfiguration(t *testing.T) {
	store := &fakeIdentityStore{}
	index := &fakeIndex{available: true}
	t.Setenv("MOUSEION_DATABASE_URL", "")
	t.Setenv("MOUSEION_DICTIONARY_INDEX", "")
	require.ErrorContains(t, run(context.Background(), []string{"extra"}, &bytes.Buffer{}, storeOpener(store, nil), indexOpener(index, nil)), usage)
	require.ErrorContains(t, run(context.Background(), nil, &bytes.Buffer{}, storeOpener(store, nil), indexOpener(index, nil)), "MOUSEION_DATABASE_URL is required")
	t.Setenv("MOUSEION_DATABASE_URL", "postgres://example")
	require.ErrorContains(t, run(context.Background(), nil, &bytes.Buffer{}, storeOpener(store, nil), indexOpener(index, nil)), "MOUSEION_DICTIONARY_INDEX is required")
}

func TestRunReportsCountsOnly(t *testing.T) {
	setEnv(t)
	store := &fakeIdentityStore{identities: []persistence.AncientGreekVocabularyIdentity{
		{CanonicalLemma: "δέω", UPOS: "VERB", Discriminator: "morpheus:1"},
		{CanonicalLemma: "δέω", UPOS: "VERB", Discriminator: "morpheus:2"},
		{CanonicalLemma: "ἄνθρωπος", UPOS: "NOUN"},
		{CanonicalLemma: "λόγος", UPOS: "NOUN"},
	}}
	index := &fakeIndex{available: true, entries: map[string]bool{"δέω|VERB": true, "ἄνθρωπος|NOUN": true}}
	var output bytes.Buffer

	require.NoError(t, run(context.Background(), nil, &output, storeOpener(store, nil), indexOpener(index, nil)))

	assert.Equal(t, "greek dictionary hit rate: identities=4 with_entry=3 without_entry=1\n"+
		"greek dictionary hit rate by upos: upos=NOUN identities=2 with_entry=1\n"+
		"greek dictionary hit rate by upos: upos=VERB identities=2 with_entry=2\n", output.String())
	assert.True(t, store.closed, "the store is closed")
	assert.NotContains(t, output.String(), "δέω", "the report prints counts only, never Greek text")
}

func TestRunRefusesAnIndexWithoutAncientGreekEntries(t *testing.T) {
	setEnv(t)
	store := &fakeIdentityStore{identities: []persistence.AncientGreekVocabularyIdentity{{CanonicalLemma: "δέω", UPOS: "VERB"}}}
	index := &fakeIndex{available: false}

	err := run(context.Background(), nil, &bytes.Buffer{}, storeOpener(store, nil), indexOpener(index, nil))

	require.ErrorContains(t, err, "no Ancient Greek entries")
}

func TestRunPropagatesOpenStoreIndexAndLookupErrors(t *testing.T) {
	setEnv(t)
	store := &fakeIdentityStore{}
	index := &fakeIndex{available: true}
	openErr := errors.New("open failed")
	require.ErrorIs(t, run(context.Background(), nil, &bytes.Buffer{}, storeOpener(nil, openErr), indexOpener(index, nil)), openErr)
	indexErr := errors.New("index failed")
	require.ErrorIs(t, run(context.Background(), nil, &bytes.Buffer{}, storeOpener(store, nil), indexOpener(nil, indexErr)), indexErr)
	lookupErr := errors.New("lookup failed")
	store = &fakeIdentityStore{identities: []persistence.AncientGreekVocabularyIdentity{{CanonicalLemma: "δέω", UPOS: "VERB"}}}
	require.ErrorIs(t, run(context.Background(), nil, &bytes.Buffer{}, storeOpener(store, nil), indexOpener(&fakeIndex{available: true, lookupErr: lookupErr}, nil)), lookupErr)
}

func TestRunPropagatesIdentityListError(t *testing.T) {
	setEnv(t)
	listErr := errors.New("list failed")

	err := run(context.Background(), nil, &bytes.Buffer{}, storeOpener(&fakeIdentityStore{err: listErr}, nil), indexOpener(&fakeIndex{available: true}, nil))

	require.ErrorIs(t, err, listErr)
}

func TestOpenIndexRejectsAnInvalidPath(t *testing.T) {
	_, err := openIndex(context.Background(), t.TempDir())
	require.Error(t, err)
}

func TestWriteReportPropagatesWriterErrors(t *testing.T) {
	report := hitRateReport{Total: 1, WithEntry: 1, ByUPOS: []uposHitRate{{UPOS: "VERB", Total: 1, WithEntry: 1}}}

	require.Error(t, writeReport(failingWriter{}, report))
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
