package queries

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
)

func newQuery(
	generators Generators,
	descriptor *stroppy.QueryDescriptor,
) (*stroppy.DriverQuery, error) {
	paramsValues := make([]*stroppy.Value, 0)

	for _, column := range descriptor.GetParams() {
		genID := NewGeneratorID(
			descriptor.GetName(),
			column.GetName(),
		)
		gen, ok := generators.Get(genID)

		if !ok {
			return nil, fmt.Errorf("no generator for column '%s'", genID) //nolint: err113
		}

		protoValue, err := gen.Next()
		if err != nil {
			return nil, fmt.Errorf(
				"failed to generate value for column '%s': %w",
				genID,
				err,
			)
		}

		paramsValues = append(paramsValues, protoValue)
	}

	resSQL := descriptor.GetSql()

	for idx, param := range descriptor.GetParams() {
		// TODO: evaluate replace regex
		resSQL = strings.ReplaceAll(
			resSQL,
			fmt.Sprintf("${%s}", param.GetName()),
			fmt.Sprintf("$%d", idx+1),
		)
	}

	return &stroppy.DriverQuery{
		Name:    descriptor.GetName(),
		Request: resSQL,
		Params:  paramsValues,
	}, nil
}

func NewQuery(
	_ context.Context,
	lg *zap.Logger,
	generators Generators,
	// buildContext *stroppy.StepContext,
	descriptor *stroppy.QueryDescriptor,
) (*stroppy.DriverTransaction, error) {
	lg.Debug("build query",
		zap.String("name", descriptor.GetName()),
		zap.String("query", descriptor.GetSql()),
		zap.Any("params", descriptor.GetParams()),
	)

	query, err := newQuery(generators, descriptor)
	if err != nil { // TODO: add ctx.Err() check
		return nil, fmt.Errorf("can't create new query due to: %w", err)
	}

	return &stroppy.DriverTransaction{
		Queries: []*stroppy.DriverQuery{query},
	}, nil
}
