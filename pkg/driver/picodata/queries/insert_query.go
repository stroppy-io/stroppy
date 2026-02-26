package queries

import (
	"fmt"
	"strings"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func NewInsertValues(
	generators Generators,
	descriptor *stroppy.InsertDescriptor,
	valuesOut []any,
) error {
	genIDs := InsertGenIDs(descriptor)

	return GenParamValues(genIDs, generators, valuesOut)
}

func InsertSQL(descriptor *stroppy.InsertDescriptor) string {
	cols := InsertColumns(descriptor)

	sb := strings.Builder{}
	fmt.Fprintf(
		&sb,
		"insert into %s (%s) values (",
		descriptor.GetTableName(),
		strings.Join(cols, ", "),
	)

	for i := range cols {
		if i > 0 {
			sb.WriteString(", ")
		}

		fmt.Fprintf(&sb, "$%d", i+1)
	}

	sb.WriteString(")")

	return sb.String()
}

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
