package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/v6/pkg/config"
	"github.com/stroppy-io/stroppy/v6/pkg/driver/sqldriver"
)

var benchmarkTransaction *sqldriver.Tx[pgx.Rows]

func BenchmarkTransactionWrapper(b *testing.B) {
	d := &Driver{logger: zap.NewNop()}

	b.ReportAllocs()

	for b.Loop() {
		benchmarkTransaction = newTx(nil, config.TxIsolationLevelReadCommitted, d)
	}
}
