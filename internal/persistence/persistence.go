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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/migrations"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

var ErrNotFound = errors.New("persistence: not found")
var ErrSecretRequired = errors.New("persistence: MOUSEION_SECRET is required for OPDS credentials")
var ErrSecretWeak = errors.New("persistence: MOUSEION_SECRET must be at least 32 bytes")
var ErrInvalidTransition = errors.New("persistence: invalid state transition")
var ErrImmutable = errors.New("persistence: ready artifact is immutable")
var ErrActiveCampaign = errors.New("persistence: owner already has an active learning campaign")
var ErrStaleCampaignState = errors.New("persistence: learning campaign state is stale")
var ErrJourneyStale = errors.New("persistence: reading journey state is stale")
var ErrGoalExists = errors.New("persistence: primary goal already exists")
var ErrGoalStale = errors.New("persistence: primary goal state is stale")
var ErrGoalIneligible = errors.New("persistence: primary goal requires an analyzed Journey member")
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
func Migrate(databaseURL string) error {
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return fmt.Errorf("initialize migrations: %w", err)
	}
	defer m.Close()
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

// PutSelectionCandidate atomically respects current suppression decisions,
// creates an initial candidate state, and records corpus-specific provenance.
// The generated state is legacy bookkeeping; generated_vocabulary is the
// authoritative generated-history source used by coverage export.
func (s *PostgresStore) PutSelectionCandidate(ctx context.Context, candidate domain.SelectionCandidate) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var state string
	err = tx.QueryRow(ctx, `SELECT state FROM vocabulary_states WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4 FOR UPDATE`, candidate.OwnerID, candidate.Language, candidate.CanonicalLemma, candidate.UPOS).Scan(&state)
	stateMissing := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if err == nil && (state == "known" || state == "ignored") {
		return false, nil
	}
	var known bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM known_vocabulary WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4)`, candidate.OwnerID, candidate.Language, candidate.CanonicalLemma, candidate.UPOS).Scan(&known); err != nil {
		return false, err
	}
	if known {
		return false, nil
	}
	var reserved bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM learning_campaign_vocabulary cv
		JOIN learning_campaigns c ON c.owner_id=cv.owner_id AND c.id=cv.campaign_id
		WHERE cv.owner_id=$1 AND cv.language=$2 AND cv.canonical_lemma=$3 AND cv.upos=$4 AND c.status='active'
	)`, candidate.OwnerID, candidate.Language, candidate.CanonicalLemma, candidate.UPOS).Scan(&reserved); err != nil {
		return false, err
	}
	if reserved {
		return false, nil
	}
	if stateMissing {
		_, err = tx.Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES($1,$2,$3,$4,'candidate') ON CONFLICT DO NOTHING`, candidate.OwnerID, candidate.Language, candidate.CanonicalLemma, candidate.UPOS)
		if err != nil {
			return false, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(owner_id,corpus_id,language,canonical_lemma,upos) DO UPDATE SET occurrence_count=excluded.occurrence_count,observed_forms=excluded.observed_forms,eligible_sentence_refs=excluded.eligible_sentence_refs,provenance=excluded.provenance,selected_at=now()`, candidate.OwnerID, candidate.CorpusID, candidate.Language, candidate.CanonicalLemma, candidate.UPOS, candidate.OccurrenceCount, candidate.ObservedForms, candidate.SentenceReferences, candidate.Provenance)
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// IsLearningCampaignVocabularyReserved reports whether an identity is assigned
// to the owner's active campaign. Queued and abandoned campaigns do not reserve
// vocabulary for future selection.
func (s *PostgresStore) IsLearningCampaignVocabularyReserved(ctx context.Context, owner, language, lemma, upos string) (bool, error) {
	var reserved bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM learning_campaign_vocabulary cv
		JOIN learning_campaigns c ON c.owner_id=cv.owner_id AND c.id=cv.campaign_id
		WHERE cv.owner_id=$1 AND cv.language=$2 AND cv.canonical_lemma=$3 AND cv.upos=$4 AND c.status='active'
	)`, owner, language, lemma, upos).Scan(&reserved)
	return reserved, err
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

const opdsColumns = `id,owner_id,name,url,username,password_encrypted,language,created_at,updated_at`

func scanOpds(row pgx.Row) (v domain.OpdsConnection, err error) {
	var encrypted []byte
	var legacyLanguage string
	err = row.Scan(&v.ID, &v.OwnerID, &v.Name, &v.URL, &v.Username, &encrypted, &legacyLanguage, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return v, missing(err)
	}
	v.Password, err = decryptCredential(encrypted)
	return v, err
}

func (s *PostgresStore) CreateOpdsConnection(ctx context.Context, ownerID string, v domain.OpdsConnection) (domain.OpdsConnection, error) {
	encrypted, err := encryptCredential(v.Password)
	if err != nil {
		return domain.OpdsConnection{}, err
	}
	return scanOpds(s.pool.QueryRow(ctx, `INSERT INTO opds_connections(owner_id,name,url,username,password_encrypted,language) VALUES($1,$2,$3,$4,$5,'') RETURNING `+opdsColumns, ownerID, v.Name, v.URL, v.Username, encrypted))
}
func (s *PostgresStore) GetOpdsConnection(ctx context.Context, ownerID, id string) (domain.OpdsConnection, error) {
	return scanOpds(s.pool.QueryRow(ctx, `SELECT `+opdsColumns+` FROM opds_connections WHERE owner_id=$1 AND id=$2`, ownerID, id))
}

// OpdsConnectionExists checks ownership without decrypting the credential.
// Callers that only enqueue work can use this at request time; workers load
// the full connection when the job executes.
func (s *PostgresStore) OpdsConnectionExists(ctx context.Context, ownerID, id string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM opds_connections WHERE owner_id=$1 AND id=$2)`, ownerID, id).Scan(&exists)
	return exists, err
}
func (s *PostgresStore) ListOpdsConnections(ctx context.Context, ownerID string) ([]domain.OpdsConnection, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+opdsColumns+` FROM opds_connections WHERE owner_id=$1 ORDER BY name,id`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.OpdsConnection
	for rows.Next() {
		v, err := scanOpds(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListAllOpdsConnectionIDs enumerates only the owner and connection identity
// needed to restore periodic schedules. It intentionally does not decrypt
// credentials during process startup; decryption happens in job execution.
func (s *PostgresStore) ListAllOpdsConnectionIDs(ctx context.Context) ([]domain.OpdsConnection, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,owner_id FROM opds_connections WHERE owner_id IS NOT NULL ORDER BY owner_id,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.OpdsConnection
	for rows.Next() {
		var connection domain.OpdsConnection
		if err = rows.Scan(&connection.ID, &connection.OwnerID); err != nil {
			return nil, err
		}
		out = append(out, connection)
	}
	return out, rows.Err()
}
func (s *PostgresStore) UpdateOpdsConnection(ctx context.Context, ownerID string, v domain.OpdsConnection) (domain.OpdsConnection, error) {
	encrypted, err := encryptCredential(v.Password)
	if err != nil {
		return domain.OpdsConnection{}, err
	}
	return scanOpds(s.pool.QueryRow(ctx, `UPDATE opds_connections SET name=$3,url=$4,username=$5,password_encrypted=$6,updated_at=now() WHERE owner_id=$1 AND id=$2 RETURNING `+opdsColumns, ownerID, v.ID, v.Name, v.URL, v.Username, encrypted))
}
func (s *PostgresStore) DeleteOpdsConnection(ctx context.Context, ownerID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM opds_connections WHERE owner_id=$1 AND id=$2`, ownerID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *PostgresStore) CreateUser(ctx context.Context, username string, admin bool) (u domain.User, err error) {
	err = s.pool.QueryRow(ctx, `INSERT INTO users(username,is_admin) VALUES($1,$2) RETURNING id,username,created_at`, username, admin).Scan(&u.ID, &u.Username, &u.CreatedAt)
	return
}
func (s *PostgresStore) CreateUserWithPassword(ctx context.Context, username, passwordHash string, admin bool) (u domain.User, err error) {
	err = s.pool.QueryRow(ctx, `INSERT INTO users(username,password_hash,is_admin) VALUES($1,$2,$3) RETURNING id,username,created_at`, username, passwordHash, admin).Scan(&u.ID, &u.Username, &u.CreatedAt)
	return
}
func (s *PostgresStore) HasUsers(ctx context.Context) (exists bool, err error) {
	err = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users)`).Scan(&exists)
	return
}
func (s *PostgresStore) CreateFirstUserAndSession(ctx context.Context, username, passwordHash, tokenHash string, expiresAt time.Time) (u domain.User, created bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return u, false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(110011)`); err != nil {
		return u, false, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users)`).Scan(&exists); err != nil || exists {
		return u, false, err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO users(username,password_hash,is_admin) VALUES($1,$2,false) RETURNING id,username,created_at`, username, passwordHash).Scan(&u.ID, &u.Username, &u.CreatedAt); err != nil {
		return u, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO sessions(user_id,token_hash,expires_at) VALUES($1,$2,$3)`, u.ID, tokenHash, expiresAt); err != nil {
		return u, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return u, false, err
	}
	return u, true, nil
}
func (s *PostgresStore) GetUserByUsername(ctx context.Context, username string) (u domain.User, passwordHash string, err error) {
	err = s.pool.QueryRow(ctx, `SELECT id,username,created_at,COALESCE(password_hash,'') FROM users WHERE username=$1`, username).Scan(&u.ID, &u.Username, &u.CreatedAt, &passwordHash)
	err = missing(err)
	return
}
func (s *PostgresStore) GetUserByID(ctx context.Context, id string) (u domain.User, err error) {
	err = s.pool.QueryRow(ctx, `SELECT id,username,created_at FROM users WHERE id=$1`, id).Scan(&u.ID, &u.Username, &u.CreatedAt)
	err = missing(err)
	return
}

// GetStoredActiveStudyLanguage returns the nullable learner context without
// resolving it against the currently derived study-language set.
func (s *PostgresStore) GetStoredActiveStudyLanguage(ctx context.Context, owner string) (string, error) {
	var language *string
	err := s.pool.QueryRow(ctx, `SELECT active_study_language FROM users WHERE id=$1`, owner).Scan(&language)
	if err = missing(err); err != nil {
		return "", err
	}
	if language == nil {
		return "", nil
	}
	return *language, nil
}

// SetActiveStudyLanguage stores only the context pointer. Callers validate it
// against the learner's allowed language context before writing; reads remain lazy.
func (s *PostgresStore) SetActiveStudyLanguage(ctx context.Context, owner, language string) error {
	language = canonicalization.NormalizeLanguage(strings.TrimSpace(language))
	var value any = language
	if language == "" {
		value = nil
	}
	tag, err := s.pool.Exec(ctx, `UPDATE users SET active_study_language=$2 WHERE id=$1`, owner, value)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// MostRecentlyActivatedStudyLanguage is deliberately a read-time fallback;
// removing or retagging a Book never updates the stored context eagerly.
func (s *PostgresStore) MostRecentlyActivatedStudyLanguage(ctx context.Context, owner string) (string, error) {
	var language string
	err := s.pool.QueryRow(ctx, `SELECT b.language_tag
		FROM books b
		JOIN book_membership m ON m.owner_id=b.owner_id AND m.book_id=b.id AND m.state='active'
		WHERE b.owner_id=$1 AND b.language_state='chosen' AND b.language_tag <> ''
		ORDER BY m.activated_at DESC NULLS LAST,b.updated_at DESC,b.id DESC
		LIMIT 1`, owner).Scan(&language)
	if err = missing(err); err != nil {
		return "", err
	}
	return language, nil
}
func (s *PostgresStore) SetUserPassword(ctx context.Context, userID, passwordHash string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1`, userID, passwordHash)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
func (s *PostgresStore) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sessions(user_id,token_hash,expires_at) VALUES($1,$2,$3)`, userID, tokenHash, expiresAt)
	return err
}
func (s *PostgresStore) GetSession(ctx context.Context, tokenHash string) (u domain.User, expiresAt time.Time, err error) {
	err = s.pool.QueryRow(ctx, `SELECT u.id,u.username,u.created_at,s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()`, tokenHash).Scan(&u.ID, &u.Username, &u.CreatedAt, &expiresAt)
	err = missing(err)
	return
}
func (s *PostgresStore) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, tokenHash)
	return err
}
func (s *PostgresStore) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, userID)
	return err
}
func (s *PostgresStore) PutSupportedLanguage(ctx context.Context, language, name string) (v domain.SupportedLanguage, err error) {
	language = canonicalization.NormalizeLanguage(language)
	name = strings.TrimSpace(name)
	if name == "" {
		name = language
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO supported_languages(language,display_name) VALUES($1,$2) ON CONFLICT(language) DO UPDATE SET display_name=excluded.display_name RETURNING language,display_name,created_at`, language, name).Scan(&v.Language, &v.DisplayName, &v.CreatedAt)
	return
}

func (s *PostgresStore) SyncSupportedLanguages(ctx context.Context, languages []domain.SupportedLanguage) error {
	for _, language := range languages {
		language.Language = canonicalization.NormalizeLanguage(language.Language)
		if language.Language == "" {
			continue
		}
		if strings.TrimSpace(language.DisplayName) == "" {
			if _, err := s.pool.Exec(ctx, `INSERT INTO supported_languages(language,display_name) VALUES($1,$1) ON CONFLICT(language) DO NOTHING`, language.Language); err != nil {
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
	rows, err := s.pool.Query(ctx, `SELECT language,display_name,created_at FROM supported_languages ORDER BY display_name,language`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SupportedLanguage
	for rows.Next() {
		var language domain.SupportedLanguage
		if err := rows.Scan(&language.Language, &language.DisplayName, &language.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, language)
	}
	return out, rows.Err()
}

func (s *PostgresStore) ListAnalysisJobs(ctx context.Context, owner string) ([]domain.AnalysisJob, error) {
	rows, err := s.pool.Query(ctx, `SELECT j.river_job_id,j.display_number,j.owner_id,j.source_material_id,j.content_hash,COALESCE(j.corpus_id::text,''),COALESCE(j.analysis_run_id::text,''),COALESCE(r.state,''),j.progress,j.error,j.created_at,j.updated_at FROM analysis_jobs j LEFT JOIN analysis_runs r ON r.owner_id=j.owner_id AND r.id=j.analysis_run_id WHERE j.owner_id=$1 ORDER BY j.created_at DESC,j.river_job_id DESC LIMIT 100`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AnalysisJob
	for rows.Next() {
		var job domain.AnalysisJob
		if err := rows.Scan(&job.ID, &job.DisplayNumber, &job.OwnerID, &job.SourceMaterialID, &job.ContentHash, &job.CorpusID, &job.AnalysisRunID, &job.AnalysisState, &job.Progress, &job.Error, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func scanCatalogueSyncStatus(row pgx.Row) (domain.CatalogueSyncStatus, error) {
	var status domain.CatalogueSyncStatus
	var state string
	if err := row.Scan(&status.OwnerID, &status.ConnectionID, &state, &status.LastSyncedAt, &status.LastUpsertedCount, &status.LastError, &status.UpdatedAt); err != nil {
		return status, missing(err)
	}
	status.State = domain.CatalogueSyncState(state)
	return status, nil
}

// SetCatalogueSyncStatus writes one owner-scoped connection status. A nil
// LastSyncedAt preserves the previous successful timestamp, which lets a
// later failure retain the last durable success.
func (s *PostgresStore) SetCatalogueSyncStatus(ctx context.Context, status domain.CatalogueSyncStatus) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO catalogue_sync_status(owner_id,connection_id,state,last_synced_at,last_upserted_count,last_error) VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(owner_id,connection_id) DO UPDATE SET state=excluded.state,last_synced_at=COALESCE(excluded.last_synced_at,catalogue_sync_status.last_synced_at),last_upserted_count=excluded.last_upserted_count,last_error=excluded.last_error,updated_at=now()`, status.OwnerID, status.ConnectionID, status.State, status.LastSyncedAt, status.LastUpsertedCount, status.LastError)
	return err
}

