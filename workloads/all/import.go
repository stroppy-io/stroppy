// Package all registers every built-in workload through blank imports.
package all

import (
	_ "github.com/stroppy-io/stroppy/workloads/baseline"
	_ "github.com/stroppy-io/stroppy/workloads/execute_sql"
	_ "github.com/stroppy-io/stroppy/workloads/simple"
	_ "github.com/stroppy-io/stroppy/workloads/tpcb"
	_ "github.com/stroppy-io/stroppy/workloads/tpcc"
	_ "github.com/stroppy-io/stroppy/workloads/tpcds"
	_ "github.com/stroppy-io/stroppy/workloads/tpch"
)
