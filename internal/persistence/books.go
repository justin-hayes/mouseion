package persistence

import (
	"context"
	"errors"
	"strings"

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

func (s *PostgresStore) GetBook(ctx context.Context, owner, bookID string) (domain.Book, error) {
	return scanBook(s.pool.QueryRow(ctx, `SELECT `+bookColumns+` FROM books WHERE owner_id=$1 AND id=$2`, owner, bookID))
}

func (s *PostgresStore) CreateBook(ctx context.Context, b domain.Book) (domain.Book, error) {
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
