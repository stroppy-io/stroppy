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
		zap.String("name", descriptor.GetName()),
		zap.Any("params", descriptor.GetParams()),
		zap.Any("groups", descriptor.GetGroups()),
	)

	tx, err := newTransaction(generators, descriptor)
	if err != nil {
		return nil, fmt.Errorf("can't create new transaction '%s' due to: %w", descriptor.GetName(), err)
	}

	return tx, nil
}

func newTransaction(
	generators Generators,
	descriptor *stroppy.TransactionDescriptor,
) (*stroppy.DriverTransaction, error) {
	// Generate transaction-level parameter values
	txGenIDs := genIDsWithPrefix(descriptor.GetName(), descriptor.GetParams(), descriptor.GetGroups())

	txParamValues, err := GenParamValues(txGenIDs, generators)
	if err != nil {
		return nil, fmt.Errorf("can't generate tx params for '%s' due to: %w", descriptor.GetName(), err)
	}

	// Build transaction param descriptors (expand groups)
	txParams := descriptor.GetParams()
	txParams = append(txParams, expandGroupParams(descriptor.GetGroups())...)

	var queries []*stroppy.DriverQuery

	for _, queryDesc := range descriptor.GetQueries() {
		query, err := newQueryWithTxParams(generators, queryDesc, txParams, txParamValues)
		if err != nil {
			return nil, fmt.Errorf("can't create query '%s' for tx '%s' due to: %w", queryDesc.GetName(), descriptor.GetName(), err)
		}

		queries = append(queries, query)
	}

	return &stroppy.DriverTransaction{
		IsolationLevel: descriptor.GetIsolationLevel(),
		Queries:        queries,
	}, nil
}

// genIDsWithPrefix generates GeneratorIDs with a specific prefix (transaction name).
func genIDsWithPrefix(prefix string, params []*stroppy.QueryParamDescriptor, groups []*stroppy.QueryParamGroup) []GeneratorID {
	genIDs := make([]GeneratorID, 0, len(params)+len(groups))
	for _, param := range params {
		genIDs = append(genIDs, NewGeneratorID(prefix, param.GetName()))
	}

	for _, group := range groups {
		genIDs = append(genIDs, NewGeneratorID(prefix, group.GetName()))
	}

	return genIDs
}