func (s *PostgresStore) GetCatalogueSyncStatus(ctx context.Context, owner, connectionID string) (domain.CatalogueSyncStatus, error) {
	return scanCatalogueSyncStatus(s.pool.QueryRow(ctx, `SELECT owner_id,connection_id,state,last_synced_at,last_upserted_count,last_error,updated_at FROM catalogue_sync_status WHERE owner_id=$1 AND connection_id=$2`, owner, connectionID))
}

func (s *PostgresStore) ListCatalogueSyncStatuses(ctx context.Context, owner string) ([]domain.CatalogueSyncStatus, error) {
	rows, err := s.pool.Query(ctx, `SELECT owner_id,connection_id,state,last_synced_at,last_upserted_count,last_error,updated_at FROM catalogue_sync_status WHERE owner_id=$1 ORDER BY connection_id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.CatalogueSyncStatus
	for rows.Next() {
		status, scanErr := scanCatalogueSyncStatus(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, status)
	}
	return out, rows.Err()
}
func (s *PostgresStore) PutSourceMaterial(ctx context.Context, v domain.SourceMaterial) (out domain.SourceMaterial, err error) {
	content := v.Content
	if content == nil {
		content = []byte{}
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(owner_id,source_identifier) DO UPDATE SET language=excluded.language,title=excluded.title,media_type=excluded.media_type RETURNING id`, v.OwnerID, v.Language, v.SourceIdentifier, v.Title, v.MediaType, v.ContentHash, content, v.FullText).Scan(&out.ID)
	if err != nil {
		return out, err
	}
	return s.GetSourceMaterial(ctx, v.OwnerID, out.ID)
}
func (s *PostgresStore) GetSourceMaterial(ctx context.Context, owner, id string) (v domain.SourceMaterial, err error) {
	err = s.pool.QueryRow(ctx, `SELECT s.id,s.owner_id,s.language,s.source_identifier,s.title,s.media_type,CASE WHEN r.digest_version=1 THEN r.content_digest ELSE s.content_hash END,COALESCE(r.content_digest,''),COALESCE(r.content,s.content),COALESCE(r.full_text,s.full_text),COALESCE(r.revision_id::text,''),COALESCE(r.digest_version,0),s.created_at FROM source_materials s LEFT JOIN source_content_revisions r ON r.owner_id=s.owner_id AND r.revision_id=s.current_content_revision_id WHERE s.owner_id=$1 AND s.id=$2`, owner, id).Scan(&v.ID, &v.OwnerID, &v.Language, &v.SourceIdentifier, &v.Title, &v.MediaType, &v.ContentHash, &v.ContentDigest, &v.Content, &v.FullText, &v.ContentRevisionID, &v.ContentDigestVersion, &v.CreatedAt)
	err = missing(err)
	return
}

