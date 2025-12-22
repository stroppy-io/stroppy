package queries

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func newIndex(
	tableName string,
	index *stroppy.IndexDescriptor,
) (*stroppy.DriverTransaction, error) { //nolint: unparam // maybe later
	return &stroppy.DriverTransaction{
		Queries: []*stroppy.DriverQuery{
			{
				Name: "create_index_" + index.GetName(),
				Request: "CREATE INDEX IF NOT EXISTS " +
					index.GetName() + " ON " +
					tableName + " (" + strings.Join(index.GetColumns(), ", ") + ");",
			},
		},
	}, nil
}

func newCreateTable(
	tableName string,
	columns []*stroppy.ColumnDescriptor,
) (*stroppy.DriverTransaction, error) { //nolint: unparam // maybe later
	columnsStr := make([]string, 0, len(columns))
	pkColumns := make([]string, 0)

	for _, column := range columns {
		constants := make([]string, 0)

		if column.GetPrimaryKey() {
			pkColumns = append(pkColumns, column.GetName())
		}

		if !column.GetNullable() {
			constants = append(constants, "NOT NULL")
		}

		if column.GetUnique() {
			constants = append(constants, "UNIQUE")
		}

		if column.GetConstraint() != "" {
			constants = []string{column.GetConstraint()}
		}

		columnsStr = append(columnsStr, fmt.Sprintf(
			"%s %s %s",
			column.GetName(),
			column.GetSqlType(),
			strings.Join(constants, " "),
		))
	}

	if len(pkColumns) > 0 {
		columnsStr = append(columnsStr, "PRIMARY KEY ("+strings.Join(pkColumns, ", ")+")")
	}

	return &stroppy.DriverTransaction{
		Queries: []*stroppy.DriverQuery{
			{
				Name: "create_table_" + tableName,
				Request: "CREATE TABLE IF NOT EXISTS " +
					tableName + " (" + strings.Join(columnsStr, ", ") + ");",
			},
		},
	}, nil
}

//goland:noinspection t
func NewCreateTable(
	lg *zap.Logger,
	descriptor *stroppy.TableDescriptor,
) (*stroppy.DriverTransaction, error) {
	lg.Debug("build table",
		zap.String("name", descriptor.GetName()),
		zap.Any("columns", descriptor.GetColumns()))

	createTableQ, err := newCreateTable(descriptor.GetName(), descriptor.GetColumns())
	if err != nil {
		return nil, fmt.Errorf("can't create table query due to: %w", err)
	}

	lg.Debug("create table query",
		zap.String("name", descriptor.GetName()),
		zap.Any("columns", descriptor.GetColumns()),
		zap.Any("query", createTableQ),
		zap.Error(err),
	)

	for _, index := range descriptor.GetTableIndexes() {
		indexQ, err := newIndex(descriptor.GetName(), index)
		if err != nil {
			return nil, fmt.Errorf("can't create table query due to: %w", err)
		}

		lg.Debug(
			"create index query",
			zap.String("name", descriptor.GetName()),
			zap.Any("index", index),
			zap.Any("query", indexQ),
			zap.Error(err),
		)

		createTableQ.Queries = append(createTableQ.Queries, indexQ.GetQueries()...)
	}

	return createTableQ, nil
}
