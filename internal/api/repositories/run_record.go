package repositories

import (
	"context"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgres/sqlexec"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type RunRecordRepository struct {
	repo orm.RunRecordRepository
}

func NewRunRecordRepository(executor sqlexec.Executor) RunRecordRepository {
	getter := func(ctx context.Context, operation orm.SqlOpType) orm.DB {
		return executor
	}
	return RunRecordRepository{
		repo: orm.NewRunRecordRepository(
			getter,
			ids.UlidFromString,
			ids.UlidToStr,
			ids.UlidFromString,
			ids.UlidToStr,
			ids.UlidFromStringPtr,
			ids.UlidToStrPtr,
		),
	}
}

func (r RunRecordRepository) FindRunRecord(ctx context.Context, id string) (*panel.RunRecord, error) {
	return r.repo.GetBy(ctx, orm.RunRecord.SelectAll().Where(orm.RunRecord.Id.Eq(id)))
}
