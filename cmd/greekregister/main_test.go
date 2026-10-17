package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// fakeReferenceWriter records that the Brownson import branch reached the
// writer.
type fakeReferenceWriter struct {
	called bool
}

func (w *fakeReferenceWriter) WriteReferenceTranslations(context.Context, ancientgreek.ReferenceTranslationWriteRequest) (int, error) {
	w.called = true
	return 0, nil
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

func referenceWriterFake(writer ancientgreek.BrownsonImportWriter, err error) func(registrationStore) (ancientgreek.BrownsonImportWriter, error) {
	return func(registrationStore) (ancientgreek.BrownsonImportWriter, error) {
		return writer, err
	}
}

func TestRunRejectsBadArguments(t *testing.T) {
	for _, args := range [][]string{nil, {}, {"a", "b", "c", "d"}, {""}, {"  "}, {"a", ""}, {"a", "b", " "}} {
		err := run(context.Background(), args, &bytes.Buffer{}, openFake(&fakeRegistrationStore{}, nil), writerFake(&fakeImportWriter{}, nil), referenceWriterFake(&fakeReferenceWriter{}, nil))
		require.ErrorContains(t, err, usage)
	}
}

func TestRunRequiresDatabaseURL(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "")
	err := run(context.Background(), []string{"learner"}, &bytes.Buffer{}, openFake(&fakeRegistrationStore{}, nil), writerFake(&fakeImportWriter{}, nil), referenceWriterFake(&fakeReferenceWriter{}, nil))
	require.ErrorContains(t, err, "MOUSEION_DATABASE_URL is required")
}

func TestRunRegistersAndReports(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "postgres://example")
	store := &fakeRegistrationStore{registration: domain.ReadingUnitBookRegistration{BookID: "book-1", Created: true}}
	var output bytes.Buffer
	require.NoError(t, run(context.Background(), []string{" learner "}, &output, openFake(store, nil), writerFake(&fakeImportWriter{}, nil), referenceWriterFake(&fakeReferenceWriter{}, nil)))
	assert.Equal(t, "ancient greek registration complete: learner=learner book=book-1 created=true\n", output.String())
	assert.True(t, store.closed, "the store is closed")
}

func TestRunRefusesAnUnreadableSourceBeforeWriting(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "postgres://example")
	store := &fakeRegistrationStore{registration: domain.ReadingUnitBookRegistration{BookID: "book-1", Created: true}}
	writer := &fakeImportWriter{}
	source := filepath.Join(t.TempDir(), "source.conllu")
	require.NoError(t, os.WriteFile(source, []byte("not the pinned file"), 0o600))

	err := run(context.Background(), []string{"learner", source}, &bytes.Buffer{}, openFake(store, nil), writerFake(writer, nil), referenceWriterFake(&fakeReferenceWriter{}, nil))
	require.Error(t, err)
	assert.False(t, writer.called, "a refused source never reaches the writer")
}

func TestStoreImportWriterRequiresThePostgresStore(t *testing.T) {
	_, err := storeImportWriter(&fakeRegistrationStore{})
	require.ErrorContains(t, err, "requires the PostgreSQL store")
}

func TestStoreReferenceWriterRequiresThePostgresStore(t *testing.T) {
	_, err := storeReferenceWriter(&fakeRegistrationStore{})
	require.ErrorContains(t, err, "requires the PostgreSQL store")
}

func TestWriteTranslationReportPrintsCountsOnly(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, writeTranslationReport(&output, ancientgreek.BrownsonImportReport{SourcePin: "sha256:abc", Sections: 205, Bytes: 1101217}))
	assert.Equal(t, "ancient greek translation import complete: pin=sha256:abc sections=205 bytes=1101217\n", output.String())
}

// pinnedAnnotationFixture returns the reviewed annotation definition pinned to
// a synthetic testdata file.
func pinnedAnnotationFixture(t *testing.T, name string) (domain.ReadingUnitDefinition, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "ancientgreek", "testdata", name)) //nolint:gosec // the fixture name is a test constant.
	require.NoError(t, err)
	definition := ancientgreek.AnabasisBook1
	definition.SourceFileSize = int64(len(data))
	sum := sha256.Sum256(data)
	definition.SourceFileSHA256 = hex.EncodeToString(sum[:])
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, data, 0o600)) //nolint:gosec // path is inside t.TempDir().
	return definition, path
}

