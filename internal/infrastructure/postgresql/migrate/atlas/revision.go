package atlas

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"

	"ariga.io/atlas/sql/migrate"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlbuild"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlcast"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlerr"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlexec"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlscan"
)

var (
	revisionTableColumns = []string{
		"version",
		"description",
		"type",
		"applied",
		"total",
		"executed_at",
		"execution_time",
		"error",
		"error_stmt",
		"hash",
		"partial_hashes",
		"operator_version",
	}

	createTableSQL = `CREATE TABLE IF NOT EXISTS atlas_migrations (
		version TEXT NOT NULL PRIMARY KEY,
		description TEXT NOT NULL,
		type BIGINT NOT NULL,
		applied BIGINT NOT NULL,
		total BIGINT NOT NULL,
		executed_at TIMESTAMPTZ NOT NULL,
		execution_time BIGINT NOT NULL,
		error TEXT NOT NULL,
		error_stmt TEXT NOT NULL,
		hash TEXT NOT NULL,
		partial_hashes TEXT[] DEFAULT '{}',
		operator_version TEXT NOT NULL
	);
`
)

func arrtoToPg(s []string) pgtype.Array[string] {
	if s == nil {
		return pgtype.Array[string]{}
	}
	return pgtype.Array[string]{Elements: s, Valid: true}
}

func intToPg(i int) pgtype.Int8 {
	return pgtype.Int8{Int64: int64(i), Valid: true}
}

type Revision struct {
	Version         pgtype.Text          `db:"version"`          // Version of the migration.
	Description     pgtype.Text          `db:"description"`      // Description of this migration.
	Type            pgtype.Int8          `db:"type"`             // Type of the migration.
	Applied         pgtype.Int8          `db:"applied"`          // Applied amount of statements in the migration.
	Total           pgtype.Int8          `db:"total"`            // Total amount of statements in the migration.
	ExecutedAt      pgtype.Timestamptz   `db:"executed_at"`      // ExecutedAt is the starting point of execution.
	ExecutionTime   pgtype.Int8          `db:"execution_time"`   // ExecutionTime of the migration.
	Error           pgtype.Text          `db:"error"`            // Error of the migration, if any occurred.
	ErrorStmt       pgtype.Text          `db:"error_stmt"`       // ErrorStmt is the statement that raised Error.
	Hash            pgtype.Text          `db:"hash"`             // Hash of migration file.
	PartialHashes   pgtype.Array[string] `db:"partial_hashes"`   // PartialHashes is the hashes of applied statements.
	OperatorVersion pgtype.Text          `db:"operator_version"` // OperatorVersion that executed this migration.
}

func (r *Revision) ToRevision() *migrate.Revision {
	return &migrate.Revision{
		Version:         sqlcast.PgTextToStr(r.Version),
		Description:     sqlcast.PgTextToStr(r.Description),
		Type:            migrate.RevisionType(r.Type.Int64),
		Applied:         int(r.Applied.Int64),
		Total:           int(r.Total.Int64),
		ExecutedAt:      sqlcast.PgTimestamptzToTime(r.ExecutedAt),
		ExecutionTime:   time.Duration(r.ExecutionTime.Int64),
		Error:           sqlcast.PgTextToStr(r.Error),
		ErrorStmt:       sqlcast.PgTextToStr(r.ErrorStmt),
		Hash:            sqlcast.PgTextToStr(r.Hash),
		PartialHashes:   r.PartialHashes.Elements,
		OperatorVersion: sqlcast.PgTextToStr(r.OperatorVersion),
	}
}

