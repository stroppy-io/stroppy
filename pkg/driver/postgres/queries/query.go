package queries

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func NewQuery(
	_ context.Context,
	lg *zap.Logger,
	generators Generators,
	descriptor *stroppy.QueryDescriptor,
) (*stroppy.DriverTransaction, error) {
	lg.Debug("build query",
		zap.String("name", descriptor.GetName()),
		zap.String("query", descriptor.GetSql()),
		zap.Any("params", descriptor.GetParams()),
		zap.Any("groups", descriptor.GetGroups()),
	)

	query, err := newQuery(generators, descriptor)
	if err != nil { // TODO: add ctx.Err() check
		return nil, fmt.Errorf("can't create new query '%s' due to: %w", descriptor.GetName(), err)
	}

	return &stroppy.DriverTransaction{
		Queries: []*stroppy.DriverQuery{query},
	}, nil
}

func newQuery(
	generators Generators,
	descriptor *stroppy.QueryDescriptor,
) (*stroppy.DriverQuery, error) {
	return newQueryWithTxParams(generators, descriptor, nil, nil)
}

// newQueryWithTxParams creates a query with optional transaction-level parameters.
func newQueryWithTxParams(
	generators Generators,
	descriptor *stroppy.QueryDescriptor,
	txParams []*stroppy.QueryParamDescriptor,
	txParamValues []*stroppy.Value,
) (*stroppy.DriverQuery, error) {
	// Build combined param list: query params first, then tx params (query params override tx params)
	queryParams := descriptor.GetParams()
	queryParams = append(queryParams, expandGroupParams(descriptor.GetGroups())...)

	// Create map of query param names for conflict detection
	queryParamNames := make(map[string]bool)
	for _, param := range queryParams {
		queryParamNames[param.GetName()] = true
	}

	// Add tx params that don't conflict with query params
	combinedParams := make([]*stroppy.QueryParamDescriptor, len(queryParams))
	copy(combinedParams, queryParams)

	txParamStartIdx := len(queryParams)

	for _, txParam := range txParams {
		if !queryParamNames[txParam.GetName()] {
			combinedParams = append(combinedParams, txParam)
		}
	}

	// Interpolate SQL and track which params are used
	resSQL, usedIndices := interpolateSQLWithTracking(descriptor.GetSql(), combinedParams, nil)

	// Generate values for query params
	queryGenIDs := genIDs(descriptor)

	queryParamValues, err := GenParamValues(queryGenIDs, generators)
	if err != nil {
		return nil, err
	}

	// Build combined param values: query values first, then tx values
	combinedValues := make([]*stroppy.Value, len(queryParams))
	copy(combinedValues, queryParamValues)

	// Add tx param values (in same order as combinedParams)
	txParamIdx := 0

	for i := txParamStartIdx; i < len(combinedParams); i++ {
		// Find corresponding tx param value
		txParamName := combinedParams[i].GetName()
		for j, txParam := range txParams {
			if txParam.GetName() == txParamName {
				if j < len(txParamValues) {
					combinedValues = append(combinedValues, txParamValues[j])
				}

				break
			}
		}

		txParamIdx++
	}

	// Filter to only used params
	usedParamValues := filterUsedParams(combinedValues, usedIndices)

	return &stroppy.DriverQuery{
		Name:    descriptor.GetName(),
		Request: resSQL,
		Params:  usedParamValues,
	}, nil
}
