package pool

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
)

type loggerTracer struct {
	tracelog.TraceLog
	queries sync.Map
}

type queryTrace struct {
	started time.Time
	sql     string
	args    []any
}

func (t *loggerTracer) TraceQueryStart(
	ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData,
) context.Context {
	if t.LogLevel >= tracelog.LogLevelInfo {
		return t.TraceLog.TraceQueryStart(ctx, conn, data)
	}

	if t.LogLevel == tracelog.LogLevelNone {
		return ctx
	}

	value, ok := t.queries.Load(conn)
	if !ok {
		value, _ = t.queries.LoadOrStore(conn, &queryTrace{})
	}

	state, ok := value.(*queryTrace)
	if !ok {
		panic("unexpected query trace state")
	}

	*state = queryTrace{started: time.Now(), sql: data.SQL, args: data.Args}

	return ctx
}

func (t *loggerTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if t.LogLevel >= tracelog.LogLevelInfo {
		t.TraceLog.TraceQueryEnd(ctx, conn, data)

		return
	}

	if t.LogLevel == tracelog.LogLevelNone {
		return
	}

	value, ok := t.queries.Load(conn)
	if !ok {
		return
	}

	state, ok := value.(*queryTrace)
	if !ok {
		panic("unexpected query trace state")
	}

	if data.Err != nil {
		t.Logger.Log(ctx, tracelog.LogLevelError, "Query", map[string]any{
			"sql": state.sql, "args": errorQueryArgs(state.args), "err": data.Err,
			t.Config.TimeKey: time.Since(state.started),
		})
	}

	*state = queryTrace{}

	if conn != nil && conn.IsClosed() {
		t.queries.Delete(conn)
	}
}

func (t *loggerTracer) TraceAcquireStart(
	ctx context.Context, p *pgxpool.Pool, data pgxpool.TraceAcquireStartData,
) context.Context {
	if t.LogLevel == tracelog.LogLevelNone {
		return ctx
	}

	return t.TraceLog.TraceAcquireStart(ctx, p, data)
}

func (t *loggerTracer) TraceAcquireEnd(ctx context.Context, p *pgxpool.Pool, data pgxpool.TraceAcquireEndData) {
	if t.LogLevel != tracelog.LogLevelNone {
		t.TraceLog.TraceAcquireEnd(ctx, p, data)
	}
}

func errorQueryArgs(args []any) []any {
	const limit = 64

	out := make([]any, len(args))
	for i, value := range args {
		switch v := value.(type) {
		case []byte:
			value = hex.EncodeToString(v[:min(len(v), limit)])
			if len(v) >= limit {
				value = fmt.Sprintf("%s (truncated %d bytes)", value, len(v)-limit)
			}
		case string:
			if len(v) > limit {
				end := 0
				for end < limit {
					_, width := utf8.DecodeRuneInString(v[end:])
					end += width
				}

				if end < len(v) {
					value = fmt.Sprintf("%s (truncated %d bytes)", v[:end], len(v)-end)
				}
			}
		}

		out[i] = value
	}

	return out
}
