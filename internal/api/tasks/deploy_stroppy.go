package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/core/ips"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/kv"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/workflow"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type deployStroppyTaskState = workflow.TaskState[*panel.WorkflowTask_DeployStroppy_Input, *panel.WorkflowTask_DeployStroppy_Output]
type DeployStroppyTaskHandler struct {
	quotaRepository   QuotaRepository
	cidrProvider      CidrProvider
	deploymentActor   DeploymentActor
	deploymentBuilder DeploymentBuilder
}

func NewDeployStroppyTaskHandler(
	quotaRepository QuotaRepository,
	cidrProvider CidrProvider,
	deploymentActor DeploymentActor,
	deploymentBuilder DeploymentBuilder,
) *DeployStroppyTaskHandler {
	return &DeployStroppyTaskHandler{
		quotaRepository:   quotaRepository,
		cidrProvider:      cidrProvider,
		deploymentActor:   deploymentActor,
		deploymentBuilder: deploymentBuilder,
	}
}

func cmdOrDefault(s string) string {
	if s != "" {
		return s
	}
	return "sudo docker run -v ./stroppy.ts:/test.ts " +
		"-e CONNECTION_STRING=${STROPPY_CONNECTION_STRING} " +
		"-e STROPPY_CLOUD_URL=${STROPPY_CLOUD_URL} " +
		"-e STROPPY_CLOUD_RUN_ID=${STROPPY_CLOUD_RUN_ID} " +
		"-e K6_OTEL_EXPORTER_TYPE=${STROPPY_K6_OTEL_EXPORTER_TYPE} " +
		"-e K6_OTEL_METRIC_PREFIX=${STROPPY_K6_OTEL_METRIC_PREFIX} " +
		"-e K6_OTEL_SERVICE_NAME=${STROPPY_K6_OTEL_SERVICE_NAME} " +
		"-e K6_OTEL_HTTP_EXPORTER_INSECURE=${STROPPY_K6_OTEL_HTTP_EXPORTER_INSECURE} " +
		"-e K6_OTEL_HTTP_EXPORTER_ENDPOINT=${STROPPY_K6_OTEL_HTTP_EXPORTER_ENDPOINT} " +
		"-e K6_OTEL_HTTP_EXPORTER_URL_PATH=${STROPPY_K6_OTEL_HTTP_EXPORTER_URL_PATH} " +
		"-e K6_OTEL_HEADERS=\"${STROPPY_K6_OTEL_HEADERS}\" " +
		"ghcr.io/stroppy-io/stroppy:${STROPPY_VERSION}"
}

const (
	homeWorkdir                = "~/"
	stroppyDatabaseAddrEnvName = "STROPPY_DATABASE_ADDR"
	stroppyCloudRunIdEnvName   = "STROPPY_CLOUD_RUN_ID"
)

func (d *DeployStroppyTaskHandler) buildScripting(
	stroppyTemplate *panel.Template_StroppyDeployment,
	readyKV *panel.KV_Map,
) (*crossplane.Deployment_Strategy_Scripting, error) {
	filesToWrite := make([]*crossplane.FsFile, 0)
	for _, file := range stroppyTemplate.GetFiles() {
		newData, err := kv.EvalKv(file.GetContent(), readyKV)
		if err != nil {
			return nil, fmt.Errorf("failed eval Kv to file content: %s, %w", file.GetPath(), err)
		}
		filesToWrite = append(filesToWrite, &crossplane.FsFile{
			Path:    file.GetPath(),
			Content: newData,
		})
	}
	cmdToWrite, err := kv.EvalKv(stroppyTemplate.GetCmd(), readyKV)
	if err != nil {
		return nil, fmt.Errorf("failed eval Kv to cmd")
	}
	return &crossplane.Deployment_Strategy_Scripting{
		Workdir:      homeWorkdir,
		Cmd:          cmdToWrite,
		FilesToWrite: filesToWrite,
	}, nil
}

