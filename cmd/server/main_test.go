package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestOpenDictionaryIndexMissingFileIsOptional(t *testing.T) {
	index, err := openDictionaryIndex(filepath.Join(t.TempDir(), "absent.sqlite"))

	require.NoError(t, err)
	assert.Nil(t, index)
}

func TestOpenDictionaryIndexRejectsInvalidPath(t *testing.T) {
	index, err := openDictionaryIndex(t.TempDir())

	require.Error(t, err)
	assert.Nil(t, index)
	assert.Contains(t, err.Error(), "directory")
}

func TestOpenDictionaryIndexOpensValidIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	index, err := openDictionaryIndex(path)

	require.NoError(t, err)
	require.NotNil(t, index)
	defer index.Close()
	assert.Equal(t, "fixture-v1", index.Version())
}

func TestRegisterPreparedDeckWorkersRegistersDurableKinds(t *testing.T) {
	workers := river.NewWorkers()
	registerPreparedDeckWorkers(workers, nil, nil, nil, nil, nil, 0, nil)

	for _, test := range []struct {
		name string
		add  func() error
	}{
		{name: "batch submit", add: func() error { return river.AddWorkerSafely(workers, &prepareddeck.BatchSubmitWorker{}) }},
		{name: "batch poll", add: func() error { return river.AddWorkerSafely(workers, &prepareddeck.BatchPollWorker{}) }},
		{name: "finalize", add: func() error { return river.AddWorkerSafely(workers, &prepareddeck.FinalizeWorker{}) }},
		{name: "batch cleanup", add: func() error { return river.AddWorkerSafely(workers, &prepareddeck.BatchCleanupWorker{}) }},
		{name: "recovery", add: func() error { return river.AddWorkerSafely(workers, &prepareddeck.RecoveryWorker{}) }},
		{name: "standard translation", add: func() error { return river.AddWorkerSafely(workers, &prepareddeck.StandardTranslationWorker{}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.add()
			assert.Error(t, err, "worker kind was not registered")
		})
	}
}
