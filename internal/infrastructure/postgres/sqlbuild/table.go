package sqlbuild

import (
	"fmt"

	"github.com/Masterminds/squirrel"
)

var PgSqlBuilder = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

func Sq() squirrel.StatementBuilderType {
	return PgSqlBuilder
}

type Table string

func (t Table) String() string {
	return string(t)
}

func (t Table) Prefix() string {
	return string(t) + "."
}

func (t Table) field(name string) string {
	return string(t) + "." + name
}

func (t Table) Eq(fieldName string, value any) squirrel.Eq {
	return squirrel.Eq{t.field(fieldName): value}
}

func (t Table) SelectAllFields() string {
	return string(t) + ".*"
}

func (t Table) SelectAll() squirrel.SelectBuilder {
	return PgSqlBuilder.Select("*").From(string(t))
}

func (t Table) Select(fields ...string) squirrel.SelectBuilder {
	return PgSqlBuilder.Select(fields...).From(string(t))
}

func (t Table) Insert(values map[string]interface{}) squirrel.InsertBuilder {
	return PgSqlBuilder.Insert(string(t)).SetMap(values)
}

func (t Table) Update(values map[string]interface{}) squirrel.UpdateBuilder {
	return PgSqlBuilder.Update(string(t)).SetMap(values)
}

func (t Table) UpdateByID(id string, values map[string]interface{}) squirrel.UpdateBuilder {
	return t.Update(values).Where(squirrel.Eq{"id": id})
}

func (t Table) UpdateField(name string, value any) squirrel.UpdateBuilder {
	return PgSqlBuilder.Update(string(t)).Set(name, value)
}

func (t Table) UpdateFieldByID(id string, name string, value any) squirrel.UpdateBuilder {
	return t.UpdateField(name, value).Where(squirrel.Eq{"id": id})
}

func (t Table) InsertOnConflictUpdate(values map[string]interface{}, update squirrel.Sqlizer) squirrel.InsertBuilder {
	updateSql, updateArgs, err := update.ToSql()
	if err != nil {
		panic(err)
	}

	return PgSqlBuilder.
		Insert(string(t)).
		SetMap(values).Suffix("ON CONFLICT DO "+updateSql, updateArgs...)
}

func (t Table) Delete() squirrel.DeleteBuilder {
	return PgSqlBuilder.Delete(string(t))
}

func (t Table) DeleteByID(id string) squirrel.DeleteBuilder {
	return PgSqlBuilder.Delete(string(t)).Where(squirrel.Eq{"id": id})
}

func (t Table) Join(other Table, left, right string) string {
	return fmt.Sprintf(
		"%s ON %s.%s = %s.%s",
		string(other),
		string(t),
		left,
		string(other),
		right,
	)
}
