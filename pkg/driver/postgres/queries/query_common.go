package queries

import (
	"errors"
	"fmt"
	"regexp"

	cmap "github.com/orcaman/concurrent-map/v2"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

var (
	ErrNoParamGen       = errors.New("no generator for parameter")
	ErrUnknownParamType = errors.New("unknown parameter value type")
	ErrNilProtoValue    = errors.New("nil proto value type for parameter")
)

// TODO: move the initialization into the validation stage
var reStorage = cmap.New[*regexp.Regexp]() //nolint:gochecknoglobals // it's just works

func GenParamValues(
	genIDs []GeneratorID,
	generators Generators,
) ([]*stroppy.Value, error) {
	var paramsValues []*stroppy.Value

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
			return nil, fmt.Errorf("%w: %s", ErrNilProtoValue, genID)
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

type generatable interface {
	GetName() string
	GetParams() []*stroppy.QueryParamDescriptor
	GetGroups() []*stroppy.QueryParamGroup
}

func genIDs(descriptor generatable) []GeneratorID {
	genIDs := make([]GeneratorID, 0, len(descriptor.GetParams())+len(descriptor.GetGroups()))
	for _, param := range descriptor.GetParams() {
		genIDs = append(genIDs, NewGeneratorID(descriptor.GetName(), param.GetName()))
	}

	for _, group := range descriptor.GetGroups() {
		genIDs = append(genIDs, NewGeneratorID(descriptor.GetName(), group.GetName()))
	}

	return genIDs
}

func interpolateSQL(
	sql string,
	params []*stroppy.QueryParamDescriptor,
	groups []*stroppy.QueryParamGroup,
) string {
	for _, group := range groups {
		params = append(params, group.GetParams()...)
	}

	idx := 0

	for _, param := range params {
		pattern := param.GetReplaceRegex()
		if pattern == "" { // fallback to name replace
			pattern = regexp.QuoteMeta(fmt.Sprintf(`${%s}`, param.GetName()))
		}

		re, ok := reStorage.Get(pattern)
		if !ok { // TODO: add pattern validation add reStorage filling at the config reading stage
			re = regexp.MustCompile(pattern)
			reStorage.Set(pattern, re)
		}

		if re.MatchString(sql) { // skip index inc if param not present
			sql = re.ReplaceAllString(sql, fmt.Sprintf(`$$%d`, idx+1))
			idx++
		}
	}

	return sql
}

// interpolateSQLWithTracking returns interpolated SQL and indices of params that were actually used.
func interpolateSQLWithTracking(
	sql string,
	params []*stroppy.QueryParamDescriptor,
	groups []*stroppy.QueryParamGroup,
) (string, []int) {
	for _, group := range groups {
		params = append(params, group.GetParams()...)
	}

	var usedIndices []int

	idx := 0

	for i, param := range params {
		pattern := param.GetReplaceRegex()
		if pattern == "" { // fallback to name replace
			pattern = regexp.QuoteMeta(fmt.Sprintf(`${%s}`, param.GetName()))
		}

		re, ok := reStorage.Get(pattern)
		if !ok {
			re = regexp.MustCompile(pattern)
			reStorage.Set(pattern, re)
		}

		if re.MatchString(sql) {
			sql = re.ReplaceAllString(sql, fmt.Sprintf(`$$%d`, idx+1))

			usedIndices = append(usedIndices, i)
			idx++
		}
	}

	return sql, usedIndices
}

// expandGroupParams expands groups into individual params.
func expandGroupParams(groups []*stroppy.QueryParamGroup) []*stroppy.QueryParamDescriptor {
	var params []*stroppy.QueryParamDescriptor
	for _, group := range groups {
		params = append(params, group.GetParams()...)
	}

	return params
}

// filterUsedParams filters param values based on used indices.
func filterUsedParams(allValues []*stroppy.Value, usedIndices []int) []*stroppy.Value {
	var result []*stroppy.Value

	for _, idx := range usedIndices {
		if idx < len(allValues) {
			result = append(result, allValues[idx])
		}
	}

	return result
}
