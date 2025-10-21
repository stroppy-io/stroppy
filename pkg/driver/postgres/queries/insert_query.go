package queries

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
)

func NewInsertQuery(
	_ context.Context,
	lg *zap.Logger,
	generators Generators,
	descriptor *stroppy.InsertDescriptor,
) (*stroppy.DriverTransaction, error) {
	genIDs := insertGenIDs(descriptor)

	params, err := genParamValues(genIDs, generators)
	if err != nil {
		return nil, err
	}

	var resSQL string

	switch descriptor.GetMethod() {
	case stroppy.InsertMethod_COPY_FROM:
		resSQL = badInsertSQL(descriptor)
	case stroppy.InsertMethod_PLAIN_QUERY:
		resSQL = insertSQL(descriptor)
	default:
		lg.Panic("unexpected proto.InsertMethod")
	}

	method := descriptor.GetMethod()

	return &stroppy.DriverTransaction{
		Queries: []*stroppy.DriverQuery{{
			Name:    descriptor.GetName(),
			Request: resSQL,
			Params:  params,
			Method:  &method,
		}},
		IsolationLevel: 0,
	}, nil
}

func badInsertSQL(descriptor *stroppy.InsertDescriptor) string {
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

func insertGenIDs(descriptor *stroppy.InsertDescriptor) []GeneratorID {
	genIDs := make([]GeneratorID, 0, len(descriptor.GetParams())+len(descriptor.GetGroups()))
	for _, param := range descriptor.GetParams() {
		genIDs = append(genIDs, NewGeneratorID(descriptor.GetName(), param.GetName()))
	}

	for _, group := range descriptor.GetGroups() {
		genIDs = append(genIDs, NewGeneratorID(descriptor.GetName(), group.GetName()))
	}

	return genIDs
}
