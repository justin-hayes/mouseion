// Package persistence provides PostgreSQL repositories with explicit ownership boundaries.
package persistence

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/migrations"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

var ErrNotFound = errors.New("persistence: not found")
var ErrSecretRequired = errors.New("persistence: MOUSEION_SECRET is required for OPDS credentials")

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

// PutSelectionCandidate atomically respects suppressing lifecycle states,
// creates an initial candidate state, and records corpus-specific provenance.
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
	if err == nil && (state == "known" || state == "ignored" || state == "generated") {
		return false, nil
	}
	var known bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM known_vocabulary WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4)`, candidate.OwnerID, candidate.Language, candidate.CanonicalLemma, candidate.UPOS).Scan(&known); err != nil {
		return false, err
	}
	if known {
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

// PutRankingComponents records an explainable score only on the selected
// candidate owned by owner. The full identity prevents cross-language updates.
func (s *PostgresStore) PutRankingComponents(ctx context.Context, owner, corpusID, language, lemma, upos string, c domain.RankingComponents) error {
	tag, err := s.pool.Exec(ctx, `UPDATE selection_candidates SET ranking_global_pct=$6,ranking_corpus_pct=$7,ranking_priority=$8,ranking_cross_text=$9,ranking_score=$10,ranked_at=now() WHERE owner_id=$1 AND corpus_id=$2 AND language=$3 AND canonical_lemma=$4 AND upos=$5`, owner, corpusID, language, lemma, upos, c.GlobalPercentile, c.CorpusPercentile, c.Priority, c.CrossText, c.Score)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
func missing(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Passwords are encrypted with AES-256-GCM. The key is derived from the
// deployment's MOUSEION_SECRET; changing it makes existing credentials unreadable.
func credentialAEAD() (cipher.AEAD, error) {
	secret := os.Getenv("MOUSEION_SECRET")
	if secret == "" {
		return nil, ErrSecretRequired
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
	err = row.Scan(&v.ID, &v.OwnerID, &v.Name, &v.URL, &v.Username, &encrypted, &v.Language, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return v, missing(err)
	}
	v.Password, err = decryptCredential(encrypted)
	return v, err
}

func (s *PostgresStore) CreateOpdsConnection(ctx context.Context, v domain.OpdsConnection) (domain.OpdsConnection, error) {
	encrypted, err := encryptCredential(v.Password)
	if err != nil {
		return domain.OpdsConnection{}, err
	}
	return scanOpds(s.pool.QueryRow(ctx, `INSERT INTO opds_connections(owner_id,name,url,username,password_encrypted,language) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+opdsColumns, v.OwnerID, v.Name, v.URL, v.Username, encrypted, v.Language))
}
func (s *PostgresStore) GetOpdsConnection(ctx context.Context, owner, id string) (domain.OpdsConnection, error) {
	return scanOpds(s.pool.QueryRow(ctx, `SELECT `+opdsColumns+` FROM opds_connections WHERE owner_id=$1 AND id=$2`, owner, id))
}
func (s *PostgresStore) ListOpdsConnections(ctx context.Context, owner string) ([]domain.OpdsConnection, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+opdsColumns+` FROM opds_connections WHERE owner_id=$1 ORDER BY name,id`, owner)
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
func (s *PostgresStore) UpdateOpdsConnection(ctx context.Context, owner string, v domain.OpdsConnection) (domain.OpdsConnection, error) {
	encrypted, err := encryptCredential(v.Password)
	if err != nil {
		return domain.OpdsConnection{}, err
	}
	return scanOpds(s.pool.QueryRow(ctx, `UPDATE opds_connections SET name=$3,url=$4,username=$5,password_encrypted=$6,language=$7,updated_at=now() WHERE owner_id=$1 AND id=$2 RETURNING `+opdsColumns, owner, v.ID, v.Name, v.URL, v.Username, encrypted, v.Language))
}
func (s *PostgresStore) DeleteOpdsConnection(ctx context.Context, owner, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM opds_connections WHERE owner_id=$1 AND id=$2`, owner, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *PostgresStore) CreateUser(ctx context.Context, username string, admin bool) (u domain.User, err error) {
	err = s.pool.QueryRow(ctx, `INSERT INTO users(username,is_admin) VALUES($1,$2) RETURNING id,username,is_admin,created_at`, username, admin).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt)
	return
}
func (s *PostgresStore) CreateUserWithPassword(ctx context.Context, username, passwordHash string, admin bool) (u domain.User, err error) {
	err = s.pool.QueryRow(ctx, `INSERT INTO users(username,password_hash,is_admin) VALUES($1,$2,$3) RETURNING id,username,is_admin,created_at`, username, passwordHash, admin).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt)
	return
}
func (s *PostgresStore) GetUserByUsername(ctx context.Context, username string) (u domain.User, passwordHash string, err error) {
	err = s.pool.QueryRow(ctx, `SELECT id,username,is_admin,created_at,COALESCE(password_hash,'') FROM users WHERE username=$1`, username).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt, &passwordHash)
	err = missing(err)
	return
}
func (s *PostgresStore) GetUserByID(ctx context.Context, id string) (u domain.User, err error) {
	err = s.pool.QueryRow(ctx, `SELECT id,username,is_admin,created_at FROM users WHERE id=$1`, id).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt)
	err = missing(err)
	return
}
func (s *PostgresStore) SetUserPassword(ctx context.Context, userID, passwordHash string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1`, userID, passwordHash)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
func (s *PostgresStore) BootstrapAdmin(ctx context.Context, username, passwordHash string) (u domain.User, created bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return u, false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(110011)`); err != nil {
		return u, false, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE is_admin)`).Scan(&exists); err != nil {
		return u, false, err
	}
	if exists {
		return u, false, nil
	}
	err = tx.QueryRow(ctx, `INSERT INTO users(username,password_hash,is_admin) VALUES($1,$2,true) RETURNING id,username,is_admin,created_at`, username, passwordHash).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		return u, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return u, false, err
	}
	return u, true, nil
}
func (s *PostgresStore) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sessions(user_id,token_hash,expires_at) VALUES($1,$2,$3)`, userID, tokenHash, expiresAt)
	return err
}
func (s *PostgresStore) GetSession(ctx context.Context, tokenHash string) (u domain.User, expiresAt time.Time, err error) {
	err = s.pool.QueryRow(ctx, `SELECT u.id,u.username,u.is_admin,u.created_at,s.expires_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()`, tokenHash).Scan(&u.ID, &u.Username, &u.IsAdmin, &u.CreatedAt, &expiresAt)
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
func (s *PostgresStore) PutLanguageProfile(ctx context.Context, owner, language, name string) (p domain.LanguageProfile, err error) {
	err = s.pool.QueryRow(ctx, `INSERT INTO language_profiles(owner_id,language,display_name) VALUES($1,$2,$3) ON CONFLICT(owner_id,language) DO UPDATE SET display_name=excluded.display_name RETURNING id,owner_id,language,display_name,created_at`, owner, language, name).Scan(&p.ID, &p.OwnerID, &p.Language, &p.DisplayName, &p.CreatedAt)
	return
}

func (s *PostgresStore) ListLanguageProfiles(ctx context.Context, owner string) ([]domain.LanguageProfile, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,owner_id,language,display_name,created_at FROM language_profiles WHERE owner_id=$1 ORDER BY display_name,language`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.LanguageProfile
	for rows.Next() {
		var p domain.LanguageProfile
		if err := rows.Scan(&p.ID, &p.OwnerID, &p.Language, &p.DisplayName, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostgresStore) ListAnalysisJobs(ctx context.Context, owner string) ([]domain.AnalysisJob, error) {
	rows, err := s.pool.Query(ctx, `SELECT river_job_id,owner_id,source_material_id,content_hash,COALESCE(corpus_id::text,''),progress,error,created_at,updated_at FROM analysis_jobs WHERE owner_id=$1 ORDER BY created_at DESC LIMIT 100`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AnalysisJob
	for rows.Next() {
		var job domain.AnalysisJob
		if err := rows.Scan(&job.ID, &job.OwnerID, &job.SourceMaterialID, &job.ContentHash, &job.CorpusID, &job.Progress, &job.Error, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}
func (s *PostgresStore) PutSourceMaterial(ctx context.Context, v domain.SourceMaterial) (out domain.SourceMaterial, err error) {
	err = s.pool.QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(owner_id,source_identifier) DO UPDATE SET title=excluded.title,media_type=excluded.media_type,content_hash=excluded.content_hash,content=excluded.content,full_text=excluded.full_text RETURNING id,owner_id,language,source_identifier,title,media_type,content_hash,content,full_text,created_at`, v.OwnerID, v.Language, v.SourceIdentifier, v.Title, v.MediaType, v.ContentHash, v.Content, v.FullText).Scan(&out.ID, &out.OwnerID, &out.Language, &out.SourceIdentifier, &out.Title, &out.MediaType, &out.ContentHash, &out.Content, &out.FullText, &out.CreatedAt)
	return
}
func (s *PostgresStore) GetSourceMaterial(ctx context.Context, owner, id string) (v domain.SourceMaterial, err error) {
	err = s.pool.QueryRow(ctx, `SELECT id,owner_id,language,source_identifier,title,media_type,content_hash,content,full_text,created_at FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, id).Scan(&v.ID, &v.OwnerID, &v.Language, &v.SourceIdentifier, &v.Title, &v.MediaType, &v.ContentHash, &v.Content, &v.FullText, &v.CreatedAt)
	err = missing(err)
	return
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
	err = s.pool.QueryRow(ctx, `INSERT INTO corpora(owner_id,source_material_id,artifact_hash) VALUES($1,$2,$3) ON CONFLICT(owner_id,source_material_id) DO UPDATE SET artifact_hash=excluded.artifact_hash RETURNING id,owner_id,source_material_id,artifact_hash,status,created_at`, owner, sourceID, hash).Scan(&v.ID, &v.OwnerID, &v.SourceMaterialID, &v.ArtifactHash, &v.Status, &v.CreatedAt)
	return
}
func (s *PostgresStore) GetCorpus(ctx context.Context, owner, id string) (v domain.Corpus, err error) {
	err = s.pool.QueryRow(ctx, `SELECT id,owner_id,source_material_id,artifact_hash,status,created_at FROM corpora WHERE owner_id=$1 AND id=$2`, owner, id).Scan(&v.ID, &v.OwnerID, &v.SourceMaterialID, &v.ArtifactHash, &v.Status, &v.CreatedAt)
	err = missing(err)
	return
}
func (s *PostgresStore) PutKnownVocabulary(ctx context.Context, owner, lang, lemma, upos string) (v domain.KnownVocabulary, err error) {
	err = s.pool.QueryRow(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos) VALUES($1,$2,$3,$4) ON CONFLICT(owner_id,language,canonical_lemma,upos) DO UPDATE SET canonical_lemma=excluded.canonical_lemma RETURNING id,owner_id,language,canonical_lemma,upos,created_at`, owner, lang, lemma, upos).Scan(&v.ID, &v.OwnerID, &v.Language, &v.CanonicalLemma, &v.UPOS, &v.CreatedAt)
	return
}
func (s *PostgresStore) GetKnownVocabulary(ctx context.Context, owner, id string) (v domain.KnownVocabulary, err error) {
	err = s.pool.QueryRow(ctx, `SELECT id,owner_id,language,canonical_lemma,upos,created_at FROM known_vocabulary WHERE owner_id=$1 AND id=$2`, owner, id).Scan(&v.ID, &v.OwnerID, &v.Language, &v.CanonicalLemma, &v.UPOS, &v.CreatedAt)
	err = missing(err)
	return
}
func (s *PostgresStore) IsKnownVocabularyIdentity(ctx context.Context, owner, lang, lemma, upos string) (bool, error) {
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