func revisionToDb(r *migrate.Revision) *Revision {
	return &Revision{
		Version:         sqlcast.StrToPgText(r.Version),
		Description:     sqlcast.StrToPgText(r.Description),
		Type:            intToPg(int(r.Type)),
		Applied:         intToPg(r.Applied),
		Total:           intToPg(r.Total),
		ExecutedAt:      sqlcast.TimeToPgTimestamptz(r.ExecutedAt),
		ExecutionTime:   intToPg(int(r.ExecutionTime.Milliseconds())),
		Error:           sqlcast.StrToPgText(r.Error),
		ErrorStmt:       sqlcast.StrToPgText(r.ErrorStmt),
		Hash:            sqlcast.StrToPgText(r.Hash),
		PartialHashes:   arrtoToPg(r.PartialHashes),
		OperatorVersion: sqlcast.StrToPgText(r.OperatorVersion),
	}
}

const (
	migrationsTableName = "atlas_migrations"
	publicSchema        = "public"
)

type RevisionReaderWriter struct {
	exec sqlexec.Executor
}

func NewRevisionReaderWriter(exec sqlexec.Executor) (*RevisionReaderWriter, error) {
	_, err := exec.Exec(context.Background(), createTableSQL)
	if err != nil {
		return nil, err
	}
	return &RevisionReaderWriter{
		exec: exec,
	}, nil
}

func (r *RevisionReaderWriter) Ident() *migrate.TableIdent {
	return &migrate.TableIdent{
		Name: migrationsTableName, Schema: publicSchema,
	}
}

func (r *RevisionReaderWriter) ReadRevisions(ctx context.Context) ([]*migrate.Revision, error) {
	ret := make([]*migrate.Revision, 0)
	sql, args, err := sqlbuild.PgSqlBuilder.
		Select(revisionTableColumns...).
		From(migrationsTableName).
		ToSql()
	if err != nil {
		if sqlerr.IsNotFound(err) {
			return ret, nil
		}
		return ret, err
	}
	revisions, err := sqlscan.Scan[Revision]().Multi(r.exec.Query(ctx, sql, args...))
	if err != nil {
		return ret, err
	}
	for _, r := range revisions {
		ret = append(ret, r.ToRevision())
	}
	return ret, nil
}

func (r *RevisionReaderWriter) ReadRevision(ctx context.Context, s string) (*migrate.Revision, error) {
	sql, args, err := sqlbuild.PgSqlBuilder.
		Select(revisionTableColumns...).
		From(migrationsTableName).
		Where("version = ?", s).
		ToSql()
	if err != nil {
		return nil, err
	}
	revision, err := sqlscan.Scan[Revision]().Single(r.exec.Query(ctx, sql, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, migrate.ErrRevisionNotExist
		}
		return nil, err
	}
	return revision.ToRevision(), nil
}

func (r *RevisionReaderWriter) WriteRevision(ctx context.Context, revision *migrate.Revision) error {
	rev := revisionToDb(revision)
	update := make([]string, 0)
	for _, i := range revisionTableColumns {
		if i == "version" {
			continue
		}
		update = append(update, fmt.Sprintf("%[1]s = EXCLUDED.%[1]s", i))
	}
	sql, args, err := sqlbuild.PgSqlBuilder.
		Insert(migrationsTableName).
		Columns(revisionTableColumns...).
		Values(
			rev.Version,
			rev.Description,
			rev.Type,
			rev.Applied,
			rev.Total,
			rev.ExecutedAt,
			rev.ExecutionTime,
			rev.Error,
			rev.ErrorStmt,
			rev.Hash,
			rev.PartialHashes,
			rev.OperatorVersion,
		).Suffix(
		fmt.Sprintf("ON CONFLICT (version) DO UPDATE SET %s;", strings.Join(update, ","))).
		ToSql()
	if err != nil {
		return err
	}

	_, err = r.exec.Exec(ctx, sql, args...)
	return err
}

func (r *RevisionReaderWriter) DeleteRevision(ctx context.Context, s string) error {
	sql, args, err := sqlbuild.PgSqlBuilder.
		Delete(migrationsTableName).
		Where("version = ?", s).ToSql()
	if err != nil {
		return err
	}

	_, err = r.exec.Exec(ctx, sql, args...)
	return err
}
