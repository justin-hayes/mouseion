package ancientgreek

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	user          domain.User
	userErr       error
	legacy        []domain.LegacyLanguageRows
	legacyErr     error
	putLanguage   string
	putName       string
	putErr        error
	registration  domain.ReadingUnitBookRegistration
	registerErr   error
	registerCalls int
	registeredFor string
	registeredDef domain.ReadingUnitDefinition
}

func (f *fakeStore) GetUserByUsername(context.Context, string) (domain.User, string, error) {
	return f.user, "", f.userErr
}

func (f *fakeStore) PutSupportedLanguage(_ context.Context, language, name string) (domain.SupportedLanguage, error) {
	f.putLanguage, f.putName = language, name
	return domain.SupportedLanguage{Language: language, DisplayName: name}, f.putErr
}

func (f *fakeStore) ListLegacyLanguageRows(context.Context, string) ([]domain.LegacyLanguageRows, error) {
	return f.legacy, f.legacyErr
}

func (f *fakeStore) RegisterReadingUnitBook(_ context.Context, owner string, definition domain.ReadingUnitDefinition) (domain.ReadingUnitBookRegistration, error) {
	f.registerCalls++
	f.registeredFor, f.registeredDef = owner, definition
	return f.registration, f.registerErr
}

func TestRegisterCreatesUnpreparedBookAndDisplayName(t *testing.T) {
	t.Parallel()
	store := &fakeStore{
		user:         domain.User{ID: "owner-1", Username: "learner"},
		registration: domain.ReadingUnitBookRegistration{BookID: "book-1", Created: true},
	}
	report, err := NewService(store).Register(context.Background(), " learner ", AnabasisBook1)
	require.NoError(t, err)
	assert.Equal(t, Report{Username: "learner", OwnerID: "owner-1", BookID: "book-1", Created: true}, report)
	assert.Equal(t, Language, store.putLanguage, "language display name is ensured")
	assert.Equal(t, DisplayName, store.putName)
	assert.Equal(t, "owner-1", store.registeredFor)
	assert.Equal(t, AnabasisBook1, store.registeredDef)
}

func TestRegisterIsIdempotentWhenBookExists(t *testing.T) {
	t.Parallel()
	store := &fakeStore{
		user:         domain.User{ID: "owner-1"},
		registration: domain.ReadingUnitBookRegistration{BookID: "book-1", Created: false},
	}
	report, err := NewService(store).Register(context.Background(), "learner", AnabasisBook1)
	require.NoError(t, err)
	assert.False(t, report.Created)
	assert.Equal(t, "book-1", report.BookID)
}

func TestRegisterRefusesWhenLegacyRowsExist(t *testing.T) {
	t.Parallel()
	store := &fakeStore{
		user: domain.User{ID: "owner-1"},
		legacy: []domain.LegacyLanguageRows{
			{OwnerID: "owner-9", Surface: "books", Count: 2},
			{OwnerID: "owner-9", Surface: "known_vocabulary", Count: 5},
		},
	}
	_, err := NewService(store).Register(context.Background(), "learner", AnabasisBook1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "owner=owner-9 surface=books count=2")
	assert.Contains(t, err.Error(), "owner=owner-9 surface=known_vocabulary count=5")
	assert.Zero(t, store.registerCalls, "refusal writes nothing")
	assert.Empty(t, store.putLanguage, "refusal does not write the display name")
}

func TestRegisterRejectsInvalidInputs(t *testing.T) {
	t.Parallel()
	store := &fakeStore{user: domain.User{ID: "owner-1"}}
	service := NewService(store)
	require.ErrorContains(t, mustErr(service.Register(context.Background(), "  ", AnabasisBook1)), "username is required")

	bad := AnabasisBook1
	bad.SourceFileSHA256 = "nope"
	require.ErrorContains(t, mustErr(service.Register(context.Background(), "learner", bad)), "SHA-256")

	store.userErr = errors.New("no such learner")
	require.ErrorContains(t, mustErr(service.Register(context.Background(), "learner", AnabasisBook1)), "resolve learner")
}

func TestRegisterPropagatesStoreErrors(t *testing.T) {
	t.Parallel()
	service := func(store *fakeStore) *Service {
		store.user = domain.User{ID: "owner-1"}
		return NewService(store)
	}
	require.ErrorContains(t, mustErr(service(&fakeStore{legacyErr: errors.New("legacy failed")}).Register(context.Background(), "learner", AnabasisBook1)), "inspect grc rows")
	require.ErrorContains(t, mustErr(service(&fakeStore{putErr: errors.New("put failed")}).Register(context.Background(), "learner", AnabasisBook1)), "ensure display name")
	require.ErrorContains(t, mustErr(service(&fakeStore{registerErr: errors.New("register failed")}).Register(context.Background(), "learner", AnabasisBook1)), "register book")
}

func TestAnabasisBook1Pin(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "grc", AnabasisBook1.Language)
	assert.Equal(t, "1.1.1", AnabasisBook1.CitationStart)
	assert.Equal(t, "1.10.19", AnabasisBook1.CitationEnd)
	assert.Equal(t, "Marchant 1904", AnabasisBook1.EditionLabel)
	assert.Equal(t, int64(7438037), AnabasisBook1.SourceFileSize)
	assert.Equal(t, "c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61", AnabasisBook1.SourceFileSHA256)
	require.NoError(t, AnabasisBook1.Validate())
}

func mustErr(_ Report, err error) error { return err }
