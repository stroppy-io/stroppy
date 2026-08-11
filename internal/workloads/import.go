// Package workloads aggregates every Go-native workload via blank import so the
// binary pulls them in and their init() registrations run. Add a line per ported
// workload.
package workloads

import (
	_ "github.com/stroppy-io/stroppy/internal/workloads/execute_sql"
	_ "github.com/stroppy-io/stroppy/internal/workloads/simple"
	_ "github.com/stroppy-io/stroppy/internal/workloads/tpcb"
	_ "github.com/stroppy-io/stroppy/internal/workloads/tpcc"
	_ "github.com/stroppy-io/stroppy/internal/workloads/tpcds"
	_ "github.com/stroppy-io/stroppy/internal/workloads/tpch"
)
