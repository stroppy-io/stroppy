package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestCopyTextRealPostgres(t *testing.T) {
	d := realPGDriver(t, 2)
	execSQL(t, d, `CREATE TEMP TABLE copy_text_values (
		i2 smallint, i4 integer, i8 bigint, amount numeric(30,7),
		label text, code varchar(20), day date, flag boolean, numbers integer[],
		missing numeric, typed_amount numeric(12,2), typed_time timestamp,
		payload jsonb, data bytea, real_value real, double_value double precision
	)`)

	date := time.Date(2026, 11, 14, 12, 30, 0, 0, time.UTC)
	src := &fakeSource{
		cols: []string{
			"i2", "i4", "i8", "amount", "label", "code", "day", "flag", "numbers",
			"missing", "typed_amount", "typed_time", "payload", "data", "real_value", "double_value",
		},
		rows: [][]any{{
			"-32768", "2147483647", "9223372036854775807", "-12345678901234567890.0012300",
			"000123.00", "00123", "2000-02-29", "true", "[0:1]={3,4}",
			nil, decimal.RequireFromString("12.34"), date, `{"n":1}`,
			[]byte{0, 1, 255},
			"1.0000000596046448", "-1.25e10",
		}},
	}
	require.NoError(t, d.copyFromRuntime(t.Context(), "copy_text_values", src))

	var valid bool
	queryOneRow(t, d, `SELECT i2 = -32768 AND i4 = 2147483647 AND i8 = 9223372036854775807
		AND amount = -12345678901234567890.0012300 AND label = '000123.00' AND code = '00123'
		AND day = DATE '2000-02-29' AND flag AND numbers = '[0:1]={3,4}'::integer[]
		AND missing IS NULL AND typed_amount = 12.34 AND typed_time = TIMESTAMP '2026-11-14 12:30:00'
		AND payload = '{"n":1}'::jsonb AND data = '\x0001ff'::bytea
		AND real_value = '1.0000000596046448'::real AND double_value = '-1.25e10'::double precision
		FROM copy_text_values`, &valid)
	require.True(t, valid)
}

func TestCopyTextFailureRollsBack(t *testing.T) {
	d := realPGDriver(t, 2)
	execSQL(t, d, `CREATE TEMP TABLE copy_text_failure (amount integer)`)

	for _, text := range []string{"2147483648", "not an integer"} {
		src := &fakeSource{cols: []string{"amount"}, rows: [][]any{{"123"}, {text}}}
		require.Error(t, d.copyFromRuntime(t.Context(), "copy_text_failure", src))

		var count int
		queryOneRow(t, d, "SELECT count(*) FROM copy_text_failure", &count)
		require.Zero(t, count)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	src := &fakeSource{cols: []string{"amount"}, rows: [][]any{{"123"}}}
	require.ErrorIs(t, d.copyFromRuntime(ctx, "copy_text_failure", src), context.Canceled)

	sourceErr := errors.New("source failed")
	err := d.copyFromRuntime(t.Context(), "copy_text_failure", &failedCopySource{err: sourceErr})
	require.ErrorContains(t, err, sourceErr.Error())

	var count int
	queryOneRow(t, d, "SELECT count(*) FROM copy_text_failure", &count)
	require.Zero(t, count)

	src = &fakeSource{cols: []string{"amount"}, rows: [][]any{{int64(123)}}}
	require.NoError(t, d.copyFromRuntime(t.Context(), "copy_text_failure", src))
	queryOneRow(t, d, "SELECT count(*) FROM copy_text_failure", &count)
	require.Equal(t, 1, count)
}

type failedCopySource struct{ err error }

func (*failedCopySource) Columns() []string      { return []string{"amount"} }
func (s *failedCopySource) Next() ([]any, error) { return nil, s.err }

var _ pgx.CopyFromSource = (*rowSource)(nil)