// FindSourceMaterialForAcquisition returns an existing owner-scoped source
// when the current bytes match an OPDS acquisition, either through its source
// identity or through its content digest. A changed download for an existing
// source identity must pass through the immutable revision path instead of
// being incorrectly reported as AlreadyPresent.
func (s *PostgresStore) FindSourceMaterialForAcquisition(ctx context.Context, owner, sourceIdentifier, contentHash string) (v domain.SourceMaterial, found bool, err error) {
	err = s.pool.QueryRow(ctx, `SELECT s.id,s.owner_id,s.language,s.source_identifier,s.title,s.media_type,CASE WHEN r.digest_version=1 THEN r.content_digest ELSE s.content_hash END,COALESCE(r.content_digest,''),COALESCE(r.content,s.content),COALESCE(r.full_text,s.full_text),COALESCE(r.revision_id::text,''),COALESCE(r.digest_version,0),s.created_at FROM source_materials s LEFT JOIN source_content_revisions r ON r.owner_id=s.owner_id AND r.revision_id=s.current_content_revision_id WHERE s.owner_id=$1 AND ((s.source_identifier=$2 AND CASE WHEN r.digest_version=1 THEN r.content_digest ELSE s.content_hash END=$3) OR r.content_digest=$3 OR s.content_hash=$3) ORDER BY (s.source_identifier=$2) DESC,s.created_at,s.id LIMIT 1`, owner, sourceIdentifier, contentHash).Scan(&v.ID, &v.OwnerID, &v.Language, &v.SourceIdentifier, &v.Title, &v.MediaType, &v.ContentHash, &v.ContentDigest, &v.Content, &v.FullText, &v.ContentRevisionID, &v.ContentDigestVersion, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SourceMaterial{}, false, nil
	}
	return v, err == nil, err
}

