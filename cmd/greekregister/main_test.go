package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRegistrationStore struct {
	registration domain.ReadingUnitBookRegistration
	registerErr  error
	closed       bool
}

func (f *fakeRegistrationStore) GetUserByUsername(context.Context, string) (domain.User, string, error) {
	return domain.User{ID: "owner-1"}, "", nil
}

func (f *fakeRegistrationStore) PutSupportedLanguage(_ context.Context, language, name string) (domain.SupportedLanguage, error) {
	return domain.SupportedLanguage{Language: language, DisplayName: name}, nil
}

func (f *fakeRegistrationStore) ListLegacyLanguageRows(context.Context, string) ([]domain.LegacyLanguageRows, error) {
	return nil, nil
}

func (f *fakeRegistrationStore) RegisterReadingUnitBook(context.Context, string, domain.ReadingUnitDefinition) (domain.ReadingUnitBookRegistration, error) {
	return f.registration, f.registerErr
}

func (f *fakeRegistrationStore) Close() error {
	f.closed = true
	return nil
}

// fakeImportWriter records that the import branch reached the writer.
type fakeImportWriter struct {
	called bool
}

func (w *fakeImportWriter) WriteImportedAnalysis(context.Context, ancientgreek.ImportWriteRequest) (ancientgreek.ImportWriteResult, error) {
	w.called = true
	return ancientgreek.ImportWriteResult{RunID: "run-1"}, nil
}

func openFake(store registrationStore, err error) func(context.Context, string) (registrationStore, error) {
	return func(context.Context, string) (registrationStore, error) {
		return store, err
	}
}

func writerFake(writer ancientgreek.ImportWriter, err error) func(registrationStore) (ancientgreek.ImportWriter, error) {
	return func(registrationStore) (ancientgreek.ImportWriter, error) {
		return writer, err
	}
}

func TestRunRejectsBadArguments(t *testing.T) {
	for _, args := range [][]string{nil, {}, {"a", "b", "c"}, {""}, {"  "}, {"a", ""}} {
		err := run(context.Background(), args, &bytes.Buffer{}, openFake(&fakeRegistrationStore{}, nil), writerFake(&fakeImportWriter{}, nil))
		require.ErrorContains(t, err, usage)
	}
}

func TestRunRequiresDatabaseURL(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "")
	err := run(context.Background(), []string{"learner"}, &bytes.Buffer{}, openFake(&fakeRegistrationStore{}, nil), writerFake(&fakeImportWriter{}, nil))
	require.ErrorContains(t, err, "MOUSEION_DATABASE_URL is required")
}

func TestRunRegistersAndReports(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "postgres://example")
	store := &fakeRegistrationStore{registration: domain.ReadingUnitBookRegistration{BookID: "book-1", Created: true}}
	var output bytes.Buffer
	require.NoError(t, run(context.Background(), []string{" learner "}, &output, openFake(store, nil), writerFake(&fakeImportWriter{}, nil)))
	assert.Equal(t, "ancient greek registration complete: learner=learner book=book-1 created=true\n", output.String())
	assert.True(t, store.closed, "the store is closed")
}

func TestRunRefusesAnUnreadableSourceBeforeWriting(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "postgres://example")
	store := &fakeRegistrationStore{registration: domain.ReadingUnitBookRegistration{BookID: "book-1", Created: true}}
	writer := &fakeImportWriter{}
	source := filepath.Join(t.TempDir(), "source.conllu")
	require.NoError(t, os.WriteFile(source, []byte("not the pinned file"), 0o600))

	err := run(context.Background(), []string{"learner", source}, &bytes.Buffer{}, openFake(store, nil), writerFake(writer, nil))
	require.Error(t, err)
	assert.False(t, writer.called, "a refused source never reaches the writer")
}

func TestStoreImportWriterRequiresThePostgresStore(t *testing.T) {
	_, err := storeImportWriter(&fakeRegistrationStore{})
	require.ErrorContains(t, err, "requires the PostgreSQL store")
}

func TestWriteImportReportPrintsCountsOnly(t *testing.T) {
	var output bytes.Buffer
	report := ancientgreek.ImportReport{
		BookID: "book-1", Sentences: 2, Tokens: 10, EllipsisTokensSkipped: 1,
		ProperNamesDerived: 2, NumberedLemmaTypes: 2, BareLemmasWithNumberedSibling: 1, Reimported: true,
		UnmappedFeatureValues: []ancientgreek.FeatureValueCount{{Feature: "Mood", Value: "n", Count: 316}},
	}
	require.NoError(t, writeImportReport(&output, report))
	assert.Contains(t, output.String(), "ancient greek import complete: book=book-1 sentences=2 tokens=10")
	assert.Contains(t, output.String(), "reimported=true")
	assert.Contains(t, output.String(), "ancient greek import unmapped: feature=Mood value=n count=316")
}

func TestRunPropagatesOpenAndRegistrationErrors(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "postgres://example")
	openErr := errors.New("open failed")
	require.ErrorIs(t, run(context.Background(), []string{"learner"}, &bytes.Buffer{}, openFake(nil, openErr), writerFake(&fakeImportWriter{}, nil)), openErr)
	require.ErrorContains(t, run(context.Background(), []string{"learner"}, &bytes.Buffer{}, openFake(&fakeRegistrationStore{registerErr: errors.New("register failed")}, nil), writerFake(&fakeImportWriter{}, nil)), "register failed")
}

func TestRunPropagatesWriterFactoryError(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "postgres://example")
	store := &fakeRegistrationStore{registration: domain.ReadingUnitBookRegistration{BookID: "book-1", Created: true}}
	source := filepath.Join(t.TempDir(), "source.conllu")
	require.NoError(t, os.WriteFile(source, []byte("x"), 0o600))
	factoryErr := errors.New("no writer")
	require.ErrorIs(t, run(context.Background(), []string{"learner", source}, &bytes.Buffer{}, openFake(store, nil), writerFake(nil, factoryErr)), factoryErr)
}
