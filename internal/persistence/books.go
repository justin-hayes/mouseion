package persistence

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const bookColumns = `id::text,owner_id::text,title,metadata_provenance,language_state,COALESCE(language_tag,''),created_at,updated_at`
const qualifiedBookColumns = `b.id::text,b.owner_id::text,b.title,b.metadata_provenance,b.language_state,COALESCE(b.language_tag,''),b.created_at,b.updated_at`

const myBooksEvidenceSelect = `SELECT ` + qualifiedBookColumns + `,
       COALESCE(s.id::text,''),COALESCE(s.owner_id::text,''),COALESCE(s.language,''),COALESCE(s.source_identifier,''),COALESCE(s.title,''),COALESCE(s.media_type,''),
       COALESCE(CASE WHEN r.digest_version=1 THEN r.content_digest ELSE s.content_hash END,''),COALESCE(r.content_digest,''),COALESCE(r.revision_id::text,''),COALESCE(r.digest_version,0),s.created_at,
       s.id IS NOT NULL,
       CASE WHEN s.id IS NULL THEN 'not_acquired'
            WHEN s.current_content_revision_id IS NULL OR s.current_snapshot_id IS NULL THEN 'unavailable'
            WHEN p.source_material_id IS NOT NULL AND ca.analysis_run_id IS NULL THEN 'stale'
            WHEN ca.analysis_run_id IS NOT NULL THEN 'analyzed'
            ELSE 'acquired_unassessed' END,
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
	       COALESCE(ca.analysis_run_id::text,''),COALESCE(ca.corpus_id::text,''),COALESCE(j.river_job_id,0)`

const myBooksEvidenceFrom = `
FROM books b
JOIN book_membership m ON m.owner_id=b.owner_id AND m.book_id=b.id AND m.state='active'
LEFT JOIN book_current_analyses p ON p.owner_id=b.owner_id AND p.book_id=b.id
LEFT JOIN current_analysis ca ON ca.owner_id=b.owner_id AND ca.book_id=b.id
LEFT JOIN LATERAL (SELECT s.* FROM source_materials s WHERE s.owner_id=b.owner_id AND s.book_id=b.id ORDER BY CASE WHEN p.source_material_id IS NOT NULL AND s.id=p.source_material_id THEN 0 ELSE 1 END,s.created_at DESC,s.id DESC LIMIT 1) s ON true
LEFT JOIN source_content_revisions r ON r.owner_id=s.owner_id AND r.revision_id=s.current_content_revision_id
LEFT JOIN LATERAL (SELECT river_job_id,error,analysis_run_id FROM analysis_jobs j WHERE j.owner_id=s.owner_id AND j.source_material_id=s.id ORDER BY j.created_at DESC,j.river_job_id DESC LIMIT 1) j ON true
LEFT JOIN analysis_runs ar ON ar.owner_id=s.owner_id AND ar.id=j.analysis_run_id
	`

// LanguageCount is one owner-scoped language pill count. Tag is "unknown"
// for books whose language state is unknown.
type LanguageCount struct {
	Tag   string
	Count int
}

type MyBooksBrowseResult struct {
	Items      []domain.MyBook
	ScopeTotal int
	Total      int
	Counts     []LanguageCount
	AllCount   int
}

func scanBook(row pgx.Row) (domain.Book, error) {
	var b domain.Book
	err := row.Scan(&b.ID, &b.OwnerID, &b.Title, &b.MetadataProvenance, &b.LanguageState, &b.LanguageTag, &b.CreatedAt, &b.UpdatedAt)
	return b, missing(err)
}

