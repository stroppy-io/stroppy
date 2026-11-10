//go:build integration && integration_optional

package integration

import "testing"

func TestTPCDSGeneratedTiDBAllQueries(t *testing.T) {
	db := NewTiDB(t)
	url := envOr(envTiDBAllURL, defaultTiDBAllURL)
	testTPCDSFullJoinResults(t, db, "tidb", url)
	testTPCDSGeneratedMySQLDialect(t, "tidb", url)
}
