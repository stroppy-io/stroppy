package queries

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func NewTransaction(
	ctx context.Context,
	lg *zap.Logger,
	generators Generators,
	descriptor *stroppy.TransactionDescriptor,
) (*stroppy.DriverTransaction, error) {
	lg.Debug("build transaction",
		zap.String("name", descriptor.GetName()))

	var queries []*stroppy.DriverQuery

	for _, query := range descriptor.GetQueries() {
		q, err := NewQuery(ctx, lg, generators, query)
		if err != nil {
			return nil, fmt.Errorf(
				"can't create query for tx '%s' due to: %w",
				descriptor.GetName(),
				err,
			)
		}

		queries = append(queries, q.GetQueries()...)
	}

	return &stroppy.DriverTransaction{
		IsolationLevel: descriptor.GetIsolationLevel(),
		Queries:        queries,
	}, nil
}
