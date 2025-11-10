package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ips"
	"go.uber.org/zap"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/sourcegraph/conc/pool"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/embed"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/resource"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/timestamps"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	postgres "github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/postgresql"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type CloudAutomationConfig struct {
	AutomationTTL        time.Duration `mapstructure:"automation_ttl" default:"4h" validate:"required"`
	CreationTimeout      time.Duration `mapstructure:"creation_timeout" default:"15m" validate:"required"`
	OrioleBaseImage      string        `mapstructure:"oriole_base_image" validate:"required"`
	StroppyTpccBaseImage string        `mapstructure:"stroppy_base_image" validate:"required"`
}

func (p *PanelService) GetAutomation(ctx context.Context, ulid *panel.Ulid) (*panel.CloudAutomation, error) {
	automation, err := p.cloudAutomationRepo.GetBy(
		ctx,
		orm.CloudAutomation.SelectAll().Where(orm.CloudAutomation.Id.Eq(ulid.GetId())),
	)
	if err != nil {
		return nil, err
	}
	return automation, nil
}

func (p *PanelService) BackgroundCheckAutomationStatus(ctx context.Context) error {
	p.logger.Info("BackgroundCheckAutomationStatus started")
	automations, err := p.cloudAutomationRepo.ListBy(ctx, orm.CloudAutomation.SelectAll().
		Where(orm.CloudAutomation.Status.Any(
			int32(panel.Status_STATUS_RUNNING),
			int32(panel.Status_STATUS_IDLE),
		)))
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	workPool := pool.New().WithContext(ctx).WithFailFast().WithFirstError()
	for _, automation := range automations {
		workPool.Go(func(ctx context.Context) error {
			return postgres.WithReadUncommitted(ctx, p.txManager, func(ctx context.Context) error {
				if automation.GetTiming().GetCreatedAt().AsTime().Add(p.automateConfig.AutomationTTL).Before(time.Now()) {
					return p.stopCrossplaneAutomation(ctx, automation, panel.Status_STATUS_FAILED)
				}
				return p.updateCrossplaneAutomation(ctx, automation)
			})
		})
	}
	return workPool.Wait()
}

var (
	ErrDatabaseRunnerClusterMustHaveExactlyOneMachine = fmt.Errorf("database runner cluster must have exactly one machine")
	ErrWorkloadRunnerClusterMustHaveExactlyOneMachine = fmt.Errorf("workload runner cluster must have exactly one machine")
)

