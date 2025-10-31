package api

import (
	"context"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlexec"
)

func dbGetter(executor sqlexec.Executor) orm.DbGetter {
	return func(ctx context.Context, operation orm.SqlOpType) orm.DB {
		return executor
	}
}

func NewUsersRepository(executor sqlexec.Executor) orm.UserRepository {
	return orm.NewUserRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
	)
}

func NewStroppyStepsRepository(executor sqlexec.Executor) orm.StroppyStepRepository {
	return orm.NewStroppyStepRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromString,
		ids.UlidToStr,
	)
}

func NewResourcesRepository(executor sqlexec.Executor) orm.ResourceRepository {
	return orm.NewResourceRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromStringPtr,
		ids.UlidToStrPtr,
	)
}

func NewRunsRepository(executor sqlexec.Executor) orm.RunRepository {
	return orm.NewRunRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromString,
		ids.UlidToStr,
	)
}
