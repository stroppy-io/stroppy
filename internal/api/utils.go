package api

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql/sqlexec"
)

func empty() (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func dbGetter(executor sqlexec.Executor) orm.DbGetter {
	return func(ctx context.Context, operation orm.SqlOpType) orm.DB {
		return executor
	}
}

func timestamptzToTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
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

func NewCloudResourceRepository(executor sqlexec.Executor) orm.CloudResourceRepository {
	return orm.NewCloudResourceRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromStringPtr,
		ids.UlidToStrPtr,
	)
}
func NewRunRecordRepository(executor sqlexec.Executor) orm.RunRecordRepository {
	return orm.NewRunRecordRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromStringPtr,
		ids.UlidToStrPtr,
	)
}
func NewCloudAutomationRepository(executor sqlexec.Executor) orm.CloudAutomationRepository {
	return orm.NewCloudAutomationRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromStringPtr,
		ids.UlidToStrPtr,
	)
}

func NewStroppyRunRepository(executor sqlexec.Executor) orm.StroppyRunRepository {
	return orm.NewStroppyRunRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
	)
}

func NewStroppyStepRepository(executor sqlexec.Executor) orm.StroppyStepRepository {
	return orm.NewStroppyStepRepository(
		dbGetter(executor),
		ids.UlidFromString,
		ids.UlidToStr,
		ids.UlidFromString,
		ids.UlidToStr,
	)
}
