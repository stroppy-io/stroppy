package api

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *PanelService) NotifyRun(ctx context.Context, stroppyRun *stroppy.StroppyRun) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, postgres.WithReadCommitted(ctx, p.txManager,
		func(ctx context.Context) error {
			if lo.Contains(
				[]stroppy.Status{
					stroppy.Status_STATUS_COMPLETED,
					stroppy.Status_STATUS_FAILED,
					stroppy.Status_STATUS_CANCELLED,
				},
				stroppyRun.GetStatus(),
			) {
				go p.stopCrossplaneAutomationOnStroppyRunFinish(&panel.Ulid{Id: stroppyRun.GetId().GetValue()})
				return nil
			}
			return p.stroppyRunRepo.Exec(ctx,
				orm.StroppyRun.Update().
					Set(
						orm.StroppyRun.Status.Set(int32(stroppyRun.GetStatus())),
						orm.StroppyRun.RunInfo.Set(orm.MessageToSliceByte(stroppyRun)),
						orm.StroppyRun.UpdatedAt.Set(time.Now()),
					).
					Where(orm.StroppyRun.Id.Eq(stroppyRun.GetId().GetValue())),
			)
		})
}

func (p *PanelService) NotifyStep(ctx context.Context, stroppyStepRun *stroppy.StroppyStepRun) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, postgres.WithReadCommitted(ctx, p.txManager,
		func(ctx context.Context) error {
			return p.stroppyStepRepo.Exec(ctx,
				orm.StroppyStep.Update().
					Set(
						orm.StroppyStep.StepInfo.Set(orm.MessageToSliceByte(stroppyStepRun)),
						orm.StroppyStep.Status.Set(int32(stroppyStepRun.GetStatus())),
						orm.StroppyStep.UpdatedAt.Set(time.Now()),
					).
					Where(orm.StroppyStep.Id.Eq(stroppyStepRun.GetId().GetValue())),
			)
		})
}
