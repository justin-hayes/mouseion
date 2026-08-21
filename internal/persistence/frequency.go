package persistence

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const frequencyDatasetColumns = `id,language,name,version,source_url,license,attribution,active,created_at`

func scanFrequencyDataset(row pgx.Row) (v domain.FrequencyDataset, err error) {
	err = row.Scan(&v.ID, &v.Language, &v.Name, &v.Version, &v.SourceURL, &v.License, &v.Attribution, &v.Active, &v.CreatedAt)
	return v, missing(err)
}

// CreateFrequencyDataset atomically creates an immutable dataset version and its entries.
func (s *PostgresStore) CreateFrequencyDataset(ctx context.Context, v domain.FrequencyDataset, entries []domain.FrequencyEntry) (domain.FrequencyDataset, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.FrequencyDataset{}, err
	}
	defer tx.Rollback(ctx)
	v, err = scanFrequencyDataset(tx.QueryRow(ctx, `INSERT INTO frequency_datasets(language,name,version,source_url,license,attribution) VALUES($1,$2,$3,$4,$5,$6) RETURNING `+frequencyDatasetColumns,
		v.Language, v.Name, v.Version, v.SourceURL, v.License, v.Attribution))
	if err != nil {
		return domain.FrequencyDataset{}, fmt.Errorf("create frequency dataset: %w", err)
	}
	rows := make([][]any, len(entries))
	for i, entry := range entries {
		rows[i] = []any{v.ID, v.Language, entry.CanonicalLemma, entry.UPOS, entry.Rank, entry.Percentile, entry.FrequencyClass}
	}
	if _, err = tx.CopyFrom(ctx, pgx.Identifier{"frequency_entries"}, []string{"dataset_id", "language", "canonical_lemma", "upos", "rank", "frequency", "frequency_class"}, pgx.CopyFromRows(rows)); err != nil {
		return domain.FrequencyDataset{}, fmt.Errorf("insert frequency entries: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.FrequencyDataset{}, err
	}
	return v, nil
}

func (s *PostgresStore) ActivateFrequencyDataset(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var language string
	if err = tx.QueryRow(ctx, `SELECT language FROM frequency_datasets WHERE id=$1 FOR UPDATE`, id).Scan(&language); err != nil {
		return missing(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE frequency_datasets SET active=false WHERE language=$1 AND active`, language); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE frequency_datasets SET active=true WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) DeactivateFrequencyDataset(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE frequency_datasets SET active=false WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *PostgresStore) RemoveFrequencyDataset(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM frequency_datasets WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *PostgresStore) GetActiveFrequencyDataset(ctx context.Context, language string) (domain.FrequencyDataset, error) {
	return scanFrequencyDataset(s.pool.QueryRow(ctx, `SELECT `+frequencyDatasetColumns+` FROM frequency_datasets WHERE language=$1 AND active`, language))
}

func (s *PostgresStore) ListFrequencyDatasets(ctx context.Context, language string) ([]domain.FrequencyDataset, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+frequencyDatasetColumns+` FROM frequency_datasets WHERE language=$1 ORDER BY created_at DESC,id`, language)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.FrequencyDataset
	for rows.Next() {
		v, err := scanFrequencyDataset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *PostgresStore) FrequencyPercentile(ctx context.Context, language, canonicalLemma, upos string) (float64, bool, error) {
	var percentile float64
	err := s.pool.QueryRow(ctx, `SELECT e.frequency FROM frequency_entries e JOIN frequency_datasets d ON d.id=e.dataset_id WHERE d.active AND e.language=$1 AND e.canonical_lemma=$2 AND e.upos=$3`, language, canonicalLemma, upos).Scan(&percentile)
	if err == pgx.ErrNoRows {
		return 0, false, nil
	}
	return percentile, err == nil, err
}
