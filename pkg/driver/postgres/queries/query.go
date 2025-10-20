package queries

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	cmap "github.com/orcaman/concurrent-map/v2"
	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
)

var (
	ErrNoParamGen       = errors.New("no generator for parameter")
	ErrUnknownParamType = errors.New("unknown parameter value type")
)

// TODO: move the initialization into the validation stage
var reStorage = cmap.New[*regexp.Regexp]() //nolint:gochecknoglobals // it's just works

func newQuery(
	generators Generators,
	descriptor *stroppy.QueryDescriptor,
) (*stroppy.DriverQuery, error) {

	genIDs := queryGenIDs(descriptor)

	paramsValues, err := genParamValues(genIDs, generators)
	if err != nil {
		return nil, err
	}

	resSQL := interpolateSQL(descriptor)

	return &stroppy.DriverQuery{
		Name:    descriptor.GetName(),
		Request: resSQL,
		Params:  paramsValues,
	}, nil
}

func queryGenIDs(descriptor *stroppy.QueryDescriptor) []GeneratorID {
	var genIDs []GeneratorID
	for _, param := range descriptor.GetParams() {
		genIDs = append(genIDs, NewGeneratorID(descriptor.GetName(), param.GetName()))
	}
	for _, group := range descriptor.GetGroups() {
		genIDs = append(genIDs, NewGeneratorID(descriptor.GetName(), group.GetName()))
	}
	return genIDs
}

func interpolateSQL(descriptor *stroppy.QueryDescriptor) string {
	params := descriptor.GetParams()
	for _, group := range descriptor.GetGroups() {
		params = append(params, group.GetParams()...)
	}
	resSQL := descriptor.GetSql()
	for idx, param := range params {
		pattern := param.GetReplaceRegex()
		if pattern == "" { // fallback to name replace
			pattern = regexp.QuoteMeta(fmt.Sprintf(`${%s}`, param.GetName()))
		}
		re, ok := reStorage.Get(pattern)
		if !ok { // TODO: add pattern validation add reStorage filling at the config reading stage
			re = regexp.MustCompile(pattern)
			reStorage.Set(pattern, re)
		}
		resSQL = re.ReplaceAllString(resSQL, fmt.Sprintf(`$$%d`, idx+1))
	}
	return resSQL
}

func genParamValues(
	genIDs []GeneratorID,
	generators Generators,
) (paramsValues []*stroppy.Value, err error) {

	for _, genID := range genIDs {
		gen, ok := generators.Get(genID)

		if !ok {
			return nil, fmt.Errorf("%w: '%s'", ErrNoParamGen, genID)
		}

		protoValue, err := gen.Next()
		if err != nil {
			return nil, fmt.Errorf(
				"failed to generate value for parameter '%s': %w",
				genID,
				err,
			)
		}

		switch actual := protoValue.GetType().(type) {
		case nil:
			return nil, fmt.Errorf("nil proto value type for parameter %s", genID)
		case *stroppy.Value_List_:
			paramsValues = append(paramsValues, actual.List.GetValues()...)
		case *stroppy.Value_Bool,
			*stroppy.Value_Datetime,
			*stroppy.Value_Decimal,
			*stroppy.Value_Double,
			*stroppy.Value_Float,
			*stroppy.Value_Int32,
			*stroppy.Value_Int64,
			*stroppy.Value_Null,
			*stroppy.Value_String_,
			*stroppy.Value_Struct_,
			*stroppy.Value_Uint32,
			*stroppy.Value_Uint64,
			*stroppy.Value_Uuid:
			paramsValues = append(paramsValues, protoValue)
		default:
			return nil, fmt.Errorf("%w: '%T': value is '%v'", ErrUnknownParamType, actual, actual)
		}
	}

	return paramsValues, nil
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
