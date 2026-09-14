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

func OpenIndex(path string) (*Index, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%w: path is empty", ErrInvalidIndex)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve path: %v", ErrInvalidIndex, err)
	}
	if _, err = os.Stat(absolute); err != nil {
		return nil, fmt.Errorf("%w: stat index: %v", ErrInvalidIndex, err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(absolute)+"?mode=ro&_query_only=1")
	if err != nil {
		return nil, fmt.Errorf("%w: open: %v", ErrInvalidIndex, err)
	}
	db.SetMaxOpenConns(8)
	index := &Index{db: db, name: "kaikki", version: ""}
	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%w: ping: %v", ErrInvalidIndex, err)
	}
	if err = db.QueryRow(`SELECT value FROM metadata WHERE key = 'provider_version'`).Scan(&index.version); err != nil || strings.TrimSpace(index.version) == "" {
		_ = db.Close()
		if err == nil {
			err = errors.New("provider_version is empty")
		}
		return nil, fmt.Errorf("%w: metadata: %v", ErrInvalidIndex, err)
	}
	if _, err = db.Exec(`SELECT language, lemma, upos, senses_json, gender, article, plural, ipa FROM entries LIMIT 0`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%w: schema: %v", ErrInvalidIndex, err)
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

func (i *Index) Lookup(ctx context.Context, request LookupRequest) (Result, bool, error) {
	if i == nil || i.db == nil {
		return Result{}, false, nil
	}
	language := canonicalization.NormalizeLanguage(request.Language)
	lemma := normalizeLemma(language, request.CanonicalLemma)
	upos := strings.ToUpper(strings.TrimSpace(request.UPOS))
	if language == "" || lemma == "" || upos == "" {
		return Result{}, false, nil
	}
	var sensesJSON, gender, article, plural, ipa string
	err := i.db.QueryRowContext(ctx, `SELECT senses_json, gender, article, plural, ipa FROM entries WHERE language = ? AND lemma = ? AND upos = ?`, language, lemma, upos).Scan(&sensesJSON, &gender, &article, &plural, &ipa)
	if errors.Is(err, sql.ErrNoRows) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, fmt.Errorf("dictionary lookup: %w", err)
	}
	var senses []Sense
	if err = json.Unmarshal([]byte(sensesJSON), &senses); err != nil {
		return Result{}, false, fmt.Errorf("dictionary senses: %w", err)
	}
	for index := range senses {
		if senses[index].Gender == "" {
			senses[index].Gender = gender
		}
		if senses[index].Article == "" {
			senses[index].Article = article
		}
		if senses[index].Plural == "" {
			senses[index].Plural = plural
		}
		if senses[index].IPA == "" {
			senses[index].IPA = ipa
		}
	}
	ordered := OrderSenses(request, senses)
	result := Result{Senses: ordered, Gloss: RenderGloss(ordered, DefaultMaxSenses, DefaultMaxTokens), Article: article, Plural: plural}
	if len(ordered) > 0 {
		result.Gender = ordered[0].Gender
		if ordered[0].Article != "" {
			result.Article = ordered[0].Article
		}
		if ordered[0].Plural != "" {
			result.Plural = ordered[0].Plural
		}
	}
	result.Morphology = map[string]string{}
	if result.Gender != "" {
		result.Morphology["Gender"] = result.Gender
	}
	if result.Article != "" {
		result.Morphology["Article"] = result.Article
	}
	if result.Plural != "" {
		result.Morphology["Plural"] = result.Plural
	}
	return result, true, nil
}

func normalizeLemma(language, lemma string) string {
	lemma = strings.ToLower(strings.TrimSpace(lemma))
	if language == "de" {
		lemma = canonicalization.GermanPost1996().Canonical(lemma)
	}
	return lemma
}
