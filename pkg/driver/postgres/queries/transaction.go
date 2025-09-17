package queries

import (
	"context"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
	"github.com/stroppy-io/stroppy-core/pkg/utils/errchan"
)

func NewTransaction(
	ctx context.Context,
	lg *zap.Logger,
	generators Generators,
	buildContext *stroppy.StepContext,
	descriptor *stroppy.TransactionDescriptor,
	channel errchan.Chan[stroppy.DriverTransaction],
) {
	defer errchan.Close[stroppy.DriverTransaction](channel)
	lg.Debug("build transaction",
		zap.String("name", descriptor.GetName()))

	var queries []*stroppy.DriverQuery

	for _, query := range descriptor.GetQueries() {
		q, err := NewQuerySync(ctx, lg, generators, buildContext, query)
		if err != nil {
			errchan.Send[stroppy.DriverTransaction](channel, nil, err)

			return
		}

		queries = append(queries, q.GetQueries()...)
	}

	errchan.Send[stroppy.DriverTransaction](channel, &stroppy.DriverTransaction{
		Queries: queries,
	}, nil)
}
