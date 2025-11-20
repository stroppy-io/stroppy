package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/build"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/ids"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/timestamps"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/infrastructure/orm"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (p *PanelService) ListRuns(
	ctx context.Context,
	request *panel.ListRunsRequest,
) (*panel.RunRecord_List, error) {
	user, err := p.getUserFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	q := orm.RunRecord.SelectAll()
	if limit := request.GetLimit(); limit != 0 {
		q = q.Limit(int(limit))
	}
	if offset := request.GetOffset(); offset != 0 {
		q = q.Offset(int(offset))
	}
	if request.Status != nil {
		q = q.Where(orm.RunRecord.Status.Eq(int32(request.GetStatus())))
	}
	if request.GetId() != "" {
		q = q.Where(orm.RunRecord.Id.Eq(request.GetId()))
	}
	if request.GetOnlyMine() {
		q = q.Where(orm.RunRecord.AuthorId.Eq(user.GetId().GetId()))
	}
	if request.GetTpsOrder() != nil {
		postfix := "ASC"
		if request.GetTpsOrder().GetDescending() {
			postfix = "DESC"
		}
		q = q.OrderByRaw(fmt.Sprintf(
			"(tps->>'%s') %s",
			strings.ToLower(
				strings.ReplaceAll(
					request.GetTpsOrder().GetParameterType().String(),
					"Tps_Order_TYPE_",
					"",
				),
			),
			postfix))
	}
	runs, err := p.runRecordRepo.ListBy(ctx, q)
	if err != nil {
		return nil, err
	}
	return &panel.RunRecord_List{Records: runs}, nil
}

func (p *PanelService) newTaskRetrySettings(kind panel.WorkflowTask_Type) *panel.Retry {
	taskRetryConfig, ok := p.workflowConfig.TaskRetryConfig[kind.String()]
	if !ok {
		return &panel.Retry{
			MaxAttempts: 10,
			Backoff: &panel.Retry_Backoff{
				Backoff: &panel.Retry_Backoff_Constant_{
					Constant: &panel.Retry_Backoff_Constant{
						Duration: durationpb.New(30 * time.Second),
					},
				},
			},
		}
	}
	return &panel.Retry{
		MaxAttempts: 10,
		Backoff: &panel.Retry_Backoff{
			Backoff: &panel.Retry_Backoff_Exponential_{
				Exponential: &panel.Retry_Backoff_Exponential{
					InitialInterval:     durationpb.New(taskRetryConfig.InitialInterval),
					MaxElapsedTime:      durationpb.New(taskRetryConfig.MaxElapsedTime),
					MaxInterval:         durationpb.New(taskRetryConfig.MaxInterval),
					Multiplier:          taskRetryConfig.Multiplier,
					RandomizationFactor: taskRetryConfig.RandomizationFactor,
					RetryStopDuration:   durationpb.New(taskRetryConfig.RetryStopDuration),
				},
			},
		},
	}
}

func (p *PanelService) RunStroppyInCloud(
	ctx context.Context,
	request *panel.CloudRunParams,
) (*panel.RunRecord, error) {
	user, err := p.getUserFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	runRecordId := ids.NewUlid()
	workflowId := ids.NewUlid()

	tasks := make([]*panel.WorkflowTask, 0)
	edges := make([]*panel.WorkflowEdge, 0)

	allMetadata := &panel.Metadata{Metadata: map[string]string{
		"created_by": user.GetId().GetId(),
		"service":    build.GlobalInstanceId,
	}}

	stroppyDeploymentTaskId := ids.NewUlid()
	stroppyDeploymentTask := &panel.WorkflowTask{
		Id:         stroppyDeploymentTaskId.String(),
		WorkflowId: workflowId.String(),
		Status:     panel.WorkflowTask_STATUS_PENDING,
		OnWorker:   "",
		CleanedUp:  false,
		RetryState: &panel.Retry_State{
			BackoffDuration: durationpb.New(0 * time.Second),
			BackoffValue:    timestamppb.New(time.Now()),
			Attempt:         0,
		},
		RetrySettings: p.newTaskRetrySettings(panel.WorkflowTask_TYPE_DEPLOY_STROPPY),
		Input: lo.Must(anypb.New(&panel.WorkflowTask_DeployStroppy_Input{
			StroppyRunId:          runRecordId,
			StroppyInstanceParams: request.GetStroppyInstanceTemplate(),
		})),
		Metadata: allMetadata,
	}

	collectResultsTaskId := ids.NewUlid()
	collectResultsTask := &panel.WorkflowTask{
		Id:         collectResultsTaskId.String(),
		WorkflowId: workflowId.String(),
		Status:     panel.WorkflowTask_STATUS_PENDING,
		OnWorker:   "",
		CleanedUp:  false,
		RetryState: &panel.Retry_State{
			BackoffDuration: durationpb.New(0 * time.Second),
			BackoffValue:    timestamppb.New(time.Now()),
			Attempt:         0,
		},
		RetrySettings: p.newTaskRetrySettings(panel.WorkflowTask_TYPE_COLLECT_RUN_RESULTS),
		Input: lo.Must(anypb.New(&panel.WorkflowTask_CollectRunResults_Input{
			StroppyRunId: runRecordId,
		})),
		Metadata: allMetadata,
	}

	tasks = append(tasks, stroppyDeploymentTask)
	tasks = append(tasks, collectResultsTask)
	edges = append(edges, &panel.WorkflowEdge{
		FromId:   stroppyDeploymentTaskId.String(),
		ToId:     collectResultsTaskId.String(),
		Metadata: allMetadata,
	})

	if request.GetDatabaseInstanceTemplate() != nil {
		databaseDeploymentTaskId := ids.NewUlid()
		databaseDeploymentTask := &panel.WorkflowTask{
			Id:         databaseDeploymentTaskId.String(),
			WorkflowId: workflowId.String(),
			Status:     panel.WorkflowTask_STATUS_PENDING,
			OnWorker:   "",
			CleanedUp:  false,
			RetryState: &panel.Retry_State{
				BackoffDuration: durationpb.New(0 * time.Second),
				BackoffValue:    timestamppb.New(time.Now()),
				Attempt:         0,
			},
			RetrySettings: p.newTaskRetrySettings(panel.WorkflowTask_TYPE_DEPLOY_DATABASE),
			Input: lo.Must(anypb.New(&panel.WorkflowTask_DeployDatabase_Input{
				StroppyRunId:           runRecordId,
				DatabaseInstanceParams: request.GetDatabaseInstanceTemplate(),
			})),
			Metadata: allMetadata,
		}
		tasks = append(tasks, databaseDeploymentTask)
		edges = append(edges, &panel.WorkflowEdge{
			FromId:   databaseDeploymentTaskId.String(),
			ToId:     stroppyDeploymentTaskId.String(),
			Metadata: allMetadata,
		})
	}

	err = p.workflowRepo.CreateWorkflow(ctx, &panel.Workflow{
		Id:     workflowId,
		Timing: timestamps.NewTiming(),
		Tasks:  tasks,
		Edges:  edges,
	})
	if err != nil {
		return nil, err
	}
	record := &panel.RunRecord{
		Id:             runRecordId,
		WorkflowId:     workflowId,
		AuthorId:       user.GetId(),
		Timing:         timestamps.NewTiming(),
		Status:         stroppy.Status_STATUS_IDLE,
		CloudRunParams: request,
	}
	return record, p.runRecordRepo.Insert(ctx, record)
}
