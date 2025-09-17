package queries

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
	"github.com/stroppy-io/stroppy-core/pkg/utils/errchan"
)

func newQuery(
	generators Generators,
	buildContext *stroppy.StepContext,
	descriptor *stroppy.QueryDescriptor,
) (*stroppy.DriverQuery, error) {
	paramsValues := make([]*stroppy.Value, 0)

	for _, column := range descriptor.GetParams() {
		gen, ok := generators.Get(NewGeneratorID(
			buildContext.GetStep().GetName(),
			descriptor.GetName(),
			column.GetName(),
		))
		if !ok {
			return nil, fmt.Errorf("no generator for column %s", column.GetName()) //nolint: err113
		}

		protoValue, err := gen.Next()
		if err != nil {
			return nil, fmt.Errorf(
				"failed to generate value for column %s: %w",
				column.GetName(),
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
	ctx context.Context,
	lg *zap.Logger,
	generators Generators,
	buildContext *stroppy.StepContext,
	descriptor *stroppy.QueryDescriptor,
	channel errchan.Chan[stroppy.DriverTransaction],
) {
	defer errchan.Close[stroppy.DriverTransaction](channel)
	lg.Debug("build query",
		zap.String("name", descriptor.GetName()),
		zap.String("query", descriptor.GetSql()),
		zap.Any("params", descriptor.GetParams()),
	)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			for i := uint64(0); i < descriptor.GetCount(); i++ { //nolint: intrange // allow
				if i%1000 == 0 {
					lg.Info("newQuery", zap.Uint64("i", i))
				}

				query, err := newQuery(generators, buildContext, descriptor)
				if err != nil { // TODO: add ctx.Err() check
					errchan.Send[stroppy.DriverTransaction](channel, nil, err)

					return
				}

				errchan.Send[stroppy.DriverTransaction](channel, &stroppy.DriverTransaction{
					Queries: []*stroppy.DriverQuery{query},
				}, err)
			}

			return
		}
	}
}

func NewQuerySync(
	ctx context.Context,
	lg *zap.Logger,
	generators Generators,
	buildContext *stroppy.StepContext,
	descriptor *stroppy.QueryDescriptor,
) (*stroppy.DriverTransaction, error) {
	channel := make(errchan.Chan[stroppy.DriverTransaction])

	go func() {
		NewQuery(ctx, lg, generators, buildContext, descriptor, channel)
	}()

	transactions, err := errchan.CollectCtx[stroppy.DriverTransaction](ctx, channel)
	if err != nil {
		return nil, err
	}

	queries := make([]*stroppy.DriverQuery, 0)

	for _, tx := range transactions {
		queries = append(queries, tx.GetQueries()...)
	}

	return &stroppy.DriverTransaction{
		Queries: queries,
	}, nil
}