func (d *DeployStroppyTaskHandler) Start(
	ctx context.Context,
	input *panel.WorkflowTask_DeployStroppy_Input,
) (*panel.WorkflowTask_DeployStroppy_Output, error) {
	if input.GetStroppyInstanceParams().GetMachineInstance().GetPublicIp() {
		err := checkQuotaExceeded(ctx, d.quotaRepository, &crossplane.Quota{
			Cloud:   input.GetStroppyInstanceParams().GetSupportedCloud(),
			Kind:    crossplane.Quota_KIND_PUBLIC_IP_ADDRESS,
			Current: 1,
		})
		if err != nil {
			return nil, err
		}
	}
	instanceParams := input.GetStroppyInstanceParams()
	instanceTemplate := instanceParams.GetStroppyDeploymentTemplate()
	stroppyTemplate := instanceTemplate.GetStroppyDeployment()

	kv.Set(instanceParams.GetEnvKv(), stroppyCloudRunIdEnvName, input.GetStroppyRunId().GetId())

	var stroppyIp string
	if input.GetDatabaseDeployment() != nil {
		dbIp := input.GetDatabaseAssignedInternalIp()
		kv.Set(instanceParams.GetEnvKv(), stroppyDatabaseAddrEnvName, dbIp.GetValue())
		newIp, err := ips.FirstFreeIP(d.cidrProvider.UsingCidr(ctx), []string{dbIp.GetValue()})
		if err != nil {
			return nil, err
		}
		stroppyIp = newIp.String()
	} else {
		newIp, err := ips.RandomIP(d.cidrProvider.UsingCidr(ctx))
		if err != nil {
			return nil, err
		}
		stroppyIp = newIp.String()
	}

	scripting, err := d.buildScripting(stroppyTemplate, instanceParams.GetEnvKv())
	if err != nil {
		return nil, fmt.Errorf("failed to create script, %w", err)
	}

	vmDeployment, err := d.deploymentBuilder.BuildVmDeployment(ctx,
		instanceParams.GetSupportedCloud(),
		input.GetStroppyRunId(),
		&crossplane.Deployment_Vm{
			PublicIp:    false, // NOTE: may be set outside in input
			InternalIp:  &crossplane.Ip{Value: stroppyIp},
			SshUser:     instanceParams.GetMachineInstance().GetSshUser(),
			MachineInfo: instanceTemplate.GetMachineDeployment().GetMachineInfo(),
			Strategy: &crossplane.Deployment_Strategy{
				Strategy: &crossplane.Deployment_Strategy_Scripting_{
					Scripting: scripting,
				},
			},
		},
	)
	if err != nil {
		return nil, err
	}
	err = checkQuotasExceededOrDecrement(ctx, d.quotaRepository,
		instanceParams.GetSupportedCloud(),
		vmDeployment.GetUsingQuotas().GetQuotas(),
	)
	if err != nil {
		return nil, err
	}
	vmDeployment, err = d.deploymentActor.CreateDeployment(ctx, vmDeployment)
	if err != nil {
		return nil, err
	}
	return &panel.WorkflowTask_DeployStroppy_Output{
		StroppyDeployment: vmDeployment,
	}, nil
}

func (d *DeployStroppyTaskHandler) Status(
	ctx context.Context,
	state deployStroppyTaskState,
) (panel.WorkflowTask_Status, error) {
	deployment, err := d.deploymentActor.ProcessDeploymentStatus(ctx, state.GetOutput().GetStroppyDeployment())
	if err != nil {
		return panel.WorkflowTask_STATUS_UNSPECIFIED,
			errors.Join(
				err,
				workflow.ErrStatusTemproraryFailed,
				fmt.Errorf("failed to process deployment status"),
			)
	}
	allResourcesReady := allDagNodesStatus(
		deployment.GetResourceDag(),
		crossplane.Resource_STATUS_READY,
	)
	if allResourcesReady {
		return panel.WorkflowTask_STATUS_COMPLETED, nil
	}
	anyDagNodeFailed := anyDagNodeInStatuses(
		deployment.GetResourceDag(),
		[]crossplane.Resource_Status{
			crossplane.Resource_STATUS_DESTROYING,
			crossplane.Resource_STATUS_DESTROYED,
			crossplane.Resource_STATUS_DEGRADED,
		},
	)
	if anyDagNodeFailed {
		return panel.WorkflowTask_STATUS_FAILED, nil
	}
	return panel.WorkflowTask_STATUS_RUNNING, nil
}

func (d *DeployStroppyTaskHandler) Cleanup(
	ctx context.Context,
	state deployStroppyTaskState,
) error {
	if state.GetOutput().GetStroppyDeployment() != nil {
		return d.deploymentActor.DestroyDeployment(ctx, state.GetOutput().GetStroppyDeployment())
	}
	return incrementQuotas(ctx, d.quotaRepository,
		state.GetInput().GetStroppyInstanceParams().GetSupportedCloud(),
		state.GetOutput().GetStroppyDeployment().GetUsingQuotas().GetQuotas(),
	)
}
