package dictionary

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	_ "modernc.org/sqlite"
)

var ErrInvalidIndex = errors.New("dictionary: invalid index")

// Index is a read-only SQLite-backed lexical provider. It is safe for
// concurrent lookups and should be closed with the server.
type Index struct {
	db      *sql.DB
	name    string
	version string
}

var _ enrichment.LexicalProvider = (*Index)(nil)

func OpenIndex(ctx context.Context, path string) (*Index, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%w: path is empty", ErrInvalidIndex)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve path: %w", ErrInvalidIndex, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("%w: stat index: %w", ErrInvalidIndex, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: %s is a directory, not a SQLite file (a missing Docker bind-mount source is created as a directory)", ErrInvalidIndex, absolute)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(absolute)+"?mode=ro&_query_only=1")
	if err != nil {
		return nil, fmt.Errorf("%w: open: %w", ErrInvalidIndex, err)
	}
	db.SetMaxOpenConns(8)
	index := &Index{db: db, name: "kaikki", version: ""}
	if err = db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("%w: ping: %w", ErrInvalidIndex, errors.Join(err, db.Close()))
	}
	if err = db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'provider_version'`).Scan(&index.version); err != nil || strings.TrimSpace(index.version) == "" {
		if err == nil {
			err = errors.New("provider_version is empty")
		}
		err = errors.Join(err, db.Close())
		return nil, fmt.Errorf("%w: metadata: %w", ErrInvalidIndex, err)
	}
	if _, err = db.ExecContext(ctx, `SELECT language, lemma, upos, senses_json, gender, article, plural, ipa, principal_parts FROM entries LIMIT 0`); err != nil {
		return nil, fmt.Errorf("%w: schema: %w", ErrInvalidIndex, errors.Join(err, db.Close()))
	}
	return index, nil
}

func (i *Index) Name() string {
	if i == nil || i.name == "" {
		return "kaikki"
	}
	return i.name
}

func (i *Index) Version() string {
	if i == nil {
		return ""
	}
	return i.version
}

func (i *Index) Close() error {
	if i == nil || i.db == nil {
		return nil
	}
	return i.db.Close()
}

func (i *Index) Lookup(ctx context.Context, request enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	if i == nil || i.db == nil {
		return enrichment.LexicalEntry{}, false, nil
	}
	language := canonicalization.NormalizeLanguage(request.Language)
	lemma := normalizeLemma(language, request.CanonicalLemma)
	upos := strings.ToUpper(strings.TrimSpace(request.UPOS))
	if language == "" || lemma == "" || upos == "" {
		return enrichment.LexicalEntry{}, false, nil
	}
	var sensesJSON, gender, article, plural, ipa, principalParts string
	err := i.db.QueryRowContext(ctx, `SELECT senses_json, gender, article, plural, ipa, principal_parts FROM entries WHERE language = ? AND lemma = ? AND upos = ?`, language, lemma, upos).Scan(&sensesJSON, &gender, &article, &plural, &ipa, &principalParts)
	if errors.Is(err, sql.ErrNoRows) {
		return enrichment.LexicalEntry{}, false, nil
	}
	if err != nil {
		return enrichment.LexicalEntry{}, false, fmt.Errorf("dictionary lookup: %w", err)
	}
	var senses []enrichment.LexicalSense
	if err = json.Unmarshal([]byte(sensesJSON), &senses); err != nil {
		return enrichment.LexicalEntry{}, false, fmt.Errorf("dictionary senses: %w", err)
	}
	ordered := enrichment.OrderSenses(request, senses)
	result := enrichment.LexicalEntry{Senses: ordered, CandidateSenses: append([]enrichment.LexicalSense(nil), senses...), IPA: ipa, PrincipalParts: principalParts}
	if language == "el" {
		result.Gender = gender
		result.Article = article
		result.Plural = plural
	} else if len(ordered) > 0 {
		result.Gender = ordered[0].Gender
		result.Article = ordered[0].Article
		result.Plural = ordered[0].Plural
	}
	return result, true, nil
}

func normalizeLemma(language, lemma string) string {
	normalized, err := canonicalization.Normalize(language, lemma)
	if err != nil {
		return ""
	}
	return normalized.CanonicalLemma
}