// pinnedTranslationFixture returns the Brownson source pinned to a synthetic
// testdata file.
func pinnedTranslationFixture(t *testing.T, name string) (ancientgreek.BrownsonSource, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "ancientgreek", "testdata", name)) //nolint:gosec // the fixture name is a test constant.
	require.NoError(t, err)
	source := ancientgreek.BrownsonAnabasisBook1
	source.FileSize = int64(len(data))
	sum := sha256.Sum256(data)
	source.FileSHA256 = hex.EncodeToString(sum[:])
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, data, 0o600)) //nolint:gosec // path is inside t.TempDir().
	return source, path
}

func TestImportPinnedSourcesImportsAnnotationAndTranslation(t *testing.T) {
	annotation, annotationPath := pinnedAnnotationFixture(t, "oga-src-deck.conllu")
	translation, translationPath := pinnedTranslationFixture(t, "perseus-eng2-section.xml")
	writer := &fakeImportWriter{}
	reference := &fakeReferenceWriter{}
	var output bytes.Buffer
	err := importPinnedSources(context.Background(), &fakeRegistrationStore{},
		ancientgreek.Report{OwnerID: "owner-1", BookID: "book-1"},
		annotationPath, translationPath,
		pinnedSources{Annotation: annotation, Translation: translation}, &output,
		func(registrationStore) (ancientgreek.ImportWriter, error) { return writer, nil },
		func(registrationStore) (ancientgreek.BrownsonImportWriter, error) { return reference, nil })
	require.NoError(t, err)
	assert.True(t, writer.called, "the annotation reaches the import writer")
	assert.True(t, reference.called, "the Brownson sections reach the reference writer")
	assert.Contains(t, output.String(), "ancient greek translation import complete: pin=sha256:")
}

func TestImportPinnedSourcesSkipsTranslationWithoutAPath(t *testing.T) {
	annotation, annotationPath := pinnedAnnotationFixture(t, "oga-src-deck.conllu")
	writer := &fakeImportWriter{}
	reference := &fakeReferenceWriter{}
	err := importPinnedSources(context.Background(), &fakeRegistrationStore{},
		ancientgreek.Report{OwnerID: "owner-1", BookID: "book-1"},
		annotationPath, "", pinnedSources{Annotation: annotation}, &bytes.Buffer{},
		func(registrationStore) (ancientgreek.ImportWriter, error) { return writer, nil },
		func(registrationStore) (ancientgreek.BrownsonImportWriter, error) { return reference, nil })
	require.NoError(t, err)
	assert.True(t, writer.called)
	assert.False(t, reference.called, "no translation file means no reference writer is built")
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
	require.ErrorIs(t, run(context.Background(), []string{"learner"}, &bytes.Buffer{}, openFake(nil, openErr), writerFake(&fakeImportWriter{}, nil), referenceWriterFake(&fakeReferenceWriter{}, nil)), openErr)
	require.ErrorContains(t, run(context.Background(), []string{"learner"}, &bytes.Buffer{}, openFake(&fakeRegistrationStore{registerErr: errors.New("register failed")}, nil), writerFake(&fakeImportWriter{}, nil), referenceWriterFake(&fakeReferenceWriter{}, nil)), "register failed")
}

func TestRunPropagatesWriterFactoryError(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "postgres://example")
	store := &fakeRegistrationStore{registration: domain.ReadingUnitBookRegistration{BookID: "book-1", Created: true}}
	source := filepath.Join(t.TempDir(), "source.conllu")
	require.NoError(t, os.WriteFile(source, []byte("x"), 0o600))
	factoryErr := errors.New("no writer")
	require.ErrorIs(t, run(context.Background(), []string{"learner", source}, &bytes.Buffer{}, openFake(store, nil), writerFake(nil, factoryErr), referenceWriterFake(&fakeReferenceWriter{}, nil)), factoryErr)
}

func TestRunRefusesAnnotationBeforeBuildingReferenceWriter(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "postgres://example")
	store := &fakeRegistrationStore{registration: domain.ReadingUnitBookRegistration{BookID: "book-1", Created: true}}
	source := filepath.Join(t.TempDir(), "source.conllu")
	brownson := filepath.Join(t.TempDir(), "brownson.xml")
	require.NoError(t, os.WriteFile(source, []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(brownson, []byte("y"), 0o600))
	// The annotation import refuses the unpinned source before the reference
	// writer is built.
	err := run(context.Background(), []string{"learner", source, brownson}, &bytes.Buffer{}, openFake(store, nil), writerFake(&fakeImportWriter{}, nil), referenceWriterFake(nil, errors.New("no reference writer")))
	require.Error(t, err)
	require.NotContains(t, err.Error(), "no reference writer")
}
