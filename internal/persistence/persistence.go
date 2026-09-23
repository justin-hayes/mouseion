// Package persistence provides PostgreSQL repositories with explicit ownership boundaries.
package persistence

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/justin-hayes/mouseion/migrations"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

var ErrNotFound = errors.New("persistence: not found")
var ErrSecretRequired = errors.New("persistence: MOUSEION_SECRET is required for OPDS credentials")
var ErrSecretWeak = errors.New("persistence: MOUSEION_SECRET must be at least 32 bytes")
var ErrInvalidTransition = errors.New("persistence: invalid state transition")
var ErrImmutable = errors.New("persistence: ready artifact is immutable")
var ErrJourneyStale = errors.New("persistence: reading journey state is stale")
var ErrGoalExists = errors.New("persistence: primary goal already exists")

var ErrGoalStale = errors.New("persistence: primary goal state is stale")
var ErrGoalIneligible = errors.New("persistence: primary goal requires an analyzed Journey member")
var ErrReadingAlreadyCompleted = errors.New("persistence: Book has already been completed")
var ErrBookLanguageRequired = errors.New("persistence: book language must be chosen before adding to Reading Journey")
var ErrJourneyLanguageRequired = errors.New("persistence: Journey language is required")
var ErrPreparedDeckClaimLost = errors.New("persistence: prepared-deck claim lost")
var ErrFenced = ErrPreparedDeckClaimLost
var ErrPreparedDeckIdentity = errors.New("persistence: prepared-deck identity mismatch")
var ErrAliasConflict = errors.New("persistence: book alias conflict")
var ErrSourceBookConflict = errors.New("persistence: source material belongs to a different book")
var ErrBookNotFound = ErrNotFound

type Store interface {
	Ping(context.Context) error
	Close() error
}
type PostgresStore struct{ pool *pgxpool.Pool }

// Pool exposes the shared pgx pool to infrastructure packages that need to
// participate in the same transaction (notably River job insertion).
func (s *PostgresStore) Pool() *pgxpool.Pool { return s.pool }

func Open(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	s := &PostgresStore{pool: pool}
	if err := s.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}
func Migrate(databaseURL string) (err error) {
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return fmt.Errorf("initialize migrations: %w", err)
	}
	defer func() {
		sourceErr, databaseErr := m.Close()
		err = errors.Join(err, sourceErr, databaseErr)
	}()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("open postgres for River migrations: %w", err)
	}
	defer pool.Close()
	riverMigrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("initialize River migrations: %w", err)
	}
	if _, err = riverMigrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("apply River migrations: %w", err)
	}
	return nil
}
func (s *PostgresStore) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }
func (s *PostgresStore) Close() error                   { s.pool.Close(); return nil }

// PutSelectionCandidate atomically respects current learner-state exclusions
// and records corpus-specific provenance.
func (s *PostgresStore) PutSelectionCandidate(ctx context.Context, candidate domain.SelectionCandidate) (bool, error) {
	var accepted bool
	err := withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		known, err := q.KnownVocabularyExists(ctx, sqlcgen.KnownVocabularyExistsParams{
			OwnerID: candidate.OwnerID, Language: candidate.Language, CanonicalLemma: candidate.CanonicalLemma, Upos: candidate.UPOS,
		})
		if err != nil {
			return err
		}
		if known {
			return nil
		}
		reserved, err := q.ReservedVocabularyExists(ctx, sqlcgen.ReservedVocabularyExistsParams{
			OwnerID: candidate.OwnerID, Language: candidate.Language, CanonicalLemma: candidate.CanonicalLemma, Upos: candidate.UPOS,
		})
		if err != nil {
			return err
		}
		if reserved {
			return nil
		}
		if err = q.PutSelectionCandidate(ctx, sqlcgen.PutSelectionCandidateParams{
			OwnerID: candidate.OwnerID, CorpusID: candidate.CorpusID, Language: candidate.Language,
			CanonicalLemma: candidate.CanonicalLemma, Upos: candidate.UPOS,
			OccurrenceCount: candidate.OccurrenceCount, ObservedForms: candidate.ObservedForms,
			EligibleSentenceRefs: candidate.SentenceReferences, Provenance: candidate.Provenance,
		}); err != nil {
			return err
		}
		accepted = true
		return nil
	})
	return accepted, err
}

// IsReservedVocabulary reports whether an identity belongs to the owner's
// active Goal snapshot.
func (s *PostgresStore) IsReservedVocabulary(ctx context.Context, owner, language, lemma, upos string) (bool, error) {
	return s.queries().ReservedVocabularyExists(ctx, sqlcgen.ReservedVocabularyExistsParams{
		OwnerID: owner, Language: language, CanonicalLemma: lemma, Upos: upos,
	})
}

// ListUnattachedGeneratedVocabulary returns generated history that has not
// been attached to a prepared deck snapshot. It never infers knowledge.
func (s *PostgresStore) ListUnattachedGeneratedVocabulary(ctx context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
	rows, err := s.queries().ListUnattachedGeneratedVocabulary(ctx, sqlcgen.ListUnattachedGeneratedVocabularyParams{
		OwnerID: owner, Language: language,
	})
	if err != nil {
		return nil, err
	}
	var result []domain.GeneratedVocabulary
	for _, row := range rows {
		result = append(result, domain.GeneratedVocabulary{
			OwnerID: row.OwnerID, Language: row.Language, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos,
			FirstDeckID: row.FirstDeckID, FirstSourceMaterialID: uuidStringPtr(row.FirstSourceMaterialID),
			FirstGeneratedAt: row.FirstGeneratedAt,
		})
	}
	return result, nil
}

