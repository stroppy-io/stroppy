package banking

import "github.com/stroppy-io/stroppy/v6/pkg/gen"

// loadBatchRows and loadBatchBytes size the load's batches: enough rows to be worth a
// round trip, capped so a batch stays a reasonable message.
const (
	loadBatchRows  = 64
	loadBatchBytes = 4096
)

// account is one row of bank_account. gen.FromRows derives the column names from
// these fields, so they must match the table: id, balance, token.
type account struct {
	ID      int64
	Balance int64
	Token   int64
}

// accountRows yields accounts whose tokens start as the permutation 1..accounts,
// which is the state the verify step expects to still find at the end — possibly
// permuted, never lost or duplicated.
func accountRows(accounts int64) gen.BatchSource {
	return gen.FromRows(accounts, func(index uint64) (account, error) {
		id := int64(index) + 1 //nolint:gosec // benchmark row index, bounded by accounts

		return account{ID: id, Balance: initialBalance, Token: id}, nil
	}, gen.BatchRows(loadBatchRows), gen.MaxBytes(loadBatchBytes))
}
