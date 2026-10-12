package persistence

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// foldingProfile canonicalizes by lowercasing, so distinct historical spellings
// such as "Foo" and "FOO" converge on one identity.
type foldingProfile struct{}

func (foldingProfile) Name() string     { return "folding" }
func (foldingProfile) Version() string  { return "1" }
func (foldingProfile) Language() string { return "de" }
func (foldingProfile) Canonical(lemma string) string {
	return strings.ToLower(lemma)
}

// conflictTx stands in for the transaction the curated-conflict check queries.
// Only Query and QueryRow are implemented; any other method panics via the nil
// embedded interface, so an unexpected call fails loudly.
type conflictTx struct {
	pgx.Tx
	rows     pgx.Rows
	queryErr error
	exists   pgx.Row
}

func (tx conflictTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return tx.rows, tx.queryErr
}

func (tx conflictTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return tx.exists
}

// conflictRows yields canned (lemma, upos) pairs and can fail while scanning or
// after the last row.
type conflictRows struct {
	pgx.Rows
	values  [][2]string
	pos     int
	scanErr error
	rowsErr error
}

func (r *conflictRows) Next() bool {
	if r.pos >= len(r.values) {
		return false
	}
	r.pos++
	return true
}

func (r *conflictRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	*dest[0].(*string) = r.values[r.pos-1][0]
	*dest[1].(*string) = r.values[r.pos-1][1]
	return nil
}

func (r *conflictRows) Err() error { return r.rowsErr }
func (r *conflictRows) Close()     {}

// conflictExistsRow answers the per-row EXISTS probe for a modern identity.
type conflictExistsRow struct {
	exists bool
	err    error
}

func (r conflictExistsRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*bool) = r.exists
	return nil
}

func TestGermanCuratedConflict(t *testing.T) {
	queryErr := errors.New("query failed")
	scanErr := errors.New("scan failed")
	rowsErr := errors.New("rows failed")
	existsErr := errors.New("exists failed")

	tests := []struct {
		name       string
		tx         conflictTx
		wantErr    error
		wantFound  bool
		wantDetail string
		wantTarget string
	}{
		{
			name:    "query failure is returned",
			tx:      conflictTx{queryErr: queryErr},
			wantErr: queryErr,
		},
		{
			name: "scan failure is returned",
			tx: conflictTx{rows: &conflictRows{
				values:  [][2]string{{"Haus", "NOUN"}},
				scanErr: scanErr,
			}},
			wantErr: scanErr,
		},
		{
			name: "rows iteration failure is returned",
			tx: conflictTx{rows: &conflictRows{
				rowsErr: rowsErr,
			}},
			wantErr: rowsErr,
		},
		{
			name: "historical facts converging on one identity conflict",
			tx: conflictTx{rows: &conflictRows{
				values: [][2]string{{"Foo", "NOUN"}, {"FOO", "NOUN"}},
			}},
			wantFound:  true,
			wantDetail: "multiple historical curated facts converge on one identity",
			wantTarget: "foo",
		},
		{
			name: "modern identity already present conflicts",
			tx: conflictTx{
				rows:   &conflictRows{values: [][2]string{{"Foo", "NOUN"}}},
				exists: conflictExistsRow{exists: true},
			},
			wantFound:  true,
			wantDetail: "both historical and modern curated facts exist",
			wantTarget: "foo",
		},
		{
			name: "existence probe failure is returned",
			tx: conflictTx{
				rows:   &conflictRows{values: [][2]string{{"Foo", "NOUN"}}},
				exists: conflictExistsRow{err: existsErr},
			},
			wantErr: existsErr,
		},
		{
			name: "already canonical facts are safe to backfill",
			tx: conflictTx{
				rows:   &conflictRows{values: [][2]string{{"foo", "NOUN"}}},
				exists: conflictExistsRow{exists: false},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conflict, found, err := germanCuratedConflict(t.Context(), test.tx, "owner", foldingProfile{})

			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				assert.False(t, found)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantFound, found)
			if !test.wantFound {
				assert.Equal(t, GermanVocabularyBackfillConflict{}, conflict)
				return
			}
			assert.Equal(t, "curated_sentences", conflict.Table)
			assert.Equal(t, test.wantTarget, conflict.CanonicalLemma)
			assert.Equal(t, "NOUN", conflict.UPOS)
			assert.Equal(t, test.wantDetail, conflict.Detail)
		})
	}
}
