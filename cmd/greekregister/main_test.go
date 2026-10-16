package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

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

func openFake(store registrationStore, err error) func(context.Context, string) (registrationStore, error) {
	return func(context.Context, string) (registrationStore, error) {
		return store, err
	}
}

func TestRunRequiresExactlyOneLearnerUsername(t *testing.T) {
	for _, args := range [][]string{nil, {}, {"a", "b"}, {""}, {"  "}} {
		err := run(context.Background(), args, &bytes.Buffer{}, openFake(&fakeRegistrationStore{}, nil))
		require.ErrorContains(t, err, usage)
	}
}

func TestRunRequiresDatabaseURL(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "")
	err := run(context.Background(), []string{"learner"}, &bytes.Buffer{}, openFake(&fakeRegistrationStore{}, nil))
	require.ErrorContains(t, err, "MOUSEION_DATABASE_URL is required")
}

func TestRunRegistersAndReports(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "postgres://example")
	store := &fakeRegistrationStore{registration: domain.ReadingUnitBookRegistration{BookID: "book-1", Created: true}}
	var output bytes.Buffer
	require.NoError(t, run(context.Background(), []string{" learner "}, &output, openFake(store, nil)))
	assert.Equal(t, "ancient greek registration complete: learner=learner book=book-1 created=true\n", output.String())
	assert.True(t, store.closed, "the store is closed")
}

func TestRunPropagatesOpenAndRegistrationErrors(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "postgres://example")
	openErr := errors.New("open failed")
	require.ErrorIs(t, run(context.Background(), []string{"learner"}, &bytes.Buffer{}, openFake(nil, openErr)), openErr)
	require.ErrorContains(t, run(context.Background(), []string{"learner"}, &bytes.Buffer{}, openFake(&fakeRegistrationStore{registerErr: errors.New("register failed")}, nil)), "register failed")
}