func missing(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Passwords are encrypted with AES-256-GCM. The key is derived from the
// deployment's MOUSEION_SECRET; changing it makes existing credentials unreadable.
func ValidateSecret(secret string) error {
	if strings.TrimSpace(secret) == "" {
		return ErrSecretRequired
	}
	if len([]byte(secret)) < 32 {
		return ErrSecretWeak
	}
	return nil
}

func credentialAEAD() (cipher.AEAD, error) {
	secret := os.Getenv("MOUSEION_SECRET")
	if err := ValidateSecret(secret); err != nil {
		return nil, err
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func encryptCredential(value string) ([]byte, error) {
	if value == "" {
		return []byte{}, nil
	}
	aead, err := credentialAEAD()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate credential nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, []byte(value), nil), nil
}
func decryptCredential(value []byte) (string, error) {
	if len(value) == 0 {
		return "", nil
	}
	aead, err := credentialAEAD()
	if err != nil {
		return "", err
	}
	if len(value) < aead.NonceSize() {
		return "", errors.New("persistence: invalid encrypted OPDS credential")
	}
	plain, err := aead.Open(nil, value[:aead.NonceSize()], value[aead.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt OPDS credential: %w", err)
	}
	return string(plain), nil
}

func (s *PostgresStore) CreateOpdsConnection(ctx context.Context, ownerID string, v domain.OpdsConnection) (domain.OpdsConnection, error) {
	encrypted, err := encryptCredential(v.Password)
	if err != nil {
		return domain.OpdsConnection{}, err
	}
	row, err := s.queries().CreateOpdsConnection(ctx, sqlcgen.CreateOpdsConnectionParams{
		OwnerID: uuidArg(ownerID), Name: v.Name, Url: v.URL, Username: v.Username, PasswordEncrypted: encrypted,
	})
	if err != nil {
		return domain.OpdsConnection{}, missing(err)
	}
	return opdsFromFields(row.ID, row.OwnerID, row.Name, row.Url, row.Username, row.PasswordEncrypted, row.Language, row.CreatedAt, row.UpdatedAt)
}
func (s *PostgresStore) GetOpdsConnection(ctx context.Context, ownerID, id string) (domain.OpdsConnection, error) {
	row, err := s.queries().GetOpdsConnection(ctx, sqlcgen.GetOpdsConnectionParams{OwnerID: uuidArg(ownerID), ID: id})
	if err != nil {
		return domain.OpdsConnection{}, missing(err)
	}
	return opdsFromFields(row.ID, row.OwnerID, row.Name, row.Url, row.Username, row.PasswordEncrypted, row.Language, row.CreatedAt, row.UpdatedAt)
}

// OpdsConnectionExists checks ownership without decrypting the credential.
// Callers that only enqueue work can use this at request time; workers load
// the full connection when the job executes.
func (s *PostgresStore) OpdsConnectionExists(ctx context.Context, ownerID, id string) (bool, error) {
	return s.queries().OpdsConnectionExists(ctx, sqlcgen.OpdsConnectionExistsParams{OwnerID: uuidArg(ownerID), ID: id})
}
func (s *PostgresStore) ListOpdsConnections(ctx context.Context, ownerID string) ([]domain.OpdsConnection, error) {
	rows, err := s.queries().ListOpdsConnections(ctx, uuidArg(ownerID))
	if err != nil {
		return nil, err
	}
	var out []domain.OpdsConnection
	for _, row := range rows {
		v, err := opdsFromFields(row.ID, row.OwnerID, row.Name, row.Url, row.Username, row.PasswordEncrypted, row.Language, row.CreatedAt, row.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// ListAllOpdsConnectionIDs enumerates only the owner and connection identity
// needed to restore periodic schedules. It intentionally does not decrypt
// credentials during process startup; decryption happens in job execution.
func (s *PostgresStore) ListAllOpdsConnectionIDs(ctx context.Context) ([]domain.OpdsConnection, error) {
	rows, err := s.queries().ListAllOpdsConnectionIDs(ctx)
	if err != nil {
		return nil, err
	}
	var out []domain.OpdsConnection
	for _, row := range rows {
		out = append(out, domain.OpdsConnection{ID: row.ID, OwnerID: row.OwnerID})
	}
	return out, nil
}
func (s *PostgresStore) UpdateOpdsConnection(ctx context.Context, ownerID string, v domain.OpdsConnection) (domain.OpdsConnection, error) {
	encrypted, err := encryptCredential(v.Password)
	if err != nil {
		return domain.OpdsConnection{}, err
	}
	row, err := s.queries().UpdateOpdsConnection(ctx, sqlcgen.UpdateOpdsConnectionParams{
		OwnerID: uuidArg(ownerID), ID: v.ID, Name: v.Name, Url: v.URL, Username: v.Username, PasswordEncrypted: encrypted,
	})
	if err != nil {
		return domain.OpdsConnection{}, missing(err)
	}
	return opdsFromFields(row.ID, row.OwnerID, row.Name, row.Url, row.Username, row.PasswordEncrypted, row.Language, row.CreatedAt, row.UpdatedAt)
}
func (s *PostgresStore) DeleteOpdsConnection(ctx context.Context, ownerID, id string) error {
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		affected, err := q.DeleteOpdsConnection(ctx, sqlcgen.DeleteOpdsConnectionParams{OwnerID: uuidArg(ownerID), ID: id})
		if err != nil {
			return err
		}
		if affected == 0 {
			return ErrNotFound
		}
		if err := q.DeleteBookCoverCandidatesForConnection(ctx, sqlcgen.DeleteBookCoverCandidatesForConnectionParams{OwnerID: ownerID, ConnectionID: id}); err != nil {
			return err
		}
		return q.ResolveBookCoverAfterConnectionDeletion(ctx, ownerID)
	})
}

func (s *PostgresStore) CreateUser(ctx context.Context, username string, admin bool) (u domain.User, err error) {
	row, err := s.queries().CreateUser(ctx, sqlcgen.CreateUserParams{Username: username, IsAdmin: admin})
	if err == nil {
		u = domain.User{ID: row.ID, Username: row.Username, CreatedAt: row.CreatedAt}
	}
	return
}
func (s *PostgresStore) CreateUserWithPassword(ctx context.Context, username, passwordHash string, admin bool) (u domain.User, err error) {
	row, err := s.queries().CreateUserWithPassword(ctx, sqlcgen.CreateUserWithPasswordParams{Username: username, PasswordHash: textArg(passwordHash), IsAdmin: admin})
	if err == nil {
		u = domain.User{ID: row.ID, Username: row.Username, CreatedAt: row.CreatedAt}
	}
	return
}
func (s *PostgresStore) HasUsers(ctx context.Context) (exists bool, err error) {
	return s.queries().HasUsers(ctx)
}
func (s *PostgresStore) CreateFirstUserAndSession(ctx context.Context, username, passwordHash, tokenHash string, expiresAt time.Time) (u domain.User, created bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return u, false, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	// This process-wide advisory lock serializes first-user creation; it is a
	// domain fence rather than a data query and therefore remains raw SQL.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(110011)`); err != nil {
		return u, false, err
	}
	exists, err := q.HasUsers(ctx)
	if err != nil || exists {
		return u, false, err
	}
	row, err := q.CreateFirstUserAndSession(ctx, sqlcgen.CreateFirstUserAndSessionParams{Username: username, PasswordHash: textArg(passwordHash)})
	if err != nil {
		return u, false, err
	}
	if err = q.InsertSession(ctx, sqlcgen.InsertSessionParams{UserID: row.ID, TokenHash: tokenHash, ExpiresAt: expiresAt}); err != nil {
		return u, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return u, false, err
	}
	return domain.User{ID: row.ID, Username: row.Username, CreatedAt: row.CreatedAt}, true, nil
}
func (s *PostgresStore) GetUserByUsername(ctx context.Context, username string) (u domain.User, passwordHash string, err error) {
	row, err := s.queries().GetUserByUsername(ctx, username)
	if err != nil {
		return domain.User{}, "", missing(err)
	}
	return domain.User{ID: row.ID, Username: row.Username, CreatedAt: row.CreatedAt}, row.PasswordHash, nil
}
func (s *PostgresStore) GetUserByID(ctx context.Context, id string) (u domain.User, err error) {
	row, err := s.queries().GetUserByID(ctx, id)
	if err != nil {
		return domain.User{}, missing(err)
	}
	return domain.User{ID: row.ID, Username: row.Username, CreatedAt: row.CreatedAt}, nil
}

// GetStoredActiveStudyLanguage returns the nullable learner context without
// resolving it against the currently derived study-language set.
func (s *PostgresStore) GetStoredActiveStudyLanguage(ctx context.Context, owner string) (string, error) {
	value, err := s.queries().GetStoredActiveStudyLanguage(ctx, owner)
	if err != nil {
		return "", missing(err)
	}
	return pgText(value), nil
}

// SetActiveStudyLanguage stores only the context pointer. Callers validate it
// against the learner's allowed language context before writing; reads remain lazy.
func (s *PostgresStore) SetActiveStudyLanguage(ctx context.Context, owner, language string) error {
	language = canonicalization.NormalizeLanguage(strings.TrimSpace(language))
	affected, err := s.queries().SetActiveStudyLanguage(ctx, sqlcgen.SetActiveStudyLanguageParams{
		ID: owner, ActiveStudyLanguage: nullableTextArg(language),
	})
	if err == nil && affected == 0 {
		return ErrNotFound
	}
	return err
}

// MostRecentlyActivatedStudyLanguage is deliberately a read-time fallback;
// removing or retagging a Book never updates the stored context eagerly.
func (s *PostgresStore) MostRecentlyActivatedStudyLanguage(ctx context.Context, owner string) (string, error) {
	value, err := s.queries().MostRecentlyActivatedStudyLanguage(ctx, owner)
	if err != nil {
		return "", missing(err)
	}
	return pgText(value), nil
}
func (s *PostgresStore) SetUserPassword(ctx context.Context, userID, passwordHash string) error {
	affected, err := s.queries().SetUserPassword(ctx, sqlcgen.SetUserPasswordParams{ID: userID, PasswordHash: textArg(passwordHash)})
	if err == nil && affected == 0 {
		return ErrNotFound
	}
	return err
}
func (s *PostgresStore) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	return s.queries().InsertSession(ctx, sqlcgen.InsertSessionParams{UserID: userID, TokenHash: tokenHash, ExpiresAt: expiresAt})
}
func (s *PostgresStore) GetSession(ctx context.Context, tokenHash string) (u domain.User, expiresAt time.Time, err error) {
	row, err := s.queries().GetSession(ctx, tokenHash)
	if err != nil {
		return domain.User{}, time.Time{}, missing(err)
	}
	return domain.User{ID: row.UID, Username: row.Username, CreatedAt: row.CreatedAt}, row.ExpiresAt, nil
}
func (s *PostgresStore) DeleteSession(ctx context.Context, tokenHash string) error {
	return s.queries().DeleteSession(ctx, tokenHash)
}
func (s *PostgresStore) DeleteUserSessions(ctx context.Context, userID string) error {
	return s.queries().DeleteUserSessions(ctx, userID)
}
func (s *PostgresStore) PutSupportedLanguage(ctx context.Context, language, name string) (v domain.SupportedLanguage, err error) {
	language = canonicalization.NormalizeLanguage(language)
	name = strings.TrimSpace(name)
	if name == "" {
		name = language
	}
	row, err := s.queries().PutSupportedLanguage(ctx, sqlcgen.PutSupportedLanguageParams{Language: language, DisplayName: name})
	if err != nil {
		return v, err
	}
	return domain.SupportedLanguage{Language: row.Language, DisplayName: row.DisplayName, CreatedAt: row.CreatedAt}, nil
}

func (s *PostgresStore) SyncSupportedLanguages(ctx context.Context, languages []domain.SupportedLanguage) error {
	for _, language := range languages {
		language.Language = canonicalization.NormalizeLanguage(language.Language)
		if language.Language == "" {
			continue
		}
		if strings.TrimSpace(language.DisplayName) == "" {
			if err := s.queries().PutSupportedLanguageOrIgnore(ctx, language.Language); err != nil {
				return err
			}
			continue
		}
		if _, err := s.PutSupportedLanguage(ctx, language.Language, language.DisplayName); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) ListSupportedLanguages(ctx context.Context) ([]domain.SupportedLanguage, error) {
	rows, err := s.queries().ListSupportedLanguages(ctx)
	if err != nil {
		return nil, err
	}
	var out []domain.SupportedLanguage
	for _, row := range rows {
		out = append(out, domain.SupportedLanguage{Language: row.Language, DisplayName: row.DisplayName, CreatedAt: row.CreatedAt})
	}
	return out, nil
}

func (s *PostgresStore) ListAnalysisJobs(ctx context.Context, owner string) ([]domain.AnalysisJob, error) {
	rows, err := s.queries().ListAnalysisJobs(ctx, owner)
	if err != nil {
		return nil, err
	}
	var out []domain.AnalysisJob
	for _, row := range rows {
		out = append(out, analysisJobFromRow(row))
	}
	return out, nil
}

// SetCatalogueSyncStatus writes one owner-scoped connection status. A nil
// LastSyncedAt preserves the previous successful timestamp, which lets a
// later failure retain the last durable success.
func (s *PostgresStore) SetCatalogueSyncStatus(ctx context.Context, status domain.CatalogueSyncStatus) error {
	return s.queries().SetCatalogueSyncStatus(ctx, sqlcgen.SetCatalogueSyncStatusParams{
		OwnerID: status.OwnerID, ConnectionID: status.ConnectionID, State: string(status.State),
		LastSyncedAt: pgTimeArgPtr(status.LastSyncedAt), LastUpsertedCount: status.LastUpsertedCount, LastError: status.LastError,
	})
}

func (s *PostgresStore) GetCatalogueSyncStatus(ctx context.Context, owner, connectionID string) (domain.CatalogueSyncStatus, error) {
	row, err := s.queries().GetCatalogueSyncStatus(ctx, sqlcgen.GetCatalogueSyncStatusParams{OwnerID: owner, ConnectionID: connectionID})
	if err != nil {
		return domain.CatalogueSyncStatus{}, missing(err)
	}
	return catalogueSyncStatusFromRow(row.OwnerID, row.ConnectionID, row.State, row.LastSyncedAt, row.LastUpsertedCount, row.LastError, row.UpdatedAt), nil
}

func (s *PostgresStore) ListCatalogueSyncStatuses(ctx context.Context, owner string) ([]domain.CatalogueSyncStatus, error) {
	rows, err := s.queries().ListCatalogueSyncStatuses(ctx, owner)
	if err != nil {
		return nil, err
	}
	var out []domain.CatalogueSyncStatus
	for _, row := range rows {
		out = append(out, catalogueSyncStatusFromRow(row.OwnerID, row.ConnectionID, row.State, row.LastSyncedAt, row.LastUpsertedCount, row.LastError, row.UpdatedAt))
	}
	return out, nil
}
func (s *PostgresStore) PutSourceMaterial(ctx context.Context, v domain.SourceMaterial) (out domain.SourceMaterial, err error) {
	content := v.Content
	if content == nil {
		content = []byte{}
	}
	id, err := s.queries().PutSourceMaterial(ctx, sqlcgen.PutSourceMaterialParams{
		OwnerID: v.OwnerID, Language: v.Language, SourceIdentifier: v.SourceIdentifier, Title: v.Title,
		MediaType: v.MediaType, ContentHash: v.ContentHash, Content: content, FullText: v.FullText,
	})
	if err != nil {
		return out, err
	}
	return s.GetSourceMaterial(ctx, v.OwnerID, id)
}
func (s *PostgresStore) GetSourceMaterial(ctx context.Context, owner, id string) (v domain.SourceMaterial, err error) {
	row, err := s.queries().GetSourceMaterial(ctx, sqlcgen.GetSourceMaterialParams{OwnerID: owner, ID: id})
	if err != nil {
		return v, missing(err)
	}
	return sourceMaterialFromFields(row.SID, row.SOwnerID, row.Language, row.SourceIdentifier, row.Title, row.MediaType, row.ContentHash, row.ContentDigest, row.ContentRevisionID, row.Content, row.FullText, row.DigestVersion, row.CreatedAt), nil
}

// FindSourceMaterialForAcquisition returns an existing owner-scoped source
// when the current bytes match an OPDS acquisition, either through its source
// identity or through its content digest. A changed download for an existing
// source identity must pass through the immutable revision path instead of
// being incorrectly reported as AlreadyPresent.
func (s *PostgresStore) FindSourceMaterialForAcquisition(ctx context.Context, owner, sourceIdentifier, contentHash string) (v domain.SourceMaterial, found bool, err error) {
	row, err := s.queries().FindSourceMaterialForAcquisition(ctx, sqlcgen.FindSourceMaterialForAcquisitionParams{
		OwnerID: owner, SourceIdentifier: sourceIdentifier, ContentHash: contentHash,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SourceMaterial{}, false, nil
	}
	if err != nil {
		return domain.SourceMaterial{}, false, err
	}
	return sourceMaterialFromFields(row.SID, row.SOwnerID, row.Language, row.SourceIdentifier, row.Title, row.MediaType, row.ContentHash, row.ContentDigest, row.ContentRevisionID, row.Content, row.FullText, row.DigestVersion, row.CreatedAt), true, nil
}

// ListSourceMaterials returns an owner's library with lifecycle state and the
// owner/book-scoped current analysis. Operational jobs remain visible for
// status, but never select a learner-facing result.
func (s *PostgresStore) ListSourceMaterials(ctx context.Context, owner string) ([]domain.SourceMaterialSummary, error) {
	rows, err := s.queries().ListSourceMaterials(ctx, owner)
	if err != nil {
		return nil, err
	}
	var out []domain.SourceMaterialSummary
	for _, row := range rows {
		out = append(out, sourceMaterialSummaryFromRow(row))
	}
	return out, nil
}

func (s *PostgresStore) PutArtifact(ctx context.Context, a domain.NormalizedArtifact, lemmas []domain.SharedLemma) error {
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		if err := q.PutNormalizedCorpusArtifact(ctx, sqlcgen.PutNormalizedCorpusArtifactParams{
			ContentHash: a.ContentHash, Language: a.Language, SchemaVersion: a.SchemaVersion,
			NormalizationProfile: a.NormalizationProfile, NormalizationVersion: a.NormalizationVersion,
			AnalyzerName: a.AnalyzerName, AnalyzerVersion: a.AnalyzerVersion,
		}); err != nil {
			return err
		}
		for _, l := range lemmas {
			if err := q.PutSharedLemma(ctx, sqlcgen.PutSharedLemmaParams{
				ContentHash: a.ContentHash, Language: a.Language, CanonicalLemma: l.CanonicalLemma,
				Upos: l.UPOS, Morphology: l.Morphology, Frequency: l.Frequency,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *PostgresStore) GetArtifact(ctx context.Context, hash string) (a domain.NormalizedArtifact, ls []domain.SharedLemma, err error) {
	artifact, err := s.queries().GetNormalizedCorpusArtifact(ctx, hash)
	if err != nil {
		return a, nil, missing(err)
	}
	a = domain.NormalizedArtifact{
		ContentHash: artifact.ContentHash, Language: artifact.Language, SchemaVersion: artifact.SchemaVersion,
		NormalizationProfile: artifact.NormalizationProfile, NormalizationVersion: artifact.NormalizationVersion,
		AnalyzerName: artifact.AnalyzerName, AnalyzerVersion: artifact.AnalyzerVersion,
		CreatedAt: artifact.CreatedAt,
	}
	rows, err := s.queries().ListSharedLemmas(ctx, hash)
	if err != nil {
		return a, nil, err
	}
	for _, row := range rows {
		ls = append(ls, domain.SharedLemma{
			ContentHash: row.ContentHash, Language: row.Language, CanonicalLemma: row.CanonicalLemma,
			UPOS: row.Upos, Morphology: row.Morphology, Frequency: row.Frequency,
		})
	}
	return a, ls, nil
}
func (s *PostgresStore) PutCorpus(ctx context.Context, owner, sourceID, hash string) (v domain.Corpus, err error) {
	err = withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		// This advisory lock serializes per-owner/source corpus replacement; it
		// is a domain fence rather than a data query and remains raw SQL.
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 191))`, owner+":"+sourceID); err != nil {
			return err
		}
		latest, err := q.LatestCorpusForSource(ctx, sqlcgen.LatestCorpusForSourceParams{OwnerID: owner, SourceMaterialID: sourceID})
		if errors.Is(err, pgx.ErrNoRows) {
			created, insertErr := q.PutCorpus(ctx, sqlcgen.PutCorpusParams{OwnerID: owner, SourceMaterialID: sourceID, ArtifactHash: hash})
			if insertErr != nil {
				return insertErr
			}
			v = domain.Corpus{ID: created.ID, OwnerID: created.OwnerID, SourceMaterialID: created.SourceMaterialID, ArtifactHash: created.ArtifactHash, Status: created.Status, CreatedAt: created.CreatedAt}
			return nil
		}
		if err != nil {
			return err
		}
		if err = q.UpdateCorpusArtifactHash(ctx, sqlcgen.UpdateCorpusArtifactHashParams{OwnerID: owner, ID: latest.ID, ArtifactHash: hash}); err != nil {
			return err
		}
		v = domain.Corpus{ID: latest.ID, OwnerID: latest.OwnerID, SourceMaterialID: latest.SourceMaterialID, ArtifactHash: hash, Status: latest.Status, CreatedAt: latest.CreatedAt}
		return nil
	})
	return v, err
}
func (s *PostgresStore) GetCorpus(ctx context.Context, owner, id string) (v domain.Corpus, err error) {
	row, err := s.queries().GetCorpus(ctx, sqlcgen.GetCorpusParams{OwnerID: owner, ID: id})
	if err != nil {
		return v, missing(err)
	}
	return corpusFromRow(row), nil
}
func (s *PostgresStore) PutKnownVocabulary(ctx context.Context, owner, lang, lemma, upos string) (v domain.KnownVocabulary, err error) {
	lang = canonicalization.NormalizeLanguage(lang)
	existing, err := s.queries().GetKnownVocabularyByIdentity(ctx, sqlcgen.GetKnownVocabularyByIdentityParams{OwnerID: owner, Language: lang, CanonicalLemma: lemma, Upos: upos})
	if err == nil {
		return knownVocabularyFromFields(existing.ID, existing.OwnerID, existing.Language, existing.CanonicalLemma, existing.Upos, existing.CreatedAt), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.KnownVocabulary{}, err
	}
	row, err := s.queries().UpsertKnownVocabulary(ctx, sqlcgen.UpsertKnownVocabularyParams{OwnerID: owner, Language: lang, CanonicalLemma: lemma, Upos: upos})
	if err != nil {
		return domain.KnownVocabulary{}, err
	}
	return knownVocabularyFromFields(row.ID, row.OwnerID, row.Language, row.CanonicalLemma, row.Upos, row.CreatedAt), nil
}
func (s *PostgresStore) GetKnownVocabulary(ctx context.Context, owner, id string) (v domain.KnownVocabulary, err error) {
	row, err := s.queries().GetKnownVocabulary(ctx, sqlcgen.GetKnownVocabularyParams{OwnerID: owner, ID: id})
	if err != nil {
		return v, missing(err)
	}
	return domain.KnownVocabulary{
		ID: row.KvID, OwnerID: row.KvOwnerID, Language: row.Language, CanonicalLemma: row.CanonicalLemma,
		UPOS: row.Upos, Provenance: row.Provenance, CreatedAt: row.CreatedAt,
	}, nil
}
func (s *PostgresStore) ListKnownVocabulary(ctx context.Context, owner, lang string) ([]domain.KnownVocabulary, error) {
	lang = canonicalization.NormalizeLanguage(lang)
	rows, err := s.queries().ListKnownVocabulary(ctx, sqlcgen.ListKnownVocabularyParams{OwnerID: owner, Language: lang})
	if err != nil {
		return nil, err
	}
	var result []domain.KnownVocabulary
	for _, row := range rows {
		result = append(result, domain.KnownVocabulary{
			ID: row.KvID, OwnerID: row.KvOwnerID, Language: row.Language, CanonicalLemma: row.CanonicalLemma,
			UPOS: row.Upos, Provenance: row.Provenance, CreatedAt: row.CreatedAt,
		})
	}
	return result, nil
}

func (s *PostgresStore) ListKnownVocabularyLanguages(ctx context.Context, owner string) ([]domain.StudyLanguage, error) {
	rows, err := s.queries().ListKnownVocabularyLanguages(ctx, owner)
	if err != nil {
		return nil, err
	}
	var out []domain.StudyLanguage
	for _, row := range rows {
		out = append(out, domain.StudyLanguage{Language: row.Language, DisplayName: row.DisplayName})
	}
	return out, nil
}
func (s *PostgresStore) IsKnownVocabularyIdentity(ctx context.Context, owner, lang, lemma, upos string) (bool, error) {
	lang = canonicalization.NormalizeLanguage(lang)
	return s.queries().IsKnownVocabularyIdentity(ctx, sqlcgen.IsKnownVocabularyIdentityParams{OwnerID: owner, Language: lang, CanonicalLemma: lemma, Upos: upos})
}
func (s *PostgresStore) PutExampleSentence(ctx context.Context, owner, corpus, key, sentence string, loc []byte) (v domain.ExampleSentence, err error) {
	row, err := s.queries().PutExampleSentence(ctx, sqlcgen.PutExampleSentenceParams{
		OwnerID: owner, CorpusID: corpus, SentenceKey: key, SentenceText: sentence, SourceLocation: loc,
	})
	if err != nil {
		return v, err
	}
	return exampleSentenceFromFields(row.ID, row.OwnerID, row.CorpusID, row.SentenceKey, row.SentenceText, row.SourceLocation, pgtype.Text{}, pgtype.Text{}, pgtype.Text{}, pgtype.Int4{}, pgtype.Int4{}, nil, false, row.CreatedAt), nil
}

// ReplaceSelectedSentences atomically replaces one owner's ranked examples for
// a vocabulary identity. The owner/corpus foreign key rejects references to a
// different owner's corpus.
func (s *PostgresStore) ReplaceSelectedSentences(ctx context.Context, owner, corpus, language, lemma, upos string, examples []domain.ExampleSentence) error {
	return withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		ownsCorpus, err := q.CorpusOwned(ctx, sqlcgen.CorpusOwnedParams{OwnerID: owner, ID: corpus})
		if err != nil {
			return err
		}
		if !ownsCorpus {
			return ErrNotFound
		}
		if err = q.DeleteSelectedSentences(ctx, sqlcgen.DeleteSelectedSentencesParams{OwnerID: owner, CorpusID: corpus, Language: textArg(language), CanonicalLemma: textArg(lemma), Upos: textArg(upos)}); err != nil {
			return err
		}
		for _, example := range examples {
			selectionRank, conversionErr := intArg(example.SelectionRank)
			if conversionErr != nil {
				return fmt.Errorf("invalid selected sentence rank: %w", conversionErr)
			}
			selectionScore, conversionErr := intArg(example.SelectionScore)
			if conversionErr != nil {
				return fmt.Errorf("invalid selected sentence score: %w", conversionErr)
			}
			if err = q.InsertSelectedSentence(ctx, sqlcgen.InsertSelectedSentenceParams{
				OwnerID: owner, CorpusID: corpus, SentenceKey: example.SentenceKey,
				SentenceText: example.Text, SourceLocation: example.SourceLocation,
				Language: textArg(language), CanonicalLemma: textArg(lemma), Upos: textArg(upos),
				SelectionRank: selectionRank, SelectionScore: selectionScore,
				SelectionReasons: example.SelectionReasons, IsChosen: example.Chosen,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *PostgresStore) ListSelectedSentences(ctx context.Context, owner, corpus, language, lemma, upos string) ([]domain.ExampleSentence, error) {
	rows, err := s.queries().ListSelectedSentences(ctx, sqlcgen.ListSelectedSentencesParams{
		Owner: owner, Corpus: corpus,
		Language: textArg(language), CanonicalLemma: textArg(lemma), Upos: textArg(upos),
	})
	if err != nil {
		return nil, err
	}
	var out []domain.ExampleSentence
	for _, row := range rows {
		out = append(out, exampleSentenceFromFields(row.ID, row.OwnerID, row.CorpusID, row.SentenceKey, row.SentenceText, row.SourceLocation, row.Language, row.CanonicalLemma, row.Upos, row.SelectionRank, row.SelectionScore, row.SelectionReasons, row.IsChosen, row.CreatedAt))
	}
	return out, nil
}
func (s *PostgresStore) PutCuratedSentence(ctx context.Context, owner, example, lang, lemma, upos, notes string) (v domain.CuratedSentence, err error) {
	row, err := s.queries().PutCuratedSentence(ctx, sqlcgen.PutCuratedSentenceParams{
		OwnerID: owner, ExampleSentenceID: example, Language: lang, CanonicalLemma: lemma, Upos: upos, Notes: notes,
	})
	if err != nil {
		return v, err
	}
	return curatedSentenceFromFields(row.ID, row.OwnerID, row.ExampleSentenceID, row.Language, row.CanonicalLemma, row.Upos, row.Notes, row.CreatedAt), nil
}

// ListReviewSentences returns the persisted sentence choices for an owner's
// vocabulary identity. The initially selected sentence is first, followed by
// alternatives in deterministic rank order.
func (s *PostgresStore) ListReviewSentences(ctx context.Context, owner, lang, lemma, upos string) ([]domain.ExampleSentence, error) {
	rows, err := s.queries().ListReviewSentences(ctx, sqlcgen.ListReviewSentencesParams{
		Owner: owner, Language: textArg(lang), CanonicalLemma: textArg(lemma), Upos: textArg(upos),
	})
	if err != nil {
		return nil, err
	}
	var out []domain.ExampleSentence
	for _, row := range rows {
		out = append(out, exampleSentenceFromFields(row.ID, row.OwnerID, row.CorpusID, row.SentenceKey, row.SentenceText, row.SourceLocation, row.Language, row.CanonicalLemma, row.Upos, row.SelectionRank, row.SelectionScore, row.SelectionReasons, row.IsChosen, row.CreatedAt))
	}
	return out, nil
}

// ListReviewSentencesForBook binds curation to the source material currently
// being reviewed, even when the same vocabulary identity occurs in other books.
func (s *PostgresStore) ListReviewSentencesForBook(ctx context.Context, owner, bookID, lang, lemma, upos string) ([]domain.ExampleSentence, error) {
	rows, err := s.queries().ListReviewSentencesForBook(ctx, sqlcgen.ListReviewSentencesForBookParams{
		Owner: owner, Book: bookID,
		Language: textArg(lang), CanonicalLemma: textArg(lemma), Upos: textArg(upos),
	})
	if err != nil {
		return nil, err
	}
	var out []domain.ExampleSentence
	for _, row := range rows {
		out = append(out, exampleSentenceFromFields(row.ID, row.OwnerID, row.CorpusID, row.SentenceKey, row.SentenceText, row.SourceLocation, row.Language, row.CanonicalLemma, row.Upos, row.SelectionRank, row.SelectionScore, row.SelectionReasons, row.IsChosen, row.CreatedAt))
	}
	return out, nil
}

// PersistReviewSentenceFromAnalysis materializes the first sentence retained by
// selection so review can curate it when sentence ranking persisted no choices.
func (s *PostgresStore) PersistReviewSentenceFromAnalysis(ctx context.Context, owner, bookID, lang, lemma, upos string) (v domain.ExampleSentence, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	q := sqlcgen.New(tx)
	// Match the original dynamic query's trim semantics: a blank (after trim)
	// bookID is treated as "any corpus", otherwise the predicate filters on the
	// raw bookID exactly as the pre-sqlc concatenation did.
	book := ""
	if strings.TrimSpace(bookID) != "" {
		book = bookID
	}
	row, err := q.SelectAcquisitionCandidate(ctx, sqlcgen.SelectAcquisitionCandidateParams{
		Owner: owner, Language: lang, CanonicalLemma: lemma, Upos: upos, Book: book,
	})
	if err != nil {
		return v, missing(err)
	}
	var candidates []struct {
		SentenceIndex int             `json:"sentence_index"`
		Text          string          `json:"text"`
		Location      json.RawMessage `json:"location"`
	}
	if err = json.Unmarshal(row.EligibleSentenceRefs, &candidates); err != nil {
		return v, err
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Text) == "" {
			continue
		}
		location := candidate.Location
		if len(location) == 0 || string(location) == "null" {
			location = json.RawMessage(`{}`)
		}
		sentenceKey := fmt.Sprintf("analysis:%d", candidate.SentenceIndex)
		inserted, err := q.UpsertReviewSentenceFromAnalysis(ctx, sqlcgen.UpsertReviewSentenceFromAnalysisParams{
			OwnerID: owner, CorpusID: row.CorpusID, SentenceKey: sentenceKey,
			SentenceText: candidate.Text, SourceLocation: location,
			Language:       textArg(lang),
			CanonicalLemma: textArg(lemma),
			Upos:           textArg(upos),
		})
		if err != nil {
			return v, err
		}
		if err = tx.Commit(ctx); err != nil {
			return v, err
		}
		return exampleSentenceFromFields(inserted.ID, inserted.OwnerID, inserted.CorpusID, inserted.SentenceKey, inserted.SentenceText, inserted.SourceLocation, inserted.Language, inserted.CanonicalLemma, inserted.Upos, inserted.SelectionRank, inserted.SelectionScore, inserted.SelectionReasons, inserted.IsChosen, inserted.CreatedAt), nil
	}
	return v, ErrNotFound
}

// CurateReviewSentence atomically applies an optional owner-scoped text edit,
// records the curated choice, and appends its audit provenance.
func (s *PostgresStore) CurateReviewSentence(ctx context.Context, owner, exampleID, lang, lemma, upos, editedText, notes string, history domain.ProcessingHistory) (v domain.CuratedSentence, err error) {
	err = withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		q := sqlcgen.New(tx)
		exists, err := q.CuratedSentenceExists(ctx, sqlcgen.CuratedSentenceExistsParams{
			OwnerID: owner, ID: exampleID, Language: textArg(lang), CanonicalLemma: textArg(lemma), Upos: textArg(upos),
		})
		if err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		if editedText != "" {
			if err = q.UpdateExampleSentenceText(ctx, sqlcgen.UpdateExampleSentenceTextParams{ID: exampleID, OwnerID: owner, SentenceText: editedText}); err != nil {
				return err
			}
		}
		row, err := q.UpsertCuratedSentence(ctx, sqlcgen.UpsertCuratedSentenceParams{
			OwnerID: owner, ExampleSentenceID: exampleID, Language: lang, CanonicalLemma: lemma, Upos: upos, Notes: notes,
		})
		if err != nil {
			return err
		}
		v = curatedSentenceFromFields(row.ID, row.OwnerID, row.ExampleSentenceID, row.Language, row.CanonicalLemma, row.Upos, row.Notes, row.CreatedAt)
		return q.InsertProcessingHistoryWithoutCorpus(ctx, sqlcgen.InsertProcessingHistoryWithoutCorpusParams{
			OwnerID: history.OwnerID, Operation: history.Operation, Status: history.Status, Details: history.Details, CompletedAt: pgTimeArgPtr(history.CompletedAt),
		})
	})
	return v, err
}
func (s *PostgresStore) PutDeck(ctx context.Context, owner, lang, name string) (v domain.Deck, err error) {
	row, err := s.queries().PutDeck(ctx, sqlcgen.PutDeckParams{OwnerID: owner, Language: lang, Name: name})
	if err != nil {
		return v, err
	}
	return domain.Deck(row), nil
}
func (s *PostgresStore) PutCard(ctx context.Context, v domain.Card) (out domain.Card, err error) {
	row, err := s.queries().PutCard(ctx, sqlcgen.PutCardParams{
		OwnerID: v.OwnerID, DeckID: v.DeckID, DedupKey: v.DedupKey,
		CanonicalLemma: v.CanonicalLemma, Upos: v.UPOS, Front: v.Front, Back: v.Back,
	})
	if err != nil {
		return out, err
	}
	return cardFromFields(row.ID, row.OwnerID, row.DeckID, row.DedupKey, row.CanonicalLemma, row.Upos, row.Front, row.Back, row.CreatedAt), nil
}
func (s *PostgresStore) PutProcessingHistory(ctx context.Context, v domain.ProcessingHistory) (out domain.ProcessingHistory, err error) {
	row, err := s.queries().PutProcessingHistory(ctx, sqlcgen.PutProcessingHistoryParams{
		OwnerID: v.OwnerID, CorpusID: nullableUUIDArg(v.CorpusID), Operation: v.Operation, Status: v.Status,
		Details: v.Details, CompletedAt: pgTimeArgPtr(v.CompletedAt),
	})
	if err != nil {
		return out, err
	}
	out = domain.ProcessingHistory{
		ID: row.ID, OwnerID: row.OwnerID, CorpusID: row.CorpusID, Operation: row.Operation, Status: row.Status,
		Details: row.Details, StartedAt: row.StartedAt, CompletedAt: pgTimePtr(row.CompletedAt),
	}
	return out, nil
}
