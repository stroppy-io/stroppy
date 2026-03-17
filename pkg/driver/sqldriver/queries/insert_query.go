package queries

import (
	"fmt"
	"strings"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func InsertGenIDs(descriptor *stroppy.InsertDescriptor) []GeneratorID {
	genIDs := make([]GeneratorID, 0, len(descriptor.GetParams())+len(descriptor.GetGroups()))
	for _, param := range descriptor.GetParams() {
		genIDs = append(genIDs, param.GetName())
	}

	for _, group := range descriptor.GetGroups() {
		genIDs = append(genIDs, group.GetName())
	}

	return genIDs
}

func InsertColumns(descriptor *stroppy.InsertDescriptor) []string {
	columns := make([]string, 0, len(descriptor.GetParams())+len(descriptor.GetGroups()))
	for _, param := range descriptor.GetParams() {
		columns = append(columns, param.GetName())
	}

	for _, group := range descriptor.GetGroups() {
		for _, param := range group.GetParams() {
			columns = append(columns, param.GetName())
		}
	}

	return columns
}

// InsertSQL builds an INSERT statement using the given dialect for placeholders.
func InsertSQL(dialect Dialect, descriptor *stroppy.InsertDescriptor) string {
	cols := InsertColumns(descriptor)

	sb := strings.Builder{}
	fmt.Fprintf(
		&sb,
		"INSERT INTO %s (%s) VALUES (",
		descriptor.GetTableName(),
		strings.Join(cols, ", "),
	)

	for i := range cols {
		if i > 0 {
			sb.WriteString(", ")
		}

		sb.WriteString(dialect.Placeholder(i))
	}

	sb.WriteString(")")

	return sb.String()
}

// BulkInsertSQL builds a multi-row INSERT statement:
// INSERT INTO t (cols) VALUES (?,?),(?,?),...
func BulkInsertSQL(dialect Dialect, descriptor *stroppy.InsertDescriptor, rowCount int) string {
	cols := InsertColumns(descriptor)
	colCount := len(cols)

	sb := strings.Builder{}
	fmt.Fprintf(
		&sb,
		"INSERT INTO %s (%s) VALUES ",
		descriptor.GetTableName(),
		strings.Join(cols, ", "),
	)

	paramIdx := 0

	for row := range rowCount {
		if row > 0 {
			sb.WriteString(", ")
		}

		sb.WriteByte('(')

		for col := range colCount {
			if col > 0 {
				sb.WriteString(", ")
			}

			sb.WriteString(dialect.Placeholder(paramIdx))
			paramIdx++
		}

		sb.WriteByte(')')
	}

	return sb.String()
}
