package queries

import (
	"fmt"

	"go.uber.org/zap"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

type QueryBuilder struct {
	dialect    Dialect
	generators Generators
	lg         *zap.Logger
	insert     *stroppy.InsertDescriptor
	cols       []string
	sql        string
	genIDs     []GeneratorID
}

func NewQueryBuilder(
	lg *zap.Logger,
	dialect Dialect,
	seed uint64,
	insert *stroppy.InsertDescriptor,
) (*QueryBuilder, error) {
	gens, err := CollectInsertGenerators(seed, insert)
	if err != nil {
		return nil, fmt.Errorf("add generators for unit :%w", err)
	}

	return &QueryBuilder{
		dialect:    dialect,
		generators: gens,
		lg:         lg,
		insert:     insert,
		sql:        InsertSQL(dialect, insert),
		cols:       InsertColumns(insert),
		genIDs:     InsertGenIDs(insert),
	}, nil
}

func (q *QueryBuilder) Build(valuesOut []any) error {
	return GenParamValues(q.dialect, q.genIDs, q.generators, valuesOut)
}

func (q *QueryBuilder) SQL() string                       { return q.sql }
func (q *QueryBuilder) Columns() []string                 { return q.cols }
func (q *QueryBuilder) Count() int32                      { return q.insert.GetCount() }
func (q *QueryBuilder) TableName() string                 { return q.insert.GetTableName() }
func (q *QueryBuilder) Dialect() Dialect                  { return q.dialect }
func (q *QueryBuilder) Insert() *stroppy.InsertDescriptor { return q.insert }
func (q *QueryBuilder) Generators() Generators            { return q.generators }