// ListSourceMaterials returns an owner's library with lifecycle state and the
// owner/book-scoped current analysis. Operational jobs remain visible for
// status, but never select a learner-facing result.
func (s *PostgresStore) ListSourceMaterials(ctx context.Context, owner string) ([]domain.SourceMaterialSummary, error) {
	rows, err := s.pool.Query(ctx, currentAnalysisCTE+`
		SELECT s.id,s.owner_id,s.language,s.source_identifier,s.title,s.media_type,COALESCE(s.book_id::text,''),CASE WHEN r.digest_version=1 THEN r.content_digest ELSE s.content_hash END,COALESCE(r.content_digest,''),COALESCE(r.revision_id::text,''),COALESCE(r.digest_version,0),s.created_at,
			       CASE WHEN ar.state IN ('queued','running') THEN 'analyzing'
		            WHEN ar.state = 'failed' THEN 'analysis failed'
		            WHEN ar.state = 'cancelled' THEN 'analysis cancelled'
		            WHEN p.source_material_id IS NOT NULL AND ca.analysis_run_id IS NULL THEN 'stale'
			            WHEN ca.analysis_run_id IS NOT NULL THEN 'analyzed'
		            WHEN j.river_job_id IS NOT NULL AND j.error = '' THEN 'analyzing'
		            ELSE 'not analyzed' END,
		       CASE WHEN ar.state IS NOT NULL THEN ar.state
		            WHEN ca.analysis_run_id IS NOT NULL THEN 'completed'
		            WHEN j.river_job_id IS NOT NULL AND j.error <> '' THEN 'failed'
		            WHEN j.river_job_id IS NOT NULL THEN 'queued'
		            ELSE '' END,
		       COALESCE(ca.analysis_run_id::text,''),
			       COALESCE(ca.corpus_id::text,''),COALESCE(j.river_job_id,0)
		FROM source_materials s
		LEFT JOIN source_content_revisions r ON r.owner_id=s.owner_id AND r.revision_id=s.current_content_revision_id
		LEFT JOIN book_current_analyses p ON p.owner_id=s.owner_id AND p.source_material_id=s.id
		LEFT JOIN current_analysis ca ON ca.owner_id=p.owner_id AND ca.source_material_id=p.source_material_id
		LEFT JOIN LATERAL (SELECT river_job_id,error,analysis_run_id FROM analysis_jobs WHERE owner_id=s.owner_id AND source_material_id=s.id ORDER BY created_at DESC,river_job_id DESC LIMIT 1) j ON true
		LEFT JOIN analysis_runs ar ON ar.owner_id=s.owner_id AND ar.id=j.analysis_run_id
		WHERE s.owner_id=$1
		ORDER BY s.created_at DESC,s.title,s.id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SourceMaterialSummary
	for rows.Next() {
		var item domain.SourceMaterialSummary
		if err := rows.Scan(&item.Source.ID, &item.Source.OwnerID, &item.Source.Language, &item.Source.SourceIdentifier, &item.Source.Title, &item.Source.MediaType, &item.BookID, &item.Source.ContentHash, &item.Source.ContentDigest, &item.Source.ContentRevisionID, &item.Source.ContentDigestVersion, &item.Source.CreatedAt, &item.AnalysisStatus, &item.AnalysisState, &item.AnalysisRunID, &item.CorpusID, &item.AnalysisJobID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *PostgresStore) PutArtifact(ctx context.Context, a domain.NormalizedArtifact, lemmas []domain.SharedLemma) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO normalized_corpus_artifacts(content_hash,language,schema_version,normalization_profile,normalization_version,analyzer_name,analyzer_version) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(content_hash) DO NOTHING`, a.ContentHash, a.Language, a.SchemaVersion, a.NormalizationProfile, a.NormalizationVersion, a.AnalyzerName, a.AnalyzerVersion)
	if err != nil {
		return err
	}
	for _, l := range lemmas {
		_, err = tx.Exec(ctx, `INSERT INTO shared_lemmas(content_hash,language,canonical_lemma,upos,morphology,frequency) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(content_hash,canonical_lemma,upos,morphology) DO UPDATE SET frequency=excluded.frequency`, a.ContentHash, a.Language, l.CanonicalLemma, l.UPOS, l.Morphology, l.Frequency)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *PostgresStore) GetArtifact(ctx context.Context, hash string) (a domain.NormalizedArtifact, ls []domain.SharedLemma, err error) {
	err = s.pool.QueryRow(ctx, `SELECT content_hash,language,schema_version,normalization_profile,normalization_version,analyzer_name,analyzer_version,created_at FROM normalized_corpus_artifacts WHERE content_hash=$1`, hash).Scan(&a.ContentHash, &a.Language, &a.SchemaVersion, &a.NormalizationProfile, &a.NormalizationVersion, &a.AnalyzerName, &a.AnalyzerVersion, &a.CreatedAt)
	if err != nil {
		return a, nil, missing(err)
	}
	rows, err := s.pool.Query(ctx, `SELECT content_hash,language,canonical_lemma,upos,morphology,frequency FROM shared_lemmas WHERE content_hash=$1 ORDER BY id`, hash)
	if err != nil {
		return a, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l domain.SharedLemma
		if err = rows.Scan(&l.ContentHash, &l.Language, &l.CanonicalLemma, &l.UPOS, &l.Morphology, &l.Frequency); err != nil {
			return a, nil, err
		}
		ls = append(ls, l)
	}
	return a, ls, rows.Err()
}
func (s *PostgresStore) PutCorpus(ctx context.Context, owner, sourceID, hash string) (v domain.Corpus, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 191))`, owner+":"+sourceID); err != nil {
		return v, err
	}
	err = tx.QueryRow(ctx, `SELECT id,owner_id,source_material_id,artifact_hash,status,created_at FROM corpora WHERE owner_id=$1 AND source_material_id=$2 ORDER BY created_at DESC LIMIT 1`, owner, sourceID).Scan(&v.ID, &v.OwnerID, &v.SourceMaterialID, &v.ArtifactHash, &v.Status, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO corpora(owner_id,source_material_id,artifact_hash) VALUES($1,$2,$3) RETURNING id,owner_id,source_material_id,artifact_hash,status,created_at`, owner, sourceID, hash).Scan(&v.ID, &v.OwnerID, &v.SourceMaterialID, &v.ArtifactHash, &v.Status, &v.CreatedAt)
		if err == nil {
			err = tx.Commit(ctx)
		}
		return
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE corpora SET artifact_hash=$3 WHERE owner_id=$1 AND id=$2`, owner, v.ID, hash)
		v.ArtifactHash = hash
		if err == nil {
			err = tx.Commit(ctx)
		}
	}
	return
}
func (s *PostgresStore) GetCorpus(ctx context.Context, owner, id string) (v domain.Corpus, err error) {
	var analyzableTokenCount, distinctLemmaCount, sentenceCount, normalizedTokenCount, emptySentenceCount, p90SentenceTokenCount, longSentenceCount *int64
	var medianSentenceTokenCount *float64
	err = s.pool.QueryRow(ctx, `SELECT id,owner_id,source_material_id,artifact_hash,COALESCE(analysis_run_id::text,''),status,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count,created_at FROM corpora WHERE owner_id=$1 AND id=$2`, owner, id).Scan(&v.ID, &v.OwnerID, &v.SourceMaterialID, &v.ArtifactHash, &v.AnalysisRunID, &v.Status, &analyzableTokenCount, &distinctLemmaCount, &sentenceCount, &normalizedTokenCount, &emptySentenceCount, &medianSentenceTokenCount, &p90SentenceTokenCount, &longSentenceCount, &v.CreatedAt)
	err = missing(err)
	if err == nil && analyzableTokenCount != nil && distinctLemmaCount != nil {
		v.Statistics = &domain.AnalysisStatistics{AnalyzableTokenCount: *analyzableTokenCount, DistinctLemmaCount: *distinctLemmaCount}
		if sentenceCount != nil && normalizedTokenCount != nil && emptySentenceCount != nil && medianSentenceTokenCount != nil && p90SentenceTokenCount != nil && longSentenceCount != nil {
			v.Statistics.TextProfile = &domain.TextProfile{SentenceCount: *sentenceCount, NormalizedTokenCount: *normalizedTokenCount, EmptySentenceCount: *emptySentenceCount, MedianSentenceTokenCount: *medianSentenceTokenCount, P90SentenceTokenCount: *p90SentenceTokenCount, LongSentenceCount: *longSentenceCount}
		}
	}
	return
}
func (s *PostgresStore) PutKnownVocabulary(ctx context.Context, owner, lang, lemma, upos string) (v domain.KnownVocabulary, err error) {
	lang = canonicalization.NormalizeLanguage(lang)
	err = s.pool.QueryRow(ctx, `SELECT id,owner_id,language,canonical_lemma,upos,created_at FROM known_vocabulary WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4 ORDER BY id LIMIT 1`, owner, lang, lemma, upos).Scan(&v.ID, &v.OwnerID, &v.Language, &v.CanonicalLemma, &v.UPOS, &v.CreatedAt)
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.KnownVocabulary{}, err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos) VALUES($1,$2,$3,$4) ON CONFLICT(owner_id,language,canonical_lemma,upos) DO UPDATE SET canonical_lemma=excluded.canonical_lemma RETURNING id,owner_id,language,canonical_lemma,upos,created_at`, owner, lang, lemma, upos).Scan(&v.ID, &v.OwnerID, &v.Language, &v.CanonicalLemma, &v.UPOS, &v.CreatedAt)
	return
}
func (s *PostgresStore) GetKnownVocabulary(ctx context.Context, owner, id string) (v domain.KnownVocabulary, err error) {
	err = s.pool.QueryRow(ctx, `SELECT kv.id,kv.owner_id,kv.language,kv.canonical_lemma,kv.upos,
		CASE WHEN EXISTS (
			SELECT 1 FROM learning_campaign_vocabulary cv
			JOIN learning_campaigns c ON c.owner_id=cv.owner_id AND c.id=cv.campaign_id
			WHERE cv.owner_id=kv.owner_id AND cv.language=kv.language
				AND cv.canonical_lemma=kv.canonical_lemma AND cv.upos=kv.upos
				AND cv.graduated_at IS NOT NULL AND c.status='complete'
		) THEN 'Graduated from completed campaign' ELSE 'Explicitly recorded' END,
		kv.created_at
		FROM known_vocabulary kv WHERE kv.owner_id=$1 AND kv.id=$2`, owner, id).Scan(&v.ID, &v.OwnerID, &v.Language, &v.CanonicalLemma, &v.UPOS, &v.Provenance, &v.CreatedAt)
	err = missing(err)
	return
}
func (s *PostgresStore) ListKnownVocabulary(ctx context.Context, owner, lang string) ([]domain.KnownVocabulary, error) {
	lang = canonicalization.NormalizeLanguage(lang)
	rows, err := s.pool.Query(ctx, `SELECT kv.id,kv.owner_id,kv.language,kv.canonical_lemma,kv.upos,
		CASE WHEN EXISTS (
			SELECT 1 FROM learning_campaign_vocabulary cv
			JOIN learning_campaigns c ON c.owner_id=cv.owner_id AND c.id=cv.campaign_id
			WHERE cv.owner_id=kv.owner_id AND cv.language=kv.language
				AND cv.canonical_lemma=kv.canonical_lemma AND cv.upos=kv.upos
				AND cv.graduated_at IS NOT NULL AND c.status='complete'
		) THEN 'Graduated from completed campaign' ELSE 'Explicitly recorded' END,
		kv.created_at
		FROM known_vocabulary kv WHERE kv.owner_id=$1 AND kv.language=$2 ORDER BY kv.canonical_lemma,kv.upos,kv.id`, owner, lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.KnownVocabulary
	for rows.Next() {
		var value domain.KnownVocabulary
		if err := rows.Scan(&value.ID, &value.OwnerID, &value.Language, &value.CanonicalLemma, &value.UPOS, &value.Provenance, &value.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *PostgresStore) ListKnownVocabularyLanguages(ctx context.Context, owner string) ([]domain.StudyLanguage, error) {
	rows, err := s.pool.Query(ctx, `WITH known_languages AS (
		SELECT DISTINCT language
		FROM known_vocabulary
		WHERE owner_id=$1 AND language <> ''
	), language_names AS (
		SELECT language, display_name
		FROM supported_languages
	)
	SELECT k.language, COALESCE(NULLIF(n.display_name,''), k.language)
	FROM known_languages k
	LEFT JOIN language_names n ON n.language=k.language
	ORDER BY COALESCE(NULLIF(n.display_name,''), k.language), k.language`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.StudyLanguage
	for rows.Next() {
		var language domain.StudyLanguage
		if err := rows.Scan(&language.Language, &language.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, language)
	}
	return out, rows.Err()
}
func (s *PostgresStore) IsKnownVocabularyIdentity(ctx context.Context, owner, lang, lemma, upos string) (bool, error) {
	lang = canonicalization.NormalizeLanguage(lang)
	var known bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM known_vocabulary WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND (upos=$4 OR upos=''))`, owner, lang, lemma, upos).Scan(&known)
	return known, err
}
func (s *PostgresStore) PutVocabularyState(ctx context.Context, owner, lang, lemma, upos, state string) (v domain.VocabularyState, err error) {
	err = s.pool.QueryRow(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner_id,language,canonical_lemma,upos) DO UPDATE SET state=excluded.state,updated_at=now() RETURNING id,owner_id,language,canonical_lemma,upos,state,updated_at`, owner, lang, lemma, upos, state).Scan(&v.ID, &v.OwnerID, &v.Language, &v.CanonicalLemma, &v.UPOS, &v.State, &v.UpdatedAt)
	return
}
func (s *PostgresStore) GetVocabularyState(ctx context.Context, owner, id string) (v domain.VocabularyState, err error) {
	err = s.pool.QueryRow(ctx, `SELECT id,owner_id,language,canonical_lemma,upos,state,updated_at FROM vocabulary_states WHERE owner_id=$1 AND id=$2`, owner, id).Scan(&v.ID, &v.OwnerID, &v.Language, &v.CanonicalLemma, &v.UPOS, &v.State, &v.UpdatedAt)
	err = missing(err)
	return
}
func (s *PostgresStore) GetVocabularyStateByIdentity(ctx context.Context, owner, lang, lemma, upos string) (v domain.VocabularyState, err error) {
	err = s.pool.QueryRow(ctx, `SELECT id,owner_id,language,canonical_lemma,upos,state,updated_at FROM vocabulary_states WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4`, owner, lang, lemma, upos).Scan(&v.ID, &v.OwnerID, &v.Language, &v.CanonicalLemma, &v.UPOS, &v.State, &v.UpdatedAt)
	err = missing(err)
	return
}
func (s *PostgresStore) DeleteVocabularyState(ctx context.Context, owner, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM vocabulary_states WHERE owner_id=$1 AND id=$2`, owner, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
func (s *PostgresStore) PutExampleSentence(ctx context.Context, owner, corpus, key, sentence string, loc []byte) (v domain.ExampleSentence, err error) {
	err = s.pool.QueryRow(ctx, `INSERT INTO example_sentences(owner_id,corpus_id,sentence_key,sentence_text,source_location) VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner_id,corpus_id,sentence_key) DO UPDATE SET sentence_text=excluded.sentence_text,source_location=excluded.source_location RETURNING id,owner_id,corpus_id,sentence_key,sentence_text,source_location,created_at`, owner, corpus, key, sentence, loc).Scan(&v.ID, &v.OwnerID, &v.CorpusID, &v.SentenceKey, &v.Text, &v.SourceLocation, &v.CreatedAt)
	return
}

// ReplaceSelectedSentences atomically replaces one owner's ranked examples for
// a vocabulary identity. The owner/corpus foreign key rejects references to a
// different owner's corpus.
func (s *PostgresStore) ReplaceSelectedSentences(ctx context.Context, owner, corpus, language, lemma, upos string, examples []domain.ExampleSentence) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var ownsCorpus bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM corpora WHERE owner_id=$1 AND id=$2)`, owner, corpus).Scan(&ownsCorpus); err != nil {
		return err
	}
	if !ownsCorpus {
		return ErrNotFound
	}
	if _, err = tx.Exec(ctx, `DELETE FROM example_sentences WHERE owner_id=$1 AND corpus_id=$2 AND language=$3 AND canonical_lemma=$4 AND upos=$5`, owner, corpus, language, lemma, upos); err != nil {
		return err
	}
	for _, example := range examples {
		_, err = tx.Exec(ctx, `INSERT INTO example_sentences(owner_id,corpus_id,sentence_key,sentence_text,source_location,language,canonical_lemma,upos,selection_rank,selection_score,selection_reasons,is_chosen) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, owner, corpus, example.SentenceKey, example.Text, example.SourceLocation, language, lemma, upos, example.SelectionRank, example.SelectionScore, example.SelectionReasons, example.Chosen)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) ListSelectedSentences(ctx context.Context, owner, corpus, language, lemma, upos string) ([]domain.ExampleSentence, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,owner_id,corpus_id,sentence_key,sentence_text,source_location,language,canonical_lemma,upos,selection_rank,selection_score,selection_reasons,is_chosen,created_at FROM example_sentences WHERE owner_id=$1 AND corpus_id=$2 AND language=$3 AND canonical_lemma=$4 AND upos=$5 ORDER BY selection_rank`, owner, corpus, language, lemma, upos)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ExampleSentence
	for rows.Next() {
		var example domain.ExampleSentence
		if err := rows.Scan(&example.ID, &example.OwnerID, &example.CorpusID, &example.SentenceKey, &example.Text, &example.SourceLocation, &example.Language, &example.CanonicalLemma, &example.UPOS, &example.SelectionRank, &example.SelectionScore, &example.SelectionReasons, &example.Chosen, &example.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, example)
	}
	return out, rows.Err()
}
func (s *PostgresStore) PutCuratedSentence(ctx context.Context, owner, example, lang, lemma, upos, notes string) (v domain.CuratedSentence, err error) {
	err = s.pool.QueryRow(ctx, `INSERT INTO curated_sentences(owner_id,example_sentence_id,language,canonical_lemma,upos,notes) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(owner_id,language,canonical_lemma,upos) DO UPDATE SET example_sentence_id=excluded.example_sentence_id,notes=excluded.notes RETURNING id,owner_id,example_sentence_id,language,canonical_lemma,upos,notes,created_at`, owner, example, lang, lemma, upos, notes).Scan(&v.ID, &v.OwnerID, &v.ExampleSentenceID, &v.Language, &v.CanonicalLemma, &v.UPOS, &v.Notes, &v.CreatedAt)
	return
}

// ListReviewSentences returns the persisted sentence choices for an owner's
// vocabulary identity. The initially selected sentence is first, followed by
// alternatives in deterministic rank order.
func (s *PostgresStore) ListReviewSentences(ctx context.Context, owner, lang, lemma, upos string) ([]domain.ExampleSentence, error) {
	rows, err := s.pool.Query(ctx, `WITH latest AS (SELECT corpus_id FROM example_sentences WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4 GROUP BY corpus_id ORDER BY max(created_at) DESC,corpus_id DESC LIMIT 1) SELECT id,owner_id,corpus_id,sentence_key,sentence_text,source_location,language,canonical_lemma,upos,selection_rank,selection_score,selection_reasons,is_chosen,created_at FROM example_sentences WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4 AND corpus_id=(SELECT corpus_id FROM latest) ORDER BY is_chosen DESC,selection_rank,id`, owner, lang, lemma, upos)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ExampleSentence
	for rows.Next() {
		var example domain.ExampleSentence
		if err := rows.Scan(&example.ID, &example.OwnerID, &example.CorpusID, &example.SentenceKey, &example.Text, &example.SourceLocation, &example.Language, &example.CanonicalLemma, &example.UPOS, &example.SelectionRank, &example.SelectionScore, &example.SelectionReasons, &example.Chosen, &example.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, example)
	}
	return out, rows.Err()
}

// ListReviewSentencesForBook binds curation to the source material currently
// being reviewed, even when the same vocabulary identity occurs in other books.
func (s *PostgresStore) ListReviewSentencesForBook(ctx context.Context, owner, bookID, lang, lemma, upos string) ([]domain.ExampleSentence, error) {
	rows, err := s.pool.Query(ctx, `SELECT e.id,e.owner_id,e.corpus_id,e.sentence_key,e.sentence_text,e.source_location,e.language,e.canonical_lemma,e.upos,e.selection_rank,e.selection_score,e.selection_reasons,e.is_chosen,e.created_at FROM example_sentences e JOIN corpora c ON c.owner_id=e.owner_id AND c.id=e.corpus_id WHERE e.owner_id=$1 AND c.source_material_id=$2 AND e.language=$3 AND e.canonical_lemma=$4 AND e.upos=$5 ORDER BY e.is_chosen DESC,e.selection_rank,e.id`, owner, bookID, lang, lemma, upos)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ExampleSentence
	for rows.Next() {
		var example domain.ExampleSentence
		if err := rows.Scan(&example.ID, &example.OwnerID, &example.CorpusID, &example.SentenceKey, &example.Text, &example.SourceLocation, &example.Language, &example.CanonicalLemma, &example.UPOS, &example.SelectionRank, &example.SelectionScore, &example.SelectionReasons, &example.Chosen, &example.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, example)
	}
	return out, rows.Err()
}

// PersistReviewSentenceFromAnalysis materializes the first sentence retained by
// selection so review can curate it when sentence ranking persisted no choices.
func (s *PostgresStore) PersistReviewSentenceFromAnalysis(ctx context.Context, owner, bookID, lang, lemma, upos string) (v domain.ExampleSentence, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer tx.Rollback(ctx)
	var corpusID string
	var refs []byte
	query := `SELECT sc.corpus_id,sc.eligible_sentence_refs FROM selection_candidates sc JOIN corpora c ON c.owner_id=sc.owner_id AND c.id::text=sc.corpus_id WHERE sc.owner_id=$1 AND sc.language=$2 AND sc.canonical_lemma=$3 AND sc.upos=$4`
	args := []any{owner, lang, lemma, upos}
	if strings.TrimSpace(bookID) != "" {
		query += ` AND c.source_material_id=$5`
		args = append(args, bookID)
	}
	query += ` ORDER BY sc.selected_at DESC,sc.corpus_id DESC LIMIT 1`
	if err = tx.QueryRow(ctx, query, args...).Scan(&corpusID, &refs); err != nil {
		return v, missing(err)
	}
	var candidates []struct {
		SentenceIndex int             `json:"sentence_index"`
		Text          string          `json:"text"`
		Location      json.RawMessage `json:"location"`
	}
	if err = json.Unmarshal(refs, &candidates); err != nil {
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
		err = tx.QueryRow(ctx, `INSERT INTO example_sentences(owner_id,corpus_id,sentence_key,sentence_text,source_location,language,canonical_lemma,upos,selection_rank,selection_score,selection_reasons,is_chosen) VALUES($1,$2,$3,$4,$5,$6,$7,$8,1,0,'[]',true) ON CONFLICT(owner_id,corpus_id,sentence_key) DO UPDATE SET sentence_text=excluded.sentence_text,source_location=excluded.source_location,language=excluded.language,canonical_lemma=excluded.canonical_lemma,upos=excluded.upos,selection_rank=excluded.selection_rank,selection_score=excluded.selection_score,selection_reasons=excluded.selection_reasons,is_chosen=true RETURNING id,owner_id,corpus_id,sentence_key,sentence_text,source_location,language,canonical_lemma,upos,selection_rank,selection_score,selection_reasons,is_chosen,created_at`, owner, corpusID, sentenceKey, candidate.Text, location, lang, lemma, upos).Scan(&v.ID, &v.OwnerID, &v.CorpusID, &v.SentenceKey, &v.Text, &v.SourceLocation, &v.Language, &v.CanonicalLemma, &v.UPOS, &v.SelectionRank, &v.SelectionScore, &v.SelectionReasons, &v.Chosen, &v.CreatedAt)
		if err != nil {
			return v, err
		}
		if err = tx.Commit(ctx); err != nil {
			return v, err
		}
		return v, nil
	}
	return v, ErrNotFound
}

// CurateReviewSentence atomically applies an optional owner-scoped text edit,
// records the curated choice, and appends its audit provenance.
func (s *PostgresStore) CurateReviewSentence(ctx context.Context, owner, exampleID, lang, lemma, upos, editedText, notes string, history domain.ProcessingHistory) (v domain.CuratedSentence, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM example_sentences WHERE id=$1 AND owner_id=$2 AND language=$3 AND canonical_lemma=$4 AND upos=$5)`, exampleID, owner, lang, lemma, upos).Scan(&exists); err != nil {
		return v, err
	}
	if !exists {
		return v, ErrNotFound
	}
	if editedText != "" {
		if _, err = tx.Exec(ctx, `UPDATE example_sentences SET sentence_text=$1 WHERE id=$2 AND owner_id=$3`, editedText, exampleID, owner); err != nil {
			return v, err
		}
	}
	err = tx.QueryRow(ctx, `INSERT INTO curated_sentences(owner_id,example_sentence_id,language,canonical_lemma,upos,notes) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(owner_id,language,canonical_lemma,upos) DO UPDATE SET example_sentence_id=excluded.example_sentence_id,notes=excluded.notes,created_at=now() RETURNING id,owner_id,example_sentence_id,language,canonical_lemma,upos,notes,created_at`, owner, exampleID, lang, lemma, upos, notes).Scan(&v.ID, &v.OwnerID, &v.ExampleSentenceID, &v.Language, &v.CanonicalLemma, &v.UPOS, &v.Notes, &v.CreatedAt)
	if err != nil {
		return v, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,operation,status,details,completed_at) VALUES($1,$2,$3,$4,$5)`, history.OwnerID, history.Operation, history.Status, history.Details, history.CompletedAt)
	if err != nil {
		return v, err
	}
	err = tx.Commit(ctx)
	return v, err
}
func (s *PostgresStore) PutDeck(ctx context.Context, owner, lang, name string) (v domain.Deck, err error) {
	err = s.pool.QueryRow(ctx, `INSERT INTO decks(owner_id,language,name) VALUES($1,$2,$3) ON CONFLICT(owner_id,language,name) DO UPDATE SET name=excluded.name RETURNING id,owner_id,language,name,created_at`, owner, lang, name).Scan(&v.ID, &v.OwnerID, &v.Language, &v.Name, &v.CreatedAt)
	return
}
func (s *PostgresStore) PutCard(ctx context.Context, v domain.Card) (out domain.Card, err error) {
	err = s.pool.QueryRow(ctx, `INSERT INTO cards(owner_id,deck_id,dedup_key,canonical_lemma,upos,front,back) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(owner_id,dedup_key) DO UPDATE SET front=excluded.front,back=excluded.back RETURNING id,owner_id,deck_id,dedup_key,canonical_lemma,upos,front,back,created_at`, v.OwnerID, v.DeckID, v.DedupKey, v.CanonicalLemma, v.UPOS, v.Front, v.Back).Scan(&out.ID, &out.OwnerID, &out.DeckID, &out.DedupKey, &out.CanonicalLemma, &out.UPOS, &out.Front, &out.Back, &out.CreatedAt)
	return
}
func (s *PostgresStore) PutProcessingHistory(ctx context.Context, v domain.ProcessingHistory) (out domain.ProcessingHistory, err error) {
	var corpus any = v.CorpusID
	if v.CorpusID == "" {
		corpus = nil
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO processing_history(owner_id,corpus_id,operation,status,details,completed_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,owner_id,COALESCE(corpus_id::text,''),operation,status,details,started_at,completed_at`, v.OwnerID, corpus, v.Operation, v.Status, v.Details, v.CompletedAt).Scan(&out.ID, &out.OwnerID, &out.CorpusID, &out.Operation, &out.Status, &out.Details, &out.StartedAt, &out.CompletedAt)
	return
}

// PutVocabularyTransition persists a lifecycle state and its audit record atomically.
// Transition validation remains the responsibility of the vocabulary domain service.
func (s *PostgresStore) PutVocabularyTransition(ctx context.Context, owner, lang, lemma, upos, state string, history domain.ProcessingHistory) (v domain.VocabularyState, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner_id,language,canonical_lemma,upos) DO UPDATE SET state=excluded.state,updated_at=now() RETURNING id,owner_id,language,canonical_lemma,upos,state,updated_at`, owner, lang, lemma, upos, state).Scan(&v.ID, &v.OwnerID, &v.Language, &v.CanonicalLemma, &v.UPOS, &v.State, &v.UpdatedAt)
	if err != nil {
		return v, err
	}
	var corpus any = history.CorpusID
	if history.CorpusID == "" {
		corpus = nil
	}
	_, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,corpus_id,operation,status,details,completed_at) VALUES($1,$2,$3,$4,$5,$6)`, history.OwnerID, corpus, history.Operation, history.Status, history.Details, history.CompletedAt)
	if err != nil {
		return v, err
	}
	err = tx.Commit(ctx)
	return v, err
}
