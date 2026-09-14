package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestHealthcheckURL(t *testing.T) {
	for _, test := range []struct {
		name    string
		addr    string
		want    string
		wantErr bool
	}{
		{name: "empty defaults to loopback 8080", addr: "", want: "http://127.0.0.1:8080/healthz"},
		{name: "wildcard host", addr: "0.0.0.0:9090", want: "http://127.0.0.1:9090/healthz"},
		{name: "explicit host", addr: "127.0.0.1:1234", want: "http://127.0.0.1:1234/healthz"},
		{name: "missing port", addr: "notaport", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := healthcheckURL(test.addr)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestProbeHealth(t *testing.T) {
	client := &http.Client{Timeout: time.Second}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintln(w, "ok")
	}))
	defer server.Close()
	require.NoError(t, probeHealth(server.URL+"/healthz", client))

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer failing.Close()
	require.Error(t, probeHealth(failing.URL+"/healthz", client))

	require.Error(t, probeHealth("http://127.0.0.1:1/healthz", client))
}

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