func (s *PostgresStore) ListMyBooks(ctx context.Context, owner string) ([]domain.Book, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+qualifiedBookColumns+` FROM books b JOIN book_membership m ON m.owner_id=b.owner_id AND m.book_id=b.id WHERE b.owner_id=$1 AND m.state='active' ORDER BY b.title,b.id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var books []domain.Book
	for rows.Next() {
		var b domain.Book
		if err := rows.Scan(&b.ID, &b.OwnerID, &b.Title, &b.MetadataProvenance, &b.LanguageState, &b.LanguageTag, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	return books, rows.Err()
}

// ListStudyLanguages derives the learner's study languages from active Books
// with a chosen canonical language. Reference names are optional so newly
// discovered tags remain importable.
func (s *PostgresStore) ListStudyLanguages(ctx context.Context, owner string) ([]domain.StudyLanguage, error) {
	rows, err := s.pool.Query(ctx, `WITH chosen_languages AS (
		SELECT DISTINCT b.language_tag AS language
		FROM books b
		JOIN book_membership m ON m.owner_id=b.owner_id AND m.book_id=b.id AND m.state='active'
		WHERE b.owner_id=$1 AND b.language_state='chosen' AND b.language_tag <> ''
	)
	SELECT c.language, COALESCE(NULLIF(s.display_name,''), c.language)
	FROM chosen_languages c
	LEFT JOIN supported_languages s ON s.language=c.language
	ORDER BY COALESCE(NULLIF(s.display_name,''), c.language), c.language`, owner)
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

// ListMyBooksWithEvidence is the My Books collection read model. Membership
// is the driving table, so metadata-only Books remain visible; the current
// acquired source and analysis projection are optional evidence on each row.
func (s *PostgresStore) ListMyBooksWithEvidence(ctx context.Context, owner string) ([]domain.MyBook, error) {
	rows, err := s.pool.Query(ctx, currentAnalysisCTE+myBooksEvidenceSelect+myBooksEvidenceFrom+`
	WHERE b.owner_id=$1
	ORDER BY b.title,b.id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMyBookRows(rows)
}

// ListMyBooksBrowse returns one owner-scoped page of the active My Books
// collection and counts for its unfiltered language pills. query is trimmed
// and lowercased, then matched as a case-insensitive literal substring of the
// locally stored title; backslash, percent, and underscore are escaped before
// the SQL LIKE expression. It does not tokenize, stem, or query OPDS. language
// is empty for all languages, "unknown" for the unknown bucket, or otherwise
// matches a canonical chosen tag. Items are ordered deterministically
// by lower(title), title, and id, and offset/limit select the requested page.
func (s *PostgresStore) ListMyBooksBrowse(ctx context.Context, owner, query, language string, offset, limit int) (MyBooksBrowseResult, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	language = strings.TrimSpace(language)
	if language != domain.LanguageUnknown {
		language = canonicalization.NormalizeLanguage(language)
	}
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}

	where, args := myBooksBrowseWhere(owner, query, language)
	pageArgs := append(append([]any(nil), args...), limit, offset)
	pageSQL := currentAnalysisCTE + myBooksEvidenceSelect + myBooksEvidenceFrom + `
	WHERE ` + where + fmt.Sprintf(`
	ORDER BY lower(b.title),b.title,b.id
	LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	rows, err := s.pool.Query(ctx, pageSQL, pageArgs...)
	if err != nil {
		return MyBooksBrowseResult{}, err
	}
	items, err := scanMyBookRows(rows)
	if err != nil {
		return MyBooksBrowseResult{}, err
	}

	var result MyBooksBrowseResult
	result.Items = items
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM books b JOIN book_membership m ON m.owner_id=b.owner_id AND m.book_id=b.id AND m.state='active' WHERE `+where, args...).Scan(&result.Total); err != nil {
		return MyBooksBrowseResult{}, err
	}
	scopeWhere, scopeArgs := myBooksBrowseWhere(owner, "", language)
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM books b JOIN book_membership m ON m.owner_id=b.owner_id AND m.book_id=b.id AND m.state='active' WHERE `+scopeWhere, scopeArgs...).Scan(&result.ScopeTotal); err != nil {
		return MyBooksBrowseResult{}, err
	}
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM books b JOIN book_membership m ON m.owner_id=b.owner_id AND m.book_id=b.id AND m.state='active' WHERE b.owner_id=$1`, owner).Scan(&result.AllCount); err != nil {
		return MyBooksBrowseResult{}, err
	}
	counts, err := s.pool.Query(ctx, `SELECT CASE WHEN b.language_state='unknown' THEN 'unknown' ELSE b.language_tag END, count(*) FROM books b JOIN book_membership m ON m.owner_id=b.owner_id AND m.book_id=b.id AND m.state='active' WHERE b.owner_id=$1 AND b.language_state IN ('chosen','unknown') GROUP BY 1`, owner)
	if err != nil {
		return MyBooksBrowseResult{}, err
	}
	for counts.Next() {
		var item LanguageCount
		if err = counts.Scan(&item.Tag, &item.Count); err != nil {
			counts.Close()
			return MyBooksBrowseResult{}, err
		}
		result.Counts = append(result.Counts, item)
	}
	if err = counts.Err(); err != nil {
		counts.Close()
		return MyBooksBrowseResult{}, err
	}
	counts.Close()
	sort.Slice(result.Counts, func(i, j int) bool {
		if result.Counts[i].Tag == "unknown" {
			return false
		}
		if result.Counts[j].Tag == "unknown" {
			return true
		}
		return result.Counts[i].Tag < result.Counts[j].Tag
	})
	return result, nil
}

