// Package all registers every built-in workload through blank imports.
package all

import (
	_ "github.com/stroppy-io/stroppy/v6/workloads/baseline"
	_ "github.com/stroppy-io/stroppy/v6/workloads/execute_sql"
	_ "github.com/stroppy-io/stroppy/v6/workloads/simple"
	_ "github.com/stroppy-io/stroppy/v6/workloads/tpcb"
	_ "github.com/stroppy-io/stroppy/v6/workloads/tpcc"
	_ "github.com/stroppy-io/stroppy/v6/workloads/tpcds"
	_ "github.com/stroppy-io/stroppy/v6/workloads/tpch"
)
