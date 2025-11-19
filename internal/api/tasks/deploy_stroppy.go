package tasks

import (
	"context"
	"errors"
	"fmt"

	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/resource"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/workflow"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type deployStroppyTaskState = workflow.TaskState[*panel.WorkflowTask_DeployStroppy_Input, *panel.WorkflowTask_DeployStroppy_Output]
type DeployStroppyTaskHandler struct {
	deploymentActor   DeploymentActor
	deploymentBuilder DeploymentBuilder
}

func NewDeployStroppyTaskHandler(
	deploymentActor DeploymentActor,
	deploymentBuilder DeploymentBuilder,
) *DeployStroppyTaskHandler {
	return &DeployStroppyTaskHandler{
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

const stroppyRunIdEnvKey = "STROPPY_CLOUD_RUN_ID"

func (d *DeployStroppyTaskHandler) Start(
	ctx context.Context,
	input *panel.WorkflowTask_DeployStroppy_Input,
) (*panel.WorkflowTask_DeployStroppy_Output, error) {
	if input.GetDatabaseDeployment() == nil {
		// TODO: don't rewrite url
	}
	instanceTemplate := input.GetStroppyInstanceTemplate()
	deploymentTemplate := instanceTemplate.GetStroppyDeployment()
	env := deploymentTemplate.GetEnvData().GetMetadata()
	env[stroppyRunIdEnvKey] = input.GetStroppyRunId().GetId()
	deployment, err := d.deploymentBuilder.BuildVmDeployment(ctx,
		instanceTemplate.GetSupportedCloud(),
		&crossplane.Deployment_Vm{
			PublicIp:    false, // NOTE: may be set outside in input
			InternalIp:  "",    // NOTE: this value will be set by DeploymentBuilder
			MachineInfo: instanceTemplate.GetMachineDeployment().GetMachineInfo(),
			Strategy: &crossplane.Deployment_Strategy{
				Strategy: &crossplane.Deployment_Strategy_Scripting_{
					Scripting: &crossplane.Deployment_Strategy_Scripting{
						Workdir: "~/stroppy",
						Cmd:     cmdOrDefault(deploymentTemplate.GetCmd()),
						Env:     env,
						FilesToWrite: []*crossplane.Deployment_Strategy_Scripting_FileToWrite{
							{
								Path:    "stroppy.ts",
								Content: deploymentTemplate.GetScriptBody(),
							},
						},
					},
				},
			},
		},
	)
	deployment, err = d.deploymentActor.CreateDeployment(ctx, deployment)
	if err != nil {
		return nil, err
	}
	return &panel.WorkflowTask_DeployStroppy_Output{
		StroppyDeployment: deployment,
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
	allResourcesReady := resource.AllDagNodesStatus(
		deployment.GetResourceDag(),
		crossplane.Resource_STATUS_READY,
	)
	if allResourcesReady {
		return panel.WorkflowTask_STATUS_COMPLETED, nil
	}
	anyDagNodeFailed := resource.AnyDagNodeInStatuses(
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
	return nil
}
