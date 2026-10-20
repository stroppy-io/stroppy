package banking

import (
	"testing"

	"github.com/stroppy-io/stroppy/v6/pkg/bench"
	"github.com/stroppy-io/stroppy/v6/pkg/bench/testkit"
	"github.com/stroppy-io/stroppy/v6/pkg/record"
)

// The statements exactly as the driver sends them: the recorder matches on the
// text verbatim, including the named placeholders.
const (
	readAccountSQL = "SELECT balance, token FROM bank_account WHERE id = :id"
	readTokenSQL   = "SELECT token FROM bank_account WHERE id = :id"
	rotateTokenSQL = "UPDATE bank_account SET token = :token WHERE id = :id AND token = :expected"
	moveBalanceSQL = "UPDATE bank_account SET balance = balance + :delta WHERE id = :id"
	totalSQL       = "SELECT coalesce(sum(balance), 0)::bigint AS total FROM bank_account"
	tokenStatsSQL  = "SELECT count(*)::bigint AS accounts_rows,\n" +
		"       coalesce(min(token), 0)::bigint AS lowest_token,\n" +
		"       coalesce(max(token), 0)::bigint AS highest_token,\n" +
		"       (SELECT count(*)::bigint FROM (\n" +
		"            SELECT token FROM bank_account GROUP BY token HAVING count(*) > 1\n" +
		"        ) AS repeated) AS repeated_tokens\n" +
		"FROM bank_account"
)

// TestSwapMovesAreASentinelExchange pins the swap's semantics with no server and
// no decoding: the first row parks its token in a sentinel unique to itself, each
// row takes the other's token, and every write is conditional on what was read.
func TestSwapMovesAreASentinelExchange(t *testing.T) {
	const (
		from      = 7
		to        = 3
		fromToken = 7
		toToken   = 3
	)

	moves := swapMoves(from, to, fromToken, toToken)
	if len(moves) != 3 {
		t.Fatalf("swapMoves returned %d moves, want 3", len(moves))
	}

	if moves[0].token != -from {
		t.Errorf("the first move parks %d, want the sentinel %d", moves[0].token, -from)
	}

	// Every move is conditional on the value that row holds at that point, which
	// is what turns a concurrent transfer into a detected conflict.
	if moves[0].expected != fromToken {
		t.Errorf("the first move expects %d, want the token that was read (%d)", moves[0].expected, fromToken)
	}

	if moves[1].expected != toToken {
		t.Errorf("the second move expects %d, want %d", moves[1].expected, toToken)
	}

	if moves[2].expected != moves[0].token {
		t.Errorf("the last move expects %d, want the sentinel this row parked (%d)", moves[2].expected, moves[0].token)
	}

	// The two rows exchange tokens: what one ends up holding is what the other
	// was holding.
	if moves[1].token != fromToken || moves[2].token != toToken {
		t.Errorf("the tokens do not exchange: %+v", moves)
	}

	// The sentinel is the negative of the row's own id, so no other row can hold
	// it: uniqueness holds at every statement boundary.
	if moves[0].id != from || moves[2].id != from || moves[1].id != to {
		t.Errorf("the moves touch the wrong rows: %+v", moves)
	}

	deltas := balanceMoves(from, to, 5)
	if len(deltas) != 2 || deltas[0].delta != -5 || deltas[1].delta != 5 {
		t.Errorf("balanceMoves = %+v, want equal and opposite", deltas)
	}

	if deltas[0].id != from || deltas[1].id != to {
		t.Errorf("balanceMoves touch the wrong rows: %+v", deltas)
	}
}

// TestOneTransferRunsAgainstCannedAnswers drives one transfer through the
// recording driver, so the whole path — reads, the three conditional writes, the
// confirmations, the balance moves and the verify step — is exercised in process,
// with no database.
func TestOneTransferRunsAgainstCannedAnswers(t *testing.T) {
	recorder := &record.Recorder{}

	// The transfer reads `from` then `to`, in that order: two distinct accounts,
	// each starting at 1000 with its own token. Replies are consumed per call, so
	// these two are the initial reads and nothing else should need a third.
	account := func(token int64) record.Response {
		return record.Response{
			Columns: []string{"balance", "token"},
			Rows:    [][]any{{int64(1000), token}},
		}
	}

	recorder.Reply(readAccountSQL, account(1)) // from
	recorder.Reply(readAccountSQL, account(2)) // to

	// Each conditional write is confirmed by a read of the value just written.
	// The queue is consumed in call order: sentinel, then the exchanged tokens.
	// The picker is deterministic (accounts=2, one seed per worker), which is what
	// makes these answerable in advance.
	recorder.Reply(readTokenSQL, record.Response{Columns: []string{"token"}, Rows: [][]any{{int64(-1)}}})
	recorder.Reply(readTokenSQL, record.Response{Columns: []string{"token"}, Rows: [][]any{{int64(1)}}})
	recorder.Reply(readTokenSQL, record.Response{Columns: []string{"token"}, Rows: [][]any{{int64(2)}}})

	// What the verify step asks, for two untouched accounts.
	recorder.Reply(totalSQL, record.Response{Columns: []string{"total"}, Rows: [][]any{{int64(2000)}}})
	recorder.Reply(tokenStatsSQL, record.Response{
		Columns: []string{"accounts_rows", "lowest_token", "highest_token", "repeated_tokens"},
		Rows:    [][]any{{int64(2), int64(1), int64(2), int64(0)}},
	})

	run, err := testkit.Record(t.Context(), Test, recorder, bench.RunOptions{
		Params: bench.ParamInputs{CLI: map[string]string{"accounts": "2"}},
	})
	if err != nil {
		t.Fatalf("record run: %v", err)
	}

	if failed := run.Errors.FailedIterations; failed != 0 {
		t.Fatalf("failed iterations = %d, want 0", failed)
	}

	counted := map[string]int{}
	for _, operation := range recorder.Operations() {
		counted[operation.SQL]++
	}

	if got := counted[rotateTokenSQL]; got != 3 {
		t.Errorf("issued %d conditional token writes, want 3 (the sentinel swap)", got)
	}

	if got := counted[moveBalanceSQL]; got != 2 {
		t.Errorf("issued %d balance moves, want 2", got)
	}

	if got := counted[readTokenSQL]; got != 3 {
		t.Errorf("issued %d confirmation reads, want one per write", got)
	}
}
