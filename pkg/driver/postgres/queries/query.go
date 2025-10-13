package queries

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	cmap "github.com/orcaman/concurrent-map/v2"
	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
)

var (
	ErrNoColumnGen      = errors.New("no generator for column")
	ErrNoGroupGen       = errors.New("no generator for group")
	ErrInvalidGroupType = errors.New("invalid group type")
)

// FIXME: is there any better place for it? delete comment if no.
var reStorage = cmap.New[*regexp.Regexp]() //nolint:gochecknoglobals // it's just works

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
			return nil, fmt.Errorf("%w: '%s'", ErrNoColumnGen, genID)
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

	for _, group := range descriptor.GetGroups() {
		genID := NewGeneratorID(
			descriptor.GetName(),
			group.GetName(),
		)
		gen, ok := generators.Get(genID)

		if !ok {
			return nil, fmt.Errorf("%w: '%s'", ErrNoGroupGen, genID)
		}

		protoValues, err := gen.Next()
		if err != nil {
			return nil, fmt.Errorf(
				"failed to generate values for group '%s': %w",
				genID,
				err,
			)
		}

		list, ok := protoValues.GetType().(*stroppy.Value_List_)
		if !ok {
			return nil, fmt.Errorf(
				"%w: '%T' != 'Value_List_': value is '%v'",
				ErrInvalidGroupType,
				protoValues.GetType(),
				protoValues.GetType(),
			)
		}

		paramsValues = append(paramsValues, list.List.GetValues()...)
	}

	resSQL := descriptor.GetSql()

	params := descriptor.GetParams()
	for _, group := range descriptor.GetGroups() {
		params = append(params, group.GetParams()...)
	}

	for idx, param := range params {
		if pattern := param.GetReplaceRegex(); pattern != "" {
			// TODO: add pattern validation at the config reading stage
			re, ok := reStorage.Get(pattern)
			if !ok {
				re = regexp.MustCompile(pattern)
				reStorage.Set(pattern, re)
			}

			re.ReplaceAllString(resSQL, fmt.Sprintf("$%d", idx+1))
		} else { // fallback to name replace
			resSQL = strings.ReplaceAll(
				resSQL,
				fmt.Sprintf("${%s}", param.GetName()),
				fmt.Sprintf("$%d", idx+1),
			)
		}
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
		return nil, fmt.Errorf("can't create new query '%s' due to: %w", descriptor.GetName(), err)
	}

	return &stroppy.DriverTransaction{
		Queries: []*stroppy.DriverQuery{query},
	}, nil
}
