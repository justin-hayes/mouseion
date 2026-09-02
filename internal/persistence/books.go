package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const bookColumns = `id::text,owner_id::text,title,metadata_provenance,language_state,COALESCE(language_tag,''),created_at,updated_at`
const qualifiedBookColumns = `b.id::text,b.owner_id::text,b.title,b.metadata_provenance,b.language_state,COALESCE(b.language_tag,''),b.created_at,b.updated_at`

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

// ListMyBooksWithEvidence is the My Books collection read model. Membership
// is the driving table, so metadata-only Books remain visible; the current
// acquired source and analysis projection are optional evidence on each row.
func (s *PostgresStore) ListMyBooksWithEvidence(ctx context.Context, owner string) ([]domain.MyBook, error) {
	rows, err := s.pool.Query(ctx, currentAnalysisCTE+`
		SELECT `+qualifiedBookColumns+`,
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
		            WHEN scope.scope_id IS NOT NULL THEN 'scope confirmed'
		            WHEN j.river_job_id IS NOT NULL AND j.error = '' THEN 'analyzing'
		            ELSE 'not analyzed' END,
		       CASE WHEN ar.state IS NOT NULL THEN ar.state
		            WHEN ca.analysis_run_id IS NOT NULL THEN 'completed'
		            WHEN j.river_job_id IS NOT NULL AND j.error <> '' THEN 'failed'
		            WHEN j.river_job_id IS NOT NULL THEN 'queued'
		            ELSE '' END,
		       COALESCE(ca.analysis_run_id::text,''),COALESCE(ca.corpus_id::text,''),COALESCE(ca.reviewed_scope_id::text,''),COALESCE(scope.scope_id::text,''),COALESCE(j.river_job_id,0)
		FROM books b
		JOIN book_membership m ON m.owner_id=b.owner_id AND m.book_id=b.id AND m.state='active'
		LEFT JOIN book_current_analyses p ON p.owner_id=b.owner_id AND p.book_id=b.id
		LEFT JOIN current_analysis ca ON ca.owner_id=b.owner_id AND ca.book_id=b.id
		LEFT JOIN LATERAL (SELECT s.* FROM source_materials s WHERE s.owner_id=b.owner_id AND s.book_id=b.id ORDER BY CASE WHEN p.source_material_id IS NOT NULL AND s.id=p.source_material_id THEN 0 ELSE 1 END,s.created_at DESC,s.id DESC LIMIT 1) s ON true
		LEFT JOIN source_content_revisions r ON r.owner_id=s.owner_id AND r.revision_id=s.current_content_revision_id
		LEFT JOIN LATERAL (SELECT river_job_id,error,analysis_run_id FROM analysis_jobs j WHERE j.owner_id=s.owner_id AND j.source_material_id=s.id ORDER BY j.created_at DESC,j.river_job_id DESC LIMIT 1) j ON true
		LEFT JOIN analysis_runs ar ON ar.owner_id=s.owner_id AND ar.id=j.analysis_run_id
		LEFT JOIN LATERAL (SELECT scope_id FROM epub_reviewed_scopes scope WHERE scope.owner_id=s.owner_id AND scope.source_material_id=s.id ORDER BY scope.created_at DESC,scope.scope_id DESC LIMIT 1) scope ON true
		WHERE b.owner_id=$1
		ORDER BY b.title,b.id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MyBook
	for rows.Next() {
		var item domain.MyBook
		var sourceID, sourceOwner, sourceLanguage, sourceIdentifier, sourceTitle, sourceMediaType string
		var sourceContentHash, sourceDigest, sourceRevisionID string
		var sourceCreatedAt *time.Time
		var sourceExists bool
		var evidenceState domain.MyBookEvidenceState
		var analysisStatus, analysisState, analysisRunID, corpusID, reviewedScopeID, confirmedScopeID string
		var analysisJobID int64
		var digestVersion int
		if err := rows.Scan(&item.Book.ID, &item.Book.OwnerID, &item.Book.Title, &item.Book.MetadataProvenance, &item.Book.LanguageState, &item.Book.LanguageTag, &item.Book.CreatedAt, &item.Book.UpdatedAt,
			&sourceID, &sourceOwner, &sourceLanguage, &sourceIdentifier, &sourceTitle, &sourceMediaType, &sourceContentHash, &sourceDigest, &sourceRevisionID, &digestVersion, &sourceCreatedAt, &sourceExists,
			&evidenceState, &analysisStatus, &analysisState, &analysisRunID, &corpusID, &reviewedScopeID, &confirmedScopeID, &analysisJobID); err != nil {
			return nil, err
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
				AnalysisStatus: analysisStatus, AnalysisState: analysisState, AnalysisRunID: analysisRunID, CorpusID: corpusID, ReviewedScopeID: reviewedScopeID, ConfirmedScopeID: confirmedScopeID, AnalysisJobID: analysisJobID,
			}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// IsMetadataOnlyMyBook reports whether the owner's Book is an active My Books
// member with no acquired source. Metadata-only membership is the only Book
// state whose page needs no acquired-content surface; every other state falls
// through to the normal Book projection.
func (s *PostgresStore) IsMetadataOnlyMyBook(ctx context.Context, owner, bookID string) (bool, error) {
	var metadataOnly bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM books b
		JOIN book_membership m ON m.owner_id=b.owner_id AND m.book_id=b.id AND m.state='active'
		WHERE b.owner_id=$1 AND b.id::text=$2
		  AND NOT EXISTS (SELECT 1 FROM source_materials s WHERE s.owner_id=b.owner_id AND s.book_id=b.id))`,
		owner, bookID).Scan(&metadataOnly)
	return metadataOnly, err
}

