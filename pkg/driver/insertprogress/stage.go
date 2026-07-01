package insertprogress

import (
	"context"
	"time"
)

const (
	// StageRuntimeNext means a worker is pulling rows from the datagen runtime.
	StageRuntimeNext = "runtime_next"
	// StageSQLBulkInsertExec means a SQL driver is executing a multi-row INSERT.
	StageSQLBulkInsertExec = "sql_bulk_insert_exec"
	// StagePostgresCopyFrom means the PostgreSQL driver is inside pgx.CopyFrom.
	StagePostgresCopyFrom = "postgres_copy_from"
	// StagePostgresBulkInsertExec means the PostgreSQL driver is executing a VALUES INSERT.
	StagePostgresBulkInsertExec = "postgres_bulk_insert_exec"
	// StagePostgresColumnarExec means the PostgreSQL driver is executing an unnest INSERT.
	StagePostgresColumnarExec = "postgres_columnar_exec"
	// StageYDBBulkUpsert means the YDB driver is flushing a BulkUpsert batch.
	StageYDBBulkUpsert = "ydb_bulk_upsert"
	// StageCSVWrite means the CSV driver is writing generated rows to a shard file.
	StageCSVWrite = "csv_write"
	// StageNoopDrain means the noop driver is draining generated rows without I/O.
	StageNoopDrain = "noop_drain"
)

// SetTotal records the actual runtime row count for the tracker attached to ctx.
func SetTotal(ctx context.Context, rows int64) {
	ctxTracker := FromContext(ctx)
	if ctxTracker == nil {
		return
	}

	ctxTracker.SetTotal(rows)
}

// SetWorkers records the effective worker count for the tracker attached to ctx.
func SetWorkers(ctx context.Context, workers int) {
	ctxTracker := FromContext(ctx)
	if ctxTracker == nil {
		return
	}

	ctxTracker.SetWorkers(workers)
}

// SetStage records the current stage for the tracker attached to ctx.
func SetStage(ctx context.Context, stage string) {
	ctxTracker := FromContext(ctx)
	if ctxTracker == nil {
		return
	}

	ctxTracker.SetStage(WorkerFromContext(ctx), stage)
}

// AddBatch records one completed driver write unit for the tracker attached to ctx.
func AddBatch(ctx context.Context, rows int64, elapsed time.Duration) {
	ctxTracker := FromContext(ctx)
	if ctxTracker == nil {
		return
	}

	ctxTracker.AddBatch(WorkerFromContext(ctx), rows, elapsed)
}