func (p *PanelService) RunAutomation(ctx context.Context, request *panel.RunAutomationRequest) (*panel.RunRecord, error) {
	if len(request.GetDatabase().GetRunnerCluster().GetMachines()) != 1 {
		return nil, ErrDatabaseRunnerClusterMustHaveExactlyOneMachine
	}
	if len(request.GetWorkload().GetRunnerCluster().GetMachines()) != 1 {
		return nil, ErrWorkloadRunnerClusterMustHaveExactlyOneMachine
	}
	if request.GetDatabase().GetDatabaseType() != panel.Database_TYPE_POSTGRES_ORIOLE {
		return nil, connect.NewError(
			connect.CodeUnimplemented,
			errors.New("unsupported database type"),
		)
	}

	if request.GetWorkload().GetWorkloadType() != panel.Workload_TYPE_TPCC {
		return nil, connect.NewError(
			connect.CodeUnimplemented,
			errors.New("unsupported workload type"),
		)
	}

	user, err := p.getUserFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	cloudBuilder, err := resource.DispatchCloudBuilder(request.GetUsingCloudProvider(), &p.k8sConfig.Crossplane)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnimplemented, err)
	}
	exsitedIps, err := p.sqlcRepo.GetAllResourcesIps(ctx, []int32{
		int32(panel.Status_STATUS_RUNNING),
		int32(panel.Status_STATUS_IDLE),
	})
	if err != nil {
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to get running resources ips: %w", err),
		)
	}
	databaseInternalIp, err := ips.FirstFreeYandexIP(
		p.k8sConfig.Crossplane.YandexCloudBuilderConfig.DefaultNetworkCidrBlock,
		exsitedIps,
	)
	if err != nil {
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to find free ip: %w", err),
		)
	}
	databaseInternalIpStr := databaseInternalIp.String()
	p.logger.Info("Found free ip for database", zap.String("ip", databaseInternalIpStr))
	workloadInternalIp, err := ips.FirstFreeYandexIP(
		p.k8sConfig.Crossplane.YandexCloudBuilderConfig.DefaultNetworkCidrBlock,
		append(exsitedIps, databaseInternalIpStr),
	)
	if err != nil {
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to find free ip: %w", err),
		)
	}
	workloadInternalIpStr := workloadInternalIp.String()
	p.logger.Info("Found free ip for workload", zap.String("ip", workloadInternalIpStr))

	newAutomationId := ids.NewUlid()
	dbDeployScript, err := embed.GetOrioleInstallScript()
	if err != nil {
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to get oriole install script: %w", err),
		)
	}
	databaseMachineName := fmt.Sprintf("stroppy-crossplane-database-%s", strings.ToLower(newAutomationId.GetId()))
	// TODO: AUTOMATE THIS LATER
	request.GetDatabase().GetRunnerCluster().GetMachines()[0].BaseImageId = &p.automateConfig.OrioleBaseImage
	request.GetDatabase().GetRunnerCluster().GetMachines()[0].StaticInternalIp = &databaseInternalIpStr
	databaseResourcesTree, err := cloudBuilder.NewSingleVmResource(
		databaseMachineName,
		request.GetDatabase().GetRunnerCluster().GetMachines()[0],
		dbDeployScript,
	)
	if err != nil {
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to create database spec: %w", err),
		)
	}
	workloadDeployScript, err := embed.GetStroppyInstallScript()
	if err != nil {
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to get stroppy install script: %w", err),
		)
	}
	driverUrl := fmt.Sprintf("postgres://st-t-postgres:st-t-postgres-pass@%s:54321/st-t-postgres", databaseInternalIpStr)
	workloadDeployScript.Body = []byte(strings.ReplaceAll(string(workloadDeployScript.Body), "${DRIVER_URL}", driverUrl))
	workloadMachineName := fmt.Sprintf("stroppy-crossplane-workload-%s", strings.ToLower(newAutomationId.GetId()))
	// TODO: AUTOMATE THIS LATER
	request.GetWorkload().GetRunnerCluster().GetMachines()[0].BaseImageId = &p.automateConfig.StroppyTpccBaseImage
	request.GetWorkload().GetRunnerCluster().GetMachines()[0].StaticInternalIp = &workloadInternalIpStr
	workloadResourcesTree, err := cloudBuilder.NewSingleVmResource(
		workloadMachineName,
		request.GetWorkload().GetRunnerCluster().GetMachines()[0],
		workloadDeployScript,
	)
	if err != nil {
		return nil, connect.NewError(
			connect.CodeInternal,
			fmt.Errorf("failed to create database spec: %w", err),
		)
	}
	// TODO: Do not hardcode paths
	return postgres.WithSerializableRet(ctx, p.txManager,
		func(ctx context.Context) (*panel.RunRecord, error) {
			err = p.createCrossplaneResourcesTree(ctx, databaseResourcesTree)
			if err != nil {
				return nil, connect.NewError(
					connect.CodeInternal,
					fmt.Errorf("failed to create database resources: %w", err),
				)
			}
			err = p.createCrossplaneResourcesTree(ctx, workloadResourcesTree)
			if err != nil {
				return nil, connect.NewError(
					connect.CodeInternal,
					fmt.Errorf("failed to create workload resources: %w", err),
				)
			}
			err = p.cloudAutomationRepo.Insert(ctx, &panel.CloudAutomation{
				Id:                     newAutomationId,
				Timing:                 timestamps.NewTiming(),
				Status:                 panel.Status_STATUS_IDLE,
				DatabaseRootResourceId: databaseResourcesTree.GetId(),
				WorkloadRootResourceId: workloadResourcesTree.GetId(),
				StroppyRunId:           nil,
			})
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			newRunRecord := &panel.RunRecord{
				Id:       newAutomationId,
				AuthorId: user.GetId(),
				Timing:   timestamps.NewTiming(),
				Status:   panel.Status_STATUS_IDLE,
				Tps: &panel.Tps{
					Average: 0,
					Max:     0,
					Min:     0,
					P95Th:   0,
					P99Th:   0,
				},
				Database:          request.GetDatabase(),
				Workload:          request.GetWorkload(),
				CloudAutomationId: newAutomationId,
			}
			err = p.runRecordRepo.Insert(ctx, newRunRecord)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			return newRunRecord, nil
		},
	)
}
func (p *PanelService) CancelAutomation(ctx context.Context, ulid *panel.Ulid) (*emptypb.Empty, error) {
	automation, err := p.GetAutomation(ctx, ulid)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return &emptypb.Empty{}, postgres.WithSerializable(ctx, p.txManager,
		func(ctx context.Context) error {
			return p.stopCrossplaneAutomation(ctx, automation, panel.Status_STATUS_CANCELED)
		})
}