func myBooksBrowseWhere(owner, query, language string) (string, []any) {
	conditions := []string{"b.owner_id=$1", "m.state='active'"}
	args := []any{owner}
	if query != "" {
		args = append(args, escapeLikePattern(query))
		conditions = append(conditions, fmt.Sprintf(`lower(b.title) LIKE '%%' || $%d || '%%' ESCAPE '\'`, len(args)))
	}
	if language != "" {
		if language == domain.LanguageUnknown {
			conditions = append(conditions, "b.language_state='unknown'")
		} else {
			args = append(args, language)
			conditions = append(conditions, fmt.Sprintf("b.language_state='chosen' AND b.language_tag=$%d", len(args)))
		}
	}
	return strings.Join(conditions, " AND "), args
}

func escapeLikePattern(value string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(value)
}

func scanMyBookRows(rows pgx.Rows) ([]domain.MyBook, error) {
	defer rows.Close()
	var out []domain.MyBook
	for rows.Next() {
		item, err := scanMyBookRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

type myBookRowScanner interface {
	Scan(...any) error
}

func scanMyBookRow(row myBookRowScanner) (domain.MyBook, error) {
	var item domain.MyBook
	var sourceID, sourceOwner, sourceLanguage, sourceIdentifier, sourceTitle, sourceMediaType string
	var sourceContentHash, sourceDigest, sourceRevisionID string
	var sourceCreatedAt *time.Time
	var sourceExists bool
	var evidenceState domain.MyBookEvidenceState
	var analysisStatus, analysisState, analysisRunID, corpusID string
	var analysisJobID int64
	var digestVersion int
	if err := row.Scan(&item.Book.ID, &item.Book.OwnerID, &item.Book.Title, &item.Book.MetadataProvenance, &item.Book.LanguageState, &item.Book.LanguageTag, &item.Book.CreatedAt, &item.Book.UpdatedAt,
		&sourceID, &sourceOwner, &sourceLanguage, &sourceIdentifier, &sourceTitle, &sourceMediaType, &sourceContentHash, &sourceDigest, &sourceRevisionID, &digestVersion, &sourceCreatedAt, &sourceExists,
		&evidenceState, &analysisStatus, &analysisState, &analysisRunID, &corpusID, &analysisJobID); err != nil {
		return domain.MyBook{}, missing(err)
	}
	item.EvidenceState = evidenceState
	if sourceExists {
		createdAt := time.Time{}
		if sourceCreatedAt != nil {
			createdAt = *sourceCreatedAt
		}
		item.Acquired = &domain.SourceMaterialSummary{
			Source:         domain.SourceMaterial{ID: sourceID, OwnerID: sourceOwner, Language: sourceLanguage, SourceIdentifier: sourceIdentifier, Title: sourceTitle, MediaType: sourceMediaType, ContentHash: sourceContentHash, ContentDigest: sourceDigest, ContentRevisionID: sourceRevisionID, ContentDigestVersion: digestVersion, CreatedAt: createdAt},
			BookID:         item.Book.ID,
			AnalysisStatus: analysisStatus, AnalysisState: analysisState, AnalysisRunID: analysisRunID, CorpusID: corpusID, AnalysisJobID: analysisJobID,
		}
	}
	return item, nil
}

// GetBookDetail resolves either the canonical Book ID or a historical source
// material ID within the owner's active My Books membership.
func (s *PostgresStore) GetBookDetail(ctx context.Context, owner, id string) (domain.MyBook, error) {
	return scanMyBookRow(s.pool.QueryRow(ctx, currentAnalysisCTE+myBooksEvidenceSelect+myBooksEvidenceFrom+`
	WHERE b.owner_id=$1 AND (b.id::text=$2 OR EXISTS (
		SELECT 1 FROM source_materials requested_source
		WHERE requested_source.owner_id=b.owner_id AND requested_source.book_id=b.id AND requested_source.id::text=$2
	))
	ORDER BY CASE WHEN b.id::text=$2 THEN 0 ELSE 1 END
	LIMIT 1`, owner, id))
}

func (s *PostgresStore) GetBook(ctx context.Context, owner, bookID string) (domain.Book, error) {
	return scanBook(s.pool.QueryRow(ctx, `SELECT `+bookColumns+` FROM books WHERE owner_id=$1 AND id=$2`, owner, bookID))
}

// GetBookCatalogEntryAlias returns the owner's recorded catalogue identity for
// an active Book. Missing and cross-owner identities are deliberately the same
// not-found result.
func (s *PostgresStore) GetBookCatalogEntryAlias(ctx context.Context, owner, bookID string) (domain.BookAlias, error) {
	var alias domain.BookAlias
	var connectionID *string
	err := s.pool.QueryRow(ctx, `SELECT a.id::text,a.owner_id::text,a.book_id::text,a.connection_id::text,a.alias_type,a.namespace,a.value,a.created_at
		FROM book_aliases a
		JOIN books b ON b.owner_id=a.owner_id AND b.id=a.book_id
		JOIN book_membership m ON m.owner_id=a.owner_id AND m.book_id=a.book_id AND m.state='active'
		WHERE a.owner_id=$1 AND a.book_id=$2 AND a.alias_type=$3 AND a.namespace=$4`, owner, bookID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier).
		Scan(&alias.ID, &alias.OwnerID, &alias.BookID, &connectionID, &alias.AliasType, &alias.Namespace, &alias.Value, &alias.CreatedAt)
	if connectionID != nil {
		alias.ConnectionID = *connectionID
	}
	return alias, missing(err)
}

// ListUnscopedCatalogueEntryAliases returns only legacy catalogue-entry aliases
// that still need the application-logic connection backfill. Strong
// bibliographic aliases are intentionally excluded.
func (s *PostgresStore) ListUnscopedCatalogueEntryAliases(ctx context.Context) ([]domain.BookAlias, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text,owner_id::text,book_id::text,connection_id::text,alias_type,namespace,value,created_at
		FROM book_aliases
		WHERE connection_id IS NULL AND alias_type=$1 AND namespace=$2
		ORDER BY owner_id,id`, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var aliases []domain.BookAlias
	for rows.Next() {
		var alias domain.BookAlias
		var connectionID *string
		if err := rows.Scan(&alias.ID, &alias.OwnerID, &alias.BookID, &connectionID, &alias.AliasType, &alias.Namespace, &alias.Value, &alias.CreatedAt); err != nil {
			return nil, err
		}
		if connectionID != nil {
			alias.ConnectionID = *connectionID
		}
		aliases = append(aliases, alias)
	}
	return aliases, rows.Err()
}

// SetCatalogueEntryAliasConnection assigns a legacy alias only while it is
// unscoped and only to a connection owned by the same learner.
func (s *PostgresStore) SetCatalogueEntryAliasConnection(ctx context.Context, owner, aliasID, connectionID string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE book_aliases a SET connection_id=$3
		WHERE a.owner_id=$1 AND a.id=$2 AND a.connection_id IS NULL
		  AND a.alias_type=$4 AND a.namespace=$5
		  AND EXISTS (SELECT 1 FROM opds_connections c WHERE c.owner_id=$1 AND c.id=$3)`, owner, aliasID, connectionID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier)
	if err != nil {
		return aliasConflictError(err)
	}
	if tag.RowsAffected() == 0 {
		var current *string
		err = s.pool.QueryRow(ctx, `SELECT connection_id::text FROM book_aliases WHERE owner_id=$1 AND id=$2`, owner, aliasID).Scan(&current)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if current != nil && *current == connectionID {
			return nil
		}
		return ErrAliasConflict
	}
	return nil
}

func (s *PostgresStore) CreateBook(ctx context.Context, b domain.Book) (domain.Book, error) {
	b.Title = strings.TrimSpace(b.Title)
	b.LanguageTag = strings.TrimSpace(b.LanguageTag)
	if err := b.Validate(); err != nil {
		return domain.Book{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Book{}, err
	}
	defer tx.Rollback(ctx)
	created, err := scanBook(tx.QueryRow(ctx, `INSERT INTO books(owner_id,title,metadata_provenance,language_state,language_tag) VALUES($1,$2,$3,$4,$5) RETURNING `+bookColumns, b.OwnerID, b.Title, b.MetadataProvenance, b.LanguageState, nullableLanguageTag(b.LanguageState, b.LanguageTag)))
	if err != nil {
		return domain.Book{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO book_membership(owner_id,book_id,state,activated_at) VALUES($1,$2,'active',now())`, created.OwnerID, created.ID); err != nil {
		return domain.Book{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Book{}, err
	}
	return created, nil
}

func nullableLanguageTag(state, tag string) any {
	if state == domain.LanguageUnknown {
		return nil
	}
	return tag
}

func (s *PostgresStore) UpdateBookMetadata(ctx context.Context, owner, bookID, title, languageState, languageTag string) (domain.Book, error) {
	current, err := s.GetBook(ctx, owner, bookID)
	if err != nil {
		return domain.Book{}, err
	}
	candidate := current
	if languageState == domain.LanguageChosen {
		languageTag = canonicalization.NormalizeLanguage(languageTag)
	}
	candidate.Title, candidate.LanguageState, candidate.LanguageTag = title, languageState, languageTag
	if err = candidate.Validate(); err != nil {
		return domain.Book{}, err
	}
	return scanBook(s.pool.QueryRow(ctx, `UPDATE books SET title=$3,language_state=$4,language_tag=$5,updated_at=now() WHERE owner_id=$1 AND id=$2 RETURNING `+bookColumns, owner, bookID, title, languageState, nullableLanguageTag(languageState, languageTag)))
}

func ensureBookExists(ctx context.Context, tx pgx.Tx, owner, bookID string) error {
	var found bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM books WHERE owner_id=$1 AND id=$2)`, owner, bookID).Scan(&found); err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

func activateMembership(ctx context.Context, tx pgx.Tx, owner, bookID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO book_membership(owner_id,book_id,state,activated_at) VALUES($1,$2,'active',now()) ON CONFLICT(owner_id,book_id) DO UPDATE SET state='active',activated_at=CASE WHEN book_membership.state='removed' THEN now() ELSE book_membership.activated_at END,removed_at=NULL`, owner, bookID)
	return err
}

func (s *PostgresStore) AddBookToMyBooks(ctx context.Context, owner, bookID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return err
	}
	if err = activateMembership(ctx, tx, owner, bookID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) RemoveBookFromMyBooks(ctx context.Context, owner, bookID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO book_membership(owner_id,book_id,state,removed_at) VALUES($1,$2,'removed',now()) ON CONFLICT(owner_id,book_id) DO UPDATE SET state='removed',removed_at=CASE WHEN book_membership.state='removed' THEN book_membership.removed_at ELSE now() END`, owner, bookID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) ResolveBookByAlias(ctx context.Context, owner, namespace, value string) (domain.Book, bool, error) {
	namespace, value = strings.TrimSpace(namespace), strings.TrimSpace(value)
	b, err := scanBook(s.pool.QueryRow(ctx, `SELECT `+qualifiedBookColumns+` FROM books b JOIN book_aliases a ON a.owner_id=b.owner_id AND a.book_id=b.id WHERE a.owner_id=$1 AND a.namespace=$2 AND a.value=$3`, owner, namespace, value))
	if errors.Is(err, ErrNotFound) {
		return domain.Book{}, false, nil
	}
	return b, err == nil, err
}

func aliasConflictError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAliasConflict
	}
	return err
}

func (s *PostgresStore) AddBookAlias(ctx context.Context, owner, bookID, aliasType, namespace, value string) error {
	alias := domain.BookAlias{OwnerID: owner, BookID: bookID, AliasType: aliasType, Namespace: strings.TrimSpace(namespace), Value: strings.TrimSpace(value)}
	if err := alias.Validate(); err != nil {
		return err
	}
	if aliasType == domain.AliasCatalogEntry {
		return errors.New("persistence: catalogue entry alias requires a connection")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return err
	}
	var inserted string
	err = tx.QueryRow(ctx, `INSERT INTO book_aliases(owner_id,book_id,alias_type,namespace,value) VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner_id,namespace,value) WHERE connection_id IS NULL DO NOTHING RETURNING id::text`, owner, bookID, aliasType, alias.Namespace, alias.Value).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingBook string
		if err = tx.QueryRow(ctx, `SELECT book_id::text FROM book_aliases WHERE owner_id=$1 AND namespace=$2 AND value=$3 AND connection_id IS NULL`, owner, alias.Namespace, alias.Value).Scan(&existingBook); err != nil {
			return err
		}
		if existingBook != bookID {
			return ErrAliasConflict
		}
		if err = activateMembership(ctx, tx, owner, bookID); err != nil {
			return err
		}
	} else if err != nil {
		return aliasConflictError(err)
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) LinkSourceToBook(ctx context.Context, owner, bookID, sourceMaterialID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return err
	}
	var current *string
	err = tx.QueryRow(ctx, `SELECT book_id::text FROM source_materials WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, sourceMaterialID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if current != nil && *current != bookID {
		return ErrSourceBookConflict
	}
	if current == nil {
		if _, err = tx.Exec(ctx, `UPDATE source_materials SET book_id=$3 WHERE owner_id=$1 AND id=$2`, owner, sourceMaterialID, bookID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) ResolveOrCreateBookForAcquisition(ctx context.Context, owner, sourceIdentifier, language, title string) (string, error) {
	return s.resolveOrCreateBookForAcquisition(ctx, owner, "", sourceIdentifier, language, title)
}

// CatalogueEntryReconcileResult reports what a metadata-only catalogue upsert
// actually changed, so connection status can distinguish "synced with changes"
// from "synced with no changes" without re-querying or implying content work.
type CatalogueEntryReconcileResult struct {
	Book            domain.Book
	Created         bool
	TitleChanged    bool
	LanguageChanged bool
}

// Upserted reports whether the entry added or updated a Book.
func (r CatalogueEntryReconcileResult) Upserted() bool {
	return r.Created || r.TitleChanged || r.LanguageChanged
}

// ReconcileCatalogueEntry performs the metadata-only, owner-scoped catalogue
// upsert. New Books are created with a chosen language only when the catalogue
// sync scope determines that language deterministically from the catalogue
// entry (never inferred). Source materials and acquired content are never
// touched here: membership and identity are metadata-only.
func (s *PostgresStore) ReconcileCatalogueEntry(ctx context.Context, owner, connectionID, sourceIdentifier, title, language string) (CatalogueEntryReconcileResult, error) {
	owner, connectionID, sourceIdentifier, title, language = strings.TrimSpace(owner), strings.TrimSpace(connectionID), strings.TrimSpace(sourceIdentifier), strings.TrimSpace(title), strings.TrimSpace(language)
	if owner == "" || connectionID == "" || sourceIdentifier == "" || title == "" || language == "" {
		return CatalogueEntryReconcileResult{}, errors.New("persistence: catalogue entry identity is incomplete")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,468))`, owner+":"+connectionID+":"+sourceIdentifier); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	var aliasBook *string
	err = tx.QueryRow(ctx, `SELECT book_id::text FROM book_aliases WHERE owner_id=$1 AND connection_id=$2 AND namespace=$3 AND value=$4 FOR UPDATE`, owner, connectionID, domain.NamespaceSourceIdentifier, sourceIdentifier).Scan(&aliasBook)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	} else if err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	bookID := ""
	if aliasBook != nil {
		bookID = *aliasBook
	}
	created := bookID == ""
	var titleChanged, languageChanged bool
	if bookID == "" {
		var createdBook domain.Book
		createdBook, err = scanBook(tx.QueryRow(ctx, `INSERT INTO books(owner_id,title,metadata_provenance,language_state,language_tag) VALUES($1,$2,$3,'chosen',$4) RETURNING `+bookColumns, owner, title, domain.MetadataProvenanceCatalogueSync, language))
		if err != nil {
			return CatalogueEntryReconcileResult{}, err
		}
		bookID = createdBook.ID
	} else {
		var currentTitle, currentLanguageState, currentLanguageTag string
		if err = tx.QueryRow(ctx, `SELECT title,language_state,COALESCE(language_tag,'') FROM books WHERE owner_id=$1 AND id=$2`, owner, bookID).Scan(&currentTitle, &currentLanguageState, &currentLanguageTag); err != nil {
			return CatalogueEntryReconcileResult{}, err
		}
		titleChanged = strings.TrimSpace(currentTitle) != title
		languageChanged = currentLanguageState != domain.LanguageChosen || currentLanguageTag != language
	}
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE books SET title=$3,language_state='chosen',language_tag=$4,updated_at=now() WHERE owner_id=$1 AND id=$2`, owner, bookID, title, language); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	if err = activateMembership(ctx, tx, owner, bookID); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	if aliasBook == nil {
		if _, err = tx.Exec(ctx, `INSERT INTO book_aliases(owner_id,book_id,connection_id,alias_type,namespace,value) VALUES($1,$2,$3,$4,$5,$6)`, owner, bookID, connectionID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, sourceIdentifier); err != nil {
			return CatalogueEntryReconcileResult{}, aliasConflictError(err)
		}
	}
	book, err := scanBook(tx.QueryRow(ctx, `SELECT `+bookColumns+` FROM books WHERE owner_id=$1 AND id=$2`, owner, bookID))
	if err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	return CatalogueEntryReconcileResult{Book: book, Created: created, TitleChanged: titleChanged, LanguageChanged: languageChanged}, nil
}

// ResolveOrCreateBookForAcquisitionForBook promotes an explicitly selected
// metadata-only Book. The Book ID is still checked in the same owner scope as
// the source identifier and alias, so a stale or cross-owner form cannot
// redirect acquired evidence.
func (s *PostgresStore) ResolveOrCreateBookForAcquisitionForBook(ctx context.Context, owner, bookID, sourceIdentifier, language, title string) (string, error) {
	return s.resolveOrCreateBookForAcquisition(ctx, owner, bookID, sourceIdentifier, language, title)
}

func (s *PostgresStore) resolveOrCreateBookForAcquisition(ctx context.Context, owner, requestedBookID, sourceIdentifier, language, title string) (string, error) {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(sourceIdentifier) == "" || strings.TrimSpace(language) == "" || strings.TrimSpace(title) == "" {
		return "", errors.New("persistence: acquisition book identity is incomplete")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,466))`, owner+":"+sourceIdentifier); err != nil {
		return "", err
	}
	if requestedBookID != "" {
		if err = ensureBookExists(ctx, tx, owner, requestedBookID); err != nil {
			return "", err
		}
		var linkedBook *string
		err = tx.QueryRow(ctx, `SELECT book_id::text FROM source_materials WHERE owner_id=$1 AND source_identifier=$2 FOR UPDATE`, owner, sourceIdentifier).Scan(&linkedBook)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		} else if err != nil {
			return "", err
		}
		if linkedBook != nil && *linkedBook != requestedBookID {
			return "", ErrSourceBookConflict
		}
		var aliasedBook *string
		err = tx.QueryRow(ctx, `SELECT book_id::text FROM book_aliases WHERE owner_id=$1 AND namespace=$2 AND value=$3 AND connection_id IS NULL FOR UPDATE`, owner, domain.NamespaceSourceIdentifier, sourceIdentifier).Scan(&aliasedBook)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		} else if err != nil {
			return "", err
		}
		if aliasedBook != nil && *aliasedBook != requestedBookID {
			return "", ErrAliasConflict
		}
		if err = activateMembership(ctx, tx, owner, requestedBookID); err != nil {
			return "", err
		}
		if err = tx.Commit(ctx); err != nil {
			return "", err
		}
		return requestedBookID, nil
	}
	var linkedBook *string
	err = tx.QueryRow(ctx, `SELECT book_id::text FROM source_materials WHERE owner_id=$1 AND source_identifier=$2 FOR UPDATE`, owner, sourceIdentifier).Scan(&linkedBook)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	} else if err != nil {
		return "", err
	}
	if linkedBook != nil {
		if err = ensureBookExists(ctx, tx, owner, *linkedBook); err != nil {
			return "", err
		}
		if err = activateMembership(ctx, tx, owner, *linkedBook); err != nil {
			return "", err
		}
		if err = tx.Commit(ctx); err != nil {
			return "", err
		}
		return *linkedBook, nil
	}

	var bookID string
	err = tx.QueryRow(ctx, `SELECT b.id::text FROM books b JOIN book_aliases a ON a.owner_id=b.owner_id AND a.book_id=b.id WHERE a.owner_id=$1 AND a.namespace=$2 AND a.value=$3 AND a.connection_id IS NULL FOR UPDATE OF b,a`, owner, domain.NamespaceSourceIdentifier, sourceIdentifier).Scan(&bookID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	} else if err != nil {
		return "", err
	}
	if bookID != "" {
		if err = activateMembership(ctx, tx, owner, bookID); err != nil {
			return "", err
		}
		if err = tx.Commit(ctx); err != nil {
			return "", err
		}
		return bookID, nil
	}

	var created domain.Book
	created, err = scanBook(tx.QueryRow(ctx, `INSERT INTO books(owner_id,title,metadata_provenance,language_state,language_tag) VALUES($1,$2,$3,$4,$5) RETURNING `+bookColumns, owner, title, domain.MetadataProvenanceAcquisition, domain.LanguageChosen, language))
	if err != nil {
		return "", err
	}
	if err = activateMembership(ctx, tx, owner, created.ID); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return created.ID, nil
}