func (s *PostgresStore) GetBook(ctx context.Context, owner, bookID string) (domain.Book, error) {
	return scanBook(s.pool.QueryRow(ctx, `SELECT `+bookColumns+` FROM books WHERE owner_id=$1 AND id=$2`, owner, bookID))
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
	if b.MetadataProvenance == domain.MetadataProvenanceManualEntry {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,467))`, b.OwnerID+":"+b.Title+":"+b.LanguageState+":"+b.LanguageTag); err != nil {
			return domain.Book{}, err
		}
		var existing domain.Book
		existing, err = scanBook(tx.QueryRow(ctx, `SELECT `+bookColumns+` FROM books WHERE owner_id=$1 AND metadata_provenance=$2 AND title=$3 AND language_state=$4 AND COALESCE(language_tag,'')=COALESCE($5,'') FOR UPDATE`, b.OwnerID, b.MetadataProvenance, b.Title, b.LanguageState, nullableLanguageTag(b.LanguageState, b.LanguageTag)))
		if err == nil {
			if err = activateMembership(ctx, tx, existing.OwnerID, existing.ID); err != nil {
				return domain.Book{}, err
			}
			if err = tx.Commit(ctx); err != nil {
				return domain.Book{}, err
			}
			return existing, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return domain.Book{}, err
		}
	}
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return err
	}
	var inserted string
	err = tx.QueryRow(ctx, `INSERT INTO book_aliases(owner_id,book_id,alias_type,namespace,value) VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner_id,namespace,value) DO NOTHING RETURNING id::text`, owner, bookID, aliasType, alias.Namespace, alias.Value).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		var existingBook string
		if err = tx.QueryRow(ctx, `SELECT book_id::text FROM book_aliases WHERE owner_id=$1 AND namespace=$2 AND value=$3`, owner, alias.Namespace, alias.Value).Scan(&existingBook); err != nil {
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
	Book         domain.Book
	Created      bool
	TitleChanged bool
}

// Upserted reports whether the entry added or updated a Book.
func (r CatalogueEntryReconcileResult) Upserted() bool { return r.Created || r.TitleChanged }

// ReconcileCatalogueEntry performs the metadata-only, owner-scoped catalogue
// upsert. New Books are created with a chosen language only when the catalogue
// sync scope determines that language deterministically from the learner's
// explicitly saved study language (never inferred); existing matched Books are
// never flipped between language states. Source materials and acquired content
// are never touched here: membership and identity are metadata-only.
func (s *PostgresStore) ReconcileCatalogueEntry(ctx context.Context, owner, sourceIdentifier, title, language string) (CatalogueEntryReconcileResult, error) {
	owner, sourceIdentifier, title, language = strings.TrimSpace(owner), strings.TrimSpace(sourceIdentifier), strings.TrimSpace(title), strings.TrimSpace(language)
	if owner == "" || sourceIdentifier == "" || title == "" || language == "" {
		return CatalogueEntryReconcileResult{}, errors.New("persistence: catalogue entry identity is incomplete")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,468))`, owner+":"+sourceIdentifier); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	var sourceBook, aliasBook *string
	err = tx.QueryRow(ctx, `SELECT book_id::text FROM source_materials WHERE owner_id=$1 AND source_identifier=$2 FOR UPDATE`, owner, sourceIdentifier).Scan(&sourceBook)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	} else if err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	err = tx.QueryRow(ctx, `SELECT book_id::text FROM book_aliases WHERE owner_id=$1 AND namespace=$2 AND value=$3 FOR UPDATE`, owner, domain.NamespaceSourceIdentifier, sourceIdentifier).Scan(&aliasBook)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	} else if err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	if sourceBook != nil && aliasBook != nil && *sourceBook != *aliasBook {
		return CatalogueEntryReconcileResult{}, ErrSourceBookConflict
	}
	bookID := ""
	if sourceBook != nil {
		bookID = *sourceBook
	} else if aliasBook != nil {
		bookID = *aliasBook
	}
	created := bookID == ""
	var titleChanged bool
	if bookID == "" {
		var createdBook domain.Book
		createdBook, err = scanBook(tx.QueryRow(ctx, `INSERT INTO books(owner_id,title,metadata_provenance,language_state,language_tag) VALUES($1,$2,$3,'chosen',$4) RETURNING `+bookColumns, owner, title, domain.MetadataProvenanceCatalogueSync, language))
		if err != nil {
			return CatalogueEntryReconcileResult{}, err
		}
		bookID = createdBook.ID
	} else {
		var currentTitle string
		if err = tx.QueryRow(ctx, `SELECT title FROM books WHERE owner_id=$1 AND id=$2`, owner, bookID).Scan(&currentTitle); err != nil {
			return CatalogueEntryReconcileResult{}, err
		}
		titleChanged = strings.TrimSpace(currentTitle) != title
	}
	if err = ensureBookExists(ctx, tx, owner, bookID); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE books SET title=$3,updated_at=now() WHERE owner_id=$1 AND id=$2`, owner, bookID, title); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	if err = activateMembership(ctx, tx, owner, bookID); err != nil {
		return CatalogueEntryReconcileResult{}, err
	}
	if aliasBook == nil {
		if _, err = tx.Exec(ctx, `INSERT INTO book_aliases(owner_id,book_id,alias_type,namespace,value) VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner_id,namespace,value) DO NOTHING`, owner, bookID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, sourceIdentifier); err != nil {
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
	return CatalogueEntryReconcileResult{Book: book, Created: created, TitleChanged: titleChanged}, nil
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
		err = tx.QueryRow(ctx, `SELECT book_id::text FROM book_aliases WHERE owner_id=$1 AND namespace=$2 AND value=$3 FOR UPDATE`, owner, domain.NamespaceSourceIdentifier, sourceIdentifier).Scan(&aliasedBook)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
		} else if err != nil {
			return "", err
		}
		if aliasedBook != nil && *aliasedBook != requestedBookID {
			return "", ErrAliasConflict
		}
		if aliasedBook == nil {
			if _, err = tx.Exec(ctx, `INSERT INTO book_aliases(owner_id,book_id,alias_type,namespace,value) VALUES($1,$2,$3,$4,$5)`, owner, requestedBookID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, sourceIdentifier); err != nil {
				return "", aliasConflictError(err)
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE books SET language_state='chosen',language_tag=$3,updated_at=now() WHERE owner_id=$1 AND id=$2 AND language_state='unknown'`, owner, requestedBookID, language); err != nil {
			return "", err
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
	err = tx.QueryRow(ctx, `SELECT b.id::text FROM books b JOIN book_aliases a ON a.owner_id=b.owner_id AND a.book_id=b.id WHERE a.owner_id=$1 AND a.namespace=$2 AND a.value=$3 FOR UPDATE OF b,a`, owner, domain.NamespaceSourceIdentifier, sourceIdentifier).Scan(&bookID)
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
	created, err = scanBook(tx.QueryRow(ctx, `INSERT INTO books(owner_id,title,metadata_provenance,language_state,language_tag) VALUES($1,$2,'acquisition',$3,$4) RETURNING `+bookColumns, owner, title, domain.LanguageChosen, language))
	if err != nil {
		return "", err
	}
	if err = activateMembership(ctx, tx, owner, created.ID); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO book_aliases(owner_id,book_id,alias_type,namespace,value) VALUES($1,$2,$3,$4,$5)`, owner, created.ID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, sourceIdentifier); err != nil {
		return "", aliasConflictError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return created.ID, nil
}
