package queries

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func NewInsertQuery(
	lg *zap.Logger,
	generators Generators,
	descriptor *stroppy.InsertDescriptor,
) (sql string, values []any, err error) {
	genIDs := InsertGenIDs(descriptor)

	values, err = GenParamValues(genIDs, generators)
	if err != nil {
		return "", nil, err
	}

	switch descriptor.GetMethod() {
	case stroppy.InsertMethod_COPY_FROM:
		sql = BadInsertSQL(descriptor)
	case stroppy.InsertMethod_PLAIN_QUERY:
		sql = insertSQL(descriptor)
	default:
		lg.Panic("unexpected proto.InsertMethod")
	}

	return sql, values, nil
}

func BadInsertSQL(descriptor *stroppy.InsertDescriptor) string {
	parts := []string{descriptor.GetTableName()}
	for _, param := range descriptor.GetParams() {
		parts = append(parts, param.GetName())
	}

	for _, group := range descriptor.GetGroups() {
		for _, param := range group.GetParams() {
			parts = append(parts, param.GetName())
		}
	}

	return strings.Join(parts, " ")
}

func insertSQL(descriptor *stroppy.InsertDescriptor) string {
	cols := make([]string, 0, len(descriptor.GetParams()))
	for _, p := range descriptor.GetParams() {
		cols = append(cols, p.GetName())
	}

	for _, g := range descriptor.GetGroups() {
		for _, p := range g.GetParams() {
			cols = append(cols, p.GetName())
		}
	}

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
		genIDs = append(genIDs, GeneratorID(param.GetName()))
	}

	for _, group := range descriptor.GetGroups() {
		genIDs = append(genIDs, GeneratorID(group.GetName()))
	}

	return genIDs
}
